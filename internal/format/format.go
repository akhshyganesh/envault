// Package format provides shared display formatting for CLI and TUI output.
package format

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// homeDir caches the user's home directory; it never changes during a run and
// ShortenPath is called for every row on every TUI render.
var homeDir = sync.OnceValue(func() string {
	h, _ := os.UserHomeDir()
	return h
})

// ExpandPath expands a leading ~/ to the user's home directory and resolves to absolute.
func ExpandPath(p string) (string, error) {
	if strings.HasPrefix(p, "~/") || p == "~" {
		home := homeDir()
		if home == "" {
			return "", fmt.Errorf("cannot resolve home directory")
		}
		p = filepath.Join(home, p[1:]) // p[1:] keeps the / or is empty for bare ~
	}
	return filepath.Abs(p)
}

// ShortenPath replaces the home directory prefix with ~.
func ShortenPath(p string) string {
	home := homeDir()
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

// TimeAgo formats a time as a human-readable relative string (e.g. "5m ago").
func TimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// HumanSize formats a byte count as a human-readable string (e.g. "1.4KB").
func HumanSize(b int64) string {
	switch {
	case b < 1024:
		return fmt.Sprintf("%dB", b)
	case b < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(b)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(b)/(1024*1024))
	}
}
