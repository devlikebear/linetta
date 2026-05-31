# Companion Phase 6 — On-Demand Read Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`).

**Goal:** 컴패니언이 `linetta-query` 블록으로 읽기 도구를 능동 호출하면, 엔진이 실행해 결과를 다음 라운드에 주입하는 제공자-무관 루프(최대 3라운드)를 추가한다.

**Architecture:** runner를 라운드 루프로 리팩터. 각 라운드 Chat → full text에 `linetta-query` 있으면 reset("")+thinking 후 조회 실행·결과 주입·재프롬프트; 없으면(또는 라운드 상한) 최종 답+제안. 읽기 도구는 기존 repo 조회(비파괴). 스트리밍/제안/취소/세션 영속은 기존 메커니즘 재사용.

**Tech Stack:** Go engine, React/TS FE, Tauri 브리지(1줄).

---

## 사전 지식 (구현자 필독)

- 루트 `/Users/changheonshin/workspace/myworks/linetta`. engine `engine/`, FE `apps/desktop`, 빌드 `bash scripts/build-engine.sh`(repo root), 브리지 `apps/desktop/src-tauri`(`cargo check`). `main`, no --no-verify, no push. LSP stale 무시, 명령 출력만 신뢰.
- `companion/runner.go` 현재: `run(ctx, runID, path, msgs, client, now)` — 단일 Chat → dedup 스트리밍(companion.delta/reset) → full = resp.Message.Content||dedup.Final() → AppendMessage(assistant) → ParseProposal→companion.proposal → companion.done. 취소는 ctx.Err()→companion.cancelled. 페이로드 구조체(deltaPayload/resetPayload/donePayload/errorPayload/cancelledPayload/proposalPayload) 정의돼 있음.
- `companion/companion.go` `Service`: 필드 sessions/projects/threads/entities/relationships/plot/notify/factory/src/workDir/memBase/runner. `NewService(sessionsDir, projects, threads, entities, relationships, plotBuilder, notify, factory, src, workDir)`. **`nodes`/`beats` repo 직접 필드 없음**(plot.Builder가 내부에 가짐) → 읽기 도구용으로 추가 필요.
- `companion/proposal.go`: `extractFencedBlocks(s, lang string) []string` 헬퍼(linetta-proposal 추출에 사용). 재사용 가능.
- repos: `entity.Repo.Search(ctx, projectID, query string, limit int) ([]entity.Entity,error)`; `node.Repo.Get(ctx, id) (node.Node,error)`(Node.ContentDoc *string, Label, ParentID), `node.Repo.ListByProject(ctx, projectID) ([]node.Node,error)`(kind "leaf"/"container"); `beat.Repo.ListByNode(ctx, nodeID) ([]beat.Beat,error)`, `beat.Repo.ListByThread(ctx, threadID) ([]beat.Beat,error)`; `thread.Repo.Get`; `memory.SearchExperiences(root, memory.SearchOptions{Query,Limit})`. main.go에 `nodes := node.NewRepo(st)`, `beats := beat.NewRepo(st)` 이미 있음.
- FE: `useEngineEvent`, `companion-*` 이벤트(브리지 allowlist는 `apps/desktop/src-tauri/src/engine.rs` match). `useCompanion`(streaming/messages/status). `CompanionPanel.streamProse(text)`가 `text.indexOf("```linetta-proposal")`로 cut.

## File Structure
- Create: `engine/internal/companion/query.go` (+ query_test.go)
- Modify: `engine/internal/companion/companion.go` (nodes/beats 필드 + NewService)
- Modify: `engine/internal/companion/runner.go` (루프 + thinking 이벤트) (+ runner_test 보강)
- Modify: `engine/cmd/linetta-engine/main.go` (NewService 인자)
- Modify: `apps/desktop/src-tauri/src/engine.rs` (companion.thinking allowlist)
- Modify: FE `lib/types.ts`, `hooks/useCompanion.ts`, `components/companion/CompanionPanel.tsx`

---

## Task 1: query.go — 쿼리 파서 + 읽기 도구 실행기

**Files:** Create `engine/internal/companion/query.go`, `query_test.go`; Modify `companion.go` (nodes/beats 필드).

