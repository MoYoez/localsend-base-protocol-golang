package boardcast

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/bytedance/sonic"
	"github.com/moyoez/localsend-base-protocol-golang/share"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// refer to https://github.com/localsend/protocol/blob/main/README.md#1-defaults
const (
	defaultMultcastAddress = "224.0.0.167"
	defaultMultcastPort    = 53317 // UDP & HTTP
)

var (
	multcastAddress = defaultMultcastAddress
	multcastPort    = defaultMultcastPort
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

// ListenMulticastUsingUDP listens for multicast UDP broadcasts to discover other devices.
// Only respond to callbacks if the remote device announce=true and is not the same device.
// * With Register Callback
// * With Prepare-upload Callback
func ListenMulticastUsingUDP(self *types.VersionMessage) {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", multcastAddress, multcastPort))
	if err != nil {
		tool.DefaultLogger.Fatalf("Failed to resolve UDP address: %v", err)
	}
	c, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		tool.DefaultLogger.Fatalf("Failed to listen on multicast UDP address: %v", err)
	}
	defer c.Close()
	c.SetReadBuffer(256 * 1024)
	buf := make([]byte, 1024*64)
	tool.DefaultLogger.Infof("Listening on multicast UDP address: %s", addr.String())
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
			if !shouldRespond(self, &incoming) {
				continue
			}
			tool.DefaultLogger.Debugf("Received %d bytes from %s\n", n, addr.String())
			tool.DefaultLogger.Debugf("Data: %s\n", string(buf[:n]))
			udpAddr, castErr := castToUDPAddr(addr)
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
				if callbackErr := CallbackMulticastMessageUsingTCP(remoteAddr, self, &remote); callbackErr != nil {
					tool.DefaultLogger.Errorf("Failed to callback TCP register: %v\n", callbackErr)
				}
			}(incoming, udpAddr)
		} else {
			// error reading from udp, consider using http.
			tool.DefaultLogger.Errorf("Error reading from UDP: %v\n", err)
		}
	}
}

