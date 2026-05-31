package companion

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

type toolConfigSource struct{}

func (toolConfigSource) Provider() string          { return "openai-codex" }
func (toolConfigSource) WebSearchProvider() string { return "brave" }
func (toolConfigSource) WebSearchAPIKey() string   { return "test-key" }

func newToolSvc(t *testing.T) (*Service, string, string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	threads := thread.NewRepo(st)
	beats := beat.NewRepo(st)
	entities := entity.NewRepo(st)
	rels := relationship.NewRepo(st)
	pb := plot.NewBuilder(nodes, beats, threads)
	svc := &Service{
		projects: projects, threads: threads, entities: entities,
		relationships: rels, plot: pb, nodes: nodes, beats: beats,
		src: toolConfigSource{},
	}
	p, err := projects.Create(ctx, 1_000, project.NewInput{
		Title: "도구 테스트", Genres: []string{"mystery"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return svc, p.ID, *p.LastOpenedNodeID
}

func TestBuildToolRegistryIncludesWebAndLinettaTools(t *testing.T) {
	svc, projectID, nodeID := newToolSvc(t)
	reg := svc.buildToolRegistry(projectID, nodeID, func() int64 { return 1 })
	var names []string
	for _, schema := range reg.Schemas() {
		names = append(names, schema.Function.Name)
	}
	got := strings.Join(names, ",")
	for _, want := range []string{"linetta_apply_ops", "web_fetch", "web_search"} {
		if !strings.Contains(got, want) {
			t.Fatalf("tool schemas %q missing %s", got, want)
		}
	}
}

func TestApplyOpsSchemaUsesOpsJSONString(t *testing.T) {
	schema := string(applyOpsSchema())
	if !strings.Contains(schema, `"ops_json"`) {
		t.Fatalf("schema should expose ops_json string input:\n%s", schema)
	}
	if strings.Contains(schema, `"ops":`) {
		t.Fatalf("schema should not expose nested ops object that strict providers expand poorly:\n%s", schema)
	}
}

func TestLinettaApplyOpsToolMutatesProjectStructure(t *testing.T) {
	ctx := context.Background()
	svc, projectID, nodeID := newToolSvc(t)
	reg := svc.buildToolRegistry(projectID, nodeID, func() int64 { return 2_000 })
	tool, ok := reg.Get("linetta_apply_ops")
	if !ok {
		t.Fatal("linetta_apply_ops not registered")
	}

	opsJSON := `[
	  {"op":"set_outline","outline":"항구 도시의 미스터리"},
	  {"op":"create_thread","ref":"t1","name":"메인 미스터리","summary":"등대의 비밀"},
	  {"op":"add_beat","thread_ref":"t1","label":"단서 발견","description":"주인공이 낡은 열쇠를 줍는다","intensity":2},
	  {"op":"create_entity","ref":"e1","kind":"character","name":"하나","role":"탐정","summary":"조용한 관찰자"},
	  {"op":"create_entity","ref":"e2","kind":"place","name":"붉은 등대","summary":"항구 끝의 금지된 장소"},
	  {"op":"create_relationship","from_ref":"e1","to_ref":"e2","label":"조사한다"}
	]`
	params := json.RawMessage(`{
	  "summary":"세계관 정리",
	  "ops_json":` + strconv.Quote(opsJSON) + `
	}`)
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool result is error: %s", result.Text())
	}

	proj, err := svc.projects.Get(ctx, projectID)
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	if proj.Outline != "항구 도시의 미스터리" {
		t.Fatalf("outline = %q", proj.Outline)
	}
	threads, _ := svc.threads.ListByProject(ctx, projectID, false)
	if len(threads) != 1 || threads[0].Summary != "등대의 비밀" {
		t.Fatalf("threads = %+v", threads)
	}
	beats, _ := svc.beats.ListByThread(ctx, threads[0].ID)
	if len(beats) != 1 || beats[0].NodeID == nil || *beats[0].NodeID != nodeID {
		t.Fatalf("beats = %+v", beats)
	}
	entities, _ := svc.entities.Search(ctx, projectID, "", 10)
	if len(entities) != 2 {
		t.Fatalf("entities = %+v", entities)
	}
	rels, _ := svc.relationships.ListByProject(ctx, projectID)
	if len(rels) != 1 || rels[0].Label != "조사한다" {
		t.Fatalf("relationships = %+v", rels)
	}
}
