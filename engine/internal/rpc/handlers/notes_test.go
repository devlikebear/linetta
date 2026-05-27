package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type noteFix struct {
	store  *store.Store
	nr     *note.Repo
	nodeID string
}

func newNoteFixture(t *testing.T) noteFix {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	return noteFix{store: s, nr: note.NewRepo(s), nodeID: *p.LastOpenedNodeID}
}

func TestCreateNoteHandler(t *testing.T) {
	f := newNoteFixture(t)
	res, err := CreateNote(f.nr, func() int64 { return 9000 })(context.Background(),
		json.RawMessage(`{"node_id":"`+f.nodeID+`","anchor":12,"body":"여기 톤 바꾸기"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var n note.Note
	_ = json.Unmarshal(res, &n)
	if n.Anchor != 12 || n.Body != "여기 톤 바꾸기" || n.CreatedAt != 9000 {
		t.Errorf("note = %+v", n)
	}
}

func TestListNotesForNodeHandler(t *testing.T) {
	f := newNoteFixture(t)
	_, _ = f.nr.Create(context.Background(), note.NewInput{NodeID: f.nodeID, Anchor: 5, Body: "A"}, 1)
	_, _ = f.nr.Create(context.Background(), note.NewInput{NodeID: f.nodeID, Anchor: 1, Body: "B"}, 2)
	res, err := ListNotesForNode(f.nr)(context.Background(),
		json.RawMessage(`{"node_id":"`+f.nodeID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var list []note.Note
	_ = json.Unmarshal(res, &list)
	if len(list) != 2 || list[0].Body != "B" || list[1].Body != "A" {
		t.Errorf("order = %+v", list)
	}
}

func TestGetNoteHandler(t *testing.T) {
	f := newNoteFixture(t)
	n, _ := f.nr.Create(context.Background(), note.NewInput{NodeID: f.nodeID, Anchor: 0, Body: "메모"}, 1)
	res, err := GetNote(f.nr)(context.Background(),
		json.RawMessage(`{"id":"`+n.ID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got note.Note
	_ = json.Unmarshal(res, &got)
	if got.Body != "메모" {
		t.Errorf("got = %+v", got)
	}
}

func TestUpdateNoteHandler(t *testing.T) {
	f := newNoteFixture(t)
	n, _ := f.nr.Create(context.Background(), note.NewInput{NodeID: f.nodeID, Anchor: 3, Body: "원본"}, 1)
	res, err := UpdateNote(f.nr)(context.Background(),
		json.RawMessage(`{"id":"`+n.ID+`","body":"수정됨"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got note.Note
	_ = json.Unmarshal(res, &got)
	if got.Body != "수정됨" || got.Anchor != 3 {
		t.Errorf("got = %+v", got)
	}
}

func TestDeleteNoteHandler(t *testing.T) {
	f := newNoteFixture(t)
	n, _ := f.nr.Create(context.Background(), note.NewInput{NodeID: f.nodeID, Anchor: 0, Body: "x"}, 1)
	if _, err := DeleteNote(f.nr)(context.Background(),
		json.RawMessage(`{"id":"`+n.ID+`"}`)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := f.nr.Get(context.Background(), n.ID); err == nil {
		t.Error("expected ErrNotFound after delete")
	}
}
