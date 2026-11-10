package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// doRaw performs a request and returns the status code plus the decoded JSON body.
func doRaw(t *testing.T, h http.Handler, method, path, body string) (int, any) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var parsed any
	if data := rec.Body.Bytes(); len(data) > 0 {
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("%s %s: response is not JSON: %q", method, path, rec.Body.String())
		}
	}
	return rec.Code, parsed
}

// doReq asserts an object response and returns it as a map.
func doReq(t *testing.T, h http.Handler, method, path, body string) (int, map[string]any) {
	t.Helper()
	code, parsed := doRaw(t, h, method, path, body)
	obj, ok := parsed.(map[string]any)
	if !ok {
		t.Fatalf("%s %s: expected object response, got %T: %v", method, path, parsed, parsed)
	}
	return code, obj
}

// doList asserts an array response.
func doList(t *testing.T, h http.Handler, method, path, body string) (int, []any) {
	t.Helper()
	code, parsed := doRaw(t, h, method, path, body)
	items, ok := parsed.([]any)
	if !ok {
		t.Fatalf("%s %s: expected array response, got %T: %v", method, path, parsed, parsed)
	}
	return code, items
}

func TestRoutingAndBody(t *testing.T) {
	h := newHandler(newTestStore(t))
	if code, body := doReq(t, h, "GET", "/api/nope", ""); code != 404 || body["error"] != "Not found" {
		t.Fatalf("unknown path: %d %v", code, body)
	}
	if code, body := doReq(t, h, "GET", "/", ""); code != 404 || body["error"] != "Not found" {
		t.Fatalf("root path: %d %v", code, body)
	}
	if code, _ := doReq(t, h, "PUT", "/api/folders", "{}"); code != 404 {
		t.Fatalf("wrong method must 404 like node (no 405): %d", code)
	}
	if code, _ := doReq(t, h, "PATCH", "/api/folders", "{}"); code != 404 {
		t.Fatalf("wrong method on collection must 404: %d", code)
	}
	if code, _ := doReq(t, h, "POST", "/api/export", "{}"); code != 404 {
		t.Fatalf("wrong method on export must 404: %d", code)
	}
	if code, _ := doReq(t, h, "POST", "/api/folders/xyz", "{}"); code != 404 {
		t.Fatalf("wrong method on item must 404: %d", code)
	}
	if code, body := doReq(t, h, "POST", "/api/folders", "{bad json"); code != 400 || body["error"] != "Invalid JSON body" {
		t.Fatalf("bad json: %d %v", code, body)
	}
	if code, body := doReq(t, h, "POST", "/api/diagrams", "{bad json"); code != 400 || body["error"] != "Invalid JSON body" {
		t.Fatalf("bad json diagrams: %d %v", code, body)
	}
	big := `{"name":"` + strings.Repeat("x", maxBodyBytes+1) + `"}`
	if code, body := doReq(t, h, "POST", "/api/folders", big); code != 400 || body["error"] != "Payload too large" {
		t.Fatalf("payload too large: %d %v", code, body)
	}
	// Absent body behaves as an empty object, like node's readBody.
	if code, body := doReq(t, h, "POST", "/api/folders", ""); code != 400 || body["error"] != "name is required" {
		t.Fatalf("absent body: %d %v", code, body)
	}
}

