package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akhshyganesh/envault/internal/config"
)

// Archiving parks a file's whole history outside the live index: it stops
// appearing in lists and scans, yet every blob stays recoverable until the
// file is brought back or explicitly forgotten.
func TestArchiveMovesHistoryOutOfTheLiveIndex(t *testing.T) {
	s := newTestStore(t)
	env := writeEnv(t, "SECRET=1")
	snap, _, err := s.SaveSnapshot(env, "auto")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(config.ArchivesDir()); !os.IsNotExist(err) {
		t.Fatalf("archives directory exists before anything was archived: %v", err)
	}

	if err := s.Archive(snap.FilePath); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	files, err := s.ListTrackedFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("archived file still listed as tracked (%d entries)", len(files))
	}

	archived, err := s.ListArchived()
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 || archived[0].FilePath != snap.FilePath {
		t.Fatalf("ListArchived = %+v, want exactly %s", archived, snap.FilePath)
	}
	if len(archived[0].Snapshots) != 1 || archived[0].Snapshots[0].ID != snap.ID {
		t.Fatalf("archived history lost its snapshot: %+v", archived[0])
	}
}

func TestArchiveAnUntrackedFileIsAnError(t *testing.T) {
	s := newTestStore(t)
	if err := s.Archive("/nowhere/.env"); err == nil {
		t.Fatal("archiving an untracked file should fail")
	}
}

func TestArchivedContentSurvivesGC(t *testing.T) {
	s := newTestStore(t)
	env := writeEnv(t, "SECRET=1")
	snap, _, err := s.SaveSnapshot(env, "auto")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Archive(snap.FilePath); err != nil {
		t.Fatal(err)
	}

	res, err := s.GC()
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 0 {
		t.Fatalf("GC removed %d blobs behind an archived file, want 0", res.Removed)
	}
	if data, err := s.Content(snap.ID); err != nil || string(data) != "SECRET=1" {
		t.Fatalf("archived content unreadable after GC: %q %v", data, err)
	}
}

func TestUnarchiveBringsHistoryBack(t *testing.T) {
	s := newTestStore(t)
	env := writeEnv(t, "SECRET=1")
	snap, _, err := s.SaveSnapshot(env, "auto")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Archive(snap.FilePath); err != nil {
		t.Fatal(err)
	}

	if err := s.Unarchive(snap.FilePath); err != nil {
		t.Fatalf("Unarchive: %v", err)
	}

	files, err := s.ListTrackedFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || len(files[0].Snapshots) != 1 || files[0].Snapshots[0].ID != snap.ID {
		t.Fatalf("unarchived history = %+v, want the original snapshot back", files)
	}
	archived, _ := s.ListArchived()
	if len(archived) != 0 {
		t.Fatalf("archive entry survived unarchiving: %+v", archived)
	}
}

func TestUnarchiveAnUnarchivedFileIsAnError(t *testing.T) {
	s := newTestStore(t)
	if err := s.Unarchive("/nowhere/.env"); err == nil {
		t.Fatal("unarchiving a file that was never archived should fail")
	}
}

// After archiving, a rescan may have started a fresh live history for the same
// path. Bringing the old one back must not discard either side.
func TestUnarchiveMergesWithNewLiveHistory(t *testing.T) {
	s := newTestStore(t)
	env := writeEnv(t, "a=1")
	old, _, err := s.SaveSnapshot(env, "auto")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Archive(env); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env, []byte("a=2"), 0600); err != nil {
		t.Fatal(err)
	}
	fresh, _, err := s.SaveSnapshot(env, "auto")
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Unarchive(env); err != nil {
		t.Fatalf("Unarchive: %v", err)
	}

	h, err := s.History(env)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Snapshots) != 2 {
		t.Fatalf("merged history has %d versions, want 2", len(h.Snapshots))
	}
	ids := map[string]bool{old.ID: false, fresh.ID: false}
	for _, snap := range h.Snapshots {
		if seen, known := ids[snap.ID]; !known || seen {
			t.Fatalf("unexpected or duplicated snapshot in merged history: %+v", snap)
		}
		ids[snap.ID] = true
	}
}

func TestListArchivedIsSortedByPath(t *testing.T) {
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
		if err := s.Archive(p); err != nil {
			t.Fatal(err)
		}
	}

	archived, err := s.ListArchived()
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 3 {
		t.Fatalf("got %d archived files, want 3", len(archived))
	}
	for i := 1; i < len(archived); i++ {
		if archived[i-1].FilePath > archived[i].FilePath {
			t.Fatalf("listing is not sorted: %s before %s", archived[i-1].FilePath, archived[i].FilePath)
		}
	}
}
