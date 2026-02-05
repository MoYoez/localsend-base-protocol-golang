package boardcast

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/moyoez/localsend-go/share"
	"github.com/moyoez/localsend-go/tool"
	"github.com/moyoez/localsend-go/types"
	"golang.org/x/time/rate"
)

// HTTPScanOptions configures concurrency and ICMP rate limit for HTTP scan.
// Concurrency: max concurrent scan goroutines; 0 or large value = effectively unlimited (e.g. scan-now).
// RateLimitPPS: ICMP probe rate limit (packets per second); 0 = no limit.
type HTTPScanOptions struct {
	Concurrency  int // max concurrent workers
	RateLimitPPS int // 0 = no rate limit
}

// scanOneIPHTTP performs ICMP probe (host reachability), then POST register (https then http on EOF), parses response and stores device via share.SetUserScanCurrent.
// Used by ListenMulticastUsingHTTPWithTimeout and ScanOnceHTTP. Returns true if a device was discovered and stored.
func scanOneIPHTTP(targetIP string, payloadBytes []byte, httpClient *http.Client) bool {
	if !tool.QuickICMPProbe(targetIP, icmpProbeTimeout) {
		return false
	}
	protocol := "https"
	urlStr := tool.BuildScanOnceRegisterUrl(protocol, targetIP, multcastPort)
	req, err := tool.NewHTTPReqWithApplication(http.NewRequest("POST", urlStr, bytes.NewReader(payloadBytes)))
	if err != nil {
		tool.DefaultLogger.Debugf("scanOneIPHTTP: failed to create request for %s: %v", urlStr, err)
		return false
	}
	tool.DefaultLogger.Debugf("scanOneIPHTTP: sending request to %s", urlStr)
	resp, err := httpClient.Do(req)
	globalProtocol := "https"
	if err != nil {
		if strings.Contains(err.Error(), "EOF") {
			tool.DefaultLogger.Warnf("Detected error, trying to use http protocol: %v", err.Error())
			protocol = "http"
			globalProtocol = "http"
			urlStr = tool.BuildScanOnceRegisterUrl(protocol, targetIP, multcastPort)
			req, err = tool.NewHTTPReqWithApplication(http.NewRequest("POST", urlStr, bytes.NewReader(payloadBytes)))
			if err != nil {
				return false
			}
			resp, err = httpClient.Do(req)
			if err != nil {
				return false
			}
		} else {
			return false
		}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			tool.DefaultLogger.Errorf("Failed to close response body: %v", err)
		}
	}()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}
	var remote types.CallbackLegacyVersionMessageHTTP
	if err := sonic.Unmarshal(body, &remote); err != nil {
		return false
	}
	tool.DefaultLogger.Infof("scanOneIPHTTP: discovered device at %s: %s (fingerprint: %s)", urlStr, remote.Alias, remote.Fingerprint)
	if remote.Fingerprint != "" {
		share.SetUserScanCurrent(remote.Fingerprint, types.UserScanCurrentItem{
			Ipaddress: targetIP,
			VersionMessage: types.VersionMessage{
				Alias:       remote.Alias,
				Version:     remote.Version,
				DeviceModel: remote.DeviceModel,
				DeviceType:  remote.DeviceType,
				Fingerprint: remote.Fingerprint,
				Port:        multcastPort,
				Protocol:    globalProtocol,
				Download:    remote.Download,
				Announce:    true,
			},
		})
		return true
	}
	return false
}

// sendRegisterRequest sends a register request to the remote device.
func sendRegisterRequest(url string, payload string) error {
	req, err := tool.NewHTTPReqWithApplication(http.NewRequest("POST", url, bytes.NewReader(tool.StringToBytes(payload))))
	if err != nil {
		return fmt.Errorf("failed to create register request: %v", err)
	}
	tool.DefaultLogger.Debugf("Sent: %s, using Payload: %s", url, payload)

	resp, err := tool.GetHttpClient().Do(req)
	if err != nil {
		return fmt.Errorf("failed to send register request: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			tool.DefaultLogger.Errorf("Failed to close response body: %v", err)
		}
	}()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("register request failed: %s", resp.Status)
	}
	return nil
}

