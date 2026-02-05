package share

import (
	"fmt"
	"net"
	"time"

	ttlworker "github.com/FloatTech/ttl"

	"github.com/moyoez/localsend-go/notify"
	"github.com/moyoez/localsend-go/tool"
	"github.com/moyoez/localsend-go/types"
)

const (
	DefaultTTL = 300 * time.Second // set 300 seconds.
)

var (
	UserScanCurrent = ttlworker.NewCache[string, types.UserScanCurrentItem](DefaultTTL)
)

func SetUserScanCurrent(sessionId string, data types.UserScanCurrentItem) {
	// Check if device exists and if info has changed
	existing, exists := GetUserScanCurrent(sessionId)

	isNew := !exists
	isChanged := exists && hasDeviceInfoChanged(existing, data)

	// Set the new data
	UserScanCurrent.Set(sessionId, data)
	tool.DefaultLogger.Debugf("Set user scan current: %s", sessionId)

	// Send notification if new device or info changed
	if isNew || isChanged {
		var eventType string
		if isNew {
			eventType = types.NotifyTypeDeviceDiscovered
			tool.DefaultLogger.Infof("New device discovered: %s (%s) at %s", data.Alias, data.Fingerprint, data.Ipaddress)
		} else {
			eventType = types.NotifyTypeDeviceUpdated
			tool.DefaultLogger.Infof("Device info updated: %s (%s) at %s", data.Alias, data.Fingerprint, data.Ipaddress)
		}

		// Send notification
		notification := &types.Notification{
			Type:    eventType,
			Title:   "Device " + map[bool]string{true: "Discovered", false: "Updated"}[isNew],
			Message: fmt.Sprintf("%s at %s", data.Alias, data.Ipaddress),
			Data: map[string]any{
				"fingerprint": data.Fingerprint,
				"alias":       data.Alias,
				"ip_address":  data.Ipaddress,
				"port":        data.Port,
				"protocol":    data.Protocol,
				"deviceType":  data.DeviceType,
				"deviceModel": data.DeviceModel,
				"version":     data.Version,
				"isNew":       isNew,
			},
		}
		if err := notify.SendNotification(notification, ""); err != nil {
			tool.DefaultLogger.Debugf("Failed to send device notification: %v", err)
		}
	}
}

// hasDeviceInfoChanged checks if device info has changed
func hasDeviceInfoChanged(a, b types.UserScanCurrentItem) bool {
	return a.Ipaddress != b.Ipaddress ||
		a.Fingerprint != b.Fingerprint ||
		a.Alias != b.Alias ||
		a.Port != b.Port ||
		a.Protocol != b.Protocol ||
		a.DeviceType != b.DeviceType ||
		a.DeviceModel != b.DeviceModel ||
		a.Version != b.Version
}

func GetUserScanCurrent(sessionId string) (types.UserScanCurrentItem, bool) {
	data := UserScanCurrent.Get(sessionId)
	return data, data.Ipaddress != ""
}

func ListUserScanCurrent() []string {
	keys := make([]string, 0)
	err := UserScanCurrent.Range(func(k string, v types.UserScanCurrentItem) error {
		keys = append(keys, k)
		return nil
	})
	if err != nil {
		return nil
	}
	return keys
}

// ClearUserScanCurrent removes all entries from the scan result cache.
// Used by scan-now to clear the list before performing a fresh scan.
func ClearUserScanCurrent() {
	keys := ListUserScanCurrent()
	for _, k := range keys {
		UserScanCurrent.Delete(k)
	}
}

// GetSelfNetworkInfos returns all valid local network interfaces with their IP and segment number.
// It ignores tun/vpn interfaces and loopback interfaces.
// The number is derived from the last octet of the IP address.
// For example: 192.168.3.12 -> #12
func GetSelfNetworkInfos() []types.SelfNetworkInfo {
	var result []types.SelfNetworkInfo

	interfaces, err := net.Interfaces()
	if err != nil {
		tool.DefaultLogger.Errorf("Failed to get network interfaces: %v", err)
		return result
	}

	for _, iface := range interfaces {
		// use tool package function to filter unsupported interfaces (including tun)
		if tool.RejectUnsupportNetworkInterface(&iface) {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipnet.IP.To4()
			if ip == nil || ip.IsLoopback() {
				continue
			}

			// get last number of ip address as number
			lastOctet := int(ip[3])
			number := fmt.Sprintf("#%d", lastOctet)

			result = append(result, types.SelfNetworkInfo{
				InterfaceName: iface.Name,
				IPAddress:     ip.String(),
				Number:        number,
				NumberInt:     lastOctet,
			})
		}
	}

	return result
}
