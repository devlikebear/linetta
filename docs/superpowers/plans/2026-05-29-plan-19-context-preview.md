# Plan 19 — AI Context Preview (실제 카운트) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** AIContextChecklist / AIPromptBar 의 `ⓘ ctx: N개` 칩이 하드코딩 대신 engine 의 실제 ContextBuilder 결과에서 추출한 정직한 counts 를 표시한다.

**Architecture:** Engine 에 새 RPC `ai.preview_context` 를 추가 — node id 받아 `ContextBuilder.Build(ctx, nodeID, "", "", Options{})` 호출 후 결과 Context 에서 `PreviewCounts` 만 추출해 JSON 으로 반환. Frontend rpc client 가 snake_case → camelCase 매핑하고 Workspace 가 Cmd+I 시점에 fetch 해서 state 로 보관, AIPromptBar/AIContextChecklist 에 prop drilling. Stale 응답은 reqId ref 로 차단.

**Tech Stack:** Go 1.26 (engine), TypeScript / React 18 (frontend), Tauri 2 + stdio JSONRPC.

---

## 파일 구조

**Engine:**
- `engine/internal/ai/ai.go` — `+PreviewCounts` 타입 정의
- `engine/internal/ai/context.go` — `+CountsFromContext` 순수 헬퍼
- `engine/internal/ai/context_test.go` — `+TestCountsFromContext_*` (3 케이스)
- `engine/internal/rpc/handlers/ai.go` — `+PreviewContext` 핸들러 + `+previewContextParams`
- `engine/internal/rpc/handlers/ai_test.go` — `+TestPreviewContext_*` (2 케이스)
- `engine/cmd/linetta-engine/main.go:165` 부근 — `+s.Handle("ai.preview_context", ...)`

**Frontend:**
- `apps/desktop/src/lib/types.ts` — `+ContextPreviewResponse` wire 타입
- `apps/desktop/src/lib/rpc.ts:123` 부근 — `+ai.previewContext` 클라이언트 (snake→camel 매핑 포함)
- `apps/desktop/src/routes/Workspace.tsx` — 하드코딩 `currentContextCounts` 제거, state/ref/fetch/clear 추가, `FALLBACK_COUNTS` 모듈 상수

---

## Task 1: Engine — `PreviewCounts` 타입 + `CountsFromContext` 헬퍼

**Files:**
- Modify: `engine/internal/ai/ai.go`
- Modify: `engine/internal/ai/context.go`
- Modify: `engine/internal/ai/context_test.go`

### Step 1: 실패 테스트 추가

`engine/internal/ai/context_test.go` 끝에 추가:

