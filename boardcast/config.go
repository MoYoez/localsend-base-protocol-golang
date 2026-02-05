package boardcast

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/moyoez/localsend-go/tool"
	"github.com/moyoez/localsend-go/types"
)

// refer to https://github.com/localsend/protocol/blob/main/README.md#1-defaults
const (
	defaultMultcastAddress = "224.0.0.167"
	defaultMultcastPort    = 53317 // UDP & HTTP
	// scanNowHTTPConcurrency is the concurrency cap for scan-now (no rate limit; high concurrency for speed)
	scanNowHTTPConcurrency = 256
	// autoScanConcurrencyLimit limits concurrent HTTP scan goroutines for periodic auto scan (16~32)
	autoScanConcurrencyLimit = 24
	// autoScanICMPRatePPS is the ICMP probe rate limit (packets per second) for auto scan; /24 ~ 6~12s
	autoScanICMPRatePPS = 30
	// icmpProbeTimeout is the timeout for ICMP echo probe (host reachability before HTTP register)
	icmpProbeTimeout = 200 * time.Millisecond
)

var (
	multcastAddress       = defaultMultcastAddress
	multcastPort          = defaultMultcastPort
	referNetworkInterface string // the specified network interface name
	listenAllInterfaces   = true // whether to listen on all network interfaces

	// networkIPsCache caches generated network IPs to avoid repeated generation
	networkIPsCacheMu  sync.RWMutex
	networkIPsCache    []string
	networkIPsCacheKey string // stores interface addresses hash to detect changes

	// currentScanConfig holds the current scan configuration
	currentScanConfigMu sync.RWMutex
	currentScanConfig   *types.ScanConfig

	// autoScanControl controls the auto scan loops
	autoScanControlMu   sync.Mutex
	autoScanRestartCh   chan restartAction // channel to signal restart
	autoScanHTTPRunning bool
	autoScanUDPRunning  bool
)

// restartAction is sent on autoScanRestartCh. When SkipHTTPImmediateScan is true (e.g. after scan-now),
// HTTP loop only resets timeout and does not run scanOnce() immediately; next scan is in 30s.
type restartAction struct {
	SkipHTTPImmediateScan bool
}

// SetMultcastAddress overrides the default multicast address
func SetMultcastAddress(address string) {
	if address != "" {
		multcastAddress = address
	}
}

// SetMultcastPort overrides the default multicast port
func SetMultcastPort(port int) {
	if port > 0 {
		multcastPort = port
	}
}

// SetReferNetworkInterface sets the network interface to use for multicast.
// If interfaceName is empty, it will use the system default interface.
// If interfaceName is "*", it will listen on all available interfaces.
func SetReferNetworkInterface(interfaceName string) {
	if interfaceName != "" && interfaceName != "*" {
		listenAllInterfaces = false
		referNetworkInterface = interfaceName
	}
}

// getNetworkInterfaces returns a list of network interfaces to listen on.
// If listenAllInterfaces is true, returns all valid interfaces.
// If referNetworkInterface is set, returns only that interface.
// Otherwise, returns nil (use system default).
func getNetworkInterfaces() ([]*net.Interface, error) {
	if listenAllInterfaces {
		// gain all network interfaces
		interfaces, err := net.Interfaces()
		if err != nil {
			return nil, fmt.Errorf("failed to get network interfaces: %v", err)
		}

		var validInterfaces []*net.Interface
		for i := range interfaces {
			iface := &interfaces[i]
			// remove tun connections.
			if tool.RejectUnsupportNetworkInterface(iface) {
				continue
			}
			validInterfaces = append(validInterfaces, iface)
		}

		if len(validInterfaces) == 0 {
			return nil, fmt.Errorf("no valid network interfaces found")
		}

		return validInterfaces, nil
	} else if referNetworkInterface != "" {
		// get the specified network interface
		iface, err := net.InterfaceByName(referNetworkInterface)
		if err != nil {
			return nil, fmt.Errorf("failed to get network interface %s: %v", referNetworkInterface, err)
		}
		if tool.RejectUnsupportNetworkInterface(iface) {
			return nil, fmt.Errorf("network interface %s is not supported", referNetworkInterface)
		}
		return []*net.Interface{iface}, nil
	}

	// use the system default interface
	return []*net.Interface{nil}, nil
}

// getCachedNetworkIPs returns cached network IPs or generates new ones if cache is invalid.
// It strictly follows useReferNetworkInterface: when a specific interface is set, only IPs from that interface's network(s) are returned.
// Cache key includes interface config to invalidate on config change.
func getCachedNetworkIPs() ([]string, error) {
	var addrs []net.Addr
	interfaces, err := getNetworkInterfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range interfaces {
		if iface == nil {
			// system default: fall back to InterfaceAddrs
			allAddrs, err := net.InterfaceAddrs()
			if err != nil {
				return nil, err
			}
			addrs = append(addrs, allAddrs...)
			continue
		}
		ifaceAddrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		addrs = append(addrs, ifaceAddrs...)
	}

	// Build a cache key: include interface config + addresses (for change detection)
	var keyBuilder strings.Builder
	keyBuilder.WriteString("li:")
	keyBuilder.WriteString(fmt.Sprint(listenAllInterfaces))
	keyBuilder.WriteString(";rif:")
	keyBuilder.WriteString(referNetworkInterface)
	keyBuilder.WriteString(";")
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
			continue
		}
		keyBuilder.WriteString(ipnet.String())
		keyBuilder.WriteString(";")
	}
	currentKey := keyBuilder.String()

	// Check if cache is valid
	networkIPsCacheMu.RLock()
	if networkIPsCacheKey == currentKey && len(networkIPsCache) > 0 {
		// Cache hit: return a copy to avoid race conditions
		result := make([]string, len(networkIPsCache))
		copy(result, networkIPsCache)
		networkIPsCacheMu.RUnlock()
		return result, nil
	}
	networkIPsCacheMu.RUnlock()

	// Cache miss: generate new IPs
	var targets []string
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
			continue
		}
		networkIPs := tool.GenerateNetworkIPs(ipnet)
		targets = append(targets, networkIPs...)
	}

	// Update cache
	networkIPsCacheMu.Lock()
	networkIPsCache = targets
	networkIPsCacheKey = currentKey
	networkIPsCacheMu.Unlock()

	// Return a copy
	result := make([]string, len(targets))
	copy(result, targets)
	return result, nil
}

// GetPreferredOutgoingBindAddr returns the local address to bind outgoing HTTP connections to.
// When useReferNetworkInterface specifies a concrete interface (not "*"), returns the first
// valid IPv4 address on that interface so HTTP requests use that interface.
// Returns (nil, nil) when listenAllInterfaces is true or referNetworkInterface is empty.
// Returns an error when the specified interface has no valid IPv4 address.
func GetPreferredOutgoingBindAddr() (*net.TCPAddr, error) {
	if listenAllInterfaces || referNetworkInterface == "" {
		return nil, nil
	}
	iface, err := net.InterfaceByName(referNetworkInterface)
	if err != nil {
		return nil, fmt.Errorf("failed to get network interface %s: %w", referNetworkInterface, err)
	}
	if tool.RejectUnsupportNetworkInterface(iface) {
		return nil, fmt.Errorf("network interface %s is not supported", referNetworkInterface)
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, fmt.Errorf("failed to get addresses for interface %s: %w", referNetworkInterface, err)
	}
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
			continue
		}
		return &net.TCPAddr{IP: ipnet.IP, Port: 0}, nil
	}
	return nil, fmt.Errorf("interface %s has no valid IPv4 address", referNetworkInterface)
}
