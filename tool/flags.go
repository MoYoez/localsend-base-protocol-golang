package tool

import "flag"

// Config holds runtime overrides from CLI flags.
type Config struct {
	Log                      string
	UseMultcastAddress       string
	UseMultcastPort          int
	UseConfigPath            string
	UseDefaultUploadFolder   string
	UseLegacyMode            bool
	UseReferNetworkInterface string // fixes when using virtual network interface. e.g. Clash TUN.
	UsePin                   string
	UseAutoSave              bool // if false, user require to confirm before recv.
	UseAutoSaveFromFavorites bool // if true and useAutoSave is false, auto-accept from favorite devices only.
	UseAlias                 string
	UseMixedScan             bool // if true, use mixed scan mode, both UDP and HTTP.
	SkipNotify               bool // if true, skip notify mode.
	UseHttps                 bool   // if true, use https protocol; if false, use http protocol. Alias for protocol config.
	ScanTimeout              int    // scan timeout in seconds, default 500. After timeout, auto scan will stop.
	UseDownload              bool   // if true, enable download API (prepare-download, download, download page)
	UseWebOutPath            string // path to Next.js static export output (default: web/out)
}

// SetFlags parses CLI flags and returns the override config.
func SetFlags() Config {
	var cfg Config
	flag.StringVar(&cfg.Log, "log", "", "log mode: dev|prod|none")
	flag.StringVar(&cfg.UseMultcastAddress, "useMultcastAddress", "", "override multicast address")
	flag.IntVar(&cfg.UseMultcastPort, "useMultcastPort", 0, "override multicast port")
	flag.StringVar(&cfg.UseConfigPath, "useConfigPath", "", "override config file path")
	flag.StringVar(&cfg.UseDefaultUploadFolder, "useDefaultUploadFolder", "", "override default upload folder")
	flag.BoolVar(&cfg.UseLegacyMode, "useLegacyMode", false, "use legacy HTTP mode to scan devices (scan every 30 seconds)")
	flag.StringVar(&cfg.UseReferNetworkInterface, "useReferNetworkInterface", "*", "specify network interface (e.g., 'en0', 'eth0') or '*' for all interfaces")
	flag.StringVar(&cfg.UsePin, "usePin", "", "specify pin for upload (only for FROM upload request)")
	flag.BoolVar(&cfg.UseAutoSave, "useAutoSave", true, "if false, user require to confirm before recv (only for FROM upload request)")
	flag.BoolVar(&cfg.UseAutoSaveFromFavorites, "useAutoSaveFromFavorites", false, "if true and useAutoSave is false, auto-accept from favorite devices only")
	flag.StringVar(&cfg.UseAlias, "useAlias", "", "specify alias for the device")
	flag.BoolVar(&cfg.UseMixedScan, "useMixedScan", false, "if true, use mixed scan mode, both UDP and HTTP.")
	flag.BoolVar(&cfg.SkipNotify, "skipNotify", false, "if true, skip notify mode.")
	flag.BoolVar(&cfg.UseHttps, "useHttps", true, "if true, use https (encrypted); if false, use http (unencrypted). Alias for protocol config.")
	flag.IntVar(&cfg.ScanTimeout, "scanTimeout", 500, "scan timeout in seconds, default 500. After timeout, auto scan will stop. Set to 0 to disable timeout.")
	flag.BoolVar(&cfg.UseDownload, "useDownload", false, "if true, enable download API (prepare-download, download, download page)")
	flag.StringVar(&cfg.UseWebOutPath, "webOutPath", "web/out", "path to Next.js static export output for download page")
	flag.Parse()
	return cfg
}
