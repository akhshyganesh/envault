package web

import "net/http"

// Handler wires every route. The whole surface of the browser UI is this one
// table — adding a feature means adding a line here and a handler beside its
// siblings in api_vault.go or api_peek.go.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// The page and its assets.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		serveAsset(w, "index.html", "text/html; charset=utf-8")
	})
	for path, contentType := range staticAssets {
		name, ct := path[1:], contentType
		mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
			serveAsset(w, name, ct)
		})
	}

	// Reading the vault.
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/history", s.handleHistory)
	mux.HandleFunc("GET /api/content", s.handleContent)
	mux.HandleFunc("GET /api/export", s.handleExport)

	// Changing the vault.
	mux.HandleFunc("POST /api/scan", s.handleScan)
	mux.HandleFunc("POST /api/restore", s.handleRestore)
	mux.HandleFunc("POST /api/forget", s.handleForget)
	mux.HandleFunc("POST /api/gc", s.handleGC)
	mux.HandleFunc("POST /api/watch", s.handleWatch)
	mux.HandleFunc("POST /api/config", s.handleConfig)
	mux.HandleFunc("POST /api/daemon", s.handleDaemon)
	mux.HandleFunc("POST /api/import", s.handleImport)

	// Reading an archive without importing it.
	mux.HandleFunc("POST /api/peek", s.handlePeek)
	mux.HandleFunc("POST /api/peek/content", s.handlePeekContent)
	mux.HandleFunc("POST /api/peek/save", s.handlePeekSave)

	return requireLoopback(s.requireToken(mux))
}
