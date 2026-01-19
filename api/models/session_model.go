package models

import (
	"sync"

	"github.com/moyoez/localsend-base-protocol-golang/types"
)

var (
	uploadSessionMu     sync.RWMutex
	DefaultUploadFolder = "uploads"
	uploadSessions      = map[string]map[string]types.FileInfo{}
	uploadValidated     = map[string]bool{}
)

func CacheUploadSession(sessionId string, files map[string]types.FileInfo) {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()
	copied := make(map[string]types.FileInfo, len(files))
	for fileId, info := range files {
		copied[fileId] = info
	}
	uploadSessions[sessionId] = copied
}

func LookupFileInfo(sessionId, fileId string) (types.FileInfo, bool) {
	uploadSessionMu.RLock()
	defer uploadSessionMu.RUnlock()
	files, ok := uploadSessions[sessionId]
	if !ok {
		return types.FileInfo{}, false
	}
	info, exists := files[fileId]
	return info, exists
}

func RemoveUploadedFile(sessionId, fileId string) {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()
	files, ok := uploadSessions[sessionId]
	if !ok {
		return
	}
	delete(files, fileId)
	if len(files) == 0 {
		delete(uploadSessions, sessionId)
	}
}

func RemoveUploadSession(sessionId string) {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()
	delete(uploadSessions, sessionId)
	delete(uploadValidated, sessionId)
}

func IsSessionValidated(sessionId string) bool {
	uploadSessionMu.RLock()
	defer uploadSessionMu.RUnlock()
	return uploadValidated[sessionId]
}

func MarkSessionValidated(sessionId string) {
	uploadSessionMu.Lock()
	defer uploadSessionMu.Unlock()
	uploadValidated[sessionId] = true
}
