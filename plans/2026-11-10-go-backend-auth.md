# Go Backend + Auth + Per-User Data Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Node storage API with a behavior-identical Go service (Phase A), then add registration/login with per-user data isolation (Phase B).

**Architecture:** A standalone Go HTTP service (`server-go/`) speaking the exact same JSON API as the current `server/*.mjs` on port 18790, backed by the same SQLite file via `modernc.org/sqlite`. Auth is stateless JWT (HS256, hand-rolled on `crypto/hmac`) in an HttpOnly cookie; isolation is enforced by a mandatory `user_id` filter in every SQL query. Frontend adds `/login` + `/register` routes, a navbar user menu, and centralized 401 → login redirect.

**Tech Stack:** Go 1.26 (stdlib + `modernc.org/sqlite` + `golang.org/x/crypto/scrypt` only), SvelteKit 2 / Svelte 5 runes, vitest, Playwright.

**Spec:** `specs/2026-11-10-go-backend-auth-design.md`

## Global Constraints

- pnpm; Node >= 24; Go 1.26 available as `go`.
- Go server listens on `HOST` (default `127.0.0.1`) and `PORT` (default `18790`), data dir `DATA_DIR` (default `server-data`) — identical env contract to the Node server.
- Only Go dependencies allowed: `modernc.org/sqlite` and `golang.org/x/crypto`. No web frameworks, no JWT libraries, no ORM.
- Phase A is a **behavior-identical port**: same endpoints, same JSON shapes (camelCase), same status codes, same error message strings as `server/index.mjs` (the Node file is the authoritative spec until deleted in Task 4).
- JWT: HS256 only, cookie `mermaid_session`, `HttpOnly; SameSite=Lax; Path=/; Max-Age=604800`, no `Secure` (HTTP LAN deployment).
- Password: scrypt N=32768 r=8 p=1 keyLen=32, per-user random salt, stored `scrypt$N$r$p$saltB64$hashB64`. Email basic `user@host.tld` shape, lowercased. Password 8–128 chars.
- `/view` and `/embed` routes stay public (share links); `/edit` and `/dashboard` require login client-side (401 from API triggers redirect).
- `server-data/` is live user data — never commit it, never point tests at it. E2e isolation config uses `server-data/e2e` (wiped on start).
- ESLint alphabetical sort-keys for 5+ key objects in `src/`; working tree may contain unrelated uncommitted user WIP — stage only files your task names; never run repo-wide `pnpm lint:fix`.
- Every commit message ends with `Co-Authored-By: Claude Code <noreply@anthropic.com>`.
- Do not touch anything under `docs/` (build output).

---

### Task 1: Go module + store layer (port of db.mjs)

**Files:**

- Create: `server-go/go.mod`, `server-go/db.go`, `server-go/db_test.go`
- Create: `server-go/.gitignore` (one line: `bin/`)

**Interfaces:**

- Consumes: the SQL schema and row semantics of `server/db.mjs` (read it first — it is the porting spec).
- Produces (used by Tasks 2, 6, 7):
  - `type NodeOffset struct { Dx float64 `json:"dx"`; Dy float64 `json:"dy"` }`
  - `type Folder struct` / `type Diagram struct` with camelCase JSON tags matching the API (`id, name, parentId, createdAt, updatedAt` / `id, folderId, name, state, favorite, positions, createdAt, updatedAt`)
  - `type store struct` + `func openDB(dir string) (*store, error)` (mkdir dir, open `diagrams.db`, pragmas, schema, idempotent `positions` column migration)
  - Phase A methods (no user scoping yet): `listFolders() []Folder`, `listAllDiagrams() []Diagram`, `getFolder(id string) *Folder`, `isSelfOrAncestor(id, ancestorID string) bool`, `createFolder(name string, parentID *string) Folder`, `updateFolder(id string, name *string, parentID parentValue) (*Folder, error)`, `deleteFolder(id string) bool`, `listDiagrams(q diagramQuery) []Diagram`, `getDiagram(id string) *Diagram`, `createDiagram(d diagramInput) Diagram`, `updateDiagram(id string, p diagramPatch) (*Diagram, error)`, `deleteDiagram(id string) bool`, `importAll(payload importPayload) (importResult, error)`
  - `parentValue{Set bool; Value *string}` — tri-state for `parentId` (absent = keep, null = move to root, value = set)
  - `positionsValue{Set bool; Value map[string]NodeOffset}` — tri-state for `positions` (absent = keep, null = clear, object = set)

- [ ] **Step 1: Init module and get dependencies**

```bash
cd server-go
go mod init mermaid-live-editor/storage
go get modernc.org/sqlite golang.org/x/crypto/scrypt
printf 'bin/\n' > .gitignore
```

- [ ] **Step 2: Write the failing tests**

Create `server-go/db_test.go` (uses a fresh temp dir per test; no `server-data/`):

```go
package main

import (
	"testing"
)

func newTestStore(t *testing.T) *store {
	t.Helper()
	s, err := openDB(t.TempDir())
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func strPtr(s string) *string { return &s }

func TestFolderCRUD(t *testing.T) {
	s := newTestStore(t)
	parent := s.createFolder("Work", nil)
	if parent.Name != "Work" || parent.ParentID != nil {
		t.Fatalf("unexpected parent: %+v", parent)
	}
	child := s.createFolder("Sub", &parent.ID)
	if *child.ParentID != parent.ID {
		t.Fatalf("child parent: %+v", child)
	}
	// rename, keep parent
	updated, err := s.updateFolder(child.ID, strPtr("Renamed"), parentValue{})
	if err != nil || updated.Name != "Renamed" || *updated.ParentID != parent.ID {
		t.Fatalf("update: %+v err=%v", updated, err)
	}
	// move to root via explicit null
	updated, err = s.updateFolder(child.ID, nil, parentValue{Set: true})
	if err != nil || updated.ParentID != nil {
		t.Fatalf("move to root: %+v err=%v", updated, err)
	}
	if !s.deleteFolder(child.ID) || s.getFolder(child.ID) != nil {
		t.Fatal("delete folder")
	}
}

func TestFolderDeleteCascadesDiagramsToNull(t *testing.T) {
	s := newTestStore(t)
	f := s.createFolder("F", nil)
	d := s.createDiagram(diagramInput{Name: "D", State: "state", FolderID: &f.ID})
	if !s.deleteFolder(f.ID) {
		t.Fatal("delete")
	}
	got := s.getDiagram(d.ID)
	if got == nil || got.FolderID != nil {
		t.Fatalf("diagram should survive with null folder: %+v", got)
	}
}

func TestDiagramCRUDAndPositions(t *testing.T) {
	s := newTestStore(t)
	d := s.createDiagram(diagramInput{Name: "D", State: "s"})
	if d.Favorite || len(d.Positions) != 0 {
		t.Fatalf("defaults: %+v", d)
	}
	// set positions
	patched, err := s.updateDiagram(d.ID, diagramPatch{Positions: positionsValue{
		Set:   true,
		Value: map[string]NodeOffset{"A": {Dx: 1.5, Dy: -2}},
	}})
	if err != nil || patched.Positions["A"].Dx != 1.5 {
		t.Fatalf("patch positions: %+v err=%v", patched, err)
	}
	// clear positions via explicit null
	patched, err = s.updateDiagram(d.ID, diagramPatch{Positions: positionsValue{Set: true}})
	if err != nil || len(patched.Positions) != 0 {
		t.Fatalf("clear positions: %+v err=%v", patched, err)
	}
	// favorite + name
	fav := true
	patched, err = s.updateDiagram(d.ID, diagramPatch{Favorite: &fav, Name: strPtr("N")})
	if err != nil || !patched.Favorite || patched.Name != "N" {
		t.Fatalf("patch favorite/name: %+v err=%v", patched, err)
	}
}

func TestListDiagramsFilters(t *testing.T) {
	s := newTestStore(t)
	f := s.createFolder("F", nil)
	a := s.createDiagram(diagramInput{Name: "alpha", State: "s"})
	b := s.createDiagram(diagramInput{Name: "beta", State: "s", FolderID: &f.ID})
	fav := true
	s.updateDiagram(b.ID, diagramPatch{Favorite: &fav})
	if got := s.listDiagrams(diagramQuery{}); len(got) != 2 {
		t.Fatalf("list all: %d", len(got))
	}
	if got := s.listDiagrams(diagramQuery{FolderID: "root"}); len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("root filter: %+v", got)
	}
	if got := s.listDiagrams(diagramQuery{FolderID: f.ID}); len(got) != 1 || got[0].ID != b.ID {
		t.Fatalf("folder filter: %+v", got)
	}
	if got := s.listDiagrams(diagramQuery{Favorite: true}); len(got) != 1 || got[0].ID != b.ID {
		t.Fatalf("favorite filter: %+v", got)
	}
	if got := s.listDiagrams(diagramQuery{Search: "alp"}); len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("search filter: %+v", got)
	}
}

func TestIsSelfOrAncestor(t *testing.T) {
	s := newTestStore(t)
	root := s.createFolder("root", nil)
	mid := s.createFolder("mid", &root.ID)
	if !s.isSelfOrAncestor(root.ID, mid.ID) {
		t.Fatal("root is ancestor of mid")
	}
	if s.isSelfOrAncestor(mid.ID, root.ID) {
		t.Fatal("mid is not ancestor of root")
	}
}

func TestImportAll(t *testing.T) {
	s := newTestStore(t)
	res, err := s.importAll(importPayload{
		Folders: []importFolder{{ID: "f1", Name: "F1", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}},
		Diagrams: []importDiagram{
			{ID: "d1", Name: "D1", State: "s", FolderID: strPtr("f1")},
			{ID: "d1", Name: "dup", State: "s"},
			{ID: "", Name: "bad", State: "s"},
			{ID: "d2", Name: "no-state", State: ""},
		},
	})
	if err != nil || res.Imported != 2 || res.Skipped != 3 {
		t.Fatalf("import: %+v err=%v", res, err)
	}
	if s.getDiagram("d1") == nil || *s.getDiagram("d1").FolderID != "f1" {
		t.Fatal("imported diagram missing or wrong folder")
	}
}

func TestOpenDBIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	s1, err := openDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	s1.createFolder("F", nil)
	s1.Close()
	s2, err := openDB(dir) // second open: migrations must no-op, data persists
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if len(s2.listFolders()) != 1 {
		t.Fatal("data lost on reopen")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd server-go && go test ./...`
