package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const systemdUnitName = "envault.service"

const systemdUnit = `[Unit]
Description=envault — versioned backups of your .env files
After=network.target

[Service]
Type=simple
ExecStart={{.BinaryPath}} start
Restart=on-failure
RestartSec=10

[Install]
WantedBy=default.target
`

func systemdUnitPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", systemdUnitName)
}

func installSystemd(ctx serviceContext) (string, error) {
	path := systemdUnitPath()
	if err := writeServiceFile(path, systemdUnit, ctx); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	// systemd will not see a new unit file until it is told to look.
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	if out, err := exec.Command("systemctl", "--user", "enable", systemdUnitName).CombinedOutput(); err != nil {
		return "", fmt.Errorf("systemctl enable: %s: %w", out, err)
	}
	return "Startup service installed — run 'systemctl --user start envault'", nil
}

func uninstallSystemd() (string, error) {
	// Best-effort: the unit may not be running or even enabled.
	_ = exec.Command("systemctl", "--user", "stop", systemdUnitName).Run()
	_ = exec.Command("systemctl", "--user", "disable", systemdUnitName).Run()

	err := os.Remove(systemdUnitPath())
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	if err != nil {
		if os.IsNotExist(err) {
			return "No startup service was installed", nil
		}
		return "", err
	}
	return "Removed the systemd startup service", nil
}
