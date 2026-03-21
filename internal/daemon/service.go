package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

// launchd plist template for macOS
const launchdPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.envault.daemon</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.BinaryPath}}</string>
        <string>start</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <false/>
    <key>StandardOutPath</key>
    <string>{{.LogPath}}</string>
    <key>StandardErrorPath</key>
    <string>{{.LogPath}}</string>
</dict>
</plist>
`

// systemd unit template for Linux
const systemdUnit = `[Unit]
Description=envault - .env file backup daemon
After=network.target

[Service]
Type=simple
ExecStart={{.BinaryPath}} start
Restart=on-failure
RestartSec=10

[Install]
WantedBy=default.target
`

type serviceContext struct {
	BinaryPath string
	LogPath    string
}

// Install sets up envault to run on system startup.
func Install() error {
	binPath, err := findBinary()
	if err != nil {
		return err
	}

	ctx := serviceContext{
		BinaryPath: binPath,
		LogPath:    LogFilePath(),
	}

	switch runtime.GOOS {
	case "darwin":
		return installLaunchd(ctx)
	case "linux":
		return installSystemd(ctx)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// Uninstall removes the startup service.
func Uninstall() error {
	switch runtime.GOOS {
	case "darwin":
		return uninstallLaunchd()
	case "linux":
		return uninstallSystemd()
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func findBinary() (string, error) {
	// Try to find the envault binary
	path, err := exec.LookPath("envault")
	if err == nil {
		return filepath.Abs(path)
	}
	// Fallback to current executable
	return os.Executable()
}

// ── macOS (launchd) ───────────────────────────────────────────────────────────

func launchdPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.envault.daemon.plist")
}

func installLaunchd(ctx serviceContext) error {
	plistPath := launchdPlistPath()
	dir := filepath.Dir(plistPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpl, err := template.New("plist").Parse(launchdPlist)
	if err != nil {
		return err
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return err
	}

	if err := os.WriteFile(plistPath, []byte(buf.String()), 0644); err != nil {
		return err
	}

	// Load the agent
	cmd := exec.Command("launchctl", "load", plistPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load failed: %s: %w", string(out), err)
	}

	fmt.Printf("✓ Installed launchd service at %s\n", plistPath)
	fmt.Println("  envault will start automatically on login.")
	return nil
}

func uninstallLaunchd() error {
	plistPath := launchdPlistPath()

	cmd := exec.Command("launchctl", "unload", plistPath)
	cmd.CombinedOutput() // ignore errors if not loaded

	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	fmt.Println("✓ Removed launchd service")
	return nil
}

// ── Linux (systemd) ──────────────────────────────────────────────────────────

func systemdUnitPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", "envault.service")
}

func installSystemd(ctx serviceContext) error {
	unitPath := systemdUnitPath()
	dir := filepath.Dir(unitPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpl, err := template.New("unit").Parse(systemdUnit)
	if err != nil {
		return err
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return err
	}

	if err := os.WriteFile(unitPath, []byte(buf.String()), 0644); err != nil {
		return err
	}

	// Enable and start
	exec.Command("systemctl", "--user", "daemon-reload").Run()
	if out, err := exec.Command("systemctl", "--user", "enable", "envault.service").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl enable failed: %s: %w", string(out), err)
	}

	fmt.Printf("✓ Installed systemd user service at %s\n", unitPath)
	fmt.Println("  Run: systemctl --user start envault")
	return nil
}

func uninstallSystemd() error {
	exec.Command("systemctl", "--user", "stop", "envault.service").Run()
	exec.Command("systemctl", "--user", "disable", "envault.service").Run()

	unitPath := systemdUnitPath()
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	exec.Command("systemctl", "--user", "daemon-reload").Run()
	fmt.Println("✓ Removed systemd service")
	return nil
}