func TestFolderEndpoints(t *testing.T) {
	h := newHandler(newTestStore(t))

	// Empty store list is an array, not null.
	if code, items := doList(t, h, "GET", "/api/folders", ""); code != 200 || len(items) != 0 {
		t.Fatalf("empty list: %d %v", code, items)
	}

	if code, body := doReq(t, h, "POST", "/api/folders", `{}`); code != 400 || body["error"] != "name is required" {
		t.Fatalf("missing folder name: %d %v", code, body)
	}
	if code, body := doReq(t, h, "POST", "/api/folders", `{"name":"  "}`); code != 400 || body["error"] != "name is required" {
		t.Fatalf("folder name: %d %v", code, body)
	}
	code, folder := doReq(t, h, "POST", "/api/folders", `{"name":" F "}`)
	if code != 201 || folder["name"] != "F" || folder["parentId"] != nil {
		t.Fatalf("create folder: %d %v", code, folder)
	}
	fid := folder["id"].(string)
	if folder["createdAt"] == "" || folder["updatedAt"] == "" {
		t.Fatalf("folder timestamps: %v", folder)
	}

	if code, body := doReq(t, h, "POST", "/api/folders", `{"name":"X","parentId":"missing"}`); code != 404 || body["error"] != "parent folder not found" {
		t.Fatalf("bad parent: %d %v", code, body)
	}
	code, child := doReq(t, h, "POST", "/api/folders", `{"name":"C","parentId":"`+fid+`"}`)
	if code != 201 || child["parentId"] != fid {
		t.Fatalf("create child: %d %v", code, child)
	}
	cid := child["id"].(string)

	// PATCH on a missing folder 404s before any body validation.
	if code, body := doReq(t, h, "PATCH", "/api/folders/missing", "{bad json"); code != 404 || body["error"] != "folder not found" {
		t.Fatalf("patch missing folder: %d %v", code, body)
	}
	if code, body := doReq(t, h, "PATCH", "/api/folders/"+fid, `{"name":"  "}`); code != 400 || body["error"] != "name must be a non-empty string" {
		t.Fatalf("patch empty name: %d %v", code, body)
	}
	if code, body := doReq(t, h, "PATCH", "/api/folders/"+fid, `{"parentId":"`+fid+`"}`); code != 400 || body["error"] != "cannot move a folder into itself or its descendant" {
		t.Fatalf("cycle self: %d %v", code, body)
	}
	if code, body := doReq(t, h, "PATCH", "/api/folders/"+fid, `{"parentId":"`+cid+`"}`); code != 400 || body["error"] != "cannot move a folder into itself or its descendant" {
		t.Fatalf("cycle descendant: %d %v", code, body)
	}
	if code, body := doReq(t, h, "PATCH", "/api/folders/"+cid, `{"parentId":"missing"}`); code != 404 || body["error"] != "parent folder not found" {
		t.Fatalf("patch bad parent: %d %v", code, body)
	}
	if code, body := doReq(t, h, "PATCH", "/api/folders/"+cid, `{"name":" C2 ","parentId":null}`); code != 200 || body["name"] != "C2" || body["parentId"] != nil {
		t.Fatalf("patch rename and move to root: %d %v", code, body)
	}
	// Empty-body PATCH is a no-op update.
	if code, body := doReq(t, h, "PATCH", "/api/folders/"+cid, `{}`); code != 200 || body["name"] != "C2" {
		t.Fatalf("patch no-op: %d %v", code, body)
	}

	if code, items := doList(t, h, "GET", "/api/folders", ""); code != 200 || len(items) != 2 {
		t.Fatalf("list folders: %d %v", code, items)
	}

	if code, body := doReq(t, h, "DELETE", "/api/folders/missing", ""); code != 404 || body["error"] != "folder not found" {
		t.Fatalf("delete missing: %d %v", code, body)
	}
	if code, body := doReq(t, h, "DELETE", "/api/folders/"+cid, ""); code != 200 || body["ok"] != true {
		t.Fatalf("delete folder: %d %v", code, body)
	}
	// Node has no GET /api/folders/{id} route: falls through to 404 "Not found".
	if code, body := doReq(t, h, "GET", "/api/folders/"+fid, ""); code != 404 || body["error"] != "Not found" {
		t.Fatalf("get single folder: %d %v", code, body)
	}
}

