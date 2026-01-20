package main

import (
	"strings"

	"github.com/charmbracelet/log"
	"github.com/moyoez/localsend-base-protocol-golang/api"
	"github.com/moyoez/localsend-base-protocol-golang/boardcast"
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
	if cfg.UseDefaultUploadFolder != "" {
		api.DefaultUploadFolder = cfg.UseDefaultUploadFolder
	}
	// initialize logger
	tool.InitLogger()

	message := &types.VersionMessage{
		Alias:       appCfg.Alias,
		Version:     appCfg.Version,
		DeviceModel: appCfg.DeviceModel,
		DeviceType:  appCfg.DeviceType,
		Fingerprint: appCfg.Fingerprint,
		Port:        appCfg.Port,
		Protocol:    appCfg.Protocol,
		Download:    appCfg.Download,
		Announce:    appCfg.Announce,
	}
	api.SetSelfDevice(message)
	if cfg.Log == "" {
		tool.DefaultLogger.SetLevel(log.DebugLevel)
	} else {
		switch strings.ToLower(cfg.Log) {
		case "dev":
			tool.DefaultLogger.SetLevel(log.DebugLevel)
		case "prod":
			tool.DefaultLogger.SetLevel(log.InfoLevel)
		default:
			tool.DefaultLogger.Warnf("Unknown log mode %q, using debug level", cfg.Log)
			tool.DefaultLogger.SetLevel(log.DebugLevel)
		}
	}

	handler := api.NewDefaultHandler()

	apiServer := api.NewServer(appCfg.Port, appCfg.Protocol, handler)
	go func() {
		if err := apiServer.Start(); err != nil {
			tool.DefaultLogger.Fatalf("API server startup failed: %v", err)
		}
	}()

	if cfg.UseLegacyMode {
		tool.DefaultLogger.Info("Using Legacy Mode: HTTP scanning (scanning every 30 seconds)")
		go boardcast.ListenMulticastUsingHTTP(message)
	} else {
		tool.DefaultLogger.Info("Using UDP multicast mode")
		go boardcast.ListenMulticastUsingUDP(message)
		go boardcast.SendMulticastUsingUDP(message)
	}

	select {}
}
