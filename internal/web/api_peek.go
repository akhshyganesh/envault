package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/store"
	"github.com/akhshyganesh/envault/internal/transfer"
)

// Peeking reads an export zip in place. Nothing in this file writes to the
// vault, and handlePeekSave actively refuses to — importing is a separate,
// deliberate action.

type peekRequest struct {
	Path    string      `json:"path"`
	File    json.Number `json:"file"`
	Version json.Number `json:"version"`
	Out     string      `json:"out"`
}

// openPeek decodes the request and opens the archive, replying with the error
// itself if either step fails. The bool reports whether to carry on.
func (s *Server) openPeek(w http.ResponseWriter, r *http.Request) (*transfer.Archive, peekRequest, bool) {
	var req peekRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return nil, req, false
	}
	path, err := format.ExpandPath(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return nil, req, false
	}
	a, err := transfer.OpenArchive(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return nil, req, false
	}
	return a, req, true
}

func (s *Server) handlePeek(w http.ResponseWriter, r *http.Request) {
	a, _, ok := s.openPeek(w, r)
	if !ok {
		return
	}
	defer a.Close()

	files := a.Files()
	entries := make([]fileEntry, 0, len(files))
	for i, f := range files {
		entries = append(entries, newFileEntry(i+1, f))
	}
	writeJSON(w, map[string]any{"files": entries})
}

func (s *Server) handlePeekContent(w http.ResponseWriter, r *http.Request) {
	a, req, ok := s.openPeek(w, r)
	if !ok {
		return
	}
	defer a.Close()

	h, idx, ok := resolveInArchive(w, a, req)
	if !ok {
		return
	}
	data, err := a.Content(h.Snapshots[idx].ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{
		"path":      h.FilePath,
		"display":   format.ShortenPath(h.FilePath),
		"version":   idx + 1,
		"content":   string(data),
		"snapshots": snapshotEntries(h.Snapshots),
	})
}

// handlePeekSave writes one archived version to a path of the user's choosing.
func (s *Server) handlePeekSave(w http.ResponseWriter, r *http.Request) {
	a, req, ok := s.openPeek(w, r)
	if !ok {
		return
	}
	defer a.Close()

	if req.Out == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("no output path given"))
		return
	}
	out, err := format.ExpandPath(req.Out)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if config.IsInsideVault(out) {
		writeError(w, http.StatusBadRequest,
			fmt.Errorf("refusing to write into the vault — peek is read-only; use Import to load an archive"))
		return
	}

	h, idx, ok := resolveInArchive(w, a, req)
	if !ok {
		return
	}
	data, err := a.Content(h.Snapshots[idx].ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(out), 0700); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.WriteFile(out, data, 0600); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"out": out, "version": idx + 1})
}

// resolveInArchive picks the requested file and version out of an archive,
// replying with the error itself if either lookup fails.
func resolveInArchive(w http.ResponseWriter, a *transfer.Archive, req peekRequest) (*store.FileHistory, int, bool) {
	found, err := a.Resolve(req.File.String())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return nil, 0, false
	}
	idx, err := pickVersion(len(found.Snapshots), req.Version.String())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return nil, 0, false
	}
	return found, idx, true
}
