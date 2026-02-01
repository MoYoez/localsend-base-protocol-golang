package boardcast

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/moyoez/localsend-base-protocol-golang/share"
	"github.com/moyoez/localsend-base-protocol-golang/tool"

	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// ScanMode defines the scanning mode
type ScanMode int

const (
	ScanModeUDP   ScanMode = iota // UDP multicast only
	ScanModeHTTP                  // HTTP scanning only (legacy mode)
	ScanModeMixed                 // Both UDP and HTTP scanning
)

// ScanConfig holds the current scan configuration for scan-now API
type ScanConfig struct {
	Mode        ScanMode
	SelfMessage *types.VersionMessage
	SelfHTTP    *types.VersionMessageHTTP
	Timeout     int // timeout in seconds, 0 means no timeout
}

// refer to https://github.com/localsend/protocol/blob/main/README.md#1-defaults
const (
	defaultMultcastAddress = "224.0.0.167"
	defaultMultcastPort    = 53317 // UDP & HTTP
	// httpScanConcurrencyLimit limits the number of concurrent HTTP scan goroutines
	httpScanConcurrencyLimit = 25
	// tcpProbeTimeout is the timeout for TCP port probe
	tcpProbeTimeout = 500 * time.Millisecond
	// httpScanTimeout is the timeout for HTTP requests during scanning
	httpScanTimeout = 2 * time.Second
)

var (
	multcastAddress       = defaultMultcastAddress
	multcastPort          = defaultMultcastPort
	referNetworkInterface string // the specified network interface name
	listenAllInterfaces   bool   // whether to listen on all network interfaces

	// networkIPsCache caches generated network IPs to avoid repeated generation
	networkIPsCacheMu  sync.RWMutex
	networkIPsCache    []string
	networkIPsCacheKey string // stores interface addresses hash to detect changes

	// currentScanConfig holds the current scan configuration
	currentScanConfigMu sync.RWMutex
	currentScanConfig   *ScanConfig

	// autoScanControl controls the auto scan loops
	autoScanControlMu   sync.Mutex
	autoScanRestartCh   chan struct{} // channel to signal restart
	autoScanHTTPRunning bool
	autoScanUDPRunning  bool
)

// SetMultcastAddress overrides the default multicast address if non-empty.
func SetMultcastAddress(address string) {
	if address == "" {
		return
	}
	multcastAddress = address
}

// SetMultcastPort overrides the default multicast port if positive.
func SetMultcastPort(port int) {
	if port <= 0 {
		return
	}
	multcastPort = port
}

// SetReferNetworkInterface sets the network interface to use for multicast.
// If interfaceName is empty, it will use the system default interface.
// If interfaceName is "*", it will listen on all available interfaces.
func SetReferNetworkInterface(interfaceName string) {
	if interfaceName == "*" {
		listenAllInterfaces = true
		referNetworkInterface = ""
	} else {
		listenAllInterfaces = false
		referNetworkInterface = interfaceName
	}
}

// getNetworkInterfaces returns a list of network interfaces to listen on.
// If listenAllInterfaces is true, returns all valid interfaces.
// If referNetworkInterface is set, returns only that interface.
// Otherwise, returns nil (use system default).
func getNetworkInterfaces() ([]*net.Interface, error) {
	if listenAllInterfaces {
		// gain all network interfaces
		interfaces, err := net.Interfaces()
		if err != nil {
			return nil, fmt.Errorf("failed to get network interfaces: %v", err)
		}

		var validInterfaces []*net.Interface
		for i := range interfaces {
			iface := &interfaces[i]
			// remove tun connections.
			if tool.RejectUnsupportNetworkInterface(iface) {
				continue
			}
			validInterfaces = append(validInterfaces, iface)
		}

		if len(validInterfaces) == 0 {
			return nil, fmt.Errorf("no valid network interfaces found")
		}

		return validInterfaces, nil
	} else if referNetworkInterface != "" {
		// get the specified network interface
		iface, err := net.InterfaceByName(referNetworkInterface)
		if err != nil {
			return nil, fmt.Errorf("failed to get network interface %s: %v", referNetworkInterface, err)
		}
		return []*net.Interface{iface}, nil
	}

	// use the system default interface
	return []*net.Interface{nil}, nil
}

