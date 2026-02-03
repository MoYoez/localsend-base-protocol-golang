package tool

import (
	"github.com/moyoez/localsend-go/types"
)

func BuildVersionMessages(appCfg *types.AppConfig, Flags types.Config) (*types.VersionMessage, *types.VersionMessageHTTP) {
	if Flags.UseAlias != "" {
		appCfg.Alias = Flags.UseAlias
	}
	// pin read flag.
	if Flags.UseHttp {
		appCfg.Protocol = "http"
	}

	if Flags.UseDownload {
		appCfg.Download = true
	}

	msg := &types.VersionMessage{
		Alias:       appCfg.Alias,
		Version:     appCfg.Version,
		DeviceModel: appCfg.DeviceModel,
		DeviceType:  appCfg.DeviceType,
		Fingerprint: appCfg.Fingerprint,
		Port:        appCfg.Port,
		Protocol:    appCfg.Protocol,
		Download:    appCfg.Download,
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
		Download:    appCfg.Download,
	}
	return msg, httpMsg
}
