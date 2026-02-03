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
	// httpScanConcurrencyLimit limits the number of concurrent HTTP scan goroutines
	httpScanConcurrencyLimit = 25
	// tcpProbeTimeout is the timeout for TCP port probe
	tcpProbeTimeout = 500 * time.Millisecond
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
	autoScanRestartCh   chan struct{} // channel to signal restart
	autoScanHTTPRunning bool
	autoScanUDPRunning  bool
)

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
		return []*net.Interface{iface}, nil
	}

	// use the system default interface
	return []*net.Interface{nil}, nil
}

// getCachedNetworkIPs returns cached network IPs or generates new ones if cache is invalid.
// It detects network interface changes by comparing the current interface addresses hash.
func getCachedNetworkIPs() ([]string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	// Build a cache key based on current interface addresses
	var keyBuilder strings.Builder
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
