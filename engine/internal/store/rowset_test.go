package store

import (
	"context"
	"path/filepath"
	"testing"
)

func newRowSetStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if _, err := s.DB().Exec(`
CREATE TABLE parents (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, picture BLOB, note TEXT);
CREATE TABLE others (id TEXT PRIMARY KEY);
CREATE TABLE children (
  id TEXT PRIMARY KEY,
  parent_id TEXT NOT NULL REFERENCES parents(id) ON DELETE CASCADE,
  other_id TEXT NOT NULL REFERENCES others(id) ON DELETE CASCADE
);
INSERT INTO parents VALUES ('p', '하린', x'00ff', NULL);
INSERT INTO others VALUES ('o1'), ('o2');
INSERT INTO children VALUES ('c1', 'p', 'o1'), ('c2', 'p', 'o2');`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return s
}

func captureParent(t *testing.T, s *Store) RowSet {
	t.Helper()
	set, err := s.Capture(context.Background(),
		Slice{Table: "parents", Where: "id = ?", Args: []any{"p"}},
		Slice{Table: "children", Where: "parent_id = ?", Args: []any{"p"}},
	)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	return set
}

func TestRowSetPutsBackWhatACascadeTook(t *testing.T) {
	ctx := context.Background()
	s := newRowSetStore(t)
	set := captureParent(t, s)
	if got := set.Count("children"); got != 2 {
		t.Fatalf("Count(children) = %d, want 2", got)
	}
	if _, err := s.DB().Exec(`DELETE FROM parents WHERE id = 'p'`); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := set.Restore(ctx); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	var picture []byte
	var note *string
	if err := s.DB().QueryRow(`SELECT picture, note FROM parents WHERE id = 'p'`).Scan(&picture, &note); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(picture) != 2 || picture[1] != 0xff || note != nil {
		t.Errorf("row came back as picture=%x note=%v", picture, note)
	}
	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM children`).Scan(&n); err != nil || n != 2 {
		t.Errorf("children = %d (%v), want 2", n, err)
	}
}

// A relationship's far end can be gone by the time the record comes back, and
// that must not cost the writer the record.
func TestRowSetDropsADependentWhoseOtherParentIsGone(t *testing.T) {
	ctx := context.Background()
	s := newRowSetStore(t)
	set := captureParent(t, s)
	if _, err := s.DB().Exec(`DELETE FROM parents WHERE id = 'p'; DELETE FROM others WHERE id = 'o2'`); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := set.Restore(ctx); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	var id string
	if err := s.DB().QueryRow(`SELECT id FROM children`).Scan(&id); err != nil || id != "c1" {
		t.Errorf("surviving child = %q (%v), want c1", id, err)
	}
}

func TestRowSetFailsWholeWhenTheRecordCannotComeBack(t *testing.T) {
	ctx := context.Background()
	s := newRowSetStore(t)
	set := captureParent(t, s)
	if _, err := s.DB().Exec(`DELETE FROM parents WHERE id = 'p'; INSERT INTO parents (id, name) VALUES ('q', '하린')`); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := set.Restore(ctx); err == nil {
		t.Fatal("Restore succeeded with the name taken")
	}
	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM children`).Scan(&n); err != nil || n != 0 {
		t.Errorf("children after a failed restore = %d (%v), want 0", n, err)
	}
}
