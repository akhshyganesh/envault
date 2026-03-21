package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds the global envault configuration.
type Config struct {
	// Directories to watch for .env files
	WatchDirs []string `json:"watch_dirs"`
	// How often (in seconds) to scan for changes when polling
	ScanIntervalSecs int `json:"scan_interval_secs"`
	// Max number of versions to keep per file (0 = unlimited)
	MaxVersions int `json:"max_versions"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		WatchDirs:        []string{home},
		ScanIntervalSecs: 60,
		MaxVersions:      0,
	}
}

// VaultDir returns the base directory for envault storage.
func VaultDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".envault")
}

// ConfigPath returns the path to the config file.
func ConfigPath() string {
	return filepath.Join(VaultDir(), "config.json")
}

// Load reads config from disk, or returns defaults if none exists.
func Load() (*Config, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save writes the config to disk.
func (c *Config) Save() error {
	dir := VaultDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigPath(), data, 0600)
}
