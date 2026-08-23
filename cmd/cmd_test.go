package cmd

import (
	"io"
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

// ── uninstall ────────────────────────────────────────────────────────────────

// Unlinking a running binary is legal on Unix, which is what makes --all able
// to remove the very process executing it.
func TestRemoveSelfDeletesTheBinary(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "envault")
	if err := os.WriteFile(exe, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := removeSelf(exe); err != nil {
		t.Fatalf("removeSelf: %v", err)
	}
	if _, err := os.Stat(exe); !os.IsNotExist(err) {
		t.Fatalf("the binary survived (err=%v)", err)
	}
}

// A system-wide install needs root; the message has to say so rather than
// leaking "permission denied".
func TestRemoveSelfExplainsAnUnwritableInstallDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: every directory is writable")
	}

	dir := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "envault")
	if err := os.WriteFile(exe, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })

	err := removeSelf(exe)
	if err == nil {
		t.Fatal("want an error for a read-only install directory")
	}
	for _, want := range []string{exe, "sudo"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message is missing %q:\n%s", want, err)
		}
	}
	if _, statErr := os.Stat(exe); statErr != nil {
		t.Error("the binary was removed despite the reported failure")
	}
}

// "uninstalled" must never overstate what happened.
func TestReportLeftoversNamesWhatSurvives(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	exe := filepath.Join(t.TempDir(), "envault")

	cases := []struct {
		name                       string
		dataRemoved, binaryRemoved bool
		wantContains               []string
		wantMissing                []string
	}{
		{
			name:        "everything gone",
			dataRemoved: true, binaryRemoved: true,
			wantContains: []string{"fully uninstalled"},
			wantMissing:  []string{"still installed", "Backups kept"},
		},
		{
			name:        "binary kept",
			dataRemoved: true, binaryRemoved: false,
			wantContains: []string{"still installed", "rm " + exe},
			wantMissing:  []string{"fully uninstalled"},
		},
		{
			name:        "nothing else touched",
			dataRemoved: false, binaryRemoved: false,
			wantContains: []string{"Backups kept", "still installed"},
			wantMissing:  []string{"fully uninstalled"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				reportLeftovers(c.dataRemoved, c.binaryRemoved, exe, nil)
			})
			for _, want := range c.wantContains {
				if !strings.Contains(out, want) {
					t.Errorf("output is missing %q:\n%s", want, out)
				}
			}
			for _, unwanted := range c.wantMissing {
				if strings.Contains(out, unwanted) {
					t.Errorf("output should not claim %q:\n%s", unwanted, out)
				}
			}
		})
	}
}

// captureStdout collects what fn prints.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = saved }()

	fn()
	_ = w.Close()

	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// ── archive ──────────────────────────────────────────────────────────────────

func TestListHasAnArchivedFlag(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"list"})
	if err != nil {
		t.Fatalf("find list: %v", err)
	}
	if f := cmd.Flags().Lookup("archived"); f == nil {
		t.Fatal("list has no --archived flag")
	}
}

func TestResolveArchivedArgAcceptsAnIndexAndAPath(t *testing.T) {
	s, paths := seedStore(t, "a.env", "b.env")
	if err := s.Archive(paths[1]); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	got, err := resolveArchivedArg("1", s)
	if err != nil {
		t.Fatalf("resolveArchivedArg(1): %v", err)
	}
	if got != paths[1] {
		t.Fatalf("index 1 resolved to %s, want %s", got, paths[1])
	}

	got, err = resolveArchivedArg(paths[1], s)
	if err != nil || got != paths[1] {
		t.Fatalf("resolveArchivedArg(path) = %s, %v", got, err)
	}
}

func TestResolveArchivedArgRejectsAnUnarchivedFile(t *testing.T) {
	s, paths := seedStore(t, "a.env")

	// Still live-tracked, so it is not in the archive listing.
	if _, err := resolveArchivedArg(paths[0], s); err == nil {
		t.Fatal("want an error for a file that is not archived")
	}
	if _, err := resolveArchivedArg("9", s); err == nil {
		t.Fatal("want an error for an out-of-range index")
	}
}

// ── command wiring ───────────────────────────────────────────────────────────

// Every documented command must actually be registered, and the flags people
// rely on must keep their names and shorthands.
func TestEveryCommandIsRegistered(t *testing.T) {
	want := []string{
		"init", "scan", "list", "history", "show", "restore", "forget", "gc",
		"archive", "unarchive",
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
		{"uninstall", "prune", ""},
		{"uninstall", "all", ""},
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
