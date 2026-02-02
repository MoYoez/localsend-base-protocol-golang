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

// baseConfig
type BaseConfig struct {
	Protocol        string
	DownloadEnabled bool
	ScanTimeout     int
}

func applyBoardcastFromFlags(cfg tool.Config) {
	if cfg.UseMultcastAddress != "" {
		boardcast.SetMultcastAddress(cfg.UseMultcastAddress)
	}
	if cfg.UseMultcastPort > 0 {
		boardcast.SetMultcastPort(cfg.UseMultcastPort)
	}
	if cfg.UseReferNetworkInterface != "" {
		boardcast.SetReferNetworkInterface(cfg.UseReferNetworkInterface)
	}
}

func applyAPIFromFlags(cfg tool.Config) {
	if cfg.UseDefaultUploadFolder != "" {
		api.SetDefaultUploadFolder(cfg.UseDefaultUploadFolder)
	}
	api.SetDoNotMakeSessionFolder(cfg.DoNotMakeSessionFolder)
	if cfg.UseWebOutPath != "" {
		api.WebOutPath = cfg.UseWebOutPath
	}
}

// mergeAppConfig applies CLI overrides to appCfg, sets notify and program config, returns merged values.
func mergeAppConfig(cfg tool.Config, appCfg *tool.AppConfig) BaseConfig {
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
	autoSaveFromFavorites := appCfg.AutoSaveFromFavorites
	if cfg.UseAutoSaveFromFavorites {
		autoSaveFromFavorites = true
	}
	tool.SetProgramConfigStatus(cfg.UsePin, cfg.UseAutoSave, autoSaveFromFavorites)

	downloadEnabled := appCfg.Download
	if cfg.UseDownload {
		downloadEnabled = true
	}

	return BaseConfig{
		Protocol:        appCfg.Protocol,
		DownloadEnabled: downloadEnabled,
		ScanTimeout:     cfg.ScanTimeout,
	}
}

func buildVersionMessages(appCfg *tool.AppConfig, downloadEnabled bool) (*types.VersionMessage, *types.VersionMessageHTTP) {
	msg := &types.VersionMessage{
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
	httpMsg := &types.VersionMessageHTTP{
		Alias:       appCfg.Alias,
		Version:     appCfg.Version,
		DeviceModel: appCfg.DeviceModel,
		DeviceType:  appCfg.DeviceType,
		Fingerprint: appCfg.Fingerprint,
		Port:        appCfg.Port,
		Protocol:    appCfg.Protocol,
		Download:    downloadEnabled,
	}
	return msg, httpMsg
}

func setLogLevel(cfg tool.Config) {
	if cfg.Log == "" {
		tool.DefaultLogger.SetLevel(log.DebugLevel)
		return
	}
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

// startAPIServer starts the API server in a goroutine (default port 53317 per protocol).
func startAPIServer(port int, protocol, configPath string) {
	apiServer := api.NewServerWithConfig(port, protocol, configPath)
	go func() {
		if err := apiServer.Start(); err != nil {
			tool.DefaultLogger.Fatalf("API server startup failed: %v", err)
			panic(err)
		}
	}()
}

func startScanMode(cfg tool.Config, message *types.VersionMessage, httpMessage *types.VersionMessageHTTP, scanTimeout int) {
	// Set scan config for scan-now API
	switch {
	case cfg.UseLegacyMode:
		tool.DefaultLogger.Info("Using Legacy Mode: HTTP scanning (scanning every 30 seconds)")
		boardcast.SetScanConfig(boardcast.ScanModeHTTP, message, httpMessage, scanTimeout)
		go boardcast.ListenMulticastUsingHTTPWithTimeout(httpMessage, scanTimeout)
	case cfg.UseMixedScan:
		tool.DefaultLogger.Info("Using Mixed Scan Mode: UDP and HTTP scanning")
		boardcast.SetScanConfig(boardcast.ScanModeMixed, message, httpMessage, scanTimeout)
		go boardcast.ListenMulticastUsingUDP(message)
		go boardcast.SendMulticastUsingUDPWithTimeout(message, scanTimeout)
		go boardcast.ListenMulticastUsingHTTPWithTimeout(httpMessage, scanTimeout)
	default:
		tool.DefaultLogger.Info("Using UDP multicast mode")
		boardcast.SetScanConfig(boardcast.ScanModeUDP, message, httpMessage, scanTimeout)
		go boardcast.ListenMulticastUsingUDP(message)
		go boardcast.SendMulticastUsingUDPWithTimeout(message, scanTimeout)
	}
}

func main() {
	cfg := tool.SetFlags()
	appCfg, err := tool.LoadConfig(cfg.UseConfigPath)
	if err != nil {
		tool.DefaultLogger.Fatalf("%v", err)
	}

	applyBoardcastFromFlags(cfg)
	applyAPIFromFlags(cfg)
	base := mergeAppConfig(cfg, &appCfg)

	message, httpMessage := buildVersionMessages(&appCfg, base.DownloadEnabled)
	api.SetSelfDevice(message)

	tool.InitLogger()
	setLogLevel(cfg)

	startAPIServer(53317, base.Protocol, cfg.UseConfigPath)
	startScanMode(cfg, message, httpMessage, base.ScanTimeout)

	select {}
}
