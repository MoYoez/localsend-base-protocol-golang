package tool

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	ConfigPath           = "config.yaml" // be aware that it can be changed, default to ./config.yaml
	CurrentConfig        AppConfig
	ProgramCurrentConfig ProgramConfig
)

// FavoriteDeviceEntry represents a favorite device with only fingerprint and alias
type FavoriteDeviceEntry struct {
	Fingerprint string `yaml:"favorite_fingerprint" json:"favorite_fingerprint"`
	Alias       string `yaml:"favorite_alias" json:"favorite_alias"`
}

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

type ProgramConfig struct {
	Pin                   string `yaml:"pin"`
	AutoSave              bool   `yaml:"autoSave"`
	AutoSaveFromFavorites bool   `yaml:"autoSaveFromFavorites"`
}

func init() {
	ProgramCurrentConfig = DefaultProgramConfig()
}

func SetProgramConfigStatus(pin string, autoSave bool, autoSaveFromFavorites bool) {
	ProgramCurrentConfig.Pin = pin
	ProgramCurrentConfig.AutoSave = autoSave
	ProgramCurrentConfig.AutoSaveFromFavorites = autoSaveFromFavorites
}

func GetProgramConfigStatus() ProgramConfig {
	return ProgramCurrentConfig
}

// this save to memory , no file provided.
func DefaultProgramConfig() ProgramConfig {
	return ProgramConfig{
		Pin:                   "",
		AutoSave:              true,
		AutoSaveFromFavorites: false,
	}
}

func defaultConfig() AppConfig {
	return AppConfig{
		Alias:                 NameGenerator(), // so I change it, use official name generator. :Ciallo~
		Version:               "2.0",           // Protocol Version: maybe(
		DeviceModel:           "steamdeck",     // you can change it if you prefer.
		DeviceType:            "headless",      // maybe you can change it, I promise it will not burn others machine:(
		Fingerprint:           "",              // will be set based on protocol
		Port:                  53317,           // default , in normal cases you dont need to change it.
		Protocol:              "https",         // ENCRYPTION is very important, I dont mind you to switch to http if you are in your home or safe network.
		Download:              false,           // document said that  default is false, i dont know how to use it, so make it default.
		Announce:              true,
		AutoSaveFromFavorites: false,
		FavoriteDevices:       []FavoriteDeviceEntry{},
	}
}

// generateRandomFingerprint generates a random 32-character fingerprint
func generateRandomFingerprintForConfig() string {
	uuid := GenerateRandomUUID()
	return strings.ReplaceAll(uuid, "-", "")
}

func LoadConfig(path string) (AppConfig, error) {
	var configChanged bool
	if path == "" {
		path = ConfigPath
	}
	// Update DefaultConfigPath so it can be used for saving favorites later
	ConfigPath = path

	cfg := defaultConfig()

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Config file doesn't exist, create with default values
			// Default protocol is https, so generate fingerprint from TLS certificate
			cfg.Fingerprint = GetOrCreateFingerprintFromConfig(&cfg)
			if writeErr := writeDefaultConfig(path, cfg); writeErr != nil {
				return cfg, fmt.Errorf("config file not found, and failed to generate default config: %v", writeErr)
			}
			DefaultLogger.Infof("Created new config file with fingerprint and certificate")
			CurrentConfig = cfg
			ProgramCurrentConfig.AutoSaveFromFavorites = cfg.AutoSaveFromFavorites
			return cfg, nil
		}
		return cfg, fmt.Errorf("failed to read config file: %v", err)
	}
	if info.IsDir() {
		return cfg, fmt.Errorf("config file path is a directory: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("failed to read config file: %v", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse config file: %v", err)
	}

	// Handle fingerprint based on protocol
	if cfg.Protocol == "https" {
		// HTTPS mode: fingerprint should match TLS certificate
		oldFingerprint := cfg.Fingerprint
		tlsFingerprint := GetOrCreateFingerprintFromConfig(&cfg)
		if oldFingerprint != tlsFingerprint {
			DefaultLogger.Infof("Updating fingerprint to match TLS certificate: %s -> %s", oldFingerprint, tlsFingerprint)
			cfg.Fingerprint = tlsFingerprint
			configChanged = true
		}
	} else {
		// HTTP mode: use random fingerprint if not set, clear certificate data
		if cfg.Fingerprint == "" {
			cfg.Fingerprint = generateRandomFingerprintForConfig()
			DefaultLogger.Infof("HTTP mode: generated random fingerprint")
			configChanged = true
		}
		// Clear certificate data in HTTP mode
		if cfg.CertPEM != "" || cfg.KeyPEM != "" {
			cfg.CertPEM = ""
			cfg.KeyPEM = ""
			configChanged = true
		}
		DefaultLogger.Debugf("HTTP mode: no TLS certificate needed")
	}

	// Save config if changed
	if configChanged {
		if writeErr := writeDefaultConfig(path, cfg); writeErr != nil {
			DefaultLogger.Warnf("Failed to update config file: %v", writeErr)
		}
	}

	CurrentConfig = cfg
	ProgramCurrentConfig.AutoSaveFromFavorites = cfg.AutoSaveFromFavorites
	return cfg, nil
}

func writeDefaultConfig(path string, cfg AppConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func GetCurrentConfig() *AppConfig {
	return &CurrentConfig
}