Expected: FAIL — `db.go` does not exist (undefined: openDB etc.).

- [ ] **Step 4: Implement `server-go/db.go`**

Port `server/db.mjs` faithfully. Complete file:

```go
package main

import (
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type NodeOffset struct {
	Dx float64 `json:"dx"`
	Dy float64 `json:"dy"`
}

type Folder struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	ParentID  *string `json:"parentId"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

type Diagram struct {
	ID        string                `json:"id"`
	FolderID  *string               `json:"folderId"`
	Name      string                `json:"name"`
	State     string                `json:"state"`
	Favorite  bool                  `json:"favorite"`
	Positions map[string]NodeOffset `json:"positions"`
	CreatedAt string                `json:"createdAt"`
	UpdatedAt string                `json:"updatedAt"`
}

// parentValue is the tri-state for parentId patches:
// Set=false -> keep current; Set=true,Value=nil -> move to root; Set=true,Value=&s -> set.
type parentValue struct {
	Set   bool
	Value *string
}

// positionsValue: Set=false -> keep; Set=true,Value=nil -> clear; Set=true,Value=m -> set.
type positionsValue struct {
	Set   bool
	Value map[string]NodeOffset
}

type diagramQuery struct {
	Favorite bool
	FolderID string // "" = all, "root" = uncategorized, other = that folder
	Search   string
}

type diagramInput struct {
	FolderID  *string
	Name      string
	State     string
	Positions map[string]NodeOffset
}

type diagramPatch struct {
	Favorite  *bool
	FolderID  parentValue
	Name      *string
	Positions positionsValue
	State     *string
}

type importFolder struct {
	CreatedAt string  `json:"createdAt"`
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	ParentID  *string `json:"parentId"`
	UpdatedAt string  `json:"updatedAt"`
}

type importDiagram struct {
	CreatedAt string                `json:"createdAt"`
	Favorite  bool                  `json:"favorite"`
	FolderID  *string               `json:"folderId"`
	ID        string                `json:"id"`
	Name      string                `json:"name"`
	Positions map[string]NodeOffset `json:"positions"`
	State     string                `json:"state"`
	UpdatedAt string                `json:"updatedAt"`
}

type importPayload struct {
	Diagrams []importDiagram `json:"diagrams"`
	Folders  []importFolder  `json:"folders"`
}

type importResult struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

type store struct {
	db *sql.DB
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func openDB(dir string) (*store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	d, err := sql.Open("sqlite", dir+"/diagrams.db")
	if err != nil {
		return nil, err
	}
	// Mirror node:sqlite's synchronous execution: one connection, no SQLITE_BUSY.
	d.SetMaxOpenConns(1)
	s := &store{db: d}
	for _, pragma := range []string{"PRAGMA journal_mode = WAL", "PRAGMA foreign_keys = ON", "PRAGMA busy_timeout = 5000"} {
		if _, err := d.Exec(pragma); err != nil {
			return nil, err
		}
	}
	schema := `
CREATE TABLE IF NOT EXISTS folders (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  parent_id TEXT REFERENCES folders(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS diagrams (
  id TEXT PRIMARY KEY,
  folder_id TEXT REFERENCES folders(id) ON DELETE SET NULL,
  name TEXT NOT NULL,
  state TEXT NOT NULL,
  favorite INTEGER NOT NULL DEFAULT 0,
  positions TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_diagrams_folder ON diagrams(folder_id);
CREATE INDEX IF NOT EXISTS idx_diagrams_favorite ON diagrams(favorite);`
	if _, err := d.Exec(schema); err != nil {
		return nil, err
	}
	// Idempotent migration (precedent: positions column in db.mjs).
	if _, err := d.Exec("ALTER TABLE diagrams ADD COLUMN positions TEXT"); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return nil, err
		}
	}
	return s, nil
}

func (s *store) Close() error { return s.db.Close() }

func (s *store) listFolders() []Folder {
	rows, err := s.db.Query("SELECT id, name, parent_id, created_at, updated_at FROM folders ORDER BY name")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Folder
	for rows.Next() {
		f, err := scanFolder(rows)
		if err != nil {
			return nil
		}
		out = append(out, f)
	}
	return out
}

func (s *store) listAllDiagrams() []Diagram {
	// NO ORDER BY: db.mjs's bare SELECT returns natural (rowid) order — parity.
	rows, err := s.db.Query(diagramSelect())
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Diagram
	for rows.Next() {
		d, err := scanDiagram(rows)
		if err != nil {
			return nil
		}
		out = append(out, d)
	}
	return out
}

func (s *store) getFolder(id string) *Folder {
	row := s.db.QueryRow("SELECT id, name, parent_id, created_at, updated_at FROM folders WHERE id = ?", id)
	f, err := scanFolder(row)
	if err != nil {
		return nil
	}
	return &f
}

func (s *store) isSelfOrAncestor(id, ancestorID string) bool {
	current := s.getFolder(ancestorID)
	for current != nil {
		if current.ID == id {
			return true
		}
		if current.ParentID == nil {
			return false
		}
		current = s.getFolder(*current.ParentID)
	}
	return false
}

func (s *store) createFolder(name string, parentID *string) Folder {
	id := newUUID()
	ts := now()
	s.db.Exec("INSERT INTO folders (id, name, parent_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		id, name, parentID, ts, ts)
	return *s.getFolder(id)
}

func (s *store) updateFolder(id string, name *string, parentID parentValue) *Folder {
	f := s.getFolder(id)
	if f == nil {
		return nil
	}
	nextName, nextParent := f.Name, f.ParentID
	if name != nil {
		nextName = *name
	}
	if parentID.Set {
		nextParent = parentID.Value
	}
	s.db.Exec("UPDATE folders SET name = ?, parent_id = ?, updated_at = ? WHERE id = ?",
		nextName, nextParent, now(), id)
	return s.getFolder(id)
}

func (s *store) deleteFolder(id string) bool {
	res, err := s.db.Exec("DELETE FROM folders WHERE id = ?", id)
	return err == nil && res != nil && rowsAffected(res) > 0
}

func (s *store) listDiagrams(q diagramQuery) []Diagram {
	where, params := []string{}, []any{}
	if q.FolderID == "root" {
		where = append(where, "folder_id IS NULL")
	} else if q.FolderID != "" {
		where = append(where, "folder_id = ?")
		params = append(params, q.FolderID)
	}
	if q.Search != "" {
		where = append(where, "name LIKE ?")
		params = append(params, "%"+q.Search+"%")
	}
	if q.Favorite {
		where = append(where, "favorite = 1")
	}
	query := diagramSelect()
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY updated_at DESC"
	rows, err := s.db.Query(query, params...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Diagram
	for rows.Next() {
		d, err := scanDiagram(rows)
		if err != nil {
			return nil
		}
		out = append(out, d)
	}
	return out
}

func (s *store) getDiagram(id string) *Diagram {
	row := s.db.QueryRow(diagramSelect()+" WHERE id = ?", id)
	d, err := scanDiagram(row)
	if err != nil {
		return nil
	}
	return &d
}

func (s *store) createDiagram(in diagramInput) Diagram {
	id := newUUID()
	ts := now()
	s.db.Exec("INSERT INTO diagrams (id, folder_id, name, state, favorite, positions, created_at, updated_at) VALUES (?, ?, ?, ?, 0, ?, ?, ?)",
		id, in.FolderID, in.Name, in.State, positionsToJSON(in.Positions), ts, ts)
	return *s.getDiagram(id)
}

func (s *store) updateDiagram(id string, p diagramPatch) *Diagram {
	d := s.getDiagram(id)
	if d == nil {
		return nil
	}
	name, folder, state, favorite, positions := d.Name, d.FolderID, d.State, d.Favorite, d.Positions
	if p.Name != nil {
		name = *p.Name
	}
	if p.FolderID.Set {
		folder = p.FolderID.Value
	}
	if p.State != nil {
		state = *p.State
	}
	if p.Favorite != nil {
		favorite = *p.Favorite
	}
	if p.Positions.Set {
		positions = p.Positions.Value
	}
	s.db.Exec("UPDATE diagrams SET name = ?, folder_id = ?, state = ?, favorite = ?, positions = ?, updated_at = ? WHERE id = ?",
		name, folder, state, favorite, positionsToJSON(positions), now(), id)
	return s.getDiagram(id)
}

func (s *store) deleteDiagram(id string) bool {
	res, err := s.db.Exec("DELETE FROM diagrams WHERE id = ?", id)
	return err == nil && res != nil && rowsAffected(res) > 0
}

func (s *store) importAll(payload importPayload) (importResult, error) {
	var result importResult
	tx, err := s.db.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	// Defer FK checks to COMMIT so folders sort before their parents on import.
	if _, err := tx.Exec("PRAGMA defer_foreign_keys = ON"); err != nil {
		return result, err
	}
	insertFolder, _ := tx.Prepare("INSERT OR IGNORE INTO folders (id, name, parent_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)")
	for _, f := range payload.Folders {
		if strings.TrimSpace(f.ID) == "" || strings.TrimSpace(f.Name) == "" {
			result.Skipped++
			continue
		}
		createdAt, updatedAt := orNow(f.CreatedAt), orNow(f.UpdatedAt)
		res, err := insertFolder.Exec(f.ID, f.Name, f.ParentID, createdAt, updatedAt)
		if err == nil && rowsAffected(res) > 0 {
			result.Imported++
		} else if err == nil {
			result.Skipped++
		}
	}
	insertDiagram, _ := tx.Prepare("INSERT OR IGNORE INTO diagrams (id, folder_id, name, state, favorite, positions, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)")
	for _, d := range payload.Diagrams {
		if strings.TrimSpace(d.ID) == "" || strings.TrimSpace(d.Name) == "" || strings.TrimSpace(d.State) == "" {
			result.Skipped++
			continue
		}
		var positions any
		if d.Positions != nil {
			positions = positionsToJSON(d.Positions)
		}
		res, err := insertDiagram.Exec(d.ID, d.FolderID, d.Name, d.State, d.Favorite, positions, orNow(d.CreatedAt), orNow(d.UpdatedAt))
		if err == nil && rowsAffected(res) > 0 {
			result.Imported++
		} else if err == nil {
			result.Skipped++
		}
	}
	return result, tx.Commit()
}

// --- scan / convert helpers ---

func diagramSelect() string {
	return "SELECT id, folder_id, name, state, favorite, positions, created_at, updated_at FROM diagrams"
}

func scanFolder(row interface{ Scan(...any) error }) (Folder, error) {
	var f Folder
	err := row.Scan(&f.ID, &f.Name, &f.ParentID, &f.CreatedAt, &f.UpdatedAt)
	return f, err
}

func scanDiagram(row interface{ Scan(...any) error }) (Diagram, error) {
	var d Diagram
	var positions sql.NullString
	err := row.Scan(&d.ID, &d.FolderID, &d.Name, &d.State, &d.Favorite, &positions, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return d, err
	}
	d.Positions = jsonToPositions(positions.String)
	return d, nil
}

// positionsToJSON: nil map -> NULL; otherwise JSON string.
func positionsToJSON(p map[string]NodeOffset) any {
	if p == nil {
		return nil
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil
	}
	return string(raw)
}

// jsonToPositions: NULL/empty/corrupt JSON -> empty map (serialized as {} — node parity).
func jsonToPositions(value string) map[string]NodeOffset {
	if value == "" {
		return map[string]NodeOffset{}
	}
	var p map[string]NodeOffset
	if err := json.Unmarshal([]byte(value), &p); err != nil || p == nil {
		return map[string]NodeOffset{}
	}
	return p
}

func orNow(ts string) string {
	if ts == "" {
		return now()
	}
	return ts
}

func rowsAffected(res sql.Result) int64 {
	n, _ := res.RowsAffected()
	return n
}

// newUUID returns a random RFC-4122-shaped v4 UUID string.
func newUUID() string {
	var b [16]byte
	readRandom(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	const hex = "0123456789abcdef"
	out := make([]byte, 0, 36)
	for i, v := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out = append(out, '-')
		}
		out = append(out, hex[v>>4], hex[v&0x0f])
	}
	return string(out)
}

func readRandom(b []byte) {
	if _, err := cryptorand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on Linux; fail loudly if it does
	}
}
```

(Add the `os` import for `MkdirAll`.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd server-go && go test ./... && go vet ./...`
Expected: PASS (all db tests).

