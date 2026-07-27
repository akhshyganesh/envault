package transfer

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/akhshyganesh/envault/internal/store"
)

// writeVaultZip builds a zip laid out like an 'envault export' archive and
// returns its path. Each entry of contents maps a file path to its versions.
func writeVaultZip(t *testing.T, contents map[string][]string) string {
	t.Helper()
	zipPath := filepath.Join(t.TempDir(), "backup.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	add := func(name string, data []byte) {
		t.Helper()
		e, err := w.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := e.Write(data); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}

	i := 0
	for path, versions := range contents {
		h := store.FileHistory{FilePath: path}
		for v, body := range versions {
			sum := sha256.Sum256([]byte(body))
			id := hex.EncodeToString(sum[:])
			h.Snapshots = append(h.Snapshots, store.Snapshot{
				ID:        id,
				Timestamp: time.Date(2026, 1, 1, 0, 0, v, 0, time.UTC),
				FilePath:  path,
				Size:      int64(len(body)),
				Comment:   "auto",
			})
			add("blobs/"+id, []byte(body))
		}
		data, err := json.Marshal(h)
		if err != nil {
			t.Fatalf("marshal history: %v", err)
		}
		add("index/"+hex.EncodeToString([]byte{byte(i)})+".json", data)
		i++
	}
	add("config.json", []byte(`{"watch_dirs":["/home/x"],"scan_interval_secs":60,"max_versions":0}`))

	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return zipPath
}

func TestOpenArchiveListsTrackedFilesSorted(t *testing.T) {
	zipPath := writeVaultZip(t, map[string][]string{
		"/home/x/b/.env": {"B=1"},
		"/home/x/a/.env": {"A=1", "A=2"},
	})

	a, err := OpenArchive(zipPath)
	if err != nil {
		t.Fatalf("OpenArchive: %v", err)
	}
	defer a.Close()

	files := a.Files()
	if len(files) != 2 {
		t.Fatalf("want 2 files, got %d", len(files))
	}
	if files[0].FilePath != "/home/x/a/.env" || files[1].FilePath != "/home/x/b/.env" {
		t.Fatalf("files not sorted by path: %+v", files)
	}
	if len(files[0].Snapshots) != 2 {
		t.Fatalf("want 2 snapshots for a/.env, got %d", len(files[0].Snapshots))
	}
}

func TestArchiveResolveByIndexAndPath(t *testing.T) {
	zipPath := writeVaultZip(t, map[string][]string{"/home/x/a/.env": {"A=1"}})
	a, err := OpenArchive(zipPath)
	if err != nil {
		t.Fatalf("OpenArchive: %v", err)
	}
	defer a.Close()

	byIdx, err := a.Resolve("1")
	if err != nil {
		t.Fatalf("Resolve by index: %v", err)
	}
	byPath, err := a.Resolve("/home/x/a/.env")
	if err != nil {
		t.Fatalf("Resolve by path: %v", err)
	}
	if byIdx.FilePath != byPath.FilePath {
		t.Fatalf("index and path resolved differently: %s vs %s", byIdx.FilePath, byPath.FilePath)
	}

	if _, err := a.Resolve("9"); err == nil {
		t.Fatal("want error for out-of-range index")
	}
	if _, err := a.Resolve("/nope/.env"); err == nil {
		t.Fatal("want error for unknown path")
	}
}

func TestArchiveResolveBySuffixWhenUnambiguous(t *testing.T) {
	zipPath := writeVaultZip(t, map[string][]string{
		"/home/x/a/.env":       {"A=1"},
		"/home/x/b/.env":       {"B=1"},
		"/home/x/b/.env.local": {"B=2"},
	})
	a, err := OpenArchive(zipPath)
	if err != nil {
		t.Fatalf("OpenArchive: %v", err)
	}
	defer a.Close()

	got, err := a.Resolve("b/.env.local")
	if err != nil {
		t.Fatalf("Resolve by suffix: %v", err)
	}
	if got.FilePath != "/home/x/b/.env.local" {
		t.Fatalf("got %s", got.FilePath)
	}

	if _, err := a.Resolve(".env"); err == nil {
		t.Fatal("want error for ambiguous suffix")
	}
}

func TestArchiveContentReturnsBlobBytes(t *testing.T) {
	zipPath := writeVaultZip(t, map[string][]string{"/home/x/a/.env": {"A=1", "A=2"}})
	a, err := OpenArchive(zipPath)
	if err != nil {
		t.Fatalf("OpenArchive: %v", err)
	}
	defer a.Close()

	h, err := a.Resolve("1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	data, err := a.Content(h.Snapshots[1].ID)
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	if string(data) != "A=2" {
		t.Fatalf("got %q, want %q", data, "A=2")
	}

	if _, err := a.Content("deadbeef"); err == nil {
		t.Fatal("want error for missing blob")
	}
}

func TestOpenArchiveRejectsNonVaultZip(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "other.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	w := zip.NewWriter(f)
	e, _ := w.Create("readme.txt")
	_, _ = e.Write([]byte("hi"))
	_ = w.Close()
	_ = f.Close()

	if _, err := OpenArchive(zipPath); err == nil {
		t.Fatal("want error for a zip that is not an envault export")
	}
}

func TestOpenArchiveMissingFile(t *testing.T) {
	if _, err := OpenArchive(filepath.Join(t.TempDir(), "nope.zip")); err == nil {
		t.Fatal("want error for missing file")
	}
}

// The whole point of peeking is that the local vault is never touched.
func TestOpenArchiveDoesNotTouchVault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	zipPath := writeVaultZip(t, map[string][]string{"/home/x/a/.env": {"A=1"}})
	a, err := OpenArchive(zipPath)
	if err != nil {
		t.Fatalf("OpenArchive: %v", err)
	}
	defer a.Close()
	if _, err := a.Content(a.Files()[0].Snapshots[0].ID); err != nil {
		t.Fatalf("Content: %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, ".envault")); !os.IsNotExist(err) {
		t.Fatalf("peeking created a vault directory (err=%v)", err)
	}
}

func TestArchiveConfigSummary(t *testing.T) {
	zipPath := writeVaultZip(t, map[string][]string{"/home/x/a/.env": {"A=1"}})
	a, err := OpenArchive(zipPath)
	if err != nil {
		t.Fatalf("OpenArchive: %v", err)
	}
	defer a.Close()

	cfg, err := a.Config()
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if len(cfg.WatchDirs) != 1 || cfg.WatchDirs[0] != "/home/x" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}
