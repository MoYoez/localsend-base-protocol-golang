package tool

import (
	"crypto/tls"
	"net/http"
	"time"
)

var (
	DefaultTimeout       = 30 * time.Second
	ConnectionHttpClient *http.Client
)

func init() {
	ConnectionHttpClient = NewHTTPClient()
}

// NewHTTPClient creates an HTTP client, skipping self-signed certificate verification in HTTPS mode.
func NewHTTPClient() *http.Client {
	client := &http.Client{Timeout: DefaultTimeout}
	client.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return client
}

func GetHttpClient() *http.Client {
	return ConnectionHttpClient
}
