package types

import "time"

// ShareFileEntry holds file metadata and local path for download
type ShareFileEntry struct {
	FileInfo  FileInfo
	LocalPath string // path on disk for serving
}

// ShareSession represents a share session for the download API
type ShareSession struct {
	SessionId  string
	Files      map[string]ShareFileEntry
	CreatedAt  time.Time
	Pin        string
	AutoAccept bool
}

// CreateShareSessionRequest represents the request body for creating a share session
type CreateShareSessionRequest struct {
	Files      map[string]FileInput `json:"files"`
	Pin        string               `json:"pin,omitempty"`
	AutoAccept bool                 `json:"autoAccept"`
}

// CreateShareSessionResponse represents the response for create-share-session
type CreateShareSessionResponse struct {
	SessionId   string `json:"sessionId"`
	DownloadUrl string `json:"downloadUrl"`
}
