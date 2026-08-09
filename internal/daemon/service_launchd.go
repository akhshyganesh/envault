package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const launchdLabel = "com.envault.daemon"

// KeepAlive is false: envault should start at login and stay out of the way,
// not be resurrected by launchd every time the user stops it deliberately.
const launchdPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>` + launchdLabel + `</string>
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

func launchdPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
}

func installLaunchd(ctx serviceContext) (string, error) {
	path := launchdPlistPath()
	if err := writeServiceFile(path, launchdPlist, ctx); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	// Unload first so reinstalling over an existing agent is not an error.
	_ = exec.Command("launchctl", "unload", path).Run()
	if out, err := exec.Command("launchctl", "load", path).CombinedOutput(); err != nil {
		return "", fmt.Errorf("launchctl load: %s: %w", out, err)
	}
	return "Startup service installed — envault starts on login", nil
}

func uninstallLaunchd() (string, error) {
	path := launchdPlistPath()
	// Best-effort: the agent may never have been loaded.
	_ = exec.Command("launchctl", "unload", path).Run()
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return "No startup service was installed", nil
		}
		return "", err
	}
	return "Removed the launchd startup service", nil
}
