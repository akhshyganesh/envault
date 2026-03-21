package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/akhshy/envault/internal/config"
)

// Snapshot represents one versioned backup of an env file.
type Snapshot struct {
	ID        string    `json:"id"`         // SHA-256 of content
	Timestamp time.Time `json:"timestamp"`  // When this snapshot was taken
	FilePath  string    `json:"file_path"`  // Original absolute path of the env file
	Size      int64     `json:"size"`       // File size in bytes
	Comment   string    `json:"comment"`    // Optional comment (e.g., "auto" or user-provided)
}

// FileHistory holds all snapshots for a single env file.
type FileHistory struct {
	FilePath  string     `json:"file_path"`
	Snapshots []Snapshot `json:"snapshots"`
}

// Store is the versioned backup store for env files.
type Store struct {
	baseDir  string // ~/.envault
	blobDir  string // ~/.envault/blobs   (content-addressed storage)
	indexDir string // ~/.envault/index    (per-file history)
}

// NewStore creates or opens the store.
func NewStore() (*Store, error) {
	base := config.VaultDir()
	s := &Store{
		baseDir:  base,
		blobDir:  filepath.Join(base, "blobs"),
		indexDir: filepath.Join(base, "index"),
	}
	for _, d := range []string{s.blobDir, s.indexDir} {
		if err := os.MkdirAll(d, 0700); err != nil {
			return nil, fmt.Errorf("failed to create store dir %s: %w", d, err)
		}
	}
	return s, nil
}

// hashContent returns the hex SHA-256 of data.
func hashContent(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// fileKey converts an absolute file path to a safe filename for the index.
func fileKey(absPath string) string {
	h := sha256.Sum256([]byte(absPath))
	return hex.EncodeToString(h[:16]) // 16 bytes = 32 hex chars, enough to avoid collisions
}

// SaveSnapshot reads an env file, stores its content, and records a snapshot.
// Returns the snapshot and whether the content was new (not a duplicate of the latest).
func (s *Store) SaveSnapshot(envFilePath string, comment string) (*Snapshot, bool, error) {
	absPath, err := filepath.Abs(envFilePath)
	if err != nil {
		return nil, false, err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, false, err
	}

	contentHash := hashContent(data)

	// Check if latest snapshot already has same content (skip duplicate)
	history, _ := s.GetHistory(absPath)
	if len(history.Snapshots) > 0 {
		latest := history.Snapshots[len(history.Snapshots)-1]
		if latest.ID == contentHash {
			return &latest, false, nil
		}
	}

	// Store blob
	blobPath := filepath.Join(s.blobDir, contentHash)
	if err := os.WriteFile(blobPath, data, 0600); err != nil {
		return nil, false, err
	}

	snap := Snapshot{
		ID:        contentHash,
		Timestamp: time.Now().UTC(),
		FilePath:  absPath,
		Size:      int64(len(data)),
		Comment:   comment,
	}

	history.Snapshots = append(history.Snapshots, snap)
	if err := s.saveHistory(history); err != nil {
		return nil, false, err
	}

	return &snap, true, nil
}

// GetHistory returns all snapshots for a given env file path.
func (s *Store) GetHistory(absPath string) (*FileHistory, error) {
	key := fileKey(absPath)
	indexPath := filepath.Join(s.indexDir, key+".json")

	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &FileHistory{FilePath: absPath}, nil
		}
		return nil, err
	}

	var h FileHistory
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

// saveHistory writes the history index for a file.
func (s *Store) saveHistory(h *FileHistory) error {
	key := fileKey(h.FilePath)
	indexPath := filepath.Join(s.indexDir, key+".json")
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, data, 0600)
}

// RestoreSnapshot restores a specific snapshot back to its original path.
func (s *Store) RestoreSnapshot(absPath string, snapshotID string) error {
	blobPath := filepath.Join(s.blobDir, snapshotID)
	data, err := os.ReadFile(blobPath)
	if err != nil {
		return fmt.Errorf("snapshot blob not found: %w", err)
	}

	// Ensure parent dir exists
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(absPath, data, 0600)
}

// GetBlobContent returns the raw content of a snapshot.
func (s *Store) GetBlobContent(snapshotID string) ([]byte, error) {
	blobPath := filepath.Join(s.blobDir, snapshotID)
	return os.ReadFile(blobPath)
}

// ListTrackedFiles returns all tracked file paths and their latest snapshot.
func (s *Store) ListTrackedFiles() ([]FileHistory, error) {
	entries, err := os.ReadDir(s.indexDir)
	if err != nil {
		return nil, err
	}

	var results []FileHistory
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.indexDir, entry.Name()))
		if err != nil {
			continue
		}
		var h FileHistory
		if err := json.Unmarshal(data, &h); err != nil {
			continue
		}
		results = append(results, h)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].FilePath < results[j].FilePath
	})
	return results, nil
}

// Diff returns a simple before/after of two snapshot contents (as strings).
func (s *Store) Diff(snapA, snapB string) (string, string, error) {
	a, err := s.GetBlobContent(snapA)
	if err != nil {
		return "", "", fmt.Errorf("reading snapshot A: %w", err)
	}
	b, err := s.GetBlobContent(snapB)
	if err != nil {
		return "", "", fmt.Errorf("reading snapshot B: %w", err)
	}
	return string(a), string(b), nil
}
