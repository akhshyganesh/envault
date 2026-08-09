package transfer

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/store"
)

// seedVault points HOME at a temp directory and records one env file.
func seedVault(t *testing.T, body string) *store.Store {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	s, err := store.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := config.DefaultConfig().Save(); err != nil {
		t.Fatalf("save config: %v", err)
	}

	env := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(env, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SaveSnapshot(env, "auto"); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	return s
}

func TestExportImportRoundTrip(t *testing.T) {
	seedVault(t, "SECRET=1")

	out := filepath.Join(t.TempDir(), "backup.zip")
	res, err := Export(out)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if res.Files == 0 || res.Size == 0 {
		t.Fatalf("empty export: %+v", res)
	}

	// A fresh machine: new HOME, no vault.
	t.Setenv("HOME", t.TempDir())
	count, err := Import(res.Path, false)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if count != res.Files {
		t.Fatalf("imported %d files, exported %d", count, res.Files)
	}

	s, err := store.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	files, err := s.ListTrackedFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d tracked files after import, want 1", len(files))
	}
	data, err := s.Content(files[0].Latest().ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "SECRET=1" {
		t.Fatalf("content survived as %q, want SECRET=1", data)
	}
}

// Carrying a stale PID or log into another machine's vault would make it think
// a daemon is running there.
func TestExportOmitsRuntimeFiles(t *testing.T) {
	seedVault(t, "A=1")
	for _, name := range []string{"envault.pid", "envault.log"} {
		if err := os.WriteFile(filepath.Join(config.VaultDir(), name), []byte("999"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	res, err := Export(filepath.Join(t.TempDir(), "backup.zip"))
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	r, err := zip.OpenReader(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, f := range r.File {
		if runtimeFiles[f.Name] {
			t.Errorf("export carried a runtime file: %s", f.Name)
		}
	}
}

func TestImportRefusesToClobberAVaultWithoutForce(t *testing.T) {
	seedVault(t, "A=1")
	res, err := Export(filepath.Join(t.TempDir(), "backup.zip"))
	if err != nil {
		t.Fatal(err)
	}

	// The vault from seedVault is still there.
	if _, err := Import(res.Path, false); err == nil {
		t.Fatal("want an error when a vault already exists")
	}
	if _, err := Import(res.Path, true); err != nil {
		t.Fatalf("Import with force: %v", err)
	}
}

func TestImportReportsAMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := Import(filepath.Join(t.TempDir(), "nope.zip"), false); err == nil {
		t.Fatal("want an error for a missing archive")
	}
}

func TestExportWithoutAVaultFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := Export(filepath.Join(t.TempDir(), "backup.zip")); err == nil {
		t.Fatal("want an error when there is no vault to export")
	}
}

// A zip entry that climbs out of the vault must be refused, not extracted.
func TestSafeExtractPathBlocksZipSlip(t *testing.T) {
	base := t.TempDir()

	for _, entry := range []string{"../escaped", "../../etc/passwd", "blobs/../../escaped"} {
		if _, err := safeExtractPath(base, entry); err == nil {
			t.Errorf("safeExtractPath(%q) allowed an escape", entry)
		}
	}
	for _, entry := range []string{"config.json", "blobs/abc123", "index/deadbeef.json"} {
		got, err := safeExtractPath(base, entry)
		if err != nil {
			t.Errorf("safeExtractPath(%q) rejected a legitimate entry: %v", entry, err)
		}
		if !strings.HasPrefix(got, base) {
			t.Errorf("safeExtractPath(%q) = %q, outside %q", entry, got, base)
		}
	}
}

func TestDefaultExportNameLooksLikeABackup(t *testing.T) {
	name := DefaultExportName()
	if !strings.HasPrefix(name, "envault-backup-") || !strings.HasSuffix(name, ".zip") {
		t.Fatalf("DefaultExportName() = %q", name)
	}
}