```go
func TestCountsFromContext_fullyPopulated(t *testing.T) {
	c := Context{
		Project: ProjectMeta{
			Genres:       []string{"판타지"},
			LengthTarget: "novel",
			DefaultPOV:   "first",
		},
		Hierarchical: HierarchicalContext{
			NearbyLeafSummaries:   []SceneSummary{{}, {}, {}},
			SameChapterSummaries:  []SceneSummary{{}, {}},
			OtherChapterSummaries: []ChapterSummary{{}},
			OtherPartSummaries:    []PartSummary{{}, {}},
			ProjectSynopsis:       "이 작품은…",
		},
		RelatedScenes: []SceneSummary{{}, {}, {}},
		Entities:      []EntityBrief{{}, {}, {}, {}},
		ActiveThreads: []ActiveThread{{}, {}},
		Notes:         []NoteBrief{{}},
		StyleNotes:    "내 톤은…",
	}
	got := CountsFromContext(c)
	if got.NearbyScenes != 3 || got.SameChapter != 2 || got.OtherChapter != 1 || got.OtherPart != 2 {
		t.Fatalf("hierarchical counts mismatch: %+v", got)
	}
	if !got.HasSynopsis {
		t.Fatalf("HasSynopsis should be true: %+v", got)
	}
	if got.RelatedScenes != 3 || got.Entities != 4 || got.ActiveThreads != 2 || got.Notes != 1 {
		t.Fatalf("collection counts mismatch: %+v", got)
	}
	if got.ProjectMetaFields != 3 {
		t.Fatalf("ProjectMetaFields=%d want 3", got.ProjectMetaFields)
	}
	if !got.HasStyleNotes {
		t.Fatalf("HasStyleNotes should be true: %+v", got)
	}
}

func TestCountsFromContext_emptyContext(t *testing.T) {
	got := CountsFromContext(Context{})
	if got.NearbyScenes != 0 || got.SameChapter != 0 || got.OtherChapter != 0 || got.OtherPart != 0 {
		t.Fatalf("counts should be zero: %+v", got)
	}
	if got.HasSynopsis || got.HasStyleNotes {
		t.Fatalf("booleans should be false: %+v", got)
	}
	if got.RelatedScenes != 0 || got.Entities != 0 || got.ActiveThreads != 0 || got.Notes != 0 {
		t.Fatalf("collection counts should be zero: %+v", got)
	}
	if got.ProjectMetaFields != 0 {
		t.Fatalf("ProjectMetaFields=%d want 0", got.ProjectMetaFields)
	}
}

func TestCountsFromContext_partialProjectMeta(t *testing.T) {
	c := Context{
		Project: ProjectMeta{
			Genres: []string{"판타지"},
			// LengthTarget, DefaultPOV 비어있음
		},
		Hierarchical: HierarchicalContext{
			ProjectSynopsis: "   ", // whitespace-only — should be treated as empty
		},
		StyleNotes: "  \n  ", // whitespace-only
	}
	got := CountsFromContext(c)
	if got.ProjectMetaFields != 1 {
		t.Fatalf("ProjectMetaFields=%d want 1 (Genres only)", got.ProjectMetaFields)
	}
	if got.HasSynopsis {
		t.Fatalf("HasSynopsis should be false (whitespace-only)")
	}
	if got.HasStyleNotes {
		t.Fatalf("HasStyleNotes should be false (whitespace-only)")
	}
}
```

### Step 2: 실패 확인

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./internal/ai -run TestCountsFromContext -v
```

기대: 컴파일 에러 (`CountsFromContext` 와 `PreviewCounts` 미존재).

### Step 3: `PreviewCounts` 타입 추가

`engine/internal/ai/ai.go` 의 다른 타입 정의들 옆에 (예: `ProjectMeta` 옆 또는 파일 끝부분) 추가:

```go
// PreviewCounts is the structural summary of a built Context, used by the
// frontend's AIContextChecklist to honestly display what will be sent to the
// LLM before the user runs a generation.
type PreviewCounts struct {
	NearbyScenes      int  `json:"nearby_scenes"`
	SameChapter       int  `json:"same_chapter"`
	OtherChapter      int  `json:"other_chapter"`
	OtherPart         int  `json:"other_part"`
	HasSynopsis       bool `json:"has_synopsis"`
	RelatedScenes     int  `json:"related_scenes"`
	Entities          int  `json:"entities"`
	ActiveThreads     int  `json:"active_threads"`
	Notes             int  `json:"notes"`
	ProjectMetaFields int  `json:"project_meta_fields"`
	HasStyleNotes     bool `json:"has_style_notes"`
}
```

### Step 4: `CountsFromContext` 헬퍼 구현

`engine/internal/ai/context.go` 의 파일 끝 (`docToPlainText` 같은 헬퍼들 옆) 에 추가:

```go
// CountsFromContext extracts a PreviewCounts from a fully-built Context.
// Pure function — no I/O. Whitespace-only strings are treated as empty.
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
	return PreviewCounts{
		NearbyScenes:      len(c.Hierarchical.NearbyLeafSummaries),
		SameChapter:       len(c.Hierarchical.SameChapterSummaries),
		OtherChapter:      len(c.Hierarchical.OtherChapterSummaries),
		OtherPart:         len(c.Hierarchical.OtherPartSummaries),
		HasSynopsis:       strings.TrimSpace(c.Hierarchical.ProjectSynopsis) != "",
		RelatedScenes:     len(c.RelatedScenes),
		Entities:          len(c.Entities),
		ActiveThreads:     len(c.ActiveThreads),
		Notes:             len(c.Notes),
		ProjectMetaFields: projectMeta,
		HasStyleNotes:     strings.TrimSpace(c.StyleNotes) != "",
	}
}
```

(`strings` 패키지는 `context.go` 가 이미 import 한다.)

### Step 5: 통과 확인

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./internal/ai -v && gofmt -l ./internal/ai
```

