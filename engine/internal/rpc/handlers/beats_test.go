package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

type beatFix struct {
	store *store.Store
	br    *beat.Repo
	tr    *thread.Repo
	pID   string
	thID  string
	nID   string
}

func newBeatFixture(t *testing.T) beatFix {
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
	tr := thread.NewRepo(s)
	th, _ := tr.Create(context.Background(), thread.NewInput{ProjectID: p.ID, Name: "T"})
	return beatFix{store: s, br: beat.NewRepo(s), tr: tr, pID: p.ID, thID: th.ID, nID: *p.LastOpenedNodeID}
}

func TestCreateBeatHandler(t *testing.T) {
	f := newBeatFixture(t)
	params := json.RawMessage(`{"thread_id":"` + f.thID + `","node_id":"` + f.nID + `","label":"첫 마디","intensity":2}`)
	res, err := CreateBeat(f.br)(context.Background(), params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var b beat.Beat
	_ = json.Unmarshal(res, &b)
	if b.Label != "첫 마디" || b.Intensity != 2 || b.Ordinal != 1 {
		t.Errorf("beat = %+v", b)
	}
}

func TestListByThreadHandler(t *testing.T) {
	f := newBeatFixture(t)
	_, _ = f.br.Create(context.Background(), beat.NewInput{ThreadID: f.thID, Label: "A"})
	_, _ = f.br.Create(context.Background(), beat.NewInput{ThreadID: f.thID, Label: "B"})
	res, err := ListBeatsByThread(f.br)(context.Background(),
		json.RawMessage(`{"thread_id":"`+f.thID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var list []beat.Beat
	_ = json.Unmarshal(res, &list)
	if len(list) != 2 {
		t.Errorf("len = %d", len(list))
	}
}

func TestListByNodeHandler(t *testing.T) {
	f := newBeatFixture(t)
	_, _ = f.br.Create(context.Background(), beat.NewInput{ThreadID: f.thID, NodeID: &f.nID, Label: "bound"})
	_, _ = f.br.Create(context.Background(), beat.NewInput{ThreadID: f.thID, Label: "unbound"})
	res, _ := ListBeatsByNode(f.br)(context.Background(),
		json.RawMessage(`{"node_id":"`+f.nID+`"}`))
	var list []beat.Beat
	_ = json.Unmarshal(res, &list)
	if len(list) != 1 || list[0].Label != "bound" {
		t.Errorf("list = %+v", list)
	}
}

func TestUpdateBeatHandler(t *testing.T) {
	f := newBeatFixture(t)
	b, _ := f.br.Create(context.Background(), beat.NewInput{ThreadID: f.thID, Label: "원본"})
	if _, err := UpdateBeat(f.br)(context.Background(),
		json.RawMessage(`{"id":"`+b.ID+`","label":"수정","intensity":3}`)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	got, _ := f.br.Get(context.Background(), b.ID)
	if got.Label != "수정" || got.Intensity != 3 {
		t.Errorf("update missed: %+v", got)
	}
}

func TestReorderBeatsHandler(t *testing.T) {
	f := newBeatFixture(t)
	b1, _ := f.br.Create(context.Background(), beat.NewInput{ThreadID: f.thID, Label: "1"})
	b2, _ := f.br.Create(context.Background(), beat.NewInput{ThreadID: f.thID, Label: "2"})
	params := json.RawMessage(`{"thread_id":"` + f.thID + `","ids":["` + b2.ID + `","` + b1.ID + `"]}`)
	if _, err := ReorderBeats(f.br)(context.Background(), params); err != nil {
		t.Fatalf("handler: %v", err)
	}
	got, _ := f.br.ListByThread(context.Background(), f.thID)
	if got[0].ID != b2.ID || got[1].ID != b1.ID {
		t.Errorf("post-reorder = %+v", got)
	}
}

func TestDeleteBeatHandler(t *testing.T) {
	f := newBeatFixture(t)
	b, _ := f.br.Create(context.Background(), beat.NewInput{ThreadID: f.thID, Label: "X"})
	if _, err := DeleteBeat(f.br)(context.Background(),
		json.RawMessage(`{"id":"`+b.ID+`"}`)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if _, err := f.br.Get(context.Background(), b.ID); err == nil {
		t.Error("not deleted")
	}
}
