package tool

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	DefaultConfigPath = "config.yaml"
)

type AppConfig struct {
	Alias       string `yaml:"alias"`
	Version     string `yaml:"version"`
	DeviceModel string `yaml:"deviceModel"`
	DeviceType  string `yaml:"deviceType"`
	Fingerprint string `yaml:"fingerprint"`
	Port        int    `yaml:"port"`
	Protocol    string `yaml:"protocol"`
	Download    bool   `yaml:"download"`
	Announce    bool   `yaml:"announce"`
}

func defaultConfig() AppConfig {
	return AppConfig{
		Alias:       "localsend-base-protocol-golang",
		Version:     "2.0",
		DeviceModel: "steamdeck",
		DeviceType:  "headless",
		Fingerprint: generateFingerprint(),
		Port:        53317,
		Protocol:    "https",
		Download:    false,
		Announce:    true,
	}
}

// generateFingerprint generate a 32 characters long random string
func generateFingerprint() string {
	uuid := GenerateRandomUUID()
	// remove hyphen, ensure 32 characters
	return strings.ReplaceAll(uuid, "-", "")
}

func LoadConfig(path string) (AppConfig, error) {
	cfg := defaultConfig()
	if path == "" {
		path = DefaultConfigPath
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// generate fingerprint
			cfg.Fingerprint = generateFingerprint()
			if writeErr := writeDefaultConfig(path, cfg); writeErr != nil {
				return cfg, fmt.Errorf("config file not found, and failed to generate default config: %v", writeErr)
			}
			// hello, world!
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

	return cfg, nil
}

func writeDefaultConfig(path string, cfg AppConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