// SendMulticastUsingUDP sends a multicast message to the multicast address to announce the device.
// https://github.com/localsend/protocol/blob/main/README.md#31-multicast-udp-default
func SendMulticastUsingUDP(message *types.VersionMessage) error {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", multcastAddress, multcastPort))
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %v", err)
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
	for {
		if c == nil {
			if err := dialConn(); err != nil {
				tool.DefaultLogger.Errorf("failed to dial UDP address: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}
		}
		payload, err := sonic.Marshal(message)
		if err != nil {
			tool.DefaultLogger.Errorf("failed to marshal message: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		_, err = c.Write(payload)
		if err != nil {
			if IsAddrNotAvailableError(err) {
				tool.DefaultLogger.Warnf("IP address not available, please check your network environment and try again: %v", err)
				_ = c.Close()
				c = nil
			} else {
				tool.DefaultLogger.Errorf("failed to write message: %v", err)
			}
		}
		time.Sleep(5 * time.Second)
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
		if IsAddrNotAvailableError(err) {
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
		if IsAddrNotAvailableError(err) {
			return fmt.Errorf("IP address not available, please check your network environment and try again: %w", err)
		}
		return fmt.Errorf("failed to write message: %v", err)
	}
	return nil
}

// CallbackMulticastMessageUsingTCP calls the /register callback using HTTP/TCP.
func CallbackMulticastMessageUsingTCP(targetAddr *net.UDPAddr, self *types.VersionMessage, remote *types.VersionMessage) error {
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
	if sendErr := sendRegisterRequest(tool.BytesToString(url), remote.Protocol, tool.BytesToString(payload)); sendErr != nil {
		tool.DefaultLogger.Warnf("Failed to send register request via HTTP: %v. Falling back to UDP multicast.", sendErr)
		// Fallback: Respond using UDP multicast (announce=false)
		response := *self
		response.Announce = false
		if udpErr := CallbackMulticastMessageUsingUDP(&response); udpErr != nil {
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
		if IsAddrNotAvailableError(err) {
			return fmt.Errorf("IP address not available, please check your network environment and try again: %w", err)
		}
		return fmt.Errorf("failed to write message: %v", err)
	}
	tool.DefaultLogger.Debugf("Sent UDP multicast message to %s", addr.String())
	return nil
}

// IsAddrNotAvailableError detects address-not-available errors across platforms.
func IsAddrNotAvailableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EADDRNOTAVAIL) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "can't assign requested address") ||
		strings.Contains(msg, "cannot assign requested address") ||
		strings.Contains(msg, "address not available")
}

// generateNetworkIPs generates all IP addresses in the given network.
// For /24 networks, it generates IPs from .1 to .254
// For larger networks, it limits to 254 IPs to avoid excessive scanning.
func generateNetworkIPs(ipnet *net.IPNet) []string {
	var ips []string
	ip := ipnet.IP.To4()
	if ip == nil {
		return ips
	}

	mask := ipnet.Mask
	network := ip.Mask(mask)

	// Calculate the number of host bits
	ones, bits := mask.Size()
	if bits != 32 {
		return ips
	}
	hostBits := 32 - ones

	// Limit to 254 hosts to avoid excessive scanning
	maxHosts := 254
	if hostBits < 8 {
		// For smaller networks, use actual host count
		maxHosts = (1 << hostBits) - 2 // -2 for network and broadcast
	}

	// Generate IPs by incrementing the host part
	for i := 1; i <= maxHosts; i++ {
		ip := make(net.IP, 4)
		copy(ip, network)

		// Calculate host part based on network size
		if hostBits <= 8 {
			// Simple case: host part is in last octet only
			ip[3] = network[3] + byte(i)
		} else if hostBits <= 16 {
			// Host part spans last two octets
			ip[3] = network[3] + byte(i&0xff)
			ip[2] = network[2] + byte((i>>8)&0xff)
		} else {
			// Host part spans last three octets
			ip[3] = network[3] + byte(i&0xff)
			ip[2] = network[2] + byte((i>>8)&0xff)
			ip[1] = network[1] + byte((i>>16)&0xff)
		}

		// Skip if it equals the network address
		if ip.Equal(network) {
			continue
		}

		ips = append(ips, ip.String())
	}
	return ips
}

// Legacy: HTTP-only fallback for devices that don't support UDP multicast.
// If multicast fails, send an HTTP POST to /api/localsend/v2/register on all local IPs to discover devices.
// This function runs in a loop, scanning every 30 seconds.
func ListenMulticastUsingHTTP(self *types.VersionMessage) {
	if self == nil {
		tool.DefaultLogger.Warn("ListenMulticastUsingHTTP: self is nil")
		return
	}

	tool.DefaultLogger.Info("Starting Legacy Mode HTTP scanning (scanning every 30 seconds)")

	payloadBytes, err := sonic.Marshal(self)
	if err != nil {
		tool.DefaultLogger.Warnf("ListenMulticastUsingHTTP: failed to marshal self message: %v", err)
		return
	}

	// Scan loop: every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Perform initial scan immediately
	scanOnce := func() {
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			tool.DefaultLogger.Warnf("ListenMulticastUsingHTTP: failed to enumerate interface addresses: %v", err)
			return
		}

		var targets []string
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
				continue
			}
			// Generate all IPs in the network
			networkIPs := generateNetworkIPs(ipnet)
			targets = append(targets, networkIPs...)
		}
		if len(targets) == 0 {
			tool.DefaultLogger.Warn("ListenMulticastUsingHTTP: no usable local IPv4 addresses found")
			return
		}

		tool.DefaultLogger.Debugf("ListenMulticastUsingHTTP: scanning %d IP addresses", len(targets))

		// Scan all targets concurrently
		for _, ip := range targets {
			go func(targetIP string) {
				url := fmt.Sprintf("http://%s:%d/api/localsend/v2/register", targetIP, multcastPort)
				req, err := http.NewRequest("POST", url, bytes.NewReader(payloadBytes))
				if err != nil {
					tool.DefaultLogger.Debugf("ListenMulticastUsingHTTP: failed to create request for %s: %v", url, err)
					return
				}
				req.Header.Set("Content-Type", "application/json")
				client := &http.Client{Timeout: tool.DefaultTimeout}
				resp, err := client.Do(req)
				if err != nil {
					// Silently ignore connection errors (most IPs won't have devices)
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
					tool.DefaultLogger.Debugf("ListenMulticastUsingHTTP: POST to %s failed with status: %s", url, resp.Status)
					return
				}

				// Parse response body
				var remote types.VersionMessage
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
						Ipaddress:      ip,
						VersionMessage: remote,
					})
				}
			}(ip)
		}
	}

	// Initial scan
	scanOnce()

	// Continue scanning every 30 seconds
	for range ticker.C {
		scanOnce()
	}
}

// sendRegisterRequest sends a register request to the remote device.
func sendRegisterRequest(url string, protocol string, payload string) error {
	req, err := http.NewRequest("POST", url, bytes.NewReader(tool.StringToBytes(payload)))
	if err != nil {
		return fmt.Errorf("failed to create register request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := tool.NewHTTPClient(protocol)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send register request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("register request failed: %s", resp.Status)
	}
	return nil
}

// ValidateCallbackParams validates the callback parameters.
// Made public for reuse in other packages.
func ValidateCallbackParams(targetAddr *net.UDPAddr, self *types.VersionMessage, remote *types.VersionMessage) error {
	if targetAddr == nil || self == nil || remote == nil {
		return fmt.Errorf("invalid callback params")
	}
	return nil
}

// validateCallbackParams validates the callback parameters (internal use).
func validateCallbackParams(targetAddr *net.UDPAddr, self *types.VersionMessage, remote *types.VersionMessage) error {
	return ValidateCallbackParams(targetAddr, self, remote)
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

// castToUDPAddr casts the address to a UDP address (internal use).
func castToUDPAddr(addr net.Addr) (*net.UDPAddr, error) {
	return CastToUDPAddr(addr)
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

// ShouldRespond determines if the device should respond to the incoming message.
// Made public for reuse in other packages.
func ShouldRespond(self *types.VersionMessage, incoming *types.VersionMessage) bool {
	if incoming == nil || !incoming.Announce {
		return false
	}
	if self != nil && self.Fingerprint != "" && incoming.Fingerprint == self.Fingerprint {
		return false
	}
	return true
}

// shouldRespond determines if the device should respond to the incoming message (internal use).
func shouldRespond(self *types.VersionMessage, incoming *types.VersionMessage) bool {
	return ShouldRespond(self, incoming)
}