- [ ] **Step 1: companion.go — nodes/beats repo 필드 추가**

`Service` 구조체에 `nodes *node.Repo` `beats *beat.Repo` 추가. `NewService` 시그니처에 두 인자 추가(파라미터 순서: projectBuilder 인자들 사이 자연스러운 위치 — 끝에 추가가 가장 단순):
```go
func NewService(
	sessionsDir string,
	projects *project.Repo, threads *thread.Repo, entities *entity.Repo,
	relationships *relationship.Repo, plotBuilder *plot.Builder,
	notify rpc.Notifier, factory ClientFactory, src ProviderSource, workDir string,
	nodes *node.Repo, beats *beat.Repo,
) *Service {
	s := &Service{
		sessions: session.NewStore(sessionsDir),
		projects: projects, threads: threads, entities: entities,
		relationships: relationships, plot: plotBuilder,
		notify: notify, factory: factory, src: src, workDir: workDir,
		memBase: filepath.Join(sessionsDir, "mem"),
		nodes:   nodes, beats: beats,
	}
	s.runner = newRunner(s)
	return s
}
```
import에 `node`, `beat` 패키지 추가.

- [ ] **Step 2: query.go 작성**

```go
package companion

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/tars/pkg/memory"
)

const queryFence = "linetta-query"
const sceneTextMaxRunes = 1200

// Query is one read request. Args holds string-valued params.
type Query struct {
	Tool string            `json:"tool"`
	Args map[string]string `json:"args"`
}

// QueryRequest is the parsed contents of a linetta-query block.
type QueryRequest struct {
	Queries []Query `json:"queries"`
}

// ParseQuery extracts a linetta-query block from model output.
// (Query{},false,nil) no block; (parsed,true,nil) ok; (zero,true,err) malformed.
func ParseQuery(full string) (QueryRequest, bool, error) {
	blocks := extractFencedBlocks(full, queryFence)
	if len(blocks) == 0 {
		return QueryRequest{}, false, nil
	}
	var qr QueryRequest
	if err := json.Unmarshal([]byte(strings.TrimSpace(blocks[0])), &qr); err != nil {
		return QueryRequest{}, true, fmt.Errorf("invalid query JSON: %w", err)
	}
	if len(qr.Queries) == 0 {
		return QueryRequest{}, true, fmt.Errorf("query block has no queries")
	}
	return qr, true, nil
}

// runQueries executes each read tool and returns a human-readable result block
// to inject back into the conversation. Unknown tools / bad args become error lines.
func (s *Service) runQueries(ctx context.Context, projectID string, qs []Query) string {
	var b strings.Builder
	b.WriteString("## 조회 결과\n")
	for _, q := range qs {
		b.WriteString("### " + q.Tool + "\n")
		b.WriteString(s.runOneQuery(ctx, projectID, q))
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func (s *Service) runOneQuery(ctx context.Context, projectID string, q Query) string {
	switch q.Tool {
	case "search_entities":
		ents, err := s.entities.Search(ctx, projectID, q.Args["query"], 20)
		if err != nil {
			return "(오류: " + err.Error() + ")"
		}
		if len(ents) == 0 {
			return "(결과 없음)"
		}
		var sb strings.Builder
		for _, e := range ents {
			sb.WriteString(fmt.Sprintf("- [%s] (%s) %s", e.ID, kindLabel(e.Kind), e.Name))
			if e.Role != "" {
				sb.WriteString(" / " + e.Role)
			}
			if e.Summary != "" {
				sb.WriteString(": " + e.Summary)
			}
			sb.WriteString("\n")
		}
		return strings.TrimRight(sb.String(), "\n")
	case "get_scene_text":
		id := q.Args["node_id"]
		if id == "" {
			return "(오류: node_id 필요)"
		}
		n, err := s.nodes.Get(ctx, id)
		if err != nil {
			return "(오류: " + err.Error() + ")"
		}
		return trimRunesLocal(plainTextFromDoc(n.ContentDoc), sceneTextMaxRunes)
	case "list_scenes":
		all, err := s.nodes.ListByProject(ctx, projectID)
		if err != nil {
			return "(오류: " + err.Error() + ")"
		}
		byID := map[string]node.Node{}
		for _, n := range all {
			byID[n.ID] = n
		}
		var sb strings.Builder
		for _, n := range all {
			if n.Kind != "leaf" {
				continue
			}
			sb.WriteString("- [" + n.ID + "] " + breadcrumb(byID, n) + "\n")
		}
		if sb.Len() == 0 {
			return "(씬 없음)"
		}
		return strings.TrimRight(sb.String(), "\n")
	case "list_beats":
		var bs []beat.Beat
		var err error
		if nid := q.Args["node_id"]; nid != "" {
			bs, err = s.beats.ListByNode(ctx, nid)
		} else if tid := q.Args["thread_id"]; tid != "" {
			bs, err = s.beats.ListByThread(ctx, tid)
		} else {
			return "(오류: node_id 또는 thread_id 필요)"
		}
		if err != nil {
			return "(오류: " + err.Error() + ")"
		}
		if len(bs) == 0 {
			return "(비트 없음)"
		}
		var sb strings.Builder
		for _, bt := range bs {
			sb.WriteString(fmt.Sprintf("- [%s] #%d %s", bt.ID, bt.Ordinal, bt.Label))
			if bt.Description != "" {
				sb.WriteString(" — " + bt.Description)
			}
			sb.WriteString("\n")
		}
		return strings.TrimRight(sb.String(), "\n")
	case "recall_memory":
		hits := s.Recall(projectID, q.Args["query"], recallLimit)
		if len(hits) == 0 {
			return "(기억 없음)"
		}
		return "- " + strings.Join(hits, "\n- ")
	default:
		return "(오류: 알 수 없는 도구 " + q.Tool + ")"
	}
}

// breadcrumb renders ancestor labels joined " / ".
func breadcrumb(byID map[string]node.Node, n node.Node) string {
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

// plainTextFromDoc extracts text content from a Tiptap doc JSON (minimal).
func plainTextFromDoc(raw *string) string {
	if raw == nil || *raw == "" {
		return ""
	}
	var v interface{}
	if err := json.Unmarshal([]byte(*raw), &v); err != nil {
		return ""
	}
	var sb strings.Builder
	var walk func(x interface{})
	walk = func(x interface{}) {
		switch t := x.(type) {
		case map[string]interface{}:
			if t["type"] == "text" {
				if s, ok := t["text"].(string); ok {
					sb.WriteString(s)
				}
			}
			if c, ok := t["content"].([]interface{}); ok {
				for _, ch := range c {
					walk(ch)
				}
			}
			if k, _ := t["type"].(string); k == "paragraph" || k == "heading" {
				sb.WriteString("\n")
			}
		case []interface{}:
			for _, ch := range t {
				walk(ch)
			}
		}
	}
	walk(v)
	return strings.TrimSpace(sb.String())
}

func trimRunesLocal(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

var _ = memory.SearchExperiences // recall_memory uses s.Recall which wraps it
```
NOTE: the `var _ = memory.SearchExperiences` line is only to avoid an unused-import if `memory` ends up unreferenced — actually `recall_memory` uses `s.Recall` (memory.go), NOT memory directly, so REMOVE the `memory` import and that `var _` line. Keep imports to what's used: context, encoding/json, fmt, strings, beat, node. (`kindLabel` is already defined in prompt.go — reuse it; do NOT redefine.)

