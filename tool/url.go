package tool

import (
	"fmt"
	"net"
	"net/url"

	"github.com/moyoez/localsend-go/types"
)

// BuildRegisterURL builds the /register callback URL
func BuildRegisterURL(targetAddr *net.UDPAddr, remote *types.VersionMessage) (string, error) {
	return fmt.Sprintf("%s://%s:%d/api/localsend/v2/register", remote.Protocol, targetAddr.IP.String(), remote.Port), nil
}

func BuildScanOnceRegisterUrl(protocol string, targetIp string, port int) string {
	return fmt.Sprintf("%s://%s:%d/api/localsend/v2/register", protocol, targetIp, port)
}

// BuildPrepareUploadURL builds the /prepare-upload URL.
// If pin is not empty, add query parameter ?pin=xxx.
func BuildPrepareUploadURL(targetAddr *net.UDPAddr, remote *types.VersionMessage, pin string) (string, error) {
	url := fmt.Sprintf("%s://%s:%d/api/localsend/v2/prepare-upload", remote.Protocol, targetAddr.IP.String(), remote.Port)
	if pin != "" {
		url += fmt.Sprintf("?pin=%s", pin)
	}
	return url, nil
}

// BuildUploadURL builds the /upload URL with sessionId, fileId, and token query parameters.
func BuildUploadURL(targetAddr *net.UDPAddr, remote *types.VersionMessage, sessionId, fileId, token string) (string, error) {
	baseURL := fmt.Sprintf("%s://%s:%d/api/localsend/v2/upload", remote.Protocol, targetAddr.IP.String(), remote.Port)
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse base URL: %v", err)
	}
	// add query parameters
	u.RawQuery = fmt.Sprintf("sessionId=%s&fileId=%s&token=%s", sessionId, fileId, token)
	return u.String(), nil
}

// BuildCancelURL builds the /cancel URL with sessionId query parameter.
func BuildCancelURL(targetAddr *net.UDPAddr, remote *types.VersionMessage, sessionId string) (string, error) {
	baseURL := fmt.Sprintf("%s://%s:%d/api/localsend/v2/cancel", remote.Protocol, targetAddr.IP.String(), remote.Port)
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse base URL: %v", err)
	}
	// add query parameters
	u.RawQuery = fmt.Sprintf("sessionId=%s", sessionId)
	return u.String(), nil
}

// BuildInfoURL builds the /info URL to get device information.
func BuildInfoURL(protocol string, ip string, port int) string {
	return fmt.Sprintf("%s://%s:%d/api/localsend/v2/info", protocol, ip, port)
}