func TestDiagramEndpoints(t *testing.T) {
	h := newHandler(newTestStore(t))

	if code, items := doList(t, h, "GET", "/api/diagrams", ""); code != 200 || len(items) != 0 {
		t.Fatalf("empty diagram list: %d %v", code, items)
	}

	if code, body := doReq(t, h, "POST", "/api/diagrams", `{"state":"s"}`); code != 400 || body["error"] != "name is required" {
		t.Fatalf("diagram name: %d %v", code, body)
	}
	if code, body := doReq(t, h, "POST", "/api/diagrams", `{"name":"D"}`); code != 400 || body["error"] != "state is required" {
		t.Fatalf("diagram state: %d %v", code, body)
	}
	for _, positions := range []string{`"bad"`, `[]`, `{"A":{"dx":"x","dy":1}}`} {
		payload := `{"name":"D","state":"s","positions":` + positions + `}`
		if code, body := doReq(t, h, "POST", "/api/diagrams", payload); code != 400 || body["error"] != "positions must be an object of {dx, dy} number pairs" {
			t.Fatalf("bad positions %s: %d %v", positions, code, body)
		}
	}
	if code, body := doReq(t, h, "POST", "/api/diagrams", `{"name":"D","state":"s","folderId":"missing"}`); code != 404 || body["error"] != "folder not found" {
		t.Fatalf("bad diagram folder: %d %v", code, body)
	}

	_, folder := doReq(t, h, "POST", "/api/folders", `{"name":"Work"}`)
	fid := folder["id"].(string)

	code, d := doReq(t, h, "POST", "/api/diagrams", `{"name":" D ","state":"s","folderId":"`+fid+`","positions":{"A":{"dx":1.5,"dy":-2}}}`)
	if code != 201 || d["name"] != "D" || d["folderId"] != fid || d["favorite"] != false {
		t.Fatalf("create diagram: %d %v", code, d)
	}
	positions := d["positions"].(map[string]any)
	if positions["A"].(map[string]any)["dx"] != 1.5 || positions["A"].(map[string]any)["dy"] != -2.0 {
		t.Fatalf("positions roundtrip: %v", positions)
	}
	did := d["id"].(string)

	_, d2 := doReq(t, h, "POST", "/api/diagrams", `{"name":"beta","state":"s"}`)
	did2 := d2["id"].(string)

	// List + filters.
	if code, items := doList(t, h, "GET", "/api/diagrams", ""); code != 200 || len(items) != 2 {
		t.Fatalf("list diagrams: %d %v", code, items)
	}
	if code, items := doList(t, h, "GET", "/api/diagrams?folderId="+fid, ""); code != 200 || len(items) != 1 || items[0].(map[string]any)["id"] != did {
		t.Fatalf("list by folder: %d %v", code, items)
	}
	if code, items := doList(t, h, "GET", "/api/diagrams?folderId=root", ""); code != 200 || len(items) != 1 || items[0].(map[string]any)["id"] != did2 {
		t.Fatalf("list root: %d %v", code, items)
	}
	if code, items := doList(t, h, "GET", "/api/diagrams?search=beta", ""); code != 200 || len(items) != 1 || items[0].(map[string]any)["id"] != did2 {
		t.Fatalf("list search: %d %v", code, items)
	}
	if code, items := doList(t, h, "GET", "/api/diagrams?favorite=1", ""); code != 200 || len(items) != 0 {
		t.Fatalf("list favorite: %d %v", code, items)
	}

	// Item get.
	if code, body := doReq(t, h, "GET", "/api/diagrams/"+did, ""); code != 200 || body["id"] != did {
		t.Fatalf("get diagram: %d %v", code, body)
	}
	if code, body := doReq(t, h, "GET", "/api/diagrams/missing", ""); code != 404 || body["error"] != "diagram not found" {
		t.Fatalf("get missing diagram: %d %v", code, body)
	}

	// PATCH: 404 first, then validation in order.
	if code, body := doReq(t, h, "PATCH", "/api/diagrams/missing", "{bad json"); code != 404 || body["error"] != "diagram not found" {
		t.Fatalf("patch missing diagram: %d %v", code, body)
	}
	if code, body := doReq(t, h, "PATCH", "/api/diagrams/"+did, `{"name":"  "}`); code != 400 || body["error"] != "name must be a non-empty string" {
		t.Fatalf("patch empty name: %d %v", code, body)
	}
	if code, body := doReq(t, h, "PATCH", "/api/diagrams/"+did, `{"state":"  "}`); code != 400 || body["error"] != "state must be a non-empty string" {
		t.Fatalf("patch empty state: %d %v", code, body)
	}
	if code, body := doReq(t, h, "PATCH", "/api/diagrams/"+did, `{"positions":"bad"}`); code != 400 || body["error"] != "positions must be an object of {dx, dy} number pairs" {
		t.Fatalf("patch bad positions: %d %v", code, body)
	}
	if code, body := doReq(t, h, "PATCH", "/api/diagrams/"+did, `{"folderId":"missing"}`); code != 404 || body["error"] != "folder not found" {
		t.Fatalf("patch bad folder: %d %v", code, body)
	}

	// Happy-path patch: name trim, favorite, folder move; positions null clears to {}.
	_, folder2 := doReq(t, h, "POST", "/api/folders", `{"name":"Other"}`)
	fid2 := folder2["id"].(string)
	code, body := doReq(t, h, "PATCH", "/api/diagrams/"+did,
		`{"name":" N ","state":"t","favorite":true,"folderId":"`+fid2+`","positions":null}`)
	if code != 200 {
		t.Fatalf("patch diagram: %d %v", code, body)
	}
	if body["name"] != "N" || body["state"] != "t" || body["favorite"] != true || body["folderId"] != fid2 {
		t.Fatalf("patch diagram fields: %d %v", code, body)
	}
	if pos, ok := body["positions"].(map[string]any); !ok || len(pos) != 0 {
		t.Fatalf("patch positions null must serialize {}: %v", body["positions"])
	}

	// favorite=1 filter now matches the patched diagram.
	if code, items := doList(t, h, "GET", "/api/diagrams?favorite=1", ""); code != 200 || len(items) != 1 || items[0].(map[string]any)["id"] != did {
		t.Fatalf("favorite filter after patch: %d %v", code, items)
	}

	if code, body := doReq(t, h, "DELETE", "/api/diagrams/missing", ""); code != 404 || body["error"] != "diagram not found" {
		t.Fatalf("delete missing diagram: %d %v", code, body)
	}
	if code, body := doReq(t, h, "DELETE", "/api/diagrams/"+did, ""); code != 200 || body["ok"] != true {
		t.Fatalf("delete diagram: %d %v", code, body)
	}
	if code, body := doReq(t, h, "GET", "/api/diagrams/"+did, ""); code != 404 {
		t.Fatalf("deleted diagram still there: %d %v", code, body)
	}
}

