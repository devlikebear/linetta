# Plan 19 — AI Context Preview (실제 카운트) Design Spec

## 목적

`AIContextChecklist` 와 `AIPromptBar` 의 `ⓘ ctx: N개` 칩이 현재 `Workspace.tsx` 의 하드코딩 (`nearbyScenes: 3`, `hasSynopsis: true` 등)을 표시한다. 이 panel 의 목적인 **"AI에게 전달되는 컨텍스트를 정직하게 보여주는 것"** 과 어긋난다. Engine 의 실제 `ContextBuilder.Build` 결과에서 counts 만 추출해 FE 로 전달하는 RPC 를 신설한다.

## Goals

1. 새 RPC `ai.preview_context` — 현재 노드 기준 ContextBuilder 를 한 번 돌려 counts JSON 만 반환.
2. Workspace 가 Cmd+I 누를 때 RPC 호출 → 실제 counts 로 chip + popover 갱신.
3. 하드코딩 useMemo 제거.
4. RPC 실패해도 AI 생성 자체는 계속 동작 (Build 는 ai.run 안에서도 다시 호출됨).

## Non-Goals

- Selection 변경에 따른 동적 카운트. 선택 영역은 `Context.SelectionText` 로만 들어가며 hierarchical/entity 카운트에는 영향 없음.
- Caching / TTL. 매 Cmd+I 마다 새로 조회.
- 노드 전환 시 background preview (Plan 19 brainstorming 에서 사용자가 명시적으로 제외 — Cmd+I 시점만).
- preview 결과를 SQL 에 저장. 휘발성.

---

## 1. Engine 측

### 1.1 새 타입 — `PreviewCounts`

`engine/internal/ai/ai.go` 에 추가:

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

### 1.2 헬퍼 — `CountsFromContext`

`engine/internal/ai/context.go` (또는 새 `preview.go`) 에 함수 추가:

```go
// CountsFromContext extracts a PreviewCounts from a fully-built Context.
// Pure function — no I/O.
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

### 1.3 새 핸들러 — `PreviewContext`

`engine/internal/rpc/handlers/ai.go` 에 추가:

```go
type previewContextParams struct {
    NodeID string `json:"node_id"`
}

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

`Build` 는 stale 한 container summary 가 있으면 `refresher.RefreshNow` 를 동기 호출한다 — preview 요청도 그 비용을 부담한다. 이는 의도된 동작: preview 의 counts 가 정확하려면 실제 Build 와 같은 데이터 흐름을 거쳐야 한다. (정확하지 않은 빠른 path 가 honesty 의 핵심을 깬다.)

### 1.4 등록

`engine/cmd/linetta-engine/main.go` 의 `ai.run` 등록 옆에:

```go
s.Handle("ai.preview_context", handlers.PreviewContext(contextBuilder))
```

### 1.5 Engine 테스트

- `engine/internal/ai/context_test.go` — `CountsFromContext` 단위 테스트:
  - 모든 필드 채워진 Context → 정확한 counts
  - 빈 Context → 모두 0/false
  - 부분 Project 메타 (예: Genres 만) → projectMetaFields=1
- `engine/internal/rpc/handlers/ai_test.go` — `PreviewContext` 핸들러 E2E:
  - 프로젝트 + leaf 노드 fixture
  - 핸들러 호출 → JSON unmarshal → counts 확인
  - node_id 빈 문자열 → InvalidParams 에러

---

## 2. Frontend 측

### 2.1 타입

`apps/desktop/src/lib/types.ts` 에 추가:

