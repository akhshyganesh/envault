package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Archive parks a file's whole history outside the live index. The file stops
// appearing in lists, scans and the daemon, but its snapshots and blobs are
// untouched and GC keeps protecting them until it is brought back.
func (s *Store) Archive(absPath string) error {
	src := s.indexPath(absPath)
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s is not tracked, nothing to archive", absPath)
		}
		return fmt.Errorf("reading history of %s: %w", absPath, err)
	}
	if err := os.MkdirAll(s.archiveDir, 0700); err != nil {
		return fmt.Errorf("creating %s: %w", s.archiveDir, err)
	}
	if err := os.Rename(src, filepath.Join(s.archiveDir, indexKey(absPath)+".json")); err != nil {
		return fmt.Errorf("archiving %s: %w", absPath, err)
	}
	return nil
}

// Unarchive brings an archived file's history back into the live index. If a
// rescan started a fresh history meanwhile, the two merge so neither side
// loses versions; identical snapshots collapse by blob ID.
func (s *Store) Unarchive(absPath string) error {
	parked := filepath.Join(s.archiveDir, indexKey(absPath)+".json")
	data, err := os.ReadFile(parked)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s is not archived", absPath)
		}
		return fmt.Errorf("reading archive entry for %s: %w", absPath, err)
	}

	var parkedHistory FileHistory
	if err := json.Unmarshal(data, &parkedHistory); err != nil {
		return fmt.Errorf("parsing archive entry for %s: %w", absPath, err)
	}

	live, err := s.History(absPath)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(live.Snapshots))
	for _, snap := range live.Snapshots {
		seen[snap.ID] = true
	}
	for _, snap := range parkedHistory.Snapshots {
		if !seen[snap.ID] {
			live.Snapshots = append(live.Snapshots, snap)
			seen[snap.ID] = true
		}
	}
	sort.Slice(live.Snapshots, func(i, j int) bool {
		return live.Snapshots[i].Timestamp.Before(live.Snapshots[j].Timestamp)
	})
	if err := s.writeHistory(live); err != nil {
		return fmt.Errorf("restoring history of %s: %w", absPath, err)
	}
	if err := os.Remove(parked); err != nil {
		return fmt.Errorf("clearing archive entry for %s: %w", absPath, err)
	}
	return nil
}

// ListArchived returns every archived file's history, sorted by path like
// ListTrackedFiles so numbering stays stable between runs.
func (s *Store) ListArchived() ([]FileHistory, error) {
	entries, err := os.ReadDir(s.archiveDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading vault archives: %w", err)
	}

	var out []FileHistory
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.archiveDir, entry.Name()))
		if err != nil {
			continue
		}
		var h FileHistory
		if err := json.Unmarshal(data, &h); err != nil {
			continue
		}
		out = append(out, h)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].FilePath < out[j].FilePath })
	return out, nil
}
