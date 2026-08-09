package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/store"
)

// fileEntry is one row of the file list, shared by the vault and archive
// listings so the page renders both with the same code.
type fileEntry struct {
	Index      int    `json:"index"`
	Path       string `json:"path"`
	Display    string `json:"display"`
	Versions   int    `json:"versions"`
	LastBackup string `json:"last_backup"`
	LastSize   int64  `json:"last_size"`
	// Exists reports whether the original file is still on disk. It is
	// meaningless for an archive listing, so it is omitted there.
	Exists bool `json:"exists,omitempty"`
}

type snapshotEntry struct {
	Version   int    `json:"version"`
	ID        string `json:"id"`
	ShortID   string `json:"short_id"`
	Timestamp string `json:"timestamp"`
	Ago       string `json:"ago"`
	Size      int64  `json:"size"`
	HumanSize string `json:"human_size"`
	Comment   string `json:"comment"`
}

func newFileEntry(index int, h store.FileHistory) fileEntry {
	e := fileEntry{
		Index:    index,
		Path:     h.FilePath,
		Display:  format.ShortenPath(h.FilePath),
		Versions: len(h.Snapshots),
	}
	if latest := h.Latest(); latest != nil {
		e.LastBackup = format.TimeAgo(latest.Timestamp)
		e.LastSize = latest.Size
	}
	return e
}

func snapshotEntries(snaps []store.Snapshot) []snapshotEntry {
	out := make([]snapshotEntry, 0, len(snaps))
	for i, s := range snaps {
		out = append(out, snapshotEntry{
			Version:   i + 1,
			ID:        s.ID,
			ShortID:   s.ID[:min(len(s.ID), 12)],
			Timestamp: s.Timestamp.Local().Format("2006-01-02 15:04"),
			Ago:       format.TimeAgo(s.Timestamp),
			Size:      s.Size,
			HumanSize: format.HumanSize(s.Size),
			Comment:   s.Comment,
		})
	}
	return out
}

// resolveFile maps the 1-based index the page shows onto a tracked file. The
// page only ever sends indices, never paths, so there is nothing else to parse.
func (s *Server) resolveFile(arg string) (*store.FileHistory, error) {
	files, err := s.store.ListTrackedFiles()
	if err != nil {
		return nil, err
	}
	idx, err := strconv.Atoi(strings.TrimSpace(arg))
	if err != nil {
		return nil, fmt.Errorf("file must be a number from the list")
	}
	if idx < 1 || idx > len(files) {
		return nil, fmt.Errorf("file %d not found (have %d)", idx, len(files))
	}
	return &files[idx-1], nil
}

// pickVersion converts a 1-based version to a slice index. An empty or zero
// version means the latest, which is what every "just show me the file" path
// wants.
func pickVersion(count int, version string) (int, error) {
	if count == 0 {
		return 0, fmt.Errorf("no versions stored for this file")
	}
	version = strings.TrimSpace(version)
	if version == "" || version == "0" {
		return count - 1, nil
	}
	v, err := strconv.Atoi(version)
	if err != nil {
		return 0, fmt.Errorf("version must be a number")
	}
	if v < 1 || v > count {
		return 0, fmt.Errorf("version %d not found (max: %d)", v, count)
	}
	return v - 1, nil
}

// readJSON decodes a request body, treating an empty one as "no fields set".
// UseNumber keeps the page free to send an index as 3 or "3".
func readJSON(r *http.Request, dst any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