기대: 모든 ai 패키지 PASS. `gofmt -l` 빈 출력.

### Step 6: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/internal/ai/ai.go engine/internal/ai/context.go engine/internal/ai/context_test.go
git commit -m "feat(ai): PreviewCounts type + CountsFromContext helper"
```

---

## Task 2: Engine — `ai.preview_context` RPC 핸들러 + 등록

**Files:**
- Modify: `engine/internal/rpc/handlers/ai.go`
- Modify: `engine/internal/rpc/handlers/ai_test.go`
- Modify: `engine/cmd/linetta-engine/main.go:165` 부근

### Step 1: 실패 테스트 추가

`engine/internal/rpc/handlers/ai_test.go` 의 기존 RunAI 테스트 패턴을 따라 새 테스트 추가. 먼저 파일 상단의 helper / 셋업을 확인 (`setupAIBuilder(t)` 같은 게 있다면 그대로 따른다; 없으면 기존 RunAI 테스트의 inline boilerplate 차용):

```go
func TestPreviewContext_returnsCountsForValidNode(t *testing.T) {
	ctx := context.Background()
	// 동일한 셋업 패턴 — 기존 RunAI 테스트의 store/project/node/builder 셋업을 그대로.
	// 핵심: builder 와 leaf 노드 ID 가 준비되면 됨.
	// ... existing setup boilerplate ...

	h := PreviewContext(builder)

	params, _ := json.Marshal(map[string]any{
		"node_id": leafID,
	})
	raw, err := h(ctx, params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got struct {
		NearbyScenes      int  `json:"nearby_scenes"`
		SameChapter       int  `json:"same_chapter"`
		OtherChapter      int  `json:"other_chapter"`
		OtherPart         int  `json:"other_part"`
		HasSynopsis       bool `json:"has_synopsis"`
		RelatedScenes     int  `json:"related_scenes"`
		Entities          int  `json:"entities"`
		ActiveThreads     int  `json:"active_threads"`
		Notes             int  `json:"notes"`
		ProjectMetaFields int  `json:"project_meta_fields"`
		HasStyleNotes     bool `json:"has_style_notes"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// 최소한 ProjectMetaFields 가 fixture project 의 값에 따라 결정되어야 함.
	// 예: fixture 가 Genres=[]string{"판타지"}, LengthTarget="novel", DefaultPOV="first" 이면 ProjectMetaFields == 3.
	if got.ProjectMetaFields != 3 {
		t.Fatalf("ProjectMetaFields=%d want 3 (fixture has all three set)", got.ProjectMetaFields)
	}
}

func TestPreviewContext_rejectsEmptyNodeID(t *testing.T) {
	ctx := context.Background()
	// 동일 셋업 — builder 만 있으면 됨, leaf 안 만들어도 됨.
	// ... existing setup ...

	h := PreviewContext(builder)

	params, _ := json.Marshal(map[string]any{
		"node_id": "",
	})
	_, err := h(ctx, params)
	if err == nil {
		t.Fatal("expected InvalidParams error for empty node_id")
	}
	// rpc.MethodError 체크 (기존 RunAI 테스트가 이미 그 패턴을 사용한다면 차용)
	var mErr *rpc.MethodError
	if !errors.As(err, &mErr) || mErr.Code != rpc.CodeInvalidParams {
		t.Fatalf("expected rpc.MethodError CodeInvalidParams, got %T %v", err, err)
	}
}
```

**중요:** 기존 `ai_test.go` 안의 `TestRunAI_*` 의 setup 패턴 (store/project/node/builder 인스턴스 생성) 을 정확히 따라야 한다. 헬퍼 함수가 있다면 그것을, 없다면 inline boilerplate 를 그대로 차용. fixture project 의 `Genres`/`LengthTarget`/`DefaultPOV` 값을 위 assertion 에 맞춰 명시.

### Step 2: 실패 확인

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./internal/rpc/handlers -run TestPreviewContext -v
```

기대: 컴파일 에러 (`PreviewContext` 미존재).

### Step 3: 핸들러 구현

`engine/internal/rpc/handlers/ai.go` 의 파일 끝 (RunAI/CancelAI 아래) 에 추가:

```go
type previewContextParams struct {
	NodeID string `json:"node_id"`
}

// PreviewContext returns a handler for ai.preview_context. It builds the full
// Context for the given node and returns just the PreviewCounts JSON — used by
// the frontend's AIContextChecklist to display honest counts before the user
// runs a generation.
func PreviewContext(builder *ai.ContextBuilder) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p previewContextParams
		if err := json.Unmarshal(params, &p); err != nil || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id required"}
		}
		c, err := builder.Build(ctx, p.NodeID, "", "", ai.Options{})
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(ai.CountsFromContext(c))
	}
}
```

(`ai`, `rpc`, `json`, `context` 는 기존 RunAI 가 이미 import 한다.)

### Step 4: main.go 등록

`engine/cmd/linetta-engine/main.go:165` 의 `s.Handle("ai.run", ...)` 바로 아래에 한 줄 추가:

```go
s.Handle("ai.preview_context", handlers.PreviewContext(contextBuilder))
```

(`contextBuilder` 는 main.go:94 에서 이미 만들어진 변수.)

### Step 5: 통과 확인 + 엔진 빌드

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./... && gofmt -l ./...
cd /Users/changheonshin/workspace/myworks/linetta && ./scripts/build-engine.sh
```

