package tool

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/moyoez/localsend-go/types"
	probing "github.com/prometheus-community/pro-bing"
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
	// Ban "Meta" (Clash) router interfaces by ignoring interfaces with "Meta" in the name.
	if strings.Contains(iface.Name, "Meta") {
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

// QuickICMPProbe checks if a host is reachable using ICMP echo (ping).
// Returns true if the host replies within timeout, false otherwise.
// Uses unprivileged mode (UDP-based ping on Linux) so it works without root/CAP_NET_RAW
// (e.g. Steam Deck plugin). See net.ipv4.ping_group_range on Linux.
func QuickICMPProbe(ip string, timeout time.Duration) bool {
	if net.ParseIP(ip) == nil {
		return false
	}
	pinger, err := probing.NewPinger(ip)
	if err != nil {
		DefaultLogger.Debugf("QuickICMPProbe: NewPinger %s: %v", ip, err)
		return false
	}
	// Unprivileged: use UDP ping (Linux) / no raw ICMP, avoids "socket: operation not permitted"
	pinger.SetPrivileged(false)
	pinger.Count = 1
	pinger.Timeout = timeout
	pinger.SetNetwork("ip4")
	if err := pinger.Run(); err != nil {
		DefaultLogger.Debugf("QuickICMPProbe: Run %s: %v", ip, err)
		return false
	}
	ok := pinger.Statistics().PacketsRecv >= 1
	if ok {
		DefaultLogger.Debugf("QuickICMPProbe: %s replied", ip)
	}
	return ok
}

// QuickTCPProbe checks if a port is open using a fast TCP connection attempt.
// Returns true if the port is open, false otherwise.
// Deprecated: Prefer QuickICMPProbe for host reachability (e.g. Steam Deck).
func QuickTCPProbe(ip string, port int, timeout time.Duration) bool {
	addr := net.JoinHostPort(ip, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	DefaultLogger.Debugf("quickTCPProbe: dial %s success", addr)
	defer func() {
		if err := conn.Close(); err != nil {
			DefaultLogger.Errorf("Failed to close TCP connection: %v", err)
		}
	}()
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

// GetIPFromSuffix returns the full IP address from an IP suffix (last octet).
// It finds the local network interface and constructs the full IP using the suffix.
// For example, if local IP is 192.168.1.10 and suffix is "12", returns "192.168.1.12".
// Suffix can be a number like "12" or with hash like "#12".
func GetIPFromSuffix(suffix string) (string, error) {
	// Remove # prefix if present
	suffix = strings.TrimPrefix(suffix, "#")

	// Parse suffix as integer
	lastOctet, err := strconv.Atoi(suffix)
	if err != nil {
		return "", err
	}

	if lastOctet < 1 || lastOctet > 254 {
		return "", errors.New("invalid IP suffix: must be between 1 and 254")
	}

	// Get all local interfaces
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	// Find the first valid IPv4 address and use its network prefix
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		ip := ipnet.IP.To4()
		if ip == nil || ip.IsLoopback() {
			continue
		}

		// Construct the target IP using the same network prefix
		targetIP := net.IPv4(ip[0], ip[1], ip[2], byte(lastOctet))
		return targetIP.String(), nil
	}

	return "", errors.New("no valid local IPv4 address found")
}

// GetAllIPsFromSuffix returns all possible full IP addresses from an IP suffix.
// It checks all local network interfaces and constructs full IPs for each.
func GetAllIPsFromSuffix(suffix string) ([]string, error) {
	// Remove # prefix if present
	suffix = strings.TrimPrefix(suffix, "#")

	// Parse suffix as integer
	lastOctet, err := strconv.Atoi(suffix)
	if err != nil {
		return nil, err
	}

	if lastOctet < 1 || lastOctet > 254 {
		return nil, errors.New("invalid IP suffix: must be between 1 and 254")
	}

	// Get all local interfaces
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	var results []string
	seen := make(map[string]struct{})

	// Find all valid IPv4 addresses and construct target IPs
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		ip := ipnet.IP.To4()
		if ip == nil || ip.IsLoopback() {
			continue
		}

		// Construct the target IP using the same network prefix
		targetIP := net.IPv4(ip[0], ip[1], ip[2], byte(lastOctet)).String()

		// Avoid duplicates
		if _, exists := seen[targetIP]; !exists {
			seen[targetIP] = struct{}{}
			results = append(results, targetIP)
		}
	}

	if len(results) == 0 {
		return nil, errors.New("no valid local IPv4 address found")
	}

	return results, nil
}
