package tool

import (
	"crypto/tls"
	"net/http"
	"time"
)

// NewHTTPClient creates an HTTP client, skipping self-signed certificate verification in HTTPS mode.
func NewHTTPClient(protocol string) *http.Client {
	client := &http.Client{Timeout: 5 * time.Second}
	if protocol == "https" {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	return client
}
