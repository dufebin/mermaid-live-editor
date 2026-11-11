package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// immutableCache is served on /_app/* file hits: Vite content-hashes those
// assets, so they are safe to cache forever.
const immutableCache = "public, max-age=31536000, immutable"

// staticHandler serves dir on the same port as the API. /api/* (and any
// non-GET/HEAD request) falls through to the API mux; existing files are
// served statically; everything else gets the adapter-static SPA fallback
// (404.html) with status 200 so client-side routing takes over.
func staticHandler(dir string, api http.Handler) http.Handler {
	files := http.FileServer(http.Dir(dir))
	fallback := filepath.Join(dir, "404.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" ||
			(r.Method != http.MethodGet && r.Method != http.MethodHead) {
			api.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/_app/") {
			w.Header().Set("Cache-Control", immutableCache)
		}
		// filepath.Clean("/"+path) stays inside dir; ServeFile also rejects
		// traversal. Directories serve their index.html if present (so "/"
		// serves the app), otherwise they fall through to the fallback (no
		// listing).
		target := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if info, err := os.Stat(target); err == nil {
			if !info.IsDir() {
				files.ServeHTTP(w, r)
				return
			}
			if _, err := os.Stat(filepath.Join(target, "index.html")); err == nil {
				files.ServeHTTP(w, r)
				return
			}
		}
		// The fallback is not content-hashed: never let it be cached
		// immutable (a missed /_app/ asset must not poison its hashed URL).
		w.Header().Del("Cache-Control")
		http.ServeFile(w, r, fallback)
	})
}

func staticDirOrNone(dir string) string {
	if dir == "" {
		return "off"
	}
	return dir
}
