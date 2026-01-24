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
	UseAlias                 string
}

// SetFlags parses CLI flags and returns the override config.
func SetFlags() Config {
	var cfg Config
	flag.StringVar(&cfg.Log, "log", "", "log mode: dev|prod")
	flag.StringVar(&cfg.UseMultcastAddress, "useMultcastAddress", "", "override multicast address")
	flag.IntVar(&cfg.UseMultcastPort, "useMultcastPort", 0, "override multicast port")
	flag.StringVar(&cfg.UseConfigPath, "useConfigPath", "", "override config file path")
	flag.StringVar(&cfg.UseDefaultUploadFolder, "useDefaultUploadFolder", "", "override default upload folder")
	flag.BoolVar(&cfg.UseLegacyMode, "useLegacyMode", false, "use legacy HTTP mode to scan devices (scan every 30 seconds)")
	flag.StringVar(&cfg.UseReferNetworkInterface, "useReferNetworkInterface", "*", "specify network interface (e.g., 'en0', 'eth0') or '*' for all interfaces")
	flag.StringVar(&cfg.UsePin, "usePin", "", "specify pin for upload (only for FROM upload request)")
	flag.BoolVar(&cfg.UseAutoSave, "useAutoSave", true, "if false, user require to confirm before recv (only for FROM upload request)")
	flag.StringVar(&cfg.UseAlias, "useAlias", "", "specify alias for the device")

	flag.Parse()
	return cfg
}