- [ ] **Step 6: Commit**

```bash
git add server-go/
git commit -m "feat: port storage db layer to Go

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 2: HTTP handlers + routing (port of index.mjs)

**Files:**

- Create: `server-go/main.go`, `server-go/handlers.go`, `server-go/handlers_test.go`

**Interfaces:**

- Consumes: Task 1 store methods and types.
- Produces:
  - `func newHandler(s *store) http.Handler` — the complete Phase A API (Task 3 parity run, Task 4 cutover; Task 7 widens the signature to `newHandler(s *store, secret []byte)`)
  - Handler semantics must match `server/index.mjs` exactly. The parity matrix (all of it):
    - unknown path OR wrong method → `404 {"error":"Not found"}` (node never 405s)
    - JSON body parse failure → `400 {"error":"Invalid JSON body"}`
    - body > 5 MB → `400 {"error":"Payload too large"}`
    - any handler error → `400 {"error":"<message>"}`
    - `GET /api/folders` → `200 [folder...]`
    - `POST /api/folders`: missing name → `400 "name is required"`; unknown parentId → `404 "parent folder not found"`; ok → `201 folder` (name trimmed)
    - `PATCH /api/folders/{id}`: unknown id → `404 "folder not found"`; empty name → `400 "name must be a non-empty string"`; parentId == id or descendant → `400 "cannot move a folder into itself or its descendant"`; unknown parent → `404 "parent folder not found"`; ok → `200 folder` (name trimmed when present)
    - `DELETE /api/folders/{id}` → `200 {"ok":true}` or `404 "folder not found"`
    - `GET /api/diagrams?folderId=&search=&favorite=1` → `200 [diagram...]` (folderId `root` → uncategorized)
    - `POST /api/diagrams`: `400 "name is required"` / `400 "state is required"` / `400 "positions must be an object of {dx, dy} number pairs"` / `404 "folder not found"`; ok → `201 diagram` (name trimmed)
    - `GET /api/diagrams/{id}` → `200 diagram` or `404 "diagram not found"`
    - `PATCH /api/diagrams/{id}`: `404 "diagram not found"` first, then `400 "name must be a non-empty string"` / `400 "state must be a non-empty string"` / `400 "positions must be an object of {dx, dy} number pairs"` / `404 "folder not found"`; ok → `200 diagram`
    - `DELETE /api/diagrams/{id}` → `200 {"ok":true}` or `404 "diagram not found"`
    - `GET /api/export` → `200 {"diagrams":[...],"exportedAt":"<ISO>","folders":[...]}`
    - `POST /api/import`: non-arrays → `400 "folders and diagrams arrays are required"`; ok → `200 {"imported":N,"skipped":N}`
  - `positions` always serializes as `{}` (never null) — Task 1's `jsonToPositions` guarantees it.

- [ ] **Step 1: Write the failing tests**

`server-go/handlers_test.go` — a `doReq(t, h, method, path, body string) (int, map[string]any)` helper driving `newHandler(newTestStore(t))` via `httptest.NewRecorder`, then one test function per matrix group (`TestRoutingAndBody`, `TestFolderEndpoints`, `TestDiagramEndpoints`, `TestExportImport`), asserting **every row** of the matrix above. Representative assertions:

```go
func TestRoutingAndBody(t *testing.T) {
	h := newHandler(newTestStore(t))
	if code, body := doReq(t, h, "GET", "/api/nope", ""); code != 404 || body["error"] != "Not found" {
		t.Fatalf("unknown path: %d %v", code, body)
	}
	if code, _ := doReq(t, h, "PUT", "/api/folders", "{}"); code != 404 {
		t.Fatalf("wrong method must 404 like node (no 405): %d", code)
	}
	if code, body := doReq(t, h, "POST", "/api/folders", "{bad json"); code != 400 || body["error"] != "Invalid JSON body" {
		t.Fatalf("bad json: %d %v", code, body)
	}
}

func TestFolderEndpoints(t *testing.T) {
	h := newHandler(newTestStore(t))
	if code, body := doReq(t, h, "POST", "/api/folders", `{"name":"  "}`); code != 400 || body["error"] != "name is required" {
		t.Fatalf("folder name: %d %v", code, body)
	}
	code, folder := doReq(t, h, "POST", "/api/folders", `{"name":" F "}`)
	if code != 201 || folder["name"] != "F" || folder["parentId"] != nil {
		t.Fatalf("create folder: %d %v", code, folder)
	}
	fid := folder["id"].(string)
	if code, body := doReq(t, h, "PATCH", "/api/folders/"+fid, `{"parentId":"`+fid+`"}`); code != 400 || body["error"] != "cannot move a folder into itself or its descendant" {
		t.Fatalf("cycle: %d %v", code, body)
	}
	if code, body := doReq(t, h, "POST", "/api/folders", `{"name":"X","parentId":"missing"}`); code != 404 || body["error"] != "parent folder not found" {
		t.Fatalf("bad parent: %d %v", code, body)
	}
}
```

(Write the full four test functions in this style; the two snippets are the required pattern, and every remaining matrix row gets an equivalent assertion — create diagram with positions roundtrip, PATCH with `positions:null` → `positions == {}`, all diagram validation messages, export shape, import validation.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server-go && go test ./...` — Expected: FAIL, `newHandler` undefined.

- [ ] **Step 3: Implement `server-go/handlers.go` and `server-go/main.go`**

`handlers.go`:

```go
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
	// Method dispatch inside each handler: node 404s (never 405s) on mismatch.
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

// readBody: absent body -> empty map; oversized -> errPayloadTooLarge.
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

func rawString(v json.RawMessage) *string { // absent OR null -> nil
	if len(v) == 0 || string(v) == "null" {
		return nil
	}
	var s string
	if json.Unmarshal(v, &s) != nil {
		return nil
	}
	return &s
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
```

Then the six route methods, porting `server/index.mjs` lines 95-271 with the exact validation order from the matrix. Full `folders` + `folderItem` as the pattern; `diagrams`, `diagramItem`, `export`, `importEndpoint` follow identically:

```go
func (a *api) folders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		writeJSON(w, 200, a.s.listFolders())
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
		writeJSON(w, 200, a.s.updateFolder(id, namePtr, parent))
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
```

`diagrams` GET parses `r.URL.Query()`: `favorite == "1"`, `folderId`, `search`. `diagramItem` GET/PATCH/DELETE; PATCH checks 404 first, then name ("name must be a non-empty string"), state ("state must be a non-empty string"), positions (400 matrix message when `rawPositions` returns ok=false), folderId 404. `export` returns `map[string]any{"diagrams": s.listAllDiagrams(), "exportedAt": now(), "folders": s.listFolders()}`. `importEndpoint` decodes into `importPayload` (strict: non-arrays → `400 "folders and diagrams arrays are required"`).

`main.go`:

```go
package main

import (
	"log"
	"net/http"
	"os"
)

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	host, port, dataDir := envOr("HOST", "127.0.0.1"), envOr("PORT", "18790"), envOr("DATA_DIR", "server-data")
	s, err := openDB(dataDir)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	log.Printf("storage API listening on http://%s:%s", host, port)
	log.Fatal(http.ListenAndServe(host+":"+port, recoverWrap(newHandler(s))))
}

// recoverWrap mirrors node's try/catch -> 400 with the error message.
func recoverWrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if err, ok := rec.(error); ok {
					writeError(w, 400, err.Error())
					return
				}
				writeError(w, 400, "Bad request")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 4: Run tests**

Run: `cd server-go && go test ./... && go vet ./...` — Expected: PASS, full matrix green.

- [ ] **Step 5: Commit**

```bash
git add server-go/
git commit -m "feat: port storage HTTP API to Go with parity tests

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 3: Differential parity harness (Node vs Go)

