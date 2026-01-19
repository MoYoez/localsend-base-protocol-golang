package tool

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const DefaultConfigPath = "config.yaml"

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
		Fingerprint: "1145141919180",
		Port:        53317,
		Protocol:    "https",
		Download:    false,
		Announce:    true,
	}
}

func LoadConfig(path string) (AppConfig, error) {
	cfg := defaultConfig()
	if path == "" {
		path = DefaultConfigPath
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			if writeErr := writeDefaultConfig(path, cfg); writeErr != nil {
				return cfg, fmt.Errorf("config file not found, and failed to generate default config: %v", writeErr)
			}
			return cfg, fmt.Errorf("config file not found, default config generated: %s", path)
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
