package transfer

import (
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// UploadFile sends file data to the receiver.
// Uses sessionId, fileId, and token from /prepare-upload response.
func UploadFile(targetAddr *net.UDPAddr, remote *types.VersionMessage, sessionId, fileId, token string, data io.Reader) error {
	if targetAddr == nil || remote == nil {
		return fmt.Errorf("invalid parameters: targetAddr and remote must not be nil")
	}
	if sessionId == "" || fileId == "" || token == "" {
		return fmt.Errorf("invalid parameters: sessionId, fileId, and token must not be empty")
	}
	if data == nil {
		return fmt.Errorf("invalid parameters: data must not be nil")
	}

	urlBytes, err := tool.BuildUploadURL(targetAddr, remote, sessionId, fileId, token)
	if err != nil {
		return fmt.Errorf("failed to build upload URL: %v", err)
	}
	url := tool.BytesToString(urlBytes)

	req, err := http.NewRequest("POST", url, data)
	if err != nil {
		return fmt.Errorf("failed to create upload request: %v", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	client := tool.NewHTTPClient(remote.Protocol)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send upload request: %v", err)
	}
	defer resp.Body.Close()

	// check status code
	switch resp.StatusCode {
	case http.StatusBadRequest:
		return fmt.Errorf("Missing parameters")
	case http.StatusForbidden:
		return fmt.Errorf("Invalid token or IP address")
	case http.StatusConflict:
		return fmt.Errorf("Blocked by another session")
	case http.StatusInternalServerError:
		return fmt.Errorf("Unknown receiver error")
	default:
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return fmt.Errorf("upload request failed: %s", resp.Status)
		}
	}

	tool.DefaultLogger.Infof("Upload request sent successfully to %s", url)
	return nil
}