// listenOnInterface listens for multicast messages on a specific network interface.
func listenOnInterface(iface *net.Interface, addr *net.UDPAddr, self *types.VersionMessage) {
	interfaceName := iface.Name

	c, err := net.ListenMulticastUDP("udp4", iface, addr)
	if err != nil {
		tool.DefaultLogger.Errorf("Failed to listen on multicast UDP address for interface %s: %v", interfaceName, err)
		return
	}
	defer c.Close()
	c.SetReadBuffer(256 * 1024)
	buf := make([]byte, 1024*64)
	tool.DefaultLogger.Infof("Listening on multicast UDP address: %s (interface: %s)", addr.String(), interfaceName)

	for {
		n, addr, err := c.ReadFrom(buf)
		if err == nil {
			var incoming types.VersionMessage
			parseErr := sonic.Unmarshal(buf[:n], &incoming)
			if parseErr != nil {
				tool.DefaultLogger.Errorf("Failed to parse UDP message: %v\n", parseErr)
				continue
			}
			// Ignore non-announce or from self broadcasts.
			if !tool.ShouldRespond(self, &incoming) {
				continue
			}
			tool.DefaultLogger.Debugf("Received %d bytes from %s on interface %s\n", n, addr.String(), interfaceName)
			tool.DefaultLogger.Debugf("Data: %s\n", string(buf[:n]))
			udpAddr, castErr := CastToUDPAddr(addr)
			if castErr != nil {
				tool.DefaultLogger.Errorf("Unexpected UDP address: %v\n", castErr)
				continue
			}
			share.SetUserScanCurrent(incoming.Fingerprint, share.UserScanCurrentItem{
				Ipaddress:      udpAddr.IP.String(),
				VersionMessage: incoming,
			})
			go func(remote types.VersionMessage, remoteAddr *net.UDPAddr) {
				// Call the /register callback using HTTP/TCP to send the device information to the remote device.
				// convert self to CallbackVersionMessageHTTP
				selfHTTP := &types.CallbackVersionMessageHTTP{
					Alias:       self.Alias,
					Version:     self.Version,
					DeviceModel: self.DeviceModel,
					DeviceType:  self.DeviceType,
					Fingerprint: self.Fingerprint,
					Port:        self.Port,
					Protocol:    self.Protocol,
					Download:    self.Download,
				}
				if callbackErr := CallbackMulticastMessageUsingTCP(remoteAddr, selfHTTP, &remote); callbackErr != nil {
					tool.DefaultLogger.Errorf("Failed to callback TCP register: %v\n", callbackErr)
				}
			}(incoming, udpAddr)
		} else {
			// error reading from udp, consider using http.
			tool.DefaultLogger.Errorf("Error reading from UDP on interface %s: %v\n", interfaceName, err)
		}
	}
}

// ListenMulticastUsingUDP listens for multicast UDP broadcasts to discover other devices.
// Only respond to callbacks if the remote device announce=true and is not the same device.
// * With Register Callback
// * With Prepare-upload Callback
func ListenMulticastUsingUDP(self *types.VersionMessage) {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", multcastAddress, multcastPort))
	if err != nil {
		tool.DefaultLogger.Fatalf("Failed to resolve UDP address: %v", err)
	}

	interfaces, err := getNetworkInterfaces()
	if err != nil {
		tool.DefaultLogger.Fatalf("Failed to get network interfaces: %v", err)
	}

	if len(interfaces) == 1 {
		// single interface, just run
		listenOnInterface(interfaces[0], addr, self)
	} else {
		// multiple interfaces, start a goroutine for each interface
		tool.DefaultLogger.Infof("Listening on %d network interfaces", len(interfaces))
		for _, iface := range interfaces {
			go listenOnInterface(iface, addr, self)
		}
		// block the main goroutine
		select {}
	}
}

