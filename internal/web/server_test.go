package web

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akhshyganesh/envault/internal/config"
	"github.com/akhshyganesh/envault/internal/store"
)

const testToken = "test-token-0123456789"

// newTestServer points the vault at a temp HOME, seeds it with the given
// env files, and returns the handler plus the sandbox root.
func newTestServer(t *testing.T, envFiles map[string]string) (http.Handler, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	if err := os.MkdirAll(filepath.Join(root, "home"), 0700); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}

	s, err := store.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	for name, body := range envFiles {
		p := filepath.Join(root, "proj", name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, _, err := s.SaveSnapshot(p, "auto"); err != nil {
			t.Fatalf("SaveSnapshot: %v", err)
		}
	}

	srv, err := NewServer(testToken)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv.Handler(), root
}

// do issues an authenticated request against the handler.
func do(t *testing.T, h http.Handler, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, target, r)
	req.Host = "127.0.0.1:7391"
	req.Header.Set(tokenHeader, testToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// decode unmarshals a JSON response body, failing on a non-200 status.
func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return v
}

type stateResp struct {
	Daemon struct {
		Running bool `json:"running"`
		PID     int  `json:"pid"`
	} `json:"daemon"`
	Config struct {
		WatchDirs        []string `json:"watch_dirs"`
		ScanIntervalSecs int      `json:"scan_interval_secs"`
		MaxVersions      int      `json:"max_versions"`
	} `json:"config"`
	Files []struct {
		Index    int    `json:"index"`
		Path     string `json:"path"`
		Display  string `json:"display"`
		Versions int    `json:"versions"`
	} `json:"files"`
}

func TestAPIRequiresToken(t *testing.T) {
	h, _ := newTestServer(t, nil)

	for _, tc := range []struct{ name, token string }{
		{"missing", ""},
		{"wrong", "not-the-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
			req.Host = "127.0.0.1:7391"
			if tc.token != "" {
				req.Header.Set(tokenHeader, tc.token)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("want 401, got %d", rec.Code)
			}
		})
	}
}

func TestRejectsNonLocalHost(t *testing.T) {
	h, _ := newTestServer(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	req.Host = "envault.evil.example"
	req.Header.Set(tokenHeader, testToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 for a non-local Host header, got %d", rec.Code)
	}
}

func TestIndexPageRequiresTokenAndEmbedsIt(t *testing.T) {
	h, _ := newTestServer(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "127.0.0.1:7391"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without a token, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/?token="+testToken, nil)
	req.Host = "127.0.0.1:7391"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 with a token, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), testToken) {
		t.Fatal("page does not carry the session token to the client")
	}
	if strings.Contains(rec.Body.String(), tokenPlaceholder) {
		t.Fatal("token placeholder was not substituted")
	}
}

func TestStateListsTrackedFiles(t *testing.T) {
	h, _ := newTestServer(t, map[string]string{"api/.env": "A=1", "web/.env.local": "B=1"})

	st := decode[stateResp](t, do(t, h, http.MethodGet, "/api/state", nil))
	if len(st.Files) != 2 {
		t.Fatalf("want 2 files, got %d", len(st.Files))
	}
	if st.Files[0].Index != 1 || st.Files[1].Index != 2 {
		t.Fatalf("files not indexed from 1: %+v", st.Files)
	}
	if st.Files[0].Display == "" || st.Files[0].Versions != 1 {
		t.Fatalf("unexpected file entry: %+v", st.Files[0])
	}
	if st.Daemon.Running {
		t.Fatal("no daemon should be running in a fresh sandbox")
	}
}