- [ ] **Step 3: query_test.go**

```go
package companion

import (
	"context"
	"strings"
	"testing"
)

func TestParseQuery(t *testing.T) {
	full := "잠깐 찾아볼게요.\n```linetta-query\n{\"queries\":[{\"tool\":\"search_entities\",\"args\":{\"query\":\"하나\"}}]}\n```"
	qr, present, err := ParseQuery(full)
	if !present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if len(qr.Queries) != 1 || qr.Queries[0].Tool != "search_entities" || qr.Queries[0].Args["query"] != "하나" {
		t.Fatalf("qr=%+v", qr)
	}
}

func TestParseQuery_None(t *testing.T) {
	if _, present, err := ParseQuery("그냥 대화"); present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
}

func TestParseQuery_Malformed(t *testing.T) {
	if _, present, err := ParseQuery("```linetta-query\n{bad}\n```"); !present || err == nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
}

func TestRunQueries_UnknownToolAndSearch(t *testing.T) {
	svc := newSvcForQuery(t) // build a Service with real repos + a seeded entity (see note)
	out := svc.runQueries(context.Background(), svc.seedProjectID, []Query{
		{Tool: "search_entities", Args: map[string]string{"query": "하나"}},
		{Tool: "bogus_tool", Args: nil},
	})
	if !strings.Contains(out, "search_entities") || !strings.Contains(out, "하나") {
		t.Fatalf("search result missing:\n%s", out)
	}
	if !strings.Contains(out, "알 수 없는 도구") {
		t.Fatalf("unknown-tool error missing:\n%s", out)
	}
}
```
NOTE for implementer: `newSvcForQuery` — reuse companion_test.go's `newSvc` pattern (it builds a Service via NewService with a temp store + repos) but you now need NewService to receive nodes/beats too (Step 1). Adapt the existing `newSvc` test helper to pass `nodes`/`beats`, and seed an entity ("하나") via `entity.Repo.Create` (check entity repo Create signature). If wiring a full Service in the test is heavy, alternatively test `runOneQuery` for the search + unknown-tool cases against a Service built with the same temp-store helper. Keep the assertions (search returns the entity; unknown tool → error line).

- [ ] **Step 4: build/test + commit**

Run: `cd engine && go build ./... && go test ./internal/companion/...`
Expected: PASS. (main.go will break until Task wiring — but Task 1 only changes companion.go's NewService signature, which main.go calls; so ALSO update main.go's NewService call in this task to add `nodes, beats` args to keep the build green. See Task 4 Step for the exact main.go edit; do that arg addition now.)
```bash
git add engine/internal/companion/query.go engine/internal/companion/query_test.go engine/internal/companion/companion.go engine/internal/companion/companion_test.go engine/cmd/linetta-engine/main.go
git commit -m "feat(companion): read-query parser + read-tool executor + nodes/beats repos"
```

NOTE: main.go edit needed in THIS task (to keep build green): in the `companion.NewService(...)` call, append `, nodes, beats` as the last two args (both already constructed in main.go).

---

## Task 2: runner — 라운드 루프 + thinking 이벤트

**Files:** `engine/internal/companion/runner.go` (+ runner/companion_test.go 보강)

- [ ] **Step 1: thinking 페이로드 + run에 projectID 전달**

- `runner.go`에 페이로드 추가:
```go
type thinkingPayload struct {
	RunID string `json:"run_id"`
	Text  string `json:"text"`
}
```
- `start`에서 `run` 호출에 projectID 전달: `go r.run(runCtx, runID, projectID, path, msgs, client, now)`. `run` 시그니처에 `projectID string` 추가(runID 다음).

- [ ] **Step 2: run을 루프로 교체**

`run` 본문을 다음으로 교체:
```go
const maxQueryRounds = 3

