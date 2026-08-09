package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akhshyganesh/envault/internal/config"
)

func TestIsRunningWithNoPidFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if running, pid := IsRunning(); running || pid != 0 {
		t.Fatalf("IsRunning() = %v/%d in a fresh vault, want false/0", running, pid)
	}
}

// A PID file left behind by a crash must be cleaned up, not believed — else
// the daemon can never be started again.
func TestIsRunningClearsAStalePidFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(config.VaultDir(), 0700); err != nil {
		t.Fatal(err)
	}
	// PID 0x7FFFFFFF is above every real pid_max, so it cannot be live.
	if err := os.WriteFile(config.PidPath(), []byte("2147483647"), 0600); err != nil {
		t.Fatal(err)
	}

	if running, _ := IsRunning(); running {
		t.Fatal("a dead PID was reported as running")
	}
	if _, err := os.Stat(config.PidPath()); !os.IsNotExist(err) {
		t.Fatalf("the stale PID file was not removed (err=%v)", err)
	}
}

func TestIsRunningIgnoresAGarbagePidFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(config.VaultDir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.PidPath(), []byte("not a pid"), 0600); err != nil {
		t.Fatal(err)
	}

	if running, _ := IsRunning(); running {
		t.Fatal("an unparseable PID file was reported as running")
	}
}

func TestStopWithoutADaemon(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := Stop(); err == nil {
		t.Fatal("want an error when no daemon is running")
	}
}

func TestWritePIDIsPrivate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := writePID(); err != nil {
		t.Fatalf("writePID: %v", err)
	}
	info, err := os.Stat(config.PidPath())
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("PID file mode = %o, want 600", perm)
	}

	// It has to name this process, or Stop would signal a stranger.
	data, err := os.ReadFile(config.PidPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != strings.TrimSpace(itoa(os.Getpid())) {
		t.Errorf("PID file says %q, want %d", data, os.Getpid())
	}
}

// The service definitions have to be readable by launchd and systemd, which is
// the one place envault relaxes its 0600/0700 rule.
func TestServiceFileIsReadableByTheServiceManager(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "envault.service")

	ctx := serviceContext{BinaryPath: "/usr/local/bin/envault", LogPath: "/tmp/envault.log"}
	if err := writeServiceFile(path, systemdUnit, ctx); err != nil {
		t.Fatalf("writeServiceFile: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0644 {
		t.Errorf("unit file mode = %o, want 644", perm)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "/usr/local/bin/envault start") {
		t.Errorf("unit does not start the binary:\n%s", data)
	}
}

func TestLaunchdPlistRendersTheBinaryAndLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "com.envault.daemon.plist")
	ctx := serviceContext{BinaryPath: "/opt/envault", LogPath: "/tmp/envault.log"}
	if err := writeServiceFile(path, launchdPlist, ctx); err != nil {
		t.Fatalf("writeServiceFile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/opt/envault", "/tmp/envault.log", launchdLabel, "<key>RunAtLoad</key>"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("plist is missing %q:\n%s", want, data)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
