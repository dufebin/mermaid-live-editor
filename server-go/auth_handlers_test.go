package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testSecret = "test-secret-0123456789abcdef"

func newAuthHandler(t *testing.T) http.Handler {
	t.Helper()
	return newHandler(newTestStore(t), []byte(testSecret))
}

// register returns the recorder so callers can read the Set-Cookie header.
func register(t *testing.T, h http.Handler, email, password string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	res, parsed := doRaw(t, h, "POST", "/api/auth/register",
		`{"email":"`+email+`","password":"`+password+`"}`, "")
	body, ok := parsed.(map[string]any)
	if !ok {
		t.Fatalf("register: expected object response, got %T: %v", parsed, parsed)
	}
	return res, body
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func TestRegisterClaimLoginLogoutMe(t *testing.T) {
	h := newAuthHandler(t)

	res, body := register(t, h, "First@X.com", "password123")
	if res.Code != 201 || body["email"] != "first@x.com" {
		t.Fatalf("register: %d %v", res.Code, body)
	}
	cookie := res.Header().Get("Set-Cookie")
	if !contains(cookie, "HttpOnly") || !contains(cookie, "SameSite=Lax") {
		t.Fatalf("cookie: %q", cookie)
	}
	if code, list := doList(t, h, "GET", "/api/diagrams", "", cookie); code != 200 {
		t.Fatalf("authed list: %d %v", code, list)
	}
	if res, body := register(t, h, "first@x.com", "password123"); res.Code != 409 || body["error"] != "email already registered" {
		t.Fatalf("duplicate: %d %v", res.Code, body)
	}
	if res, body := register(t, h, "not-an-email", "password123"); res.Code != 400 || body["error"] != "email must be a valid address" {
		t.Fatalf("bad email: %d %v", res.Code, body)
	}
	if res, _ := register(t, h, "ok@x.com", "short"); res.Code != 400 {
		t.Fatalf("bad password: %d", res.Code)
	}
	res2, _ := register(t, h, "second@x.com", "password123")
	cookie2 := res2.Header().Get("Set-Cookie")

	res3, parsed3 := doRaw(t, h, "POST", "/api/auth/login", `{"email":"FIRST@X.com","password":"password123"}`, "")
	body3, ok := parsed3.(map[string]any)
	if !ok {
		t.Fatalf("login: expected object response, got %T: %v", parsed3, parsed3)
	}
	if res3.Code != 200 || body3["email"] != "first@x.com" || res3.Header().Get("Set-Cookie") == "" {
		t.Fatalf("login: %d %v", res3.Code, body3)
	}
	if code, body := doReq(t, h, "POST", "/api/auth/login", `{"email":"first@x.com","password":"wrong"}`, ""); code != 401 || body["error"] != "invalid email or password" {
		t.Fatalf("bad login: %d %v", code, body)
	}
	if code, body := doReq(t, h, "GET", "/api/auth/me", "", cookie); code != 200 || body["email"] != "first@x.com" {
		t.Fatalf("me: %d %v", code, body)
	}
	if code, body := doReq(t, h, "POST", "/api/auth/logout", "", cookie); code != 200 || body["ok"] != true {
		t.Fatalf("logout: %d %v", code, body)
	}
	if code, body := doReq(t, h, "GET", "/api/folders", "", ""); code != 401 || body["error"] != "unauthorized" {
		t.Fatalf("anonymous must 401: %d %v", code, body)
	}
	_ = cookie2
}

func TestIsolationOverHTTP(t *testing.T) {
	s := newTestStore(t)
	// legacy NULL-owner row: first registrant claims it
	if _, err := s.db.Exec("INSERT INTO diagrams (id, folder_id, name, state, favorite, positions, created_at, updated_at, user_id) VALUES ('legacy', NULL, 'L', 's', 0, NULL, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', NULL)"); err != nil {
		t.Fatal(err)
	}
	h := newHandler(s, []byte(testSecret))

	resA, _ := register(t, h, "a@x.com", "password123")
	cookieA := resA.Header().Get("Set-Cookie")
	resB, _ := register(t, h, "b@x.com", "password123")
	cookieB := resB.Header().Get("Set-Cookie")

	// first user claimed the legacy row; second user starts empty
	if code, list := doList(t, h, "GET", "/api/diagrams", "", cookieA); code != 200 || len(list) != 1 {
		t.Fatalf("claim: %d %v", code, list)
	}
	if code, list := doList(t, h, "GET", "/api/diagrams", "", cookieB); code != 200 || len(list) != 0 {
		t.Fatalf("second user must be empty: %d %v", code, list)
	}

	// A creates; B cannot read/patch/delete it
	_, folder := doReq(t, h, "POST", "/api/folders", `{"name":"A-folder"}`, cookieA)
	fid := folder["id"].(string)
	_, diagram := doReq(t, h, "POST", "/api/diagrams", `{"name":"A-diagram","state":"s"}`, cookieA)
	did := diagram["id"].(string)
	if code, _ := doReq(t, h, "GET", "/api/diagrams/"+did, "", cookieB); code != 404 {
		t.Fatalf("B must not read A's diagram: %d", code)
	}
	if code, _ := doReq(t, h, "PATCH", "/api/diagrams/"+did, `{"name":"hax"}`, cookieB); code != 404 {
		t.Fatalf("B must not patch A's diagram: %d", code)
	}
	if code, _ := doReq(t, h, "DELETE", "/api/diagrams/"+did, "", cookieB); code != 404 {
		t.Fatalf("B must not delete A's diagram: %d", code)
	}
	if code, _ := doReq(t, h, "GET", "/api/folders/"+fid, "", cookieB); code != 404 {
		t.Fatalf("B must not read A's folder: %d", code)
	}
	if code, body := doReq(t, h, "GET", "/api/diagrams/"+did, "", cookieA); code != 200 || body["name"] != "A-diagram" {
		t.Fatalf("A's diagram must be intact: %d %v", code, body)
	}
}