func (r *Runner) run(ctx context.Context, runID, projectID, path string, msgs []llm.ChatMessage, client llm.Client, now func() int64) {
	defer r.finish(runID)

	for round := 0; round < maxQueryRounds; round++ {
		dedup := streamdedup.New()
		resp, err := client.Chat(ctx, msgs, llm.ChatOptions{
			OnDelta: func(text string) {
				switch act, payload := dedup.Observe(text); act {
				case streamdedup.ActionEmit:
					_ = r.svc.notify.Notify("companion.delta", deltaPayload{RunID: runID, Text: payload})
				case streamdedup.ActionReset:
					_ = r.svc.notify.Notify("companion.reset", resetPayload{RunID: runID, Text: payload})
				case streamdedup.ActionSkip:
				}
			},
		})
		if ctx.Err() != nil {
			_ = r.svc.notify.Notify("companion.cancelled", cancelledPayload{RunID: runID})
			return
		}
		if err != nil {
			_ = r.svc.notify.Notify("companion.error", errorPayload{RunID: runID, Message: err.Error()})
			return
		}
		full := resp.Message.Content
		if full == "" {
			full = dedup.Final()
		}

		// If not the last allowed round, check for a read-query and loop.
		if round < maxQueryRounds-1 {
			if qr, present, qerr := ParseQuery(full); present && qerr == nil {
				// This round was a query, not the final answer: clear partial prose,
				// surface a thinking status, run reads, feed results, continue.
				_ = r.svc.notify.Notify("companion.reset", resetPayload{RunID: runID, Text: ""})
				_ = r.svc.notify.Notify("companion.thinking", thinkingPayload{RunID: runID, Text: querySummary(qr)})
				result := r.svc.runQueries(ctx, projectID, qr.Queries)
				msgs = append(msgs,
					llm.ChatMessage{Role: "assistant", Content: full},
					llm.ChatMessage{Role: "user", Content: result},
				)
				continue
			}
		}

		// Final round.
		_ = session.AppendMessage(path, session.Message{Role: "assistant", Content: full, Timestamp: time.UnixMilli(now())})
		if prop, present, perr := ParseProposal(full); present {
			pp := proposalPayload{RunID: runID, Valid: perr == nil, Summary: prop.Summary, Ops: prop.Ops}
			if perr != nil {
				pp.Error = perr.Error()
				pp.Ops = nil
			}
			_ = r.svc.notify.Notify("companion.proposal", pp)
		}
		_ = r.svc.notify.Notify("companion.done", donePayload{RunID: runID, FullText: full})
		return
	}
}

