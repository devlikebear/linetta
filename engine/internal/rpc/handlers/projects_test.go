package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
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

type projectSynopsisFixture struct {
	projects *project.Repo
	nodes    *node.Repo
	builder  *ai.ContextBuilder
	project  project.Project
	root     node.Node
}

type fakeSynopsisRefresher struct {
	nodes *node.Repo
	text  string
}

func (r fakeSynopsisRefresher) RefreshNow(ctx context.Context, nodeID string) {
	n, err := r.nodes.Get(ctx, nodeID)
	if err != nil {
		return
	}
	_ = r.nodes.SetSummary(ctx, nodeID, r.text, n.ContentVersion)
}

func newProjectSynopsisFixture(t *testing.T) projectSynopsisFixture {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	projects := project.NewRepo(s)
	nodes := node.NewRepo(s)
	p, err := projects.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	root, err := nodes.CreateSibling(context.Background(), *p.LastOpenedNodeID, node.KindContainer, "1부", "", 1100)
	if err != nil {
		t.Fatalf("CreateSibling: %v", err)
	}
	builder := ai.NewContextBuilder(
		projects,
		nodes,
		mention.NewRepo(s),
		thread.NewRepo(s),
		beat.NewRepo(s),
		note.NewRepo(s),
		relationship.NewRepo(s),
	).WithSummaryRefresher(fakeSynopsisRefresher{nodes: nodes, text: "재작성된 시놉시스"})
	return projectSynopsisFixture{projects: projects, nodes: nodes, builder: builder, project: p, root: root}
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

func TestCreateProjectHandler_acceptsWebnovelOutlinePreset(t *testing.T) {
	repo := newRepo(t)
	h := CreateProject(repo, func() int64 { return 12345 })

	params := json.RawMessage(`{
		"title": "Serial",
		"genres": ["modern fantasy"],
		"length_target": "series",
		"default_pov": "third_limited",
		"outline_preset": "webnovel"
	}`)
	res, err := h(context.Background(), params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var p project.Project
	if err := json.Unmarshal(res, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.OutlinePreset != project.OutlinePresetWebNovel {
		t.Fatalf("outline_preset = %q, want %q", p.OutlinePreset, project.OutlinePresetWebNovel)
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

	_, err = h(context.Background(), json.RawMessage(`{
		"title": "Bad",
		"genres": ["SF"],
		"length_target": "short",
		"default_pov": "first",
		"outline_preset": "screenplay"
	}`))
	me, ok = err.(*rpc.MethodError)
	if !ok || me.Code != rpc.CodeInvalidParams {
		t.Fatalf("outline err = %v, want invalid params", err)
	}
}

func TestUpdateProjectHandler_episodeCharTarget(t *testing.T) {
	repo := newRepo(t)
	create := CreateProject(repo, func() int64 { return 1000 })
	res, err := create(context.Background(), json.RawMessage(`{
		"title": "Serial",
		"genres": ["fantasy"],
		"length_target": "series",
		"default_pov": "third_limited"
	}`))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var created project.Project
	if err := json.Unmarshal(res, &created); err != nil {
		t.Fatalf("unmarshal created: %v", err)
	}

	update := UpdateProject(repo, func() int64 { return 2000 })
	res, err = update(context.Background(), json.RawMessage(`{"id":"`+created.ID+`","episode_char_target":5500}`))
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	var got project.Project
	if err := json.Unmarshal(res, &got); err != nil {
		t.Fatalf("unmarshal updated: %v", err)
	}
	if got.EpisodeCharTarget != 5500 {
		t.Fatalf("episode_char_target = %d, want 5500", got.EpisodeCharTarget)
	}
	if got.UpdatedAt != 2000 {
		t.Fatalf("updated_at = %d, want 2000", got.UpdatedAt)
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

func TestRestoreProjectHandler(t *testing.T) {
	repo := newRepo(t)
	create := CreateProject(repo, func() int64 { return 1 })
	res, _ := create(context.Background(), json.RawMessage(`{"title":"x","genres":["SF"],"length_target":"short","default_pov":"first"}`))
	var created project.Project
	_ = json.Unmarshal(res, &created)

	arch := ArchiveProject(repo, func() int64 { return 99 })
	if _, err := arch(context.Background(), json.RawMessage(`{"id":"`+created.ID+`"}`)); err != nil {
		t.Fatalf("archive: %v", err)
	}
	restore := RestoreProject(repo, func() int64 { return 150 })
	if _, err := restore(context.Background(), json.RawMessage(`{"id":"`+created.ID+`"}`)); err != nil {
		t.Fatalf("restore: %v", err)
	}

	got, err := repo.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ArchivedAt != nil {
		t.Fatalf("archived_at = %v, want nil", *got.ArchivedAt)
	}
	if got.UpdatedAt != 150 {
		t.Fatalf("updated_at = %d, want 150", got.UpdatedAt)
	}
}

func TestDeleteProjectHandler(t *testing.T) {
	repo := newRepo(t)
	create := CreateProject(repo, func() int64 { return 1 })
	res, _ := create(context.Background(), json.RawMessage(`{"title":"x","genres":["SF"],"length_target":"short","default_pov":"first"}`))
	var created project.Project
	_ = json.Unmarshal(res, &created)

	cleanedID := ""
	del := DeleteProject(repo, func(_ context.Context, id string) error {
		cleanedID = id
		return nil
	})
	if _, err := del(context.Background(), json.RawMessage(`{"id":"`+created.ID+`"}`)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if cleanedID != created.ID {
		t.Fatalf("cleaned id = %q, want %q", cleanedID, created.ID)
	}

	get := GetProject(repo)
	_, err := get(context.Background(), json.RawMessage(`{"id":"`+created.ID+`"}`))
	me, ok := err.(*rpc.MethodError)
	if !ok || me.Code != rpc.CodeInvalidParams {
		t.Fatalf("get deleted err = %v, want invalid params", err)
	}
}

func TestRewriteProjectSynopsisHandler(t *testing.T) {
	f := newProjectSynopsisFixture(t)
	h := RewriteProjectSynopsis(f.projects, f.builder, func() int64 { return 2000 })

	res, err := h(context.Background(), json.RawMessage(`{"id":"`+f.project.ID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got project.Project
	if err := json.Unmarshal(res, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Synopsis != "재작성된 시놉시스" {
		t.Fatalf("synopsis = %q", got.Synopsis)
	}
	if got.Outline != "" {
		t.Fatalf("rewrite synopsis should not touch outline, got %q", got.Outline)
	}
}

func TestClearProjectSynopsisHandler(t *testing.T) {
	f := newProjectSynopsisFixture(t)
	body := "잘못된 시놉시스"
	if _, err := f.projects.Update(context.Background(), 1500, project.UpdateInput{ID: f.project.ID, Synopsis: &body}); err != nil {
		t.Fatalf("seed synopsis: %v", err)
	}
	h := ClearProjectSynopsis(f.projects, func() int64 { return 2000 })

	res, err := h(context.Background(), json.RawMessage(`{"id":"`+f.project.ID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got project.Project
	if err := json.Unmarshal(res, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Synopsis != "" {
		t.Fatalf("synopsis should be clear, got %q", got.Synopsis)
	}
}
