package tool

import (
	"fmt"
	"net"
	"net/url"

	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// BuildRegisterURL builds the /register callback URL.
func BuildRegisterURL(targetAddr *net.UDPAddr, remote *types.VersionMessage) ([]byte, error) {
	scheme := remote.Protocol
	if scheme == "" {
		scheme = "http"
	}
	port := remote.Port
	if port == 0 {
		port = 53317
	}
	return StringToBytes(fmt.Sprintf("%s://%s:%d/api/localsend/v2/register", scheme, targetAddr.IP.String(), port)), nil
}

// BuildPrepareUploadURL builds the /prepare-upload URL.
// If pin is not empty, add query parameter ?pin=xxx.
func BuildPrepareUploadURL(targetAddr *net.UDPAddr, remote *types.VersionMessage, pin string) ([]byte, error) {
	scheme := remote.Protocol
	if scheme == "" {
		scheme = "http"
	}
	port := remote.Port
	if port == 0 {
		port = 53317
	}
	url := fmt.Sprintf("%s://%s:%d/api/localsend/v2/prepare-upload", scheme, targetAddr.IP.String(), port)
	if pin != "" {
		url = fmt.Sprintf("%s?pin=%s", url, pin)
	}
	return StringToBytes(url), nil
}

// BuildUploadURL builds the /upload URL with sessionId, fileId, and token query parameters.
func BuildUploadURL(targetAddr *net.UDPAddr, remote *types.VersionMessage, sessionId, fileId, token string) ([]byte, error) {
	scheme := remote.Protocol
	if scheme == "" {
		scheme = "http"
	}
	port := remote.Port
	if port == 0 {
		port = 53317
	}
	baseURL := fmt.Sprintf("%s://%s:%d/api/localsend/v2/upload", scheme, targetAddr.IP.String(), port)
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse base URL: %v", err)
	}
	q := u.Query()
	q.Set("sessionId", sessionId)
	q.Set("fileId", fileId)
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return StringToBytes(u.String()), nil
}

// BuildCancelURL builds the /cancel URL with sessionId query parameter.
func BuildCancelURL(targetAddr *net.UDPAddr, remote *types.VersionMessage, sessionId string) ([]byte, error) {
	scheme := remote.Protocol
	if scheme == "" {
		scheme = "http"
	}
	port := remote.Port
	if port == 0 {
		port = 53317
	}
	baseURL := fmt.Sprintf("%s://%s:%d/api/localsend/v2/cancel", scheme, targetAddr.IP.String(), port)
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse base URL: %v", err)
	}
	q := u.Query()
	q.Set("sessionId", sessionId)
	u.RawQuery = q.Encode()
	return StringToBytes(u.String()), nil
}
