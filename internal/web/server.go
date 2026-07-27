// ABOUTME: Local HTTP server exposing envault's vault operations to a browser.
// ABOUTME: Loopback-only and token-authenticated — it serves secrets, so it is never public.
package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/daemon"
	"github.com/akhshyganesh/envault/internal/format"
	"github.com/akhshyganesh/envault/internal/scanner"
	"github.com/akhshyganesh/envault/internal/store"
	"github.com/akhshyganesh/envault/internal/transfer"
)

// tokenHeader carries the session token on every API call. A custom header
// means a cross-origin page cannot send one without a CORS preflight, which
// this server never approves.
const tokenHeader = "X-Envault-Token"

// tokenPlaceholder is substituted with the live session token when index.html
// is served.
const tokenPlaceholder = "__ENVAULT_TOKEN__"

// NewToken returns a fresh random session token.
func NewToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("cannot generate session token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// Server serves the browser UI and its JSON API.
type Server struct {
	token string
	store *store.Store
}

// NewServer opens the vault and returns a server authenticated by token.
func NewServer(token string) (*Server, error) {
	s, err := store.NewStore()
	if err != nil {
		return nil, err
	}
	return &Server{token: token, store: s}, nil
}

// Handler returns the fully wired HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/history", s.handleHistory)
	mux.HandleFunc("GET /api/content", s.handleContent)
	mux.HandleFunc("GET /api/export", s.handleExport)
	mux.HandleFunc("POST /api/scan", s.handleScan)
	mux.HandleFunc("POST /api/restore", s.handleRestore)
	mux.HandleFunc("POST /api/forget", s.handleForget)
	mux.HandleFunc("POST /api/gc", s.handleGC)
	mux.HandleFunc("POST /api/watch", s.handleWatch)
	mux.HandleFunc("POST /api/config", s.handleConfig)
	mux.HandleFunc("POST /api/daemon", s.handleDaemon)
	mux.HandleFunc("POST /api/import", s.handleImport)
	mux.HandleFunc("POST /api/peek", s.handlePeek)
	mux.HandleFunc("POST /api/peek/content", s.handlePeekContent)
	mux.HandleFunc("POST /api/peek/save", s.handlePeekSave)

	return s.withLocalHost(s.withAuth(mux))
}

// withLocalHost rejects requests whose Host header is not a loopback name.
// Without this, a hostile site could point a DNS name at 127.0.0.1 and reach
// the API from the victim's browser (DNS rebinding).
func (s *Server) withLocalHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		switch strings.ToLower(strings.Trim(host, "[]")) {
		case "localhost", "127.0.0.1", "::1":
			next.ServeHTTP(w, r)
		default:
			http.Error(w, "envault web only serves localhost", http.StatusForbidden)
		}
	})
}

// withAuth requires the session token: in the header for API calls, in the
// query string for the page itself (the only way a browser can bootstrap).
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get(tokenHeader)
		if got == "" && !strings.HasPrefix(r.URL.Path, "/api/") {
			got = r.URL.Query().Get("token")
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
			http.Error(w, "invalid or missing session token — open the URL printed by 'envault web'", http.StatusUnauthorized)
			return
		}
		// The UI is a live view of local secrets; never let anything cache it.
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	page := strings.ReplaceAll(indexHTML, tokenPlaceholder, s.token)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}

// --- state ---

