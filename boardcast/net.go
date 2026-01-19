package boardcast

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/bytedance/sonic"
	"github.com/charmbracelet/log"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// refer to https://github.com/localsend/protocol/blob/main/README.md#1-defaults
const (
	defaultMultcastAddress = "224.0.0.167"
	defaultMultcastPort    = 53317 // UDP & HTTP
)

// ListenMulticastUsingUDP listens for multicast UDP broadcasts to discover other devices.
// Only respond to callbacks if the remote device announce=true and is not the same device.
// * With Register Callback
// * With Prepare-upload Callback
func ListenMulticastUsingUDP(self *types.VersionMessage) {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", defaultMultcastAddress, defaultMultcastPort))
	if err != nil {
		log.Fatalf("Failed to resolve UDP address: %v", err)
	}
	c, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		log.Fatalf("Failed to listen on multicast UDP address: %v", err)
	}
	defer c.Close()
	c.SetReadBuffer(256 * 1024)
	buf := make([]byte, 1024*64)
	log.Infof("Listening on multicast UDP address: %s", addr.String())
	for {
		n, addr, err := c.ReadFrom(buf)
		if err == nil {
			var incoming types.VersionMessage
			parseErr := sonic.Unmarshal(buf[:n], &incoming)
			if parseErr != nil {
				log.Errorf("Failed to parse UDP message: %v\n", parseErr)
				continue
			}
			// Ignore non-announce or from self broadcasts.
			if !shouldRespond(self, &incoming) {
				continue
			}
			log.Debugf("Received %d bytes from %s\n", n, addr.String())
			log.Debugf("Data: %s\n", string(buf[:n]))
			udpAddr, castErr := castToUDPAddr(addr)
			if castErr != nil {
				log.Errorf("Unexpected UDP address: %v\n", castErr)
				continue
			}
			go func(remote types.VersionMessage, remoteAddr *net.UDPAddr) {
				// Call the /register callback using HTTP/TCP to send the device information to the remote device.
				if callbackErr := CallbackMulticastMessageUsingTCP(remoteAddr, self, &remote); callbackErr != nil {
					log.Errorf("Failed to callback TCP register: %v\n", callbackErr)
				}
			}(incoming, udpAddr)
		} else {
			log.Errorf("Error reading from UDP: %v\n", err)
		}
	}
}

// SendMulticastUsingUDP sends a multicast message to the multicast address to announce the device.
// https://github.com/localsend/protocol/blob/main/README.md#31-multicast-udp-default
func SendMulticastUsingUDP(message *types.VersionMessage) error {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", defaultMultcastAddress, defaultMultcastPort))
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %v", err)
	}
	c, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return fmt.Errorf("failed to dial UDP address: %v", err)
	}
	defer c.Close()
	for {
		payload, err := sonic.Marshal(message)
		if err != nil {
			log.Errorf("failed to marshal message: %v", err)
			continue
		}
		_, err = c.Write(payload)
		if err != nil {
			log.Errorf("failed to write message: %v", err)
		}
		time.Sleep(5 * time.Second)
	}
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
		log.Warnf("Failed to send register request via HTTP: %v. Falling back to UDP multicast.", sendErr)
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
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", defaultMultcastAddress, defaultMultcastPort))
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
		return fmt.Errorf("failed to write message: %v", err)
	}
	log.Debugf("Sent UDP multicast message to %s", addr.String())
	return nil
}

// Legacy: HTTP-only fallback for devices that don't support UDP multicast.
// If multicast fails, send an HTTP POST to /api/localsend/v2/register on all local IPs to discover devices.
func ListenMulticastUsingHTTP(self *types.VersionMessage) {
	if self == nil {
		log.Warn("ListenMulticastUsingHTTP: self is nil")
		return
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Warnf("ListenMulticastUsingHTTP: failed to enumerate interface addresses: %v", err)
		return
	}

	var targets []string
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
			continue
		}
		targets = append(targets, ipnet.IP.String())
	}
	if len(targets) == 0 {
		log.Warn("ListenMulticastUsingHTTP: no usable local IPv4 addresses found")
		return
	}

	payloadBytes, err := sonic.Marshal(self)
	if err != nil {
		log.Warnf("ListenMulticastUsingHTTP: failed to marshal self message: %v", err)
		return
	}

	for _, ip := range targets {
		url := fmt.Sprintf("http://%s:%d/api/localsend/v2/register", ip, defaultMultcastPort)
		req, err := http.NewRequest("POST", url, bytes.NewReader(payloadBytes))
		if err != nil {
			log.Warnf("ListenMulticastUsingHTTP: failed to create request for %s: %v", url, err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Debugf("ListenMulticastUsingHTTP: POST to %s failed: %v", url, err)
			continue
		}
		func() {
			defer resp.Body.Close()
			if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
				log.Debugf("ListenMulticastUsingHTTP: POST to %s failed with status: %s", url, resp.Status)
				return
			}

			// Parse response body
			var remote types.VersionMessage
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Debugf("ListenMulticastUsingHTTP: failed reading response from %s: %v", url, err)
				return
			}
			if err := sonic.Unmarshal(body, &remote); err != nil {
				log.Debugf("ListenMulticastUsingHTTP: failed to unmarshal response from %s: %v", url, err)
				return
			}
			log.Infof("ListenMulticastUsingHTTP: received device response from %s: %+v", url, remote)
			// Optionally: act on `remote`
		}()
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
