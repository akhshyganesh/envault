package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/daemon"
	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/scanner"
	"github.com/akhshyganesh/envault/internal/transfer"
)

// Every handler here delegates to the same store, config, scanner and daemon
// calls the CLI uses. New behaviour belongs in those packages, not in this one,
// so the terminal and the browser can never disagree about what a command does.

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	files, err := s.store.ListTrackedFiles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	running, pid := daemon.IsRunning()

	entries := make([]fileEntry, 0, len(files))
	for i, f := range files {
		e := newFileEntry(i+1, f)
		if _, err := os.Stat(f.FilePath); err == nil {
			e.Exists = true
		}
		entries = append(entries, e)
	}

	writeJSON(w, map[string]any{
		"daemon":    map[string]any{"running": running, "pid": pid},
		"config":    cfg,
		"vault_dir": config.VaultDir(),
		"files":     entries,
	})
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	h, err := s.resolveFile(r.URL.Query().Get("file"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{
		"path":      h.FilePath,
		"display":   format.ShortenPath(h.FilePath),
		"snapshots": snapshotEntries(h.Snapshots),
	})
}

func (s *Server) handleContent(w http.ResponseWriter, r *http.Request) {
	h, err := s.resolveFile(r.URL.Query().Get("file"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	idx, err := pickVersion(len(h.Snapshots), r.URL.Query().Get("version"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	data, err := s.store.Content(h.Snapshots[idx].ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{
		"path":    h.FilePath,
		"display": format.ShortenPath(h.FilePath),
		"version": idx + 1,
		"content": string(data),
	})
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Dirs []string `json:"dirs"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	dirs := req.Dirs
	if len(dirs) == 0 {
		cfg, err := config.Load()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		dirs = cfg.WatchDirs
	}

	res, err := scanner.ScanDirectories(dirs, s.store)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	warnings := make([]string, 0, len(res.Errors))
	for _, e := range res.Errors {
		warnings = append(warnings, e.Error())
	}
	writeJSON(w, map[string]any{
		"found":    len(res.Found),
		"backed":   res.Backed,
		"skipped":  res.Skipped,
		"warnings": warnings,
	})
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	var req struct {
		File    json.Number `json:"file"`
		Version json.Number `json:"version"`
		Path    string      `json:"path"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	h, err := s.resolveFile(req.File.String())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	idx, err := pickVersion(len(h.Snapshots), req.Version.String())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	target := h.FilePath
	if req.Path != "" {
		if target, err = format.ExpandPath(req.Path); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	if err := s.store.Restore(target, h.Snapshots[idx].ID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"path": target, "version": idx + 1})
}

func (s *Server) handleForget(w http.ResponseWriter, r *http.Request) {
	var req struct {
		File json.Number `json:"file"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	h, err := s.resolveFile(req.File.String())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.Forget(h.FilePath); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"path": h.FilePath})
}

func (s *Server) handleGC(w http.ResponseWriter, r *http.Request) {
	res, err := s.store.GC()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{
		"removed":     res.Removed,
		"freed":       res.Freed,
		"human_freed": format.HumanSize(res.Freed),
	})
}

func (s *Server) handleWatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Dir string `json:"dir"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	dir, err := format.ExpandPath(req.Dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := config.AddWatchDir(dir); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"dir": dir})
}

// handleConfig patches only the fields the page sent — pointers distinguish
// "set this to zero" from "leave it alone".
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WatchDirs        *[]string `json:"watch_dirs"`
		ScanIntervalSecs *int      `json:"scan_interval_secs"`
		MaxVersions      *int      `json:"max_versions"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if req.ScanIntervalSecs != nil {
		if *req.ScanIntervalSecs < 1 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("scan interval must be at least 1 second"))
			return
		}
		cfg.ScanIntervalSecs = *req.ScanIntervalSecs
	}
	if req.MaxVersions != nil {
		if *req.MaxVersions < 0 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("versions to keep cannot be negative"))
			return
		}
		cfg.MaxVersions = *req.MaxVersions
	}
	if req.WatchDirs != nil {
		dirs := make([]string, 0, len(*req.WatchDirs))
		for _, d := range *req.WatchDirs {
			abs, err := format.ExpandPath(d)
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if info, err := os.Stat(abs); err != nil || !info.IsDir() {
				writeError(w, http.StatusBadRequest, fmt.Errorf("not a directory: %s", d))
				return
			}
			dirs = append(dirs, abs)
		}
		cfg.WatchDirs = dirs
	}

	if err := cfg.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, cfg)
}

func (s *Server) handleDaemon(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	switch req.Action {
	case "stop":
		if err := daemon.Stop(); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	case "start":
		exe, err := os.Executable()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		// Its own process group, so stopping the web server leaves it running.
		c := exec.Command(exe, "start")
		c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := c.Start(); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		// Let the child claim the PID file so the reply reflects reality.
		time.Sleep(400 * time.Millisecond)
	default:
		writeError(w, http.StatusBadRequest, fmt.Errorf("action must be 'start' or 'stop'"))
		return
	}

	running, pid := daemon.IsRunning()
	writeJSON(w, map[string]any{"running": running, "pid": pid})
}

// handleExport builds the archive in a temp directory and streams it as a
// download, so the browser decides where it lands.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	tmpDir, err := os.MkdirTemp("", "envault-export")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer os.RemoveAll(tmpDir)

	name := transfer.DefaultExportName()
	res, err := transfer.Export(filepath.Join(tmpDir, name))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	http.ServeFile(w, r, res.Path)
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path  string `json:"path"`
		Force bool   `json:"force"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	path, err := format.ExpandPath(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	count, err := transfer.Import(path, req.Force)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"files": count})
}
