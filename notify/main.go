package notify

import (
	"fmt"
	"io"
	"maps"
	"net"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
)

// Configuration for Unix Domain Socket notification
var (
	// DefaultUnixSocketPath is the default Unix socket path for IPC
	DefaultUnixSocketPath = "/tmp/localsend-notify.sock"
	// UnixSocketTimeout is the timeout for Unix socket operations
	UnixSocketTimeout = 3 * time.Second // set unix socket quickly, actually they dont need to change
	UseNotify         = true
)

// Notification represents a notification message structure
type Notification struct {
	Type       string         `json:"type,omitempty"`       // Notification type, e.g. "upload_start", "upload_end", etc.
	Title      string         `json:"title,omitempty"`      // Notification title
	Message    string         `json:"message,omitempty"`    // Notification message/content
	Data       map[string]any `json:"data,omitempty"`       // Additional data fields
	IsTextOnly bool           `json:"isTextOnly,omitempty"` // Indicates if this is plain text content
}

// SetUseNotify sets whether to use notify
func SetUseNotify(use bool) {
	UseNotify = use
}

// SendNotification sends notification via Unix Domain Socket
func SendNotification(notification *Notification, socketPath string) error {
	if !UseNotify {
		return nil
	}
	if socketPath == "" {
		socketPath = DefaultUnixSocketPath
	}

	// Check if socket file exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return fmt.Errorf("unix socket not found: %s (is the Python server running?)", socketPath)
	}

	// Serialize notification data to JSON
	var payload []byte
	var err error
	if notification != nil {
		payload, err = sonic.Marshal(notification)
		if err != nil {
			return fmt.Errorf("failed to serialize notification data: %v", err)
		}
	} else {
		payload = []byte("{}")
	}

	// Connect to Unix socket
	conn, err := net.DialTimeout("unix", socketPath, UnixSocketTimeout)
	if err != nil {
		return fmt.Errorf("failed to connect to Unix socket %s: %v", socketPath, err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			tool.DefaultLogger.Errorf("Failed to close Unix socket connection: %v", err)
		}
	}()

	// Set write deadline
	err = conn.SetWriteDeadline(time.Now().Add(UnixSocketTimeout))
	if err != nil {
		tool.DefaultLogger.Errorf("Failed to set write deadline: %v", err)
	}

	// Send data
	tool.DefaultLogger.Debugf("Sending notification to Unix socket: %s", tool.BytesToString(payload))
	_, err = conn.Write(payload)
	if err != nil {
		return fmt.Errorf("failed to write to Unix socket: %v", err)
	}

	// Set read deadline
	err = conn.SetReadDeadline(time.Now().Add(UnixSocketTimeout))
	if err != nil {
		tool.DefaultLogger.Errorf("Failed to set read deadline: %v", err)
	}

	// Read response
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read response from Unix socket: %v", err)
	}

	// Parse response
	var response map[string]any
	if n > 0 {
		if err := sonic.Unmarshal(buf[:n], &response); err != nil {
			tool.DefaultLogger.Debugf("Unix socket response (raw): %s", string(buf[:n]))
		} else {
			tool.DefaultLogger.Debugf("Unix socket response: %v", response)
			// Check for error in response
			if errMsg, ok := response["error"].(string); ok && errMsg != "" {
				return fmt.Errorf("server returned error: %s", errMsg)
			}
		}
	}

	// Log success
	if notification != nil {
		tool.DefaultLogger.Infof("[UnixSocket] Notification sent: %s - %s", notification.Type, notification.Title)
	} else {
		tool.DefaultLogger.Infof("[UnixSocket] Notification sent")
	}

	return nil
}

// SendUploadNotification sends upload-related notifications using Unix Domain Socket
// eventType should be "upload_start" or "upload_end"
func SendUploadNotification(eventType, sessionId, fileId string, fileInfo map[string]any) error {
	notification := &Notification{
		Type: eventType,
		Data: map[string]any{
			"sessionId": sessionId,
			"fileId":    fileId,
		},
	}

	// Add file info if provided
	if fileInfo != nil {
		maps.Copy(notification.Data, fileInfo)
	}

	// Check if this is plain text content
	if fileInfo != nil {
		// First check direct fileType field
		if fileType, ok := fileInfo["fileType"].(string); ok {
			isTxt := false
			if fileName, ok := fileInfo["fileName"].(string); ok {
				isTxt = strings.HasSuffix(strings.ToLower(fileName), ".txt")
			}
			notification.IsTextOnly = isPlainTextType(fileType) || isTxt
			if notification.IsTextOnly {
				tool.DefaultLogger.Infof("[Notify] Detected plain text content: fileType=%s fileName=%v", fileType, fileInfo["fileName"])
			}
		} else if files, ok := fileInfo["files"].([]map[string]any); ok && len(files) == 1 {
			// For single file in batch upload, check the nested file info
			file := files[0]
			if fileType, ok := file["fileType"].(string); ok {
				isTxt := false
				if fileName, ok := file["fileName"].(string); ok {
					isTxt = strings.HasSuffix(strings.ToLower(fileName), ".txt")
				}
				notification.IsTextOnly = isPlainTextType(fileType) || isTxt
				if notification.IsTextOnly {
					tool.DefaultLogger.Infof("[Notify] Detected plain text content from files array: fileType=%s fileName=%v", fileType, file["fileName"])
				}
			}
		}
	}

	// Set title and message based on event type
	switch eventType {
	case "upload_start":
		if notification.IsTextOnly {
			notification.Title = "Text Upload Started"
			notification.Message = fmt.Sprintf("Text content upload started: sessionId=%s, fileId=%s", sessionId, fileId)
		} else {
			notification.Title = "Upload Started"
			notification.Message = fmt.Sprintf("File upload started: sessionId=%s, fileId=%s", sessionId, fileId)
		}
	case "upload_end":
		if notification.IsTextOnly {
			notification.Title = "Text Upload Completed"
			notification.Message = fmt.Sprintf("Text content upload completed: sessionId=%s, fileId=%s", sessionId, fileId)
		} else {
			notification.Title = "Upload Completed"
			notification.Message = fmt.Sprintf("File upload completed: sessionId=%s, fileId=%s", sessionId, fileId)
		}
	default:
		notification.Title = "Upload Event"
		notification.Message = fmt.Sprintf("Upload event: %s, sessionId=%s, fileId=%s", eventType, sessionId, fileId)
	}

	return SendNotification(notification, DefaultUnixSocketPath)
}

// SendSimpleNotification sends a simple text notification
func SendSimpleNotification(title, message string) error {
	notification := &Notification{
		Type:    "info",
		Title:   title,
		Message: message,
	}
	return SendNotification(notification, DefaultUnixSocketPath)
}

// isPlainTextType checks if the given file type is a plain text type
func isPlainTextType(fileType string) bool {
	if fileType == "" {
		return false
	}

	fileType = strings.ToLower(strings.TrimSpace(fileType))

	// Common plain text MIME types
	plainTextTypes := []string{
		"text/plain",
		"text/txt",
		"application/txt",
		"text/x-log",
		"text/x-markdown",
		"text/markdown",
		"text/x-diff",
		"text/x-patch",
	}

	// Check exact match
	if slices.Contains(plainTextTypes, fileType) {
		return true
	}

	// Check if it starts with "text/"
	if strings.HasPrefix(fileType, "text/") {
		return true
	}

	return false
}
