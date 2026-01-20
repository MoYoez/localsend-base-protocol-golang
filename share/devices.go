package share

import (
	"time"

	ttlworker "github.com/FloatTech/ttl"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// ttl

type UserScanCurrentItem struct {
	Ipaddress string `json:"ip_address"`
	types.VersionMessage
}

const (
	DefaultTTL = 120 * time.Second
)

var (
	UserScanCurrent = ttlworker.NewCache[string, UserScanCurrentItem](DefaultTTL)
)

func SetUserScanCurrent(sessionId string, data UserScanCurrentItem) {
	UserScanCurrent.Set(sessionId, data)
	tool.DefaultLogger.Debugf("Set user scan current: %s", sessionId)
}

func GetUserScanCurrent(sessionId string) (UserScanCurrentItem, bool) {
	data := UserScanCurrent.Get(sessionId)
	return data, data.Ipaddress != ""
}

func ListUserScanCurrent() []string {
	keys := make([]string, 0)
	err := UserScanCurrent.Range(func(k string, v UserScanCurrentItem) error {
		keys = append(keys, k)
		return nil
	})
	if err != nil {
		return nil
	}
	return keys
}
