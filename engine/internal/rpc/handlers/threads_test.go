package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

type threadFix struct {
	store *store.Store
	tr    *thread.Repo
	pID   string
}

func newThreadFixture(t *testing.T) threadFix {
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
	return threadFix{store: s, tr: thread.NewRepo(s), pID: p.ID}
}

func TestCreateThreadHandler(t *testing.T) {
	f := newThreadFixture(t)
	res, err := CreateThread(f.tr)(context.Background(),
		json.RawMessage(`{"project_id":"`+f.pID+`","name":"잃어버린 시간","color":"#c08a3e"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var th thread.Thread
	_ = json.Unmarshal(res, &th)
	if th.Name != "잃어버린 시간" || th.Color != "#c08a3e" {
		t.Errorf("thread = %+v", th)
	}
}

func TestListThreadsHandler_filtersClosed(t *testing.T) {
	f := newThreadFixture(t)
	open, _ := f.tr.Create(context.Background(), thread.NewInput{ProjectID: f.pID, Name: "열린"})
	closed, _ := f.tr.Create(context.Background(), thread.NewInput{ProjectID: f.pID, Name: "닫힌"})
	_ = f.tr.Close(context.Background(), closed.ID, 1000)

	res, err := ListThreads(f.tr)(context.Background(),
		json.RawMessage(`{"project_id":"`+f.pID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var list []thread.Thread
	_ = json.Unmarshal(res, &list)
	if len(list) != 1 || list[0].ID != open.ID {
		t.Errorf("default list = %+v", list)
	}

	res2, _ := ListThreads(f.tr)(context.Background(),
		json.RawMessage(`{"project_id":"`+f.pID+`","include_closed":true}`))
	var all []thread.Thread
	_ = json.Unmarshal(res2, &all)
	if len(all) != 2 {
		t.Errorf("include_closed = %d", len(all))
	}
}

func TestUpdateThreadHandler(t *testing.T) {
	f := newThreadFixture(t)
	th, _ := f.tr.Create(context.Background(), thread.NewInput{ProjectID: f.pID, Name: "원본"})
	res, err := UpdateThread(f.tr)(context.Background(),
		json.RawMessage(`{"id":"`+th.ID+`","name":"새 이름","summary":"한 줄"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got thread.Thread
	_ = json.Unmarshal(res, &got)
	if got.Name != "새 이름" || got.Summary != "한 줄" {
		t.Errorf("update missed: %+v", got)
	}
}

func TestCloseAndReopenHandlers(t *testing.T) {
	f := newThreadFixture(t)
	th, _ := f.tr.Create(context.Background(), thread.NewInput{ProjectID: f.pID, Name: "T"})
	if _, err := CloseThread(f.tr, func() int64 { return 2000 })(context.Background(),
		json.RawMessage(`{"id":"`+th.ID+`"}`)); err != nil {
		t.Fatalf("close: %v", err)
	}
	got, _ := f.tr.Get(context.Background(), th.ID)
	if got.ClosedAt == nil || *got.ClosedAt != 2000 {
		t.Errorf("ClosedAt = %v", got.ClosedAt)
	}
	if _, err := ReopenThread(f.tr)(context.Background(),
		json.RawMessage(`{"id":"`+th.ID+`"}`)); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got2, _ := f.tr.Get(context.Background(), th.ID)
	if got2.ClosedAt != nil {
		t.Errorf("ClosedAt after reopen = %v", got2.ClosedAt)
	}
}

func TestGetThreadHandler_notFound(t *testing.T) {
	f := newThreadFixture(t)
	_, err := GetThread(f.tr)(context.Background(), json.RawMessage(`{"id":"missing"}`))
	if err == nil {
		t.Error("expected error for missing id")
	}
}
