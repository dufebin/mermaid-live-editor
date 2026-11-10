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

// testUser creates a real user row so scoped store calls satisfy the user_id FK.
func testUser(t *testing.T, s *store) string {
	t.Helper()
	u, err := s.createUser("u@x.com", "h")
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func TestFolderCRUD(t *testing.T) {
	s := newTestStore(t)
	u := testUser(t, s)
	parent := s.createFolder(u, "Work", nil)
	if parent.Name != "Work" || parent.ParentID != nil {
		t.Fatalf("unexpected parent: %+v", parent)
	}
	child := s.createFolder(u, "Sub", &parent.ID)
	if *child.ParentID != parent.ID {
		t.Fatalf("child parent: %+v", child)
	}
	// rename, keep parent
	updated, err := s.updateFolder(u, child.ID, strPtr("Renamed"), parentValue{})
	if err != nil || updated.Name != "Renamed" || *updated.ParentID != parent.ID {
		t.Fatalf("update: %+v err=%v", updated, err)
	}
	// move to root via explicit null
	updated, err = s.updateFolder(u, child.ID, nil, parentValue{Set: true})
	if err != nil || updated.ParentID != nil {
		t.Fatalf("move to root: %+v err=%v", updated, err)
	}
	if !s.deleteFolder(u, child.ID) || s.getFolder(u, child.ID) != nil {
		t.Fatal("delete folder")
	}
}

func TestFolderDeleteCascadesDiagramsToNull(t *testing.T) {
	s := newTestStore(t)
	u := testUser(t, s)
	f := s.createFolder(u, "F", nil)
	d := s.createDiagram(u, diagramInput{Name: "D", State: "state", FolderID: &f.ID})
	if !s.deleteFolder(u, f.ID) {
		t.Fatal("delete")
	}
	got := s.getDiagram(u, d.ID)
	if got == nil || got.FolderID != nil {
		t.Fatalf("diagram should survive with null folder: %+v", got)
	}
}

func TestDiagramCRUDAndPositions(t *testing.T) {
	s := newTestStore(t)
	u := testUser(t, s)
	d := s.createDiagram(u, diagramInput{Name: "D", State: "s"})
	if d.Favorite || len(d.Positions) != 0 {
		t.Fatalf("defaults: %+v", d)
	}
	// set positions
	patched, err := s.updateDiagram(u, d.ID, diagramPatch{Positions: positionsValue{
		Set:   true,
		Value: map[string]NodeOffset{"A": {Dx: 1.5, Dy: -2}},
	}})
	if err != nil || patched.Positions["A"].Dx != 1.5 {
		t.Fatalf("patch positions: %+v err=%v", patched, err)
	}
	// clear positions via explicit null
	patched, err = s.updateDiagram(u, d.ID, diagramPatch{Positions: positionsValue{Set: true}})
	if err != nil || len(patched.Positions) != 0 {
		t.Fatalf("clear positions: %+v err=%v", patched, err)
	}
	// favorite + name
	fav := true
	patched, err = s.updateDiagram(u, d.ID, diagramPatch{Favorite: &fav, Name: strPtr("N")})
	if err != nil || !patched.Favorite || patched.Name != "N" {
		t.Fatalf("patch favorite/name: %+v err=%v", patched, err)
	}
}

func TestListDiagramsFilters(t *testing.T) {
	s := newTestStore(t)
	u := testUser(t, s)
	f := s.createFolder(u, "F", nil)
	a := s.createDiagram(u, diagramInput{Name: "alpha", State: "s"})
	b := s.createDiagram(u, diagramInput{Name: "beta", State: "s", FolderID: &f.ID})
	fav := true
	s.updateDiagram(u, b.ID, diagramPatch{Favorite: &fav})
	if got := s.listDiagrams(u, diagramQuery{}); len(got) != 2 {
		t.Fatalf("list all: %d", len(got))
	}
	if got := s.listDiagrams(u, diagramQuery{FolderID: "root"}); len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("root filter: %+v", got)
	}
	if got := s.listDiagrams(u, diagramQuery{FolderID: f.ID}); len(got) != 1 || got[0].ID != b.ID {
		t.Fatalf("folder filter: %+v", got)
	}
	if got := s.listDiagrams(u, diagramQuery{Favorite: true}); len(got) != 1 || got[0].ID != b.ID {
		t.Fatalf("favorite filter: %+v", got)
	}
	if got := s.listDiagrams(u, diagramQuery{Search: "alp"}); len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("search filter: %+v", got)
	}
}

func TestIsSelfOrAncestor(t *testing.T) {
	s := newTestStore(t)
	u := testUser(t, s)
	root := s.createFolder(u, "root", nil)
	mid := s.createFolder(u, "mid", &root.ID)
	if !s.isSelfOrAncestor(u, root.ID, mid.ID) {
		t.Fatal("root is ancestor of mid")
	}
	if s.isSelfOrAncestor(u, mid.ID, root.ID) {
		t.Fatal("mid is not ancestor of root")
	}
}

func TestImportAll(t *testing.T) {
	s := newTestStore(t)
	u := testUser(t, s)
	res, err := s.importAll(u, importPayload{
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
	if s.getDiagram(u, "d1") == nil || *s.getDiagram(u, "d1").FolderID != "f1" {
		t.Fatal("imported diagram missing or wrong folder")
	}
}

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

func TestOpenDBIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	s1, err := openDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	u := testUser(t, s1)
	s1.createFolder(u, "F", nil)
	s1.Close()
	s2, err := openDB(dir) // second open: migrations must no-op, data persists
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if len(s2.listFolders(u)) != 1 {
		t.Fatal("data lost on reopen")
	}
}
