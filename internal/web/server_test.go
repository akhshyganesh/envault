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

// newTestServer relocates the vault into a temp HOME, seeds it with env files,
// and returns the handler plus the sandbox root.
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

// do issues an authenticated request from a loopback Host.
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
		Exists   bool   `json:"exists"`
	} `json:"files"`
}

// ── Access control ───────────────────────────────────────────────────────────

func TestAPIRequiresTheSessionToken(t *testing.T) {
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

// A hostile page could point a DNS name at 127.0.0.1; the Host check is what
// stops it reaching the API from the victim's browser.
func TestRejectsANonLoopbackHost(t *testing.T) {
	h, _ := newTestServer(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	req.Host = "envault.evil.example"
	req.Header.Set(tokenHeader, testToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 for a non-loopback Host, got %d", rec.Code)
	}
}

func TestPageNeedsATokenAndNeverEmbedsOne(t *testing.T) {
	h, _ := newTestServer(t, nil)

	get := func(target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Host = "127.0.0.1:7391"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := get("/"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without a token, got %d", rec.Code)
	}

	rec := get("/?token=" + testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 with a token, got %d", rec.Code)
	}
	// The page reads the token from the URL, so it must never be baked into
	// the served bytes — otherwise a cached copy would carry the session.
	if strings.Contains(rec.Body.String(), testToken) {
		t.Fatal("the served page contains the session token")
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cc)
	}
}

// The stylesheet and script hold no vault data, and the page cannot present a
// token when the browser fetches them.
func TestStaticAssetsAreServedWithoutAToken(t *testing.T) {
	h, _ := newTestServer(t, nil)

	for path, wantType := range map[string]string{
		"/app.css": "text/css",
		"/app.js":  "text/javascript",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Host = "127.0.0.1:7391"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d, want 200", path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, wantType) {
			t.Errorf("%s: Content-Type = %q, want %s", path, ct, wantType)
		}
		if rec.Body.Len() == 0 {
			t.Errorf("%s: served no bytes", path)
		}
	}
}

func TestNewTokenIsRandomAndLongEnough(t *testing.T) {
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

// ── Reading the vault ────────────────────────────────────────────────────────

func TestStateListsTrackedFiles(t *testing.T) {
	h, _ := newTestServer(t, map[string]string{"api/.env": "A=1", "web/.env.local": "B=1"})

	st := decode[stateResp](t, do(t, h, http.MethodGet, "/api/state", nil))
	if len(st.Files) != 2 {
		t.Fatalf("want 2 files, got %d", len(st.Files))
	}
	if st.Files[0].Index != 1 || st.Files[1].Index != 2 {
		t.Fatalf("files are not numbered from 1: %+v", st.Files)
	}
	if st.Files[0].Display == "" || st.Files[0].Versions != 1 {
		t.Fatalf("unexpected entry: %+v", st.Files[0])
	}
	if !st.Files[0].Exists {
		t.Error("a file still on disk was reported as gone")
	}
	if st.Daemon.Running {
		t.Error("no daemon should be running in a fresh sandbox")
	}
}

func TestHistoryAndContent(t *testing.T) {
	h, root := newTestServer(t, map[string]string{"api/.env": "A=1"})

	// A second version, so version selection means something.
	if err := os.WriteFile(filepath.Join(root, "proj", "api", ".env"), []byte("A=2"), 0600); err != nil {
		t.Fatal(err)
	}
	do(t, h, http.MethodPost, "/api/scan", map[string]any{"dirs": []string{filepath.Join(root, "proj")}})

	hist := decode[struct {
		Snapshots []struct {
			Version int    `json:"version"`
			ShortID string `json:"short_id"`
		} `json:"snapshots"`
	}](t, do(t, h, http.MethodGet, "/api/history?file=1", nil))
	if len(hist.Snapshots) != 2 {
		t.Fatalf("want 2 versions, got %d", len(hist.Snapshots))
	}
	if hist.Snapshots[0].Version != 1 || hist.Snapshots[0].ShortID == "" {
		t.Fatalf("unexpected version: %+v", hist.Snapshots[0])
	}

	type contentResp struct {
		Version int    `json:"version"`
		Content string `json:"content"`
	}
	if latest := decode[contentResp](t, do(t, h, http.MethodGet, "/api/content?file=1", nil)); latest.Content != "A=2" {
		t.Fatalf("default content = %q, want the newest (A=2)", latest.Content)
	}
	if first := decode[contentResp](t, do(t, h, http.MethodGet, "/api/content?file=1&version=1", nil)); first.Content != "A=1" {
		t.Fatalf("v1 content = %q, want A=1", first.Content)
	}

	if rec := do(t, h, http.MethodGet, "/api/content?file=1&version=99", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for an out-of-range version, got %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/content?file=42", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for an unknown file, got %d", rec.Code)
	}
}

// ── Changing the vault ───────────────────────────────────────────────────────

func TestScanBacksUpNewFiles(t *testing.T) {
	h, root := newTestServer(t, nil)

	dir := filepath.Join(root, "proj")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("X=1"), 0600); err != nil {
		t.Fatal(err)
	}

	res := decode[struct {
		Found  int `json:"found"`
		Backed int `json:"backed"`
	}](t, do(t, h, http.MethodPost, "/api/scan", map[string]any{"dirs": []string{dir}}))
	if res.Found != 1 || res.Backed != 1 {
		t.Fatalf("want 1 found / 1 backed, got %+v", res)
	}

	if st := decode[stateResp](t, do(t, h, http.MethodGet, "/api/state", nil)); len(st.Files) != 1 {
		t.Fatalf("scan did not track the file: %+v", st.Files)
	}
}

