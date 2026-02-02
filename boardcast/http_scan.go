package boardcast

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/moyoez/localsend-base-protocol-golang/share"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// scanOneIPHTTP performs TCP probe, POST register (https then http on EOF), parses response and stores device via share.SetUserScanCurrent.
// Used by ListenMulticastUsingHTTPWithTimeout and ScanOnceHTTP. Returns true if a device was discovered and stored.
func scanOneIPHTTP(targetIP string, payloadBytes []byte, httpClient *http.Client) bool {
	if !tool.QuickTCPProbe(targetIP, multcastPort, tcpProbeTimeout) {
		return false
	}
	protocol := "https"
	urlStr := tool.BuildRegisterUrlWithParams(protocol, targetIP, multcastPort)
	req, err := tool.NewHTTPReqWithApplication(http.NewRequest("POST", urlStr, bytes.NewReader(payloadBytes)))
	if err != nil {
		tool.DefaultLogger.Debugf("scanOneIPHTTP: failed to create request for %s: %v", urlStr, err)
		return false
	}
	resp, err := httpClient.Do(req)
	globalProtocol := "https"
	if err != nil {
		if strings.Contains(err.Error(), "EOF") {
			tool.DefaultLogger.Warnf("Detected error, trying to use http protocol: %v", err.Error())
			protocol = "http"
			globalProtocol = "http"
			urlStr = tool.BuildRegisterUrlWithParams(protocol, targetIP, multcastPort)
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
		share.SetUserScanCurrent(remote.Fingerprint, share.UserScanCurrentItem{
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
func ListenMulticastUsingHTTPWithTimeout(self *types.VersionMessageHTTP, timeout int) {
	if self == nil {
		tool.DefaultLogger.Warn("ListenMulticastUsingHTTP: self is nil")
		return
	}

	autoScanControlMu.Lock()
	autoScanHTTPRunning = true
	if autoScanRestartCh == nil {
		autoScanRestartCh = make(chan struct{}, 1)
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

	payloadBytes, err := sonic.Marshal(self)
	if err != nil {
		tool.DefaultLogger.Warnf("ListenMulticastUsingHTTP: failed to marshal self message: %v", err)
		return
	}

	httpClient := &http.Client{
		Timeout: httpScanTimeout,
		Transport: &http.Transport{
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
			MaxIdleConns:        50,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     10 * time.Second,
			DisableKeepAlives:   false,
		},
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
		targets, err := getCachedNetworkIPs()
		if err != nil {
			tool.DefaultLogger.Warnf("ListenMulticastUsingHTTP: failed to get network IPs: %v", err)
			return
		}
		if len(targets) == 0 {
			tool.DefaultLogger.Warn("ListenMulticastUsingHTTP: no usable local IPv4 addresses found")
			return
		}
		selfIPs := tool.GetLocalIPv4Set()
		filtered := targets[:0]
		for _, ip := range targets {
			if _, isSelf := selfIPs[ip]; isSelf {
				continue
			}
			filtered = append(filtered, ip)
		}
		targets = filtered

		sem := make(chan struct{}, httpScanConcurrencyLimit)
		var wg sync.WaitGroup
		for _, ip := range targets {
			wg.Add(1)
			go func(targetIP string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				scanOneIPHTTP(targetIP, payloadBytes, httpClient)
			}(ip)
		}
		wg.Wait()
	}

	scanOnce()

	for {
		select {
		case <-timeoutCh:
			elapsed := time.Since(startTime)
			tool.DefaultLogger.Infof("HTTP scanning stopped after timeout (%v elapsed)", elapsed.Round(time.Second))
			return
		case <-restartCh:
			resetTimeout()
			startTime = time.Now()
			scanOnce()
		case <-ticker.C:
			scanOnce()
		}
	}
}

// ScanOnceHTTP performs a single HTTP scan for devices.
func ScanOnceHTTP(self *types.VersionMessageHTTP) error {
	if self == nil {
		return fmt.Errorf("self message is nil")
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
	tool.DefaultLogger.Debugf("ScanOnceHTTP: scanning %d IP addresses", len(targets))

	selfIPs := tool.GetLocalIPv4Set()
	filtered := targets[:0]
	for _, ip := range targets {
		if _, isSelf := selfIPs[ip]; isSelf {
			continue
		}
		filtered = append(filtered, ip)
	}
	targets = filtered

	httpClient := &http.Client{
		Timeout: httpScanTimeout,
		Transport: &http.Transport{
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
			MaxIdleConns:        50,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     10 * time.Second,
			DisableKeepAlives:   false,
		},
	}

	sem := make(chan struct{}, httpScanConcurrencyLimit)
	var wg sync.WaitGroup
	for _, ip := range targets {
		wg.Add(1)
		go func(targetIP string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			scanOneIPHTTP(targetIP, payloadBytes, httpClient)
		}(ip)
	}
	wg.Wait()
	return nil
}
