package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akhshyganesh/envault/internal/config"
)

// newTestStore relocates the whole vault into a temp HOME.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

func writeEnv(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatalf("write env: %v", err)
	}
	return p
}

func setMaxVersions(t *testing.T, n int) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.MaxVersions = n
	if err := cfg.Save(); err != nil {
		t.Fatalf("save config: %v", err)
	}
}

func TestSaveSnapshotDeduplicates(t *testing.T) {
	s := newTestStore(t)
	env := writeEnv(t, "KEY=value")

	if _, isNew, err := s.SaveSnapshot(env, "auto"); err != nil || !isNew {
		t.Fatalf("first save: isNew=%v err=%v, want true/nil", isNew, err)
	}
	if _, isNew, err := s.SaveSnapshot(env, "auto"); err != nil || isNew {
		t.Fatalf("unchanged save: isNew=%v err=%v, want false/nil", isNew, err)
	}

	if err := os.WriteFile(env, []byte("KEY=changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, isNew, err := s.SaveSnapshot(env, "auto"); err != nil || !isNew {
		t.Fatalf("changed save: isNew=%v err=%v, want true/nil", isNew, err)
	}

	h, err := s.History(env)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Snapshots) != 2 {
		t.Fatalf("got %d versions, want 2", len(h.Snapshots))
	}
}

// The same content in two places must occupy one blob — that is the whole
// point of naming blobs by their hash.
func TestIdenticalContentSharesOneBlob(t *testing.T) {
	s := newTestStore(t)
	a := writeEnv(t, "SHARED=1")
	b := writeEnv(t, "SHARED=1")

	if _, _, err := s.SaveSnapshot(a, "auto"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SaveSnapshot(b, "auto"); err != nil {
		t.Fatal(err)
	}

	blobs, err := os.ReadDir(config.BlobsDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(blobs) != 1 {
		t.Fatalf("got %d blobs for identical content, want 1", len(blobs))
	}

	files, err := s.ListTrackedFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d tracked files, want 2", len(files))
	}
}

func TestHistoryOfUnknownFileIsEmptyNotAnError(t *testing.T) {
	s := newTestStore(t)
	h, err := s.History("/nowhere/.env")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(h.Snapshots) != 0 || h.Latest() != nil {
		t.Fatalf("want an empty history, got %+v", h)
	}
}

func TestPruneKeepsTheNewestVersions(t *testing.T) {
	s := newTestStore(t)
	setMaxVersions(t, 2)

	env := writeEnv(t, "v=0")
	for _, body := range []string{"v=1", "v=2", "v=3", "v=4", "v=5"} {
		if err := os.WriteFile(env, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.SaveSnapshot(env, "auto"); err != nil {
			t.Fatal(err)
		}
	}

	h, err := s.History(env)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Snapshots) != 2 {
		t.Fatalf("got %d versions, want 2 (max_versions)", len(h.Snapshots))
	}

	// Pruning keeps the tail, so the surviving latest must be the last write.
	data, err := s.Content(h.Latest().ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "v=5" {
		t.Fatalf("latest surviving version = %q, want v=5", data)
	}
}

func TestGCRemovesOnlyUnreferencedBlobs(t *testing.T) {
	s := newTestStore(t)
	setMaxVersions(t, 1) // every change orphans the version before it

	env := writeEnv(t, "a=1")
	for _, body := range []string{"a=1", "a=2", "a=3"} {
		if err := os.WriteFile(env, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.SaveSnapshot(env, "auto"); err != nil {
			t.Fatal(err)
		}
	}

	res, err := s.GC()
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 2 || res.Freed == 0 {
		t.Fatalf("GC reclaimed %+v, want 2 blobs and non-zero bytes", res)
	}

	// The referenced blob must survive, and still be readable.
	h, _ := s.History(env)
	if _, err := s.Content(h.Latest().ID); err != nil {
		t.Fatalf("GC deleted a referenced blob: %v", err)
	}
	if again, _ := s.GC(); again.Removed != 0 {
		t.Fatalf("second GC removed %d, want 0", again.Removed)
	}
}

func TestForgetDropsHistoryButKeepsContentForGC(t *testing.T) {
	s := newTestStore(t)
	env := writeEnv(t, "x=1")
	snap, _, err := s.SaveSnapshot(env, "auto")
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Forget(snap.FilePath); err != nil {
		t.Fatal(err)
	}
	h, _ := s.History(snap.FilePath)
	if len(h.Snapshots) != 0 {
		t.Fatalf("after Forget got %d versions, want 0", len(h.Snapshots))
	}
	if _, err := s.Content(snap.ID); err != nil {
		t.Fatalf("Forget deleted content that GC should have handled: %v", err)
	}
}

func TestForgetAnUntrackedFileIsNotAnError(t *testing.T) {
	s := newTestStore(t)
	if err := s.Forget("/nowhere/.env"); err != nil {
		t.Fatalf("Forget on an untracked file: %v", err)
	}
}

func TestRestoreWritesContentPrivately(t *testing.T) {
	s := newTestStore(t)
	env := writeEnv(t, "SECRET=1")
	snap, _, err := s.SaveSnapshot(env, "auto")
	if err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "nested", "restored.env")
	if err := s.Restore(out, snap.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "SECRET=1" {
		t.Fatalf("restored content = %q", data)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("restored file mode = %o, want 600 — it holds a secret", perm)
	}
}

func TestListTrackedFilesIsSortedByPath(t *testing.T) {
	s := newTestStore(t)
	dir := t.TempDir()
	for _, name := range []string{"c.env", "a.env", "b.env"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("K="+name), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.SaveSnapshot(p, "auto"); err != nil {
			t.Fatal(err)
		}
	}

	files, err := s.ListTrackedFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("got %d files, want 3", len(files))
	}
	for i := 1; i < len(files); i++ {
		if files[i-1].FilePath > files[i].FilePath {
			t.Fatalf("listing is not sorted: %s before %s", files[i-1].FilePath, files[i].FilePath)
		}
	}
}
