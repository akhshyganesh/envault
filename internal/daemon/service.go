package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	"github.com/akhshyganesh/envault/internal/config"
)

// serviceContext is what both the launchd and systemd templates render from.
type serviceContext struct {
	BinaryPath string
	LogPath    string
}

// Install registers envault to start with the user's session and returns a
// line describing what happened, for the caller to display.
func Install() (string, error) {
	bin, err := binaryPath()
	if err != nil {
		return "", err
	}
	ctx := serviceContext{BinaryPath: bin, LogPath: config.LogPath()}

	switch runtime.GOOS {
	case "darwin":
		return installLaunchd(ctx)
	case "linux":
		return installSystemd(ctx)
	default:
		return "", fmt.Errorf("no startup service for %s", runtime.GOOS)
	}
}

// ServiceInstalled reports whether the startup service is registered, and the
// file that would hold it. Callers use the path to tell the user where to look
// even when the answer is no.
func ServiceInstalled() (bool, string) {
	path := servicePath()
	if path == "" {
		return false, ""
	}
	_, err := os.Stat(path)
	return err == nil, path
}

func servicePath() string {
	switch runtime.GOOS {
	case "darwin":
		return launchdPlistPath()
	case "linux":
		return systemdUnitPath()
	default:
		return ""
	}
}

// Uninstall removes the startup service registration.
func Uninstall() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return uninstallLaunchd()
	case "linux":
		return uninstallSystemd()
	default:
		return "", fmt.Errorf("no startup service for %s", runtime.GOOS)
	}
}

// binaryPath prefers envault on PATH over the running executable, so a service
// installed from ./envault still points at the installed copy.
func binaryPath() (string, error) {
	if p, err := exec.LookPath("envault"); err == nil {
		return filepath.Abs(p)
	}
	return os.Executable()
}

// writeServiceFile renders a template to disk. Service definitions are 0644 in
// a 0755 directory — unlike everything else envault writes — because launchd
// and systemd have to read them.
func writeServiceFile(path, tmplText string, ctx serviceContext) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmpl, err := template.New("service").Parse(tmplText)
	if err != nil {
		return err
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(buf.String()), 0644)
}
