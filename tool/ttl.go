package tool

import (
	"fmt"
	"time"

	ttlworker "github.com/FloatTech/ttl"
)

const (
	DefaultTTL = 3600 * time.Second
)

var (
	SessionCache = ttlworker.NewCache[string, bool](DefaultTTL)
)

func JoinSession(sessionId string) error {
	if SessionCache.Get(sessionId) {
		return fmt.Errorf("session %s already joined", sessionId)
	}
	SessionCache.Set(sessionId, true)
	DefaultLogger.Debugf("Session %s joined", sessionId)
	return nil
}

func QuerySessionIsValid(sessionId string) bool {
	DefaultLogger.Debugf("Querying session %s validity", sessionId)
	valid := SessionCache.Get(sessionId)
	if valid {
		DefaultLogger.Debugf("Session %s is valid", sessionId)
		return true
	}
	DefaultLogger.Debugf("Session %s is invalid", sessionId)
	return false
}

func DestorySession(sessionId string) {
	DefaultLogger.Debugf("Destroying session %s", sessionId)
	SessionCache.Delete(sessionId)
	DefaultLogger.Debugf("Session %s destroyed", sessionId)
}
