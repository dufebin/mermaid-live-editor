package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const maxBodyBytes = 5_000_000

type api struct {
	s      *store
	secret []byte
}

func newHandler(s *store, secret []byte) http.Handler {
	a := &api{s, secret}
	mux := http.NewServeMux()
	// Method dispatch happens inside each handler: node 404s (never 405s) on mismatch.
	mux.HandleFunc("/api/auth/register", a.handleRegister)
	mux.HandleFunc("/api/auth/login", a.handleLogin)
	mux.HandleFunc("/api/auth/logout", a.handleLogout)
	mux.HandleFunc("/api/auth/me", a.handleMe)
	mux.HandleFunc("/api/folders", a.requireAuth(a.folders))
	mux.HandleFunc("/api/folders/", a.requireAuth(a.folderItem))
	mux.HandleFunc("/api/diagrams", a.requireAuth(a.diagrams))
	mux.HandleFunc("/api/diagrams/", a.requireAuth(a.diagramItem))
	mux.HandleFunc("/api/export", a.requireAuth(a.export))
	mux.HandleFunc("/api/import", a.requireAuth(a.importEndpoint))
	mux.HandleFunc("/", a.notFound)
	return mux
}

// requireAuth gates a handler behind a valid session cookie, threading the JWT's
// userID into the handler for per-user store scoping.
func (a *api) requireAuth(handler func(w http.ResponseWriter, r *http.Request, userID string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := a.userIDFromRequest(r)
		if !ok {
			writeError(w, 401, "unauthorized")
			return
		}
		handler(w, r, userID)
	}
}

func (a *api) notFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, 404, "Not found")
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	data, _ := json.Marshal(body)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

type apiError struct{ message string }

func (e *apiError) Error() string { return e.message }

var errInvalidJSON = &apiError{"Invalid JSON body"}

// readBody: absent body -> empty map; oversized -> Payload too large.
func readBody(r *http.Request) (map[string]json.RawMessage, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		return nil, &apiError{err.Error()}
	}
	if len(raw) > maxBodyBytes {
		return nil, &apiError{"Payload too large"}
	}
	if len(raw) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, errInvalidJSON
	}
	return body, nil
}

// rawString: absent OR null OR non-string -> nil.
func rawString(v json.RawMessage) *string {
	if len(v) == 0 || string(v) == "null" {
		return nil
	}
	var s string
	if json.Unmarshal(v, &s) != nil {
		return nil
	}
	return &s
}

// rawBool: absent OR null OR non-boolean -> nil (keep the current value).
func rawBool(v json.RawMessage) *bool {
	if len(v) == 0 || string(v) == "null" {
		return nil
	}
	var b bool
	if json.Unmarshal(v, &b) != nil {
		return nil
	}
	return &b
}

func rawParent(v json.RawMessage) parentValue {
	if len(v) == 0 {
		return parentValue{}
	}
	if string(v) == "null" {
		return parentValue{Set: true}
	}
	var s string
	if json.Unmarshal(v, &s) != nil {
		return parentValue{Set: true}
	}
	return parentValue{Set: true, Value: &s}
}

// rawPositions: (value, ok). ok=false means present-but-invalid -> 400.
func rawPositions(v json.RawMessage) (positionsValue, bool) {
	if len(v) == 0 {
		return positionsValue{}, true
	}
	if string(v) == "null" {
		return positionsValue{Set: true}, true
	}
	var p map[string]NodeOffset
	if json.Unmarshal(v, &p) != nil {
		return positionsValue{}, false
	}
	for _, offset := range p {
		if offset.Dx != offset.Dx || offset.Dy != offset.Dy { // NaN
			return positionsValue{}, false
		}
	}
	return positionsValue{Set: true, Value: p}, true
}

func isName(s *string) bool { return s != nil && strings.TrimSpace(*s) != "" }

// orEmpty keeps node's `.map()` shape: an empty list is [] in JSON, not null.
func orEmpty[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func isJSONArray(v json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(v))
	return len(trimmed) > 0 && trimmed[0] == '['
}

func (a *api) folders(w http.ResponseWriter, r *http.Request, userID string) {
	switch r.Method {
	case "GET":
		writeJSON(w, 200, orEmpty(a.s.listFolders(userID)))
	case "POST":
		body, err := readBody(r)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		name := rawString(body["name"])
		if !isName(name) {
			writeError(w, 400, "name is required")
			return
		}
		parent := rawParent(body["parentId"])
		if parent.Set && parent.Value != nil && a.s.getFolder(userID, *parent.Value) == nil {
			writeError(w, 404, "parent folder not found")
			return
		}
		writeJSON(w, 201, a.s.createFolder(userID, strings.TrimSpace(*name), parent.Value))
	default:
		a.notFound(w, r)
	}
}