기대: 모든 패키지 PASS. `gofmt -l` 빈 출력. 엔진 빌드 성공.

### Step 6: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/internal/ai/ai.go engine/internal/rpc/handlers/ai.go engine/internal/rpc/handlers/ai_test.go engine/cmd/linetta-engine/main.go
git commit -m "feat(rpc): add ai.preview_context — return PreviewCounts for a node"
```

(Task 1 에서 `ai.go` 가 이미 커밋되어 있어도 차이 없으면 자동으로 skip. 안전하게 list 에 포함.)

---

## Task 3: FE — `ContextPreviewResponse` 타입 + `ai.previewContext` 클라이언트

**Files:**
- Modify: `apps/desktop/src/lib/types.ts`
- Modify: `apps/desktop/src/lib/rpc.ts:123` 부근

### Step 1: wire 타입 추가

`apps/desktop/src/lib/types.ts` 의 다른 AI/RPC 타입들 옆에 추가:

```ts
// Wire shape from ai.preview_context RPC. Mirrors engine PreviewCounts JSON.
// Mapped to ContextCounts (camelCase) inside the rpc client.
export interface ContextPreviewResponse {
  nearby_scenes: number;
  same_chapter: number;
  other_chapter: number;
  other_part: number;
  has_synopsis: boolean;
  related_scenes: number;
  entities: number;
  active_threads: number;
  notes: number;
  project_meta_fields: number;
  has_style_notes: boolean;
}
```

### Step 2: rpc 클라이언트에 메서드 추가

`apps/desktop/src/lib/rpc.ts` 의 `export const ai = { ... }` 블록을 다음으로 교체 (`previewContext` 메서드 추가):

```ts
export const ai = {
  run: (nodeId: string, prompt: string, options: AIOptions, selectionText: string = "") =>
    rpcCall<{ run_id: string }>("ai.run", { node_id: nodeId, prompt, selection_text: selectionText, options }),
  cancel: (runId: string) => rpcCall<{ ok: true }>("ai.cancel", { run_id: runId }),
  previewContext: (nodeId: string): Promise<ContextCounts> =>
    rpcCall<ContextPreviewResponse>("ai.preview_context", { node_id: nodeId })
      .then((r) => ({
        nearbyScenes: r.nearby_scenes,
        sameChapter: r.same_chapter,
        otherChapter: r.other_chapter,
        otherPart: r.other_part,
        hasSynopsis: r.has_synopsis,
        relatedScenes: r.related_scenes,
        entities: r.entities,
        activeThreads: r.active_threads,
        notes: r.notes,
        projectMetaFields: r.project_meta_fields,
        hasStyleNotes: r.has_style_notes,
      })),
};
```

`ContextCounts` 와 `ContextPreviewResponse` 가 import 되어 있는지 확인 후 (이미 import 되어 있지 않으면) `rpc.ts` 상단 import 블록에 추가:

```ts
import type {
  // ... 기존 ...
  ContextPreviewResponse,
} from "./types";
import type { ContextCounts } from "../components/ai/AIContextChecklist";
```

(`ContextCounts` 는 `AIContextChecklist.tsx` 에서 export. 기존 코드베이스가 그 컴포넌트에서 import 하는 패턴을 따른다.)

### Step 3: 타입체크

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
```

