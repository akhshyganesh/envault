package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadWithoutAFileReturnsDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ScanIntervalSecs != 60 {
		t.Errorf("ScanIntervalSecs = %d, want 60", cfg.ScanIntervalSecs)
	}
	if cfg.MaxVersions != 0 {
		t.Errorf("MaxVersions = %d, want 0 (unlimited)", cfg.MaxVersions)
	}
	if len(cfg.WatchDirs) != 1 {
		t.Errorf("WatchDirs = %v, want just the home directory", cfg.WatchDirs)
	}
}

// A wizard quit half way, or a bare 'envault start', creates the vault
// directory without a config. That must still count as unconfigured, or the
// first-run wizard is never offered again.
func TestIsConfiguredIgnoresABareVaultDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if IsConfigured() {
		t.Error("IsConfigured on an empty home, want false")
	}
	if err := os.MkdirAll(BlobsDir(), 0700); err != nil {
		t.Fatal(err)
	}
	if IsConfigured() {
		t.Error("IsConfigured with a vault directory but no config, want false")
	}
	if err := DefaultConfig().Save(); err != nil {
		t.Fatal(err)
	}
	if !IsConfigured() {
		t.Error("IsConfigured after Save, want true")
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	want := &Config{WatchDirs: []string{"/a", "/b"}, ScanIntervalSecs: 300, MaxVersions: 5}
	if err := want.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ScanIntervalSecs != 300 || got.MaxVersions != 5 || strings.Join(got.WatchDirs, ",") != "/a,/b" {
		t.Fatalf("round trip lost data: %+v", got)
	}
}

// A config written by an older version will not have every field. Missing ones
// must fall back to the default rather than to Go's zero value, or an upgrade
// would silently set the scan interval to zero.
func TestLoadFillsMissingFieldsWithDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(VaultDir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ConfigPath(), []byte(`{"watch_dirs":["/only"]}`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ScanIntervalSecs != 60 {
		t.Fatalf("ScanIntervalSecs = %d, want the default 60", cfg.ScanIntervalSecs)
	}
	if len(cfg.WatchDirs) != 1 || cfg.WatchDirs[0] != "/only" {
		t.Fatalf("WatchDirs = %v, want the file's value", cfg.WatchDirs)
	}
}

func TestLoadReportsAnUnreadableConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(VaultDir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ConfigPath(), []byte("this is not json"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("want an error for a corrupt config, got nil")
	}
}

func TestSaveCreatesThePrivateVaultDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := DefaultConfig().Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(VaultDir())
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("vault directory mode = %o, want 700", perm)
	}
	fileInfo, err := os.Stat(ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0600 {
		t.Errorf("config file mode = %o, want 600", perm)
	}
}

func TestAddWatchDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()

	if _, err := AddWatchDir(dir); err != nil {
		t.Fatalf("AddWatchDir: %v", err)
	}
	cfg, _ := Load()
	if len(cfg.WatchDirs) == 0 || cfg.WatchDirs[len(cfg.WatchDirs)-1] != dir {
		t.Fatalf("directory not persisted: %v", cfg.WatchDirs)
	}

	// Adding it twice is an error, and must not duplicate the entry.
	before := len(cfg.WatchDirs)
	if _, err := AddWatchDir(dir); err == nil {
		t.Fatal("want an error when adding a directory twice")
	}
	after, _ := Load()
	if len(after.WatchDirs) != before {
		t.Fatalf("watch list changed on a rejected add: %v", after.WatchDirs)
	}
}

func TestAddWatchDirRejectsNonDirectories(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	file := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := AddWatchDir(file); err == nil {
		t.Fatal("want an error for a file")
	}
	if _, err := AddWatchDir(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("want an error for a missing path")
	}
}

func TestIsInsideVault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	inside := []string{
		VaultDir() + "/sneaky.env",
		BlobsDir() + "/deadbeef",
		IndexDir() + "/abc.json",
		VaultDir() + "/nested/deep/.env",
		VaultDir() + "/../.envault/config.json", // resolves back inside
		VaultDir(),                              // writing a file here clobbers the vault directory
	}
	for _, p := range inside {
		if !IsInsideVault(p) {
			t.Errorf("IsInsideVault(%q) = false, want true", p)
		}
	}

	outside := []string{
		"/tmp/out.env",
		filepath.Dir(VaultDir()) + "/.env",
		VaultDir() + "-not-really/x", // shares a prefix, different directory
	}
	for _, p := range outside {
		if IsInsideVault(p) {
			t.Errorf("IsInsideVault(%q) = true, want false", p)
		}
	}
}

// Everything must hang off VaultDir, so a test HOME relocates the whole vault.
func TestEveryPathLivesUnderTheVaultDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	for name, p := range map[string]string{
		"ConfigPath": ConfigPath(),
		"BlobsDir":   BlobsDir(),
		"IndexDir":   IndexDir(),
		"PidPath":    PidPath(),
		"LogPath":    LogPath(),
	} {
		if !strings.HasPrefix(p, VaultDir()+string(os.PathSeparator)) {
			t.Errorf("%s = %q, want it under %q", name, p, VaultDir())
		}
	}
	if !strings.HasPrefix(VaultDir(), home) {
		t.Errorf("VaultDir = %q, want it under the test HOME %q", VaultDir(), home)
	}
}
