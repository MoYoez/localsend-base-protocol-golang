package boardcast

import (
	"fmt"
	"net"

	"github.com/bytedance/sonic"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

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

// validateCallbackParams validates the callback parameters (internal use).
func validateCallbackParams(targetAddr *net.UDPAddr, self *types.CallbackVersionMessageHTTP, remote *types.VersionMessage) error {
	if targetAddr == nil || self == nil || remote == nil {
		return fmt.Errorf("invalid callback params")
	}
	return nil
}
