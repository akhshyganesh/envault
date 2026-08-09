// Package config owns the vault's on-disk layout and its settings file.
package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config is the contents of ~/.envault/config.json.
type Config struct {
	// WatchDirs are the roots the scanner walks looking for env files.
	WatchDirs []string `json:"watch_dirs"`
	// ScanIntervalSecs is how often the daemon rescans.
	ScanIntervalSecs int `json:"scan_interval_secs"`
	// MaxVersions caps snapshots kept per file; 0 keeps every version.
	MaxVersions int `json:"max_versions"`
}

// DefaultConfig watches the whole home directory once a minute and keeps
// unlimited history — safe defaults for someone who has not chosen yet.
func DefaultConfig() *Config {
	return &Config{
		WatchDirs:        []string{home()},
		ScanIntervalSecs: 60,
		MaxVersions:      0,
	}
}

// IsConfigured reports whether setup has been completed. It keys on the
// settings file, not the vault directory: a wizard quit half way, or a bare
// 'envault start', leaves the directory behind with no config, and keying on
// the directory would mean the wizard is never offered again.
func IsConfigured() bool {
	_, err := os.Stat(ConfigPath())
	return err == nil
}

// Load reads the settings file, falling back to defaults when it is absent.
// Unset fields keep their default, so a partial config.json stays valid.
func Load() (*Config, error) {
	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", ConfigPath(), err)
	}
	return cfg, nil
}

// Save writes the settings file, creating the vault directory if needed.
func (c *Config) Save() error {
	if err := os.MkdirAll(VaultDir(), 0700); err != nil {
		return fmt.Errorf("creating vault directory: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(ConfigPath(), data, 0600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

// AddWatchDir appends a directory to the watch list after checking it exists.
// Adding one that is already watched is reported as an error but leaves the
// config untouched.
func AddWatchDir(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("directory not found: %s", path)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", path)
	}

	cfg, err := Load()
	if err != nil {
		return "", err
	}
	for _, d := range cfg.WatchDirs {
		if d == path {
			return path, fmt.Errorf("already watching %s", path)
		}
	}

	cfg.WatchDirs = append(cfg.WatchDirs, path)
	if err := cfg.Save(); err != nil {
		return "", err
	}
	return path, nil
}
