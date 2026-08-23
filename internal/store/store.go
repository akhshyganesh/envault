// Package store is the content-addressed backup store: file bytes live in
// blobs/ under the SHA-256 of their content, and index/ records which snapshot
// of which file points at which blob.
//
// Content addressing gives deduplication for free — the same .env copied into
// ten projects occupies one blob — and makes "did this file change?" a hash
// comparison rather than a diff.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/akhshyganesh/envault/internal/config"
)

// Store reads and writes the vault. It holds no state beyond its paths, so it
// is cheap to construct and safe to share.
type Store struct {
	blobDir    string
	indexDir   string
	archiveDir string
}

// NewStore opens the vault, creating its directories if this is a first run.
func NewStore() (*Store, error) {
	s := &Store{blobDir: config.BlobsDir(), indexDir: config.IndexDir(), archiveDir: config.ArchivesDir()}
	for _, dir := range []string{s.blobDir, s.indexDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	return s, nil
}

func (s *Store) blobPath(id string) string {
	return filepath.Join(s.blobDir, id)
}

// SaveSnapshot records the current contents of an env file. The bool reports
// whether this created a new version; re-saving unchanged content is a no-op
// that returns the existing snapshot, which is what makes a 60-second polling
// daemon cheap.
func (s *Store) SaveSnapshot(envFilePath, comment string) (*Snapshot, bool, error) {
	absPath, err := filepath.Abs(envFilePath)
	if err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, false, err
	}

	sum := sha256.Sum256(data)
	id := hex.EncodeToString(sum[:])

	history, err := s.History(absPath)
	if err != nil {
		return nil, false, err
	}
	if latest := history.Latest(); latest != nil && latest.ID == id {
		return latest, false, nil
	}

	// A blob's name is its content hash, so an existing file with this name
	// already holds exactly these bytes and does not need rewriting.
	if _, err := os.Stat(s.blobPath(id)); os.IsNotExist(err) {
		if err := os.WriteFile(s.blobPath(id), data, 0600); err != nil {
			return nil, false, fmt.Errorf("storing content: %w", err)
		}
	}

	snap := Snapshot{
		ID:        id,
		Timestamp: time.Now().UTC(),
		FilePath:  absPath,
		Size:      int64(len(data)),
		Comment:   comment,
	}
	history.Snapshots = append(history.Snapshots, snap)
	prune(history)
	if err := s.writeHistory(history); err != nil {
		return nil, false, fmt.Errorf("recording snapshot: %w", err)
	}
	return &snap, true, nil
}

// Content returns the stored bytes of a snapshot.
func (s *Store) Content(snapshotID string) ([]byte, error) {
	data, err := os.ReadFile(s.blobPath(snapshotID))
	if err != nil {
		return nil, fmt.Errorf("snapshot content not found: %w", err)
	}
	return data, nil
}

// Restore writes a snapshot's content to a path, creating parent directories.
// The result is a secret on disk, so it is written 0600 like everything else
// envault produces.
func (s *Store) Restore(absPath, snapshotID string) error {
	data, err := s.Content(snapshotID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0700); err != nil {
		return err
	}
	return os.WriteFile(absPath, data, 0600)
}