The existing e2e suite never touches the storage API, so Phase A's real acceptance is a differential test: replay an identical corpus against the Node server and the Go server (fresh DATA_DIRs) and diff every response.

**Files:**

- Create: `scripts/parity-check.mjs`
- Modify: `package.json` (one script)

**Interfaces:**

- Consumes: `server/index.mjs` (port **28790** — as-built: 18790 is occupied by the live systemd `mermaid-storage` service and must not be touched, `DATA_DIR=server-data/parity-node`), Go binary (port **28791**, `DATA_DIR=server-data/parity-go`). As-built harness fixes vs the plan's code: refs are captured from the RAW (pre-normalization) response bodies — capturing from normalized bodies would make every templated request 404 identically on both servers (false parity); corpus order keeps all `{{...}}` sources (GET list responses) ahead of their uses, and an unresolved template now throws instead of silently expanding to `''`.
- Produces: `pnpm parity` — exit 0 iff all responses match (UUIDs and ISO timestamps normalized).

- [ ] **Step 1: Write the harness**

`scripts/parity-check.mjs` (complete):

```js
import { execFileSync, spawn } from 'node:child_process';
import { mkdirSync, rmSync } from 'node:fs';

const ROOT = new URL('..', import.meta.url).pathname;
const UUID_RE = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/g;
const ISO_RE = /\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z/g;
const normalize = (text) => text.replaceAll(UUID_RE, '<uuid>').replaceAll(ISO_RE, '<ts>');

const startServer = (name, command, args, env, port) =>
  new Promise((resolve, reject) => {
    const child = spawn(command, args, { cwd: ROOT, env: { ...process.env, ...env } });
    const listener = (buffer) => {
      if (String(buffer).includes(`:${port}`)) {
        child.stdout.off('data', listener);
        child.stderr.off('data', listener);
        resolve(child);
      }
    };
    child.stdout.on('data', listener);
    child.stderr.on('data', listener);
    child.on('exit', (code) => reject(new Error(`${name} exited early: ${code}`)));
  });

const expand = (text, refs) =>
  text.replaceAll(/\{\{([^}]+)\}\}/g, (_, expr) => {
    const [key, index, field] = expr.split('.');
    return String(refs[key]?.[Number(index)]?.[field] ?? '');
  });

// Structural comparison: Node and Go marshal JSON with different key orders
// (object insertion order vs struct order) and different positions key order
// (insertion vs sorted), so raw-text diffing would false-positive. Parse both
// bodies, canonicalize (recursively sort object keys), THEN compare strings.
const canonicalize = (value) => {
  if (Array.isArray(value)) {
    return value.map(canonicalize);
  }
  if (value !== null && typeof value === 'object') {
    return Object.fromEntries(
      Object.keys(value)
        .sort()
        .map((key) => [key, canonicalize(value[key])])
    );
  }
  return value;
};

const request = async (port, method, path, body, refs) => {
  const response = await fetch(`http://127.0.0.1:${port}${expand(path, refs)}`, {
    body: body === undefined ? undefined : expand(JSON.stringify(body), refs),
    headers: { 'Content-Type': 'application/json' },
    method
  });
  const text = await response.text();
  let comparable;
  try {
    comparable = normalize(JSON.stringify(canonicalize(JSON.parse(text))));
  } catch {
    comparable = normalize(text); // non-JSON bodies: raw text, normalized
  }
  return { body: comparable, status: response.status };
};

// Corpus: every route + every validation branch. {{folders.0.id}} etc. come
// from each server's own previous list responses.
const corpus = [
  ['POST', '/api/folders', { name: '  Work  ' }],
  ['POST', '/api/folders', { name: 'Sub', parentId: '{{folders.0.id}}' }],
  ['GET', '/api/folders'],
  ['POST', '/api/folders', { name: '' }],
  ['POST', '/api/folders', { name: 'X', parentId: 'nope' }],
  ['PATCH', '/api/folders/{{folders.0.id}}', { name: 'Renamed' }],
  ['PATCH', '/api/folders/{{folders.1.id}}', { parentId: '{{folders.1.id}}' }],
  ['PATCH', '/api/folders/{{folders.1.id}}', { parentId: null }],
  ['PATCH', '/api/folders/missing', { name: 'Y' }],
  [
    'POST',
    '/api/diagrams',
    { name: ' alpha', state: 'code-1', positions: { A: { dx: 1, dy: 2 } } }
  ],
  ['POST', '/api/diagrams', { name: 'beta', state: 'code-2', folderId: '{{folders.0.id}}' }],
  ['POST', '/api/diagrams', { name: 'gamma', state: 'code-3' }],
  ['GET', '/api/diagrams'],
  ['POST', '/api/diagrams', { state: 'x' }],
  ['POST', '/api/diagrams', { name: 'x' }],
  ['POST', '/api/diagrams', { name: 'x', state: 'y', positions: [1] }],
  ['GET', '/api/diagrams?folderId=root'],
  ['GET', '/api/diagrams?folderId={{folders.0.id}}'],
  ['GET', '/api/diagrams?favorite=1'],
  ['GET', '/api/diagrams?search=alp'],
  ['GET', '/api/diagrams/{{diagrams.0.id}}'],
  ['GET', '/api/diagrams/missing'],
  ['PATCH', '/api/diagrams/{{diagrams.0.id}}', { favorite: true, positions: null }],
  ['PATCH', '/api/diagrams/{{diagrams.0.id}}', { name: '' }],
  ['PATCH', '/api/diagrams/{{diagrams.0.id}}', { state: '' }],
  ['PATCH', '/api/diagrams/missing', { name: 'z' }],
  ['DELETE', '/api/diagrams/{{diagrams.2.id}}'],
  ['DELETE', '/api/diagrams/missing'],
  ['DELETE', '/api/folders/{{folders.0.id}}'],
  ['GET', '/api/export'],
  ['POST', '/api/import', { folders: {}, diagrams: [] }],
  [
    'POST',
    '/api/import',
    { folders: [{ id: 'if1', name: 'Imp' }], diagrams: [{ id: 'id1', name: 'D', state: 's' }] }
  ],
  ['POST', '/api/import', { folders: [{ id: 'if1', name: 'dup' }], diagrams: [] }],
  ['GET', '/api/nope'],
  ['PUT', '/api/folders', {}]
];

const run = async (port) => {
  const refs = { diagrams: [], folders: [] };
  const out = [];
  for (const [method, path, body] of corpus) {
    const response = await request(port, method, path, body, refs);
    out.push(`${method} ${path} -> ${response.status} ${response.body}`);
    let parsed;
    try {
      parsed = JSON.parse(response.body);
    } catch {
      continue;
    }
    if (Array.isArray(parsed) && method === 'GET' && path === '/api/folders') {
      refs.folders = parsed;
    }
    if (Array.isArray(parsed) && method === 'GET' && path === '/api/diagrams') {
      refs.diagrams = parsed;
    }
  }
  return out;
};

const main = async () => {
  for (const dir of ['server-data/parity-node', 'server-data/parity-go']) {
    rmSync(`${ROOT}${dir}`, { recursive: true, force: true });
    mkdirSync(`${ROOT}${dir}`, { recursive: true });
  }
  execFileSync('go', ['build', '-o', 'bin/mermaid-storage', '.'], { cwd: `${ROOT}server-go` });
  const node = await startServer(
    'node',
    'node',
    ['server/index.mjs'],
    { DATA_DIR: 'server-data/parity-node', PORT: '18790' },
    18790
  );
  const go = await startServer(
    'go',
    'server-go/bin/mermaid-storage',
    [],
    { DATA_DIR: 'server-data/parity-go', PORT: '18791' },
    18791
  );
  try {
    const [nodeOut, goOut] = await Promise.all([run(18790), run(18791)]);
    let diffs = 0;
    for (let i = 0; i < nodeOut.length; i++) {
      if (nodeOut[i] !== goOut[i]) {
        diffs++;
        console.log(
          `DIFF at corpus[${i}] ${corpus[i][0]} ${corpus[i][1]}:\n  node: ${nodeOut[i]}\n  go:   ${goOut[i]}`
        );
      }
    }
    console.log(
      diffs === 0 ? `parity OK: ${nodeOut.length} responses identical` : `${diffs} diffs`
    );
    process.exitCode = diffs === 0 ? 0 : 1;
  } finally {
    node.kill();
    go.kill();
    rmSync(`${ROOT}server-data/parity-node`, { recursive: true, force: true });
    rmSync(`${ROOT}server-data/parity-go`, { recursive: true, force: true });
  }
};

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
```

Note: `refs` are captured from the GET list calls (corpus indices 2 and 13) which precede every templated use; verify while running that no `{{...}}` expands empty (an empty expansion 404s on BOTH servers — diff still passes but hides the branch; fix the index if seen).

- [ ] **Step 2: npm script** — `package.json` scripts: `"parity": "node scripts/parity-check.mjs"`.

- [ ] **Step 3: Run and fix diffs**

Run: `pnpm parity`
Expected: `parity OK: 35 responses identical`. Any DIFF line names a divergence — fix the Go side (Node is the spec) and re-run until zero diffs.

- [ ] **Step 4: Commit**

```bash
git add scripts/parity-check.mjs package.json server-go/
git commit -m "test: add node-vs-go differential parity harness for storage API

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 4: Cutover to the Go server

**Files:**

- Create: `server-go/Makefile`
- Modify: `package.json`, `systemd/mermaid-storage.service`, `README-LOCAL.md`
- Delete: `server/index.mjs`, `server/db.mjs`, `scripts/parity-check.mjs`, `"parity"` script

**Interfaces:**

- Consumes: Tasks 1-3.
- Produces: production storage = Go binary; `pnpm server` runs it; systemd unit starts it.

- [ ] **Step 1: Makefile**

```make
.PHONY: build run test vet

build:
	go build -o bin/mermaid-storage .

run: build
	./bin/mermaid-storage

test:
	go test ./... -count=1

vet:
	go vet ./...
```

