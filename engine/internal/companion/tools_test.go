package companion

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

type toolConfigSource struct{}

func (toolConfigSource) Resolve() ai.ResolvedProvider {
	return ai.ResolvedProvider{Provider: "openai-codex"}
}
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
	facts := fact.NewRepo(st)
	pb := plot.NewBuilder(nodes, beats, threads)
	svc := &Service{
		projects: projects, threads: threads, entities: entities,
		relationships: rels, facts: facts, plot: pb, nodes: nodes, beats: beats,
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

func TestLinettaApplyOpsToolCreatesFactCard(t *testing.T) {
	ctx := context.Background()
	svc, projectID, nodeID := newToolSvc(t)
	reg := svc.buildToolRegistry(projectID, nodeID, func() int64 { return 3_000 })
	tool, ok := reg.Get("linetta_apply_ops")
	if !ok {
		t.Fatal("linetta_apply_ops not registered")
	}

	opsJSON := `[
	  {"op":"create_fact_card","claim":"런던 일반 경찰은 항상 총기를 휴대한다","result":"일반 경찰은 통상 비무장 근무이며 무장 경찰은 별도 단위다.","status":"verified","category":"police","sources":[{"url":"https://www.met.police.uk/","title":"Met Police","snippet":"official reference"}]}
	]`
	params := json.RawMessage(`{
	  "summary":"자료집 카드 저장",
	  "ops_json":` + strconv.Quote(opsJSON) + `
	}`)
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool result is error: %s", result.Text())
	}

	list, err := svc.facts.List(ctx, fact.ListFilter{ProjectID: projectID, NodeID: &nodeID})
	if err != nil {
		t.Fatalf("facts.List: %v", err)
	}
	if len(list) != 1 || list[0].Claim == "" || len(list[0].Sources) != 1 {
		t.Fatalf("facts = %+v", list)
	}
	if list[0].NodeID == nil || *list[0].NodeID != nodeID {
		t.Fatalf("fact should link to current node %s: %+v", nodeID, list[0])
	}
}

func TestLinettaApplyOpsToolCreatesFactCardWithStringAccessedAt(t *testing.T) {
	ctx := context.Background()
	svc, projectID, nodeID := newToolSvc(t)
	reg := svc.buildToolRegistry(projectID, nodeID, func() int64 { return 3_000 })
	tool, ok := reg.Get("linetta_apply_ops")
	if !ok {
		t.Fatal("linetta_apply_ops not registered")
	}

	opsJSON := `[
	  {"op":"create_fact_card","claim":"비 온 뒤 흙냄새","result":"페트리코어와 지오스민이 관련된다.","status":"verified","sources":[{"url":"https://example.com/petrichor","accessed_at":""},{"url":"https://example.com/geosmin","accessed_at":"123"}]}
	]`
	params := json.RawMessage(`{
	  "summary":"자료집 카드 저장",
	  "ops_json":` + strconv.Quote(opsJSON) + `
	}`)
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool result is error: %s", result.Text())
	}

	list, err := svc.facts.List(ctx, fact.ListFilter{ProjectID: projectID, NodeID: &nodeID})
	if err != nil {
		t.Fatalf("facts.List: %v", err)
	}
	if len(list) != 1 || len(list[0].Sources) != 2 {
		t.Fatalf("facts = %+v", list)
	}
	if list[0].Sources[0].AccessedAt != 3_000 {
		t.Fatalf("empty accessed_at should fall back to now, got %d", list[0].Sources[0].AccessedAt)
	}
	if list[0].Sources[1].AccessedAt != 123 {
		t.Fatalf("numeric accessed_at string should persist, got %d", list[0].Sources[1].AccessedAt)
	}
}

func TestLinettaApplyOpsToolRejectsFactCardWithoutSource(t *testing.T) {
	ctx := context.Background()
	svc, projectID, nodeID := newToolSvc(t)
	reg := svc.buildToolRegistry(projectID, nodeID, func() int64 { return 3_000 })
	tool, ok := reg.Get("linetta_apply_ops")
	if !ok {
		t.Fatal("linetta_apply_ops not registered")
	}

	opsJSON := `[{"op":"create_fact_card","claim":"출처 없는 주장","result":"검증됨","status":"verified"}]`
	params := json.RawMessage(`{
	  "summary":"잘못된 자료집 카드",
	  "ops_json":` + strconv.Quote(opsJSON) + `
	}`)
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Fatalf("source-less fact card should be rejected: %s", result.Text())
	}
	list, err := svc.facts.List(ctx, fact.ListFilter{ProjectID: projectID})
	if err != nil {
		t.Fatalf("facts.List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("source-less fact card should not persist: %+v", list)
	}
}