```ts
// Wire shape from ai.preview_context RPC. Mirrors engine PreviewCounts JSON.
// Mapped to ContextCounts (which uses camelCase) inside the rpc client.
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

`ContextCounts` (이미 `AIContextChecklist.tsx` 에 export 됨, camelCase) 는 기존 타입 유지.

### 2.2 RPC 클라이언트

`apps/desktop/src/lib/rpc.ts` 의 `ai` 객체에 추가:

```ts
export const ai = {
  run: (nodeId, prompt, options, selectionText = "") => ...,
  cancel: (runId) => ...,
  previewContext: (nodeId: string) =>
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

snake_case → camelCase 변환을 rpc client 안에서 한 번에. 호출처는 `ContextCounts` 그대로.

### 2.3 Workspace 통합

`apps/desktop/src/routes/Workspace.tsx` 변경:

**제거:** 기존 `currentContextCounts` useMemo 블록 전체 (현재 `~Workspace.tsx:380-400` 부근의 하드코딩).

**추가:**

```tsx
const [contextCounts, setContextCounts] = useState<ContextCounts | null>(null);
const previewReqIdRef = useRef(0);

const FALLBACK_COUNTS: ContextCounts = {
  nearbyScenes: 0, sameChapter: 0, otherChapter: 0, otherPart: 0,
  hasSynopsis: false, relatedScenes: 0, entities: 0, activeThreads: 0,
  notes: 0, projectMetaFields: 0, hasStyleNotes: false,
};
```

(`FALLBACK_COUNTS` 는 컴포넌트 외부 상수로 — 매 render 재생성 방지.)

Cmd+I 핸들러 안에 preview fetch 추가 (anchor 설정 직후):

```tsx
if ((e.metaKey || e.ctrlKey) && e.key === "i") {
  e.preventDefault();
  if (aiPromptAnchor) {
    setAiPromptAnchor(null);
    return;
  }
  // ... existing anchor calc ...
  setAiPromptAnchor({ top: ..., left: ... });

  // Fetch real context counts for the current node.
  if (load) {
    const reqId = ++previewReqIdRef.current;
    aiApi.previewContext(load.node.id)
      .then((counts) => {
        // Drop stale responses (user closed bar, switched node, etc.).
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

prompt bar 가 닫힐 때 `contextCounts` 클리어:

```tsx
onClose={() => {
  ghost.drop();
  setAiPromptAnchor(null);
  setContextCounts(null);
  previewReqIdRef.current++;  // invalidate any in-flight preview response
}}
```

AIPromptBar / AIContextChecklist 호출:

```tsx
const counts = contextCounts ?? FALLBACK_COUNTS;
// ... in JSX ...
contextItemCount={totalContextItems(counts)}
// ...
counts={counts}
```

### 2.4 FE 검증

타입체크 + 수동 스모크. FE 테스트 인프라 부재는 기존 plan 들과 동일.

---

## 3. UX 디테일

- **로딩 중 칩 표시**: preview 도착 전엔 `ⓘ ctx: 0개` 로 표시되고 popover 의 모든 항목이 "—" (회색). RPC 가 빠르면 (~수십 ms) 거의 눈에 안 띈다. 느린 경우 짧은 0→실제값 깜박임. 첫 버전 수용.
- **에러 시 토스트**: `컨텍스트 정보를 가져오지 못했습니다: {error}` — 한 줄. AI 생성 자체는 시도 가능 (engine 이 Build 를 다시 함).
- **stale 응답 방지**: `previewReqIdRef` 카운터로 가장 최근 요청만 채택.
- **노드 전환 시**: prompt bar 가 열려있던 상태에서 사이드바에서 다른 씬 클릭하는 케이스 — 기존 Workspace 의 노드 전환 시 ghost 자동 drop 흐름이 이미 있으므로, 그 시점에 prompt bar 도 닫힘 / `contextCounts` 도 clear. 이미 `aiPromptAnchor` 가 노드 종속적이므로 자연 처리.

---

## 4. 위험 / 미해결 사항

- **`Build` 비용**: Plan 16 의 `loadHierarchicalContext` 는 `b.nodes.ListByProject` (모든 노드 로드) + container summarizer 동기 호출 가능. 큰 작품에선 수십~수백 ms 가능. 첫 버전에선 그대로 진행 — 실측 후 캐싱 도입 검토.
- **`refresher.RefreshNow` 가 LLM 을 호출하는 경우**: stale container 에 대해 LLM 요약을 새로 굽는다. preview 시점에도 토큰을 쓴다. 의도된 정직성 비용. 실제 ai.run 도 같은 경로를 거치므로 추가 비용은 아님 (한 번 굽고 캐시되면 다음 호출은 무료).
- **타입 mapping 일관성**: snake_case wire ↔ camelCase TS. rpc client 안 한 곳에서만 매핑. 새 필드 추가 시 rpc client 도 같이 갱신해야 함.

---

## 5. 파일 변경 요약

**Engine:**
- `engine/internal/ai/ai.go` — `+PreviewCounts` 타입
- `engine/internal/ai/context.go` — `+CountsFromContext` 헬퍼
- `engine/internal/ai/context_test.go` — `+TestCountsFromContext_*`
- `engine/internal/rpc/handlers/ai.go` — `+PreviewContext` 핸들러 + `+previewContextParams`
- `engine/internal/rpc/handlers/ai_test.go` — `+TestPreviewContext_*`
- `engine/cmd/linetta-engine/main.go` — `+s.Handle("ai.preview_context", ...)`

**Frontend:**
- `apps/desktop/src/lib/types.ts` — `+ContextPreviewResponse`
- `apps/desktop/src/lib/rpc.ts` — `+ai.previewContext`
- `apps/desktop/src/routes/Workspace.tsx` — 하드코딩 useMemo 제거, state/ref/fetch/clear 추가, FALLBACK_COUNTS 상수
