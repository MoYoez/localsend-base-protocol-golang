package tool

import (
	"flag"

	"github.com/moyoez/localsend-go/types"
)

// SetFlags parses CLI flags and returns the override config.
func SetFlags() types.Config {
	var cfg types.Config
	flag.StringVar(&cfg.Log, "log", "prod", "log mode: dev|prod|none")
	flag.StringVar(&cfg.UseMultcastAddress, "useMultcastAddress", "", "override multicast address")
	flag.IntVar(&cfg.UseMultcastPort, "useMultcastPort", 0, "override multicast port")
	flag.StringVar(&cfg.UseConfigPath, "useConfigPath", "config.yaml", "override config file path")
	flag.StringVar(&cfg.UseDefaultUploadFolder, "useDefaultUploadFolder", "uploads", "override default upload folder")
	flag.BoolVar(&cfg.UseLegacyMode, "useLegacyMode", false, "use legacy HTTP mode to scan devices (scan every 30 seconds)")
	flag.StringVar(&cfg.UseReferNetworkInterface, "useReferNetworkInterface", "*", "specify network interface (e.g., 'en0', 'eth0') or '*' for all interfaces")
	flag.StringVar(&cfg.UsePin, "usePin", "", "specify pin for upload (only for FROM upload request)")
	flag.BoolVar(&cfg.UseAutoSave, "useAutoSave", false, "if false, user require to confirm before recv (only for FROM upload request)")
	flag.BoolVar(&cfg.UseAutoSaveFromFavorites, "useAutoSaveFromFavorites", false, "if true and useAutoSave is false, auto-accept from favorite devices only")
	flag.StringVar(&cfg.UseAlias, "useAlias", "", "specify alias for the device")
	flag.BoolVar(&cfg.UseMixedScan, "useMixedScan", false, "if true, use mixed scan mode, both UDP and HTTP.")
	flag.BoolVar(&cfg.SkipNotify, "skipNotify", false, "if true, skip notify mode.")
	flag.BoolVar(&cfg.UseHttp, "useHttp", false, "if true, use http; if false, use https. Alias for protocol config.")
	flag.IntVar(&cfg.ScanTimeout, "scanTimeout", 500, "scan timeout in seconds, default 500. After timeout, auto scan will stop. Set to 0 to disable timeout.")
	flag.BoolVar(&cfg.UseDownload, "useDownload", false, "if true, enable download API (prepare-download, download, download page)")
	flag.StringVar(&cfg.UseWebOutPath, "useWebOutPath", "", "path to Next.js static export output for download page, maybe you dont need to change.")
	flag.BoolVar(&cfg.DoNotMakeSessionFolder, "doNotMakeSessionFolder", false, "if true, do not create session subfolder; when file name exists, save as name-2.ext, name-3.ext, ...")
	flag.Parse()
	return cfg
}
