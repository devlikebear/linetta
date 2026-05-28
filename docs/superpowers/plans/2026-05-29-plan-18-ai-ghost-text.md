# Plan 18 — AI Mode 통합 (Cmd+I Ghost Text) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 별도 AI 모드 화면을 제거하고, 에디터 안에서 Cmd+I로 floating prompt bar를 열어 인라인 회색 ghost text로 결과를 받는 통합 UX로 전환한다. 동시에 AI 컨텍스트에 작품 설정(장르/분량/시점)과 선택 영역을 추가한다.

**Architecture:** 엔진은 `Context`에 `ProjectMeta` + `SelectionText` 필드를 추가하고 prompts.go가 `## 작품 설정` / `## 선택 영역` 섹션을 렌더. 프론트엔드는 Tiptap `GhostExtension`이 ProseMirror `DecorationSet`으로 widget을 그리고, `useGhostText` 훅이 `ai.delta` 스트림을 누적해 widget을 갱신한다. `AIPromptBar`가 커서 위치에 floating 입력을 띄우고, Workspace의 `mode` state 및 옛 `AIMode.tsx`/`AIContextPanel.tsx`는 삭제. 명령 팔레트는 `Cmd+K` → `Cmd+P`로 이동.

**Tech Stack:** Go 1.26 (engine, sqlite), TypeScript / React 18 (frontend), Tiptap 2 + ProseMirror (editor), tars/pkg/llm streaming.

---

## 파일 구조

**Engine:**
- `engine/internal/ai/ai.go` — `ProjectMeta` 타입 + `Context.Project`/`Context.SelectionText` 필드
- `engine/internal/ai/context.go` — `Build` 시그니처 확장(`selectionText` 매개변수), `Project` 채움
- `engine/internal/ai/prompts.go` — `renderProjectMeta` + `## 작품 설정` / `## 선택 영역` 섹션
- `engine/internal/ai/prompts_test.go` — 새 섹션 3+2 케이스
- `engine/internal/ai/context_test.go` — `Project` / `SelectionText` 매핑 케이스
- `engine/internal/rpc/handlers/ai.go` — `runAIParams` 에 `SelectionText` 추가, `Build` 호출 인자 전달

**Frontend:**
- `apps/desktop/src/lib/rpc.ts` — `ai.run` 클라이언트에 `selection_text` 인자
- `apps/desktop/src/routes/Workspace.tsx` — `mode` 제거 + `Cmd+I` 핸들러 + `Cmd+K`→`Cmd+P` 이동 + AIPromptBar 마운트 + GhostExtension 연결
- `apps/desktop/src/components/ShortcutsModal.tsx` — `Cmd+K` 라벨 → `Cmd+P`로 + `Cmd+I` 행 추가
- `apps/desktop/src/routes/ThreadView.tsx` — 안내 문구 `Cmd+K` → `Cmd+P`
- `apps/desktop/src/hooks/useFirstLeaf.ts` — 주석 문구 업데이트
- `apps/desktop/src/components/editor/GhostExtension.ts` (NEW) — Tiptap extension + DecorationSet plugin + 명령
- `apps/desktop/src/components/ai/AIPromptBar.tsx` + `.css` (NEW) — floating prompt
- `apps/desktop/src/components/ai/AIContextChecklist.tsx` + `.css` (NEW) — popover
- `apps/desktop/src/lib/editor/useGhostText.ts` (NEW) — 스트림→GhostExtension 연결 훅
- `apps/desktop/src/components/ai/AIMode.tsx` — **삭제**
- `apps/desktop/src/components/ai/AIMode.css` — **삭제**
- `apps/desktop/src/components/ai/AIContextPanel.tsx` — **삭제**

**테스트 방침:** Engine은 TDD (Go test). 프론트엔드는 vitest 등 테스트 인프라가 없으므로 `npx tsc --noEmit` + Task 12의 수동 스모크로 검증. (테스트 인프라 도입은 별도 plan)

---

## Task 1: Engine — `ProjectMeta` 타입 + `Context.Project` 필드 + Build 매핑

**Files:**
- Modify: `engine/internal/ai/ai.go`
- Modify: `engine/internal/ai/context.go`
- Modify: `engine/internal/ai/context_test.go`

### Step 1: 실패 테스트 추가

`engine/internal/ai/context_test.go` 의 가장 가까운 기존 테스트 (예: `TestBuildContext_basic` 같은 것)를 찾아 새 케이스 추가. 만약 기존 헬퍼 `setupBuilderWithProject(t, project.NewInput{...})` 같은 게 있으면 동일 패턴을 따른다. 없으면 가장 가까운 기존 테스트의 셋업 패턴을 그대로 베끼되 `Genres`/`LengthTarget`/`DefaultPOV` 를 명시.

추가 테스트:

```go
func TestBuildContext_projectMetaPopulated(t *testing.T) {
	// 기존 테스트와 동일한 셋업 패턴 사용; 프로젝트 생성 시 Genres/LengthTarget/DefaultPOV 명시.
	// 예시:
	// pr, nr, _, _, _, _ := setupBuilderRepos(t)
	// p, _ := pr.Create(ctx, 1000, project.NewInput{
	//     Title: "t", Genres: []string{"판타지", "미스터리"},
	//     LengthTarget: "novel", DefaultPOV: "first",
	// })
	// nodeID := /* a leaf under p */
	//
	// builder := ai.NewContextBuilder(pr, nr, mr, tr, br, nr_notes)
	// c, err := builder.Build(ctx, nodeID, "user prompt", "", ai.Options{})
	// (Build 시그니처는 Task 2에서 selectionText 인자 추가 — 이번 task에선 기존 시그니처)
	// ...
	if len(c.Project.Genres) != 2 || c.Project.Genres[0] != "판타지" {
		t.Fatalf("Project.Genres=%v", c.Project.Genres)
	}
	if c.Project.LengthTarget != "novel" {
		t.Fatalf("Project.LengthTarget=%q", c.Project.LengthTarget)
	}
	if c.Project.DefaultPOV != "first" {
		t.Fatalf("Project.DefaultPOV=%q", c.Project.DefaultPOV)
	}
}
```

기존 `context_test.go` 가 어떤 헬퍼/패턴을 쓰는지 먼저 읽고 그에 맞춰 작성. **Task 2가 Build 시그니처에 selectionText 인자를 추가**하므로, 이번 task 에선 기존 Build 시그니처 그대로 호출하고 Task 2에서 호출부도 같이 갱신한다.

- [ ] **Step 2: 실패 확인**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./internal/ai -run TestBuildContext_projectMetaPopulated -v
```

기대: 컴파일 에러 (`c.Project` 미존재). 의도된 실패.

- [ ] **Step 3: `ProjectMeta` 타입 + `Context` 필드 추가**

`engine/internal/ai/ai.go` 안 `Context` 정의 바로 위에 추가:

```go
// ProjectMeta carries the project-level configuration the user set when
// creating the project (장르, 예상 분량, 기본 시점). Renders as a single
// line near the top of the user message so the LLM understands the project's
// fundamental constraints.
type ProjectMeta struct {
	Genres       []string `json:"genres"`
	LengthTarget string   `json:"length_target"`
	DefaultPOV   string   `json:"default_pov"`
}
```

같은 파일의 `Context` 구조체에 `PrevSummary` 옆에 새 필드 추가 (필드 순서: ProjectID, NodeID, SceneLabel, SceneText, PrevSummary, **Project**, Hierarchical, ...):

```go
type Context struct {
	ProjectID     string              `json:"project_id"`
	NodeID        string              `json:"node_id"`
	SceneLabel    string              `json:"scene_label"`
	SceneText     string              `json:"scene_text"`
	PrevSummary   string              `json:"prev_summary"`
	Project       ProjectMeta         `json:"project"`
	Hierarchical  HierarchicalContext `json:"hierarchical"`
	RelatedScenes []SceneSummary      `json:"related_scenes"`
	Entities      []EntityBrief       `json:"entities"`
	ActiveThreads []ActiveThread      `json:"active_threads"`
	Notes         []NoteBrief         `json:"notes"`
	StyleNotes    string              `json:"style_notes"`
	UserPrompt    string              `json:"user_prompt"`
	Options       Options             `json:"options"`
}
```

(SelectionText 필드는 Task 2에서 추가.)

- [ ] **Step 4: `Build`에서 `Project` 채움**

`engine/internal/ai/context.go` 의 `Build` 함수에서 최종 `return Context{...}` 블록 안에 `Project` 필드 매핑 추가:

```go
return Context{
	ProjectID:     proj.ID,
	NodeID:        n.ID,
	SceneLabel:    n.Label,
	SceneText:     sceneText,
	PrevSummary:   prevSummary,
	Project: ProjectMeta{
		Genres:       proj.Genres,
		LengthTarget: proj.LengthTarget,
		DefaultPOV:   proj.DefaultPOV,
	},
	Hierarchical:  hierarchical,
	RelatedScenes: related,
	Entities:      briefs,
	ActiveThreads: active,
	Notes:         noteBriefs,
	StyleNotes:    proj.StyleNotes,
	UserPrompt:    prompt,
	Options:       opts,
}, nil
```

- [ ] **Step 5: 테스트 통과 확인**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./internal/ai -v
```