기대: 에러 없음.

### Step 4: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/lib/types.ts apps/desktop/src/lib/rpc.ts
git commit -m "feat(rpc-client): ai.previewContext — snake/camel mapping"
```

---

## Task 4: FE — Workspace 통합 (하드코딩 제거 + fetch 흐름)

**Files:**
- Modify: `apps/desktop/src/routes/Workspace.tsx`

### Step 1: FALLBACK_COUNTS 모듈 상수 + state/ref 추가

`apps/desktop/src/routes/Workspace.tsx` 상단 (export 안쪽이 아닌, 파일 상단 — Workspace 함수 정의 위) 에 모듈 상수 추가:

```tsx
const FALLBACK_COUNTS: ContextCounts = {
  nearbyScenes: 0,
  sameChapter: 0,
  otherChapter: 0,
  otherPart: 0,
  hasSynopsis: false,
  relatedScenes: 0,
  entities: 0,
  activeThreads: 0,
  notes: 0,
  projectMetaFields: 0,
  hasStyleNotes: false,
};
```

(`ContextCounts` 타입은 이미 `AIContextChecklist` 에서 import 되어 있다. 아니면 추가.)

Workspace 함수 본문 안에 새 state 와 ref 추가 (기존 `aiPromptAnchor` state 옆):

```tsx
const [contextCounts, setContextCounts] = useState<ContextCounts | null>(null);
const previewReqIdRef = useRef(0);
```

(`useRef` 가 이미 import 되어 있다. `useState` 도.)

### Step 2: 하드코딩 `currentContextCounts` useMemo 제거

`Workspace.tsx` 안의 다음 블록 (대략 라인 380-400 부근, `useMemo` 로 감싸진 `currentContextCounts: ContextCounts = useMemo(() => {...}, [...])`) 을 **완전 제거**:

```tsx
// REMOVE:
const currentContextCounts: ContextCounts = useMemo(() => {
  const proj = load?.project;
  return {
    nearbyScenes: 3,
    sameChapter: 0,
    // ... 하드코딩 ...
  };
}, [load, mentioned]);
```

그리고 AIPromptBar/AIContextChecklist 가 props 로 받던 `currentContextCounts` 사용처를 `contextCounts ?? FALLBACK_COUNTS` 로 교체:

```tsx
{aiPromptAnchor && load && (
  <AIPromptBar
    // ... 기존 props ...
    contextItemCount={totalContextItems(contextCounts ?? FALLBACK_COUNTS)}
    // ...
  />
)}
{aiCtxChecklistOpen && aiPromptAnchor && (
  <AIContextChecklist
    anchor={{ top: aiPromptAnchor.top + 180, left: aiPromptAnchor.left }}
    counts={contextCounts ?? FALLBACK_COUNTS}
    onClose={() => setAiCtxChecklistOpen(false)}
  />
)}
```

### Step 3: Cmd+I 핸들러에서 preview fetch

기존 Cmd+I 키 핸들러를 찾아 (Workspace.tsx 의 전역 keydown handler 안 `e.key === "i"` 분기) anchor 설정 직후에 fetch 추가:

```tsx
if ((e.metaKey || e.ctrlKey) && e.key === "i") {
  e.preventDefault();
  if (aiPromptAnchor) {
    setAiPromptAnchor(null);
    return;
  }
  // ... 기존 anchor 계산 ...
  setAiPromptAnchor({ top: ..., left: ... });

  // Plan 19: fetch real context counts for the current node.
  if (load) {
    const reqId = ++previewReqIdRef.current;
    aiApi.previewContext(load.node.id)
      .then((counts) => {
        if (reqId !== previewReqIdRef.current) return;
        setContextCounts(counts);
      })
      .catch((err) => {
        if (reqId !== previewReqIdRef.current) return;
        showToast(`컨텍스트 정보를 가져오지 못했습니다: ${err}`);
      });
  }
}
```

(`aiApi` 는 `import { ai as aiApi } from "../lib/rpc";` — 기존에 있는지 확인 후 없으면 추가. `showToast` 는 ToastProvider 의 useToast hook — 이미 사용 중일 가능성. 없으면 추가: `const { showToast } = useToast();`)

### Step 4: prompt bar 닫힐 때 contextCounts 클리어

기존 AIPromptBar 의 `onClose` 핸들러를 찾아 `setContextCounts(null)` 와 reqId 증가를 추가:

```tsx
onClose={() => {
  ghost.drop();
  setAiPromptAnchor(null);
  setContextCounts(null);
  previewReqIdRef.current++;  // invalidate any in-flight preview response
}}
```

### Step 5: 타입체크 + 엔진 빌드 회귀 확인

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./...
```

