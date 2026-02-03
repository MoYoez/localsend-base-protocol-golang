package models

import (
	"sync"

	"github.com/moyoez/localsend-go/types"
)

var (
	selfDeviceMu sync.RWMutex
	selfDevice   *types.VersionMessage
)

// SetSelfDevice sets the local device info used for user-side scanning.
func SetSelfDevice(device *types.VersionMessage) {
	selfDeviceMu.Lock()
	defer selfDeviceMu.Unlock()
	selfDevice = device
}

func GetSelfDevice() *types.VersionMessage {
	selfDeviceMu.RLock()
	defer selfDeviceMu.RUnlock()
	if selfDevice == nil {
		return nil
	}
	copied := *selfDevice
	return &copied
}
