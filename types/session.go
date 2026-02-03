package types

import "context"

// SessionUploadStats tracks upload statistics for a session
type SessionUploadStats struct {
	TotalFiles    int
	SuccessFiles  int
	FailedFiles   int
	FailedFileIds []string
}

// SessionContext holds the context and cancel function for a session
type SessionContext struct {
	Ctx    context.Context
	Cancel context.CancelFunc
}

// UserUploadSessionContext holds the context and cancel function for a user upload session
type UserUploadSessionContext struct {
	Ctx    context.Context
	Cancel context.CancelFunc
}