- [ ] **Step 2: Swap npm script and unit**

- `package.json`: `"server": "make -C server-go run"`; delete the `"parity"` entry.
- `systemd/mermaid-storage.service`: replace `ExecStart=/usr/bin/pnpm run server` with:

```ini
# build with: make -C server-go build
ExecStart=/data/opensource/mermaid-live-editor/server-go/bin/mermaid-storage
```

(keep WorkingDirectory/Restart/After unchanged).

- [ ] **Step 3: Live cutover with e2e verification**

```bash
make -C server-go build
sudo systemctl stop mermaid-storage
./server-go/bin/mermaid-storage &    # same port 18790, same DATA_DIR
sleep 1 && curl -s http://127.0.0.1:18790/api/folders | head -c 200
pnpm test:e2e --workers=1
kill %1
```

Expected: curl returns the existing folders JSON (real data intact — same `server-data/diagrams.db`); full e2e green (48 passed / 4 skipped / 1 pre-existing errorDisplay failure).

- [ ] **Step 4: Install the new unit and verify**

```bash
sudo systemctl daemon-reload && sudo systemctl restart mermaid-storage
systemctl is-active mermaid-storage && curl -s http://127.0.0.1:18790/api/folders | head -c 200
```

Expected: `active` + JSON. Confirm `/etc/systemd/system/mermaid-storage.service` is a symlink to the repo file (existing deployment layout); re-link if it is a copy.

- [ ] **Step 5: Delete Node server and harness; update docs**

```bash
git rm server/index.mjs server/db.mjs scripts/parity-check.mjs
```

Update `README-LOCAL.md`: `pnpm server` → `make -C server-go run`, new ExecStart, build command, remove parity mentions.

- [ ] **Step 6: Full verification**

Run: `make -C server-go test && pnpm vitest run --dir src && pnpm check`
Expected: green (no frontend changes in Phase A).

- [ ] **Step 7: Commit**

```bash
git add -A server-go/ systemd/ scripts/ package.json README-LOCAL.md
git commit -m "feat!: replace Node storage server with Go binary

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 5: Auth primitives — scrypt, JWT, validation

**Files:**

- Create: `server-go/auth.go`, `server-go/auth_test.go`

**Interfaces:**

- Consumes: `readRandom` from Task 1.
- Produces (Task 7):
  - `const sessionCookie = "mermaid_session"`, `const sessionTTL = 7 * 24 * time.Hour`
  - `func hashPassword(password string) (string, error)` → `scrypt$32768$8$1$<saltB64>$<hashB64>`
  - `func verifyPassword(password, stored string) bool`
  - `func signJWT(userID string, issued time.Time, secret []byte) string`
  - `func verifyJWT(token string, secret []byte, now time.Time) (userID string, ok bool)`
  - `func validateEmail(email string) (string, bool)` (lowercased, `user@host.tld`)
  - `func validatePassword(password string) bool` (8–128)
  - `func base64url(b []byte) string` (test helper reuse)

- [ ] **Step 1: Write the failing tests**

`server-go/auth_test.go` (complete):

```go
package main

import (
	"strings"
	"testing"
	"time"
)

func TestPasswordRoundtrip(t *testing.T) {
	hash, err := hashPassword("correct horse battery")
	if err != nil || !strings.HasPrefix(hash, "scrypt$32768$8$1$") {
		t.Fatalf("hash: %q err=%v", hash, err)
	}
	if !verifyPassword("correct horse battery", hash) {
		t.Fatal("correct password must verify")
	}
	if verifyPassword("wrong", hash) {
		t.Fatal("wrong password must not verify")
	}
	if verifyPassword("correct horse battery", "scrypt$32768$8$1$AAAA$BBBB") {
		t.Fatal("corrupt hash must not verify")
	}
}

func TestJWTRoundtrip(t *testing.T) {
	secret := []byte("s3cret")
	issued := time.Unix(1_700_000_000, 0)
	token := signJWT("user-1", issued, secret)
	uid, ok := verifyJWT(token, secret, issued.Add(time.Hour))
	if !ok || uid != "user-1" {
		t.Fatalf("roundtrip: %q %v", uid, ok)
	}
}

func TestJWTVerificationFailures(t *testing.T) {
	secret := []byte("s3cret")
	issued := time.Unix(1_700_000_000, 0)
	token := signJWT("user-1", issued, secret)
	now := issued.Add(time.Hour)

	if _, ok := verifyJWT(token, []byte("other"), now); ok {
		t.Fatal("wrong secret must fail")
	}
	if _, ok := verifyJWT(token+"x", secret, now); ok {
		t.Fatal("tampered signature must fail")
	}
	parts := strings.Split(token, ".")
	swapped := parts[0] + "." + base64url(mustJSON(jwtClaims{Exp: 9_999_999_999, Iat: 1, Sub: "attacker"})) + "." + parts[2]
	if _, ok := verifyJWT(swapped, secret, now); ok {
		t.Fatal("swapped payload must fail")
	}
	if _, ok := verifyJWT(token, secret, issued.Add(sessionTTL+time.Minute)); ok {
		t.Fatal("expired token must fail")
	}
	forged := base64url(mustJSON(jwtHeader{Alg: "HS512", Typ: "JWT"})) + "." + parts[1] + "." + parts[2]
	if _, ok := verifyJWT(forged, secret, now); ok {
		t.Fatal("non-HS256 alg must fail")
	}
	if _, ok := verifyJWT("not-a-jwt", secret, now); ok {
		t.Fatal("malformed token must fail")
	}
}

func TestValidation(t *testing.T) {
	if e, ok := validateEmail("User@Example.COM"); !ok || e != "user@example.com" {
		t.Fatalf("email normalize: %q %v", e, ok)
	}
	for _, bad := range []string{"nope", "a@b", "@b.com", "a@.com", ""} {
		if _, ok := validateEmail(bad); ok {
			t.Fatalf("bad email accepted: %q", bad)
		}
	}
	for _, bad := range []string{"", "short", strings.Repeat("x", 129)} {
		if validatePassword(bad) {
			t.Fatalf("bad password accepted: len=%d", len(bad))
		}
	}
	if !validatePassword("eightchars") {
		t.Fatal("8-char password must be valid")
	}
}
```

- [ ] **Step 2: Run to verify failure** — `cd server-go && go test ./...` → FAIL (undefined).

- [ ] **Step 3: Implement `server-go/auth.go`** (complete):

```go
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/scrypt"
)

const (
	sessionCookie = "mermaid_session"
	sessionTTL    = 7 * 24 * time.Hour
	scryptN       = 32768
	scryptR       = 8
	scryptP       = 1
	scryptKeyLen  = 32
)

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	readRandom(salt)
	key, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, scryptKeyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("scrypt$%d$%d$%d$%s$%s", scryptN, scryptR, scryptP,
		base64.StdEncoding.EncodeToString(salt), base64.StdEncoding.EncodeToString(key)), nil
}