// timeout: total duration in seconds after which sending stops. 0 means no timeout.
// Supports restart via RestartAutoScan() which resets the timeout timer.
// SendMulticastUsingUDP sends a multicast message to the multicast address to announce the device.
// https://github.com/localsend/protocol/blob/main/README.md#31-multicast-udp-default
func SendMulticastUsingUDPWithTimeout(message *types.VersionMessage, timeout int) error {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", multcastAddress, multcastPort))
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %v", err)
	}

	// Register UDP scan as running and get restart channel
	autoScanControlMu.Lock()
	autoScanUDPRunning = true
	if autoScanRestartCh == nil {
		autoScanRestartCh = make(chan struct{}, 1)
	}
	restartCh := autoScanRestartCh
	autoScanControlMu.Unlock()

	defer func() {
		autoScanControlMu.Lock()
		autoScanUDPRunning = false
		autoScanControlMu.Unlock()
	}()

	if timeout > 0 {
		tool.DefaultLogger.Infof("Starting UDP multicast sending (every 30 seconds, timeout: %d seconds)", timeout)
	} else {
		tool.DefaultLogger.Info("Starting UDP multicast sending (every 30 seconds, no timeout)")
	}

	var c *net.UDPConn
	dialConn := func() error {
		conn, dialErr := net.DialUDP("udp4", nil, addr)
		if dialErr != nil {
			return dialErr
		}
		if c != nil {
			_ = c.Close()
		}
		c = conn
		return nil
	}
	if err := dialConn(); err != nil {
		return fmt.Errorf("failed to dial UDP address: %v", err)
	}
	defer func() {
		if c != nil {
			c.Close()
		}
	}()

	// Setup timeout timer if timeout > 0
	var timeoutTimer *time.Timer
	var timeoutCh <-chan time.Time
	resetTimeout := func() {
		if timeout > 0 {
			if timeoutTimer != nil {
				timeoutTimer.Stop()
			}
			timeoutTimer = time.NewTimer(time.Duration(timeout) * time.Second)
			timeoutCh = timeoutTimer.C
			tool.DefaultLogger.Infof("UDP scan timeout reset to %d seconds", timeout)
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
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Send immediately first
	sendOnce := func() {
		if c == nil {
			if err := dialConn(); err != nil {
				tool.DefaultLogger.Errorf("failed to dial UDP address: %v", err)
				return
			}
		}
		payload, err := sonic.Marshal(message)
		if err != nil {
			tool.DefaultLogger.Errorf("failed to marshal message: %v", err)
			return
		}
		_, err = c.Write(payload)
		if err != nil {
			if tool.IsAddrNotAvailableError(err) {
				tool.DefaultLogger.Warnf("IP address not available, please check your network environment and try again: %v", err)
				_ = c.Close()
				c = nil
			} else {
				tool.DefaultLogger.Errorf("failed to write message: %v", err)
			}
		}
	}

	// Initial send
	sendOnce()

	// Continue sending until timeout
	for {
		select {
		case <-timeoutCh:
			elapsed := time.Since(startTime)
			tool.DefaultLogger.Infof("UDP multicast sending stopped after timeout (%v elapsed)", elapsed.Round(time.Second))
			return nil
		case <-restartCh:
			// Restart signal received, reset timeout and continue sending
			resetTimeout()
			startTime = time.Now()
			sendOnce()
		case <-ticker.C:
			sendOnce()
		}
	}
}

// SendMulticastOnce sends a single multicast message to the multicast address.
func SendMulticastOnce(message *types.VersionMessage) error {
	if message == nil {
		return fmt.Errorf("missing message")
	}
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", multcastAddress, multcastPort))
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %v", err)
	}
	c, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		if tool.IsAddrNotAvailableError(err) {
			return fmt.Errorf("IP address not available, please check your network environment and try again: %w", err)
		}
		return fmt.Errorf("failed to dial UDP address: %v", err)
	}
	defer c.Close()
	payload, err := sonic.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}
	if _, err := c.Write(payload); err != nil {
		if tool.IsAddrNotAvailableError(err) {
			return fmt.Errorf("IP address not available, please check your network environment and try again: %w", err)
		}
		return fmt.Errorf("failed to write message: %v", err)
	}
	return nil
}

