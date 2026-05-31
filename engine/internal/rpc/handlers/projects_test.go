package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newRepo(t *testing.T) *project.Repo {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return project.NewRepo(s)
}

func TestCreateProjectHandler(t *testing.T) {
	repo := newRepo(t)
	h := CreateProject(repo, func() int64 { return 12345 })

	params := json.RawMessage(`{
		"title": "Test",
		"genres": ["SF"],
		"length_target": "short",
		"default_pov": "first"
	}`)
	res, err := h(context.Background(), params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var p project.Project
	if err := json.Unmarshal(res, &p); err != nil {
		t.Fatalf("unmarshal: %v (raw=%s)", err, string(res))
	}
	if p.Title != "Test" {
		t.Errorf("title = %q", p.Title)
	}
	if p.CreatedAt != 12345 {
		t.Errorf("created_at = %d, want 12345 (clock injected)", p.CreatedAt)
	}
}

func TestCreateProjectHandler_invalidEnumsReturnInvalidParams(t *testing.T) {
	repo := newRepo(t)
	h := CreateProject(repo, func() int64 { return 1 })
	_, err := h(context.Background(), json.RawMessage(`{
		"title": "Bad",
		"genres": ["SF"],
		"length_target": "epic",
		"default_pov": "first"
	}`))
	me, ok := err.(*rpc.MethodError)
	if !ok || me.Code != rpc.CodeInvalidParams {
		t.Fatalf("err = %v, want invalid params", err)
	}
}

func TestListProjectsHandler(t *testing.T) {
	repo := newRepo(t)
	clock := int64(1000)
	create := CreateProject(repo, func() int64 {
		clock += 100
		return clock
	})
	for _, name := range []string{"A", "B", "C"} {
		params, _ := json.Marshal(project.NewInput{Title: name, Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first"})
		if _, err := create(context.Background(), params); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	list := ListProjects(repo)
	res, err := list(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var out []project.Project
	if err := json.Unmarshal(res, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("got %d projects, want 3", len(out))
	}
	if out[0].Title != "C" {
		t.Errorf("first = %q, want C (most recent)", out[0].Title)
	}
}

func TestArchiveAndGetProject(t *testing.T) {
	repo := newRepo(t)
	create := CreateProject(repo, func() int64 { return 1 })
	res, _ := create(context.Background(), json.RawMessage(`{"title":"x","genres":["SF"],"length_target":"short","default_pov":"first"}`))
	var created project.Project
	_ = json.Unmarshal(res, &created)

	arch := ArchiveProject(repo, func() int64 { return 99 })
	if _, err := arch(context.Background(), json.RawMessage(`{"id":"`+created.ID+`"}`)); err != nil {
		t.Fatalf("archive: %v", err)
	}

	get := GetProject(repo)
	gotRes, err := get(context.Background(), json.RawMessage(`{"id":"`+created.ID+`"}`))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	var fetched project.Project
	_ = json.Unmarshal(gotRes, &fetched)
	if fetched.ArchivedAt == nil || *fetched.ArchivedAt != 99 {
		t.Errorf("archived_at = %v, want 99", fetched.ArchivedAt)
	}
}
