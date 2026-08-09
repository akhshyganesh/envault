// Package format holds the display helpers shared by the CLI, the TUI and the
// web API, so a path or a size reads the same wherever it is printed.
package format

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// homeDir is cached: it cannot change during a run, and ShortenPath is called
// once per row on every TUI render.
var homeDir = sync.OnceValue(func() string {
	h, _ := os.UserHomeDir()
	return h
})

// ExpandPath resolves a leading ~ to the user's home directory and returns an
// absolute path.
func ExpandPath(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home := homeDir()
		if home == "" {
			return "", fmt.Errorf("cannot resolve home directory")
		}
		// p[1:] keeps the separator, and is empty for a bare "~".
		p = filepath.Join(home, p[1:])
	}
	return filepath.Abs(p)
}

// ShortenPath is the inverse of ExpandPath for display: it abbreviates the
// home directory back to "~".
func ShortenPath(p string) string {
	home := homeDir()
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

// TimeAgo renders a timestamp as a coarse relative age, e.g. "5m ago".
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

// HumanSize renders a byte count as e.g. "1.4KB".
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
