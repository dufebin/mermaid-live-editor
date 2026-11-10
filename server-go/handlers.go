package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const maxBodyBytes = 5_000_000

type api struct{ s *store }

func newHandler(s *store) http.Handler {
	a := &api{s}
	mux := http.NewServeMux()
	// Method dispatch happens inside each handler: node 404s (never 405s) on mismatch.
	mux.HandleFunc("/api/folders", a.folders)
	mux.HandleFunc("/api/folders/", a.folderItem)
	mux.HandleFunc("/api/diagrams", a.diagrams)
	mux.HandleFunc("/api/diagrams/", a.diagramItem)
	mux.HandleFunc("/api/export", a.export)
	mux.HandleFunc("/api/import", a.importEndpoint)
	mux.HandleFunc("/", a.notFound)
	return mux
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

func (a *api) folders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		writeJSON(w, 200, orEmpty(a.s.listFolders()))
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
		if parent.Set && parent.Value != nil && a.s.getFolder(*parent.Value) == nil {
			writeError(w, 404, "parent folder not found")
			return
		}
		writeJSON(w, 201, a.s.createFolder(strings.TrimSpace(*name), parent.Value))
	default:
		a.notFound(w, r)
	}
}

func (a *api) folderItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/folders/")
	// Node's /([^/]+)/ pattern never matches an empty or nested id.
	if id == "" || strings.Contains(id, "/") {
		a.notFound(w, r)
		return
	}
	switch r.Method {
	case "PATCH":
		if a.s.getFolder(id) == nil {
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
			if *parent.Value == id || a.s.isSelfOrAncestor(id, *parent.Value) {
				writeError(w, 400, "cannot move a folder into itself or its descendant")
				return
			}
			if a.s.getFolder(*parent.Value) == nil {
				writeError(w, 404, "parent folder not found")
				return
			}
		}
		var namePtr *string
		if name != nil {
			trimmed := strings.TrimSpace(*name)
			namePtr = &trimmed
		}
		updated, err := a.s.updateFolder(id, namePtr, parent)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, updated)
	case "DELETE":
		if a.s.deleteFolder(id) {
			writeJSON(w, 200, map[string]bool{"ok": true})
		} else {
			writeError(w, 404, "folder not found")
		}
	default:
		a.notFound(w, r)
	}
}

func (a *api) diagrams(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		query := r.URL.Query()
		writeJSON(w, 200, orEmpty(a.s.listDiagrams(diagramQuery{
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
		if folder.Set && folder.Value != nil && a.s.getFolder(*folder.Value) == nil {
			writeError(w, 404, "folder not found")
			return
		}
		writeJSON(w, 201, a.s.createDiagram(diagramInput{
			FolderID:  folder.Value,
			Name:      strings.TrimSpace(*name),
			Positions: positions.Value,
			State:     *state,
		}))
	default:
		a.notFound(w, r)
	}
}

func (a *api) diagramItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/diagrams/")
	// Node's /([^/]+)/ pattern never matches an empty or nested id.
	if id == "" || strings.Contains(id, "/") {
		a.notFound(w, r)
		return
	}
	switch r.Method {
	case "GET":
		if d := a.s.getDiagram(id); d != nil {
			writeJSON(w, 200, d)
		} else {
			writeError(w, 404, "diagram not found")
		}
	case "PATCH":
		if a.s.getDiagram(id) == nil {
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
		if folder.Set && folder.Value != nil && a.s.getFolder(*folder.Value) == nil {
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
		patched, err := a.s.updateDiagram(id, diagramPatch{
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
		if a.s.deleteDiagram(id) {
			writeJSON(w, 200, map[string]bool{"ok": true})
		} else {
			writeError(w, 404, "diagram not found")
		}
	default:
		a.notFound(w, r)
	}
}

func (a *api) export(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		a.notFound(w, r)
		return
	}
	writeJSON(w, 200, map[string]any{
		"diagrams":   orEmpty(a.s.listAllDiagrams()),
		"exportedAt": now(),
		"folders":    orEmpty(a.s.listFolders()),
	})
}

func (a *api) importEndpoint(w http.ResponseWriter, r *http.Request) {
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
	raw, err := json.Marshal(body)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	var payload importPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		writeError(w, 400, "Invalid JSON body")
		return
	}
	result, err := a.s.importAll(payload)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, result)
}