func TestHistoryAndContent(t *testing.T) {
	h, root := newTestServer(t, map[string]string{"api/.env": "A=1"})

	// A second version so version selection is meaningful.
	p := filepath.Join(root, "proj", "api", ".env")
	if err := os.WriteFile(p, []byte("A=2"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	do(t, h, http.MethodPost, "/api/scan", map[string]any{"dirs": []string{filepath.Join(root, "proj")}})

	hist := decode[struct {
		Snapshots []struct {
			Version int    `json:"version"`
			ShortID string `json:"short_id"`
			Size    int64  `json:"size"`
		} `json:"snapshots"`
	}](t, do(t, h, http.MethodGet, "/api/history?file=1", nil))
	if len(hist.Snapshots) != 2 {
		t.Fatalf("want 2 snapshots, got %d", len(hist.Snapshots))
	}
	if hist.Snapshots[0].Version != 1 || hist.Snapshots[0].ShortID == "" {
		t.Fatalf("unexpected snapshot: %+v", hist.Snapshots[0])
	}

	type contentResp struct {
		Version int    `json:"version"`
		Content string `json:"content"`
	}
	latest := decode[contentResp](t, do(t, h, http.MethodGet, "/api/content?file=1", nil))
	if latest.Content != "A=2" {
		t.Fatalf("latest content = %q, want A=2", latest.Content)
	}
	first := decode[contentResp](t, do(t, h, http.MethodGet, "/api/content?file=1&version=1", nil))
	if first.Content != "A=1" {
		t.Fatalf("v1 content = %q, want A=1", first.Content)
	}

	if rec := do(t, h, http.MethodGet, "/api/content?file=1&version=99", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for an out-of-range version, got %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/content?file=42", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for an unknown file, got %d", rec.Code)
	}
}

func TestScanBacksUpNewFiles(t *testing.T) {
	h, root := newTestServer(t, nil)

	dir := filepath.Join(root, "proj")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("X=1"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	res := decode[struct {
		Found  int `json:"found"`
		Backed int `json:"backed"`
	}](t, do(t, h, http.MethodPost, "/api/scan", map[string]any{"dirs": []string{dir}}))
	if res.Found != 1 || res.Backed != 1 {
		t.Fatalf("want 1 found / 1 backed, got %+v", res)
	}

	st := decode[stateResp](t, do(t, h, http.MethodGet, "/api/state", nil))
	if len(st.Files) != 1 {
		t.Fatalf("scan did not track the file: %+v", st.Files)
	}
}

func TestRestoreWritesToChosenPath(t *testing.T) {
	h, root := newTestServer(t, map[string]string{"api/.env": "A=1"})

	out := filepath.Join(root, "out", "restored.env")
	res := decode[struct {
		Path string `json:"path"`
	}](t, do(t, h, http.MethodPost, "/api/restore", map[string]any{"file": 1, "version": 1, "path": out}))
	if res.Path != out {
		t.Fatalf("restored to %s, want %s", res.Path, out)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(data) != "A=1" {
		t.Fatalf("restored content = %q", data)
	}
}

func TestForgetStopsTracking(t *testing.T) {
	h, _ := newTestServer(t, map[string]string{"api/.env": "A=1"})

	if rec := do(t, h, http.MethodPost, "/api/forget", map[string]any{"file": 1}); rec.Code != http.StatusOK {
		t.Fatalf("forget: status %d: %s", rec.Code, rec.Body.String())
	}
	st := decode[stateResp](t, do(t, h, http.MethodGet, "/api/state", nil))
	if len(st.Files) != 0 {
		t.Fatalf("file still tracked after forget: %+v", st.Files)
	}
}

func TestGCReclaimsOrphanedBlobs(t *testing.T) {
	h, _ := newTestServer(t, map[string]string{"api/.env": "A=1"})

	do(t, h, http.MethodPost, "/api/forget", map[string]any{"file": 1})
	res := decode[struct {
		Removed int   `json:"removed"`
		Freed   int64 `json:"freed"`
	}](t, do(t, h, http.MethodPost, "/api/gc", nil))
	if res.Removed != 1 || res.Freed == 0 {
		t.Fatalf("want 1 blob reclaimed, got %+v", res)
	}
}

func TestWatchDirValidation(t *testing.T) {
	h, root := newTestServer(t, nil)

	dir := filepath.Join(root, "watched")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if rec := do(t, h, http.MethodPost, "/api/watch", map[string]any{"dir": dir}); rec.Code != http.StatusOK {
		t.Fatalf("watch: status %d: %s", rec.Code, rec.Body.String())
	}
	st := decode[stateResp](t, do(t, h, http.MethodGet, "/api/state", nil))
	found := false
	for _, d := range st.Config.WatchDirs {
		if d == dir {
			found = true
		}
	}
	if !found {
		t.Fatalf("watch dir not persisted: %+v", st.Config.WatchDirs)
	}

	rec := do(t, h, http.MethodPost, "/api/watch", map[string]any{"dir": filepath.Join(root, "nope")})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for a missing directory, got %d", rec.Code)
	}
}

func TestConfigUpdate(t *testing.T) {
	h, _ := newTestServer(t, nil)

	if rec := do(t, h, http.MethodPost, "/api/config", map[string]any{
		"scan_interval_secs": 120, "max_versions": 5,
	}); rec.Code != http.StatusOK {
		t.Fatalf("config: status %d: %s", rec.Code, rec.Body.String())
	}
	st := decode[stateResp](t, do(t, h, http.MethodGet, "/api/state", nil))
	if st.Config.ScanIntervalSecs != 120 || st.Config.MaxVersions != 5 {
		t.Fatalf("config not saved: %+v", st.Config)
	}

	rec := do(t, h, http.MethodPost, "/api/config", map[string]any{"scan_interval_secs": 0})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for a zero scan interval, got %d", rec.Code)
	}
}

func TestExportDownloadsZip(t *testing.T) {
	h, _ := newTestServer(t, map[string]string{"api/.env": "A=1"})

	rec := do(t, h, http.MethodGet, "/api/export", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("export: status %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("content-type = %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("content-disposition = %q", cd)
	}
	body := rec.Body.Bytes()
	if _, err := zip.NewReader(bytes.NewReader(body), int64(len(body))); err != nil {
		t.Fatalf("response is not a valid zip: %v", err)
	}
}

func TestPeekReadsArchiveWithoutImporting(t *testing.T) {
	h, root := newTestServer(t, map[string]string{"api/.env": "A=1"})

	rec := do(t, h, http.MethodGet, "/api/export", nil)
	zipPath := filepath.Join(root, "backup.zip")
	if err := os.WriteFile(zipPath, rec.Body.Bytes(), 0600); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	listing := decode[struct {
		Files []struct {
			Index    int    `json:"index"`
			Path     string `json:"path"`
			Versions int    `json:"versions"`
		} `json:"files"`
	}](t, do(t, h, http.MethodPost, "/api/peek", map[string]any{"path": zipPath}))
	if len(listing.Files) != 1 || listing.Files[0].Versions != 1 {
		t.Fatalf("unexpected peek listing: %+v", listing.Files)
	}

	content := decode[struct {
		Content string `json:"content"`
	}](t, do(t, h, http.MethodPost, "/api/peek/content", map[string]any{"path": zipPath, "file": 1}))
	if content.Content != "A=1" {
		t.Fatalf("peek content = %q", content.Content)
	}

	rec = do(t, h, http.MethodPost, "/api/peek", map[string]any{"path": filepath.Join(root, "missing.zip")})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for a missing archive, got %d", rec.Code)
	}
}

func TestPeekSaveWritesOutsideTheVault(t *testing.T) {
	h, root := newTestServer(t, map[string]string{"api/.env": "A=1"})

	rec := do(t, h, http.MethodGet, "/api/export", nil)
	zipPath := filepath.Join(root, "backup.zip")
	if err := os.WriteFile(zipPath, rec.Body.Bytes(), 0600); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	out := filepath.Join(root, "pulled", "out.env")
	res := decode[struct {
		Out string `json:"out"`
	}](t, do(t, h, http.MethodPost, "/api/peek/save",
		map[string]any{"path": zipPath, "file": 1, "out": out}))
	if res.Out != out {
		t.Fatalf("saved to %s, want %s", res.Out, out)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(data) != "A=1" {
		t.Fatalf("saved content = %q", data)
	}

	// Peek must never write into the vault, even when asked to.
	inVault := filepath.Join(config.VaultDir(), "sneaky.env")
	if rec := do(t, h, http.MethodPost, "/api/peek/save",
		map[string]any{"path": zipPath, "file": 1, "out": inVault}); rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 when saving into the vault, got %d", rec.Code)
	}
	if _, err := os.Stat(inVault); !os.IsNotExist(err) {
		t.Fatalf("peek wrote into the vault (err=%v)", err)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h, _ := newTestServer(t, nil)

	if rec := do(t, h, http.MethodGet, "/api/scan", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405 for GET on a POST-only route, got %d", rec.Code)
	}
}

// The page ships inside the binary and must render with no network at all.
func TestPageIsSelfContained(t *testing.T) {
	for _, forbidden := range []string{"src=\"http", "href=\"http", "@import", "cdn."} {
		if strings.Contains(indexHTML, forbidden) {
			t.Errorf("page reaches outside the binary: found %q", forbidden)
		}
	}
}

// Values from .env files flow into the page, so nothing may be parsed as markup.
func TestPageNeverAssignsInnerHTML(t *testing.T) {
	for _, forbidden := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write"} {
		if strings.Contains(indexHTML, forbidden) {
			t.Errorf("page builds markup from strings: found %q", forbidden)
		}
	}
}

func TestNewTokenIsRandomAndOpaque(t *testing.T) {
	a, err := NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	b, _ := NewToken()
	if a == b {
		t.Fatal("tokens are not random")
	}
	if len(a) < 32 {
		t.Fatalf("token too short: %q", a)
	}
}
