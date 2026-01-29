package transfer

import (
	"fmt"
	"net"
	"net/http"

	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// CancelSession cancels a transfer session.
// Uses sessionId from /send-request or /prepare-upload response.
func CancelSession(targetAddr *net.UDPAddr, remote *types.VersionMessage, sessionId string) error {
	if targetAddr == nil || remote == nil {
		return fmt.Errorf("invalid parameters: targetAddr and remote must not be nil")
	}
	if sessionId == "" {
		return fmt.Errorf("invalid parameters: sessionId must not be empty")
	}

	urlBytes, err := tool.BuildCancelURL(targetAddr, remote, sessionId)
	if err != nil {
		return fmt.Errorf("failed to build cancel URL: %v", err)
	}
	url := tool.BytesToString(urlBytes)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create cancel request: %v", err)
	}

	client := tool.GetHttpClient()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send cancel request: %v", err)
	}
	defer resp.Body.Close()

	// check status code
	if resp.StatusCode == http.StatusBadRequest {
		return fmt.Errorf("Missing parameters")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("cancel request failed: %s", resp.Status)
	}

	tool.DefaultLogger.Infof("Cancel request sent successfully to %s", url)
	return nil
}
