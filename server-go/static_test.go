package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// newStaticFixture builds a temp static dir like an adapter-static build:
// index.html at root, 404.html SPA fallback, one asset with an extension.
func newStaticFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, content string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("index.html", "<html>home</html>")
	write("404.html", "<html>spa-fallback</html>")
	write("_app/immutable/assets/app.css", "body{}")
	return dir
}

func TestStaticHandler(t *testing.T) {
	dir := newStaticFixture(t)
	api := newHandler(newTestStore(t), []byte("test-secret-0123456789abcdef"))
	h := staticHandler(dir, api)

	// file hit serves the file with its content type
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/_app/immutable/assets/app.css", nil))
	if rec.Code != 200 || rec.Body.String() != "body{}" || rec.Header().Get("Content-Type") != "text/css; charset=utf-8" {
		t.Fatalf("asset: %d %q %q", rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
	}

	// root serves index.html
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 || rec.Body.String() != "<html>home</html>" {
		t.Fatalf("root: %d %q", rec.Code, rec.Body.String())
	}

	// unknown client-side route -> SPA fallback, status 200
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/edit", nil))
	if rec.Code != 200 || rec.Body.String() != "<html>spa-fallback</html>" {
		t.Fatalf("spa fallback: %d %q", rec.Code, rec.Body.String())
	}

	// directory path -> fallback (no listing)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/_app/immutable/", nil))
	if rec.Code != 200 || rec.Body.String() != "<html>spa-fallback</html>" {
		t.Fatalf("dir: %d %q", rec.Code, rec.Body.String())
	}

	// /api keeps the mux behavior: gated route 401s, unknown path 404 JSON
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/auth/me", nil))
	if rec.Code != 401 {
		t.Fatalf("api gating: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/nope", nil))
	if rec.Code != 404 || rec.Body.String() != `{"error":"Not found"}` {
		t.Fatalf("api 404: %d %q", rec.Code, rec.Body.String())
	}

	// non-GET non-api falls through to the mux (404, not static)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/edit", nil))
	if rec.Code != 404 {
		t.Fatalf("non-get: %d", rec.Code)
	}
}
