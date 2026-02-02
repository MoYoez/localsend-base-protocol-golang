package models

import (
	"context"
	"maps"
	"sync"

	ttlworker "github.com/FloatTech/ttl"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// SessionContext holds the context and cancel function for a session
type SessionContext struct {
	Ctx    context.Context
	Cancel context.CancelFunc
}

// SessionUploadStats tracks upload statistics for a session
type SessionUploadStats struct {
	TotalFiles    int
	SuccessFiles  int
	FailedFiles   int
	FailedFileIds []string
}

var (
	uploadSessionMu       sync.RWMutex
	DefaultUploadFolder   = "uploads"
	DoNotMakeSessionFolder bool // if true, save under upload folder only; same filename -> name-2.ext, name-3.ext, ...
	uploadSessions      = ttlworker.NewCache[string, map[string]types.FileInfo](tool.DefaultTTL)
	uploadValidated     = ttlworker.NewCache[string, bool](tool.DefaultTTL)
	confirmRecvChans    = ttlworker.NewCache[string, chan types.ConfirmResult](tool.DefaultTTL)
	v1Sessions          = ttlworker.NewCache[string, string](tool.DefaultTTL)
	// sessionContexts stores the context for each session to support cancellation
	sessionContexts = ttlworker.NewCache[string, *SessionContext](tool.DefaultTTL)
	// uploadStats tracks success/failure counts per session
	uploadStats = ttlworker.NewCache[string, *SessionUploadStats](tool.DefaultTTL)
)

func CacheUploadSession(sessionId string, files map[string]types.FileInfo) {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()
	copied := make(map[string]types.FileInfo, len(files))
	maps.Copy(copied, files)
	uploadSessions.Set(sessionId, copied)
}

func LookupFileInfo(sessionId, fileId string) (types.FileInfo, bool) {
	uploadSessionMu.RLock()
	defer uploadSessionMu.RUnlock()
	files := uploadSessions.Get(sessionId)
	if files == nil {
		return types.FileInfo{}, false
	}
	info, exists := files[fileId]
	return info, exists
}

func RemoveUploadedFile(sessionId, fileId string) {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()
	files := uploadSessions.Get(sessionId)
	if files == nil {
		return
	}
	delete(files, fileId)
	if len(files) == 0 {
		uploadSessions.Delete(sessionId)
		return
	}
	uploadSessions.Set(sessionId, files)
}

// InitSessionStats initializes upload statistics for a session
func InitSessionStats(sessionId string, totalFiles int) {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()
	uploadStats.Set(sessionId, &SessionUploadStats{
		TotalFiles:    totalFiles,
		SuccessFiles:  0,
		FailedFiles:   0,
		FailedFileIds: make([]string, 0),
	})
}

// MarkFileUploadedAndCheckComplete marks a file as uploaded (success or failure) and returns
// (remaining, isLast, stats) to help determine if all files are done
func MarkFileUploadedAndCheckComplete(sessionId, fileId string, success bool) (remaining int, isLast bool, stats *SessionUploadStats) {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()

	files := uploadSessions.Get(sessionId)
	if files == nil {
		return 0, true, nil
	}

	// Update stats
	sessionStats := uploadStats.Get(sessionId)
	if sessionStats == nil {
		sessionStats = &SessionUploadStats{
			TotalFiles:    len(files),
			FailedFileIds: make([]string, 0),
		}
	}

	if success {
		sessionStats.SuccessFiles++
	} else {
		sessionStats.FailedFiles++
		sessionStats.FailedFileIds = append(sessionStats.FailedFileIds, fileId)
	}
	uploadStats.Set(sessionId, sessionStats)

	// Remove from pending files
	delete(files, fileId)
	remaining = len(files)
	isLast = remaining == 0

	if isLast {
		uploadSessions.Delete(sessionId)
		// Keep stats for the notification, will be cleaned up later
	} else {
		uploadSessions.Set(sessionId, files)
	}

	return remaining, isLast, sessionStats
}

// GetSessionStats returns the upload statistics for a session
func GetSessionStats(sessionId string) *SessionUploadStats {
	uploadSessionMu.RLock()
	defer uploadSessionMu.RUnlock()
	return uploadStats.Get(sessionId)
}

// CleanupSessionStats removes the upload statistics for a session
func CleanupSessionStats(sessionId string) {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()
	uploadStats.Delete(sessionId)
}

func RemoveUploadSession(sessionId string) {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()
	uploadSessions.Delete(sessionId)
	uploadValidated.Delete(sessionId)
	confirmRecvChans.Delete(sessionId)
	// Cancel the session context to interrupt ongoing uploads
	if sessCtx := sessionContexts.Get(sessionId); sessCtx != nil {
		sessCtx.Cancel()
		sessionContexts.Delete(sessionId)
	}
}

func IsSessionValidated(sessionId string) bool {
	uploadSessionMu.RLock()
	defer uploadSessionMu.RUnlock()
	return uploadValidated.Get(sessionId)
}

func MarkSessionValidated(sessionId string) {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()
	uploadValidated.Set(sessionId, true)
}

func SetConfirmRecvChannel(sessionId string, ch chan types.ConfirmResult) {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()
	confirmRecvChans.Set(sessionId, ch)
}

func GetConfirmRecvChannel(sessionId string) (chan types.ConfirmResult, bool) {
	uploadSessionMu.RLock()
	defer uploadSessionMu.RUnlock()
	ch := confirmRecvChans.Get(sessionId)
	if ch == nil {
		return nil, false
	}
	return ch, true
}

func DeleteConfirmRecvChannel(sessionId string) {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()
	confirmRecvChans.Delete(sessionId)
}

func GetUploadSessionFiles(sessionId string) (map[string]types.FileInfo, bool) {
	uploadSessionMu.RLock()
	defer uploadSessionMu.RUnlock()
	files := uploadSessions.Get(sessionId)
	if files == nil {
		return nil, false
	}
	copied := make(map[string]types.FileInfo, len(files))
	maps.Copy(copied, files)
	return copied, true
}

// StoreV1Session stores the IP -> sessionId mapping for V1 protocol
func StoreV1Session(ip, sessionId string) {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()
	v1Sessions.Set(ip, sessionId)
}

// GetV1Session retrieves the sessionId for the given IP address (V1 protocol)
func GetV1Session(ip string) string {
	uploadSessionMu.RLock()
	defer uploadSessionMu.RUnlock()
	return v1Sessions.Get(ip)
}

// RemoveV1Session removes the IP -> sessionId mapping for V1 protocol
func RemoveV1Session(ip string) {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()
	v1Sessions.Delete(ip)
}

// CreateSessionContext creates a new context for the session and returns it
func CreateSessionContext(sessionId string) context.Context {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	sessionContexts.Set(sessionId, &SessionContext{
		Ctx:    ctx,
		Cancel: cancel,
	})
	return ctx
}

// GetSessionContext returns the context for the session, or nil if not found
func GetSessionContext(sessionId string) context.Context {
	uploadSessionMu.RLock()
	defer uploadSessionMu.RUnlock()
	sessCtx := sessionContexts.Get(sessionId)
	if sessCtx == nil {
		return nil
	}
	return sessCtx.Ctx
}

// IsSessionCancelled checks if the session has been cancelled
func IsSessionCancelled(sessionId string) bool {
	ctx := GetSessionContext(sessionId)
	if ctx == nil {
		return true // Session not found, treat as cancelled
	}
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
