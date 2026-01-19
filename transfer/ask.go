package transfer

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/bytedance/sonic"
	"github.com/charmbracelet/log"
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

	urlBytes, err := tool.BuildPrepareUploadURL(targetAddr, remote, pin)
	if err != nil {
		return nil, fmt.Errorf("failed to build prepare-upload URL: %v", err)
	}
	url := tool.BytesToString(urlBytes)

	payload, err := sonic.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal prepare-upload request: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create prepare-upload request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := tool.NewHTTPClient(remote.Protocol)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send prepare-upload request: %v", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		log.Warnf("Failed to read response body: %v", readErr)
	} else if len(body) > 0 {
		log.Debugf("Prepare-upload response: %s", string(body))
	}

	// check status code
	switch resp.StatusCode {
	case StatusFinishedNoTransfer:
		log.Infof("Prepare-upload finished with no transfer needed for %s", url)
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
		log.Infof("Prepare-upload request sent successfully to %s", url)
		return &response, nil
	case StatusInvalidBody:
		return nil, fmt.Errorf("prepare-upload request failed: invalid body")
	case StatusPinRequiredOrInvalid:
		return nil, fmt.Errorf("prepare-upload request failed: pin required or invalid")
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
