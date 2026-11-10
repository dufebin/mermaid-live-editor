package main

import (
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type NodeOffset struct {
	Dx float64 `json:"dx"`
	Dy float64 `json:"dy"`
}

// User is the public user shape; the password hash is never serialized.
type User struct {
	CreatedAt string `json:"createdAt"`
	Email     string `json:"email"`
	ID        string `json:"id"`
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
	// Pragmas ride in the DSN so they apply to EVERY pooled connection —
	// database/sql may silently replace the connection, and a replacement
	// would otherwise run with foreign_keys OFF. DSN syntax is
	// modernc.org/sqlite's `_pragma=name(value)`.
	d, err := sql.Open("sqlite", dir+"/diagrams.db?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	// Mirror node:sqlite's synchronous execution: one connection, no SQLITE_BUSY.
	d.SetMaxOpenConns(1)
	s := &store{db: d}
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
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_at TEXT NOT NULL
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
	for _, table := range []string{"folders", "diagrams"} {
		if _, err := d.Exec("ALTER TABLE " + table + " ADD COLUMN user_id TEXT REFERENCES users(id)"); err != nil {
			if !strings.Contains(err.Error(), "duplicate column name") {
				return nil, err
			}
		}
	}
	return s, nil
}

func (s *store) Close() error { return s.db.Close() }

func (s *store) listFolders(userID string) []Folder {
	rows, err := s.db.Query("SELECT id, name, parent_id, created_at, updated_at FROM folders WHERE user_id = ? ORDER BY name", userID)
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

func (s *store) listAllDiagrams(userID string) []Diagram {
	// NO ORDER BY: db.mjs's bare SELECT returns natural (rowid) order — parity.
	rows, err := s.db.Query(diagramSelect()+" WHERE user_id = ?", userID)
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

func (s *store) getFolder(userID, id string) *Folder {
	row := s.db.QueryRow("SELECT id, name, parent_id, created_at, updated_at FROM folders WHERE id = ? AND user_id = ?", id, userID)
	f, err := scanFolder(row)
	if err != nil {
		return nil
	}
	return &f
}

func (s *store) isSelfOrAncestor(userID, id, ancestorID string) bool {
	current := s.getFolder(userID, ancestorID)
	for current != nil {
		if current.ID == id {
			return true
		}
		if current.ParentID == nil {
			return false
		}
		current = s.getFolder(userID, *current.ParentID)
	}
	return false
}

func (s *store) createFolder(userID, name string, parentID *string) Folder {
	id := newUUID()
	ts := now()
	s.db.Exec("INSERT INTO folders (id, name, parent_id, user_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		id, name, parentID, userID, ts, ts)
	return *s.getFolder(userID, id)
}

// updateFolder patches a folder; a missing id yields (nil, nil), matching
// db.mjs returning null for missing rows.
func (s *store) updateFolder(userID, id string, name *string, parentID parentValue) (*Folder, error) {
	f := s.getFolder(userID, id)
	if f == nil {
		return nil, nil
	}
	nextName, nextParent := f.Name, f.ParentID
	if name != nil {
		nextName = *name
	}
	if parentID.Set {
		nextParent = parentID.Value
	}
	if _, err := s.db.Exec("UPDATE folders SET name = ?, parent_id = ?, updated_at = ? WHERE id = ? AND user_id = ?",
		nextName, nextParent, now(), id, userID); err != nil {
		return nil, err
	}
	return s.getFolder(userID, id), nil
}

func (s *store) deleteFolder(userID, id string) bool {
	res, err := s.db.Exec("DELETE FROM folders WHERE id = ? AND user_id = ?", id, userID)
	return err == nil && res != nil && rowsAffected(res) > 0
}

func (s *store) listDiagrams(userID string, q diagramQuery) []Diagram {
	where, params := []string{"user_id = ?"}, []any{userID}
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

func (s *store) getDiagram(userID, id string) *Diagram {
	row := s.db.QueryRow(diagramSelect()+" WHERE id = ? AND user_id = ?", id, userID)
	d, err := scanDiagram(row)
	if err != nil {
		return nil
	}
	return &d
}

func (s *store) createDiagram(userID string, in diagramInput) Diagram {
	id := newUUID()
	ts := now()
	s.db.Exec("INSERT INTO diagrams (id, folder_id, name, state, favorite, positions, user_id, created_at, updated_at) VALUES (?, ?, ?, ?, 0, ?, ?, ?, ?)",
		id, in.FolderID, in.Name, in.State, positionsToJSON(in.Positions), userID, ts, ts)
	return *s.getDiagram(userID, id)
}

// updateDiagram patches a diagram; a missing id yields (nil, nil), matching
// db.mjs returning null for missing rows.
func (s *store) updateDiagram(userID, id string, p diagramPatch) (*Diagram, error) {
	d := s.getDiagram(userID, id)
	if d == nil {
		return nil, nil
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
	if _, err := s.db.Exec("UPDATE diagrams SET name = ?, folder_id = ?, state = ?, favorite = ?, positions = ?, updated_at = ? WHERE id = ? AND user_id = ?",
		name, folder, state, favorite, positionsToJSON(positions), now(), id, userID); err != nil {
		return nil, err
	}
	return s.getDiagram(userID, id), nil
}

func (s *store) deleteDiagram(userID, id string) bool {
	res, err := s.db.Exec("DELETE FROM diagrams WHERE id = ? AND user_id = ?", id, userID)
	return err == nil && res != nil && rowsAffected(res) > 0
}

// --- users ---

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

// createUserAndClaim inserts the user and, if they are the first user,
// claims all NULL-owner rows — atomically, so concurrent first
// registrations cannot strand legacy data.
func (s *store) createUserAndClaim(email, passwordHash string) (User, error) {
	email = strings.ToLower(email)
	id := newUUID()
	ts := now()
	tx, err := s.db.Begin()
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return User{}, err
	}
	if _, err := tx.Exec("INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, ?)", id, email, passwordHash, ts); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return User{}, errEmailExists
		}
		return User{}, err
	}
	if count == 0 {
		if _, err := tx.Exec("UPDATE folders SET user_id = ? WHERE user_id IS NULL", id); err != nil {
			return User{}, err
		}
		if _, err := tx.Exec("UPDATE diagrams SET user_id = ? WHERE user_id IS NULL", id); err != nil {
			return User{}, err
		}
	}
	if err := tx.Commit(); err != nil {
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

func (s *store) importAll(userID string, payload importPayload) (importResult, error) {
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
	insertFolder, _ := tx.Prepare("INSERT OR IGNORE INTO folders (id, name, parent_id, user_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)")
	for _, f := range payload.Folders {
		if strings.TrimSpace(f.ID) == "" || strings.TrimSpace(f.Name) == "" {
			result.Skipped++
			continue
		}
		createdAt, updatedAt := orNow(f.CreatedAt), orNow(f.UpdatedAt)
		res, err := insertFolder.Exec(f.ID, f.Name, f.ParentID, userID, createdAt, updatedAt)
		if err == nil && rowsAffected(res) > 0 {
			result.Imported++
		} else if err == nil {
			result.Skipped++
		}
	}
	insertDiagram, _ := tx.Prepare("INSERT OR IGNORE INTO diagrams (id, folder_id, name, state, favorite, positions, user_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)")
	for _, d := range payload.Diagrams {
		if strings.TrimSpace(d.ID) == "" || strings.TrimSpace(d.Name) == "" || strings.TrimSpace(d.State) == "" {
			result.Skipped++
			continue
		}
		// A diagram may reference a folder its importer does not own (e.g. a
		// crafted payload with another user's folder UUID). Import such
		// diagrams as uncategorized instead of attaching them across users —
		// createDiagram/PATCH already enforce this on the write paths.
		// NOTE: resolve on tx, not s.db — getFolder uses s.db, which on the
		// single-connection pool would deadlock inside this open transaction.
		if d.FolderID != nil {
			var exists int
			if err := tx.QueryRow("SELECT COUNT(*) FROM folders WHERE id = ? AND user_id = ?", *d.FolderID, userID).Scan(&exists); err != nil {
				return result, err
			}
			if exists == 0 {
				d.FolderID = nil
			}
		}
		var positions any
		if d.Positions != nil {
			positions = positionsToJSON(d.Positions)
		}
		res, err := insertDiagram.Exec(d.ID, d.FolderID, d.Name, d.State, d.Favorite, positions, userID, orNow(d.CreatedAt), orNow(d.UpdatedAt))
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
