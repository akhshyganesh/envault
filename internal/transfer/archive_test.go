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

// buildExportZip builds a zip laid out like an 'envault export', mapping each
// file path to its versions in order.
func buildExportZip(t *testing.T, contents map[string][]string) string {
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

func openTestArchive(t *testing.T, contents map[string][]string) *Archive {
	t.Helper()
	a, err := OpenArchive(buildExportZip(t, contents))
	if err != nil {
		t.Fatalf("OpenArchive: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func TestOpenArchiveListsFilesSorted(t *testing.T) {
	a := openTestArchive(t, map[string][]string{
		"/home/x/b/.env": {"B=1"},
		"/home/x/a/.env": {"A=1", "A=2"},
	})

	files := a.Files()
	if len(files) != 2 {
		t.Fatalf("want 2 files, got %d", len(files))
	}
	if files[0].FilePath != "/home/x/a/.env" || files[1].FilePath != "/home/x/b/.env" {
		t.Fatalf("files not sorted by path: %+v", files)
	}
	if len(files[0].Snapshots) != 2 {
		t.Fatalf("want 2 versions for a/.env, got %d", len(files[0].Snapshots))
	}
}

func TestArchiveResolveByIndexAndPath(t *testing.T) {
	a := openTestArchive(t, map[string][]string{"/home/x/a/.env": {"A=1"}})

	byIndex, err := a.Resolve("1")
	if err != nil {
		t.Fatalf("Resolve by index: %v", err)
	}
	byPath, err := a.Resolve("/home/x/a/.env")
	if err != nil {
		t.Fatalf("Resolve by path: %v", err)
	}
	if byIndex.FilePath != byPath.FilePath {
		t.Fatalf("index and path disagreed: %s vs %s", byIndex.FilePath, byPath.FilePath)
	}

	if _, err := a.Resolve("9"); err == nil {
		t.Error("want an error for an out-of-range index")
	}
	if _, err := a.Resolve("/nope/.env"); err == nil {
		t.Error("want an error for an unknown path")
	}
}

func TestArchiveResolveBySuffixOnlyWhenUnambiguous(t *testing.T) {
	a := openTestArchive(t, map[string][]string{
		"/home/x/a/.env":       {"A=1"},
		"/home/x/b/.env":       {"B=1"},
		"/home/x/b/.env.local": {"B=2"},
	})

	got, err := a.Resolve("b/.env.local")
	if err != nil {
		t.Fatalf("Resolve by suffix: %v", err)
	}
	if got.FilePath != "/home/x/b/.env.local" {
		t.Fatalf("got %s", got.FilePath)
	}

	// ".env" matches two files; guessing would be worse than asking.
	if _, err := a.Resolve(".env"); err == nil {
		t.Fatal("want an error for an ambiguous suffix")
	}
}

func TestArchiveContent(t *testing.T) {
	a := openTestArchive(t, map[string][]string{"/home/x/a/.env": {"A=1", "A=2"}})

	h, err := a.Resolve("1")
	if err != nil {
		t.Fatal(err)
	}
	data, err := a.Content(h.Snapshots[1].ID)
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	if string(data) != "A=2" {
		t.Fatalf("got %q, want A=2", data)
	}
	if _, err := a.Content("deadbeef"); err == nil {
		t.Fatal("want an error for a blob that is not in the archive")
	}
}

func TestArchiveConfig(t *testing.T) {
	a := openTestArchive(t, map[string][]string{"/home/x/a/.env": {"A=1"}})

	cfg, err := a.Config()
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if len(cfg.WatchDirs) != 1 || cfg.WatchDirs[0] != "/home/x" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestOpenArchiveRejectsAZipThatIsNotAnExport(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "other.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	e, _ := w.Create("readme.txt")
	_, _ = e.Write([]byte("hi"))
	_ = w.Close()
	_ = f.Close()

	if _, err := OpenArchive(zipPath); err == nil {
		t.Fatal("want an error for a zip that is not an envault export")
	}
}

func TestOpenArchiveReportsAMissingFile(t *testing.T) {
	if _, err := OpenArchive(filepath.Join(t.TempDir(), "nope.zip")); err == nil {
		t.Fatal("want an error for a missing file")
	}
}

// The whole point of peeking is that the local vault is never touched.
func TestReadingAnArchiveNeverCreatesAVault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	a := openTestArchive(t, map[string][]string{"/home/x/a/.env": {"A=1"}})
	if _, err := a.Content(a.Files()[0].Snapshots[0].ID); err != nil {
		t.Fatalf("Content: %v", err)
	}
	if _, err := a.Config(); err != nil {
		t.Fatalf("Config: %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, ".envault")); !os.IsNotExist(err) {
		t.Fatalf("reading an archive created a vault directory (err=%v)", err)
	}
}
