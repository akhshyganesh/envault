package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/store"
)

// ── upgrade preflight ────────────────────────────────────────────────────────

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
		t.Fatalf("the probe file was left behind: %v", names)
	}
}

func TestEnsureWritableInstallExplainsARootOwnedDir(t *testing.T) {
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
	// The message has to say where the problem is and what to do about it.
	for _, want := range []string{dir, "sudo envault upgrade"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error message is missing %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "permission denied") {
		t.Fatalf("the message leaks the raw syscall error instead of explaining:\n%s", msg)
	}
}

// ── argument resolution ──────────────────────────────────────────────────────

func seedStore(t *testing.T, names ...string) (*store.Store, []string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	s, err := store.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	dir := t.TempDir()
	paths := make([]string, 0, len(names))
	for _, name := range names {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("K="+name), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.SaveSnapshot(p, "auto"); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	return s, paths
}

func TestResolveFileArgAcceptsAnIndex(t *testing.T) {
	s, paths := seedStore(t, "a.env", "b.env")

	got, err := resolveFileArg("1", s)
	if err != nil {
		t.Fatalf("resolveFileArg: %v", err)
	}
	if got != paths[0] {
		t.Fatalf("index 1 resolved to %s, want %s", got, paths[0])
	}
}

func TestResolveFileArgAcceptsAPath(t *testing.T) {
	s, paths := seedStore(t, "a.env")

	got, err := resolveFileArg(paths[0], s)
	if err != nil {
		t.Fatalf("resolveFileArg: %v", err)
	}
	if got != paths[0] {
		t.Fatalf("got %s, want %s", got, paths[0])
	}
}

// An out-of-range number must say what the valid range is, not just fail.
func TestResolveFileArgRejectsAnOutOfRangeIndex(t *testing.T) {
	s, _ := seedStore(t, "a.env")

	_, err := resolveFileArg("9", s)
	if err == nil {
		t.Fatal("want an error for an out-of-range index")
	}
	if !strings.Contains(err.Error(), "envault list") {
		t.Errorf("the error does not tell the user how to find the right number: %v", err)
	}
}

func TestLoadHistoryFailsClearlyForAnUnbackedFile(t *testing.T) {
	s, _ := seedStore(t)

	_, err := loadHistory(filepath.Join(t.TempDir(), ".env"), s)
	if err == nil {
		t.Fatal("want an error for a file with no backups")
	}
	if !strings.Contains(err.Error(), "no backups") {
		t.Errorf("unexpected message: %v", err)
	}
}

// ── version selection ────────────────────────────────────────────────────────

func TestPickSnapshot(t *testing.T) {
	snaps := []store.Snapshot{
		{ID: "aaa", Timestamp: time.Now().Add(-time.Hour)},
		{ID: "bbb", Timestamp: time.Now()},
	}

	if got, err := pickSnapshot(snaps, 0); err != nil || got.ID != "bbb" {
		t.Errorf("version 0 gave %v/%v, want the newest", got, err)
	}
	if got, err := pickSnapshot(snaps, 1); err != nil || got.ID != "aaa" {
		t.Errorf("version 1 gave %v/%v, want the oldest", got, err)
	}
	if _, err := pickSnapshot(snaps, 3); err == nil {
		t.Error("want an error for a version that does not exist")
	}
}

func TestShortID(t *testing.T) {
	if got := shortID(strings.Repeat("a", 64)); len(got) != 12 {
		t.Errorf("shortID length = %d, want 12", len(got))
	}
	if got := shortID("abc"); got != "abc" {
		t.Errorf("shortID(abc) = %q, want it unchanged", got)
	}
}

func TestPlural(t *testing.T) {
	if got := plural(1, "file", "files"); got != "file" {
		t.Errorf("plural(1) = %q", got)
	}
	if got := plural(2, "file", "files"); got != "files" {
		t.Errorf("plural(2) = %q", got)
	}
}

// ── peek stays read-only ─────────────────────────────────────────────────────

// peek prints "Your vault was not modified", so it had better be true — even
// when the user points --out straight at the vault.
func TestPeekRefusesToWriteIntoTheVault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	target := filepath.Join(config.VaultDir(), "blobs", "sneaky")
	err := extractFromArchive(nil, &store.Snapshot{ID: "x"}, target)
	if err == nil {
		t.Fatal("peek wrote into the vault")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("unhelpful error: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("a file appeared inside the vault (err=%v)", err)
	}
}

// ── command wiring ───────────────────────────────────────────────────────────

// Every documented command must actually be registered, and the flags people
// rely on must keep their names and shorthands.
func TestEveryCommandIsRegistered(t *testing.T) {
	want := []string{
		"init", "scan", "list", "history", "show", "restore", "forget", "gc",
		"watch", "start", "stop", "status", "install", "uninstall",
		"export", "import", "peek", "upgrade", "version", "ui", "web",
	}
	registered := map[string]bool{}
	for _, c := range rootCmd.Commands() {
		registered[c.Name()] = true
	}
	for _, name := range want {
		if !registered[name] {
			t.Errorf("command %q is not registered", name)
		}
	}
}

func TestListHasItsAlias(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		if c.Name() == "list" {
			if len(c.Aliases) == 0 || c.Aliases[0] != "ls" {
				t.Fatalf("list lost its 'ls' alias: %v", c.Aliases)
			}
			return
		}
	}
	t.Fatal("list command not found")
}

func TestFlagNamesAndShorthands(t *testing.T) {
	cases := []struct{ command, flag, shorthand string }{
		{"show", "version", "v"},
		{"restore", "version", "v"},
		{"restore", "output", "o"},
		{"export", "output", "o"},
		{"peek", "version", "v"},
		{"peek", "out", "o"}, // deliberately --out, not --output
		{"web", "port", "p"},
	}
	for _, c := range cases {
		cmd, _, err := rootCmd.Find([]string{c.command})
		if err != nil {
			t.Errorf("find %s: %v", c.command, err)
			continue
		}
		f := cmd.Flags().Lookup(c.flag)
		if f == nil {
			t.Errorf("%s has no --%s flag", c.command, c.flag)
			continue
		}
		if f.Shorthand != c.shorthand {
			t.Errorf("%s --%s shorthand = %q, want %q", c.command, c.flag, f.Shorthand, c.shorthand)
		}
	}

	// peek must not grow an --output alias; that is the documented exception.
	peek, _, _ := rootCmd.Find([]string{"peek"})
	if peek.Flags().Lookup("output") != nil {
		t.Error("peek gained an --output flag; the documented flag is --out")
	}
}
