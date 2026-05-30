# Companion Phase 3 — Memory Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`).

**Goal:** 컴패니언에 키워드 장기 메모리를 더한다 — 제안 기반 쓰기(`remember` op → `companion.remember` → tars `AppendExperience`)와 매 턴 회상 주입(`SearchExperiences` → `## 기억`).

**Architecture:** tars `pkg/memory` 키워드 프리미티브(임베딩 없음). engine companion 패키지에 memory 헬퍼 + proposal `remember` op + 컨텍스트 회상 주입 + `companion.remember` RPC; FE는 op 적용 경로만 확장. 프로젝트별 메모리 루트 `<home>/companion/mem/{projectID}`.

**Tech Stack:** Go 1.26 engine + tars `pkg/memory` v0.33.0, React/TS FE.

---

## 사전 지식 (구현자 필독)

- 루트 `/Users/changheonshin/workspace/myworks/linetta`. engine `engine/`(테스트 `cd engine && go test ./...`), FE `apps/desktop`(`npx tsc --noEmit`). 빌드 `bash scripts/build-engine.sh`. `main` 브랜치, no --no-verify, no push. LSP stale 무시.
- tars `pkg/memory`(이미 의존, v0.33.0): `AppendExperience(root string, exp memory.Experience) error`(EnsureWorkspace 자동, Summary 필수, Category 빈→"fact", Timestamp 영→now); `SearchExperiences(root string, opts memory.SearchOptions) ([]memory.Experience, error)`(대소문자 무시 substring on Summary/Tags/SourceSession, Category 정확일치, 최근순, 파일 없으면 `[]`); `Experience{Timestamp time.Time; Category,Summary string; Tags []string; SourceSession string; Importance int; Auto bool}`; `SearchOptions{Query,Category string; Limit int}`. import: `"github.com/devlikebear/tars/pkg/memory"`. 키워드 경로는 stdlib만(임베더/네트워크 없음). 파일 `<root>/memory/experiences.jsonl`.
- 현재 companion 패키지(Phase 1):
  - `companion.go`: `Service` 구조체(필드 projects/threads/entities/relationships/plot/notify/factory/src/workDir/sessions/runner), `NewService(sessionsDir string, projects, threads, entities, relationships, plotBuilder, notify, factory, src, workDir)`, `gatherContext(ctx, projectID, nodeID string) (PromptData, error)`, `History`, `Send`, `Cancel`.
  - `prompt.go`: `PromptData{Outline string; Spine plot.Spine; HasSpine bool; Threads []thread.Thread; Entities []entity.Entity; Relationships []relationship.Relationship}`; `buildSystem() string`; `buildContext(d PromptData) string`.
  - `proposal.go`: `Op` 구조체(Type, Ref, Name, Color, Summary, ThreadID, ThreadRef, NodeID, BeatID, Label, Description, Intensity, Outline), `knownOps map[string]bool`, `ParseProposal`, `validateProposal`.
  - `runner.go`: `start`에서 `data, err := r.svc.gatherContext(ctx, projectID, nodeID)` 호출.
  - 핸들러 `engine/internal/rpc/handlers/companion.go`: `CompanionSend/History/Cancel`. main.go에 `companionSvc` + `s.Handle("companion.*")`.
- FE(Phase 2): `ProposalOp`/`ProposalOpType`(types.ts), `companion` rpc 네임스페이스(rpc.ts), `applyProposal`(applyProposal.ts), `ProposalCard.opLabel`.

