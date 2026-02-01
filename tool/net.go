package tool

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/moyoez/localsend-base-protocol-golang/types"
)

// UDP4 unsupport multicast
func RejectUnsupportNetworkInterface(iface *net.Interface) bool {
	if iface.Flags&net.FlagUp == 0 {
		return true
	}
	if iface.Flags&net.FlagLoopback != 0 {
		return true
	}
	if iface.Flags&net.FlagPointToPoint != 0 {
		return true // utun / tun / vpn
	}
	if iface.Flags&net.FlagMulticast == 0 {
		return true
	}
	// reject no v4 ipaddress.
	ips, err := iface.Addrs()
	if err != nil {
		return true
	}
	for _, ip := range ips {
		if ipnet, ok := ip.(*net.IPNet); ok && ipnet.IP.To4() != nil && !ipnet.IP.IsLoopback() {
			return false
		}
	}
	return true
}

func GetLocalIPv4Set() map[string]struct{} {
	result := make(map[string]struct{})

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return result
	}

	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		ip := ipnet.IP
		if ip == nil || ip.IsLoopback() {
			continue
		}

		ipv4 := ip.To4()
		if ipv4 == nil {
			continue
		}

		result[ipv4.String()] = struct{}{}
	}

	return result
}

// generateNetworkIPs generates all IP addresses in the given network.
// For /24 networks, it generates IPs from .1 to .254
// For larger networks, it limits to 254 IPs to avoid excessive scanning.
func GenerateNetworkIPs(ipnet *net.IPNet) []string {
	var ips []string
	ip := ipnet.IP.To4()
	if ip == nil {
		return ips
	}

	mask := ipnet.Mask
	network := ip.Mask(mask)

	// Calculate the number of host bits
	ones, bits := mask.Size()
	if bits != 32 {
		return ips
	}
	hostBits := 32 - ones

	// Limit to 254 hosts to avoid excessive scanning
	maxHosts := 254
	if hostBits < 8 {
		// For smaller networks, use actual host count
		maxHosts = (1 << hostBits) - 2 // -2 for network and broadcast
	}

	// Generate IPs by incrementing the host part
	for i := 1; i <= maxHosts; i++ {
		ip := make(net.IP, 4)
		copy(ip, network)

		// Calculate host part based on network size
		if hostBits <= 8 {
			// Simple case: host part is in last octet only
			ip[3] = network[3] + byte(i)
		} else if hostBits <= 16 {
			// Host part spans last two octets
			ip[3] = network[3] + byte(i&0xff)
			ip[2] = network[2] + byte((i>>8)&0xff)
		} else {
			// Host part spans last three octets
			ip[3] = network[3] + byte(i&0xff)
			ip[2] = network[2] + byte((i>>8)&0xff)
			ip[1] = network[1] + byte((i>>16)&0xff)
		}

		// Skip if it equals the network address
		if ip.Equal(network) {
			continue
		}

		ips = append(ips, ip.String())
	}
	return ips
}

// quickTCPProbe checks if a port is open using a fast TCP connection attempt.
// Returns true if the port is open, false otherwise.
func QuickTCPProbe(ip string, port int, timeout time.Duration) bool {
	addr := net.JoinHostPort(ip, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	DefaultLogger.Debugf("quickTCPProbe: dial %s success", addr)
	conn.Close()
	return true
}

func NewHTTPReqWithApplication(req *http.Request, err error) (*http.Request, error) {
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil

}

// IsAddrNotAvailableError detects address-not-available errors across platforms.
func IsAddrNotAvailableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EADDRNOTAVAIL) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "can't assign requested address") ||
		strings.Contains(msg, "cannot assign requested address") ||
		strings.Contains(msg, "address not available")
}

// shouldRespond determines if the device should respond to the incoming message (internal use).
func ShouldRespond(self *types.VersionMessage, incoming *types.VersionMessage) bool {
	if incoming == nil || !incoming.Announce {
		return false
	}
	if self != nil && self.Fingerprint != "" && incoming.Fingerprint == self.Fingerprint {
		return false
	}
	return true
}
