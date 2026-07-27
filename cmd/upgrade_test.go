package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureWritableInstallAcceptsAWritableDir(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "envault")
	if err := os.WriteFile(exe, []byte("binary"), 0755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}

	if err := ensureWritableInstall(exe); err != nil {
		t.Fatalf("want nil for a writable install dir, got %v", err)
	}
}

func TestEnsureWritableInstallLeavesNothingBehind(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "envault")
	if err := os.WriteFile(exe, []byte("binary"), 0755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}

	if err := ensureWritableInstall(exe); err != nil {
		t.Fatalf("ensureWritableInstall: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "envault" {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Fatalf("probe file left behind: %v", names)
	}
}

func TestEnsureWritableInstallExplainsRootOwnedDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: every directory is writable")
	}

	dir := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(dir, 0555); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })

	err := ensureWritableInstall(filepath.Join(dir, "envault"))
	if err == nil {
		t.Fatal("want an error for a read-only install directory")
	}

	msg := err.Error()
	// The message has to tell the user where the problem is and what to do.
	for _, want := range []string{dir, "sudo envault upgrade"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error message missing %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "permission denied") {
		t.Fatalf("message leaks the raw syscall error instead of explaining:\n%s", msg)
	}
}
