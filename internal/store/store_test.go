package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akhshyganesh/envault/internal/config"
)

// newTestStore points the vault at a temp HOME and returns a fresh store.
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

func TestSaveSnapshotDedup(t *testing.T) {
	s := newTestStore(t)
	env := writeEnv(t, "KEY=value")

	if _, isNew, err := s.SaveSnapshot(env, "auto"); err != nil || !isNew {
		t.Fatalf("first save: isNew=%v err=%v, want true/nil", isNew, err)
	}

	// Same content → no new snapshot.
	if _, isNew, err := s.SaveSnapshot(env, "auto"); err != nil || isNew {
		t.Fatalf("duplicate save: isNew=%v err=%v, want false/nil", isNew, err)
	}

	// Changed content → new snapshot.
	if err := os.WriteFile(env, []byte("KEY=changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, isNew, err := s.SaveSnapshot(env, "auto"); err != nil || !isNew {
		t.Fatalf("changed save: isNew=%v err=%v, want true/nil", isNew, err)
	}

	abs, _ := filepath.Abs(env)
	h, err := s.GetHistory(abs)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Snapshots) != 2 {
		t.Fatalf("got %d snapshots, want 2", len(h.Snapshots))
	}
}

func TestPruneRespectsMaxVersions(t *testing.T) {
	s := newTestStore(t)
	cfg := config.DefaultConfig()
	cfg.MaxVersions = 2
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	env := writeEnv(t, "v=0")
	for i := 1; i <= 5; i++ {
		if err := os.WriteFile(env, []byte("v="+string(rune('0'+i))), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.SaveSnapshot(env, "auto"); err != nil {
			t.Fatal(err)
		}
	}

	abs, _ := filepath.Abs(env)
	h, _ := s.GetHistory(abs)
	if len(h.Snapshots) != 2 {
		t.Fatalf("got %d snapshots, want 2 (max_versions)", len(h.Snapshots))
	}
}

func TestGCRemovesOrphans(t *testing.T) {
	s := newTestStore(t)
	cfg := config.DefaultConfig()
	cfg.MaxVersions = 1 // each change orphans the previous blob
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	env := writeEnv(t, "a=1")
	for _, c := range []string{"a=1", "a=2", "a=3"} {
		if err := os.WriteFile(env, []byte(c), 0600); err != nil {
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
	if res.Removed != 2 {
		t.Fatalf("GC removed %d blobs, want 2", res.Removed)
	}

	// The one referenced blob must survive a second GC.
	res2, _ := s.GC()
	if res2.Removed != 0 {
		t.Fatalf("second GC removed %d, want 0", res2.Removed)
	}
}

func TestForget(t *testing.T) {
	s := newTestStore(t)
	env := writeEnv(t, "x=1")
	if _, _, err := s.SaveSnapshot(env, "auto"); err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(env)
	if err := s.Forget(abs); err != nil {
		t.Fatal(err)
	}
	h, _ := s.GetHistory(abs)
	if len(h.Snapshots) != 0 {
		t.Fatalf("after Forget got %d snapshots, want 0", len(h.Snapshots))
	}
}