// CallbackMulticastMessageUsingTCP calls the /register callback using HTTP/TCP.
func CallbackMulticastMessageUsingTCP(targetAddr *net.UDPAddr, self *types.CallbackVersionMessageHTTP, remote *types.VersionMessage) error {
	if err := validateCallbackParams(targetAddr, self, remote); err != nil {
		return err
	}
	// Only respond to callbacks if announce=true.
	if !remote.Announce {
		return nil
	}

	// Call the /register callback to send the device information to the remote device.
	url, buildErr := tool.BuildRegisterURL(targetAddr, remote)
	if buildErr != nil {
		return buildErr
	}
	payload, err := sonic.Marshal(self)
	if err != nil {
		return err
	}
	// Try sending register request via HTTP
	if sendErr := sendRegisterRequest(tool.BytesToString(url), tool.BytesToString(payload)); sendErr != nil {
		// debug what msg sent
		tool.DefaultLogger.Warnf("Failed to send register request via HTTP: %v. Falling back to UDP multicast.", sendErr)
		// Fallback: Respond using UDP multicast (announce=false)
		response := *self
		//	https://github.com/localsend/protocol/blob/main/README.md#31-multicast-udp-default
		if udpErr := CallbackMulticastMessageUsingUDP(&types.VersionMessage{
			Alias:       response.Alias,
			Version:     response.Version,
			DeviceModel: response.DeviceModel,
			DeviceType:  response.DeviceType,
			Fingerprint: response.Fingerprint,
			Port:        response.Port,
			Protocol:    response.Protocol,
			Announce:    false,
		}); udpErr != nil {
			return fmt.Errorf("both HTTP and UDP multicast fallback failed: %v; original: %v", udpErr, sendErr)
		}
	}
	return nil
}

// CallbackMulticastMessageUsingUDP sends a multicast message to the multicast address to announce the device.
func CallbackMulticastMessageUsingUDP(message *types.VersionMessage) error {
	if message == nil {
		return fmt.Errorf("missing response message")
	}
	response := *message
	// The UDP response needs to explicitly mark announce=false to avoid triggering a callback from the remote device.
	response.Announce = false
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", multcastAddress, multcastPort))
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %v", err)
	}
	c, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return fmt.Errorf("failed to dial UDP address: %v", err)
	}
	defer c.Close()
	payload, err := sonic.Marshal(&response)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}
	_, err = c.Write(payload)
	if err != nil {
		if tool.IsAddrNotAvailableError(err) {
			return fmt.Errorf("IP address not available, please check your network environment and try again: %w", err)
		}
		return fmt.Errorf("failed to write message: %v", err)
	}
	tool.DefaultLogger.Debugf("Sent UDP multicast message to %s", addr.String())
	return nil
}

// getCachedNetworkIPs returns cached network IPs or generates new ones if cache is invalid.
// It detects network interface changes by comparing the current interface addresses hash.
func getCachedNetworkIPs() ([]string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	// Build a cache key based on current interface addresses
	var keyBuilder strings.Builder
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
			continue
		}
		keyBuilder.WriteString(ipnet.String())
		keyBuilder.WriteString(";")
	}
	currentKey := keyBuilder.String()

	// Check if cache is valid
	networkIPsCacheMu.RLock()
	if networkIPsCacheKey == currentKey && len(networkIPsCache) > 0 {
		// Cache hit: return a copy to avoid race conditions
		result := make([]string, len(networkIPsCache))
		copy(result, networkIPsCache)
		networkIPsCacheMu.RUnlock()
		return result, nil
	}
	networkIPsCacheMu.RUnlock()

	// Cache miss: generate new IPs
	var targets []string
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
			continue
		}
		networkIPs := tool.GenerateNetworkIPs(ipnet)
		targets = append(targets, networkIPs...)
	}

	// Update cache
	networkIPsCacheMu.Lock()
	networkIPsCache = targets
	networkIPsCacheKey = currentKey
	networkIPsCacheMu.Unlock()

	// Return a copy
	result := make([]string, len(targets))
	copy(result, targets)
	return result, nil
}

