package tool

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

var (
	DefaultTimeout       = 30 * time.Second
	// ScanTimeout is the overall request timeout for device scan (scan-now). Shorter than DefaultTimeout
	// so that non-responding IPs fail fast and scan-now returns in seconds instead of ~30s.
	ScanTimeout       = 5 * time.Second
	ScanDialTimeout   = 3 * time.Second // dial timeout for scan client
	ConnectionHttpClient *http.Client
	DetectHttpClient     *http.Client
	ScanDetectHttpClient *http.Client
)

func init() {
	ConnectionHttpClient = NewHTTPClient()
	DetectHttpClient = NewHTTPClient()
	ScanDetectHttpClient = newHTTPClientForScan(nil)
}

// NewHTTPClient creates an HTTP client, skipping self-signed certificate verification in HTTPS mode.
func NewHTTPClient() *http.Client {
	return newHTTPClientWithBindAddr(nil)
}

// newHTTPClientWithBindAddr creates an HTTP client. When bindAddr is non-nil, outgoing connections
// are bound to that local address (e.g. to force use of a specific network interface).
func newHTTPClientWithBindAddr(bindAddr *net.TCPAddr) *http.Client {
	transport := &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     300 * time.Millisecond,
		DisableKeepAlives:   false,
	}
	if bindAddr != nil {
		dialer := &net.Dialer{
			LocalAddr: bindAddr,
			Timeout:   DefaultTimeout,
			KeepAlive: 30 * time.Second,
		}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		}
	}
	return &http.Client{
		Timeout:   DefaultTimeout,
		Transport: transport,
	}
}

// newHTTPClientForScan creates an HTTP client for device scanning (scan-now) with short timeouts
// so that non-responding IPs fail fast; overall timeout ScanTimeout (e.g. 5s), dial timeout ScanDialTimeout (e.g. 3s).
func newHTTPClientForScan(bindAddr *net.TCPAddr) *http.Client {
	transport := &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     300 * time.Millisecond,
		DisableKeepAlives:   false,
	}
	dialTimeout := ScanDialTimeout
	if bindAddr != nil {
		dialer := &net.Dialer{
			LocalAddr: bindAddr,
			Timeout:   dialTimeout,
			KeepAlive: 30 * time.Second,
		}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		}
	} else {
		dialer := &net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}
		transport.DialContext = dialer.DialContext
	}
	return &http.Client{
		Timeout:   ScanTimeout,
		Transport: transport,
	}
}

// InitHTTPClients (re)initializes the HTTP clients with optional bind address.
// Call this after boardcast.SetReferNetworkInterface. When bindAddr is nil (e.g. useReferNetworkInterface is "*"),
// clients use the default transport without interface binding.
func InitHTTPClients(bindAddr *net.TCPAddr) {
	ConnectionHttpClient = newHTTPClientWithBindAddr(bindAddr)
	DetectHttpClient = newHTTPClientWithBindAddr(bindAddr)
	ScanDetectHttpClient = newHTTPClientForScan(bindAddr)
}

func GetHttpClient() *http.Client {
	return ConnectionHttpClient
}

// GetScanHttpClient returns the HTTP client used for device scanning (scan-now), with short timeouts.
func GetScanHttpClient() *http.Client {
	return ScanDetectHttpClient
}
