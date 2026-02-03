package main

import (
	"github.com/charmbracelet/log"
	"github.com/moyoez/localsend-go/api"
	"github.com/moyoez/localsend-go/boardcast"
	"github.com/moyoez/localsend-go/notify"
	"github.com/moyoez/localsend-go/tool"
	"github.com/moyoez/localsend-go/types"
)

func main() {
	// method: always use config first, then flag overwrite config.
	FlagConfig := tool.SetFlags() // get flags
	appCfg, err := tool.LoadConfig(FlagConfig.UseConfigPath)
	if err != nil {
		tool.DefaultLogger.Fatalf("%v", err)
	}
	tool.InitLogger()

	// set user self action.
	message, httpMessage := tool.BuildVersionMessages(&appCfg, FlagConfig)
	api.SetSelfDevice(message)

	// set default sets here.

	// LOG
	switch FlagConfig.Log {
	case "dev":
		tool.DefaultLogger.SetLevel(log.DebugLevel)
	case "prod":
		tool.DefaultLogger.SetLevel(log.InfoLevel)
	case "none":
		tool.DefaultLogger.SetLevel(log.ErrorLevel)
	default:
		tool.DefaultLogger.SetLevel(log.InfoLevel)
	}

	// sets here.
	boardcast.SetMultcastAddress(FlagConfig.UseMultcastAddress)
	boardcast.SetMultcastPort(FlagConfig.UseMultcastPort)
	boardcast.SetReferNetworkInterface(FlagConfig.UseReferNetworkInterface)
	api.SetDefaultUploadFolder(FlagConfig.UseDefaultUploadFolder)
	api.SetDoNotMakeSessionFolder(FlagConfig.DoNotMakeSessionFolder)
	tool.SetProgramConfigStatus(FlagConfig.UsePin, FlagConfig.UseAutoSave, FlagConfig.UseAutoSaveFromFavorites)
	api.SetDefaultWebOutPath(FlagConfig.UseWebOutPath)
	notify.SetUseNotify(!FlagConfig.SkipNotify)

	// armed, clear this area. // port should focus on 53317
	apiServer := api.NewServerWithConfig(53317, message.Protocol, FlagConfig.UseConfigPath)
	go func() {
		if err := apiServer.Start(); err != nil {
			tool.DefaultLogger.Fatalf("API server startup failed: %v", err)
			panic(err)
		}
	}()

	switch {
	case FlagConfig.UseLegacyMode:
		tool.DefaultLogger.Info("Using Legacy Mode: HTTP scanning")
		boardcast.SetScanConfig(types.ScanModeHTTP, message, httpMessage, FlagConfig.ScanTimeout)
		go boardcast.ListenMulticastUsingHTTPWithTimeout(httpMessage, FlagConfig.ScanTimeout)
	case FlagConfig.UseMixedScan:
		tool.DefaultLogger.Info("Using Mixed Scan Mode: UDP and HTTP scanning")
		boardcast.SetScanConfig(types.ScanModeMixed, message, httpMessage, FlagConfig.ScanTimeout)
		go boardcast.ListenMulticastUsingUDP(message)
		go boardcast.SendMulticastUsingUDPWithTimeout(message, FlagConfig.ScanTimeout)
		go boardcast.ListenMulticastUsingHTTPWithTimeout(httpMessage, FlagConfig.ScanTimeout)
	default:
		tool.DefaultLogger.Info("Using UDP multicast mode")
		boardcast.SetScanConfig(types.ScanModeUDP, message, httpMessage, FlagConfig.ScanTimeout)
		go boardcast.ListenMulticastUsingUDP(message)
		go boardcast.SendMulticastUsingUDPWithTimeout(message, FlagConfig.ScanTimeout)
	}

	select {}
}
