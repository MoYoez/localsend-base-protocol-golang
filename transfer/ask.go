package transfer

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/bytedance/sonic"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

const (
	StatusFinishedNoTransfer    = 204 // Finished (No file transfer needed)
	StatusInvalidBody           = 400 // Invalid body
	StatusPinRequiredOrInvalid  = 401 // PIN required / Invalid PIN
	StatusRejected              = 403 // Rejected
	StatusBlockedByOtherSession = 409 // Blocked by another session
	StatusTooManyRequests       = 429 // Too many requests
	StatusUnknownReceiverError  = 500 // Unknown error by receiver
)

// ReadyToUploadTo sends metadata to the receiver to prepare for upload.
// The receiver will decide whether to accept, partially accept, or reject the request.
// If a PIN is required, it should be provided in the pin parameter.
func ReadyToUploadTo(targetAddr *net.UDPAddr, remote *types.VersionMessage, request *types.PrepareUploadRequest, pin string) (*types.PrepareUploadResponse, error) {
	if targetAddr == nil || remote == nil || request == nil {
		return nil, fmt.Errorf("invalid parameters: targetAddr, remote, and request must not be nil")
	}

	url, err := tool.BuildPrepareUploadURL(targetAddr, remote, pin)
	if err != nil {
		return nil, fmt.Errorf("failed to build prepare-upload URL: %v", err)
	}

	payload, err := sonic.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal prepare-upload request: %v", err)
	}

	req, err := tool.NewHTTPReqWithApplication(http.NewRequest("POST", url, bytes.NewReader(payload)))
	if err != nil {
		return nil, fmt.Errorf("failed to create prepare-upload request: %v", err)
	}
	client := tool.GetHttpClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send prepare-upload request: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			tool.DefaultLogger.Errorf("Failed to close response body: %v", err)
		}
	}()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		tool.DefaultLogger.Warnf("Failed to read response body: %v", readErr)
	} else if len(body) > 0 {
		tool.DefaultLogger.Debugf("Prepare-upload response: %s", string(body))
	}

	// check status code
	switch resp.StatusCode {
	case StatusFinishedNoTransfer:
		tool.DefaultLogger.Infof("Prepare-upload finished with no transfer needed for %s", url)
		return nil, nil
	case http.StatusOK:
		if len(body) == 0 {
			return nil, fmt.Errorf("prepare-upload response body is empty")
		}
		var response types.PrepareUploadResponse
		if err := sonic.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to parse prepare-upload response: %v", err)
		}
		if response.SessionId == "" {
			return nil, fmt.Errorf("prepare-upload response missing sessionId")
		}
		if len(response.Files) == 0 {
			return nil, fmt.Errorf("prepare-upload response missing files")
		}
		tool.DefaultLogger.Infof("Prepare-upload request sent successfully to %s", url)
		return &response, nil
	case StatusInvalidBody:
		return nil, fmt.Errorf("prepare-upload request failed: invalid body")
	case StatusPinRequiredOrInvalid:
		// Try to parse error message from response body
		var errorResponse struct {
			Error string `json:"error"`
		}
		if len(body) > 0 {
			if err := sonic.Unmarshal(body, &errorResponse); err == nil && errorResponse.Error != "" {
				// Return the error message from response
				if errorResponse.Error == "PIN required" || errorResponse.Error == "Invalid PIN" ||
					errorResponse.Error == "pin required" || errorResponse.Error == "invalid pin" {
					// Standardize error message
					if errorResponse.Error == "pin required" {
						return nil, fmt.Errorf("pin required")
					}
					if errorResponse.Error == "invalid pin" {
						return nil, fmt.Errorf("invalid PIN")
					}
					return nil, fmt.Errorf("%s", errorResponse.Error)
				}
			}
		}
		// Default error message if parsing fails
		return nil, fmt.Errorf("pin required / invalid PIN")
	case StatusRejected:
		return nil, fmt.Errorf("prepare-upload request rejected")
	case StatusBlockedByOtherSession:
		return nil, fmt.Errorf("prepare-upload blocked by another session")
	case StatusTooManyRequests:
		return nil, fmt.Errorf("prepare-upload too many requests")
	case StatusUnknownReceiverError:
		return nil, fmt.Errorf("prepare-upload receiver error")
	default:
		return nil, fmt.Errorf("prepare-upload request failed: %s", resp.Status)
	}
}

// FetchDeviceInfo fetches device information from the target device using /api/localsend/v2/info endpoint.
// Returns the device info response or an error.
func FetchDeviceInfo(ip string, port int) (*types.CallbackLegacyVersionMessageHTTP, string, error) {
	// Try HTTPS first, then fallback to HTTP
	protocols := []string{"https", "http"}

	var lastErr error
	for _, protocol := range protocols {
		url := tool.BuildInfoURL(protocol, ip, port)

		req, err := tool.NewHTTPReqWithApplication(http.NewRequest("GET", url, nil))
		if err != nil {
			lastErr = fmt.Errorf("failed to create info request: %v", err)
			continue
		}

		client := tool.GetHttpClient()
		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to send info request to %s: %v", url, err)
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil {
			tool.DefaultLogger.Errorf("Failed to close response body: %v", closeErr)
		}

		if readErr != nil {
			lastErr = fmt.Errorf("failed to read info response body: %v", readErr)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("info request failed with status: %s", resp.Status)
			continue
		}

		var deviceInfo types.CallbackLegacyVersionMessageHTTP
		if err := sonic.Unmarshal(body, &deviceInfo); err != nil {
			lastErr = fmt.Errorf("failed to parse info response: %v", err)
			continue
		}

		tool.DefaultLogger.Infof("FetchDeviceInfo: successfully got device info from %s: %s (fingerprint: %s)",
			url, deviceInfo.Alias, deviceInfo.Fingerprint)
		return &deviceInfo, protocol, nil
	}

	return nil, "", fmt.Errorf("failed to fetch device info from %s:%d: %v", ip, port, lastErr)
}
