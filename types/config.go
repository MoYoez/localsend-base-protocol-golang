package types

// AppConfig represents the application configuration loaded from config file
type AppConfig struct {
	Alias                 string                `yaml:"alias"`
	Version               string                `yaml:"version"`
	DeviceModel           string                `yaml:"deviceModel"`
	DeviceType            string                `yaml:"deviceType"`
	Fingerprint           string                `yaml:"fingerprint"`
	Port                  int                   `yaml:"port"`
	Protocol              string                `yaml:"protocol"`
	Download              bool                  `yaml:"download"`
	Announce              bool                  `yaml:"announce"`
	CertPEM               string                `yaml:"certPEM,omitempty"`
	KeyPEM                string                `yaml:"keyPEM,omitempty"`
	AutoSaveFromFavorites bool                  `yaml:"autoSaveFromFavorites,omitempty"`
	FavoriteDevices       []FavoriteDeviceEntry `yaml:"favoriteDevices,omitempty"`
}

// ProgramConfig holds runtime program configuration (pin, auto-save, etc.)
type ProgramConfig struct {
	Pin                   string `yaml:"pin"`
	AutoSave              bool   `yaml:"autoSave"`
	AutoSaveFromFavorites bool   `yaml:"autoSaveFromFavorites"`
}

// Config holds runtime overrides from CLI flags
type Config struct {
	Log                    string
	UseMultcastAddress     string
	UseMultcastPort        int
	UseConfigPath          string
	UseDefaultUploadFolder string
	UseLegacyMode          bool
	UseReferNetworkInterface string // fixes when using virtual network interface. e.g. Clash TUN.
	UsePin                 string
	UseAutoSave            bool // if false, user require to confirm before recv.
	UseAutoSaveFromFavorites bool // if true and useAutoSave is false, auto-accept from favorite devices only.
	UseAlias               string
	UseMixedScan           bool   // if true, use mixed scan mode, both UDP and HTTP.
	SkipNotify             bool   // if true, skip notify mode.
	UseHttp                bool   // if true, use http protocol; if false, use https protocol. Alias for protocol config.
	ScanTimeout            int    // scan timeout in seconds, default 500. After timeout, auto scan will stop.
	UseDownload            bool   // if true, enable download API (prepare-download, download, download page)
	UseWebOutPath          string // path to Next.js static export output (default: web/out)
	DoNotMakeSessionFolder bool   // if true, do not make any session folder, if meet same files
}