## File Structure
- Create: `engine/internal/companion/memory.go` (+ `memory_test.go`)
- Modify: `engine/internal/companion/companion.go` (memBase 필드, gatherContext query + Memories)
- Modify: `engine/internal/companion/prompt.go` (PromptData.Memories, ## 기억, buildSystem)
- Modify: `engine/internal/companion/proposal.go` (remember op) (+ proposal_test.go 케이스)
- Modify: `engine/internal/companion/runner.go` (gatherContext에 text 전달)
- Modify: `engine/internal/rpc/handlers/companion.go` (CompanionRemember) (+ test)
- Modify: `engine/cmd/linetta-engine/main.go` (companion.remember 등록)
- Modify: FE `types.ts`, `rpc.ts`, `applyProposal.ts`, `components/companion/ProposalCard.tsx`

---

## Task 1: proposal `remember` op

**Files:** `engine/internal/companion/proposal.go`, `engine/internal/companion/proposal_test.go`

- [ ] **Step 1: Op 필드 + knownOps + 검증 추가**

`proposal.go`:
- `Op` 구조체에 필드 추가(set_outline 블록 근처):
```go
	// remember
	Text     string `json:"text,omitempty"`
	Category string `json:"category,omitempty"`
```
- `knownOps`에 추가: `"remember": true,`
- `validateProposal`의 switch에 케이스 추가:
```go
		case "remember":
			if strings.TrimSpace(op.Text) == "" {
				return fmt.Errorf("op[%d] remember: text required", i)
			}
```

- [ ] **Step 2: 테스트 추가**

`proposal_test.go`에 추가:
```go
func TestParseProposal_Remember(t *testing.T) {
	p, present, err := ParseProposal(block(`{"ops":[{"op":"remember","text":"작가는 단문을 선호","category":"preference"}]}`))
	if !present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if len(p.Ops) != 1 || p.Ops[0].Text != "작가는 단문을 선호" {
		t.Fatalf("p=%+v", p)
	}
}

func TestParseProposal_RememberRequiresText(t *testing.T) {
	_, present, err := ParseProposal(block(`{"ops":[{"op":"remember","category":"x"}]}`))
	if !present || err == nil {
		t.Fatalf("expected text-required error, present=%v err=%v", present, err)
	}
}
```

- [ ] **Step 3: 실행 + 커밋**

Run: `cd engine && go test ./internal/companion/...`
Expected: PASS (기존 + 2 신규).
```bash
git add engine/internal/companion/proposal.go engine/internal/companion/proposal_test.go
git commit -m "feat(companion): add remember op to proposal schema"
```

---

## Task 2: memory 헬퍼 (Remember/Recall) + Service.memBase

**Files:** Create `engine/internal/companion/memory.go`, `engine/internal/companion/memory_test.go`; Modify `engine/internal/companion/companion.go`

- [ ] **Step 1: `memory.go` 작성**

```go
package companion

import (
	"context"
	"path/filepath"

	"github.com/devlikebear/tars/pkg/memory"
)

// recallLimit caps how many remembered facts are injected per turn.
const recallLimit = 5

// memRoot returns the per-project memory workspace root.
func memRoot(memBase, projectID string) string {
	return filepath.Join(memBase, projectID)
}

// Remember persists a fact to the project's keyword memory (experiences.jsonl).
func (s *Service) Remember(projectID, text, category string) error {
	return memory.AppendExperience(memRoot(s.memBase, projectID), memory.Experience{
		Summary:  text,
		Category: category,
	})
}

// Recall returns up to recallLimit remembered fact summaries matching query
// (case-insensitive substring). Best-effort: returns nil on error/empty.
func (s *Service) Recall(projectID, query string, limit int) []string {
	hits, err := memory.SearchExperiences(memRoot(s.memBase, projectID), memory.SearchOptions{
		Query: query, Limit: limit,
	})
	if err != nil || len(hits) == 0 {
		return nil
	}
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		if h.Summary != "" {
			out = append(out, h.Summary)
		}
	}
	return out
}

var _ = context.Background // (remove if context unused; AppendExperience/SearchExperiences are root-based, no ctx)
```
주의: `memory.AppendExperience`/`SearchExperiences`는 ctx를 받지 않는다(root 기반). 위 `var _ = context.Background` 줄과 `context` import는 불필요하면 제거하라(빌드 에러 메시지대로). 즉 import에서 `context` 빼고 그 줄도 삭제하는 게 정상.

- [ ] **Step 2: `companion.go` — memBase 필드 + 생성자**

- `Service` 구조체에 `memBase string` 추가.
- `NewService`에서 설정: `sessions: session.NewStore(sessionsDir)` 근처에 `memBase: filepath.Join(sessionsDir, "mem"),` 추가. (`path/filepath`가 companion.go에 import 안 돼 있으면 추가.)

- [ ] **Step 3: `memory_test.go`**

```go
package companion

import (
	"testing"
)

func TestRememberAndRecall(t *testing.T) {
	svc := &Service{memBase: t.TempDir()}
	if err := svc.Remember("p1", "작가는 단문을 선호한다", "preference"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Remember("p1", "세계관: 마법은 금지되어 있다", "lore"); err != nil {
		t.Fatal(err)
	}
	hits := svc.Recall("p1", "단문", recallLimit)
	if len(hits) != 1 || hits[0] != "작가는 단문을 선호한다" {
		t.Fatalf("recall(단문) = %+v", hits)
	}
	// unrelated query → empty
	if got := svc.Recall("p1", "존재하지않는키워드", recallLimit); len(got) != 0 {
		t.Fatalf("expected no hits, got %+v", got)
	}
	// project isolation
	if got := svc.Recall("p2", "단문", recallLimit); len(got) != 0 {
		t.Fatalf("p2 should have no memory, got %+v", got)
	}
}

func TestRememberRequiresText(t *testing.T) {
	svc := &Service{memBase: t.TempDir()}
	if err := svc.Remember("p1", "", "x"); err == nil {
		t.Fatal("expected error for empty text")
	}
}
```

- [ ] **Step 4: 실행 + 커밋**

Run: `cd engine && go build ./... && go test ./internal/companion/...`
Expected: PASS.
```bash
git add engine/internal/companion/memory.go engine/internal/companion/memory_test.go engine/internal/companion/companion.go
git commit -m "feat(companion): keyword memory helpers (Remember/Recall) + memBase"
```

---

## Task 3: 회상 주입 (gatherContext query + ## 기억 + buildSystem)

**Files:** `engine/internal/companion/companion.go`, `prompt.go`, `runner.go`; tests in `prompt_test.go`

- [ ] **Step 1: gatherContext에 query 추가 + Memories 채우기**

`companion.go` `gatherContext` 시그니처를 `(ctx context.Context, projectID, nodeID, query string) (PromptData, error)`로 변경. best-effort 로드 블록 끝에 추가:
```go
	d.Memories = s.Recall(projectID, query, recallLimit)
```

- [ ] **Step 2: runner에서 text 전달**

`runner.go` `start`의 `gatherContext` 호출을:
```go
	data, err := r.svc.gatherContext(ctx, projectID, nodeID, text)
```

- [ ] **Step 3: PromptData.Memories + buildContext ## 기억 + buildSystem 안내**

`prompt.go`:
- `PromptData`에 `Memories []string` 추가.
- `buildContext`에 (개요 섹션 앞 또는 뒤, 적절한 위치 — 권장: 개요 다음) `## 기억` 섹션:
```go
	if len(d.Memories) > 0 {
		b.WriteString("## 기억\n")
		for _, m := range d.Memories {
			b.WriteString("- " + m + "\n")
		}
		b.WriteString("\n")
	}
```
- `buildSystem`에 remember 안내 추가(op 목록 줄에 `remember{text,category?}`를 더하고, 다음 문장 추가):
```go
	b.WriteString("이전 대화에서 알게 된 작품 설정·작가 취향은 아래 '기억'에 주어집니다. 기억할 가치가 있는 새 사실(작가 취향, 세계관 규칙 등)은 remember op로 제안하세요(작가가 승인하면 저장됩니다).\n")
```
(op 종류 나열 문자열에 `, remember{text,category?}` 추가.)

- [ ] **Step 4: 테스트**

`prompt_test.go`에 추가:
```go
func TestBuildContext_RendersMemories(t *testing.T) {
	out := buildContext(PromptData{Memories: []string{"작가는 단문을 선호"}})
	if !strings.Contains(out, "## 기억") || !strings.Contains(out, "작가는 단문을 선호") {
		t.Fatalf("memories not rendered:\n%s", out)
	}
}

func TestBuildSystem_MentionsRemember(t *testing.T) {
	s := buildSystem()
	if !strings.Contains(s, "remember") || !strings.Contains(s, "기억") {
		t.Fatal("buildSystem missing remember/memory guidance")
	}
}
```
또한 companion service 레벨 테스트(선택, 강력 권장): Remember 후 gatherContext(query)가 Memories를 채우는지 — 기존 companion_test.go의 `newSvc` 패턴 사용(단 newSvc는 NewService로 memBase가 t.TempDir()/mem로 설정됨). 추가:
```go
func TestGatherContext_InjectsMemory(t *testing.T) {
	svc, _, projectID := newSvc(t, "안녕")
	if err := svc.Remember(projectID, "작가는 반전을 좋아한다", "preference"); err != nil {
		t.Fatal(err)
	}
	d, err := svc.gatherContext(context.Background(), projectID, "", "반전")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range d.Memories {
		if m == "작가는 반전을 좋아한다" { found = true }
	}
	if !found {
		t.Fatalf("memory not recalled: %+v", d.Memories)
	}
}
```
(`newSvc` 시그니처/반환 확인 후 맞출 것. companion_test.go가 `import "context"` 안 했으면 추가.)

- [ ] **Step 5: 빌드/테스트 + 커밋**

Run: `cd engine && go build ./... && go test ./internal/companion/...`
Expected: PASS.
```bash
git add engine/internal/companion/companion.go engine/internal/companion/prompt.go engine/internal/companion/runner.go engine/internal/companion/prompt_test.go engine/internal/companion/companion_test.go
git commit -m "feat(companion): recall memory into context + remember guidance in system prompt"
```

---

## Task 4: companion.remember RPC + main 배선

**Files:** `engine/internal/rpc/handlers/companion.go`, `engine/cmd/linetta-engine/main.go`; test in `companion_test.go`(handlers)

- [ ] **Step 1: 핸들러 추가**

`handlers/companion.go`에 추가:
```go
type companionRememberParams struct {
	ProjectID string `json:"project_id"`
	Text      string `json:"text"`
	Category  string `json:"category"`
}

// CompanionRemember returns a handler for companion.remember.
func CompanionRemember(svc *companion.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p companionRememberParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" || p.Text == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id and text required"}
		}
		if err := svc.Remember(p.ProjectID, p.Text, p.Category); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
```

- [ ] **Step 2: main.go 등록**

`s.Handle("companion.cancel", ...)` 다음 줄에:
```go
	s.Handle("companion.remember", handlers.CompanionRemember(companionSvc))
```

- [ ] **Step 3: 핸들러 테스트(가드)**

`handlers/companion_test.go`에 추가(기존 가드 패턴):
```go
func TestCompanionRememberInvalidParams(t *testing.T) {
	h := CompanionRemember(nil)
	_, err := h(context.Background(), json.RawMessage(`{"project_id":"p"}`)) // text 누락
	var me *rpc.MethodError
	if !errors.As(err, &me) || me.Code != rpc.CodeInvalidParams {
		t.Fatalf("expected InvalidParams, got %v", err)
	}
}
```
(기존 companion_test.go의 import/패턴에 맞출 것.)

- [ ] **Step 4: 빌드/테스트 + 커밋**

Run: `cd engine && go build ./... && go test ./...`
Expected: 전 패키지 PASS.
```bash
git add engine/internal/rpc/handlers/companion.go engine/internal/rpc/handlers/companion_test.go engine/cmd/linetta-engine/main.go
git commit -m "feat(engine): companion.remember RPC + wiring"
```

---

## Task 5: FE — remember op 적용

**Files:** `apps/desktop/src/lib/types.ts`, `rpc.ts`, `applyProposal.ts`, `components/companion/ProposalCard.tsx`

- [ ] **Step 1: types.ts**

- `ProposalOpType` 유니온에 `| "remember"` 추가.
- `ProposalOp`에 필드 추가: `text?: string;` `category?: string;`

- [ ] **Step 2: rpc.ts — companion.remember**

`companion` 네임스페이스에 추가:
```ts
  remember: (projectId: string, text: string, category?: string) =>
    rpcCall<{ ok: true }>("companion.remember", { project_id: projectId, text, category }),
```

- [ ] **Step 3: applyProposal.ts — remember 케이스**

switch에 추가:
```ts
        case "remember": {
          if (!op.text || !op.text.trim()) throw new Error("기억할 내용 없음");
          await companionApi.remember(projectId, op.text, op.category);
          break;
        }
```
import에 `companion as companionApi` 추가: `import { threads as threadsApi, beats as beatsApi, projects as projectsApi, companion as companionApi } from "./rpc";`

- [ ] **Step 4: ProposalCard.tsx — opLabel**

`opLabel` switch에 추가:
```tsx
    case "remember": return `기억하기: ${op.text ?? ""}`;
```

- [ ] **Step 5: tsc + 커밋**

Run: `cd apps/desktop && npx tsc --noEmit`
Expected: 클린.
```bash
git add apps/desktop/src/lib/types.ts apps/desktop/src/lib/rpc.ts apps/desktop/src/lib/applyProposal.ts apps/desktop/src/components/companion/ProposalCard.tsx
git commit -m "feat(desktop): apply remember proposal op via companion.remember"
```

---

## Task 6: 최종 검증

- [ ] `cd engine && go test ./...` 전 패키지 PASS
- [ ] `cd apps/desktop && npx tsc --noEmit` 클린
- [ ] repo root `bash scripts/build-engine.sh` → ok
- [ ] 수동 스모크(사용자): 컴패니언에 "내가 단문을 선호한다는 걸 기억해줘" → remember 제안 카드 → 적용 → 새 대화에서 관련 질문 시 `## 기억`로 반영(컨텍스트 체크 불가하지만 답변 일관성으로 체감)

## 범위 밖
- 시맨틱(임베딩) 회상, 자동 기억 추출, 메모리 편집/삭제 UI, MEMORY.md 노트 — 후속.
