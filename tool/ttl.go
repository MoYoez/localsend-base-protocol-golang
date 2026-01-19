package tool

import (
	"fmt"
	"time"

	ttlworker "github.com/FloatTech/ttl"
	"github.com/charmbracelet/log"
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
	log.Debugf("Session %s joined", sessionId)
	return nil
}

func QuerySessionIsValid(sessionId string) bool {
	log.Debugf("Querying session %s validity", sessionId)
	valid := SessionCache.Get(sessionId)
	if valid {
		log.Debugf("Session %s is valid", sessionId)
		return true
	}
	log.Debugf("Session %s is invalid", sessionId)
	return false
}

func DestorySession(sessionId string) {
	log.Debugf("Destroying session %s", sessionId)
	SessionCache.Delete(sessionId)
	log.Debugf("Session %s destroyed", sessionId)
}