// querySummary returns a short "조회: toolA, toolB" status string.
func querySummary(qr QueryRequest) string {
	names := make([]string, 0, len(qr.Queries))
	for _, q := range qr.Queries {
		names = append(names, q.Tool)
	}
	return "조회 중: " + strings.Join(names, ", ")
}
```
import에 `strings` 추가(querySummary).

- [ ] **Step 3: 테스트 — 라운드 루프**

companion_test.go의 `fakeClient`를 라운드별 응답 큐로 확장(현재 `full string` 단일 → `[]string` 큐, Chat 호출마다 다음 것 반환). 그리고:
```go
func TestRun_QueryThenFinal(t *testing.T) {
	// round0: a linetta-query; round1: final answer with a proposal.
	round0 := "찾아볼게요\n```linetta-query\n{\"queries\":[{\"tool\":\"list_scenes\",\"args\":{}}]}\n```"
	round1 := "이렇게 제안해요\n```linetta-proposal\n{\"summary\":\"s\",\"ops\":[{\"op\":\"set_outline\",\"outline\":\"x\"}]}\n```"
	svc, notif, projectID := newSvcQueue(t, []string{round0, round1})
	runID, err := svc.Send(context.Background(), projectID, "", "플롯 구상", func() int64 { return 1 })
	if err != nil || runID == "" {
		t.Fatal(err)
	}
	waitFor(t, notif, "companion.done")
	if notif.get("companion.thinking") == "" {
		t.Fatal("expected a thinking event during the query round")
	}
	if !strings.Contains(notif.get("companion.done"), "이렇게 제안해요") {
		t.Fatalf("final answer missing: %s", notif.get("companion.done"))
	}
	if !strings.Contains(notif.get("companion.proposal"), "\"valid\":true") {
		t.Fatalf("final proposal missing: %s", notif.get("companion.proposal"))
	}
	// transcript: user + only the FINAL assistant (query round not persisted)
	msgs, _ := svc.History(context.Background(), projectID)
	if len(msgs) != 2 {
		t.Fatalf("want 2 transcript msgs (user+final assistant), got %d: %+v", len(msgs), msgs)
	}
}
```
`newSvcQueue` = `newSvc`의 응답-큐 버전(fakeClient가 큐를 순서대로 반환). 기존 `newSvc`/`TestSend_*` 테스트는 fakeClient를 `[]string{full}` 큐 1개로 맞춰 동작하도록 fakeClient 시그니처 변경에 따라 갱신.

- [ ] **Step 4: build/test + commit**

Run: `cd engine && go build ./... && go test ./internal/companion/... ./...`
Expected: PASS. 기존 단일-턴 테스트(쿼리 없는 응답)는 round0에서 바로 done — 회귀 없음.
```bash
git add engine/internal/companion/runner.go engine/internal/companion/companion_test.go
git commit -m "feat(companion): on-demand read-query loop (max 3 rounds) + thinking event"
```

---

## Task 3: Rust 브리지 — companion.thinking allowlist

**Files:** `apps/desktop/src-tauri/src/engine.rs`

- [ ] **Step 1:** match에 추가(companion.* 군에):
```rust
            "companion.thinking" => "companion-thinking",