// ListenMulticastUsingHTTPWithTimeout is the same as ListenMulticastUsingHTTP but with configurable timeout.
// timeout: total duration in seconds after which scanning stops. 0 means no timeout.
// skipInitialScan: if true (e.g. after scan-now), do not run initial scan; first scan in 30s.
func ListenMulticastUsingHTTPWithTimeout(self *types.VersionMessageHTTP, timeout int, skipInitialScan bool) {
	if self == nil {
		tool.DefaultLogger.Warn("ListenMulticastUsingHTTP: self is nil")
		return
	}

	autoScanControlMu.Lock()
	autoScanHTTPRunning = true
	if autoScanRestartCh == nil {
		autoScanRestartCh = make(chan restartAction, 1)
	}
	restartCh := autoScanRestartCh
	autoScanControlMu.Unlock()

	defer func() {
		autoScanControlMu.Lock()
		autoScanHTTPRunning = false
		autoScanControlMu.Unlock()
	}()

	if timeout > 0 {
		tool.DefaultLogger.Infof("Starting Legacy Mode HTTP scanning (scanning every 30 seconds, timeout: %d seconds)", timeout)
	} else {
		tool.DefaultLogger.Info("Starting Legacy Mode HTTP scanning (scanning every 30 seconds, no timeout)")
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	var timeoutTimer *time.Timer
	var timeoutCh <-chan time.Time
	resetTimeout := func() {
		if timeout > 0 {
			if timeoutTimer != nil {
				timeoutTimer.Stop()
			}
			timeoutTimer = time.NewTimer(time.Duration(timeout) * time.Second)
			timeoutCh = timeoutTimer.C
			tool.DefaultLogger.Infof("HTTP scan timeout reset to %d seconds", timeout)
		}
	}
	if timeout > 0 {
		timeoutTimer = time.NewTimer(time.Duration(timeout) * time.Second)
		timeoutCh = timeoutTimer.C
	}
	defer func() {
		if timeoutTimer != nil {
			timeoutTimer.Stop()
		}
	}()

	startTime := time.Now()

	scanOnce := func() {
		opts := &HTTPScanOptions{Concurrency: autoScanConcurrencyLimit, RateLimitPPS: autoScanICMPRatePPS}
		if err := ScanOnceHTTP(self, opts); err != nil {
			tool.DefaultLogger.Warnf("ListenMulticastUsingHTTP: scan failed: %v", err)
		}
	}

	if !skipInitialScan {
		scanOnce()
	} else {
		tool.DefaultLogger.Debug("HTTP scan: skipping initial scan, first scan in 30s")
	}

	for {
		select {
		case <-timeoutCh:
			elapsed := time.Since(startTime)
			tool.DefaultLogger.Infof("HTTP scanning stopped after timeout (%v elapsed)", elapsed.Round(time.Second))
			return
		case action := <-restartCh:
			resetTimeout()
			startTime = time.Now()
			if !action.SkipHTTPImmediateScan {
				scanOnce()
			} else {
				tool.DefaultLogger.Debug("HTTP scan: restart without immediate scan, next scan in 30s")
			}
		case <-ticker.C:
			scanOnce()
		}
	}
}

// ScanOnceHTTP performs a single HTTP scan for devices.
// opts: nil or RateLimitPPS=0 and Concurrency=0 means unlimited (scan-now style).
func ScanOnceHTTP(self *types.VersionMessageHTTP, opts *HTTPScanOptions) error {
	if self == nil {
		return fmt.Errorf("self message is nil")
	}
	if opts == nil {
		opts = &HTTPScanOptions{Concurrency: scanNowHTTPConcurrency, RateLimitPPS: 0}
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = scanNowHTTPConcurrency
	}
	payloadBytes, err := sonic.Marshal(self)
	if err != nil {
		return fmt.Errorf("failed to marshal self message: %v", err)
	}
	targets, err := getCachedNetworkIPs()
	if err != nil {
		return fmt.Errorf("failed to get network IPs: %v", err)
	}
	if len(targets) == 0 {
		return fmt.Errorf("no usable local IPv4 addresses found")
	}
	tool.DefaultLogger.Debugf("ScanOnceHTTP: scanning %d IP addresses (concurrency=%d, ratePPS=%d)", len(targets), concurrency, opts.RateLimitPPS)

	selfIPs := tool.GetLocalIPv4Set()
	filtered := targets[:0]
	for _, ip := range targets {
		if _, isSelf := selfIPs[ip]; isSelf {
			continue
		}
		filtered = append(filtered, ip)
	}
	targets = filtered

	var limiter *rate.Limiter
	if opts.RateLimitPPS > 0 {
		burst := opts.RateLimitPPS + 10
		if burst < 20 {
			burst = 20
		}
		limiter = rate.NewLimiter(rate.Limit(opts.RateLimitPPS), burst)
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	ctx := context.Background()
	for _, ip := range targets {
		wg.Add(1)
		go func(targetIP string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if limiter != nil {
				if err := limiter.Wait(ctx); err != nil {
					return
				}
			}
			scanOneIPHTTP(targetIP, payloadBytes, tool.GetScanHttpClient())
		}(ip)
	}
	wg.Wait()
	return nil
}