기대: 타입체크 clean. Engine 테스트 모두 PASS.

### Step 6: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/routes/Workspace.tsx
git commit -m "feat(workspace): real context counts from ai.preview_context RPC"
```

---

## 통합 검증 (Task 4 직후 수동 스모크)

```bash
rm -rf /tmp/linetta-plan19 && LINETTA_HOME=/tmp/linetta-plan19 ./scripts/dev.sh
```

1. **장르 + 분량 + 시점** 모두 채운 작품 생성. 본문 입력.
2. `Cmd+I` → prompt bar 표시. ⓘ ctx 칩 클릭 → checklist popover. **작품 설정 (장르/분량/시점)** 항목이 `3/3` 표시되어야 한다.
3. 빈 작품 (헤딩 없는 작품) 만들어서 같은 시험 → 모든 항목 `—` 회색 + ctx 칩 `1개` 또는 `0개` 가까운 작은 값.
4. **빠른 close**: Cmd+I → 즉시 Esc → prompt bar 닫힘. 콘솔에 toast 안 떠야 함 (stale 응답이 차단됐다는 증거).
5. **에러 시뮬레이션**: engine 끄고 Cmd+I → 토스트 `컨텍스트 정보를 가져오지 못했습니다: ...`. AI 생성은 어차피 안 됨 (engine down).
6. **Plan 18 회귀**: Cmd+I → 자유 prompt 입력 → Enter → ghost streaming → 자동 commit + bar 닫기. 정상.

통과 시:

```bash
git tag plan-19-context-preview-done
```

---

## Self-Review

**1. Spec 커버리지:**

| Spec 요구 | 구현 task |
|---|---|
| `PreviewCounts` 타입 + JSON 태그 | Task 1 |
| `CountsFromContext` 순수 헬퍼 (공백 trim 포함) | Task 1 |
| `ai.preview_context` RPC 핸들러 | Task 2 |
| `previewContextParams.NodeID` + InvalidParams 가드 | Task 2 |
| main.go 등록 | Task 2 |
| FE `ContextPreviewResponse` wire 타입 | Task 3 |
| FE `ai.previewContext` 클라이언트 + snake→camel 매핑 | Task 3 |
| Workspace 하드코딩 useMemo 제거 | Task 4 |
| `contextCounts` state + `previewReqIdRef` stale 가드 | Task 4 |
| Cmd+I 시 fetch + 토스트 에러 처리 | Task 4 |
| `FALLBACK_COUNTS` 모듈 상수 | Task 4 |
| prompt bar 닫힐 때 clear + reqId 증가 | Task 4 |
| 수동 스모크 시나리오 6개 | Task 4 직후 |

모든 spec 요구 매핑.

**2. Placeholder scan:** "TBD" / "TODO" 없음. Task 2 의 setup boilerplate 는 implementer 가 기존 RunAI 테스트를 참조하라고 명시 (placeholder 아님 — context 제공).

**3. Type 일관성:**
- `PreviewCounts` Go 필드 (Task 1) ↔ `ContextPreviewResponse` TS 필드 (Task 3) 의 snake_case JSON tag 완전 일치.
- `ContextCounts` TS interface (`AIContextChecklist.tsx` 기존 — Plan 18) ↔ rpc client 매핑 결과 (Task 3) 의 camelCase 키 완전 일치.
- `FALLBACK_COUNTS` (Task 4) 의 필드 모두 `ContextCounts` 키와 일치 — 11개.
- `previewReqIdRef.current++` 패턴 일관: 새 요청 시작 시 증가, close 시 증가 (stale invalidate). 두 곳 모두 동일.

체크 완료.
