package tool

import "flag"

// Config holds runtime overrides from CLI flags.
type Config struct {
	Log                string
	UseMultcastAddress string
	UseMultcastPort    int
	UseConfigPath      string
}

// SetFlags parses CLI flags and returns the override config.
func SetFlags() Config {
	var cfg Config
	flag.StringVar(&cfg.Log, "log", "", "log mode: dev|prod")
	flag.StringVar(&cfg.UseMultcastAddress, "useMultcastAddress", "", "override multicast address")
	flag.IntVar(&cfg.UseMultcastPort, "useMultcastPort", 0, "override multicast port")
	flag.StringVar(&cfg.UseConfigPath, "useConfigPath", "", "override config file path")
	flag.Parse()
	return cfg
}