// ListenMulticastUsingHTTPWithTimeout is the same as ListenMulticastUsingHTTP but with configurable timeout.
// timeout: total duration in seconds after which scanning stops. 0 means no timeout.
func ListenMulticastUsingHTTPWithTimeout(self *types.VersionMessageHTTP, timeout int) {
	if self == nil {
		tool.DefaultLogger.Warn("ListenMulticastUsingHTTP: self is nil")
		return
	}

	// Register HTTP scan as running and get restart channel
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

	// Scan all targets concurrently
	// set one client, do not use twice :(
	// solve tls: failed to verify certificate: x509: “LocalSend User” certificate is not standards compliant
	// Create a reusable HTTP client with optimized settings
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

	// Scan loop: every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Setup timeout timer if timeout > 0
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

	// Perform initial scan immediately
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

		//	tool.DefaultLogger.Debugf("ListenMulticastUsingHTTP: scanning %d IP addresses", len(targets))

		// remove self ip here.

		selfIPs := tool.GetLocalIPv4Set()

		filtered := targets[:0]
		for _, ip := range targets {
			if _, isSelf := selfIPs[ip]; isSelf {
				continue
			}
			filtered = append(filtered, ip)
		}
		targets = filtered

		// Use semaphore to limit concurrency
		sem := make(chan struct{}, httpScanConcurrencyLimit)
		var wg sync.WaitGroup

		for _, ip := range targets {
			wg.Add(1)
			go func(targetIP string) {
				defer wg.Done()

				// Acquire semaphore
				sem <- struct{}{}
				defer func() { <-sem }()

				// Quick TCP probe first - skip if port is not open
				if !tool.QuickTCPProbe(targetIP, multcastPort, tcpProbeTimeout) {
					return
				}

				// Port is open, proceed with HTTP request
				protocol := "https"
				url := fmt.Sprintf("%s://%s:%d/api/localsend/v2/register", protocol, targetIP, multcastPort)
				req, err := http.NewRequest("POST", url, bytes.NewReader(payloadBytes))
				if err != nil {
					tool.DefaultLogger.Debugf("ListenMulticastUsingHTTP: failed to create request for %s: %v", url, err)
					return
				}
				req.Header.Set("Content-Type", "application/json")

				resp, err := httpClient.Do(req)
				globalProtocol := "https"
				if err != nil {
					//	tool.DefaultLogger.Debugf("ListenMulticastUsingHTTP: failed to send request to %s: %v", url, err)
					//  check if is tls error, if is, try to use http protocol.
					if strings.Contains(err.Error(), "EOF") {
						tool.DefaultLogger.Warnf("Detected error, trying to use http protocol: %v", err.Error())
						protocol = "http"
						globalProtocol = "http"
						url = fmt.Sprintf("%s://%s:%d/api/localsend/v2/register", protocol, targetIP, multcastPort)
						req, err = http.NewRequest("POST", url, bytes.NewReader(payloadBytes))
						if err != nil {
							tool.DefaultLogger.Debugf("ListenMulticastUsingHTTP: failed to create request for %s: %v", url, err)
							return
						}
						req.Header.Set("Content-Type", "application/json")

						resp, err = httpClient.Do(req)
						if err != nil {
							tool.DefaultLogger.Debugf("ListenMulticastUsingHTTP: failed to send request to %s: %v", url, err)
							return
						}
					} else {
						// tool.DefaultLogger.Debugf("ListenMulticastUsingHTTP: failed to send request to %s: %v", url, err)
						return
					}
				}
				defer resp.Body.Close()
				if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
					tool.DefaultLogger.Debugf("ListenMulticastUsingHTTP: POST to %s failed with status: %s", url, resp.Status)
					return
				}

				// Parse response body
				var remote types.CallbackLegacyVersionMessageHTTP
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					tool.DefaultLogger.Debugf("ListenMulticastUsingHTTP: failed reading response from %s: %v", url, err)
					return
				}
				if err := sonic.Unmarshal(body, &remote); err != nil {
					tool.DefaultLogger.Debugf("ListenMulticastUsingHTTP: failed to unmarshal response from %s: %v", url, err)
					return
				}
				tool.DefaultLogger.Infof("ListenMulticastUsingHTTP: discovered device at %s: %s (fingerprint: %s)", url, remote.Alias, remote.Fingerprint)
				// Store the discovered device
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
				}
			}(ip)
		}

		// Wait for all scans to complete
		wg.Wait()
	}

	// Initial scan
	scanOnce()

	// Continue scanning every 30 seconds until timeout
	for {
		select {
		case <-timeoutCh:
			elapsed := time.Since(startTime)
			tool.DefaultLogger.Infof("HTTP scanning stopped after timeout (%v elapsed)", elapsed.Round(time.Second))
			return
		case <-restartCh:
			// Restart signal received, reset timeout and continue scanning
			resetTimeout()
			startTime = time.Now()
			scanOnce()
		case <-ticker.C:
			scanOnce()
		}
	}
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
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("register request failed: %s", resp.Status)
	}
	return nil
}

// validateCallbackParams validates the callback parameters (internal use).
func validateCallbackParams(targetAddr *net.UDPAddr, self *types.CallbackVersionMessageHTTP, remote *types.VersionMessage) error {
	if targetAddr == nil || self == nil || remote == nil {
		return fmt.Errorf("invalid callback params")
	}
	return nil
}