func TestRestoreWritesToTheChosenPath(t *testing.T) {
	h, root := newTestServer(t, map[string]string{"api/.env": "A=1"})

	out := filepath.Join(root, "out", "restored.env")
	res := decode[struct {
		Path string `json:"path"`
	}](t, do(t, h, http.MethodPost, "/api/restore",
		map[string]any{"file": 1, "version": 1, "path": out}))
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

func TestForgetThenGCReclaimsTheSpace(t *testing.T) {
	h, _ := newTestServer(t, map[string]string{"api/.env": "A=1"})

	if rec := do(t, h, http.MethodPost, "/api/forget", map[string]any{"file": 1}); rec.Code != http.StatusOK {
		t.Fatalf("forget: status %d: %s", rec.Code, rec.Body.String())
	}
	if st := decode[stateResp](t, do(t, h, http.MethodGet, "/api/state", nil)); len(st.Files) != 0 {
		t.Fatalf("still tracked after forget: %+v", st.Files)
	}

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
		t.Fatal(err)
	}
	if rec := do(t, h, http.MethodPost, "/api/watch", map[string]any{"dir": dir}); rec.Code != http.StatusOK {
		t.Fatalf("watch: status %d: %s", rec.Code, rec.Body.String())
	}

	st := decode[stateResp](t, do(t, h, http.MethodGet, "/api/state", nil))
	if !slicesContains(st.Config.WatchDirs, dir) {
		t.Fatalf("watch dir not persisted: %+v", st.Config.WatchDirs)
	}

	rec := do(t, h, http.MethodPost, "/api/watch", map[string]any{"dir": filepath.Join(root, "nope")})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for a missing directory, got %d", rec.Code)
	}
}

func TestConfigUpdateValidates(t *testing.T) {
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

	for name, body := range map[string]map[string]any{
		"zero interval":        {"scan_interval_secs": 0},
		"negative versions":    {"max_versions": -1},
		"missing watch folder": {"watch_dirs": []string{"/definitely/not/here"}},
	} {
		if rec := do(t, h, http.MethodPost, "/api/config", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d", name, rec.Code)
		}
	}
}

