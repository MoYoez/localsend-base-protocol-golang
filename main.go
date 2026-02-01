package main

import (
	"strings"

	"github.com/charmbracelet/log"
	"github.com/moyoez/localsend-base-protocol-golang/api"
	"github.com/moyoez/localsend-base-protocol-golang/boardcast"
	"github.com/moyoez/localsend-base-protocol-golang/notify"
	"github.com/moyoez/localsend-base-protocol-golang/tool"
	"github.com/moyoez/localsend-base-protocol-golang/types"
)

func main() {
	cfg := tool.SetFlags()
	appCfg, err := tool.LoadConfig(cfg.UseConfigPath)
	if err != nil {
		tool.DefaultLogger.Fatalf("%v", err)
	}
	if cfg.UseMultcastAddress != "" {
		boardcast.SetMultcastAddress(cfg.UseMultcastAddress)
	}
	if cfg.UseMultcastPort > 0 {
		boardcast.SetMultcastPort(cfg.UseMultcastPort)
	}
	if cfg.UseReferNetworkInterface != "" {
		boardcast.SetReferNetworkInterface(cfg.UseReferNetworkInterface)
	}
	if cfg.UseDefaultUploadFolder != "" {
		api.DefaultUploadFolder = cfg.UseDefaultUploadFolder
		api.SetDefaultUploadFolder(cfg.UseDefaultUploadFolder)
	}
	if cfg.UseAlias != "" {
		appCfg.Alias = cfg.UseAlias
	}
	if !cfg.UseHttps {
		appCfg.Protocol = "http"
	} else {
		appCfg.Protocol = "https"
	}

	if cfg.SkipNotify {
		notify.UseNotify = false
	}

	// Determine autoSaveFromFavorites: flag overrides config if set
	autoSaveFromFavorites := appCfg.AutoSaveFromFavorites
	if cfg.UseAutoSaveFromFavorites {
		autoSaveFromFavorites = true
	}
	tool.SetProgramConfigStatus(cfg.UsePin, cfg.UseAutoSave, autoSaveFromFavorites)

	// initialize logger
	tool.InitLogger()

	// Download API: config or flag
	downloadEnabled := appCfg.Download
	if cfg.UseDownload {
		downloadEnabled = true
	}

	message := &types.VersionMessage{
		Alias:       appCfg.Alias,
		Version:     appCfg.Version,
		DeviceModel: appCfg.DeviceModel,
		DeviceType:  appCfg.DeviceType,
		Fingerprint: appCfg.Fingerprint,
		Port:        appCfg.Port,
		Protocol:    appCfg.Protocol,
		Download:    downloadEnabled,
		Announce:    true,
	}
	api.SetSelfDevice(message)

	if cfg.UseWebOutPath != "" {
		api.WebOutPath = cfg.UseWebOutPath
	}
	if cfg.Log == "" {
		tool.DefaultLogger.SetLevel(log.DebugLevel)
	} else {
		switch strings.ToLower(cfg.Log) {
		case "dev":
			tool.DefaultLogger.SetLevel(log.DebugLevel)
		case "prod":
			tool.DefaultLogger.SetLevel(log.InfoLevel)
		case "none":
			tool.DefaultLogger.SetLevel(log.FatalLevel)
		default:
			tool.DefaultLogger.Warnf("Unknown log mode %q, using debug level", cfg.Log)
			tool.DefaultLogger.SetLevel(log.DebugLevel)
		}
	}

	handler := api.NewDefaultHandler()
	// Determine config path for TLS certificate storage
	// due to protocol request, need to 53317 by default
	apiServer := api.NewServerWithConfig(53317, appCfg.Protocol, handler, cfg.UseConfigPath)
	go func() {
		if err := apiServer.Start(); err != nil {
			tool.DefaultLogger.Fatalf("API server startup failed: %v", err)
			panic(err)
		}
	}()

	// Prepare HTTP version message for scan config
	httpMessage := &types.VersionMessageHTTP{
		Alias:       appCfg.Alias,
		Version:     appCfg.Version,
		DeviceModel: appCfg.DeviceModel,
		DeviceType:  appCfg.DeviceType,
		Fingerprint: appCfg.Fingerprint,
		Port:        appCfg.Port,
		Protocol:    appCfg.Protocol,
		Download:    appCfg.Download,
	}

	// Set scan timeout (default 500 seconds)
	scanTimeout := cfg.ScanTimeout

	switch {
	case cfg.UseLegacyMode:
		tool.DefaultLogger.Info("Using Legacy Mode: HTTP scanning (scanning every 30 seconds)")
		// Set scan config for scan-now API
		boardcast.SetScanConfig(boardcast.ScanModeHTTP, message, httpMessage, scanTimeout)
		go boardcast.ListenMulticastUsingHTTPWithTimeout(httpMessage, scanTimeout)
	case cfg.UseMixedScan:
		tool.DefaultLogger.Info("Using Mixed Scan Mode: UDP and HTTP scanning")
		// Set scan config for scan-now API
		boardcast.SetScanConfig(boardcast.ScanModeMixed, message, httpMessage, scanTimeout)
		go boardcast.ListenMulticastUsingUDP(message)
		go boardcast.SendMulticastUsingUDPWithTimeout(message, scanTimeout)
		go boardcast.ListenMulticastUsingHTTPWithTimeout(httpMessage, scanTimeout)
	default:
		tool.DefaultLogger.Info("Using UDP multicast mode")
		// Set scan config for scan-now API
		boardcast.SetScanConfig(boardcast.ScanModeUDP, message, httpMessage, scanTimeout)
		go boardcast.ListenMulticastUsingUDP(message)
		go boardcast.SendMulticastUsingUDPWithTimeout(message, scanTimeout)
	}

	select {}
}
