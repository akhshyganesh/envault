package web

import (
	"embed"
	"net/http"
)

// The UI ships inside the binary: one HTML document, one stylesheet, one
// script. Splitting them out of a single file keeps each readable, and they
// stay embedded so 'envault web' works with no network and no install step.
//
//go:embed assets/index.html assets/app.css assets/app.js
var assetFS embed.FS

// staticAssets are the files served without a session token. They are the same
// bytes for every user and contain no vault data — only the page they bootstrap
// does, and that one is authenticated.
var staticAssets = map[string]string{
	"/app.css": "text/css; charset=utf-8",
	"/app.js":  "text/javascript; charset=utf-8",
}

func serveAsset(w http.ResponseWriter, name, contentType string) {
	data, err := assetFS.ReadFile("assets/" + name)
	if err != nil {
		http.Error(w, "asset missing from binary: "+name, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

// assetSource returns an embedded file's text. Tests read the page this way to
// assert things about it that no HTTP round trip would reveal.
func assetSource(name string) string {
	data, err := assetFS.ReadFile("assets/" + name)
	if err != nil {
		return ""
	}
	return string(data)
}
