package boardcast

import (
	"fmt"
	"net"
	"time"

	"github.com/bytedance/sonic"
	"github.com/moyoez/localsend-go/share"
	"github.com/moyoez/localsend-go/tool"
	"github.com/moyoez/localsend-go/types"
)

// listenOnInterface listens for multicast messages on a specific network interface. (UDP4)
func listenOnInterface(iface *net.Interface, addr *net.UDPAddr, self *types.VersionMessage) {
	interfaceName := iface.Name

	c, err := net.ListenMulticastUDP("udp4", iface, addr)
	if err != nil {
		tool.DefaultLogger.Errorf("Failed to listen on multicast UDP address for interface %s: %v", interfaceName, err)
		return
	}
	defer func() {
		if err := c.Close(); err != nil {
			tool.DefaultLogger.Errorf("Failed to close multicast UDP connection: %v", err)
		}
	}()
	err = c.SetReadBuffer(1024 * 8)
	if err != nil {
		tool.DefaultLogger.Errorf("Failed to set read buffer: %v", err)
	}
	buf := make([]byte, 1024*8)
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
			share.SetUserScanCurrent(incoming.Fingerprint, types.UserScanCurrentItem{
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
func SendMulticastUsingUDPWithTimeout(message *types.VersionMessage, timeout int) {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", multcastAddress, multcastPort))
	if err != nil {
		tool.DefaultLogger.Errorf("Failed to resolve UDP address: %v", err)
		return
	}

	// Register UDP scan as running and get restart channel
	autoScanControlMu.Lock()
	autoScanUDPRunning = true
	if autoScanRestartCh == nil {
		autoScanRestartCh = make(chan restartAction, 1)
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
		tool.DefaultLogger.Errorf("Failed to dial UDP address: %v", err)
		return
	}
	defer func() {
		if c != nil {
			if err := c.Close(); err != nil {
				tool.DefaultLogger.Errorf("Failed to close multicast UDP connection: %v", err)
			}
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
			if tool.ShouldRedialUDP(err) {
				tool.DefaultLogger.Warnf("IP/network unavailable (e.g. after network change), will redial on next send: %v", err)
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
			return
		case <-restartCh:
			// Restart signal received, reset timeout and continue sending
			resetTimeout()
			startTime = time.Now()
			sendOnce() // UDP always sends immediately on restart
		case <-ticker.C:
			sendOnce()
		}
	}
}

// SendMulticastOnce sends a single multicast message to the multicast address.
func SendMulticastOnce(message *types.VersionMessage) error {
	if message == nil {
		tool.DefaultLogger.Errorf("Missing message")
		return nil
	}
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", multcastAddress, multcastPort))
	if err != nil {
		tool.DefaultLogger.Errorf("Failed to resolve UDP address: %v", err)
		return nil
	}
	c, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		if tool.IsAddrNotAvailableError(err) {
			return fmt.Errorf("IP address not available, please check your network environment and try again: %w", err)
		}
		return fmt.Errorf("failed to dial UDP address: %v", err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			tool.DefaultLogger.Errorf("Failed to close multicast UDP connection: %v", err)
		}
	}()
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
	defer func() {
		if err := c.Close(); err != nil {
			tool.DefaultLogger.Errorf("Failed to close multicast UDP connection: %v", err)
		}
	}()
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