func TestExportImport(t *testing.T) {
	h := newHandler(newTestStore(t))

	// Empty export: arrays, not null.
	code, exp := doReq(t, h, "GET", "/api/export", "")
	if code != 200 {
		t.Fatalf("export: %d", code)
	}
	diagrams, ok := exp["diagrams"].([]any)
	if !ok || len(diagrams) != 0 {
		t.Fatalf("export diagrams array: %v", exp["diagrams"])
	}
	folders, ok := exp["folders"].([]any)
	if !ok || len(folders) != 0 {
		t.Fatalf("export folders array: %v", exp["folders"])
	}
	if ts, ok := exp["exportedAt"].(string); !ok || ts == "" {
		t.Fatalf("exportedAt: %v", exp["exportedAt"])
	}

	// Seed data, then export contains it.
	_, folder := doReq(t, h, "POST", "/api/folders", `{"name":"Work"}`)
	_, d := doReq(t, h, "POST", "/api/diagrams", `{"name":"D","state":"s","positions":{"A":{"dx":1,"dy":2}},"folderId":"`+folder["id"].(string)+`"}`)
	code, exp = doReq(t, h, "GET", "/api/export", "")
	if code != 200 || len(exp["diagrams"].([]any)) != 1 || len(exp["folders"].([]any)) != 1 {
		t.Fatalf("export seeded: %d %v", code, exp)
	}
	exported := exp["diagrams"].([]any)[0].(map[string]any)
	if exported["id"] != d["id"] {
		t.Fatalf("export diagram id: %v", exported)
	}
	if _, ok := exported["positions"].(map[string]any); !ok {
		t.Fatalf("export positions: %v", exported)
	}

	// Import validation: both keys must be arrays.
	for _, bad := range []string{"", `{}`, `{"folders":{},"diagrams":[]}`, `{"folders":[],"diagrams":"x"}`} {
		if code, body := doReq(t, h, "POST", "/api/import", bad); code != 400 || body["error"] != "folders and diagrams arrays are required" {
			t.Fatalf("import validation %q: %d %v", bad, code, body)
		}
	}

	// Import: one folder + four diagrams (one duplicate, two invalid -> skipped).
	payload := `{"folders":[{"id":"f1","name":"F1","createdAt":"2026-01-01T00:00:00.000Z","updatedAt":"2026-01-01T00:00:00.000Z"}],` +
		`"diagrams":[{"id":"i1","name":"I1","state":"s","folderId":"f1","positions":{"B":{"dx":3,"dy":4}}},` +
		`{"id":"i1","name":"dup","state":"s"},{"id":"","name":"bad","state":"s"},{"id":"i2","name":"no-state","state":""}]}`
	code, res := doReq(t, h, "POST", "/api/import", payload)
	if code != 200 || res["imported"] != float64(2) || res["skipped"] != float64(3) {
		t.Fatalf("import: %d %v", code, res)
	}
	code, got := doReq(t, h, "GET", "/api/diagrams/i1", "")
	if code != 200 || got["name"] != "I1" || got["folderId"] != "f1" {
		t.Fatalf("imported diagram: %d %v", code, got)
	}
	if pos, ok := got["positions"].(map[string]any); !ok || pos["B"].(map[string]any)["dx"] != 3.0 {
		t.Fatalf("imported positions: %v", got["positions"])
	}

	// Lenient import (node's per-item tolerance in db.mjs importAll): a numeric
	// favorite still imports as favorite, a non-string id/name is skipped (not a
	// 400), and a string positions value imports with effectively-empty positions.
	lenient := `{"folders":[{"id":"fx","name":5}],` +
		`"diagrams":[{"id":"L1","name":"L1","state":"s","favorite":1},` +
		`{"id":123,"name":"L2","state":"s"},` +
		`{"id":"L3","name":"L3","state":"s","positions":"x"}]}`
	code, res = doReq(t, h, "POST", "/api/import", lenient)
	if code != 200 || res["imported"] != float64(2) || res["skipped"] != float64(2) {
		t.Fatalf("lenient import: %d %v", code, res)
	}
	code, got = doReq(t, h, "GET", "/api/diagrams/L1", "")
	if code != 200 || got["favorite"] != true {
		t.Fatalf("numeric favorite must import as favorite: %d %v", code, got)
	}
	code, got = doReq(t, h, "GET", "/api/diagrams/L3", "")
	if code != 200 || got["favorite"] != false {
		t.Fatalf("imported L3: %d %v", code, got)
	}
	if pos, ok := got["positions"].(map[string]any); !ok || len(pos) != 0 {
		t.Fatalf("string positions must import as {}: %v", got["positions"])
	}
	if code, body := doReq(t, h, "GET", "/api/diagrams/123", ""); code != 404 {
		t.Fatalf("non-string id must be skipped: %d %v", code, body)
	}
}
