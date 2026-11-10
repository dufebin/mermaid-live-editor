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