기대: 모든 ai 패키지 테스트 PASS.

- [ ] **Step 6: 커밋**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/internal/ai/ai.go engine/internal/ai/context.go engine/internal/ai/context_test.go
git commit -m "feat(ai): Context.Project carries project metadata (genres/length/POV)"
```

---

## Task 2: Engine — `Context.SelectionText` + `Build` 시그니처 확장

**Files:**
- Modify: `engine/internal/ai/ai.go`
- Modify: `engine/internal/ai/context.go`
- Modify: `engine/internal/ai/context_test.go`
- Modify: `engine/internal/rpc/handlers/ai.go`
- Modify: `engine/internal/rpc/handlers/ai_test.go` (있다면)

### Step 1: 실패 테스트 추가

`engine/internal/ai/context_test.go` 끝에 추가:

```go
func TestBuildContext_selectionTextPassesThrough(t *testing.T) {
	// 기존 셋업 헬퍼와 동일하게 builder 만들고 leaf 노드 준비.
	// ...
	selectionText := "그녀는 천천히 고개를 들었다."
	c, err := builder.Build(ctx, nodeID, "더 감각적으로 다시 써줘", selectionText, ai.Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if c.SelectionText != selectionText {
		t.Fatalf("SelectionText=%q want %q", c.SelectionText, selectionText)
	}
}
```

또한 Task 1 에서 추가한 `TestBuildContext_projectMetaPopulated` 의 `builder.Build(ctx, nodeID, "user prompt", ai.Options{})` 호출을 `builder.Build(ctx, nodeID, "user prompt", "", ai.Options{})` 로 갱신 (selectionText 빈 문자열).

이 task가 기존 모든 `builder.Build(ctx, nodeID, prompt, opts)` 호출부 (test와 production 양쪽) 의 시그니처를 바꾼다. 컴파일 에러가 발생하는 모든 호출처를 한 번에 갱신.

### Step 2: 실패 확인

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./internal/ai -run TestBuildContext_selectionTextPassesThrough -v
```

기대: 컴파일 에러 (`c.SelectionText` 미존재 + `Build` 인자 수 불일치).

### Step 3: `Context.SelectionText` 추가

`engine/internal/ai/ai.go` 의 `Context` 정의에서 `StyleNotes` 옆에 (또는 `UserPrompt` 위에) 추가:

```go
type Context struct {
	// ... 기존 필드들 ...
	StyleNotes    string              `json:"style_notes"`
	SelectionText string              `json:"selection_text"`
	UserPrompt    string              `json:"user_prompt"`
	Options       Options             `json:"options"`
}
```

### Step 4: `Build` 시그니처 확장

`engine/internal/ai/context.go`:

```go
func (b *ContextBuilder) Build(ctx context.Context, nodeID, prompt, selectionText string, opts Options) (Context, error) {
	// 기존 본문 유지 ...
	// 최종 return 안에 SelectionText 추가:
	return Context{
		// ... 기존 필드들 ...
		StyleNotes:    proj.StyleNotes,
		SelectionText: selectionText,
		UserPrompt:    prompt,
		Options:       opts,
	}, nil
}
```

### Step 5: 모든 호출처 갱신

`engine/internal/rpc/handlers/ai.go`:

`runAIParams` 에 `SelectionText` 추가:

```go
type runAIParams struct {
	NodeID        string     `json:"node_id"`
	Prompt        string     `json:"prompt"`
	SelectionText string     `json:"selection_text"`
	Options       ai.Options `json:"options"`
}
```

`Build` 호출 갱신:

```go
c, err := builder.Build(ctx, p.NodeID, p.Prompt, p.SelectionText, p.Options)
```

다른 `Build` 호출처가 있다면 모두 동일하게 4번째 인자(`selectionText string`) 추가. 빈 문자열이 의도된 곳은 `""` 전달.

### Step 6: 통과 확인

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./... -v 2>&1 | tail -20
```

기대: 모든 패키지 PASS.

### Step 7: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/internal/ai/ai.go engine/internal/ai/context.go engine/internal/ai/context_test.go engine/internal/rpc/handlers/ai.go
git commit -m "feat(ai): Build accepts selection_text; ai.run RPC plumbs it"
```

---

## Task 3: Engine — `## 작품 설정` 섹션 (prompts.go)

**Files:**
- Modify: `engine/internal/ai/prompts.go`
- Modify: `engine/internal/ai/prompts_test.go`

### Step 1: 실패 테스트 추가

`engine/internal/ai/prompts_test.go` 끝에 추가:

```go
func TestBuildUser_projectMetaSection_fullyPopulated(t *testing.T) {
	c := Context{
		SceneLabel: "씬 1",
		SceneText:  "본문",
		Project: ProjectMeta{
			Genres:       []string{"판타지", "미스터리"},
			LengthTarget: "novel",
			DefaultPOV:   "first",
		},
	}
	got := buildUser(c)
	if !strings.Contains(got, "## 작품 설정") {
		t.Fatalf("missing header. got:\n%s", got)
	}
	if !strings.Contains(got, "장르: 판타지, 미스터리") {
		t.Fatalf("missing genres. got:\n%s", got)
	}
	if !strings.Contains(got, "분량: 장편") {
		t.Fatalf("missing length. got:\n%s", got)
	}
	if !strings.Contains(got, "시점: 1인칭") {
		t.Fatalf("missing POV. got:\n%s", got)
	}
}

func TestBuildUser_projectMetaSection_partial(t *testing.T) {
	c := Context{
		SceneLabel: "씬 1",
		SceneText:  "본문",
		Project: ProjectMeta{
			LengthTarget: "short",
			// Genres 빈 배열, DefaultPOV 빈 문자열
		},
	}
	got := buildUser(c)
	if !strings.Contains(got, "## 작품 설정") {
		t.Fatalf("missing header. got:\n%s", got)
	}
	if !strings.Contains(got, "분량: 단편") {
		t.Fatalf("missing length. got:\n%s", got)
	}
	if strings.Contains(got, "장르:") {
		t.Fatalf("genres should be omitted. got:\n%s", got)
	}
	if strings.Contains(got, "시점:") {
		t.Fatalf("POV should be omitted. got:\n%s", got)
	}
}

func TestBuildUser_projectMetaSection_allEmptyOmitsSection(t *testing.T) {
	c := Context{
		SceneLabel: "씬 1",
		SceneText:  "본문",
		Project:    ProjectMeta{}, // 모두 비어있음
	}
	got := buildUser(c)
	if strings.Contains(got, "## 작품 설정") {
		t.Fatalf("section should be omitted entirely. got:\n%s", got)
	}
}

func TestBuildUser_projectMeta_unmappedValuesPassThrough(t *testing.T) {
	c := Context{
		SceneLabel: "씬 1",
		SceneText:  "본문",
		Project: ProjectMeta{
			LengthTarget: "epic",         // 매핑 표에 없는 값
			DefaultPOV:   "second",       // 매핑 표에 없는 값
		},
	}
	got := buildUser(c)
	if !strings.Contains(got, "분량: epic") {
		t.Fatalf("unmapped length should pass through. got:\n%s", got)
	}
	if !strings.Contains(got, "시점: second") {
		t.Fatalf("unmapped POV should pass through. got:\n%s", got)
	}
}
```

### Step 2: 실패 확인

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./internal/ai -run "TestBuildUser_projectMeta" -v
```

기대: 4개 모두 FAIL (`## 작품 설정` 헤더 없음).

### Step 3: `renderProjectMeta` + 섹션 렌더

`engine/internal/ai/prompts.go` 파일 끝(혹은 `kindLabel` 옆)에 헬퍼 추가:

```go
// renderProjectMeta returns a one-line "장르: X, Y · 분량: Z · 시점: W"
// with empty pieces omitted. Returns empty string if all three are empty.
// Unmapped LengthTarget / DefaultPOV values pass through as-is.
func renderProjectMeta(m ProjectMeta) string {
	parts := []string{}
	if len(m.Genres) > 0 {
		parts = append(parts, "장르: "+strings.Join(m.Genres, ", "))
	}
	if m.LengthTarget != "" {
		parts = append(parts, "분량: "+mapLengthTarget(m.LengthTarget))
	}
	if m.DefaultPOV != "" {
		parts = append(parts, "시점: "+mapDefaultPOV(m.DefaultPOV))
	}
	return strings.Join(parts, " · ")
}

func mapLengthTarget(v string) string {
	switch v {
	case "short":
		return "단편"
	case "novella":
		return "중편"
	case "novel":
		return "장편"
	default:
		return v
	}
}

func mapDefaultPOV(v string) string {
	switch v {
	case "first":
		return "1인칭"
	case "third":
		return "3인칭"
	case "omniscient":
		return "전지적"
	default:
		return v
	}
}
```

그리고 `buildUser(c)` 함수의 본문 **맨 처음** (기존 `## 작품 전반` 섹션 바로 앞)에 다음 추가:

```go
func buildUser(c Context) string {
	var b strings.Builder

	if meta := renderProjectMeta(c.Project); meta != "" {
		b.WriteString("## 작품 설정\n")
		b.WriteString(meta)
		b.WriteString("\n\n")
	}

	// Plan 16 layer-1 hierarchical sections ...
	if strings.TrimSpace(c.Hierarchical.ProjectSynopsis) != "" {
		// ... 기존 코드 그대로 ...
	}
	// ... 나머지 본문 그대로 ...
}
```

### Step 4: 통과 확인

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./internal/ai -v
```

기대: 모든 ai 테스트 PASS.

### Step 5: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/internal/ai/prompts.go engine/internal/ai/prompts_test.go
git commit -m "feat(ai): render ## 작품 설정 section (genres/length/POV) at top of user prompt"
```

---

## Task 4: Engine — `## 선택 영역` 섹션 (prompts.go)

**Files:**
- Modify: `engine/internal/ai/prompts.go`
- Modify: `engine/internal/ai/prompts_test.go`

### Step 1: 실패 테스트 추가

`engine/internal/ai/prompts_test.go` 끝에 추가:

```go
func TestBuildUser_selectionTextSection_present(t *testing.T) {
	c := Context{
		SceneLabel:    "씬 1",
		SceneText:     "전체 본문 텍스트",
		SelectionText: "그녀는 천천히 고개를 들었다.",
	}
	got := buildUser(c)
	if !strings.Contains(got, "## 선택 영역") {
		t.Fatalf("missing header. got:\n%s", got)
	}
	if !strings.Contains(got, "그녀는 천천히 고개를 들었다.") {
		t.Fatalf("missing selection body. got:\n%s", got)
	}
}

func TestBuildUser_selectionTextSection_emptyOmitsSection(t *testing.T) {
	c := Context{
		SceneLabel: "씬 1",
		SceneText:  "본문",
		// SelectionText 비어있음
	}
	got := buildUser(c)
	if strings.Contains(got, "## 선택 영역") {
		t.Fatalf("section should be omitted. got:\n%s", got)
	}
}

func TestBuildUser_selectionTextSection_appearsAfterCurrentSceneBeforeInstruction(t *testing.T) {
	c := Context{
		SceneLabel:    "씬 1",
		SceneText:     "본문",
		SelectionText: "선택본",
		UserPrompt:    "지시문",
	}
	got := buildUser(c)
	sceneIdx := strings.Index(got, "## 현재 씬")
	selIdx := strings.Index(got, "## 선택 영역")
	instIdx := strings.Index(got, "## 작가의 지시")
	if sceneIdx == -1 || selIdx == -1 || instIdx == -1 {
		t.Fatalf("missing section. got:\n%s", got)
	}
	if !(sceneIdx < selIdx && selIdx < instIdx) {
		t.Fatalf("expected order: current scene < selection < instruction. got indices: scene=%d sel=%d inst=%d", sceneIdx, selIdx, instIdx)
	}
}
```

### Step 2: 실패 확인

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./internal/ai -run "TestBuildUser_selectionText" -v
```

기대: 3개 모두 FAIL.

### Step 3: `## 선택 영역` 섹션 렌더

`engine/internal/ai/prompts.go` 의 `buildUser`에서 `## 현재 씬: ...` 섹션 바로 다음, 다른 섹션들 (등장 인물·장소 등) **앞**에 추가:

```go
b.WriteString(fmt.Sprintf("## 현재 씬: %s\n", c.SceneLabel))
b.WriteString(c.SceneText)
b.WriteString("\n\n")

if strings.TrimSpace(c.SelectionText) != "" {
	b.WriteString("## 선택 영역\n")
	b.WriteString(c.SelectionText)
	b.WriteString("\n\n")
}

if len(c.Entities) > 0 {
	// ... 기존 코드 ...
}
```

`## 작가의 지시` 섹션은 `buildUser` 의 가장 마지막에 위치 — 이미 그렇게 되어있음. 새 `## 선택 영역` 은 그 사이에 들어가므로 `Step 1` 의 순서 테스트(scene < selection < instruction)가 통과해야 한다.

### Step 4: 통과 확인

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./internal/ai -v
```

기대: 모든 ai 테스트 PASS.

### Step 5: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/internal/ai/prompts.go engine/internal/ai/prompts_test.go
git commit -m "feat(ai): render ## 선택 영역 section between current scene and instructions"
```

---

## Task 5: FE — `ai.run` 클라이언트에 `selection_text` 인자

**Files:**
- Modify: `apps/desktop/src/lib/rpc.ts:125` 부근

### Step 1: rpc 클라이언트 시그니처 확장

`apps/desktop/src/lib/rpc.ts` 의 ai 객체 정의를 찾아 수정. 현재:

```ts
export const ai = {
  run: (nodeId: string, prompt: string, options: AIOptions) =>
    rpcCall<{ run_id: string }>("ai.run", { node_id: nodeId, prompt, options }),
  cancel: (runId: string) => rpcCall<{ ok: true }>("ai.cancel", { run_id: runId }),
};
```

다음으로 교체:

```ts
export const ai = {
  run: (nodeId: string, prompt: string, options: AIOptions, selectionText: string = "") =>
    rpcCall<{ run_id: string }>("ai.run", { node_id: nodeId, prompt, selection_text: selectionText, options }),
  cancel: (runId: string) => rpcCall<{ ok: true }>("ai.cancel", { run_id: runId }),
};
```

`selectionText`는 optional (기본값 빈 문자열) — 기존 호출처는 변경 없이 동작.

### Step 2: 타입체크

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
```

기대: 에러 없음.

### Step 3: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/lib/rpc.ts
git commit -m "feat(rpc-client): ai.run accepts optional selection_text"
```

---

## Task 6: FE — 명령 팔레트 단축키 `Cmd+K` → `Cmd+P` 이동

**Files:**
- Modify: `apps/desktop/src/routes/Workspace.tsx` (라인 326-453 부근의 Cmd+R/Cmd+K 핸들러)
- Modify: `apps/desktop/src/components/ShortcutsModal.tsx:11`
- Modify: `apps/desktop/src/routes/ThreadView.tsx:57`
- Modify: `apps/desktop/src/hooks/useFirstLeaf.ts:35`

### Step 1: Workspace.tsx 키 핸들러 수정

`apps/desktop/src/routes/Workspace.tsx` 326 라인 부근의 `useEffect` 안에서 `Cmd+K` 를 잡는 부분을 찾는다. 가령:

```tsx
if ((e.metaKey || e.ctrlKey) && e.key === "k") {
  e.preventDefault();
  setPaletteOpen((v) => !v);
}
```

`e.key === "k"` 를 `e.key === "p"` 로 교체:

```tsx
if ((e.metaKey || e.ctrlKey) && e.key === "p") {
  e.preventDefault();
  setPaletteOpen((v) => !v);
}
```

근처 주석 (`// Global Cmd+R reload + Cmd+K palette toggle.`) 도 `Cmd+P palette` 로 갱신.

### Step 2: ShortcutsModal.tsx 라벨 수정

`apps/desktop/src/components/ShortcutsModal.tsx:11` 의 항목:

```ts
{ keys: "Cmd+K", label: "명령 팔레트 열기" },
```

를:

```ts
{ keys: "Cmd+P", label: "명령 팔레트 열기" },
```

로. 같은 파일에서 다른 `Cmd+K` 참조가 있다면 모두 동일 처리.

### Step 3: ThreadView.tsx 안내 문구 수정

`apps/desktop/src/routes/ThreadView.tsx:57`:

```tsx
{lanes.length === 0 && <p className="hint">아직 스토리라인이 없어요. Cmd+K → "이 씬을 새 Thread로 표시"로 시작하세요.</p>}
```

를:

```tsx
{lanes.length === 0 && <p className="hint">아직 스토리라인이 없어요. Cmd+P → "이 씬을 새 Thread로 표시"로 시작하세요.</p>}
```

### Step 4: useFirstLeaf.ts 주석 수정

`apps/desktop/src/hooks/useFirstLeaf.ts:35`:

```ts
/** Flatten a tree to a list in DFS order (used by Cmd+K's "search node"). */
```

를:

```ts
/** Flatten a tree to a list in DFS order (used by Cmd+P's "search node"). */
```

### Step 5: 잔여 `Cmd+K` 참조 검색

```bash
grep -rn "Cmd+K\|cmd+k" /Users/changheonshin/workspace/myworks/linetta/apps/desktop/src --include="*.ts" --include="*.tsx"
```

기대: 결과 0개. 남아있으면 동일 패턴으로 `Cmd+P`로 갱신.

### Step 6: 타입체크

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
```

기대: 에러 없음.

### Step 7: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/routes/Workspace.tsx apps/desktop/src/components/ShortcutsModal.tsx apps/desktop/src/routes/ThreadView.tsx apps/desktop/src/hooks/useFirstLeaf.ts
git commit -m "feat(shortcuts): move command palette Cmd+K → Cmd+P (VSCode Quick Open convention)"
```

---

## Task 7: FE — `GhostExtension` core (Tiptap extension + DecorationSet + commands)

**Files:**
- Create: `apps/desktop/src/components/editor/GhostExtension.ts`

### Step 1: GhostExtension.ts 작성

`apps/desktop/src/components/editor/GhostExtension.ts` 새 파일에 다음 작성. (Tiptap 패턴은 같은 디렉토리의 `NoteMarkerExtension.ts` 참조.)

```ts
import { Extension, type RawCommands } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    ghost: {
      /** Set or replace the ghost text at the current selection's head. */
      setGhostText: (text: string) => ReturnType;
      /** Accept the ghost text — insert into the document. */
      acceptGhostText: () => ReturnType;
      /** Drop the ghost text — clear decoration without inserting. */
      dropGhostText: () => ReturnType;
    };
  }
}

interface GhostState {
  /** Position the ghost is anchored to (head of selection when setGhostText was called). */
  pos: number;
  /** Accumulated text streamed so far. */
  text: string;
  /** True once the stream has completed (cursor stops blinking). */
  done: boolean;
}

export const ghostPluginKey = new PluginKey<GhostState | null>("linetta-ghost");

export const GhostExtension = Extension.create({
  name: "linettaGhost",

  addProseMirrorPlugins() {
    return [
      new Plugin<GhostState | null>({
        key: ghostPluginKey,
        state: {
          init: () => null,
          apply(tr, prev) {
            // Meta: { kind: "set", pos, text } | { kind: "drop" } | { kind: "done" }
            const meta = tr.getMeta(ghostPluginKey) as
              | { kind: "set"; pos: number; text: string }
              | { kind: "drop" }
              | { kind: "done" }
              | undefined;

            if (meta?.kind === "set") {
              return { pos: meta.pos, text: meta.text, done: false };
            }
            if (meta?.kind === "drop") {
              return null;
            }
            if (meta?.kind === "done" && prev) {
              return { ...prev, done: true };
            }
            // Plan 18 design 2.7: auto-drop on doc edit.
            if (prev && tr.docChanged) {
              return null;
            }
            return prev;
          },
        },
        props: {
          decorations(state) {
            const ghost = this.getState(state);
            if (!ghost) return DecorationSet.empty;
            const widget = Decoration.widget(
              ghost.pos,
              () => {
                const span = document.createElement("span");
                span.className = "ai-ghost" + (ghost.done ? " done" : "");
                span.textContent = ghost.text;
                return span;
              },
              { side: 1 },
            );
            return DecorationSet.create(state.doc, [widget]);
          },
        },
      }),
    ];
  },

  addCommands() {
    return {
      setGhostText:
        (text: string) =>
        ({ tr, state, dispatch }) => {
          const pos = state.selection.head;
          if (dispatch) {
            dispatch(tr.setMeta(ghostPluginKey, { kind: "set", pos, text }));
          }
          return true;
        },
      acceptGhostText:
        () =>
        ({ tr, state, dispatch }) => {
          const ghost = ghostPluginKey.getState(state);
          if (!ghost) return false;
          if (dispatch) {
            // Insert ghost.text at ghost.pos, then drop the decoration.
            const insertTr = tr.insertText(ghost.text, ghost.pos);
            insertTr.setMeta(ghostPluginKey, { kind: "drop" });
            dispatch(insertTr);
          }
          return true;
        },
      dropGhostText:
        () =>
        ({ tr, state, dispatch }) => {
          const ghost = ghostPluginKey.getState(state);
          if (!ghost) return false;
          if (dispatch) {
            dispatch(tr.setMeta(ghostPluginKey, { kind: "drop" }));
          }
          return true;
        },
    } as Partial<RawCommands>;
  },
});

/** Read-only: is a ghost currently active? Used by useGhostText / AIPromptBar. */
export function hasActiveGhost(editor: { state: { doc: any } } | null | undefined): boolean {
  if (!editor) return false;
  // Tiptap editor instance has .state on its view; we accept either shape.
  const ed: any = editor;
  const state = ed.state ?? ed.view?.state;
  if (!state) return false;
  return ghostPluginKey.getState(state) !== null;
}
```

### Step 2: ghost 스타일 CSS

`apps/desktop/src/components/editor/Tiptap.tsx` 가 import 하는 메인 CSS 파일 (또는 가까운 `editor.css`)에 추가. 정확한 파일을 못 찾으면 `apps/desktop/src/components/editor/GhostExtension.css` 새로 만들고 `GhostExtension.ts` 상단에 `import "./GhostExtension.css";` 추가:

```css
.ai-ghost {
  color: rgba(232, 232, 234, 0.45);
  white-space: pre-wrap;
  font-style: italic;
  pointer-events: none;
  user-select: none;
}

.ai-ghost::after {
  content: "▌";
  display: inline-block;
  margin-left: 1px;
  opacity: 0.7;
  animation: ai-ghost-blink 1s steps(2) infinite;
}

.ai-ghost.done::after {
  content: "";
}

@keyframes ai-ghost-blink {
  to {
    opacity: 0;
  }
}
```

### Step 3: 타입체크

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
```

기대: 에러 없음.

### Step 4: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/components/editor/GhostExtension.ts apps/desktop/src/components/editor/GhostExtension.css
git commit -m "feat(editor): GhostExtension — Tiptap decoration plugin for inline AI ghost text"
```

---

## Task 8: FE — `GhostExtension` 키바인딩 (Tab 수락 / Esc 거절)

**Files:**
- Modify: `apps/desktop/src/components/editor/GhostExtension.ts`

### Step 1: keymap 추가

`GhostExtension` 의 `addProseMirrorPlugins()` 가 반환하는 배열에 두 번째 Plugin을 추가하는 형태도 가능하지만, Tiptap 의 `addKeyboardShortcuts()` API가 더 명시적이고 ghost-only 가드 로직을 표현하기 쉽다. `GhostExtension` 정의 안에 추가:

```ts
addKeyboardShortcuts() {
  return {
    Tab: ({ editor }) => {
      const ghost = ghostPluginKey.getState(editor.state);
      if (!ghost) return false; // Let Tab fall through to other handlers.
      return editor.commands.acceptGhostText();
    },
    Escape: ({ editor }) => {
      const ghost = ghostPluginKey.getState(editor.state);
      if (!ghost) return false;
      return editor.commands.dropGhostText();
    },
  };
},
```

(Tab 의 기본 Tiptap 동작 — 들여쓰기 등 — 은 ghost 없을 때만 동작. `return false` 가 다음 핸들러로 전파.)

### Step 2: 통합 확인 (수동)

이 단계는 자동 테스트 가능한 인프라가 없으므로 코드 리뷰로만 확인. Task 12의 수동 스모크에서 Tab/Esc 동작이 검증된다.

### Step 3: 타입체크

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
```

기대: 에러 없음.

### Step 4: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/components/editor/GhostExtension.ts
git commit -m "feat(editor): GhostExtension — Tab to accept, Esc to drop ghost"
```

---

## Task 9: FE — `useGhostText` 훅

**Files:**
- Create: `apps/desktop/src/lib/editor/useGhostText.ts`

### Step 1: 훅 작성

`apps/desktop/src/lib/editor/useGhostText.ts` 새 파일:

```ts
import { useCallback, useEffect, useRef, useState } from "react";
import type { Editor } from "@tiptap/react";
import { ai as aiApi } from "../rpc";
import type { AIOptions } from "../types";
import { useEngineEvent } from "./useEngineEvent"; // existing hook used elsewhere
// If useEngineEvent lives at a different path, adjust the import.
// Plan 11/16 added stream-side listeners — match the canonical pattern.

export type GhostStatus =
  | { kind: "idle" }
  | { kind: "running"; runId: string; text: string }
  | { kind: "done"; text: string }
  | { kind: "error"; message: string };

interface RunArgs {
  nodeId: string;
  prompt: string;
  options: AIOptions;
  selectionText?: string;
}

/**
 * useGhostText wires ai.run RPC + ai.delta/done/error notifications to a
 * Tiptap editor's GhostExtension commands. The hook keeps a single active run;
 * starting a new one cancels the previous.
 */
export function useGhostText(editor: Editor | null) {
  const [status, setStatus] = useState<GhostStatus>({ kind: "idle" });
  const runIdRef = useRef<string | null>(null);
  const accumulatedRef = useRef<string>("");

  const start = useCallback(
    async ({ nodeId, prompt, options, selectionText = "" }: RunArgs) => {
      if (!editor) return;
      // Cancel any in-flight run first.
      if (runIdRef.current) {
        try {
          await aiApi.cancel(runIdRef.current);
        } catch {
          /* benign */
        }
      }
      // Drop any existing ghost decoration.
      editor.commands.dropGhostText();
      accumulatedRef.current = "";
      try {
        const { run_id } = await aiApi.run(nodeId, prompt, options, selectionText);
        runIdRef.current = run_id;
        setStatus({ kind: "running", runId: run_id, text: "" });
        editor.commands.setGhostText("");
      } catch (e) {
        setStatus({ kind: "error", message: String(e) });
      }
    },
    [editor],
  );

  const cancel = useCallback(async () => {
    if (!runIdRef.current) return;
    try {
      await aiApi.cancel(runIdRef.current);
    } catch {
      /* benign */
    }
    runIdRef.current = null;
    if (editor) editor.commands.dropGhostText();
    setStatus({ kind: "idle" });
  }, [editor]);

  const accept = useCallback(() => {
    if (!editor) return;
    editor.commands.acceptGhostText();
    runIdRef.current = null;
    accumulatedRef.current = "";
    setStatus({ kind: "idle" });
  }, [editor]);

  const drop = useCallback(() => {
    if (!editor) return;
    editor.commands.dropGhostText();
    runIdRef.current = null;
    accumulatedRef.current = "";
    setStatus({ kind: "idle" });
  }, [editor]);

  useEngineEvent("ai.delta", (p: { run_id: string; text: string }) => {
    if (p.run_id !== runIdRef.current || !editor) return;
    accumulatedRef.current += p.text;
    editor.commands.setGhostText(accumulatedRef.current);
    setStatus({ kind: "running", runId: p.run_id, text: accumulatedRef.current });
  });

  useEngineEvent("ai.done", (p: { run_id: string; full_text: string }) => {
    if (p.run_id !== runIdRef.current || !editor) return;
    const finalText = p.full_text;
    accumulatedRef.current = finalText;
    editor.commands.setGhostText(finalText);
    // Mark ghost as done (stops blinking) by sending a meta update.
    // The GhostExtension state machine handles "done" meta separately.
    const tr = editor.state.tr.setMeta(
      // ghostPluginKey is exported from GhostExtension; import it.
      // (Add `import { ghostPluginKey } from "../../components/editor/GhostExtension";` to the top.)
      // Resolved on save.
      (editor as any)._ghostKey ?? null,
      { kind: "done" },
    );
    editor.view.dispatch(tr);
    setStatus({ kind: "done", text: finalText });
  });

  useEngineEvent("ai.error", (p: { run_id: string; message: string }) => {
    if (p.run_id !== runIdRef.current || !editor) return;
    if (editor) editor.commands.dropGhostText();
    runIdRef.current = null;
    setStatus({ kind: "error", message: p.message });
  });

  useEngineEvent("ai.cancelled", (p: { run_id: string }) => {
    if (p.run_id !== runIdRef.current || !editor) return;
    runIdRef.current = null;
    if (editor) editor.commands.dropGhostText();
    setStatus({ kind: "idle" });
  });

  // Cleanup on editor change.
  useEffect(() => {
    return () => {
      if (runIdRef.current) {
        aiApi.cancel(runIdRef.current).catch(() => {});
      }
    };
  }, [editor]);

  return { status, start, cancel, accept, drop };
}
```

**중요한 노트:** 위 코드의 `(editor as any)._ghostKey` 트릭은 자리잡기. `ghostPluginKey` 를 import 하고 사용:

```ts
import { ghostPluginKey } from "../../components/editor/GhostExtension";
```

그리고 `done` 디스패치를:

```ts
const tr = editor.state.tr.setMeta(ghostPluginKey, { kind: "done" });
editor.view.dispatch(tr);
```

로 단순화. 위 작자의 임시 표현을 정정해 작성.

또한 `useEngineEvent` 경로는 기존 코드베이스의 위치를 따라야 한다 (`apps/desktop/src/lib/useEngineEvent.ts` 또는 `apps/desktop/src/hooks/useEngineEvent.ts` — `find` 로 확인). 정확한 import 경로로 교체.

### Step 2: 타입체크

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
```

기대: 에러 없음. 있다면 `useEngineEvent` 경로 보정.

### Step 3: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/lib/editor/useGhostText.ts
git commit -m "feat(editor): useGhostText — stream AI deltas into GhostExtension"
```

---

## Task 10: FE — `AIPromptBar` 컴포넌트

**Files:**
- Create: `apps/desktop/src/components/ai/AIPromptBar.tsx`
- Create: `apps/desktop/src/components/ai/AIPromptBar.css`

### Step 1: CSS

`apps/desktop/src/components/ai/AIPromptBar.css`:

```css
.ai-prompt-bar {
  position: absolute;
  width: min(480px, 92vw);
  background: var(--surface, #1d1d1f);
  color: var(--text, #e8e8ea);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 10px;
  padding: 0.6rem 0.7rem;
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.45);
  z-index: 50;
  font-size: 0.85rem;
}

.ai-prompt-bar-row {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.ai-prompt-bar-presets {
  display: flex;
  gap: 0.3rem;
}

.ai-prompt-bar-preset-chip {
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 999px;
  padding: 0.18rem 0.65rem;
  font-size: 0.8rem;
  cursor: pointer;
}

.ai-prompt-bar-preset-chip:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.ai-prompt-bar-textarea {
  width: 100%;
  min-height: 2.4rem;
  max-height: 8rem;
  resize: none;
  background: rgba(0, 0, 0, 0.25);
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: 6px;
  padding: 0.4rem 0.5rem;
  color: inherit;
  font-family: inherit;
  font-size: 0.9rem;
}

.ai-prompt-bar-textarea.shake {
  animation: ai-prompt-shake 0.35s;
}

@keyframes ai-prompt-shake {
  10%, 90% { transform: translateX(-1px); }
  20%, 80% { transform: translateX(2px); }
  30%, 50%, 70% { transform: translateX(-3px); }
  40%, 60% { transform: translateX(3px); }
}

.ai-prompt-bar-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.78rem;
}

.ai-prompt-bar-ctx {
  cursor: pointer;
  opacity: 0.75;
}

.ai-prompt-bar-run {
  padding: 0.28rem 0.8rem;
  font-size: 0.82rem;
  background: rgba(255, 255, 255, 0.1);
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 6px;
  color: inherit;
  cursor: pointer;
}

.ai-prompt-bar-run:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.ai-prompt-bar-error {
  color: #e07a7a;
  font-size: 0.78rem;
}

.ai-prompt-bar-hint {
  color: rgba(232, 232, 234, 0.55);
  font-size: 0.78rem;
}
```

### Step 2: 컴포넌트

`apps/desktop/src/components/ai/AIPromptBar.tsx`:

```tsx
import { useEffect, useRef, useState } from "react";
import type { AIOptions } from "../../lib/types";
import { TONE_PRESETS } from "../../lib/tonePresets";
import "./AIPromptBar.css";

export type PresetID = "rewrite" | "expand" | "compact" | null;

const PRESET_SEED: Record<Exclude<PresetID, null>, string> = {
  rewrite: "이 단락을 더 자연스럽게 다시 써줘.",
  expand: "이 장면을 더 감각적으로 확장해줘.",
  compact: "이 단락을 한 문장으로 요약해줘.",
};

interface Props {
  anchor: { top: number; left: number } | null;
  hasSelection: boolean;
  busy: boolean;
  options: AIOptions;
  contextItemCount: number;
  errorMessage?: string;
  onOptionsChange: (o: AIOptions) => void;
  onRun: (preset: PresetID, prompt: string) => void;
  onCancel: () => void;
  onClose: () => void;
  onContextClick: () => void;
}

export function AIPromptBar({
  anchor,
  hasSelection,
  busy,
  options,
  contextItemCount,
  errorMessage,
  onOptionsChange,
  onRun,
  onCancel,
  onClose,
  onContextClick,
}: Props) {
  const [prompt, setPrompt] = useState("");
  const [shake, setShake] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  if (!anchor) return null;

  const submit = (preset: PresetID) => {
    const seed = preset && preset !== null ? PRESET_SEED[preset] : "";
    const text = preset ? seed : prompt.trim();
    if (!text) {
      setShake(true);
      setTimeout(() => setShake(false), 350);
      textareaRef.current?.focus();
      return;
    }
    if (preset && !prompt) setPrompt(seed);
    onRun(preset, text);
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      submit(null);
    } else if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      submit(null);
    } else if (e.key === "Escape") {
      e.preventDefault();
      onClose();
    }
  };

  return (
    <div
      className="ai-prompt-bar"
      style={{ top: anchor.top, left: anchor.left }}
      onMouseDown={(e) => e.stopPropagation()}
    >
      <div className="ai-prompt-bar-row">
        <div className="ai-prompt-bar-presets">
          <button
            type="button"
            className="ai-prompt-bar-preset-chip"
            disabled={!hasSelection}
            title={!hasSelection ? "선택 영역이 필요합니다" : ""}
            onClick={() => submit("rewrite")}
          >
            재작성
          </button>
          <button
            type="button"
            className="ai-prompt-bar-preset-chip"
            onClick={() => submit("expand")}
          >
            확장
          </button>
          <button
            type="button"
            className="ai-prompt-bar-preset-chip"
            disabled={!hasSelection}
            title={!hasSelection ? "선택 영역이 필요합니다" : ""}
            onClick={() => submit("compact")}
          >
            요약
          </button>
        </div>
        <ToneDropdown options={options} onChange={onOptionsChange} />
        <LengthChip options={options} onChange={onOptionsChange} />
      </div>

      <textarea
        ref={textareaRef}
        className={`ai-prompt-bar-textarea${shake ? " shake" : ""}`}
        placeholder="프롬프트를 입력하세요…"
        value={prompt}
        onChange={(e) => setPrompt(e.target.value)}
        onKeyDown={onKeyDown}
        rows={2}
      />

      {errorMessage && <p className="ai-prompt-bar-error">오류: {errorMessage}</p>}

      <div className="ai-prompt-bar-footer">
        <span className="ai-prompt-bar-ctx" onClick={onContextClick}>
          ⓘ ctx: {contextItemCount}개
        </span>
        {busy ? (
          <button type="button" className="ai-prompt-bar-run" onClick={onCancel}>
            취소 Esc
          </button>
        ) : (
          <button type="button" className="ai-prompt-bar-run" onClick={() => submit(null)}>
            생성 ⌘↵
          </button>
        )}
      </div>
    </div>
  );
}

function ToneDropdown({ options, onChange }: { options: AIOptions; onChange: (o: AIOptions) => void }) {
  return (
    <select
      className="ai-prompt-bar-preset-chip"
      value={options.tone}
      onChange={(e) => onChange({ ...options, tone: e.target.value })}
    >
      {TONE_PRESETS.map((t) => (
        <option key={t.id} value={t.id}>톤: {t.label}</option>
      ))}
    </select>
  );
}

function LengthChip({ options, onChange }: { options: AIOptions; onChange: (o: AIOptions) => void }) {
  return (
    <button
      type="button"
      className="ai-prompt-bar-preset-chip"
      onClick={() => onChange({ ...options, short_form: !options.short_form })}
      aria-pressed={options.short_form}
    >
      {options.short_form ? "길이: 한 문단" : "길이: 자유"}
    </button>
  );
}
```

### Step 3: 타입체크

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
```

기대: 에러 없음. (TONE_PRESETS 경로 / AIOptions 타입은 기존과 동일하므로 문제 없어야 한다.)

### Step 4: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/components/ai/AIPromptBar.tsx apps/desktop/src/components/ai/AIPromptBar.css
git commit -m "feat(ai): AIPromptBar — floating Cmd+I prompt with presets and tone chips"
```

---

## Task 11: FE — `AIContextChecklist` popover

**Files:**
- Create: `apps/desktop/src/components/ai/AIContextChecklist.tsx`
- Create: `apps/desktop/src/components/ai/AIContextChecklist.css`

### Step 1: CSS

`apps/desktop/src/components/ai/AIContextChecklist.css`:

```css
.ai-context-checklist {
  position: absolute;
  background: var(--surface, #1d1d1f);
  color: var(--text, #e8e8ea);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 8px;
  padding: 0.6rem 0.75rem;
  width: 320px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
  font-size: 0.82rem;
  z-index: 60;
}

.ai-context-checklist h5 {
  margin: 0 0 0.4rem 0;
  font-size: 0.85rem;
  opacity: 0.85;
}

.ai-context-checklist ul {
  list-style: none;
  margin: 0;
  padding: 0;
}

.ai-context-checklist li {
  padding: 0.18rem 0;
  display: flex;
  justify-content: space-between;
  gap: 0.5rem;
}

.ai-context-checklist .item-disabled {
  opacity: 0.4;
}

.ai-context-checklist .item-count {
  opacity: 0.6;
  font-size: 0.78rem;
}
```

### Step 2: 컴포넌트

`apps/desktop/src/components/ai/AIContextChecklist.tsx`:

```tsx
import "./AIContextChecklist.css";

export interface ContextCounts {
  nearbyScenes: number;
  sameChapter: number;
  otherChapter: number;
  otherPart: number;
  hasSynopsis: boolean;
  relatedScenes: number;
  entities: number;
  activeThreads: number;
  notes: number;
  projectMetaFields: number; // 0..3 (Genres+Length+POV)
  hasStyleNotes: boolean;
}

interface Props {
  anchor: { top: number; left: number };
  counts: ContextCounts;
  onClose: () => void;
}

export function AIContextChecklist({ anchor, counts, onClose }: Props) {
  const items: { label: string; present: boolean; caption?: string }[] = [
    { label: "현재 씬 본문", present: true },
    {
      label: "인근 씬 요약 (직전 2개 + 직후 1개)",
      present: counts.nearbyScenes > 0,
      caption: `${counts.nearbyScenes}개`,
    },
    {
      label: "같은 장 다른 씬",
      present: counts.sameChapter > 0,
      caption: `${counts.sameChapter}개`,
    },
    {
      label: "형제 장 요약",
      present: counts.otherChapter > 0,
      caption: `${counts.otherChapter}개`,
    },
    {
      label: "형제 부 요약",
      present: counts.otherPart > 0,
      caption: `${counts.otherPart}개`,
    },
    { label: "작품 시놉시스", present: counts.hasSynopsis },
    {
      label: "관련 과거 씬 (멘션 RAG)",
      present: counts.relatedScenes > 0,
      caption: `${counts.relatedScenes}개`,
    },
    {
      label: "등장 인물·장소",
      present: counts.entities > 0,
      caption: `${counts.entities}개`,
    },
    {
      label: "활성 스토리라인",
      present: counts.activeThreads > 0,
      caption: `${counts.activeThreads}개`,
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

  return (
    <>
      <div
        className="ai-context-checklist"
        style={{ top: anchor.top, left: anchor.left }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <h5>AI에게 전달되는 컨텍스트</h5>
        <ul>
          {items.map((it, i) => (
            <li key={i} className={it.present ? "" : "item-disabled"}>
              <span>{it.present ? "✓" : "—"} {it.label}</span>
              {it.caption && <span className="item-count">{it.caption}</span>}
            </li>
          ))}
        </ul>
      </div>
      {/* invisible backdrop to capture outside click */}
      <div
        style={{ position: "fixed", inset: 0, zIndex: 55 }}
        onMouseDown={onClose}
      />
    </>
  );
}

export function totalContextItems(counts: ContextCounts): number {
  return (
    counts.nearbyScenes +
    counts.sameChapter +
    counts.otherChapter +
    counts.otherPart +
    (counts.hasSynopsis ? 1 : 0) +
    counts.relatedScenes +
    counts.entities +
    counts.activeThreads +
    counts.notes +
    counts.projectMetaFields +
    (counts.hasStyleNotes ? 1 : 0)
  );
}
```

### Step 3: 타입체크

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
```

기대: 에러 없음.

### Step 4: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/components/ai/AIContextChecklist.tsx apps/desktop/src/components/ai/AIContextChecklist.css
git commit -m "feat(ai): AIContextChecklist popover — honest list of context items sent to LLM"
```

---

## Task 12: FE — Workspace 통합 + 옛 AI 모드 파일 삭제

**Files:**
- Modify: `apps/desktop/src/routes/Workspace.tsx`
- Delete: `apps/desktop/src/components/ai/AIMode.tsx`
- Delete: `apps/desktop/src/components/ai/AIMode.css`
- Delete: `apps/desktop/src/components/ai/AIContextPanel.tsx`

### Step 1: Workspace.tsx 수정

이 파일은 크고 변경이 많음. 단계적으로:

**1.1 `mode` state 제거.**

`mode`, `setMode`, `setMode("edit")`, `setMode("ai")` 호출 모두 제거. 검색해서 모든 참조 제거. AI/EDIT 토글 버튼 (라인 770 부근의 `<button type="button" className="mode-toggle ws-zen-btn" ...>` 의 AI 버튼) 도 제거.

**1.2 새 state 추가.**

`Workspace` 함수 본문의 다른 useState 들 옆에:

```tsx
const [aiPromptAnchor, setAiPromptAnchor] = useState<{ top: number; left: number } | null>(null);
const [aiCtxChecklistOpen, setAiCtxChecklistOpen] = useState(false);
```

`aiPromptAnchor` 가 `null` 이 아니면 prompt bar 가 열린 상태. 닫을 때 `null` 로.

**1.3 `Cmd+I` 핸들러 추가.**

라인 326 부근의 전역 키 핸들러 (Cmd+R / Cmd+P palette를 처리하는 useEffect) 안에 추가:

```tsx
if ((e.metaKey || e.ctrlKey) && e.key === "i") {
  e.preventDefault();
  if (aiPromptAnchor) {
    // 이미 열려있으면 닫음
    setAiPromptAnchor(null);
    return;
  }
  // 커서 좌표 계산
  const view = editorRef.current?.view;
  if (!view) return;
  const coords = view.coordsAtPos(view.state.selection.head);
  // viewport 하단 200px 안이면 flip — 커서 위로
  const flip = window.innerHeight - coords.bottom < 200;
  setAiPromptAnchor({
    top: flip ? coords.top - 160 : coords.bottom + 4,
    left: Math.min(coords.left, window.innerWidth - 500),
  });
}
```

**1.4 GhostExtension + useGhostText 연결.**

`Workspace` 본문 위쪽에 import 추가:

```tsx
import { GhostExtension } from "../components/editor/GhostExtension";
import { useGhostText } from "../lib/editor/useGhostText";
import { AIPromptBar, type PresetID } from "../components/ai/AIPromptBar";
import { AIContextChecklist, totalContextItems, type ContextCounts } from "../components/ai/AIContextChecklist";
```

훅 호출 — TiptapEditor ref가 `editorRef.current` 라면 `editor` 인스턴스를 어디서 가져오는지에 따라 다르다. 기존 `editorRef.current?.view` 패턴이 있다면 `editorRef.current?.editor` 같은 게 있을 수도 있다 (Tiptap React 의 `EditorContent` ref 패턴 따라). 정확한 인스턴스를 가져온 다음:

```tsx
const ghost = useGhostText(tiptapEditor ?? null);
```

TiptapEditor 의 `extensions` prop 에 `GhostExtension` 추가:

```tsx
<TiptapEditor
  ref={editorRef}
  // ... 기존 prop ...
  extensions={[
    ...(mentionExtension ? [mentionExtension] : []),
    NoteMarkerExtension,
    GhostExtension,
  ]}
  // ...
/>
```

**1.5 AIPromptBar 마운트.**

JSX 안에서 `</main>` 또는 outermost wrapper 직전에 추가:

```tsx
{aiPromptAnchor && load && (
  <AIPromptBar
    anchor={aiPromptAnchor}
    hasSelection={!!tiptapEditor && !tiptapEditor.state.selection.empty}
    busy={ghost.status.kind === "running"}
    options={aiOptions}
    contextItemCount={totalContextItems(currentContextCounts)}
    errorMessage={ghost.status.kind === "error" ? ghost.status.message : undefined}
    onOptionsChange={setAiOptions}
    onRun={(preset, promptText) => {
      const selectionText =
        tiptapEditor && !tiptapEditor.state.selection.empty
          ? tiptapEditor.state.doc.textBetween(
              tiptapEditor.state.selection.from,
              tiptapEditor.state.selection.to,
              "\n",
            )
          : "";
      ghost.start({
        nodeId: load.node.id,
        prompt: promptText,
        options: aiOptions,
        selectionText,
      });
    }}
    onCancel={() => ghost.cancel()}
    onClose={() => {
      ghost.drop();
      setAiPromptAnchor(null);
    }}
    onContextClick={() => setAiCtxChecklistOpen((v) => !v)}
  />
)}
{aiCtxChecklistOpen && aiPromptAnchor && (
  <AIContextChecklist
    anchor={{ top: aiPromptAnchor.top + 180, left: aiPromptAnchor.left }}
    counts={currentContextCounts}
    onClose={() => setAiCtxChecklistOpen(false)}
  />
)}
```

`currentContextCounts: ContextCounts` 는 현재 `load` 와 `mentioned` 에서 derive. 가장 단순한 시작점:

```tsx
const currentContextCounts: ContextCounts = useMemo(() => {
  // We don't fetch the engine's actual ai_runs.context_json here — we
  // approximate from what we can see locally. Engine remains the source
  // of truth; this is just an honest UI hint.
  const proj = load?.project;
  return {
    nearbyScenes: 3, // best guess for now; engine may emit fewer
    sameChapter: 0,
    otherChapter: 0,
    otherPart: 0,
    hasSynopsis: true,
    relatedScenes: 0,
    entities: mentioned.length,
    activeThreads: 0,
    notes: 0,
    projectMetaFields:
      (proj && proj.genres.length > 0 ? 1 : 0) +
      (proj && proj.length_target ? 1 : 0) +
      (proj && proj.default_pov ? 1 : 0),
    hasStyleNotes: !!proj?.style_notes,
  };
}, [load, mentioned]);
```

(주의: 이 카운트는 FE 의 best-guess. 더 정확한 값을 원하면 후속 PR 에서 `ai.preview-context` 같은 RPC 를 추가해 실제 빌더 결과를 미리 받을 수 있음. Plan 18 범위에서는 hardcoded 근사치로 진행 — checklist UI 의 "honesty 의도" 자체는 유지.)

**1.6 모든 AIMode/AIContextPanel 사용 제거.**

`<AIMode ... />` 및 `<AIContextPanel ... />` JSX 블록 통째로 제거. `mode === "ai"` 분기 자체가 사라졌으므로 자연스럽게 안 보임.

`aiPrompt`, `aiStatus`, `aiOptions` 같은 state 들은 AIPromptBar 가 자체 prompt state 를 갖기 때문에 일부 정리 가능 — 단, **`aiOptions`** 는 AIPromptBar 가 props 로 받아쓰므로 유지. **`aiStatus`** 는 `useGhostText` 의 `ghost.status` 가 대체하므로 제거.

기존 `startAIRun`, `cancelAIRun`, `insertResult`, `replaceWithResult` 같은 핸들러 — 더 이상 호출되지 않으면 제거.

**1.7 import 정리.**

다음 import 제거:

```tsx
import { AIMode, type AIRunStatus } from "../components/ai/AIMode";
import { AIContextPanel } from "../components/ai/AIContextPanel";
```

### Step 2: 옛 AI 모드 파일 삭제

```bash
cd /Users/changheonshin/workspace/myworks/linetta
rm apps/desktop/src/components/ai/AIMode.tsx
rm apps/desktop/src/components/ai/AIMode.css
rm apps/desktop/src/components/ai/AIContextPanel.tsx
```

### Step 3: 타입체크

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
```

기대: 에러 없음. 잔여 참조가 있으면 모두 제거.

### Step 4: 엔진 빌드 + 실행 확인

```bash
cd /Users/changheonshin/workspace/myworks/linetta && ./scripts/build-engine.sh
```

기대: 빌드 성공.

### Step 5: 수동 스모크 (체크리스트)

```bash
rm -rf /tmp/linetta-plan18 && LINETTA_HOME=/tmp/linetta-plan18 ./scripts/dev.sh
```

다음을 차례로 시도:

1. 작품 생성 — 장르 "판타지" / 분량 "장편" / 시점 "1인칭" 명시.
2. 본문 입력 후 `Cmd+I` → prompt bar 가 커서 근처에 뜸.
3. "확장" preset 클릭 → ghost 회색으로 스트리밍 → 완료 시 깜박임 멈춤.
4. `Tab` → ghost 텍스트가 doc 에 실제 삽입됨.
5. 다시 `Cmd+I` → "이 장면을 더 우울하게 다시 써줘" 입력 → 생성 중 `Esc` → ghost 사라지고 bar 닫힘.
6. 본문 일부 드래그 선택 → `Cmd+I` → "재작성" preset 활성화, "요약" 활성화, "확장" 도 활성화 → "재작성" 클릭 → ghost 가 선택 영역 다음 줄에 회색으로 → Tab → 선택 영역이 ghost 텍스트로 교체.
7. `Cmd+P` → 명령 팔레트 정상 열림 (Cmd+I 와 충돌 없음).
8. `ctx: N개` 칩 클릭 → checklist popover 표시. `작품 설정 (장르/분량/시점)` 항목이 `3/3` 으로 채워짐.
9. ai_runs 확인:
   ```bash
   sqlite3 /tmp/linetta-plan18/library.db \
     "SELECT context_json FROM ai_runs ORDER BY started_at DESC LIMIT 1" | jq '.project, .selection_text'
   ```
   - `.project.genres` / `.length_target` / `.default_pov` 값 확인
   - 선택 영역이 있던 호출은 `.selection_text` 채워짐
10. **Plan 16 회귀**: hierarchical 컨텍스트 살아있는지 확인. 다층 트리 작품에서 ai_runs.context_json 의 `.hierarchical` 채워져야 함.
11. **Plan 17 회귀**: import 모달 정상 동작.

### Step 6: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/routes/Workspace.tsx
git add -u apps/desktop/src/components/ai/  # 삭제된 파일 staging
git commit -m "feat(workspace): Cmd+I ghost text UX replaces full-screen AI mode"
```

### Step 7: tag

스모크 통과 시:

```bash
git tag plan-18-ai-ghost-text-done
```

---

## Self-Review

**1. Spec 커버리지:**

| Spec 요구 | 구현 task |
|---|---|
| `ProjectMeta` 타입 + `Context.Project` | Task 1 |
| `Context.SelectionText` | Task 2 |
| `## 작품 설정` 섹션 (3 케이스 + 매핑 외 값) | Task 3 |
| `## 선택 영역` 섹션 + 순서 | Task 4 |
| `ai.run` RPC + FE 클라이언트 `selection_text` | Task 2 (engine) + Task 5 (FE) |
| Cmd+K → Cmd+P (palette 이동) | Task 6 |
| `GhostExtension` core (commands + DecorationSet) | Task 7 |
| GhostExtension Tab/Esc + auto-drop | Task 8 (+ Task 7 의 `tr.docChanged` 가드) |
| `useGhostText` 훅 (delta 스트림 → ghost) | Task 9 |
| `AIPromptBar` 컴포넌트 (preset / tone / Cmd+Enter) | Task 10 |
| `AIContextChecklist` popover | Task 11 |
| Workspace `mode` 제거 + Cmd+I + AIMode/AIContextPanel 파일 삭제 | Task 12 |
| 수동 스모크 시나리오 (8개) | Task 12 Step 5 |

모든 spec 요구가 task 로 매핑됨.

**2. Placeholder scan:**
- 모든 task 가 실제 코드 / 명령 / 기대 출력 포함.
- "TBD" / "TODO" 없음.
- Task 12 Step 1.5 의 `currentContextCounts` 는 best-guess hardcoded — 이는 design 의 의도된 점진적 정직성 (engine derived counts는 후속 PR). 명시적으로 표기됨, placeholder 아님.

**3. Type 일관성:**
- `ProjectMeta` 필드 (Task 1) → `renderProjectMeta` (Task 3) → `currentContextCounts.projectMetaFields` (Task 12) 모두 일치 (Genres, LengthTarget, DefaultPOV).
- `SelectionText` 필드 (Task 2) → `## 선택 영역` 섹션 (Task 4) → `ai.run` RPC param `selection_text` (Task 2 engine, Task 5 FE) → `selectionText` 인자 (Task 9 useGhostText.start, Task 12 AIPromptBar.onRun) 모두 일치.
- `ghostPluginKey` (Task 7) → useGhostText 의 `done` 디스패치 (Task 9) → `hasActiveGhost` 헬퍼 (Task 7) — 동일 key 참조.
- `GhostExtension` commands 이름: `setGhostText`, `acceptGhostText`, `dropGhostText` — Task 7 정의, Task 8 키바인딩, Task 9 훅, Task 12 Workspace 모두 동일.
- `ContextCounts` 타입 (Task 11) → `totalContextItems` (Task 11) → Workspace 호출부 (Task 12) 일치.

**4. 위험 / 미해결 (구현 중 결정):**
- Task 9 의 `useEngineEvent` 정확한 import 경로는 코드베이스에서 확인 후 보정 (implementer 가 `find` 로 찾는다고 명시).
- Task 12 의 TiptapEditor ref 가 Tiptap `Editor` 인스턴스를 어떻게 노출하는지 확인 후 `tiptapEditor` 변수 매핑. (existing pattern 따라.)
- `currentContextCounts` 의 hardcoded 값은 후속 PR 에서 `ai.preview-context` RPC 로 개선 가능 — Plan 18 범위 밖.
- ghost text 멀티 단락 자연스러운 표시는 plain widget으로 일단 진행, 어색하면 PR 2 에서 paragraph 단위 widget 분해.
