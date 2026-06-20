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
	"github.com/devlikebear/linetta/engine/internal/snapshot"
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
		snaps: snapshot.NewRepo(st),
		src:   toolConfigSource{},
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
	if !strings.Contains(schema, "set_scene_text") {
		t.Fatalf("schema should document scene body rewrite op:\n%s", schema)
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
	  {"op":"create_entity","ref":"e3","kind":"concept","name":"빛의 맹약","role":"마법","summary":"진실을 드러내는 계약 마법","attributes":{"효과":"거짓말을 하면 손등의 문장이 빛난다","비용":"사용자의 기억 한 조각"}},
	  {"op":"create_relationship","from_ref":"e1","to_ref":"e2","label":"조사한다"},
	  {"op":"create_relationship","from_ref":"e1","to_ref":"e3","label":"사용함"}
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
	if len(entities) != 3 {
		t.Fatalf("entities = %+v", entities)
	}
	magic, err := svc.entities.Get(ctx, entities[0].ID)
	if err != nil {
		t.Fatalf("get entity: %v", err)
	}
	for _, ent := range entities {
		if ent.Name == "빛의 맹약" {
			magic = ent
			break
		}
	}
	if magic.Name != "빛의 맹약" || magic.Kind != entity.KindConcept ||
		magic.Attributes["효과"] == "" || magic.Attributes["비용"] == "" {
		t.Fatalf("magic entity = %+v", magic)
	}
	rels, _ := svc.relationships.ListByProject(ctx, projectID)
	if len(rels) != 2 {
		t.Fatalf("relationships = %+v", rels)
	}
}

