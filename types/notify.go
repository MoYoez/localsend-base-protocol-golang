package types

// Notification type constants (used as Notification.Type).
// Type determines how the receiver (e.g. Decky) interprets the notification and Data shape.
const (
	NotifyTypeUploadStart      = "upload_start"
	NotifyTypeUploadEnd        = "upload_end"
	NotifyTypeConfirmRecv      = "confirm_recv"
	NotifyTypeConfirmDownload  = "confirm_download"
	NotifyTypePinRequired      = "pin_required"
	NotifyTypeDeviceDiscovered = "device_discovered"
	NotifyTypeDeviceUpdated    = "device_updated"
	NotifyTypeInfo             = "info"
)

// Notification represents a notification message structure sent via Unix socket (e.g. to Decky).
// Type: use NotifyTypeXxx constants. Data keys vary by type (sessionId, from, fileCount, files, etc.).
type Notification struct {
	Type       string         `json:"type,omitempty"`       // Notification type; use NotifyTypeXxx constants
	Title      string         `json:"title,omitempty"`      // Notification title
	Message    string         `json:"message,omitempty"`    // Notification message/content
	Data       map[string]any `json:"data,omitempty"`       // Additional data fields (shape depends on Type)
	IsTextOnly bool           `json:"isTextOnly,omitempty"`  // Indicates if this is plain text content (upload notifications)
}
