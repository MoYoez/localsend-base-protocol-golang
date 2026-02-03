package models

import (
	"sync"
	"time"

	ttlworker "github.com/FloatTech/ttl"
	"github.com/moyoez/localsend-go/tool"
	"github.com/moyoez/localsend-go/types"
)

const (
	ShareSessionTTL = 3600 * time.Second // 1 hour
)

var (
	shareSessionMu        sync.RWMutex
	shareSessions         = ttlworker.NewCache[string, *types.ShareSession](ShareSessionTTL)
	confirmDownloadChans  = ttlworker.NewCache[string, chan types.ConfirmResult](tool.DefaultTTL)
	confirmedDownloadSess = ttlworker.NewCache[string, bool](ShareSessionTTL) // 已确认的会话，同意后可直接下载任意文件
)

// CacheShareSession stores a share session
func CacheShareSession(session *types.ShareSession) {
	shareSessionMu.Lock()
	defer shareSessionMu.Unlock()
	shareSessions.Set(session.SessionId, session)
}

// GetShareSession retrieves a share session by ID
func GetShareSession(sessionId string) (*types.ShareSession, bool) {
	shareSessionMu.RLock()
	defer shareSessionMu.RUnlock()
	sess := shareSessions.Get(sessionId)
	if sess == nil {
		return nil, false
	}
	return sess, true
}

// RemoveShareSession removes a share session
func RemoveShareSession(sessionId string) {
	shareSessionMu.Lock()
	defer shareSessionMu.Unlock()
	shareSessions.Delete(sessionId)
	confirmDownloadChans.Delete(sessionId)
	confirmedDownloadSess.Delete(sessionId)
}

// IsDownloadConfirmed returns true if the session has been confirmed (user agreed, can download any file)
func IsDownloadConfirmed(sessionId string) bool {
	shareSessionMu.RLock()
	defer shareSessionMu.RUnlock()
	return confirmedDownloadSess.Get(sessionId)
}

// MarkDownloadConfirmed marks the session as confirmed after user agrees
func MarkDownloadConfirmed(sessionId string) {
	shareSessionMu.Lock()
	defer shareSessionMu.Unlock()
	confirmedDownloadSess.Set(sessionId, true)
}

// SetConfirmDownloadChannel sets the channel for confirm-download callback
func SetConfirmDownloadChannel(sessionId string, ch chan types.ConfirmResult) {
	shareSessionMu.Lock()
	defer shareSessionMu.Unlock()
	confirmDownloadChans.Set(sessionId, ch)
}

// GetConfirmDownloadChannel retrieves the confirm-download channel
func GetConfirmDownloadChannel(sessionId string) (chan types.ConfirmResult, bool) {
	shareSessionMu.RLock()
	defer shareSessionMu.RUnlock()
	ch := confirmDownloadChans.Get(sessionId)
	if ch == nil {
		return nil, false
	}
	return ch, true
}

// DeleteConfirmDownloadChannel removes the confirm-download channel
func DeleteConfirmDownloadChannel(sessionId string) {
	shareSessionMu.Lock()
	defer shareSessionMu.Unlock()
	confirmDownloadChans.Delete(sessionId)
}

// GetShareSessionFiles returns the files map for prepare-download response
func GetShareSessionFiles(session *types.ShareSession) map[string]types.FileInfo {
	files := make(map[string]types.FileInfo, len(session.Files))
	for id, entry := range session.Files {
		files[id] = entry.FileInfo
	}
	return files
}

// LookupShareFile looks up a file in a share session
func LookupShareFile(session *types.ShareSession, fileId string) (types.ShareFileEntry, bool) {
	entry, ok := session.Files[fileId]
	return entry, ok
}
