// Package web serves the browser UI and its JSON API on loopback only.
//
// The threat model is narrow but real: this server hands out the contents of
// every .env file on the machine. Three things keep that contained — it binds
// 127.0.0.1, it rejects any Host header that is not loopback (so a hostile site
// cannot reach it by pointing DNS at 127.0.0.1), and every request carries a
// random per-session token in a custom header, which a cross-origin page cannot
// send without a preflight this server never approves.
package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/akhshyganesh/envault/internal/store"
)

// tokenHeader carries the session token on API calls.
const tokenHeader = "X-Envault-Token"

// NewToken returns a fresh random session token.
func NewToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("cannot generate session token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// Server holds one browser session's authentication and its open vault.
type Server struct {
	token string
	store *store.Store
}

// NewServer opens the vault for a session authenticated by token.
func NewServer(token string) (*Server, error) {
	s, err := store.NewStore()
	if err != nil {
		return nil, err
	}
	return &Server{token: token, store: s}, nil
}

// requireLoopback rejects requests whose Host is not a loopback name, which is
// what stops DNS rebinding.
func requireLoopback(next http.Handler) http.Handler {
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

// requireToken checks the session token: in the header for API calls, in the
// query string for the page itself, which is the only way a fresh browser tab
// can present one. Static assets are exempt — they carry no vault data.
func (s *Server) requireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, public := staticAssets[r.URL.Path]; public {
			next.ServeHTTP(w, r)
			return
		}

		got := r.Header.Get(tokenHeader)
		if got == "" && !strings.HasPrefix(r.URL.Path, "/api/") {
			got = r.URL.Query().Get("token")
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
			http.Error(w, "invalid or missing session token — open the URL printed by 'envault web'",
				http.StatusUnauthorized)
			return
		}

		// This is a live view of local secrets; never let anything cache it.
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
