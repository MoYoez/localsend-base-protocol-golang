package boardcast

import (
	"fmt"

	"github.com/moyoez/localsend-go/tool"
	"github.com/moyoez/localsend-go/types"
)

// SetScanConfig sets the current scan configuration for scan-now API
func SetScanConfig(mode types.ScanMode, selfMessage *types.VersionMessage, selfHTTP *types.VersionMessageHTTP, timeout int) {
	currentScanConfigMu.Lock()
	defer currentScanConfigMu.Unlock()
	currentScanConfig = &types.ScanConfig{
		Mode:        mode,
		SelfMessage: selfMessage,
		SelfHTTP:    selfHTTP,
		Timeout:     timeout,
	}
}

// GetScanConfig returns the current scan configuration
func GetScanConfig() *types.ScanConfig {
	currentScanConfigMu.RLock()
	defer currentScanConfigMu.RUnlock()
	return currentScanConfig
}

// ScanOnceUDP sends a single UDP multicast message to trigger device discovery.
func ScanOnceUDP(message *types.VersionMessage) error {
	return SendMulticastOnce(message)
}

// RestartAutoScan sends a restart signal to all running auto scan loops.
// This resets their timeout timers and triggers an immediate scan.
func RestartAutoScan() {
	autoScanControlMu.Lock()
	defer autoScanControlMu.Unlock()

	if autoScanRestartCh == nil {
		tool.DefaultLogger.Debug("No auto scan restart channel, creating one")
		autoScanRestartCh = make(chan struct{}, 1)
	}

	select {
	case autoScanRestartCh <- struct{}{}:
		tool.DefaultLogger.Info("Auto scan restart signal sent")
	default:
		tool.DefaultLogger.Debug("Auto scan restart channel full, signal already pending")
	}
}

// IsAutoScanRunning returns whether any auto scan loop is currently running.
func IsAutoScanRunning() bool {
	autoScanControlMu.Lock()
	defer autoScanControlMu.Unlock()
	return autoScanHTTPRunning || autoScanUDPRunning
}

// ScanNow performs a single scan based on current configuration.
// If auto scan has timed out (stopped), it restarts the auto scan loops.
// If auto scan is still running, it sends a restart signal to reset the timeout.
// Returns error if scan config is not set or scan fails.
func ScanNow() error {
	config := GetScanConfig()
	if config == nil {
		return fmt.Errorf("scan config not set")
	}

	tool.DefaultLogger.Info("Performing manual scan...")

	if IsAutoScanRunning() {
		tool.DefaultLogger.Debug("Auto scan is running, sending restart signal")
		RestartAutoScan()
	} else {
		tool.DefaultLogger.Info("Auto scan has stopped, restarting auto scan loops")
		restartAutoScanLoops(config)
	}

	switch config.Mode {
	case types.ScanModeUDP:
		if config.SelfMessage == nil {
			return fmt.Errorf("self message not configured for UDP scan")
		}
		tool.DefaultLogger.Debug("Sending UDP multicast scan...")
		return ScanOnceUDP(config.SelfMessage)

	case types.ScanModeHTTP:
		if config.SelfHTTP == nil {
			return fmt.Errorf("self HTTP message not configured for HTTP scan")
		}
		tool.DefaultLogger.Debug("Performing HTTP scan...")
		return ScanOnceHTTP(config.SelfHTTP)

	case types.ScanModeMixed:
		var udpErr, httpErr error
		if config.SelfMessage != nil {
			tool.DefaultLogger.Debug("Sending UDP multicast scan (mixed mode)...")
			udpErr = ScanOnceUDP(config.SelfMessage)
			if udpErr != nil {
				tool.DefaultLogger.Warnf("UDP scan failed: %v", udpErr)
			}
		}
		if config.SelfHTTP != nil {
			tool.DefaultLogger.Debug("Performing HTTP scan (mixed mode)...")
			httpErr = ScanOnceHTTP(config.SelfHTTP)
			if httpErr != nil {
				tool.DefaultLogger.Warnf("HTTP scan failed: %v", httpErr)
			}
		}
		if udpErr != nil && httpErr != nil {
			return fmt.Errorf("both UDP and HTTP scan failed: UDP: %v, HTTP: %v", udpErr, httpErr)
		}
		return nil

	default:
		return fmt.Errorf("unknown scan mode: %d", config.Mode)
	}
}

// restartAutoScanLoops restarts the auto scan goroutines based on configuration.
// This is called when auto scan has timed out and needs to be restarted.
func restartAutoScanLoops(config *types.ScanConfig) {
	if config == nil {
		return
	}
	timeout := config.Timeout
	switch config.Mode {
	case types.ScanModeUDP:
		if config.SelfMessage != nil {
			go SendMulticastUsingUDPWithTimeout(config.SelfMessage, timeout)
		}
	case types.ScanModeHTTP:
		if config.SelfHTTP != nil {
			go ListenMulticastUsingHTTPWithTimeout(config.SelfHTTP, timeout)
		}
	case types.ScanModeMixed:
		if config.SelfMessage != nil {
			go SendMulticastUsingUDPWithTimeout(config.SelfMessage, timeout)
		}
		if config.SelfHTTP != nil {
			go ListenMulticastUsingHTTPWithTimeout(config.SelfHTTP, timeout)
		}
	}
}
