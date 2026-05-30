# 플롯 스파인 (Plot Spine) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 작품 개요와 씬 단위 플롯 비트를 1급 데이터로 만들고, AI 컨텍스트를 "개요 + 전/현/후 씬 플롯 + 등장 엔티티·관계 + 현재 씬 본문"이라는 상한 고정 플롯 스파인으로 재편한다.

**Architecture:** 기존 `threads`/`beats`를 보강한다. `beats`에 `description`, `projects`에 `outline` 컬럼을 추가하고, 전/현/후 leaf의 beat를 모으는 공유 `plot` 패키지를 신설한다. AI 컨텍스트 빌더는 무거운 계층 요약(다른 부/장, 같은 장)을 걷어내고 개요+플롯 스파인+관계로 재편한다. 집필 화면 우측 패널은 `PlotPanel`로 교체한다.

**Tech Stack:** Go 1.26 엔진(modernc.org/sqlite, stdio JSONRPC), React 18 + Vite + TS 프런트엔드, Tauri 2.

---

## 사전 지식 (구현자 필독)

- 엔진 테스트: 패키지 디렉터리에서 `go test ./...` (repo root: `engine/`). 테스트 헬퍼로 임시 SQLite를 여는 패턴이 각 `*_test.go`에 이미 있으니 같은 패키지의 기존 테스트를 먼저 읽고 그 헬퍼(예: 임시 store 생성)를 재사용할 것.
- 마이그레이션은 `engine/internal/store/migrations/*.sql`에 두면 `//go:embed migrations/*.sql` + 파일명 정렬로 자동 적용된다(`engine/internal/store/migrations.go`). 파일명만 올바르면 등록 코드는 필요 없다.
- LSP가 테스트 작성 직후 "undefined" 류 진단을 잠깐 보일 수 있다. **항상 실제 `go test` / `npx tsc --noEmit` 출력만 신뢰**하고 LSP 힌트는 무시한다.
- 프런트엔드는 테스트 인프라가 없다. FE 검증 = `cd apps/desktop && npx tsc --noEmit` 클린 + 수동 스모크.
- 빌드 검증(엔진 바이너리)은 **repo root**(`/Users/changheonshin/workspace/myworks/linetta`)에서 `./build-engine.sh` 실행. 하위 디렉터리에서 실행하면 경로 오류.
- 커밋은 각 Task 끝에서. `--no-verify` 금지, 훅 우회 금지. push는 하지 않는다(별도 지시 시에만).
- 현재 브랜치는 `main`. 안전 태그 `pre-greenfield-20260525` 존재. 그대로 main에서 작업한다(기존 플랜과 동일).

## File Structure

**엔진 (Go):**
- Create: `engine/internal/store/migrations/0005_plot_spine.sql` — beats.description, projects.outline 컬럼.
- Modify: `engine/internal/beat/beat.go` — Beat/NewInput/UpdateInput에 description.
- Modify: `engine/internal/beat/repo.go` — Create/Update/baseSelect/scan에 description.
- Modify: `engine/internal/project/project.go` — Project.Outline, UpdateInput 신설.
- Modify: `engine/internal/project/repo.go` — baseSelect/scan/Create에 outline, Update 메서드 신설.
- Modify: `engine/internal/relationship/repo.go` — ListByProject 신설.
- Create: `engine/internal/plot/plot.go` — Spine/SceneBeats/Beat 타입.
- Create: `engine/internal/plot/builder.go` — Builder.Build(전/현/후 leaf + beat 엔리치).
- Create: `engine/internal/plot/builder_test.go` — Builder 테스트.
- Create: `engine/internal/rpc/handlers/plot.go` — plot.spine_panel 핸들러.
- Modify: `engine/internal/rpc/handlers/projects.go` — UpdateProject 핸들러.
- Modify: `engine/internal/ai/ai.go` — Context/PreviewCounts/타입 재편.
- Modify: `engine/internal/ai/context.go` — outline + plot spine + 관계 + 슬림화.
- Modify: `engine/internal/ai/prompts.go` — 개요/플롯/관계 렌더, 제거 섹션 삭제.
- Modify: `engine/cmd/linetta-engine/main.go` — ContextBuilder 인자 + plot.spine_panel/projects.update 등록.

**프런트엔드 (TS/React):**
- Modify: `apps/desktop/src/lib/types.ts` — Project.outline, Beat.description, 입력 타입, 스파인 패널 타입, ContextCounts 재편.
- Modify: `apps/desktop/src/lib/rpc.ts` — projects.update, plot.spinePanel, previewContext 매핑.
- Create: `apps/desktop/src/components/PlotPanel.tsx` — 인라인 플롯 패널.
- Create: `apps/desktop/src/components/PlotPanel.css` — 패널 스타일.
- Modify: `apps/desktop/src/components/ContextPanel.tsx` — ActiveThreadsPanel → PlotPanel.
- Modify: `apps/desktop/src/components/ThreadSheet.tsx` — beat description textarea.
- Modify: `apps/desktop/src/components/ai/AIContextChecklist.tsx` — 항목 재편.
- Modify: `apps/desktop/src/routes/Workspace.tsx` — ContextPanel props(onProjectChanged), 정리.
- Delete: `apps/desktop/src/components/ActiveThreadsPanel.tsx` — PlotPanel이 완전 대체.

---

## Task 1: 데이터 계층 — 마이그레이션 + beat.description + project.outline + relationship.ListByProject

**Files:**
- Create: `engine/internal/store/migrations/0005_plot_spine.sql`
- Modify: `engine/internal/beat/beat.go`
- Modify: `engine/internal/beat/repo.go`
- Modify: `engine/internal/project/project.go`
- Modify: `engine/internal/project/repo.go`
- Modify: `engine/internal/relationship/repo.go`
- Test: `engine/internal/beat/repo_test.go`, `engine/internal/project/repo_test.go`, `engine/internal/relationship/repo_test.go` (기존 파일에 추가; 없으면 생성하되 같은 패키지 기존 테스트의 store 헬퍼 패턴을 따를 것)

- [ ] **Step 1: 마이그레이션 작성**

`engine/internal/store/migrations/0005_plot_spine.sql`:
```sql
-- Plan 24 플롯 스파인: beat에 서술 본문, project에 작가 편집 개요를 추가한다.
-- 둘 다 NOT NULL DEFAULT '' 이므로 기존 행에 안전하게 적용된다.
ALTER TABLE beats ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN outline TEXT NOT NULL DEFAULT '';
```

- [ ] **Step 2: beat 도메인 타입에 description 추가**

`engine/internal/beat/beat.go` — `Beat`, `NewInput`, `UpdateInput`를 다음으로 교체:
```go
// Beat mirrors the SQLite row. NodeID is nil when the beat is unbound or its
// bound node was deleted. Description carries the "what happens here" plot body.
type Beat struct {
	ID          string  `json:"id"`
	ThreadID    string  `json:"thread_id"`
	NodeID      *string `json:"node_id,omitempty"`
	Ordinal     int     `json:"ordinal"`
	Label       string  `json:"label"`
	Description string  `json:"description"`
	Intensity   int     `json:"intensity"`
}

// NewInput is what `beats.create` accepts. Ordinal is assigned by the repo.
type NewInput struct {
	ThreadID    string  `json:"thread_id"`
	NodeID      *string `json:"node_id,omitempty"`
	Label       string  `json:"label"`
	Description string  `json:"description"`
	Intensity   int     `json:"intensity"`
}

// UpdateInput is what `beats.update` accepts. Empty Label leaves the field
// alone; Intensity == 0 leaves it alone (use 1..3 to set). Description is a
// pointer: nil leaves it alone, a non-nil pointer (including "") sets it — so
// the body can be explicitly cleared.
type UpdateInput struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`
	Description *string `json:"description,omitempty"`
	Intensity   int     `json:"intensity"`
}
```

- [ ] **Step 3: beat repo에 description 반영**

`engine/internal/beat/repo.go`:
- `baseSelect` 상수를 교체:
```go
const baseSelect = `
SELECT id, thread_id, node_id, ordinal, label, description, intensity
FROM beats`
```
- `scan` 함수의 Scan 호출을 교체(컬럼 순서 일치):
```go
	if err := row.Scan(&b.ID, &b.ThreadID, &nodeID, &b.Ordinal, &b.Label, &b.Description, &b.Intensity); err != nil {
		return Beat{}, err
	}
```
- `Create`의 INSERT를 교체:
```go
	if _, err := tx.ExecContext(ctx, `
INSERT INTO beats (id, thread_id, node_id, ordinal, label, description, intensity)
VALUES (?, ?, ?, ?, ?, ?, ?)`, id, in.ThreadID, nullStr(in.NodeID), ordinal, in.Label, in.Description, intensity); err != nil {
		return Beat{}, err
	}
