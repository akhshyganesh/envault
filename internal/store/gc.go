package store

import (
	"fmt"
	"os"
	"path/filepath"
)

// GCResult reports what a collection pass reclaimed.
type GCResult struct {
	Removed int   // blobs deleted
	Freed   int64 // bytes reclaimed
}

// GC deletes blobs no snapshot references any more. Orphans appear when
// version pruning drops old snapshots or a file is forgotten; neither deletes
// blobs itself, because content addressing means another file may still point
// at the same bytes.
func (s *Store) GC() (*GCResult, error) {
	files, err := s.ListTrackedFiles()
	if err != nil {
		return nil, err
	}
	archived, err := s.ListArchived()
	if err != nil {
		return nil, err
	}
	referenced := make(map[string]bool)
	for _, f := range append(files, archived...) {
		for _, snap := range f.Snapshots {
			referenced[snap.ID] = true
		}
	}

	entries, err := os.ReadDir(s.blobDir)
	if err != nil {
		return nil, fmt.Errorf("reading blob directory: %w", err)
	}

	res := &GCResult{}
	for _, entry := range entries {
		if entry.IsDir() || referenced[entry.Name()] {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if err := os.Remove(filepath.Join(s.blobDir, entry.Name())); err == nil {
			res.Removed++
			res.Freed += info.Size()
		}
	}
	return res, nil
}

// Forget drops a file's recorded history. The file on disk is untouched, and
// its blobs survive until the next GC.
func (s *Store) Forget(absPath string) error {
	if err := os.Remove(s.indexPath(absPath)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("forgetting %s: %w", absPath, err)
	}
	return nil
}