func TestLinettaApplyOpsToolRejectsPlotOnlyOpsForOutlineRequest(t *testing.T) {
	ctx := context.Background()
	svc, projectID, nodeID := newToolSvc(t)
	reg := svc.buildToolRegistry(projectID, nodeID, func() int64 { return 2_000 }, "run-1", "아웃라인 작성해줘")
	tool, ok := reg.Get("linetta_apply_ops")
	if !ok {
		t.Fatal("linetta_apply_ops not registered")
	}

	opsJSON := `[
	  {"op":"create_thread","ref":"t1","name":"메인 플롯"},
	  {"op":"add_beat","thread_ref":"t1","label":"첫 충돌","description":"화신들이 처음 맞선다"}
	]`
	params := json.RawMessage(`{
	  "summary":"아웃라인 작성",
	  "ops_json":` + strconv.Quote(opsJSON) + `
	}`)
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Fatalf("plot-only outline request should be rejected: %s", result.Text())
	}
	if !strings.Contains(result.Text(), "left outline tree") {
		t.Fatalf("error should explain outline tree mismatch: %s", result.Text())
	}
	threads, _ := svc.threads.ListByProject(ctx, projectID, false)
	if len(threads) != 0 {
		t.Fatalf("plot-only rejected request should not create threads: %+v", threads)
	}
}

func TestLinettaApplyOpsToolCreatesOutlineTree(t *testing.T) {
	ctx := context.Background()
	svc, projectID, nodeID := newToolSvc(t)
	reg := svc.buildToolRegistry(projectID, nodeID, func() int64 { return 2_000 }, "run-1", "물과 불의 화신들이 펼치는 판타지 히어로 아웃라인 작성해줘")
	tool, ok := reg.Get("linetta_apply_ops")
	if !ok {
		t.Fatal("linetta_apply_ops not registered")
	}

	opsJSON := `[
	  {"op":"create_outline_node","ref":"p1","kind":"container","label":"1부","title":"불씨와 파도"},
	  {"op":"create_outline_node","ref":"c1","kind":"container","parent_node_ref":"p1","label":"1장","title":"두 화신의 조우"},
	  {"op":"create_outline_node","ref":"s1","kind":"leaf","parent_node_ref":"c1","label":"씬 1","title":"국경의 첫 충돌"},
	  {"op":"create_thread","ref":"t1","name":"물과 불의 전쟁"},
	  {"op":"add_beat","thread_ref":"t1","node_ref":"s1","label":"첫 충돌","description":"오롯과 카엘이 전장에서 맞선다"}
	]`
	params := json.RawMessage(`{
	  "summary":"아웃라인 트리 작성",
	  "ops_json":` + strconv.Quote(opsJSON) + `
	}`)
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool result is error: %s", result.Text())
	}

	nodes, err := svc.nodes.ListByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	byLabel := map[string]node.Node{}
	for _, n := range nodes {
		byLabel[n.Label] = n
	}
	part := byLabel["1부"]
	chapter := byLabel["1장"]
	scene := byLabel["씬 1"]
	if part.ID == "" || chapter.ID == "" || scene.ID == "" {
		t.Fatalf("missing outline nodes: %+v", byLabel)
	}
	if part.Kind != node.KindContainer || chapter.Kind != node.KindContainer || scene.Kind != node.KindLeaf {
		t.Fatalf("unexpected outline kinds: part=%s chapter=%s scene=%s", part.Kind, chapter.Kind, scene.Kind)
	}
	if chapter.ParentID == nil || *chapter.ParentID != part.ID {
		t.Fatalf("chapter parent = %v, want %s", chapter.ParentID, part.ID)
	}
	if scene.ParentID == nil || *scene.ParentID != chapter.ID {
		t.Fatalf("scene parent = %v, want %s", scene.ParentID, chapter.ID)
	}
	threads, _ := svc.threads.ListByProject(ctx, projectID, false)
	beats, _ := svc.beats.ListByThread(ctx, threads[0].ID)
	if len(beats) != 1 || beats[0].NodeID == nil || *beats[0].NodeID != scene.ID {
		t.Fatalf("beat should attach to created outline scene %s: %+v", scene.ID, beats)
	}
}
