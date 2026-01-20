package notify

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"

	"github.com/bytedance/sonic"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
)

// Notification represents a notification message structure
type Notification struct {
	Type    string                 `json:"type,omitempty"`    // Notification type, e.g. "transfer_complete", "error", "info", etc.
	Title   string                 `json:"title,omitempty"`   // Notification title
	Message string                 `json:"message,omitempty"` // Notification message/content
	Data    map[string]interface{} `json:"data,omitempty"`    // Additional data fields
}

// Options contains options for sending notifications
type Options struct {
	URL     string            // Target URL
	Method  string            // HTTP method, defaults to POST
	Headers map[string]string // Custom HTTP headers
	Timeout int               // Timeout in seconds, 0 means use default
}

// SendNotification sends a notification to the specified HTTP URL
// If notification is nil, an empty JSON object will be sent
func SendNotification(notification *Notification, options Options) error {
	if options.URL == "" {
		return fmt.Errorf("notification URL cannot be empty")
	}

	// Validate URL format and extract protocol
	parsedURL, err := url.Parse(options.URL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %v", err)
	}

	// Extract protocol from URL, default to http
	protocol := parsedURL.Scheme
	if protocol == "" {
		protocol = "http"
	}

	// Default to POST method
	method := options.Method
	if method == "" {
		method = http.MethodPost
	}

	// Serialize notification data to JSON
	var payload []byte
	if notification != nil {
		payload, err = sonic.Marshal(notification)
		if err != nil {
			return fmt.Errorf("failed to serialize notification data: %v", err)
		}
	} else {
		// If notification is nil, send empty JSON object
		payload = []byte("{}")
	}

	// Create HTTP request
	req, err := http.NewRequest(method, options.URL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %v", err)
	}

	// Set default Content-Type
	req.Header.Set("Content-Type", "application/json")

	// Set custom headers
	for key, value := range options.Headers {
		req.Header.Set(key, value)
	}

	// Create HTTP client (based on URL protocol)
	client := tool.NewHTTPClient(protocol)
	if options.Timeout > 0 {
		client.Timeout = tool.DefaultTimeout // Can be extended to support custom timeout
	}

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %v", err)
	}
	defer resp.Body.Close()

	// Read response body (for debugging)
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		tool.DefaultLogger.Debugf("failed to read response body: %v", readErr)
	}

	// Check status code
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		bodyStr := ""
		if len(body) > 0 {
			bodyStr = string(body)
		}
		return fmt.Errorf("notification send failed, HTTP status code: %d, response: %s", resp.StatusCode, bodyStr)
	}

	// Log success
	if notification != nil {
		tool.DefaultLogger.Infof("notification successfully sent to %s: %s - %s", options.URL, notification.Type, notification.Title)
	} else {
		tool.DefaultLogger.Infof("notification successfully sent to %s", options.URL)
	}

	// Log response body (debug mode)
	if len(body) > 0 {
		tool.DefaultLogger.Debugf("notification response: %s", string(body))
	}

	return nil
}

// SendSimpleNotification sends a simple text notification
// This is a convenience function for quickly sending text messages
func SendSimpleNotification(webhookURL, title, message string) error {
	notification := &Notification{
		Type:    "info",
		Title:   title,
		Message: message,
	}
	return SendNotification(notification, Options{URL: webhookURL})
}

// SendJSONNotification sends a notification with custom JSON data
// This is a convenience function for sending JSON data with custom structure
func SendJSONNotification(webhookURL string, data interface{}) error {
	// Serialize any data to JSON
	payload, err := sonic.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to serialize data: %v", err)
	}

	// Try to parse JSON into Notification structure (if possible)
	var notification *Notification
	if err := sonic.Unmarshal(payload, &notification); err == nil && notification != nil {
		// If parsing succeeds, use Notification structure
		return SendNotification(notification, Options{URL: webhookURL})
	}

	// Otherwise, send raw JSON directly
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Extract protocol from URL
	parsedURL, err := url.Parse(webhookURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %v", err)
	}
	protocol := parsedURL.Scheme
	if protocol == "" {
		protocol = "http"
	}

	client := tool.NewHTTPClient(protocol)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("notification send failed, HTTP status code: %d, response: %s", resp.StatusCode, string(body))
	}

	tool.DefaultLogger.Infof("JSON notification successfully sent to %s", webhookURL)
	return nil
}

// DefaultNotificationURL is the default notification endpoint
const DefaultNotificationURL = "http://localhost:9000/api/py-backend/v1/notify"

// SendUploadNotification sends upload-related notifications to the default endpoint
// eventType should be "upload_start" or "upload_end"
func SendUploadNotification(eventType, sessionId, fileId string, fileInfo map[string]interface{}) error {
	notification := &Notification{
		Type: eventType,
		Data: map[string]interface{}{
			"sessionId": sessionId,
			"fileId":    fileId,
		},
	}

	// Add file info if provided
	if fileInfo != nil {
		maps.Copy(notification.Data, fileInfo)
	}

	// Set title and message based on event type
	switch eventType {
	case "upload_start":
		notification.Title = "Upload Started"
		notification.Message = fmt.Sprintf("File upload started: sessionId=%s, fileId=%s", sessionId, fileId)
	case "upload_end":
		notification.Title = "Upload Completed"
		notification.Message = fmt.Sprintf("File upload completed: sessionId=%s, fileId=%s", sessionId, fileId)
	default:
		notification.Title = "Upload Event"
		notification.Message = fmt.Sprintf("Upload event: %s, sessionId=%s, fileId=%s", eventType, sessionId, fileId)
	}

	return SendNotification(notification, Options{URL: DefaultNotificationURL})
}
