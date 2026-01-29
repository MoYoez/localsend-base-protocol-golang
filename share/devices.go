package share

import (
	"fmt"
	"net"
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

// SelfNetworkInfo represents the local device's network information
// including IP address and broadcast segment number
type SelfNetworkInfo struct {
	InterfaceName string `json:"interface_name"` // network interface name
	IPAddress     string `json:"ip_address"`     // ip address
	Number        string `json:"number"`         // number
	NumberInt     int    `json:"number_int"`     // number int
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

// GetSelfNetworkInfos returns all valid local network interfaces with their IP and segment number.
// It ignores tun/vpn interfaces and loopback interfaces.
// The number is derived from the last octet of the IP address.
// For example: 192.168.3.12 -> #12
func GetSelfNetworkInfos() []SelfNetworkInfo {
	var result []SelfNetworkInfo

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

			result = append(result, SelfNetworkInfo{
				InterfaceName: iface.Name,
				IPAddress:     ip.String(),
				Number:        number,
				NumberInt:     lastOctet,
			})
		}
	}

	return result
}