```
- [ ] **Step 2:** `cd apps/desktop/src-tauri && cargo check` (성공) + 커밋:
```bash
git add apps/desktop/src-tauri/src/engine.rs
git commit -m "feat(tauri): forward companion.thinking event"
```

---

## Task 4: FE — thinking 상태 + 쿼리 블록 숨김

**Files:** `apps/desktop/src/lib/types.ts`, `hooks/useCompanion.ts`, `components/companion/CompanionPanel.tsx`

- [ ] **Step 1: types.ts** — `export interface CompanionThinking { run_id: string; text: string; }`

- [ ] **Step 2: useCompanion.ts** — thinking 상태 추가
- import에 `CompanionThinking` 추가. 상태 `const [thinking, setThinking] = useState("");`.
- `companion-thinking` 구독:
```ts
  useEngineEvent<CompanionThinking>("companion-thinking", (p) => {
    if (p.run_id !== runIdRef.current) return;
    setThinking(p.text);
  });
```
- `companion-done`/`-error`/`-cancelled` 핸들러에서 `setThinking("")`로 클리어(스트림 종료 시 상태 제거). send 시작 시에도 `setThinking("")`.
- 반환 객체에 `thinking` 추가: `return { messages, streaming, thinking, status, send, cancel };`

- [ ] **Step 3: CompanionPanel.tsx**
- `streamProse`가 `linetta-query`도 cut하도록(둘 중 먼저 나오는 펜스에서 자름):
```ts
function streamProse(text: string): string {
  let idx = -1;
  for (const fence of ["```linetta-proposal", "```linetta-query"]) {
    const i = text.indexOf(fence);
    if (i >= 0 && (idx < 0 || i < idx)) idx = i;
  }
  return (idx >= 0 ? text.slice(0, idx) : text).trimEnd();
}
```
- `useCompanion`에서 `thinking`도 구조분해. 스트리밍 영역에 thinking 상태 표시(streaming 말풍선 위/아래):
```tsx
        {status === "streaming" && (
          <div className="companion-msg assistant">
            {thinking && <div className="companion-thinking">🔎 {thinking}</div>}
            <div className="companion-bubble">{liveProse || "…"}</div>
          </div>
        )}
```
- CSS(`CompanionPanel.css`)에 `.companion-thinking { font-size: 0.78rem; color: #8a857b; margin-bottom: 0.2rem; }` 추가.

- [ ] **Step 4: tsc + 커밋**
Run: `cd apps/desktop && npx tsc --noEmit` (클린)
```bash
git add apps/desktop/src/lib/types.ts apps/desktop/src/hooks/useCompanion.ts apps/desktop/src/components/companion/CompanionPanel.tsx apps/desktop/src/components/companion/CompanionPanel.css
git commit -m "feat(desktop): companion thinking status + hide query block in stream"
```

---

## Task 5: 최종 검증
- [ ] `cd engine && go test ./...` PASS, `go test ./internal/companion/... -race` 클린
- [ ] `cd apps/desktop && npx tsc --noEmit` 클린
- [ ] `cd apps/desktop/src-tauri && cargo check` 성공
- [ ] repo root `bash scripts/build-engine.sh` → ok
- [ ] 수동 스모크(사용자): "3장 본문 보고 이어서 플롯 제안해줘" → 🔎 조회 중 표시 → 답+제안. 일반 대화는 즉시 1라운드.

## 범위 밖
- 쓰기 도구 자동 실행(쓰기는 제안→검토 유지), 조회 라운드 영속, 시맨틱 검색, 외부 MCP — 후속.
