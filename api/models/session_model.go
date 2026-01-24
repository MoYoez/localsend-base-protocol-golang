package models

import (
	"maps"
	"sync"

	ttlworker "github.com/FloatTech/ttl"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

var (
	uploadSessionMu     sync.RWMutex
	DefaultUploadFolder = "uploads"
	uploadSessions      = ttlworker.NewCache[string, map[string]types.FileInfo](tool.DefaultTTL)
	uploadValidated     = ttlworker.NewCache[string, bool](tool.DefaultTTL)
	confirmRecvChans    = ttlworker.NewCache[string, chan types.ConfirmResult](tool.DefaultTTL)
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

func RemoveUploadSession(sessionId string) {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()
	uploadSessions.Delete(sessionId)
	uploadValidated.Delete(sessionId)
	confirmRecvChans.Delete(sessionId)
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
