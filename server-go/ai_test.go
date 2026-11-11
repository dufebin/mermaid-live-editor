package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeGateway records the last request's path and Authorization header.
func fakeGateway(t *testing.T, gotPath, gotAuth *string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotPath = r.URL.Path
		*gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestAIProxyHeaderOverride(t *testing.T) {
	var gotPath, gotAuth string
	gateway := fakeGateway(t, &gotPath, &gotAuth)
	ai, err := aiProxyHandler(gateway.URL, "sekrit")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/ai/v1/chat/completions", strings.NewReader(`{"q":1}`))
	req.Header.Set("Authorization", "Bearer forged-by-client")
	rec := httptest.NewRecorder()
	ai.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("proxy status: %d", rec.Code)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("prefix not stripped: %q", gotPath)
	}
	if gotAuth != "Bearer sekrit" {
		t.Fatalf("authorization not injected/overridden: %q", gotAuth)
	}
}

func TestAIProxyRoutingAndGate(t *testing.T) {
	var gotPath, gotAuth string
	gateway := fakeGateway(t, &gotPath, &gotAuth)
	t.Setenv("AI_PROXY_TARGET", gateway.URL)
	t.Setenv("AI_PROXY_KEY", "sekrit")
	h := newHandler(newTestStore(t), []byte("test-secret-0123456789abcdef"))

	// Anonymous request: routed to /ai but gated by the session.
	if code, body := doReq(t, h, "GET", "/ai/v1/models", "", ""); code != 401 || body["error"] != "unauthorized" {
		t.Fatalf("anonymous /ai: %d %v", code, body)
	}

	// Authenticated request: proxied with prefix stripped and key injected.
	res, _ := register(t, h, "ai@x.com", "password123")
	cookie := res.Header().Get("Set-Cookie")
	code, body := doReq(t, h, "POST", "/ai/v1/chat/completions", `{"q":1}`, cookie)
	if code != 200 || body["ok"] != true {
		t.Fatalf("authed /ai: %d %v", code, body)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("prefix not stripped via mux: %q", gotPath)
	}
	if gotAuth != "Bearer sekrit" {
		t.Fatalf("key not injected via mux: %q", gotAuth)
	}
}
