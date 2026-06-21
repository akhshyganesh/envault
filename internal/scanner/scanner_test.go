package scanner

import "testing"

func TestIsEnvFile(t *testing.T) {
	cases := map[string]bool{
		".env":            true,
		".env.local":      true,
		".env.production": true,
		"production.env":  true,
		"app.env":         true,
		"env":             false,
		"config.yaml":     false,
		".environment":    false,
		"notes.txt":       false,
		".env.sample.bak": true, // .env.* prefix
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