```
- `Update`의 패치 블록과 UPDATE를 교체:
```go
	if in.Label != "" {
		cur.Label = in.Label
	}
	if in.Description != nil {
		cur.Description = *in.Description
	}
	if in.Intensity != 0 {
		cur.Intensity = clampIntensity(in.Intensity)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE beats SET label = ?, description = ?, intensity = ? WHERE id = ?`,
		cur.Label, cur.Description, cur.Intensity, in.ID); err != nil {
		return err
	}
```

- [ ] **Step 4: beat repo 테스트 추가**

`engine/internal/beat/repo_test.go`에 추가(기존 테스트의 store/thread 셋업 헬퍼 재사용 — 기존 파일을 먼저 읽고 동일 패턴 사용):
```go
func TestBeatDescriptionCRUD(t *testing.T) {
	repo, threadID := newBeatRepoWithThread(t) // 기존 헬퍼 이름에 맞게 조정
	ctx := context.Background()

	b, err := repo.Create(ctx, NewInput{ThreadID: threadID, Label: "재회", Description: "항구에서 마주친다."})
	if err != nil {
		t.Fatal(err)
	}
	if b.Description != "항구에서 마주친다." {
		t.Fatalf("create description = %q", b.Description)
	}

	// nil description leaves it alone.
	if err := repo.Update(ctx, UpdateInput{ID: b.ID, Label: "재회(수정)"}); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Get(ctx, b.ID)
	if got.Description != "항구에서 마주친다." {
		t.Fatalf("nil description should preserve, got %q", got.Description)
	}
	if got.Label != "재회(수정)" {
		t.Fatalf("label patch failed: %q", got.Label)
	}

	// explicit empty clears.
	empty := ""
	if err := repo.Update(ctx, UpdateInput{ID: b.ID, Description: &empty}); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(ctx, b.ID)
	if got.Description != "" {
		t.Fatalf("empty-pointer description should clear, got %q", got.Description)
	}

	// set to new value.
	body := "편지로 신분이 드러난다."
	if err := repo.Update(ctx, UpdateInput{ID: b.ID, Description: &body}); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(ctx, b.ID)
	if got.Description != body {
		t.Fatalf("description set failed: %q", got.Description)
	}
}
```
※ 기존 테스트 파일에 thread+beat repo를 세우는 헬퍼가 없으면, 같은 패키지의 기존 `*_test.go`가 store를 여는 방식을 그대로 본떠 `newBeatRepoWithThread`를 만들어라(임시 DB → ApplyMigrations → thread INSERT → beat.NewRepo).

- [ ] **Step 5: beat 테스트 실행**

Run: `cd engine && go test ./internal/beat/...`
Expected: PASS

- [ ] **Step 6: project 도메인 타입에 outline + UpdateInput 추가**

`engine/internal/project/project.go`:
- `Project` 구조체에 `StyleNotes` 다음 줄로 필드 추가:
```go
	StyleNotes       string   `json:"style_notes"`
	Outline          string   `json:"outline"`
```
- 파일 끝에 추가:
```go
// UpdateInput patches editable project fields. Each pointer field is nil to
// leave the value alone, or non-nil (including "") to set it.
type UpdateInput struct {
	ID      string  `json:"id"`
	Outline *string `json:"outline,omitempty"`
}
```

- [ ] **Step 7: project repo에 outline 반영 + Update 메서드**

`engine/internal/project/repo.go`:
- `baseSelect` 교체(outline을 style_notes 뒤에):
```go
const baseSelect = `
SELECT id, title, genres, length_target, default_pov, style_notes, outline,
       word_count, last_opened_node_id, created_at, updated_at, archived_at
FROM projects`
```
- `scan`의 Scan 호출 교체(컬럼 순서 일치, `&p.Outline`를 `&p.StyleNotes` 뒤에):
```go
	if err := row.Scan(&p.ID, &p.Title, &genresJSON, &p.LengthTarget, &p.DefaultPOV,
		&p.StyleNotes, &p.Outline, &p.WordCount, &lastNode, &p.CreatedAt, &p.UpdatedAt, &archivedAt); err != nil {
		return Project{}, err
	}
```
- `Create`의 projects INSERT 교체(outline 컬럼 명시 + 기본 ''):
```go
	if _, err := tx.ExecContext(ctx, `
INSERT INTO projects (id, title, genres, length_target, default_pov, style_notes, outline,
                      word_count, last_opened_node_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, '', '', 0, ?, ?, ?)`,
		projectID, in.Title, string(genresJSON), in.LengthTarget, in.DefaultPOV,
		nodeID, now, now); err != nil {
		return Project{}, err
	}
```
- `Archive` 메서드 아래에 `Update` 추가:
```go
// Update patches editable fields (currently outline) and bumps updated_at.
func (r *Repo) Update(ctx context.Context, now int64, in UpdateInput) (Project, error) {
	if in.ID == "" {
		return Project{}, fmt.Errorf("update project: id required")
	}
	cur, err := r.Get(ctx, in.ID)
	if err != nil {
		return Project{}, err
	}
	if in.Outline != nil {
		cur.Outline = *in.Outline
	}
	if _, err := r.s.DB().ExecContext(ctx,
		`UPDATE projects SET outline = ?, updated_at = ? WHERE id = ?`,
		cur.Outline, now, in.ID); err != nil {
		return Project{}, err
	}
	return r.Get(ctx, in.ID)
}
```

- [ ] **Step 8: project repo 테스트 추가**

`engine/internal/project/repo_test.go`에 추가(기존 store 헬퍼 재사용):
```go
func TestProjectOutlineUpdate(t *testing.T) {
	repo := newProjectRepo(t) // 기존 헬퍼 이름에 맞게 조정
	ctx := context.Background()
	p, err := repo.Create(ctx, 1000, NewInput{Title: "테스트작", Genres: []string{"판타지"}, LengthTarget: "novel", DefaultPOV: "third_limited"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Outline != "" {
		t.Fatalf("new project outline should be empty, got %q", p.Outline)
	}
	body := "한 줄 로그라인과 3막 개요."
	updated, err := repo.Update(ctx, 2000, UpdateInput{ID: p.ID, Outline: &body})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Outline != body {
		t.Fatalf("outline = %q", updated.Outline)
	}
	if updated.UpdatedAt != 2000 {
		t.Fatalf("updated_at not bumped: %d", updated.UpdatedAt)
	}
	// nil leaves alone.
	again, err := repo.Update(ctx, 3000, UpdateInput{ID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if again.Outline != body {
		t.Fatalf("nil outline should preserve, got %q", again.Outline)
	}
}
```

- [ ] **Step 9: relationship.ListByProject 추가 + 테스트**

`engine/internal/relationship/repo.go` — `ListByEntity` 아래에 추가:
```go
// ListByProject returns every relationship row in the project, ordered by id.
// Both directions of a pair are returned as separate rows; callers dedupe by
// pair_id if needed.
func (r *Repo) ListByProject(ctx context.Context, projectID string) ([]Relationship, error) {
	rows, err := r.s.DB().QueryContext(ctx,
		baseSelect+` WHERE project_id = ? ORDER BY id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Relationship
	for rows.Next() {
		rel, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rel)
	}
	return out, rows.Err()
}
```
`engine/internal/relationship/repo_test.go`에 추가(기존 헬퍼 재사용):
```go
func TestListByProject(t *testing.T) {
	repo, projectID, a, bID := newRelRepoWithEntities(t) // 기존 헬퍼/엔티티 셋업에 맞게 조정
	ctx := context.Background()
	if _, err := repo.CreateOne(ctx, NewInput{ProjectID: projectID, FromID: a, ToID: bID, Label: "라이벌"}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.ListByProject(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Label != "라이벌" {
		t.Fatalf("ListByProject = %+v", got)
	}
}
```

- [ ] **Step 10: 전체 엔진 테스트 + 커밋**

Run: `cd engine && go test ./...`
Expected: PASS (마이그레이션이 모든 테스트 store에 적용되어 새 컬럼 포함)
```bash
git add engine/internal/store/migrations/0005_plot_spine.sql engine/internal/beat engine/internal/project engine/internal/relationship
git commit -m "feat(engine): Plan 24 T1 — beat.description, project.outline, relationship.ListByProject"
```

---

## Task 2: `plot` 패키지 — 공유 스파인 빌더 (전/현/후 씬 + beat)

**Files:**
- Create: `engine/internal/plot/plot.go`
- Create: `engine/internal/plot/builder.go`
- Test: `engine/internal/plot/builder_test.go`

스파인은 문서 순서(DFS leaf order)에서 현재 leaf의 직전/현재/직후 leaf를 잡고, 각 leaf에 묶인 beat를 thread 이름·색으로 엔리치해 반환한다. AI 컨텍스트 빌더와 `plot.spine_panel` 핸들러가 **이 한 함수를 공유**한다.

- [ ] **Step 1: 타입 정의**

`engine/internal/plot/plot.go`:
```go
// Package plot builds the "plot spine": the beats bound to the previous,
// current, and next scene (leaf) in document order, enriched with their
// thread's name and color. Shared by the AI context builder and the
// plot.spine_panel RPC handler so both agree on which scenes are neighbors.
package plot

// Beat is a thread beat enriched with its parent thread's display fields.
type Beat struct {
	ID          string `json:"id"`
	ThreadID    string `json:"thread_id"`
	ThreadName  string `json:"thread_name"`
	ThreadColor string `json:"thread_color"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Intensity   int    `json:"intensity"`
	Ordinal     int    `json:"ordinal"`
}

// SceneBeats is one scene (leaf) and the beats bound to it, in (thread, ordinal)
// order. Label is the breadcrumb path, e.g. "1부 / 1장 / 씬 3".
type SceneBeats struct {
	NodeID string `json:"node_id"`
	Label  string `json:"label"`
	Beats  []Beat `json:"beats"`
}

// Spine is the prev/current/next window. Prev/Next are nil at the document
// boundaries (first/last leaf). Current is always present.
type Spine struct {
	Prev    *SceneBeats `json:"prev"`
	Current SceneBeats  `json:"current"`
	Next    *SceneBeats `json:"next"`
}
```

- [ ] **Step 2: Builder 구현**

`engine/internal/plot/builder.go`:
```go
package plot

import (
	"context"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// Builder assembles a Spine from the node/beat/thread repos.
type Builder struct {
	nodes   *node.Repo
	beats   *beat.Repo
	threads *thread.Repo
}

// NewBuilder returns a Builder backed by the given repos.
func NewBuilder(nodes *node.Repo, beats *beat.Repo, threads *thread.Repo) *Builder {
	return &Builder{nodes: nodes, beats: beats, threads: threads}
}

// Build returns the plot spine centered on nodeID. If nodeID is a container (or
// otherwise absent from the leaf ordering), Prev/Next are nil and Current holds
// whatever beats are bound directly to nodeID.
func (b *Builder) Build(ctx context.Context, nodeID string) (Spine, error) {
	cur, err := b.nodes.Get(ctx, nodeID)
	if err != nil {
		return Spine{}, err
	}
	all, err := b.nodes.ListByProject(ctx, cur.ProjectID)
	if err != nil {
		return Spine{}, err
	}
	byID := make(map[string]node.Node, len(all))
	children := map[string][]node.Node{}
	for _, n := range all {
		byID[n.ID] = n
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}
	var leaves []node.Node
	var walk func(parent string)
	walk = func(parent string) {
		for _, c := range children[parent] {
			if c.Kind == "leaf" {
				leaves = append(leaves, c)
			}
			walk(c.ID)
		}
	}
	walk("")

	curIdx := -1
	for i, l := range leaves {
		if l.ID == cur.ID {
			curIdx = i
			break
		}
	}

	threadCache := map[string]thread.Thread{}
	sceneOf := func(n node.Node) (SceneBeats, error) {
		sb := SceneBeats{NodeID: n.ID, Label: breadcrumbLabel(byID, n), Beats: []Beat{}}
		bs, err := b.beats.ListByNode(ctx, n.ID)
		if err != nil {
			return SceneBeats{}, err
		}
		for _, bt := range bs {
			th, ok := threadCache[bt.ThreadID]
			if !ok {
				t, err := b.threads.Get(ctx, bt.ThreadID)
				if err != nil {
					continue // benign: stale thread ref; skip the beat
				}
				th = t
				threadCache[bt.ThreadID] = t
			}
			sb.Beats = append(sb.Beats, Beat{
				ID: bt.ID, ThreadID: bt.ThreadID, ThreadName: th.Name, ThreadColor: th.Color,
				Label: bt.Label, Description: bt.Description, Intensity: bt.Intensity, Ordinal: bt.Ordinal,
			})
		}
		return sb, nil
	}

	out := Spine{}
	// Current: prefer the leaf entry; fall back to cur directly (e.g. container).
	if curIdx >= 0 {
		out.Current, err = sceneOf(leaves[curIdx])
	} else {
		out.Current, err = sceneOf(cur)
	}
	if err != nil {
		return Spine{}, err
	}
	if curIdx > 0 {
		prev, err := sceneOf(leaves[curIdx-1])
		if err != nil {
			return Spine{}, err
		}
		out.Prev = &prev
	}
	if curIdx >= 0 && curIdx+1 < len(leaves) {
		next, err := sceneOf(leaves[curIdx+1])
		if err != nil {
			return Spine{}, err
		}
		out.Next = &next
	}
	return out, nil
}

// breadcrumbLabel renders the slash-joined ancestor path ending in n.Label.
func breadcrumbLabel(byID map[string]node.Node, n node.Node) string {
	parts := []string{n.Label}
	cur := n
	for cur.ParentID != nil {
		p, ok := byID[*cur.ParentID]
		if !ok {
			break
		}
		parts = append([]string{p.Label}, parts...)
		cur = p
	}
	return strings.Join(parts, " / ")
}
```

- [ ] **Step 3: Builder 테스트 작성**

`engine/internal/plot/builder_test.go` — 같은 repo 패턴(임시 store + ApplyMigrations)으로 프로젝트/노드/스레드/비트를 세우고 검증. 다른 패키지 테스트가 store를 여는 헬퍼를 본떠 작성:
```go
package plot

import (
	"context"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

func setup(t *testing.T) (*Builder, *project.Repo, *node.Repo, *thread.Repo, *beat.Repo) {
	t.Helper()
	st := store.OpenTestStore(t) // 기존 테스트가 쓰는 헬퍼명에 맞게 조정; 없으면 임시파일+store.Open+ApplyMigrations
	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	threads := thread.NewRepo(st)
	beats := beat.NewRepo(st)
	return NewBuilder(nodes, beats, threads), projects, nodes, threads, beats
}

func TestSpinePrevCurrentNext(t *testing.T) {
	b, projects, nodes, threads, beats := setup(t)
	ctx := context.Background()
	p, err := projects.Create(ctx, 1, project.NewInput{Title: "작", Genres: []string{"f"}, LengthTarget: "novel", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	// Create() already made "씬 1" as the first leaf; add two more siblings.
	first := *p.LastOpenedNodeID
	s2, err := nodes.CreateSibling(ctx, first, "leaf", "씬 2", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	s3, err := nodes.CreateSibling(ctx, s2.ID, "leaf", "씬 3", "", 3)
	if err != nil {
		t.Fatal(err)
	}
	th, err := threads.Create(ctx, thread.NewInput{ProjectID: p.ID, Name: "메인플롯"})
	if err != nil {
		t.Fatal(err)
	}
	mk := func(nodeID, label, desc string) {
		nid := nodeID
		if _, err := beats.Create(ctx, beat.NewInput{ThreadID: th.ID, NodeID: &nid, Label: label, Description: desc}); err != nil {
			t.Fatal(err)
		}
	}
	mk(first, "발단", "주인공 등장")
	mk(s2.ID, "전개", "사건 발생")
	mk(s3.ID, "위기", "추격")

	sp, err := b.Build(ctx, s2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sp.Prev == nil || sp.Prev.NodeID != first {
		t.Fatalf("prev = %+v", sp.Prev)
	}
	if sp.Current.NodeID != s2.ID || len(sp.Current.Beats) != 1 || sp.Current.Beats[0].Label != "전개" {
		t.Fatalf("current = %+v", sp.Current)
	}
	if sp.Current.Beats[0].ThreadName != "메인플롯" {
		t.Fatalf("thread enrich failed: %+v", sp.Current.Beats[0])
	}
	if sp.Next == nil || sp.Next.NodeID != s3.ID {
		t.Fatalf("next = %+v", sp.Next)
	}

	// First leaf: prev nil.
	sp0, _ := b.Build(ctx, first)
	if sp0.Prev != nil {
		t.Fatalf("first leaf prev should be nil")
	}
	// Last leaf: next nil.
	spLast, _ := b.Build(ctx, s3.ID)
	if spLast.Next != nil {
		t.Fatalf("last leaf next should be nil")
	}
}

func TestSpineEmptyBeats(t *testing.T) {
	b, projects, _, _, _ := setup(t)
	ctx := context.Background()
	p, _ := projects.Create(ctx, 1, project.NewInput{Title: "작", Genres: []string{"f"}, LengthTarget: "novel", DefaultPOV: "first"})
	sp, err := b.Build(ctx, *p.LastOpenedNodeID)
	if err != nil {
		t.Fatal(err)
	}
	if sp.Current.Beats == nil {
		t.Fatalf("Beats must be empty slice, not nil")
	}
	if len(sp.Current.Beats) != 0 {
		t.Fatalf("expected 0 beats, got %d", len(sp.Current.Beats))
	}
}
```
※ `node.Repo.CreateSibling`의 시그니처를 실제 코드에서 확인(요약 기준 `CreateSibling(ctx, referenceID, kind, label, title string, now int64)`). store 테스트 헬퍼 이름(`OpenTestStore` 등)은 같은 모듈의 기존 테스트에서 실제 이름을 확인해 맞춰라.

- [ ] **Step 4: plot 테스트 실행 + 커밋**

Run: `cd engine && go test ./internal/plot/...`
Expected: PASS
```bash
git add engine/internal/plot
git commit -m "feat(engine): Plan 24 T2 — plot package (shared spine builder)"
```

---

## Task 3: 핸들러 — `plot.spine_panel` + `projects.update` + 등록

**Files:**
- Create: `engine/internal/rpc/handlers/plot.go`
- Modify: `engine/internal/rpc/handlers/projects.go`
- Modify: `engine/cmd/linetta-engine/main.go`
- Test: `engine/internal/rpc/handlers/plot_test.go` (있으면 패턴 재사용)

- [ ] **Step 1: plot.spine_panel 핸들러**

`engine/internal/rpc/handlers/plot.go`:
```go
package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type plotSpinePanelParams struct {
	NodeID string `json:"node_id"`
}

// PlotSpinePanel returns a handler for plot.spine_panel. It returns the
// prev/current/next scene beats for the inline plot panel.
func PlotSpinePanel(builder *plot.Builder) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p plotSpinePanelParams
		if err := json.Unmarshal(params, &p); err != nil || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id required"}
		}
		spine, err := builder.Build(ctx, p.NodeID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(spine)
	}
}
```

- [ ] **Step 2: projects.update 핸들러**

`engine/internal/rpc/handlers/projects.go` — `ArchiveProject` 아래에 추가(파일 상단 import에 이미 `project`가 있음):
```go
// UpdateProject returns a handler for projects.update. Currently patches outline.
func UpdateProject(repo *project.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in project.UpdateInput
		if err := json.Unmarshal(params, &in); err != nil || in.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		p, err := repo.Update(ctx, now(), in)
		if errors.Is(err, project.ErrNotFound) {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project not found"}
		}
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(p)
	}
}
```
(`errors`는 projects.go에 이미 import 되어 있음.)

- [ ] **Step 3: main.go에서 plot.Builder 생성 + 핸들러 등록**

`engine/cmd/linetta-engine/main.go`:
- import 블록에 추가: `"github.com/devlikebear/linetta/engine/internal/plot"`
- repo 생성부(line ~72, `relationships := relationship.NewRepo(st)` 아래)에 추가:
```go
	plotBuilder := plot.NewBuilder(nodes, beats, threads)
```
- `s.Handle("projects.archive", ...)` 다음 줄에 추가:
```go
	s.Handle("projects.update", handlers.UpdateProject(projects, clock))
```
- `s.Handle("ai.cancel", ...)` 다음 줄(또는 beats 핸들러 묶음 근처)에 추가:
```go
	s.Handle("plot.spine_panel", handlers.PlotSpinePanel(plotBuilder))
```

- [ ] **Step 4: 핸들러 테스트(있으면) + 전체 빌드**

`engine/internal/rpc/handlers/`에 핸들러 테스트 패턴이 있으면 `plot.spine_panel` 빈 node_id → InvalidParams, 정상 노드 → prev/current/next 형태를 검증하는 테스트를 추가(기존 핸들러 테스트의 서버 셋업 헬퍼 재사용). 패턴이 없으면 plot 패키지 테스트로 갈음하고 핸들러는 빌드만 확인.

Run: `cd engine && go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 5: 커밋**
```bash
git add engine/internal/rpc/handlers/plot.go engine/internal/rpc/handlers/projects.go engine/cmd/linetta-engine/main.go engine/internal/rpc/handlers/plot_test.go
git commit -m "feat(engine): Plan 24 T3 — plot.spine_panel + projects.update handlers"
```

---

## Task 4: AI 컨텍스트 + 프롬프트 재편 (개요 + 플롯 + 관계 + 슬림화)

이 Task는 한 번에 컴파일·통과해야 한다(구조체 필드 제거가 prompts.go·context.go·main.go에 동시 영향). 순서대로 모두 적용한 뒤 빌드/테스트.

**Files:**
- Modify: `engine/internal/ai/ai.go`
- Modify: `engine/internal/ai/context.go`
- Modify: `engine/internal/ai/prompts.go`
- Modify: `engine/cmd/linetta-engine/main.go`
- Test: `engine/internal/ai/context_test.go`, `engine/internal/ai/prompts_test.go` (기존 파일에 추가/수정)

- [ ] **Step 1: ai.go — Context/PreviewCounts/타입 재편**

`engine/internal/ai/ai.go`:
- import에 추가(파일 상단; 현재 ai.go는 import 없음 → 새 import 블록 필요):
```go
import "github.com/devlikebear/linetta/engine/internal/plot"
```
- `Context` 구조체 교체(ActiveThreads 제거, Outline/Plot/Relationships 추가, HierarchicalContext는 유지하되 슬림 필드만 사용):
```go
type Context struct {
	ProjectID     string              `json:"project_id"`
	NodeID        string              `json:"node_id"`
	SceneLabel    string              `json:"scene_label"`
	SceneText     string              `json:"scene_text"`
	PrevSummary   string              `json:"prev_summary"`
	Project       ProjectMeta         `json:"project"`
	Outline       string              `json:"outline"`
	Hierarchical  HierarchicalContext `json:"hierarchical"`
	RelatedScenes []SceneSummary      `json:"related_scenes"`
	Entities      []EntityBrief       `json:"entities"`
	Relationships []RelationBrief     `json:"relationships"`
	Plot          plot.Spine          `json:"plot"`
	Notes         []NoteBrief         `json:"notes"`
	StyleNotes    string              `json:"style_notes"`
	SelectionText string              `json:"selection_text"`
	UserPrompt    string              `json:"user_prompt"`
	Options       Options             `json:"options"`
}

// RelationBrief is one relationship between two entities present in the current
// scene. Bidirectional pairs render with "↔", singletons with "→".
type RelationBrief struct {
	From          string `json:"from"`
	To            string `json:"to"`
	Label         string `json:"label"`
	Notes         string `json:"notes"`
	Bidirectional bool   `json:"bidirectional"`
}
```
- `HierarchicalContext`에서 SameChapterSummaries/OtherChapterSummaries/OtherPartSummaries 제거하고 Nearby + ProjectSynopsis만 남김:
```go
type HierarchicalContext struct {
	NearbyLeafSummaries []SceneSummary `json:"nearby_leaf_summaries"`
	ProjectSynopsis     string         `json:"project_synopsis"`
}
```
- `ChapterSummary`, `PartSummary` 타입 삭제(더 이상 사용 안 함).
- `ActiveThread` 타입 삭제. `BeatBrief` 타입은 다른 곳(ai_runs 등)에서 안 쓰면 삭제, 쓰면 유지 — `grep -rn BeatBrief engine/` 로 확인 후 미사용이면 삭제.
- `PreviewCounts` 교체(제거 필드 삭제, plot/관계/개요 추가):
```go
type PreviewCounts struct {
	NearbyScenes      int  `json:"nearby_scenes"`
	HasOutline        bool `json:"has_outline"`
	HasSynopsis       bool `json:"has_synopsis"`
	RelatedScenes     int  `json:"related_scenes"`
	Entities          int  `json:"entities"`
	Relationships     int  `json:"relationships"`
	PlotBeats         int  `json:"plot_beats"`
	Notes             int  `json:"notes"`
	ProjectMetaFields int  `json:"project_meta_fields"`
	HasStyleNotes     bool `json:"has_style_notes"`
}
```

- [ ] **Step 2: context.go — 빌더 필드 + 슬림화 + 개요/플롯/관계 로드**

`engine/internal/ai/context.go`:
- import 추가: `"github.com/devlikebear/linetta/engine/internal/entity"`, `"github.com/devlikebear/linetta/engine/internal/plot"`, `"github.com/devlikebear/linetta/engine/internal/relationship"`.
- `ContextBuilder` 교체. **주의:** 슬림화 후 context.go는 `beats`/`threads`를 직접 쓰지 않는다(plot.Builder가 흡수). 그래서 두 필드는 구조체에 보관하지 않고 생성자에서 plot.Builder로만 넘긴다. `loadActiveThreads` 삭제 후 `b.beats`/`b.threads` 참조가 남아 있지 않은지 `grep -n "b\.beats\|b\.threads" engine/internal/ai/context.go`로 확인할 것(없어야 함):
```go
type ContextBuilder struct {
	projects      *project.Repo
	nodes         *node.Repo
	mentions      *mention.Repo
	notes         *note.Repo
	relationships *relationship.Repo
	plot          *plot.Builder
	refresher     SummaryRefresher
}
```
- `NewContextBuilder` 시그니처 교체(인자 순서는 기존과 동일하게 유지 + relationships 추가; beats/threads는 받되 plot.Builder 생성에만 사용):
```go
func NewContextBuilder(projects *project.Repo, nodes *node.Repo, mentions *mention.Repo, threads *thread.Repo, beats *beat.Repo, notes *note.Repo, relationships *relationship.Repo) *ContextBuilder {
	return &ContextBuilder{
		projects:      projects,
		nodes:         nodes,
		mentions:      mentions,
		notes:         notes,
		relationships: relationships,
		plot:          plot.NewBuilder(nodes, beats, threads),
		refresher:     noopRefresher{},
	}
}
```
- 이 변경으로 `thread`/`beat` 패키지 import가 context.go에서 더는 직접 쓰이지 않으면(타입 참조가 사라지면) **해당 import를 제거**해야 `go build`가 통과한다(미사용 import는 컴파일 에러). `NewContextBuilder`의 매개변수 타입 `*thread.Repo`, `*beat.Repo`가 여전히 import를 사용하므로 import는 유지된다 — 빌드 에러가 나면 메시지에 따라 정리.
- `Build` 메서드: `active, err := b.loadActiveThreads(...)` 호출을 plot/관계 로드로 교체. `briefs` 생성 직후, `hierarchical`/`related` 로드 부근을 다음으로 조정:
```go
	spine, err := b.plot.Build(ctx, nodeID)
	if err != nil {
		return Context{}, err
	}
	relations, err := b.loadRelationships(ctx, proj.ID, ents)
	if err != nil {
		return Context{}, err
	}
```
  (`ents`는 이미 `b.mentions.ListEntitiesForNode`로 받은 `[]entity.Entity`.)
- `return Context{...}` 리터럴 교체: `ActiveThreads: active,` 제거하고 `Outline: proj.Outline,`, `Relationships: relations,`, `Plot: spine,` 추가:
```go
	return Context{
		ProjectID:   proj.ID,
		NodeID:      n.ID,
		SceneLabel:  n.Label,
		SceneText:   sceneText,
		PrevSummary: prevSummary,
		Project: ProjectMeta{
			Genres:       proj.Genres,
			LengthTarget: proj.LengthTarget,
			DefaultPOV:   proj.DefaultPOV,
		},
		Outline:       proj.Outline,
		Hierarchical:  hierarchical,
		RelatedScenes: related,
		Entities:      briefs,
		Relationships: relations,
		Plot:          spine,
		Notes:         noteBriefs,
		StyleNotes:    proj.StyleNotes,
		SelectionText: selectionText,
		UserPrompt:    prompt,
		Options:       opts,
	}, nil
```
- `loadActiveThreads`, `capActiveThreads` 함수 **삭제**.
- `loadRelationships` 추가:
```go
// loadRelationships returns relationships whose both endpoints appear in the
// current scene's mentioned entities. Pairs are deduped by pair_id (first wins).
func (b *ContextBuilder) loadRelationships(ctx context.Context, projectID string, ents []entity.Entity) ([]RelationBrief, error) {
	if b.relationships == nil || len(ents) == 0 {
		return nil, nil
	}
	rels, err := b.relationships.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	nameByID := make(map[string]string, len(ents))
	present := make(map[string]bool, len(ents))
	for _, e := range ents {
		nameByID[e.ID] = e.Name
		present[e.ID] = true
	}
	seenPair := map[string]bool{}
	out := make([]RelationBrief, 0)
	for _, r := range rels {
		if !present[r.FromID] || !present[r.ToID] {
			continue
		}
		bidir := false
		if r.PairID != nil && *r.PairID != "" {
			if seenPair[*r.PairID] {
				continue
			}
			seenPair[*r.PairID] = true
			bidir = true
		}
		out = append(out, RelationBrief{
			From: nameByID[r.FromID], To: nameByID[r.ToID],
			Label: r.Label, Notes: r.Notes, Bidirectional: bidir,
		})
	}
	return out, nil
}
```
- `loadHierarchicalContext` 슬림화: SameChapter/OtherChapter/OtherPart 계산 블록을 **모두 삭제**하고 Nearby(1전+1후) + ProjectSynopsis만 남긴다. Nearby 루프의 인덱스 집합을 `[]int{curIdx - 2, curIdx - 1, curIdx + 1}` → `[]int{curIdx - 1, curIdx + 1}` 로 변경. 함수 끝의 `trimToBudget(&out, hierarchicalMaxChars)` 호출은 유지하되, `trimToBudget`을 Nearby + synopsis만 다루도록 단순화(아래).
- `trimToBudget` 교체(other/same 분기 제거):
```go
func trimToBudget(h *HierarchicalContext, maxChars int) {
	estimate := func() int {
		total := len(h.ProjectSynopsis)
		for _, s := range h.NearbyLeafSummaries {
			total += len(s.Label) + len(s.Body) + 4
		}
		return total
	}
	for estimate() > maxChars && len(h.NearbyLeafSummaries) > 0 {
		h.NearbyLeafSummaries = h.NearbyLeafSummaries[:len(h.NearbyLeafSummaries)-1]
	}
}
```
- `loadRelatedScenes`: RAG 개수 3 → 2. 함수 내 `make([]SceneSummary, 0, 3)` → `0, 2`, `if len(out) >= 3` → `>= 2`, 그리고 `b.mentions.CoMentionLeaves(ctx, cur.ID, 3+len(excludeIDs))` → `2+len(excludeIDs)`.
- `parentKeyOf` 함수가 다른 곳에서 안 쓰이면(`grep -n parentKeyOf engine/internal/ai`) 삭제. SameChapter/Other 블록 삭제로 미사용이 되면 컴파일 에러를 막기 위해 제거.
- `CountsFromContext` 교체:
```go
func CountsFromContext(c Context) PreviewCounts {
	projectMeta := 0
	if len(c.Project.Genres) > 0 {
		projectMeta++
	}
	if c.Project.LengthTarget != "" {
		projectMeta++
	}
	if c.Project.DefaultPOV != "" {
		projectMeta++
	}
	plotBeats := len(c.Plot.Current.Beats)
	if c.Plot.Prev != nil {
		plotBeats += len(c.Plot.Prev.Beats)
	}
	if c.Plot.Next != nil {
		plotBeats += len(c.Plot.Next.Beats)
	}
	return PreviewCounts{
		NearbyScenes:      len(c.Hierarchical.NearbyLeafSummaries),
		HasOutline:        strings.TrimSpace(c.Outline) != "",
		HasSynopsis:       strings.TrimSpace(c.Hierarchical.ProjectSynopsis) != "",
		RelatedScenes:     len(c.RelatedScenes),
		Entities:          len(c.Entities),
		Relationships:     len(c.Relationships),
		PlotBeats:         plotBeats,
		Notes:             len(c.Notes),
		ProjectMetaFields: projectMeta,
		HasStyleNotes:     strings.TrimSpace(c.StyleNotes) != "",
	}
}
```

- [ ] **Step 3: prompts.go — 개요/플롯/관계 렌더 + 제거 섹션 삭제**

`engine/internal/ai/prompts.go` `buildUser` 수정:
- `## 작품 전반` 블록을 개요 우선으로 교체:
```go
	// 개요(작가 편집) 우선, 없으면 자동 파생 synopsis로 폴백.
	overview := strings.TrimSpace(c.Outline)
	if overview == "" {
		overview = strings.TrimSpace(c.Hierarchical.ProjectSynopsis)
	}
	if overview != "" {
		b.WriteString("## 작품 개요\n")
		b.WriteString(overview)
		b.WriteString("\n\n")
	}
```
- `## 인근 줄거리` 블록(OtherPart/OtherChapter)과 `## 같은 장 다른 씬` 블록을 **삭제**.
- `## 직전·직후 씬 발췌`(NearbyLeafSummaries) 블록은 유지.
- `## 관련 과거 씬`(RelatedScenes) 블록은 유지.
- `## 활성 스토리라인`(ActiveThreads) 블록을 **삭제**하고, 그 자리에 `## 플롯` 렌더 추가(현재 씬 본문/선택 영역 다음, 등장 인물 다음에 두면 됨 — 아래 순서로 통일). 등장 인물·장소 블록 뒤, 작가 주석 블록 앞에 삽입:
```go
	if hasPlot(c.Plot) {
		b.WriteString("## 플롯\n")
		writeScene := func(tag string, s *plot.SceneBeats) {
			if s == nil || len(s.Beats) == 0 {
				return
			}
			b.WriteString(tag)
			b.WriteString("\n")
			for _, bt := range s.Beats {
				line := fmt.Sprintf("  · [%s] #%d %s", bt.ThreadName, bt.Ordinal, bt.Label)
				if strings.TrimSpace(bt.Description) != "" {
					line += " — " + bt.Description
				}
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
		writeScene("[이전 씬]", c.Plot.Prev)
		writeScene("[현재 씬]", &c.Plot.Current)
		writeScene("[다음 씬]", c.Plot.Next)
		b.WriteString("\n")
	}
	if len(c.Relationships) > 0 {
		b.WriteString("## 관계\n")
		for _, r := range c.Relationships {
			arrow := "→"
			if r.Bidirectional {
				arrow = "↔"
			}
			line := fmt.Sprintf("- %s %s %s: %s", r.From, arrow, r.To, r.Label)
			if strings.TrimSpace(r.Notes) != "" {
				line += " — " + r.Notes
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
```
- prompts.go import에 `"github.com/devlikebear/linetta/engine/internal/plot"` 추가.
- `## 플롯` char budget 적용 헬퍼 + `hasPlot` 추가(파일 하단):
```go
const plotMaxChars = 2000

func hasPlot(s plot.Spine) bool {
	if len(s.Current.Beats) > 0 {
		return true
	}
	if s.Prev != nil && len(s.Prev.Beats) > 0 {
		return true
	}
	if s.Next != nil && len(s.Next.Beats) > 0 {
		return true
	}
	return false
}
```
  budget 적용: `buildUser` 진입 직후(또는 `## 플롯` 렌더 직전)에 description을 잘라 상한을 맞춘다. 단순 구현 — 누적 길이가 `plotMaxChars`를 넘으면 이후 beat의 Description을 비운다:
```go
	capPlotDescriptions(&c.Plot, plotMaxChars)
```
  그리고 헬퍼:
```go
// capPlotDescriptions zeroes out beat descriptions (keeping labels + thread
// names) once the running size of the plot section exceeds maxChars.
func capPlotDescriptions(s *plot.Spine, maxChars int) {
	total := 0
	trim := func(sb *plot.SceneBeats) {
		if sb == nil {
			return
		}
		for i := range sb.Beats {
			head := len(sb.Beats[i].ThreadName) + len(sb.Beats[i].Label) + 12
			if total+head > maxChars {
				sb.Beats[i].Description = ""
				total += head
				continue
			}
			total += head + len(sb.Beats[i].Description)
			if total > maxChars {
				sb.Beats[i].Description = ""
			}
		}
	}
	trim(s.Prev)
	trim(&s.Current)
	trim(s.Next)
}
```
  `buildUser`는 값 복사본 `c Context`를 받으므로 `c.Plot`을 수정해도 호출자에 영향 없음(`BuildMessages`가 `buildUser(c)` 호출). `capPlotDescriptions(&c.Plot, plotMaxChars)`를 `buildUser` 함수 본문 맨 위에 둔다.

- [ ] **Step 4: main.go — ContextBuilder 인자 추가**

`engine/cmd/linetta-engine/main.go` line 94 부근 ContextBuilder 생성을 교체:
```go
	contextBuilder := ai.NewContextBuilder(projects, nodes, mentions, threads, beats, notes, relationships).
		WithSummaryRefresher(summ)
```

- [ ] **Step 5: ai 테스트 갱신**

기존 `engine/internal/ai/context_test.go` / `prompts_test.go`가 제거된 필드(ActiveThreads, SameChapter 등)나 옛 `NewContextBuilder` 시그니처를 참조하면 컴파일이 깨진다. 다음을 수행:
- 옛 시그니처 호출을 새 7-인자 시그니처로 갱신(테스트가 relationship repo를 만들어 전달; nil 불가 시 `relationship.NewRepo(st)`).
- 제거된 섹션을 검증하던 테스트 케이스 삭제/수정.
- 신규 검증 추가(`prompts_test.go`):
```go
func TestBuildUserRendersPlotAndRelations(t *testing.T) {
	prev := plot.SceneBeats{NodeID: "p", Label: "1장 / 씬1", Beats: []plot.Beat{{ThreadName: "메인", Ordinal: 1, Label: "재회", Description: "항구에서"}}}
	c := Context{
		SceneLabel: "씬2", SceneText: "본문",
		Outline: "전체 개요",
		Plot: plot.Spine{
			Prev:    &prev,
			Current: plot.SceneBeats{NodeID: "c", Beats: []plot.Beat{{ThreadName: "메인", Ordinal: 2, Label: "발각", Description: "편지"}}},
		},
		Relationships: []RelationBrief{{From: "A", To: "B", Label: "라이벌", Bidirectional: true}},
		UserPrompt:    "확장해줘",
	}
	out := buildUser(c)
	for _, want := range []string{"## 작품 개요", "전체 개요", "## 플롯", "[이전 씬]", "[현재 씬]", "메인", "재회", "## 관계", "A ↔ B: 라이벌"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	for _, gone := range []string{"## 인근 줄거리", "## 같은 장 다른 씬", "## 활성 스토리라인"} {
		if strings.Contains(out, gone) {
			t.Fatalf("removed section %q still present", gone)
		}
	}
}

func TestOverviewFallsBackToSynopsis(t *testing.T) {
	c := Context{SceneLabel: "씬", UserPrompt: "x", Hierarchical: HierarchicalContext{ProjectSynopsis: "파생 시놉시스"}}
	out := buildUser(c)
	if !strings.Contains(out, "## 작품 개요") || !strings.Contains(out, "파생 시놉시스") {
		t.Fatalf("synopsis fallback failed:\n%s", out)
	}
}
```
  (테스트 파일 import에 `plot` 패키지 추가.)

- [ ] **Step 6: 전체 빌드 + 테스트 + 커밋**

Run: `cd engine && go build ./... && go test ./...`
Expected: PASS
```bash
git add engine/internal/ai engine/cmd/linetta-engine/main.go
git commit -m "feat(engine): Plan 24 T4 — AI context reworked to plot spine + outline + relations"
```

---

## Task 5: 프런트엔드 타입 + RPC 클라이언트

**Files:**
- Modify: `apps/desktop/src/lib/types.ts`
- Modify: `apps/desktop/src/lib/rpc.ts`

- [ ] **Step 1: types.ts — Project/Beat/입력/스파인/카운트**

- `Project`에 `style_notes` 다음 줄 추가: `outline: string;`
- `Beat`에 `label` 다음 줄 추가: `description: string;`
- `NewBeatInput`에 추가: `description?: string;`
- `UpdateBeatInput`에 추가: `description?: string;`
- 새 입력 타입 추가(파일 내 Project 관련 근처):
```ts
export interface UpdateProjectInput {
  id: string;
  outline?: string;
}
```
- `BeatBrief` / `ActiveThread` 인터페이스 삭제(엔진에서 제거됨; FE에서 미사용 확인 후 삭제 — `grep -rn "ActiveThread\|BeatBrief" apps/desktop/src`).
- 플롯 스파인 패널 타입 추가:
```ts
// Mirrors engine/internal/plot Spine / SceneBeats / Beat (plot.spine_panel RPC).
export interface PlotBeat {
  id: string;
  thread_id: string;
  thread_name: string;
  thread_color: string;
  label: string;
  description: string;
  intensity: number;
  ordinal: number;
}

export interface PlotScene {
  node_id: string;
  label: string;
  beats: PlotBeat[];
}

export interface PlotSpine {
  prev: PlotScene | null;
  current: PlotScene;
  next: PlotScene | null;
}
```
- `ContextPreviewResponse` 교체:
```ts
export interface ContextPreviewResponse {
  nearby_scenes: number;
  has_outline: boolean;
  has_synopsis: boolean;
  related_scenes: number;
  entities: number;
  relationships: number;
  plot_beats: number;
  notes: number;
  project_meta_fields: number;
  has_style_notes: boolean;
}
```
- `ContextCounts` 교체:
```ts
export interface ContextCounts {
  nearbyScenes: number;
  hasOutline: boolean;
  hasSynopsis: boolean;
  relatedScenes: number;
  entities: number;
  relationships: number;
  plotBeats: number;
  notes: number;
  projectMetaFields: number;
  hasStyleNotes: boolean;
}
```

- [ ] **Step 2: rpc.ts — projects.update, plot.spinePanel, previewContext 매핑**

- import 타입에 `UpdateProjectInput`, `PlotSpine` 추가(파일 상단 type import 목록).
- `projects` 네임스페이스에 추가:
```ts
  update: (input: UpdateProjectInput) => rpcCall<Project>("projects.update", input),
```
- `ai.previewContext`의 매핑 객체 교체:
```ts
      .then((r) => ({
        nearbyScenes: r.nearby_scenes,
        hasOutline: r.has_outline,
        hasSynopsis: r.has_synopsis,
        relatedScenes: r.related_scenes,
        entities: r.entities,
        relationships: r.relationships,
        plotBeats: r.plot_beats,
        notes: r.notes,
        projectMetaFields: r.project_meta_fields,
        hasStyleNotes: r.has_style_notes,
      })),
```
- 새 `plot` 네임스페이스 추가(파일 하단, beats 근처):
```ts
export const plot = {
  spinePanel: (nodeId: string) =>
    rpcCall<PlotSpine>("plot.spine_panel", { node_id: nodeId }),
};
```

- [ ] **Step 3: tsc 검증 + 커밋**

Run: `cd apps/desktop && npx tsc --noEmit`
Expected: 이 단계에서 AIContextChecklist.tsx / PlotPanel 등 아직 안 고친 소비처에서 에러가 날 수 있음. **T5 자체의 types.ts/rpc.ts 파일에는 에러가 없어야 함.** 남은 에러가 T6~T9에서 다룰 파일(AIContextChecklist, ContextPanel, Workspace)이면 정상.
```bash
git add apps/desktop/src/lib/types.ts apps/desktop/src/lib/rpc.ts
git commit -m "feat(desktop): Plan 24 T5 — FE types + rpc for plot spine/outline"
```

---

## Task 6: AIContextChecklist 항목 재편

**Files:**
- Modify: `apps/desktop/src/components/ai/AIContextChecklist.tsx`

- [ ] **Step 1: 항목 리스트 교체**

`AIContextChecklistList`의 `items` 배열을 새 ContextCounts에 맞게 교체:
```tsx
  const items: { label: string; present: boolean; caption?: string }[] = [
    { label: "현재 씬 본문", present: true },
    { label: "작품 개요", present: counts.hasOutline },
    { label: "작품 시놉시스(폴백)", present: counts.hasSynopsis },
    {
      label: "직전·직후 씬 발췌",
      present: counts.nearbyScenes > 0,
      caption: `${counts.nearbyScenes}개`,
    },
    {
      label: "관련 과거 씬 (멘션 RAG)",
      present: counts.relatedScenes > 0,
      caption: `${counts.relatedScenes}개`,
    },
    {
      label: "플롯 (전/현/후 씬 비트)",
      present: counts.plotBeats > 0,
      caption: `${counts.plotBeats}개`,
    },
    {
      label: "등장 인물·장소",
      present: counts.entities > 0,
      caption: `${counts.entities}개`,
    },
    {
      label: "관계",
      present: counts.relationships > 0,
      caption: `${counts.relationships}개`,
    },
    {
      label: "작가 주석",
      present: counts.notes > 0,
      caption: `${counts.notes}개`,
    },
    {
      label: "작품 설정 (장르/분량/시점)",
      present: counts.projectMetaFields > 0,
      caption: `${counts.projectMetaFields}/3`,
    },
    { label: "작가 style notes", present: counts.hasStyleNotes },
  ];
```

- [ ] **Step 2: totalContextItems 교체**
```tsx
export function totalContextItems(counts: ContextCounts): number {
  return (
    counts.nearbyScenes +
    (counts.hasOutline ? 1 : 0) +
    (counts.hasSynopsis ? 1 : 0) +
    counts.relatedScenes +
    counts.plotBeats +
    counts.entities +
    counts.relationships +
    counts.notes +
    counts.projectMetaFields +
    (counts.hasStyleNotes ? 1 : 0)
  );
}
```

- [ ] **Step 3: tsc + 커밋**

Run: `cd apps/desktop && npx tsc --noEmit`
Expected: AIContextChecklist.tsx 관련 에러 없음(남은 에러는 T7~T9 파일).
```bash
git add apps/desktop/src/components/ai/AIContextChecklist.tsx
git commit -m "feat(desktop): Plan 24 T6 — context checklist items for plot spine"
```

---

## Task 7: PlotPanel 컴포넌트 (인라인 플롯 패널)

**Files:**
- Create: `apps/desktop/src/components/PlotPanel.tsx`
- Create: `apps/desktop/src/components/PlotPanel.css`

집필 화면 우측에서 개요(접이식 편집) + 전/현/후 씬 beat(현재 씬은 추가·편집)를 보여준다.

- [ ] **Step 1: PlotPanel.tsx 작성**

`apps/desktop/src/components/PlotPanel.tsx`:
```tsx
import { useCallback, useEffect, useRef, useState } from "react";
import type { Project, PlotSpine, PlotScene, Thread } from "../lib/types";
import { plot as plotApi, beats as beatsApi, threads as threadsApi, projects as projectsApi } from "../lib/rpc";
import { Plus, X } from "../lib/icons";
import "./PlotPanel.css";

interface Props {
  project: Project;
  nodeId: string;
  onOpenThread: (threadId: string) => void;
  onProjectChanged?: (project: Project) => void;
}

export function PlotPanel({ project, nodeId, onOpenThread, onProjectChanged }: Props) {
  const [spine, setSpine] = useState<PlotSpine | null>(null);
  const [openThreads, setOpenThreads] = useState<Thread[]>([]);
  const [outlineOpen, setOutlineOpen] = useState(false);
  const [outline, setOutline] = useState(project.outline ?? "");
  const [editingBeat, setEditingBeat] = useState<string | null>(null);
  const [adding, setAdding] = useState<"current" | "next" | null>(null);
  const [draftThread, setDraftThread] = useState("");
  const [draftLabel, setDraftLabel] = useState("");
  const saveTimer = useRef<number | null>(null);

  const reload = useCallback(async () => {
    try {
      const [sp, ths] = await Promise.all([
        plotApi.spinePanel(nodeId),
        threadsApi.list(project.id, false),
      ]);
      setSpine(sp);
      setOpenThreads(ths);
    } catch {
      setSpine(null);
    }
  }, [nodeId, project.id]);

  useEffect(() => { reload(); }, [reload]);
  useEffect(() => { setOutline(project.outline ?? ""); }, [project.id, project.outline]);

  const saveOutline = (next: string) => {
    setOutline(next);
    if (saveTimer.current) window.clearTimeout(saveTimer.current);
    saveTimer.current = window.setTimeout(async () => {
      try {
        const updated = await projectsApi.update({ id: project.id, outline: next });
        onProjectChanged?.(updated);
      } catch { /* benign; keep local draft */ }
    }, 600);
  };

  const addBeat = async (target: "current" | "next") => {
    const sceneId = target === "current" ? spine?.current.node_id : spine?.next?.node_id;
    const threadId = draftThread || openThreads[0]?.id;
    if (!sceneId || !threadId || !draftLabel.trim()) { setAdding(null); return; }
    try {
      await beatsApi.create({ thread_id: threadId, node_id: sceneId, label: draftLabel.trim() });
      setAdding(null); setDraftLabel(""); setDraftThread("");
      reload();
    } catch { /* benign */ }
  };

  const patchBeat = async (id: string, patch: { label?: string; description?: string; intensity?: number }) => {
    try {
      await beatsApi.update({ id, ...patch });
      reload();
    } catch { /* benign */ }
  };

  const deleteBeat = async (id: string) => {
    try { await beatsApi.delete(id); reload(); } catch { /* benign */ }
  };

  const renderScene = (scene: PlotScene | null | undefined, mode: "prev" | "current" | "next") => {
    if (!scene) return null;
    const editable = mode === "current";
    return (
      <div className={`plot-scene plot-scene-${mode}`}>
        <div className="plot-scene-label">{mode === "current" ? "현재 씬" : `${mode === "prev" ? "이전" : "다음"} 씬 · ${scene.label}`}</div>
        {scene.beats.length === 0 && !editable && <p className="plot-empty">비트 없음</p>}
        {scene.beats.map((bt) => (
          <div className="plot-beat" key={bt.id}>
            <button type="button" className="plot-beat-head" onClick={() => onOpenThread(bt.thread_id)}>
              <span className="plot-dot" style={{ backgroundColor: bt.thread_color }} aria-hidden />
              <span className="plot-thread">{bt.thread_name}</span>
              <span className="plot-label">{bt.label || "(제목 없음)"}</span>
            </button>
            {editable && (
              <button type="button" className="plot-edit" aria-label="비트 편집" onClick={() => setEditingBeat(editingBeat === bt.id ? null : bt.id)}>✎</button>
            )}
            {bt.description && editingBeat !== bt.id && <p className="plot-desc">{bt.description}</p>}
            {editable && editingBeat === bt.id && (
              <div className="plot-beat-edit">
                <input className="attr-value" defaultValue={bt.label} placeholder="제목"
                  onBlur={(e) => e.target.value !== bt.label && patchBeat(bt.id, { label: e.target.value })} />
                <textarea defaultValue={bt.description} placeholder="무슨 일이 일어나는지" rows={3}
                  onBlur={(e) => e.target.value !== bt.description && patchBeat(bt.id, { description: e.target.value })} />
                <div className="plot-beat-edit-actions">
                  <div className="plot-intensity">
                    {[1, 2, 3].map((lvl) => (
                      <button key={lvl} type="button" className={bt.intensity === lvl ? "sel" : ""} onClick={() => patchBeat(bt.id, { intensity: lvl })}>{lvl}</button>
                    ))}
                  </div>
                  <button type="button" className="attr-del" aria-label="삭제" onClick={() => deleteBeat(bt.id)}><X size={14} /></button>
                </div>
              </div>
            )}
          </div>
        ))}
        {(mode === "current" || mode === "next") && (
          adding === mode ? (
            <div className="plot-add">
              <select className="attr-value" value={draftThread} onChange={(e) => setDraftThread(e.target.value)}>
                {openThreads.length === 0 && <option value="">스토리라인 없음</option>}
                {openThreads.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
              </select>
              <input autoFocus className="attr-value" value={draftLabel} placeholder="비트 제목 (Enter)"
                onChange={(e) => setDraftLabel(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") { e.preventDefault(); addBeat(mode); }
                  else if (e.key === "Escape") { e.preventDefault(); setAdding(null); }
                }} />
            </div>
          ) : (
            <button type="button" className="plot-add-btn" disabled={openThreads.length === 0}
              onClick={() => { setAdding(mode); setDraftThread(openThreads[0]?.id ?? ""); setDraftLabel(""); }}>
              <Plus size={12} /> {mode === "current" ? "비트 추가" : "다음 씬에 비트 추가"}
            </button>
          )
        )}
      </div>
    );
  };

  return (
    <section className="ctx-section plot-panel">
      <h4>플롯</h4>
      <div className="plot-outline">
        <button type="button" className="plot-outline-toggle" onClick={() => setOutlineOpen((v) => !v)}>
          {outlineOpen ? "▾" : "▸"} 개요
        </button>
        {outlineOpen && (
          <textarea className="plot-outline-text" value={outline} rows={5} placeholder="작품 전체 개요 (로그라인 + 줄거리)"
            onChange={(e) => saveOutline(e.target.value)} />
        )}
      </div>
      {openThreads.length === 0 && (
        <p className="ctx-empty">스토리라인이 없어요. 명령 팔레트에서 “이 씬을 새 Thread로 표시”로 시작하세요.</p>
      )}
      {renderScene(spine?.prev, "prev")}
      {renderScene(spine?.current, "current")}
      {renderScene(spine?.next, "next")}
    </section>
  );
}
```
※ `../lib/icons`에 `Plus`, `X`가 export됨(기존 사용 확인됨). 없으면 동일 모듈에서 쓰는 아이콘으로 대체.

- [ ] **Step 2: PlotPanel.css 작성**

`apps/desktop/src/components/PlotPanel.css`(기존 ctx-* 톤에 맞춘 미니멀 스타일):
```css
.plot-panel .plot-outline { margin-bottom: 0.6rem; }
.plot-outline-toggle {
  background: none; border: none; cursor: pointer; padding: 0.1rem 0;
  font-size: 0.82rem; color: #6b675e;
}
.plot-outline-text {
  width: 100%; margin-top: 0.3rem; resize: vertical;
  font: inherit; padding: 0.4rem; border: 1px solid #d8d6cf; border-radius: 4px;
}
.plot-scene { margin-bottom: 0.7rem; }
.plot-scene-label { font-size: 0.74rem; color: #9a958b; margin-bottom: 0.25rem; }
.plot-scene-current .plot-scene-label { color: #2c2a26; font-weight: 600; }
.plot-scene-prev, .plot-scene-next { opacity: 0.72; }
.plot-empty { font-size: 0.76rem; color: #b3aea4; margin: 0.1rem 0; }
.plot-beat { margin-bottom: 0.3rem; }
.plot-beat-head {
  display: flex; align-items: center; gap: 0.4rem; width: 100%;
  background: none; border: none; cursor: pointer; padding: 0.15rem 0; text-align: left;
}
.plot-dot { width: 9px; height: 9px; border-radius: 50%; display: inline-block; flex: none; }
.plot-thread { font-size: 0.72rem; color: #8a857b; flex: none; }
.plot-label { font-size: 0.84rem; color: #2c2a26; }
.plot-edit { background: none; border: none; cursor: pointer; color: #9a958b; padding: 0 0.3rem; }
.plot-desc { font-size: 0.78rem; color: #6b675e; margin: 0.1rem 0 0.1rem 1rem; line-height: 1.4; }
.plot-beat-edit { display: flex; flex-direction: column; gap: 0.35rem; margin: 0.3rem 0 0.3rem 1rem; }
.plot-beat-edit textarea { resize: vertical; font: inherit; padding: 0.35rem; border: 1px solid #d8d6cf; border-radius: 4px; }
.plot-beat-edit-actions { display: flex; align-items: center; justify-content: space-between; }
.plot-intensity button { border: 1px solid #d8d6cf; background: #fff; cursor: pointer; padding: 0 0.4rem; margin-right: 0.2rem; border-radius: 3px; }
.plot-intensity button.sel { background: #2980b9; color: #fff; border-color: #2980b9; }
.plot-add { display: flex; flex-direction: column; gap: 0.3rem; margin: 0.3rem 0; }
.plot-add-btn {
  display: inline-flex; align-items: center; gap: 0.3rem;
  background: none; border: 1px dashed #d8d6cf; border-radius: 4px;
  cursor: pointer; padding: 0.25rem 0.5rem; font-size: 0.78rem; color: #6b675e;
}
.plot-add-btn:disabled { opacity: 0.5; cursor: default; }
```

- [ ] **Step 3: tsc + 커밋**

Run: `cd apps/desktop && npx tsc --noEmit`
Expected: PlotPanel.tsx 관련 에러 없음(남은 에러는 ContextPanel/Workspace).
```bash
git add apps/desktop/src/components/PlotPanel.tsx apps/desktop/src/components/PlotPanel.css
git commit -m "feat(desktop): Plan 24 T7 — inline PlotPanel (outline + prev/current/next beats)"
```

---

## Task 8: ThreadSheet에 beat description 편집

**Files:**
- Modify: `apps/desktop/src/components/ThreadSheet.tsx`

- [ ] **Step 1: description textarea + 패치 반영**

`ThreadSheet.tsx`:
- `updateBeat` 시그니처/본문 교체(description 허용):
```tsx
  const updateBeat = async (b: Beat, patch: { label?: string; description?: string; intensity?: number }) => {
    const next = { ...b, ...patch };
    setBeatList((prev) => prev.map((x) => (x.id === b.id ? next : x)));
    try {
      await beatsApi.update({ id: b.id, label: next.label, description: next.description, intensity: next.intensity });
    } catch (e) {
      setError(String(e));
    }
  };
```
- beat 행(`.beat-row`) 렌더에 label input 아래로 description textarea를 추가. `beatList.map((b) => (...))` 내부를 다음으로 교체:
```tsx
              <div className="beat-row" key={b.id}>
                <span className="beat-ordinal">#{b.ordinal}</span>
                <div className="beat-fields">
                  <input
                    className="attr-value"
                    value={b.label}
                    onChange={(e) => updateBeat(b, { label: e.target.value })}
                    placeholder="마디 제목"
                  />
                  <textarea
                    className="beat-desc"
                    value={b.description}
                    onChange={(e) => updateBeat(b, { description: e.target.value })}
                    placeholder="무슨 일이 일어나는지"
                    rows={2}
                  />
                </div>
                <div className="beat-intensity">
                  {[1, 2, 3].map((lvl) => (
                    <button
                      key={lvl}
                      type="button"
                      className={b.intensity === lvl ? "sel" : ""}
                      onClick={() => updateBeat(b, { intensity: lvl })}
                    >{lvl}</button>
                  ))}
                </div>
                <button type="button" className="attr-del" onClick={() => deleteBeat(b)} aria-label="삭제">
                  <X size={14} />
                </button>
              </div>
```
- `apps/desktop/src/components/ThreadSheet.css`에 추가:
```css
.beat-fields { display: flex; flex-direction: column; gap: 0.25rem; flex: 1; }
.beat-desc { resize: vertical; font: inherit; padding: 0.3rem; border: 1px solid #d8d6cf; border-radius: 4px; }
```
  (기존 `.beat-row`가 flex라면 `.beat-fields`가 자연스럽게 늘어남. 레이아웃이 깨지면 `.beat-row { align-items: flex-start; }` 추가.)

- [ ] **Step 2: tsc + 커밋**

Run: `cd apps/desktop && npx tsc --noEmit`
Expected: ThreadSheet 관련 에러 없음.
```bash
git add apps/desktop/src/components/ThreadSheet.tsx apps/desktop/src/components/ThreadSheet.css
git commit -m "feat(desktop): Plan 24 T8 — beat description editing in ThreadSheet"
```

---

## Task 9: ContextPanel/Workspace 배선 + ActiveThreadsPanel 제거

**Files:**
- Modify: `apps/desktop/src/components/ContextPanel.tsx`
- Modify: `apps/desktop/src/routes/Workspace.tsx`
- Delete: `apps/desktop/src/components/ActiveThreadsPanel.tsx`

- [ ] **Step 1: ContextPanel에서 PlotPanel 사용**

`ContextPanel.tsx`:
- import 교체: `import { ActiveThreadsPanel } from "./ActiveThreadsPanel";` → `import { PlotPanel } from "./PlotPanel";`
- `Props`에 `onProjectChanged?: (project: Project) => void;` 추가(이미 `project: Project` 있음).
- 함수 시그니처 구조분해에 `onProjectChanged` 추가.
- `<ActiveThreadsPanel .../>` 블록을 교체:
```tsx
      <PlotPanel
        project={project}
        nodeId={node.id}
        onOpenThread={onOpenThread}
        onProjectChanged={onProjectChanged}
      />
```
- `onThreadDataChanged` prop은 더 이상 PlotPanel이 쓰지 않는다. ContextPanel Props에서 `onThreadDataChanged?`는 남겨두되 미사용이면 제거(아래 Workspace에서도 함께 정리). 깔끔하게: Props에서 `onThreadDataChanged` 제거.

- [ ] **Step 2: Workspace에서 props 전달 + load.project 갱신**

`apps/desktop/src/routes/Workspace.tsx`의 `<ContextPanel .../>`(line ~865) 교체:
```tsx
          <ContextPanel
            project={load.project}
            node={load.node}
            charCount={charCount}
            typewriter={typewriter}
            onToggleTypewriter={() => setTypewriter((v) => !v)}
            saveStatus={saveStatus}
            mentionedEntities={mentioned}
            onMentionClick={(id) => setEntitySheetId(id)}
            onOpenThread={setThreadSheetId}
            onProjectChanged={(p) =>
              setLoad((prev) => (prev ? { ...prev, project: p } : prev))
            }
          />
```
  ※ `setLoad`의 실제 setter 이름/`load` 형태(`{ project, node, tree }`)는 Workspace 상단에서 확인해 맞춘다. `onThreadDataChanged` prop 라인은 제거.

- [ ] **Step 3: ActiveThreadsPanel 삭제 + 잔여 참조 정리**

```bash
git rm apps/desktop/src/components/ActiveThreadsPanel.tsx
```
`grep -rn "ActiveThreadsPanel" apps/desktop/src` 로 잔여 import가 없는지 확인(있으면 제거).

- [ ] **Step 4: tsc 클린 + 엔진 빌드 + 커밋**

Run: `cd apps/desktop && npx tsc --noEmit`
Expected: PASS (에러 0)

Run (repo root): `./build-engine.sh`
Expected: "... / ok"
```bash
git add apps/desktop/src/components/ContextPanel.tsx apps/desktop/src/routes/Workspace.tsx
git commit -m "feat(desktop): Plan 24 T9 — wire PlotPanel into ContextPanel/Workspace, drop ActiveThreadsPanel"
```

---

## 최종 검증 (모든 Task 후)

- [ ] `cd engine && go test ./...` — 전 패키지 PASS
- [ ] `cd apps/desktop && npx tsc --noEmit` — 클린
- [ ] repo root `./build-engine.sh` — ok
- [ ] 수동 스모크(Tauri, 사용자 영역): 다중 씬 프로젝트에서 (1) 우측 플롯 패널에 전/현/후 씬 beat 표시, (2) 개요 펼쳐 편집·자동 저장, (3) 현재/다음 씬에 비트 추가, (4) 비트 description 편집, (5) ThreadSheet에서 description 편집, (6) AI 생성 시 컨텍스트 체크리스트에 "작품 개요/플롯/관계"가 켜지고 "같은 장 다른 씬/활성 스토리라인"이 사라졌는지 확인.

## 범위 밖 (이번 플랜 아님)

- 서브시스템 ②(문체·톤·서식 일관성 규칙), ③(일관성 검증), ④(AI 대화 설정 관리) — 각각 별도 spec/plan.
- ThreadView 타임라인 라우트의 description 시각화.
- 개요의 구조화(막/섹션).