func TestLinettaApplyOpsToolRewritesCurrentSceneText(t *testing.T) {
	ctx := context.Background()
	svc, projectID, nodeID := newToolSvc(t)
	if err := svc.nodes.UpdateContent(ctx, nodeID, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"기존 본문"}]}]}`, 1_100); err != nil {
		t.Fatalf("seed content: %v", err)
	}
	reg := svc.buildToolRegistry(projectID, nodeID, func() int64 { return 2_000 }, "run-1", "이번 캐릭터와 플롯 고려해서 현재 씬 재작성해줘")
	tool, ok := reg.Get("linetta_apply_ops")
	if !ok {
		t.Fatal("linetta_apply_ops not registered")
	}

	opsJSON := `[
	  {"op":"set_scene_text","text":"새 본문 첫 줄\n새 본문 둘째 줄\n\n새 문단"}
	]`
	params := json.RawMessage(`{
	  "summary":"현재 씬 재작성",
	  "ops_json":` + strconv.Quote(opsJSON) + `
	}`)
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool result is error: %s", result.Text())
	}
	var applied ApplyOpsResult
	if err := json.Unmarshal([]byte(result.Text()), &applied); err != nil {
		t.Fatalf("decode result: %v\n%s", err, result.Text())
	}
	if applied.Applied != 1 || len(applied.ChangedNodes) != 1 {
		t.Fatalf("changed nodes missing from result: %+v", applied)
	}
	if applied.ChangedNodes[0].NodeID != nodeID ||
		applied.ChangedNodes[0].Op != "set_scene_text" ||
		applied.ChangedNodes[0].ContentVersion == 0 ||
		applied.ChangedNodes[0].CharCount == 0 ||
		!strings.Contains(applied.ChangedNodes[0].TextPreview, "새 본문 첫 줄") {
		t.Fatalf("unexpected changed node metadata: %+v", applied.ChangedNodes[0])
	}

	got, err := svc.nodes.Get(ctx, nodeID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if got.ContentDoc == nil {
		t.Fatal("content_doc is nil")
	}
	if !strings.Contains(*got.ContentDoc, `"새 본문 첫 줄"`) || !strings.Contains(*got.ContentDoc, `"hardBreak"`) || !strings.Contains(*got.ContentDoc, `"새 문단"`) {
		t.Fatalf("scene text was not rewritten as tiptap doc: %s", *got.ContentDoc)
	}
	if got.WordCount == 0 {
		t.Fatalf("word count should be recomputed: %+v", got)
	}
}

func TestLinettaApplyOpsToolSnapshotsBeforeSceneText(t *testing.T) {
	ctx := context.Background()
	svc, projectID, nodeID := newToolSvc(t)

	originalDoc, err := plainTextToTiptapDoc("원본 본문")
	if err != nil {
		t.Fatalf("plainTextToTiptapDoc: %v", err)
	}
	if err := svc.nodes.UpdateContent(ctx, nodeID, originalDoc, 500); err != nil {
		t.Fatalf("seed content: %v", err)
	}

	p := Proposal{Ops: []Op{{Type: "set_scene_text", Text: "AI가 바꾼 본문"}}}
	res := svc.ApplyOps(ctx, projectID, nodeID, p, func() int64 { return 1000 })
	if res.Applied != 1 || len(res.Failures) != 0 {
		t.Fatalf("apply set_scene_text failed: %+v", res)
	}

	snap, err := svc.snaps.LatestForNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("LatestForNode: %v", err)
	}
	if snap.Reason != snapshot.ReasonCompanionBefore {
		t.Errorf("reason = %q, want companion-before", snap.Reason)
	}
	if snap.ContentDoc != originalDoc {
		t.Errorf("snapshot did not capture pre-AI content; got %q", snap.ContentDoc)
	}
}

func TestLinettaApplyOpsToolRequiresSceneTextForSceneIntent(t *testing.T) {
	ctx := context.Background()
	svc, projectID, nodeID := newToolSvc(t)
	reg := svc.buildToolRegistry(projectID, nodeID, func() int64 { return 2_000 }, "run-1", "아니 1장 1씬 작성해달라고")
	tool, ok := reg.Get("linetta_apply_ops")
	if !ok {
		t.Fatal("linetta_apply_ops not registered")
	}

	opsJSON := `[
	  {"op":"create_thread","ref":"t1","name":"감정선","summary":"주인공이 좌절한다"}
	]`
	params := json.RawMessage(`{
	  "summary":"씬 작성",
	  "ops_json":` + strconv.Quote(opsJSON) + `
	}`)
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected scene intent without set_scene_text to fail: %s", result.Text())
	}
	if !strings.Contains(result.Text(), "set_scene_text") {
		t.Fatalf("error should tell the model to write scene text: %s", result.Text())
	}
}

func TestLinettaApplyOpsToolRejectsAccidentalEmptySceneText(t *testing.T) {
	ctx := context.Background()
	svc, projectID, nodeID := newToolSvc(t)
	reg := svc.buildToolRegistry(projectID, nodeID, func() int64 { return 2_000 }, "run-1", "현재 씬 본문 작성해줘")
	tool, ok := reg.Get("linetta_apply_ops")
	if !ok {
		t.Fatal("linetta_apply_ops not registered")
	}

	opsJSON := `[
	  {"op":"set_scene_text","text":"","allow_empty":true}
	]`
	params := json.RawMessage(`{
	  "summary":"현재 씬 작성",
	  "ops_json":` + strconv.Quote(opsJSON) + `
	}`)
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected accidental empty scene write to fail: %s", result.Text())
	}
	if !strings.Contains(result.Text(), "empty") && !strings.Contains(result.Text(), "비우") {
		t.Fatalf("error should explain empty scene text is not allowed: %s", result.Text())
	}
}

func TestLinettaApplyOpsToolRejectsWrongSceneTextTarget(t *testing.T) {
	ctx := context.Background()
	svc, projectID, nodeID := newToolSvc(t)
	reg := svc.buildToolRegistry(projectID, nodeID, func() int64 { return 2_000 }, "run-1", "현재 씬 본문 재작성해줘")
	tool, ok := reg.Get("linetta_apply_ops")
	if !ok {
		t.Fatal("linetta_apply_ops not registered")
	}

	opsJSON := `[
	  {"op":"set_scene_text","node_id":"other-node","text":"다른 곳에 쓰면 안 되는 본문"}
	]`
	params := json.RawMessage(`{
	  "summary":"현재 씬 재작성",
	  "ops_json":` + strconv.Quote(opsJSON) + `
	}`)
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected wrong scene target to fail: %s", result.Text())
	}
	if !strings.Contains(result.Text(), "current scene") {
		t.Fatalf("error should mention current scene target: %s", result.Text())
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

func TestApplyOpsCreateOutlineNodeIsIdempotentAtRoot(t *testing.T) {
	ctx := context.Background()
	svc, projectID, nodeID := newToolSvc(t)
	ops := []Op{
		{Type: "create_outline_node", Ref: "p1", Kind: node.KindContainer, Label: "1부", Title: "개별성의 경계선"},
		{Type: "create_outline_node", Ref: "c1", Kind: node.KindContainer, ParentNodeRef: "p1", Label: "1장", Title: "경계의 틈"},
		{Type: "create_outline_node", Ref: "s1", Kind: node.KindLeaf, ParentNodeRef: "c1", Label: "씬 1", Title: "조각난 아침"},
	}

	first := svc.ApplyOps(ctx, projectID, nodeID, Proposal{Summary: "아웃라인", Ops: ops}, func() int64 { return 10 })
	second := svc.ApplyOps(ctx, projectID, nodeID, Proposal{Summary: "아웃라인 재적용", Ops: ops}, func() int64 { return 20 })
	if len(first.Failures) > 0 || len(second.Failures) > 0 {
		t.Fatalf("unexpected failures: first=%+v second=%+v", first, second)
	}

	list, err := svc.nodes.ListByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	var parts []node.Node
	for _, n := range list {
		if n.Kind == node.KindContainer && n.Label == "1부" && n.ParentID == nil {
			parts = append(parts, n)
		}
	}
	if len(parts) != 1 {
		t.Fatalf("root part count = %d, nodes=%+v", len(parts), list)
	}
	part := parts[0]
	if part.ParentID != nil {
		t.Fatalf("root part should not be created under the current scene parent: %+v", part)
	}
	chapters := matchingChildren(list, part.ID, node.KindContainer, "1장")
	if len(chapters) != 1 {
		t.Fatalf("chapter count under part = %d, nodes=%+v", len(chapters), list)
	}
	scenes := matchingChildren(list, chapters[0].ID, node.KindLeaf, "씬 1")
	if len(scenes) != 1 {
		t.Fatalf("scene count under chapter = %d, nodes=%+v", len(scenes), list)
	}
	if first.Created["node:p1"] != second.Created["node:p1"] {
		t.Fatalf("reused ref should point to same id: first=%+v second=%+v", first.Created, second.Created)
	}
}

func matchingChildren(list []node.Node, parentID, kind, label string) []node.Node {
	var out []node.Node
	for _, n := range list {
		if n.ParentID == nil || *n.ParentID != parentID {
			continue
		}
		if n.Kind == kind && n.Label == label {
			out = append(out, n)
		}
	}
	return out
}

func TestApplyOpsOutlineMaintenanceOps(t *testing.T) {
	ctx := context.Background()
	svc, projectID, nodeID := newToolSvc(t)
	result := svc.ApplyOps(ctx, projectID, nodeID, Proposal{Summary: "초기", Ops: []Op{
		{Type: "create_outline_node", Ref: "p1", Kind: node.KindContainer, Label: "1부", Title: "낡은 제목"},
		{Type: "create_outline_node", Ref: "p2", Kind: node.KindContainer, Label: "2부", Title: "뒤쪽"},
	}}, func() int64 { return 10 })
	if len(result.Failures) > 0 {
		t.Fatalf("create failures: %+v", result)
	}
	part1 := result.Created["node:p1"]
	part2 := result.Created["node:p2"]

	result = svc.ApplyOps(ctx, projectID, nodeID, Proposal{Summary: "정리", Ops: []Op{
		{Type: "rename_outline_node", NodeID: part1, Label: "1부", Title: "새 제목"},
		{Type: "move_outline_node", NodeID: part2, Direction: "up"},
		{Type: "delete_outline_node", NodeID: part1},
	}}, func() int64 { return 20 })
	if len(result.Failures) > 0 {
		t.Fatalf("maintenance failures: %+v", result)
	}
	if _, err := svc.nodes.Get(ctx, part1); err != node.ErrNotFound {
		t.Fatalf("part1 should be deleted, err=%v", err)
	}
	got, err := svc.nodes.Get(ctx, part2)
	if err != nil {
		t.Fatalf("part2 get: %v", err)
	}
	if got.Ordinal != 1 {
		t.Fatalf("part2 should move above the initial root scene but keep deterministic order, got ordinal %d", got.Ordinal)
	}
}
