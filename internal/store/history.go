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

	"github.com/akhshyganesh/envault/internal/config"
)

// Snapshot is one versioned backup of an env file.
type Snapshot struct {
	ID        string    `json:"id"`        // SHA-256 of the content, and the blob's filename
	Timestamp time.Time `json:"timestamp"` // when the snapshot was taken, in UTC
	FilePath  string    `json:"file_path"` // absolute path the content came from
	Size      int64     `json:"size"`      // content length in bytes
	Comment   string    `json:"comment"`   // "auto" for daemon scans, or a user note
}

// FileHistory is every snapshot recorded for one env file, oldest first.
type FileHistory struct {
	FilePath  string     `json:"file_path"`
	Snapshots []Snapshot `json:"snapshots"`
}

// Latest returns the newest snapshot, or nil when nothing is recorded yet.
func (h *FileHistory) Latest() *Snapshot {
	if len(h.Snapshots) == 0 {
		return nil
	}
	return &h.Snapshots[len(h.Snapshots)-1]
}

// indexKey turns an absolute file path into a stable index filename. Hashing
// sidesteps every question about slashes and case in the original path.
func indexKey(absPath string) string {
	sum := sha256.Sum256([]byte(absPath))
	return hex.EncodeToString(sum[:16])
}

func (s *Store) indexPath(absPath string) string {
	return filepath.Join(s.indexDir, indexKey(absPath)+".json")
}

// History returns the recorded snapshots for a path. A file that has never
// been backed up gets an empty history rather than an error.
func (s *Store) History(absPath string) (*FileHistory, error) {
	data, err := os.ReadFile(s.indexPath(absPath))
	if err != nil {
		if os.IsNotExist(err) {
			return &FileHistory{FilePath: absPath}, nil
		}
		return nil, fmt.Errorf("reading history for %s: %w", absPath, err)
	}
	var h FileHistory
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("parsing history for %s: %w", absPath, err)
	}
	return &h, nil
}

func (s *Store) writeHistory(h *FileHistory) error {
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath(h.FilePath), data, 0600)
}

// prune trims a history to the newest MaxVersions snapshots. The blobs the
// dropped snapshots referenced are left for GC, since another file may share
// them through content addressing.
func prune(h *FileHistory) {
	cfg, err := config.Load()
	if err != nil || cfg.MaxVersions <= 0 {
		return
	}
	if extra := len(h.Snapshots) - cfg.MaxVersions; extra > 0 {
		h.Snapshots = h.Snapshots[extra:]
	}
}

// ListTrackedFiles returns every file with recorded history, sorted by path so
// the numbering shown by 'envault list' is stable between runs. Index entries
// that cannot be read are skipped: one corrupt file should not hide the rest.
func (s *Store) ListTrackedFiles() ([]FileHistory, error) {
	entries, err := os.ReadDir(s.indexDir)
	if err != nil {
		return nil, fmt.Errorf("reading vault index: %w", err)
	}

	var out []FileHistory
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
		out = append(out, h)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].FilePath < out[j].FilePath })
	return out, nil
}
