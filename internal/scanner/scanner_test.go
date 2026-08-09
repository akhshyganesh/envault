package scanner

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/akhshyganesh/envault/internal/store"
)

func TestIsEnvFile(t *testing.T) {
	cases := map[string]bool{
		".env":            true,
		".env.local":      true,
		".env.production": true,
		".env.sample.bak": true, // still has the .env. prefix
		"production.env":  true,
		"app.env":         true,
		"env":             false,
		"config.yaml":     false,
		".environment":    false,
		"notes.txt":       false,
	}
	for name, want := range cases {
		if got := isEnvFile(name); got != want {
			t.Errorf("isEnvFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestShouldSkipDir(t *testing.T) {
	for _, name := range []string{"node_modules", ".git", "vendor", ".envault"} {
		if !shouldSkipDir(name) {
			t.Errorf("shouldSkipDir(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"src", "internal", "myproject"} {
		if shouldSkipDir(name) {
			t.Errorf("shouldSkipDir(%q) = true, want false", name)
		}
	}
}

func TestScanDirectoryFindsAndSkips(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, err := store.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("app/.env", "A=1")
	write("app/.env.local", "B=1")
	write("api/production.env", "C=1")
	write("app/readme.md", "not an env file")
	write("node_modules/pkg/.env", "SHOULD=not be found")

	res, err := ScanDirectory(root, s)
	if err != nil {
		t.Fatalf("ScanDirectory: %v", err)
	}
	if len(res.Found) != 3 {
		t.Fatalf("found %d files, want 3: %v", len(res.Found), res.Found)
	}
	if res.Backed != 3 || res.Skipped != 0 {
		t.Fatalf("first scan: backed=%d skipped=%d, want 3/0", res.Backed, res.Skipped)
	}
	for _, p := range res.Found {
		if filepath.Base(filepath.Dir(p)) == "pkg" {
			t.Errorf("scanner descended into node_modules: %s", p)
		}
	}

	// Nothing changed, so a second pass must create no new versions.
	again, err := ScanDirectory(root, s)
	if err != nil {
		t.Fatalf("second ScanDirectory: %v", err)
	}
	if again.Backed != 0 || again.Skipped != 3 {
		t.Fatalf("second scan: backed=%d skipped=%d, want 0/3", again.Backed, again.Skipped)
	}
}

func TestScanDirectoriesMergesResults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, err := store.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	dirs := []string{t.TempDir(), t.TempDir()}
	for _, d := range dirs {
		if err := os.WriteFile(filepath.Join(d, ".env"), []byte("X="+d), 0600); err != nil {
			t.Fatal(err)
		}
	}

	res, err := ScanDirectories(dirs, s)
	if err != nil {
		t.Fatalf("ScanDirectories: %v", err)
	}
	if res.Backed != 2 {
		t.Fatalf("backed %d, want 2", res.Backed)
	}
}

// An unreadable directory must be recorded and stepped over, never fatal.
func TestScanDirectoriesSurvivesAMissingDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, err := store.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	good := t.TempDir()
	if err := os.WriteFile(filepath.Join(good, ".env"), []byte("A=1"), 0600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	res, err := ScanDirectories([]string{missing, good}, s)
	if err != nil {
		t.Fatalf("ScanDirectories returned a fatal error: %v", err)
	}
	if res.Backed != 1 {
		t.Fatalf("backed %d, want 1 — the good directory was abandoned", res.Backed)
	}
	if len(res.Errors) == 0 {
		t.Fatal("the missing directory was not reported")
	}
	if slices.Contains(res.Found, missing) {
		t.Fatal("a missing directory was reported as found")
	}
}
