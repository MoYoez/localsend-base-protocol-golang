package tool

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/google/uuid"
)

func GenerateRandomUUID() string {
	return uuid.New().String()
}

// GenerateShortSessionID returns a short alphanumeric ID (e.g. 8 chars) for share/download URLs.
// Shorter than UUID so links are easier to share and type.
func GenerateShortSessionID() string {
	b := make([]byte, 4) // 4 bytes = 8 hex chars
	if _, err := rand.Read(b); err != nil {
		return GenerateRandomUUID()[:8] // fallback
	}
	return hex.EncodeToString(b)
}
