package tool

import (
	"fmt"
	"net"
	"net/url"

	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// BuildRegisterURL builds the /register callback URL.
func BuildRegisterURL(targetAddr *net.UDPAddr, remote *types.VersionMessage) ([]byte, error) {
	return StringToBytes(fmt.Sprintf("%s://%s:%d/api/localsend/v2/register", remote.Protocol, targetAddr.IP.String(), remote.Port)), nil
}

// BuildOnly BuildRegisterUrlWithParams
func BuildRegisterUrlWithParams(protocol, targetIP string, multcastPort int) string {
	return fmt.Sprintf("%s://%s:%d/api/localsend/v2/register", protocol, targetIP, multcastPort)
}

// BuildPrepareUploadURL builds the /prepare-upload URL.
// If pin is not empty, add query parameter ?pin=xxx.
func BuildPrepareUploadURL(targetAddr *net.UDPAddr, remote *types.VersionMessage, pin string) ([]byte, error) {
	url := fmt.Sprintf("%s://%s:%d/api/localsend/v2/prepare-upload", remote.Protocol, targetAddr.IP.String(), remote.Port)
	if pin != "" {
		url += fmt.Sprintf("?pin=%s", pin)
	}
	return StringToBytes(url), nil
}

// BuildUploadURL builds the /upload URL with sessionId, fileId, and token query parameters.
func BuildUploadURL(targetAddr *net.UDPAddr, remote *types.VersionMessage, sessionId, fileId, token string) ([]byte, error) {
	baseURL := fmt.Sprintf("%s://%s:%d/api/localsend/v2/upload", remote.Protocol, targetAddr.IP.String(), remote.Port)
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse base URL: %v", err)
	}
	// add query parameters
	u.RawQuery = fmt.Sprintf("sessionId=%s&fileId=%s&token=%s", sessionId, fileId, token)
	return StringToBytes(u.String()), nil
}

// BuildCancelURL builds the /cancel URL with sessionId query parameter.
func BuildCancelURL(targetAddr *net.UDPAddr, remote *types.VersionMessage, sessionId string) ([]byte, error) {
	baseURL := fmt.Sprintf("%s://%s:%d/api/localsend/v2/cancel", remote.Protocol, targetAddr.IP.String(), remote.Port)
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse base URL: %v", err)
	}
	// add query parameters
	u.RawQuery = fmt.Sprintf("sessionId=%s", sessionId)
	return StringToBytes(u.String()), nil
}