func (a *api) folderItem(w http.ResponseWriter, r *http.Request, userID string) {
	id := strings.TrimPrefix(r.URL.Path, "/api/folders/")
	// Node's /([^/]+)/ pattern never matches an empty or nested id.
	if id == "" || strings.Contains(id, "/") {
		a.notFound(w, r)
		return
	}
	switch r.Method {
	case "PATCH":
		if a.s.getFolder(userID, id) == nil {
			writeError(w, 404, "folder not found")
			return
		}
		body, err := readBody(r)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		name := rawString(body["name"])
		if len(body["name"]) > 0 && !isName(name) {
			writeError(w, 400, "name must be a non-empty string")
			return
		}
		parent := rawParent(body["parentId"])
		if parent.Set && parent.Value != nil {
			if *parent.Value == id || a.s.isSelfOrAncestor(userID, id, *parent.Value) {
				writeError(w, 400, "cannot move a folder into itself or its descendant")
				return
			}
			if a.s.getFolder(userID, *parent.Value) == nil {
				writeError(w, 404, "parent folder not found")
				return
			}
		}
		var namePtr *string
		if name != nil {
			trimmed := strings.TrimSpace(*name)
			namePtr = &trimmed
		}
		updated, err := a.s.updateFolder(userID, id, namePtr, parent)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, updated)
	case "DELETE":
		if a.s.deleteFolder(userID, id) {
			writeJSON(w, 200, map[string]bool{"ok": true})
		} else {
			writeError(w, 404, "folder not found")
		}
	default:
		a.notFound(w, r)
	}
}

func (a *api) diagrams(w http.ResponseWriter, r *http.Request, userID string) {
	switch r.Method {
	case "GET":
		query := r.URL.Query()
		writeJSON(w, 200, orEmpty(a.s.listDiagrams(userID, diagramQuery{
			Favorite: query.Get("favorite") == "1",
			FolderID: query.Get("folderId"),
			Search:   query.Get("search"),
		})))
	case "POST":
		body, err := readBody(r)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		name := rawString(body["name"])
		if !isName(name) {
			writeError(w, 400, "name is required")
			return
		}
		state := rawString(body["state"])
		if !isName(state) {
			writeError(w, 400, "state is required")
			return
		}
		positions, ok := rawPositions(body["positions"])
		if !ok {
			writeError(w, 400, "positions must be an object of {dx, dy} number pairs")
			return
		}
		folder := rawParent(body["folderId"])
		if folder.Set && folder.Value != nil && a.s.getFolder(userID, *folder.Value) == nil {
			writeError(w, 404, "folder not found")
			return
		}
		writeJSON(w, 201, a.s.createDiagram(userID, diagramInput{
			FolderID:  folder.Value,
			Name:      strings.TrimSpace(*name),
			Positions: positions.Value,
			State:     *state,
		}))
	default:
		a.notFound(w, r)
	}
}

func (a *api) diagramItem(w http.ResponseWriter, r *http.Request, userID string) {
	id := strings.TrimPrefix(r.URL.Path, "/api/diagrams/")
	// Node's /([^/]+)/ pattern never matches an empty or nested id.
	if id == "" || strings.Contains(id, "/") {
		a.notFound(w, r)
		return
	}
	switch r.Method {
	case "GET":
		if d := a.s.getDiagram(userID, id); d != nil {
			writeJSON(w, 200, d)
		} else {
			writeError(w, 404, "diagram not found")
		}
	case "PATCH":
		if a.s.getDiagram(userID, id) == nil {
			writeError(w, 404, "diagram not found")
			return
		}
		body, err := readBody(r)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		name := rawString(body["name"])
		if len(body["name"]) > 0 && !isName(name) {
			writeError(w, 400, "name must be a non-empty string")
			return
		}
		state := rawString(body["state"])
		if len(body["state"]) > 0 && !isName(state) {
			writeError(w, 400, "state must be a non-empty string")
			return
		}
		positions, ok := rawPositions(body["positions"])
		if !ok {
			writeError(w, 400, "positions must be an object of {dx, dy} number pairs")
			return
		}
		folder := rawParent(body["folderId"])
		if folder.Set && folder.Value != nil && a.s.getFolder(userID, *folder.Value) == nil {
			writeError(w, 404, "folder not found")
			return
		}
		var namePtr, statePtr *string
		if name != nil {
			trimmed := strings.TrimSpace(*name)
			namePtr = &trimmed
		}
		if state != nil {
			statePtr = state
		}
		patched, err := a.s.updateDiagram(userID, id, diagramPatch{
			Favorite:  rawBool(body["favorite"]),
			FolderID:  folder,
			Name:      namePtr,
			Positions: positions,
			State:     statePtr,
		})
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, patched)
	case "DELETE":
		if a.s.deleteDiagram(userID, id) {
			writeJSON(w, 200, map[string]bool{"ok": true})
		} else {
			writeError(w, 404, "diagram not found")
		}
	default:
		a.notFound(w, r)
	}
}