// CastToUDPAddr casts the address to a UDP address.
// Made public for reuse in other packages.
func CastToUDPAddr(addr net.Addr) (*net.UDPAddr, error) {
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return nil, fmt.Errorf("unexpected address type: %T", addr)
	}
	return udpAddr, nil
}

// ParseVersionMessageFromBody parses a VersionMessage from HTTP request body.
// Made public for reuse in API server.
func ParseVersionMessageFromBody(body []byte) (*types.VersionMessage, error) {
	var incoming types.VersionMessage
	if err := sonic.Unmarshal(body, &incoming); err != nil {
		return nil, fmt.Errorf("failed to parse version message: %v", err)
	}
	return &incoming, nil
}

// ParsePrepareUploadRequestFromBody parses a PrepareUploadRequest from HTTP request body.
// Made public for reuse in API server.
func ParsePrepareUploadRequestFromBody(body []byte) (*types.PrepareUploadRequest, error) {
	var request types.PrepareUploadRequest
	if err := sonic.Unmarshal(body, &request); err != nil {
		return nil, fmt.Errorf("failed to parse prepare-upload request: %v", err)
	}
	return &request, nil
}

// SetScanConfig sets the current scan configuration for scan-now API
func SetScanConfig(mode ScanMode, selfMessage *types.VersionMessage, selfHTTP *types.VersionMessageHTTP, timeout int) {
	currentScanConfigMu.Lock()
	defer currentScanConfigMu.Unlock()
	currentScanConfig = &ScanConfig{
		Mode:        mode,
		SelfMessage: selfMessage,
		SelfHTTP:    selfHTTP,
		Timeout:     timeout,
	}
}

// GetScanConfig returns the current scan configuration
func GetScanConfig() *ScanConfig {
	currentScanConfigMu.RLock()
	defer currentScanConfigMu.RUnlock()
	return currentScanConfig
}

// ScanOnceHTTP performs a single HTTP scan for devices.
// This is extracted from ListenMulticastUsingHTTP for reuse.
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

	// Remove self IP
	selfIPs := tool.GetLocalIPv4Set()
	filtered := targets[:0]
	for _, ip := range targets {
		if _, isSelf := selfIPs[ip]; isSelf {
			continue
		}
		filtered = append(filtered, ip)
	}
	targets = filtered

	// Create HTTP client
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

	// Use semaphore to limit concurrency
	sem := make(chan struct{}, httpScanConcurrencyLimit)
	var wg sync.WaitGroup

	for _, ip := range targets {
		wg.Add(1)
		go func(targetIP string) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Quick TCP probe first
			if !tool.QuickTCPProbe(targetIP, multcastPort, tcpProbeTimeout) {
				return
			}
			// Port is open, proceed with HTTP request
			protocol := "https"
			url := tool.BuildRegisterUrlWithParams(protocol, targetIP, multcastPort)
			req, err := tool.NewHTTPReqWithApplication(http.NewRequest("POST", url, bytes.NewReader(payloadBytes)))
			if err != nil {
				return
			}
			resp, err := httpClient.Do(req)
			globalProtocol := "https"
			if err != nil {
				if strings.Contains(err.Error(), "EOF") {
					protocol = "http"
					globalProtocol = "http"
					url = tool.BuildRegisterUrlWithParams(protocol, targetIP, multcastPort)
					req, err = tool.NewHTTPReqWithApplication(http.NewRequest("POST", url, bytes.NewReader(payloadBytes)))
					if err != nil {
						return
					}
					resp, err = httpClient.Do(req)
					if err != nil {
						return
					}
				} else {
					return
				}
			}
			defer resp.Body.Close()
			if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
				return
			}

			var remote types.CallbackLegacyVersionMessageHTTP
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return
			}
			if err := sonic.Unmarshal(body, &remote); err != nil {
				return
			}
			tool.DefaultLogger.Infof("ScanOnceHTTP: discovered device at %s: %s (fingerprint: %s)", url, remote.Alias, remote.Fingerprint)
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
			}
		}(ip)
	}

	wg.Wait()
	return nil
}

// ScanOnceUDP sends a single UDP multicast message to trigger device discovery.
func ScanOnceUDP(message *types.VersionMessage) error {
	return SendMulticastOnce(message)
}