func verifyPassword(password, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 6 || parts[0] != "scrypt" {
		return false
	}
	var n, r, p int
	if _, err := fmt.Sscanf(strings.Join(parts[1:4], "$"), "%d$%d$%d", &n, &r, &p); err != nil {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.StdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got, err := scrypt.Key([]byte(password), salt, n, r, p, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

func base64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type jwtClaims struct {
	Exp int64  `json:"exp"`
	Iat int64  `json:"iat"`
	Sub string `json:"sub"`
}

func mustJSON(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}

func signJWT(userID string, issued time.Time, secret []byte) string {
	header := base64url(mustJSON(jwtHeader{Alg: "HS256", Typ: "JWT"}))
	claims := base64url(mustJSON(jwtClaims{Exp: issued.Add(sessionTTL).Unix(), Iat: issued.Unix(), Sub: userID}))
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(header + "." + claims))
	return header + "." + claims + "." + base64url(mac.Sum(nil))
}

func verifyJWT(token string, secret []byte, now time.Time) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", false
	}
	var header jwtHeader
	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(headerRaw, &header) != nil || header.Alg != "HS256" {
		return "", false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	want, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(mac.Sum(nil), want) {
		return "", false
	}
	claimsRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	var claims jwtClaims
	if err != nil || json.Unmarshal(claimsRaw, &claims) != nil {
		return "", false
	}
	if now.Unix() >= claims.Exp || claims.Sub == "" {
		return "", false
	}
	return claims.Sub, true
}

func validateEmail(email string) (string, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	at := strings.Index(email, "@")
	if at <= 0 || len(email)-at <= 1 {
		return "", false
	}
	domain := email[at+1:]
	dot := strings.LastIndex(domain, ".")
	if dot <= 0 || dot == len(domain)-1 || strings.ContainsAny(email, " \t") {
		return "", false
	}
	return email, true
}

func validatePassword(password string) bool {
	return len(password) >= 8 && len(password) <= 128
}
```

- [ ] **Step 4: Run tests** — `cd server-go && go test ./... && go vet ./...` → PASS.

- [ ] **Step 5: Commit**

```bash
git add server-go/
git commit -m "feat: add scrypt password hashing and HS256 JWT primitives

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 6: Users table and NULL-row claiming (no Phase A signature changes)

**Files:**

- Modify: `server-go/db.go` (users table + user methods + claimUnowned)
- Test: `server-go/db_test.go` (extend)

**Interfaces:**

- Consumes: Task 1.
- Produces (Task 7):
  - `type User struct { CreatedAt string `json:"createdAt"`; Email string `json:"email"`; ID string `json:"id"` }` (no password field — never serialized)
  - Migration in `openDB`: `users` table + idempotent `user_id TEXT REFERENCES users(id)` ALTERs on folders/diagrams
  - `getUserByEmail(email string) *User`, `getUser(id string) *User`, `getPasswordHash(email string) string`, `createUser(email, passwordHash string) (User, error)`, `countUsers() int`, `claimUnowned(userID string) error`
  - `var errEmailExists`
- IMPORTANT: do NOT change the thirteen Phase A method signatures in this task — that happens atomically with the middleware in Task 7 so every task stays green.

- [ ] **Step 1: Write the failing tests** (append to `db_test.go`):

```go
func TestUserMethods(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.createUser("dup@x.com", "h"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.createUser("DUP@x.com", "h"); err == nil { // createUser lowercases; unique must fire
		t.Fatal("duplicate email must fail")
	}
	if s.getUserByEmail("dup@x.com") == nil {
		t.Fatal("get by email")
	}
	if s.countUsers() != 1 {
		t.Fatalf("count: %d", s.countUsers())
	}
	if s.getPasswordHash("dup@x.com") != "h" {
		t.Fatal("password hash lookup")
	}
}

func TestClaimUnowned(t *testing.T) {
	s := newTestStore(t)
	s.db.Exec("INSERT INTO folders (id, name, parent_id, created_at, updated_at) VALUES ('legacy-f', 'Legacy', NULL, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')")
	s.db.Exec("INSERT INTO diagrams (id, folder_id, name, state, favorite, positions, created_at, updated_at) VALUES ('legacy-d', 'legacy-f', 'L', 's', 0, NULL, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')")

	first, _ := s.createUser("first@x.com", "h")
	if err := s.claimUnowned(first.ID); err != nil {
		t.Fatal(err)
	}
	var owner string
	s.db.QueryRow("SELECT user_id FROM folders WHERE id = 'legacy-f'").Scan(&owner)
	if owner != first.ID {
		t.Fatalf("first user must own legacy folder: %q", owner)
	}
	second, _ := s.createUser("second@x.com", "h")
	if err := s.claimUnowned(second.ID); err != nil {
		t.Fatal(err)
	}
	s.db.QueryRow("SELECT user_id FROM folders WHERE id = 'legacy-f'").Scan(&owner)
	if owner != first.ID {
		t.Fatal("claim must be first-writer-wins")
	}
}
```

- [ ] **Step 2: Run to verify failure** — `cd server-go && go test ./...` → FAIL (undefined).

- [ ] **Step 3: Implement** — in `openDB`'s schema string add:

```sql
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_at TEXT NOT NULL
);
```

and after the positions migration:

```go
	for _, table := range []string{"folders", "diagrams"} {
		if _, err := d.Exec("ALTER TABLE " + table + " ADD COLUMN user_id TEXT REFERENCES users(id)"); err != nil {
			if !strings.Contains(err.Error(), "duplicate column name") {
				return nil, err
			}
		}
	}
```

New methods in `db.go`:

```go
var errEmailExists = errors.New("email already registered")

func (s *store) getUserByEmail(email string) *User {
	row := s.db.QueryRow("SELECT id, email, created_at FROM users WHERE email = ?", strings.ToLower(email))
	return scanUser(row)
}

func (s *store) getUser(id string) *User {
	row := s.db.QueryRow("SELECT id, email, created_at FROM users WHERE id = ?", id)
	return scanUser(row)
}

func (s *store) getPasswordHash(email string) string {
	var hash string
	s.db.QueryRow("SELECT password_hash FROM users WHERE email = ?", strings.ToLower(email)).Scan(&hash)
	return hash
}

func (s *store) countUsers() int {
	var n int
	s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&n)
	return n
}

func (s *store) createUser(email, passwordHash string) (User, error) {
	email = strings.ToLower(email)
	id := newUUID()
	_, err := s.db.Exec("INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, ?)", id, email, passwordHash, now())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return User{}, errEmailExists
		}
		return User{}, err
	}
	return *s.getUser(id), nil
}

// claimUnowned assigns every NULL-owner row to userID (first registration).
func (s *store) claimUnowned(userID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE folders SET user_id = ? WHERE user_id IS NULL", userID); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE diagrams SET user_id = ? WHERE user_id IS NULL", userID); err != nil {
		return err
	}
	return tx.Commit()
}

func scanUser(row interface{ Scan(...any) error }) *User {
	var u User
	if row.Scan(&u.ID, &u.Email, &u.CreatedAt) != nil {
		return nil
	}
	return &u
}
```

- [ ] **Step 4: Run tests** — `cd server-go && go test ./...` → PASS (Phase A tests untouched).

- [ ] **Step 5: Commit**

```bash
git add server-go/
git commit -m "feat: add users table with NULL-row claiming to Go storage

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 7: Auth endpoints, middleware, per-user scoping

**Files:**

- Create: `server-go/auth_handlers.go`, `server-go/auth_handlers_test.go`
- Modify: `server-go/db.go` (thirteen userID signatures), `server-go/handlers.go` (auth routes + requireAuth + threading), `server-go/main.go` (secret)

**Interfaces:**

- Consumes: Tasks 1, 2, 5, 6.
- Produces (Tasks 8-10):
  - `newHandler(s *store, secret []byte) http.Handler` (supersedes Task 2's form)
  - `POST /api/auth/register` `{email, password}` → `201 {createdAt, email, id}` + Set-Cookie; `400 "email must be a valid address"` / `400 "password must be 8-128 characters"` / `409 "email already registered"`; first user atomically claims NULL rows
  - `POST /api/auth/login` → `200 user` + Set-Cookie; `401 "invalid email or password"`
  - `POST /api/auth/logout` → `200 {"ok":true}` + expired cookie
  - `GET /api/auth/me` → `200 user` or `401 "unauthorized"`
  - All other `/api/*` without valid cookie → `401 {"error":"unauthorized"}`
  - `func loadOrCreateSecret(dataDir string) ([]byte, error)` — `MERMAID_JWT_SECRET` env wins; else read/write `<dataDir>/jwt-secret` (32 random bytes, 0600)
  - Store: every Phase A method gains a leading `userID string` param + `user_id = ?` predicate (SELECT/WHERE) or insert value (INSERT). Pattern (createDiagram): INSERT lists `user_id` with userID; every `getFolder/getDiagram` SELECT gains `AND user_id = ?`; `listFolders/listDiagrams/listAllDiagrams` gain the predicate; `isSelfOrAncestor` threads userID into its getFolder calls.

- [ ] **Step 1: Write the failing tests**

`server-go/auth_handlers_test.go` (complete; helpers `doReq(t, h, method, path, body, cookie) (*httptest.ResponseRecorder, map[string]any)` and `contains(haystack, needle string) bool`):

```go
package main

import (
	"net/http"
	"testing"
)

const testSecret = "test-secret-0123456789abcdef"

func newAuthHandler(t *testing.T) http.Handler {
	return newHandler(newTestStore(t), []byte(testSecret))
}

func register(t *testing.T, h http.Handler, email, password string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	return doReq(t, h, "POST", "/api/auth/register",
		`{"email":"`+email+`","password":"`+password+`"}`, "")
}

func TestRegisterClaimLoginLogoutMe(t *testing.T) {
	h := newAuthHandler(t)
	s := h.(*muxStore).s // if needed for legacy seeding — otherwise seed via a second handler on the same store; see note below

	res, body := register(t, h, "First@X.com", "password123")
	if res.Code != 201 || body["email"] != "first@x.com" {
		t.Fatalf("register: %d %v", res.Code, body)
	}
	cookie := res.Header().Get("Set-Cookie")
	if !contains(cookie, "HttpOnly") || !contains(cookie, "SameSite=Lax") {
		t.Fatalf("cookie: %q", cookie)
	}
	if code, list := doReq(t, h, "GET", "/api/diagrams", "", cookie); code != 200 {
		t.Fatalf("authed list: %d %v", code, list)
	}
	if code, body := register(t, h, "first@x.com", "password123"); code != 409 || body["error"] != "email already registered" {
		t.Fatalf("duplicate: %d %v", code, body)
	}
	if code, body := register(t, h, "not-an-email", "password123"); code != 400 || body["error"] != "email must be a valid address" {
		t.Fatalf("bad email: %d %v", code, body)
	}
	if code, _ := register(t, h, "ok@x.com", "short"); code != 400 {
		t.Fatalf("bad password: %d", code)
	}
	res2, _ := register(t, h, "second@x.com", "password123")
	cookie2 := res2.Header().Get("Set-Cookie")

	res3, body3 := doReq(t, h, "POST", "/api/auth/login", `{"email":"FIRST@X.com","password":"password123"}`, "")
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
	s.db.Exec("INSERT INTO diagrams (id, folder_id, name, state, favorite, positions, created_at, updated_at) VALUES ('legacy', NULL, 'L', 's', 0, NULL, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', NULL)")
	h := newHandler(s, []byte(testSecret))

	resA, _ := register(t, h, "a@x.com", "password123")
	cookieA := resA.Header().Get("Set-Cookie")
	resB, _ := register(t, h, "b@x.com", "password123")
	cookieB := resB.Header().Get("Set-Cookie")

	// first user claimed the legacy row; second user starts empty
	if code, list := doReq(t, h, "GET", "/api/diagrams", "", cookieA); code != 200 || len(list.([]any)) != 1 {
		t.Fatalf("claim: %d %v", code, list)
	}
	if code, list := doReq(t, h, "GET", "/api/diagrams", "", cookieB); code != 200 || len(list.([]any)) != 0 {
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
```

Note: drop the `muxStore` line from `TestRegisterClaimLoginLogoutMe` (leftover sketch artifact) — that test needs no direct store access. Import `net/http/httptest` where the helpers live.

- [ ] **Step 2: Run to verify failure** — `cd server-go && go test ./...` → FAIL (auth routes undefined; anonymous /api/folders still 200).

- [ ] **Step 3: Implement**

`server-go/auth_handlers.go` (complete):

```go
package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// loadOrCreateSecret: MERMAID_JWT_SECRET wins; else persist 32 random bytes to <dataDir>/jwt-secret.
func loadOrCreateSecret(dataDir string) ([]byte, error) {
	if secret := os.Getenv("MERMAID_JWT_SECRET"); secret != "" {
		return []byte(secret), nil
	}
	path := filepath.Join(dataDir, "jwt-secret")
	if secret, err := os.ReadFile(path); err == nil && len(secret) >= 32 {
		return secret, nil
	}
	secret := make([]byte, 32)
	readRandom(secret)
	if err := os.WriteFile(path, secret, 0o600); err != nil {
		return nil, err
	}
	return secret, nil
}

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		HttpOnly: true,
		MaxAge:   int(sessionTTL.Seconds()),
		Name:     sessionCookie,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Value:    token,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		HttpOnly: true,
		MaxAge:   -1,
		Name:     sessionCookie,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
	})
}

func decodeAuthBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err := decoder.Decode(target); err != nil {
		writeError(w, 400, "Invalid JSON body")
		return false
	}
	return true
}

func (a *api) userIDFromRequest(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return "", false
	}
	return verifyJWT(cookie.Value, a.secret, time.Now())
}

func (a *api) handleRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeAuthBody(w, r, &body) {
		return
	}
	email, ok := validateEmail(body.Email)
	if !ok {
		writeError(w, 400, "email must be a valid address")
		return
	}
	if !validatePassword(body.Password) {
		writeError(w, 400, "password must be 8-128 characters")
		return
	}
	hash, err := hashPassword(body.Password)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if a.s.getUserByEmail(email) != nil {
		writeError(w, 409, "email already registered")
		return
	}
	user, err := a.s.createUser(email, hash)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if a.s.countUsers() == 1 {
		if err := a.s.claimUnowned(user.ID); err != nil {
			writeError(w, 400, err.Error())
			return
		}
	}
	setSessionCookie(w, signJWT(user.ID, time.Now(), a.secret))
	writeJSON(w, 201, user)
}

func (a *api) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeAuthBody(w, r, &body) {
		return
	}
	email, ok := validateEmail(body.Email)
	if !ok {
		writeError(w, 401, "invalid email or password")
		return
	}
	hash := a.s.getPasswordHash(email)
	if hash == "" || !verifyPassword(body.Password, hash) {
		writeError(w, 401, "invalid email or password")
		return
	}
	user := a.s.getUserByEmail(email)
	if user == nil {
		writeError(w, 401, "invalid email or password")
		return
	}
	setSessionCookie(w, signJWT(user.ID, time.Now(), a.secret))
	writeJSON(w, 200, user)
}

func (a *api) handleLogout(w http.ResponseWriter, _ *http.Request) {
	clearSessionCookie(w)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *api) handleMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.userIDFromRequest(r)
	if !ok {
		writeError(w, 401, "unauthorized")
		return
	}
	user := a.s.getUser(userID)
	if user == nil {
		writeError(w, 401, "unauthorized")
		return
	}
	writeJSON(w, 200, user)
}
```

`handlers.go` changes:

- `api` struct becomes `type api struct { s *store; secret []byte }`; `newHandler(s *store, secret []byte) http.Handler`.
- Add:

```go
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
```

- In `newHandler`: register `/api/auth/register`, `/api/auth/login`, `/api/auth/logout`, `/api/auth/me` (method-checked inside each; wrong method → `a.notFound`), and wrap the six existing routes with `requireAuth`, threading `userID` into every handler method and store call (`a.s.listFolders(userID)` etc.).
- Update Task 2's `handlers_test.go`: `newHandler(newTestStore(t), []byte("test-secret"))`, and each test registers a user first (`register(t, h, "t@x.com", "password123")`) and sends its cookie in `doReq` (extend the helper with a cookie param). The matrix assertions stay identical.

`db.go`: the thirteen signature changes (pattern in Interfaces). `main.go`: `secret, err := loadOrCreateSecret(dataDir)` before `newHandler(s, secret)`.

- [ ] **Step 4: Run all Go tests** — `cd server-go && go test ./... -count=1 && go vet ./...` → PASS.

- [ ] **Step 5: Commit**

```bash
git add server-go/
git commit -m "feat: add JWT auth endpoints and per-user data isolation to Go server

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 8: Frontend — auth state, API functions, login/register pages

**Files:**

- Create: `src/lib/util/auth.svelte.ts`, `src/lib/components/AuthForm.svelte`, `src/routes/(auth)/login/+page.svelte`, `src/routes/(auth)/register/+page.svelte`
- Modify: `src/lib/util/api.ts`, `src/lib/constants.ts`

**Interfaces:**

- Consumes: Task 7 endpoints.
- Produces (Task 9):
  - `authState.current: User | null | undefined` (undefined = unchecked), `fetchMe()`, `login()`, `register()`, `logout()` (auth.svelte.ts)
  - `apiRegister/apiLogin/apiLogout` + `setUnauthorizedHandler` + `User` type (api.ts)
  - TIDs: `authConfirm`, `authEmail`, `authError`, `authPassword`, `authSubmit`, `authSwitchLink`

- [ ] **Step 1: TID constants** — insert alphabetically into `TID` (between `aiRepairButton` and `copyMarkdown`):

```ts
  authConfirm: 'auth-confirm',
  authEmail: 'auth-email',
  authError: 'auth-error',
  authPassword: 'auth-password',
  authSubmit: 'auth-submit',
  authSwitchLink: 'auth-switch-link',
```

- [ ] **Step 2: `src/lib/util/api.ts`** — add `User` interface (after `Folder`), auth functions and the 401 hook after the `request` helper:

```ts
export interface User {
  createdAt: string;
  email: string;
  id: string;
}
```

```ts
let onUnauthorized: (() => void) | undefined;
export const setUnauthorizedHandler = (handler: () => void): void => {
  onUnauthorized = handler;
};

const authRequest = async <T>(path: string, init?: RequestInit): Promise<T> => {
  const response = await fetch(`/api/auth${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => undefined)) as { error?: string } | undefined;
    throw new Error(body?.error ?? `API request failed (${response.status})`);
  }
  return (await response.json()) as T;
};

export const apiRegister = (email: string, password: string): Promise<User> =>
  authRequest<User>('', { body: JSON.stringify({ email, password }), method: 'POST' });

export const apiLogin = (email: string, password: string): Promise<User> =>
  authRequest<User>('/login', { body: JSON.stringify({ email, password }), method: 'POST' });

export const apiLogout = (): Promise<{ ok: boolean }> =>
  authRequest<{ ok: boolean }>('/logout', { method: 'POST' });
```

Inside the existing `request` helper, at the start of the `if (!response.ok)` branch, add:

```ts
if (response.status === 401) {
  onUnauthorized?.();
}
```

- [ ] **Step 3: `src/lib/util/auth.svelte.ts`** (complete file):

```ts
import { base } from '$app/paths';
import { apiLogin, apiLogout, apiRegister, setUnauthorizedHandler, type User } from '$/util/api';

let user: User | null | undefined = $state.raw(undefined);

export const authState = {
  get current(): User | null | undefined {
    return user;
  }
};

export const fetchMe = async (): Promise<User | null> => {
  try {
    const response = await fetch('/api/auth/me');
    user = response.ok ? ((await response.json()) as User) : null;
  } catch {
    user = null;
  }
  return user;
};

export const login = async (email: string, password: string): Promise<User> => {
  user = await apiLogin(email, password);
  return user;
};

export const register = async (email: string, password: string): Promise<User> => {
  user = await apiRegister(email, password);
  return user;
};

export const logout = async (): Promise<void> => {
  await apiLogout().catch(() => undefined);
  user = null;
};

// 401 from any storage API call means the session died: drop state and go to
// the login page, preserving the current location for post-login return.
setUnauthorizedHandler(() => {
  user = null;
  if (location.pathname === `${base}/login`) {
    return; // already there — no redirect loop
  }
  location.assign(
    `${base}/login?redirect=${encodeURIComponent(location.pathname + location.hash)}`
  );
});
```

- [ ] **Step 4: `src/lib/components/AuthForm.svelte`** (complete; reuse the existing `Button`/`Input` shadcn components — check the exact import paths in `SaveDialog.svelte` and match):

```svelte
<script lang="ts">
  import { Button } from '$/components/ui/button';
  import { Input } from '$/components/ui/input';
  import { TID } from '$/constants';
  import { login, register } from '$/util/auth.svelte';
  import { base } from '$app/paths';
  import { goto } from '$app/navigation';

  let { mode }: { mode: 'login' | 'register' } = $props();

  let email = $state('');
  let password = $state('');
  let confirm = $state('');
  let error = $state('');
  let busy = $state(false);

  const emailOk = (value: string): boolean => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value);
  const passwordOk = (value: string): boolean => value.length >= 8 && value.length <= 128;

  const submit = async () => {
    error = '';
    if (!emailOk(email)) {
      error = 'Enter a valid email address';
      return;
    }
    if (!passwordOk(password)) {
      error = 'Password must be 8-128 characters';
      return;
    }
    if (mode === 'register' && password !== confirm) {
      error = 'Passwords do not match';
      return;
    }
    busy = true;
    try {
      await (mode === 'login' ? login(email, password) : register(email, password));
      const redirect = new URLSearchParams(location.search).get('redirect');
      await goto(redirect && redirect.startsWith(base) ? redirect : `${base}/edit`);
    } catch (err) {
      error = err instanceof Error ? err.message : 'Something went wrong';
    } finally {
      busy = false;
    }
  };
</script>

<div class="mx-auto mt-24 w-full max-w-sm rounded-lg border border-border p-6 shadow-sm">
  <h1 class="mb-4 text-center text-xl font-semibold">
    {mode === 'login' ? 'Log in' : 'Create an account'}
  </h1>
  <form
    class="flex flex-col gap-3"
    onsubmit={(event) => {
      event.preventDefault();
      void submit();
    }}>
    <Input
      bind:value={email}
      data-testid={TID.authEmail}
      placeholder="email@example.com"
      type="email" />
    <Input
      bind:value={password}
      data-testid={TID.authPassword}
      placeholder="Password"
      type="password" />
    {#if mode === 'register'}
      <Input
        bind:value={confirm}
        data-testid={TID.authConfirm}
        placeholder="Confirm password"
        type="password" />
    {/if}
    {#if error}
      <p class="text-sm text-destructive" data-testid={TID.authError}>{error}</p>
    {/if}
    <Button data-testid={TID.authSubmit} disabled={busy} type="submit" variant="accent">
      {busy ? 'Please wait…' : mode === 'login' ? 'Log in' : 'Register'}
    </Button>
  </form>
  <a
    class="mt-4 block text-center text-sm text-accent hover:underline"
    data-testid={TID.authSwitchLink}
    href={`${base}/${mode === 'login' ? 'register' : 'login'}`}>
    {mode === 'login' ? 'Need an account? Register' : 'Already have an account? Log in'}
  </a>
</div>
```

- [ ] **Step 5: Route pages**

`src/routes/(auth)/login/+page.svelte`:

```svelte
<script lang="ts">
  import AuthForm from '$/components/AuthForm.svelte';
</script>

<AuthForm mode="login" />
```

`src/routes/(auth)/register/+page.svelte`: identical with `mode="register"`.

- [ ] **Step 6: Verify** — `pnpm check` clean; lint changed files only (`pnpm exec eslint <files>`, `pnpm exec prettier --check <files>`); `pnpm vitest run --dir src` green.

- [ ] **Step 7: Commit**

```bash
git add src/lib/util/auth.svelte.ts src/lib/util/api.ts src/lib/constants.ts src/lib/components/AuthForm.svelte 'src/routes/(auth)/'
git commit -m "feat: add login and register pages with auth state and 401 redirect

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 9: Navbar user menu + route guards

**Files:**

- Modify: `src/lib/components/Navbar.svelte`, `src/routes/(app)/edit/+page.svelte`, `src/routes/(app)/dashboard/+page.svelte`, `src/lib/constants.ts`

**Interfaces:**

- Consumes: Task 8 (`authState`, `logout`, `fetchMe`).
- Produces: TIDs `navbarLogin`, `navbarLogout`, `navbarUserMenu`; complete UI wiring.

- [ ] **Step 1: TIDs** — insert `navbarLogin`, `navbarLogout`, `navbarUserMenu` alphabetically near the existing `navbarDownload*` keys.

- [ ] **Step 2: Navbar** — READ `Navbar.svelte` FIRST: it contains uncommitted user WIP; preserve every WIP line and follow the file's existing `DropdownNavMenu` pattern for the new block. In the right-side actions area add:

```svelte
{#if authState.current}
  <!-- mirror the file's existing DropdownNavMenu usage; title = authState.current.email -->
  <DropdownNavMenu data-testid={TID.navbarUserMenu} title={authState.current.email}>
    <button
      data-testid={TID.navbarLogout}
      onclick={() => {
        void logout().then(() => location.assign('/login'));
      }}>
      Log out
    </button>
  </DropdownNavMenu>
{:else if authState.current === null}
  <a data-testid={TID.navbarLogin} href="/login">Log in</a>
{/if}
```

(Adapt props/classes to `DropdownNavMenu`'s real API as used elsewhere in the file; also `import { authState, logout } from '$/util/auth.svelte';` and call `void fetchMe()` on mount so the navbar populates on first load.)

- [ ] **Step 3: Route guards** — in `src/routes/(app)/edit/+page.svelte` and `src/routes/(app)/dashboard/+page.svelte`, add at the top of the existing `onMount` (or create one):

```ts
void fetchMe().then((user) => {
  if (!user) {
    location.assign(`/login?redirect=${encodeURIComponent(location.pathname + location.hash)}`);
  }
});
```

NO guard on `/view` or `/embed` (share links stay public).

- [ ] **Step 4: Manual smoke** (dev server on :3000 with the Go storage running): `/edit` → redirects to `/login`; register → `/edit`; navbar shows the email menu; save a diagram; logout → login page; open a `/view#pako:...` share link while logged out → renders.

- [ ] **Step 5: Verify and commit** — `pnpm check`; lint changed files; `pnpm vitest run --dir src`; `pnpm test:e2e tests/canvasInteraction.spec.ts --workers=1` still green.

```bash
git add src/lib/components/Navbar.svelte 'src/routes/(app)/edit/+page.svelte' 'src/routes/(app)/dashboard/+page.svelte' src/lib/constants.ts
git commit -m "feat: add navbar user menu and login guards for edit and dashboard

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 10: Isolated e2e + deployment docs

**Files:**

- Create: `playwright.auth.config.ts`, `tests/auth.spec.ts`
- Modify: `package.json`, `README-LOCAL.md`

**Interfaces:**

- Consumes: everything above.
- Produces: `pnpm test:e2e:auth` — isolated (fresh `server-data/e2e`, ports 18799/3100, zero contact with real data or systemd services).

- [ ] **Step 1: `playwright.auth.config.ts`** (complete):

```ts
import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  fullyParallel: false,
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  reporter: 'list',
  testDir: './tests',
  testMatch: 'auth.spec.ts',
  use: {
    baseURL: 'http://localhost:3100',
    browserName: 'chromium',
    permissions: ['clipboard-read', 'clipboard-write'],
    trace: 'retain-on-failure',
    viewport: { width: 1920, height: 1080 }
  },
  webServer: [
    {
      command:
        'rm -rf server-data/e2e && make -C server-go build && DATA_DIR=server-data/e2e PORT=18799 ./server-go/bin/mermaid-storage',
      url: 'http://127.0.0.1:18799/api/folders',
      reuseExistingServer: false
    },
    {
      command: 'API_PROXY_TARGET=http://127.0.0.1:18799 pnpm dev --port 3100 --strictPort',
      url: 'http://localhost:3100',
      reuseExistingServer: false
    }
  ],
  workers: 1
});
```

Note: if Playwright's readiness probe rejects the 401 from `/api/folders`, add `mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })` to `newHandler` and use `http://127.0.0.1:18799/healthz` as the url.

- [ ] **Step 2: `tests/auth.spec.ts`** (complete):

```ts
import { expect, test } from '@playwright/test';
import { TID } from '$/constants';

const register = async (page: import('@playwright/test').Page, email: string) => {
  await page.goto('/register');
  await page.getByTestId(TID.authEmail).fill(email);
  await page.getByTestId(TID.authPassword).fill('password123');
  await page.getByTestId(TID.authConfirm).fill('password123');
  await page.getByTestId(TID.authSubmit).click();
};

test.describe('Auth and per-user data', () => {
  test('register, save, isolate, redirect, public view', async ({ page }) => {
    await page.goto('/edit');
    await expect(page).toHaveURL(/\/login\?redirect=/);

    await register(page, 'a@e2e.test');
    await expect(page.getByTestId(TID.navbarUserMenu)).toBeVisible();
    await page.getByTestId(TID.saveButton).click();
    await page.getByTestId(TID.saveDialogInput).fill('A-diagram');
    await page.getByTestId(TID.saveDialogInput).press('Enter');
    await expect(page.getByText('Saved')).toBeVisible();

    await page.getByTestId(TID.navbarUserMenu).click();
    await page.getByTestId(TID.navbarLogout).click();
    await expect(page).toHaveURL(/\/login/);

    await register(page, 'b@e2e.test');
    await page.goto('/dashboard');
    await expect(page.getByTestId(TID.diagramCard)).toHaveCount(0);

    await page.getByTestId(TID.navbarUserMenu).click();
    await page.getByTestId(TID.navbarLogout).click();
    await page.goto(
      '/view#pako:eNpljjEKwzAMRa8SNOcEnlt6gK7eVFvYJsgOqkwpIXevg9smEE-PnyfxF3DFExgISW-CczQ2D21cYU7a-SGYXRwyvTp9jUhuKlVP-eHy7zA-leQsMEmg_QOM0BLG5FujZVMsaCQmC6ahR5ks2Lw2r84ela4-aREwKpVGwKrl_s7ut3fnkjAIcg_XDzuaUhs'
    );
    await expect(page.locator('#view')).toContainText('gitGraph');
  });

  test('login rejects wrong password with server message', async ({ page }) => {
    await register(page, 'c@e2e.test');
    await page.getByTestId(TID.navbarUserMenu).click();
    await page.getByTestId(TID.navbarLogout).click();
    await page.goto('/login');
    await page.getByTestId(TID.authEmail).fill('c@e2e.test');
    await page.getByTestId(TID.authPassword).fill('wrong-password');
    await page.getByTestId(TID.authSubmit).click();
    await expect(page.getByTestId(TID.authError)).toContainText('invalid email or password');
  });
});
```

- [ ] **Step 3: npm script + run** — `"test:e2e:auth": "playwright test -c playwright.auth.config.ts"`; run `pnpm test:e2e:auth`. Expected: 2/2 PASS. The :3100 dev server is independent of the always-running :3000 instance (`--strictPort` guarantees no silent reuse).

- [ ] **Step 4: README-LOCAL.md** — add an "Accounts" section: first registration inherits existing data; `jwt-secret` location; `MERMAID_JWT_SECRET` override; how to run `pnpm test:e2e:auth`.

- [ ] **Step 5: Full regression + production restart**

```bash
make -C server-go test && pnpm vitest run --dir src && pnpm check && pnpm test:e2e --workers=1
make -C server-go build && sudo systemctl restart mermaid-storage
curl -s http://127.0.0.1:18790/api/folders   # -> {"error":"unauthorized"} (gating live)
```

Then verify the app on :3000 loads and the register flow works against production data.

- [ ] **Step 6: Commit**

```bash
git add playwright.auth.config.ts tests/auth.spec.ts package.json README-LOCAL.md
git commit -m "test: add isolated e2e for auth and per-user data isolation

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

## Self-Review Notes

- Spec coverage: Go port (Tasks 1-3), differential acceptance (Task 3 — replaces the spec's "existing e2e unchanged" because that suite never exercises the storage API), cutover incl. systemd + Node deletion (Task 4), scrypt/JWT/validation incl. alg-confusion and constant-time compare (Task 5), users/claim (Task 6), auth endpoints + middleware + isolation + first-user claim (Task 7), frontend pages/api/401 (Task 8), navbar/guards with public `/view`+`/embed` (Task 9), isolated e2e + docs + production restart (Task 10).
- Sequencing: Task 6 lands the users table WITHOUT touching Phase A signatures (keeps every task green); the thirteen signature changes land atomically with the middleware in Task 7.
- Type consistency: `parentValue`/`positionsValue` (Task 1) consumed by `rawParent`/`rawPositions` (Task 2); `newHandler(s, secret)` (Task 7) supersedes Task 2's single-arg form with test updates in Task 7 Step 3; `User` (Task 6) serialized by Task 7 and imported as `User` (Task 8).