func (a *api) export(w http.ResponseWriter, r *http.Request, userID string) {
	if r.Method != "GET" {
		a.notFound(w, r)
		return
	}
	writeJSON(w, 200, map[string]any{
		"diagrams":   orEmpty(a.s.listAllDiagrams(userID)),
		"exportedAt": now(),
		"folders":    orEmpty(a.s.listFolders(userID)),
	})
}

func (a *api) importEndpoint(w http.ResponseWriter, r *http.Request, userID string) {
	if r.Method != "POST" {
		a.notFound(w, r)
		return
	}
	body, err := readBody(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if !isJSONArray(body["folders"]) || !isJSONArray(body["diagrams"]) {
		writeError(w, 400, "folders and diagrams arrays are required")
		return
	}
	var foldersRaw, diagramsRaw []json.RawMessage
	if err := json.Unmarshal(body["folders"], &foldersRaw); err != nil {
		writeError(w, 400, "Invalid JSON body")
		return
	}
	if err := json.Unmarshal(body["diagrams"], &diagramsRaw); err != nil {
		writeError(w, 400, "Invalid JSON body")
		return
	}
	// Node's importAll reads elements as plain JS objects: a non-object element
	// or a non-string id/name(/state) is skipped, and other field-type quirks
	// (numeric favorite, non-object positions, ...) degrade instead of failing
	// the whole request.
	folders, skippedFolders := lenientFolders(foldersRaw)
	diagrams, skippedDiagrams := lenientDiagrams(diagramsRaw)
	result, err := a.s.importAll(userID, importPayload{Diagrams: diagrams, Folders: folders})
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	result.Skipped += skippedFolders + skippedDiagrams
	writeJSON(w, 200, result)
}

// jsonText extracts a JSON string field value; absent/null/non-string -> not ok.
func jsonText(v json.RawMessage) (string, bool) {
	if len(v) == 0 || string(v) == "null" {
		return "", false
	}
	var s string
	if json.Unmarshal(v, &s) != nil {
		return "", false
	}
	return s, true
}

// jsTruthy mirrors JS truthiness for JSON-decodable values: false, 0, "" and
// absent/null are falsy; everything else (including objects and arrays) is truthy.
func jsTruthy(v json.RawMessage) bool {
	if len(v) == 0 || string(v) == "null" {
		return false
	}
	var parsed any
	if json.Unmarshal(v, &parsed) != nil {
		return false
	}
	switch value := parsed.(type) {
	case bool:
		return value
	case float64:
		return value != 0
	case string:
		return value != ""
	default:
		return true
	}
}

// lenientFolders decodes imported folder elements like node's isNonEmpty gate:
// non-object elements and non-string (or blank) ids/names are skipped; other
// type quirks degrade (parentId/createdAt/updatedAt become null/absent).
func lenientFolders(items []json.RawMessage) ([]importFolder, int) {
	out := make([]importFolder, 0, len(items))
	skipped := 0
	for _, item := range items {
		var fields map[string]json.RawMessage
		if json.Unmarshal(item, &fields) != nil {
			skipped++
			continue
		}
		id, okID := jsonText(fields["id"])
		name, okName := jsonText(fields["name"])
		if !okID || !okName || strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" {
			skipped++
			continue
		}
		f := importFolder{ID: id, Name: name}
		if parent, ok := jsonText(fields["parentId"]); ok {
			f.ParentID = &parent
		}
		f.CreatedAt, _ = jsonText(fields["createdAt"])
		f.UpdatedAt, _ = jsonText(fields["updatedAt"])
		out = append(out, f)
	}
	return out, skipped
}

// lenientDiagrams decodes imported diagram elements like node's isNonEmpty gate:
// non-object elements and non-string (or blank) ids/names/states are skipped.
// favorite uses JS truthiness (numeric 1 is favorite), and non-object (or
// undecodable-offset) positions import as absent — observably identical to
// node, which stores the raw value but reads it back as {}.
func lenientDiagrams(items []json.RawMessage) ([]importDiagram, int) {
	out := make([]importDiagram, 0, len(items))
	skipped := 0
	for _, item := range items {
		var fields map[string]json.RawMessage
		if json.Unmarshal(item, &fields) != nil {
			skipped++
			continue
		}
		id, okID := jsonText(fields["id"])
		name, okName := jsonText(fields["name"])
		state, okState := jsonText(fields["state"])
		if !okID || !okName || !okState || strings.TrimSpace(id) == "" ||
			strings.TrimSpace(name) == "" || strings.TrimSpace(state) == "" {
			skipped++
			continue
		}
		d := importDiagram{ID: id, Name: name, State: state, Favorite: jsTruthy(fields["favorite"])}
		if folder, ok := jsonText(fields["folderId"]); ok {
			d.FolderID = &folder
		}
		var positions map[string]NodeOffset
		if json.Unmarshal(fields["positions"], &positions) == nil {
			d.Positions = positions
		}
		d.CreatedAt, _ = jsonText(fields["createdAt"])
		d.UpdatedAt, _ = jsonText(fields["updatedAt"])
		out = append(out, d)
	}
	return out, skipped
}