// RestartAutoScan sends a restart signal to all running auto scan loops.
// This resets their timeout timers and triggers an immediate scan.
func RestartAutoScan() {
	autoScanControlMu.Lock()
	defer autoScanControlMu.Unlock()

	if autoScanRestartCh == nil {
		tool.DefaultLogger.Debug("No auto scan restart channel, creating one")
		autoScanRestartCh = make(chan struct{}, 1)
	}

	// Send restart signal (non-blocking)
	select {
	case autoScanRestartCh <- struct{}{}:
		tool.DefaultLogger.Info("Auto scan restart signal sent")
	default:
		tool.DefaultLogger.Debug("Auto scan restart channel full, signal already pending")
	}
}

// IsAutoScanRunning returns whether any auto scan loop is currently running.
func IsAutoScanRunning() bool {
	autoScanControlMu.Lock()
	defer autoScanControlMu.Unlock()
	return autoScanHTTPRunning || autoScanUDPRunning
}

// ScanNow performs a single scan based on current configuration.
// If auto scan has timed out (stopped), it restarts the auto scan loops.
// If auto scan is still running, it sends a restart signal to reset the timeout.
// Returns error if scan config is not set or scan fails.
func ScanNow() error {
	config := GetScanConfig()
	if config == nil {
		return fmt.Errorf("scan config not set")
	}

	tool.DefaultLogger.Info("Performing manual scan...")

	// Check if auto scan is still running
	if IsAutoScanRunning() {
		// Auto scan is running, send restart signal to reset timeout
		tool.DefaultLogger.Debug("Auto scan is running, sending restart signal")
		RestartAutoScan()
	} else {
		// Auto scan has stopped (timed out), restart the goroutines
		tool.DefaultLogger.Info("Auto scan has stopped, restarting auto scan loops")
		restartAutoScanLoops(config)
	}

	switch config.Mode {
	case ScanModeUDP:
		if config.SelfMessage == nil {
			return fmt.Errorf("self message not configured for UDP scan")
		}
		tool.DefaultLogger.Debug("Sending UDP multicast scan...")
		return ScanOnceUDP(config.SelfMessage)

	case ScanModeHTTP:
		if config.SelfHTTP == nil {
			return fmt.Errorf("self HTTP message not configured for HTTP scan")
		}
		tool.DefaultLogger.Debug("Performing HTTP scan...")
		return ScanOnceHTTP(config.SelfHTTP)

	case ScanModeMixed:
		var udpErr, httpErr error

		// UDP scan
		if config.SelfMessage != nil {
			tool.DefaultLogger.Debug("Sending UDP multicast scan (mixed mode)...")
			udpErr = ScanOnceUDP(config.SelfMessage)
			if udpErr != nil {
				tool.DefaultLogger.Warnf("UDP scan failed: %v", udpErr)
			}
		}

		// HTTP scan
		if config.SelfHTTP != nil {
			tool.DefaultLogger.Debug("Performing HTTP scan (mixed mode)...")
			httpErr = ScanOnceHTTP(config.SelfHTTP)
			if httpErr != nil {
				tool.DefaultLogger.Warnf("HTTP scan failed: %v", httpErr)
			}
		}

		// Return error only if both failed
		if udpErr != nil && httpErr != nil {
			return fmt.Errorf("both UDP and HTTP scan failed: UDP: %v, HTTP: %v", udpErr, httpErr)
		}
		return nil

	default:
		return fmt.Errorf("unknown scan mode: %d", config.Mode)
	}
}

// restartAutoScanLoops restarts the auto scan goroutines based on configuration.
// This is called when auto scan has timed out and needs to be restarted.
func restartAutoScanLoops(config *ScanConfig) {
	if config == nil {
		return
	}

	timeout := config.Timeout

	switch config.Mode {
	case ScanModeUDP:
		if config.SelfMessage != nil {
			go SendMulticastUsingUDPWithTimeout(config.SelfMessage, timeout)
		}
	case ScanModeHTTP:
		if config.SelfHTTP != nil {
			go ListenMulticastUsingHTTPWithTimeout(config.SelfHTTP, timeout)
		}
	case ScanModeMixed:
		if config.SelfMessage != nil {
			go SendMulticastUsingUDPWithTimeout(config.SelfMessage, timeout)
		}
		if config.SelfHTTP != nil {
			go ListenMulticastUsingHTTPWithTimeout(config.SelfHTTP, timeout)
		}
	}
}
