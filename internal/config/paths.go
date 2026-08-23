package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Every path under the vault is derived from this one function, so a test can
// relocate the whole vault with t.Setenv("HOME", t.TempDir()).
func home() string {
	h, _ := os.UserHomeDir()
	return h
}

// VaultDir is the root of everything envault stores: ~/.envault.
func VaultDir() string { return filepath.Join(home(), ".envault") }

// ConfigPath is the settings file inside the vault.
func ConfigPath() string { return filepath.Join(VaultDir(), "config.json") }

// BlobsDir holds file contents, each named by the SHA-256 of its bytes.
func BlobsDir() string { return filepath.Join(VaultDir(), "blobs") }

// IndexDir holds one JSON file of snapshot history per tracked env file.
func IndexDir() string { return filepath.Join(VaultDir(), "index") }

// ArchivesDir holds the history files of archived env files, parked outside
// the live index until they are brought back.
func ArchivesDir() string { return filepath.Join(VaultDir(), "archives") }

// PidPath records the running daemon's process ID.
func PidPath() string { return filepath.Join(VaultDir(), "envault.pid") }

// LogPath is where the daemon writes its activity log.
func LogPath() string { return filepath.Join(VaultDir(), "envault.log") }

// IsInsideVault reports whether a path lands in the vault.
//
// Read-only operations use this to refuse writing there: dropping a loose file
// into blobs/ or index/ would leave content the index knows nothing about, or
// history pointing at content that does not exist. Importing is the deliberate,
// supported way to put something into the vault.
func IsInsideVault(path string) bool {
	sep := string(os.PathSeparator)
	return strings.HasPrefix(filepath.Clean(path)+sep, filepath.Clean(VaultDir())+sep)
}
