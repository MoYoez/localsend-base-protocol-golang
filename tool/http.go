package tool

import (
	"crypto/tls"
	"net/http"
	"time"
)

var (
	DefaultTimeout = 30 * time.Second
)

// NewHTTPClient creates an HTTP client, skipping self-signed certificate verification in HTTPS mode.
func NewHTTPClient(protocol string) *http.Client {
	client := &http.Client{Timeout: DefaultTimeout}
	if protocol == "https" {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	return client
}