type fileEntry struct {
	Index      int    `json:"index"`
	Path       string `json:"path"`
	Display    string `json:"display"`
	Versions   int    `json:"versions"`
	LastBackup string `json:"last_backup"`
	LastSize   int64  `json:"last_size"`
	// Exists reports whether the original file is still on disk. It is only
	// meaningful for vault listings, so it is omitted from archive listings.
	Exists bool `json:"exists,omitempty"`
}

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
		e := fileEntry{
			Index:   i + 1,
			Path:    f.FilePath,
			Display: format.ShortenPath(f.FilePath),
		}
		if n := len(f.Snapshots); n > 0 {
			last := f.Snapshots[n-1]
			e.Versions = n
			e.LastBackup = format.TimeAgo(last.Timestamp)
			e.LastSize = last.Size
		}
		if _, statErr := os.Stat(f.FilePath); statErr == nil {
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

// --- history & content ---

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
	data, err := s.store.GetBlobContent(h.Snapshots[idx].ID)
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

// --- actions ---

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

	result, err := scanner.ScanDirectories(dirs, s.store)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	warnings := make([]string, 0, len(result.Errors))
	for _, e := range result.Errors {
		warnings = append(warnings, e.Error())
	}
	writeJSON(w, map[string]any{
		"found":    len(result.Found),
		"backed":   result.Backed,
		"skipped":  result.Skipped,
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
		target, err = format.ExpandPath(req.Path)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	if err := s.store.RestoreSnapshot(target, h.Snapshots[idx].ID); err != nil {
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
			writeError(w, http.StatusBadRequest, fmt.Errorf("max versions cannot be negative"))
			return
		}
		cfg.MaxVersions = *req.MaxVersions
	}
	if req.WatchDirs != nil {
		dirs := make([]string, 0, len(*req.WatchDirs))
		for _, d := range *req.WatchDirs {
			abs, expErr := format.ExpandPath(d)
			if expErr != nil {
				writeError(w, http.StatusBadRequest, expErr)
				return
			}
			if info, statErr := os.Stat(abs); statErr != nil || !info.IsDir() {
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
		c := exec.Command(exe, "start")
		c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := c.Start(); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		// Give the child time to claim the PID file so the reply is accurate.
		time.Sleep(400 * time.Millisecond)
	default:
		writeError(w, http.StatusBadRequest, fmt.Errorf("action must be 'start' or 'stop'"))
		return
	}

	running, pid := daemon.IsRunning()
	writeJSON(w, map[string]any{"running": running, "pid": pid})
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	tmpDir, err := os.MkdirTemp("", "envault-export")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer os.RemoveAll(tmpDir)

	name := fmt.Sprintf("envault-backup-%s.zip", time.Now().Format("20060102-150405"))
	zipPath, _, _, err := transfer.Export(filepath.Join(tmpDir, name))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	http.ServeFile(w, r, zipPath)
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

// --- peek (read-only archive browsing) ---

func (s *Server) handlePeek(w http.ResponseWriter, r *http.Request) {
	a, _, ok := s.openPeek(w, r)
	if !ok {
		return
	}
	defer a.Close()

	files := a.Files()
	entries := make([]fileEntry, 0, len(files))
	for i, f := range files {
		e := fileEntry{Index: i + 1, Path: f.FilePath, Display: format.ShortenPath(f.FilePath)}
		if n := len(f.Snapshots); n > 0 {
			e.Versions = n
			e.LastBackup = format.TimeAgo(f.Snapshots[n-1].Timestamp)
			e.LastSize = f.Snapshots[n-1].Size
		}
		entries = append(entries, e)
	}
	writeJSON(w, map[string]any{"files": entries})
}

func (s *Server) handlePeekContent(w http.ResponseWriter, r *http.Request) {
	a, req, ok := s.openPeek(w, r)
	if !ok {
		return
	}
	defer a.Close()

	h, err := a.Resolve(req.File.String())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	idx, err := pickVersion(len(h.Snapshots), req.Version.String())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
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

// handlePeekSave writes one version out of an archive to a path of the user's
// choosing. The vault is never touched.
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
	if strings.HasPrefix(out+string(os.PathSeparator), config.VaultDir()+string(os.PathSeparator)) {
		writeError(w, http.StatusBadRequest,
			fmt.Errorf("refusing to write into the vault — peek is read-only; use Import to load an archive"))
		return
	}

	h, err := a.Resolve(req.File.String())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	idx, err := pickVersion(len(h.Snapshots), req.Version.String())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
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

type peekRequest struct {
	Path    string      `json:"path"`
	File    json.Number `json:"file"`
	Version json.Number `json:"version"`
	Out     string      `json:"out"`
}

// openPeek decodes a peek request and opens the archive, replying with an
// error itself if either step fails.
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

// --- helpers ---

// resolveFile maps the 1-based index used by the UI to a tracked file.
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

// pickVersion converts a 1-based version string to a slice index, defaulting
// to the latest snapshot when empty or zero.
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

func snapshotEntries(snaps []store.Snapshot) []snapshotEntry {
	out := make([]snapshotEntry, 0, len(snaps))
	for i, snap := range snaps {
		out = append(out, snapshotEntry{
			Version:   i + 1,
			ID:        snap.ID,
			ShortID:   snap.ID[:12],
			Timestamp: snap.Timestamp.Local().Format("2006-01-02 15:04"),
			Ago:       format.TimeAgo(snap.Timestamp),
			Size:      snap.Size,
			HumanSize: format.HumanSize(snap.Size),
			Comment:   snap.Comment,
		})
	}
	return out
}

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
