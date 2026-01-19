package models

import (
	"fmt"
	"sync"
	"time"

	"github.com/moyoez/localsend-base-protocol-golang/types"
)

var (
	selfDeviceMu sync.RWMutex
	selfDevice   *types.VersionMessage

	deviceCacheMu sync.RWMutex
	deviceCache   = map[string]discoveredDevice{}
)

type discoveredDevice struct {
	info     types.VersionMessage
	address  string
	lastSeen time.Time
}

func deviceCacheKey(info *types.VersionMessage, address string) string {
	if info == nil {
		return ""
	}
	if info.Fingerprint != "" {
		return info.Fingerprint
	}
	return fmt.Sprintf("%s|%s|%d", address, info.Alias, info.Port)
}

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

func CacheDiscoveredDevice(info *types.VersionMessage, address string) {
	if info == nil {
		return
	}
	key := deviceCacheKey(info, address)
	if key == "" {
		return
	}
	deviceCacheMu.Lock()
	defer deviceCacheMu.Unlock()
	deviceCache[key] = discoveredDevice{
		info:     *info,
		address:  address,
		lastSeen: time.Now(),
	}
}

func ListRecentDevices(since time.Time) []discoveredDevice {
	deviceCacheMu.RLock()
	defer deviceCacheMu.RUnlock()
	devices := make([]discoveredDevice, 0, len(deviceCache))
	for _, device := range deviceCache {
		if device.lastSeen.After(since) || device.lastSeen.Equal(since) {
			devices = append(devices, device)
		}
	}
	return devices
}