func TestExportDownloadsAZip(t *testing.T) {
	h, _ := newTestServer(t, map[string]string{"api/.env": "A=1"})

	rec := do(t, h, http.MethodGet, "/api/export", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("export: status %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition = %q", cd)
	}
	body := rec.Body.Bytes()
	if _, err := zip.NewReader(bytes.NewReader(body), int64(len(body))); err != nil {
		t.Fatalf("response is not a valid zip: %v", err)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h, _ := newTestServer(t, nil)

	if rec := do(t, h, http.MethodGet, "/api/scan", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405 for GET on a POST-only route, got %d", rec.Code)
	}
}

// ── Peeking ──────────────────────────────────────────────────────────────────

// exportToFile writes the vault out and returns the archive's path.
func exportToFile(t *testing.T, h http.Handler, root string) string {
	t.Helper()
	rec := do(t, h, http.MethodGet, "/api/export", nil)
	zipPath := filepath.Join(root, "backup.zip")
	if err := os.WriteFile(zipPath, rec.Body.Bytes(), 0600); err != nil {
		t.Fatalf("write zip: %v", err)
	}
	return zipPath
}

func TestPeekReadsAnArchiveWithoutImporting(t *testing.T) {
	h, root := newTestServer(t, map[string]string{"api/.env": "A=1"})
	zipPath := exportToFile(t, h, root)

	listing := decode[struct {
		Files []struct {
			Index    int `json:"index"`
			Versions int `json:"versions"`
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

	rec := do(t, h, http.MethodPost, "/api/peek", map[string]any{"path": filepath.Join(root, "missing.zip")})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for a missing archive, got %d", rec.Code)
	}
}

func TestPeekSaveWritesOutsideTheVaultOnly(t *testing.T) {
	h, root := newTestServer(t, map[string]string{"api/.env": "A=1"})
	zipPath := exportToFile(t, h, root)

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

	// Peek must refuse the vault even when asked directly.
	inVault := filepath.Join(config.VaultDir(), "sneaky.env")
	if rec := do(t, h, http.MethodPost, "/api/peek/save",
		map[string]any{"path": zipPath, "file": 1, "out": inVault}); rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 when saving into the vault, got %d", rec.Code)
	}
	if _, err := os.Stat(inVault); !os.IsNotExist(err) {
		t.Fatalf("peek wrote into the vault (err=%v)", err)
	}
}

// ── The page itself ──────────────────────────────────────────────────────────

// It ships inside the binary and must render with no network at all.
func TestPageIsSelfContained(t *testing.T) {
	page := assetSource("index.html")
	if page == "" {
		t.Fatal("index.html is not embedded")
	}
	for _, forbidden := range []string{`src="http`, `href="http`, "@import", "cdn."} {
		if strings.Contains(page, forbidden) {
			t.Errorf("the page reaches outside the binary: found %q", forbidden)
		}
	}
}

// .env values flow into this DOM, so nothing may be parsed as markup.
func TestPageNeverBuildsMarkupFromStrings(t *testing.T) {
	for _, name := range []string{"index.html", "app.js"} {
		source := assetSource(name)
		if source == "" {
			t.Fatalf("%s is not embedded", name)
		}
		for _, forbidden := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write"} {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s builds markup from strings: found %q", name, forbidden)
			}
		}
	}
}

// The mask must be a fixed string. One built from the value — repeating a dot
// per character — would draw a picture of how long every secret is.
func TestMaskDoesNotTrackSecretLength(t *testing.T) {
	script := assetSource("app.js")
	if !strings.Contains(script, `const MASK = "`) {
		t.Fatal("the mask is no longer a fixed string constant")
	}
	for _, line := range strings.Split(script, "\n") {
		if !strings.Contains(line, "MASK") {
			continue
		}
		for _, forbidden := range []string{".length", "repeat(", "slice(", "padEnd(", "padStart("} {
			if strings.Contains(line, forbidden) {
				t.Errorf("the mask is derived from the value on this line: %s", strings.TrimSpace(line))
			}
		}
	}
}

// Every icon the script asks for has to exist in the sprite, or it renders blank.
func TestEveryReferencedIconExists(t *testing.T) {
	page := assetSource("index.html")
	script := assetSource("app.js")

	for _, name := range []string{
		"lock", "search", "check", "alert", "eye", "eye-off", "copy", "restore",
		"save", "upload", "trash", "scan", "folder", "archive", "broom", "gear",
		"keyboard", "file", "back", "play", "stop",
	} {
		if !strings.Contains(page, `id="i-`+name+`"`) {
			t.Errorf("sprite is missing icon %q", name)
		}
	}

	// Nothing should reference an icon by a name the sprite does not define.
	for _, quoted := range []string{`icon("`, `icon: "`} {
		rest := script
		for {
			at := strings.Index(rest, quoted)
			if at < 0 {
				break
			}
			rest = rest[at+len(quoted):]
			end := strings.IndexByte(rest, '"')
			if end < 0 {
				break
			}
			if name := rest[:end]; !strings.Contains(page, `id="i-`+name+`"`) {
				t.Errorf("script uses icon %q, which the sprite does not define", name)
			}
		}
	}
}

func slicesContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
