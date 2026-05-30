# Companion Phase 1 — Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Linetta engine에 컴패니언 백엔드를 추가한다 — tars `pkg/session`으로 대화를 영속하고, 프로젝트 컨텍스트를 주입해 LLM 응답을 스트리밍하며, 응답의 `linetta-proposal` JSON 블록을 파싱해 FE로 전달한다(적용은 안 함).

**Architecture:** 새 패키지 `engine/internal/companion/`(proposal 파서 / prompt 빌더 / service+runner). 기존 `ai.Runner`의 스트리밍·취소·dedup 패턴을 따르되, 재사용을 위해 dedup을 공유 패키지 `engine/internal/streamdedup`으로 추출한다. 제안 적용은 Phase 2(FE), 메모리는 Phase 3.

**Tech Stack:** Go 1.26 engine (modernc.org/sqlite), tars `v0.33.0` (`pkg/session`/`pkg/llm`), stdio JSONRPC.

---

## 작업 레포 / 사전 지식 (구현자 필독)

- **모든 경로는 Linetta 레포: `/Users/changheonshin/workspace/myworks/linetta`.** engine 루트는 `engine/`. 테스트: `cd engine && go test ./...`. 빌드: repo root에서 `bash scripts/build-engine.sh`.
- `main` 브랜치에서 작업. `--no-verify` 금지. push는 명시 요청 시에만.
- LSP가 테스트 작성 직후 stale "undefined" 진단을 보일 수 있다 — 항상 실제 `go test` 출력만 신뢰.
- **FE 작업 없음** (Phase 1은 engine 전용). 채팅 패널·rpc 클라이언트·제안 카드·적용은 Phase 2.

### 확인된 기존 코드 (verbatim 시그니처)
- `rpc.Notifier` (`engine/internal/rpc/server.go`): `type Notifier interface { Notify(method string, params any) error }`. 서버에서 `s.Notifier()`로 획득.
- `ai.ClientFactory` (`engine/internal/ai/client.go`): `type ClientFactory func(provider, workDir string) (llm.Client, error)` + `func DefaultClientFactory(provider, workDir string) (llm.Client, error)`.
- `ai.ProviderSource` (`engine/internal/ai/runner.go`): `type ProviderSource interface { Provider() string }`. `*settings.Store`가 `func (s *Store) Provider() string`로 만족.
- `llm` (tars `github.com/devlikebear/tars/pkg/llm`): `type Client interface { Ask(...); Chat(ctx, []ChatMessage, ChatOptions) (ChatResponse, error) }`; `ChatMessage{Role, Content string}`; `ChatOptions{ OnDelta func(text string) }`; `ChatResponse{ Message ChatMessage; ... }`.
- `session` (tars `github.com/devlikebear/tars/pkg/session`, v0.33.0): `NewStore(dir string) *Store`; `(*Store).EnsureWorker(projectID string) (Session, error)`; `(*Store).TranscriptPath(id string) string`; `AppendMessage(path string, msg Message) error`; `ReadMessages(path string) ([]Message, error)`; `LoadHistory(path string, maxTokens int) ([]Message, error)`; `Message{ ID, Role, Content string; Timestamp time.Time; ... }`.
- `plot` (`engine/internal/plot`): `NewBuilder(nodes *node.Repo, beats *beat.Repo, threads *thread.Repo) *Builder`; `(*Builder).Build(ctx, nodeID string) (Spine, error)`; `Spine{ Prev *SceneBeats; Current SceneBeats; Next *SceneBeats }`; `SceneBeats{ NodeID, Label string; Beats []Beat }`; `Beat{ ID, ThreadID, ThreadName, ThreadColor, Label, Description string; Intensity, Ordinal int }`.
- repos: `project.Repo.Get(ctx, id) (Project,error)` (Project has `Outline string`, `LastOpenedNodeID *string`); `thread.Repo.ListByProject(ctx, projectID string, includeClosed bool) ([]Thread,error)` (Thread{ID,ProjectID,Name,Color,Summary string; ClosedAt *int64}); `entity.Repo.Search(ctx, projectID, query string, limit int) ([]Entity,error)` (empty query → recent; Entity{ID,Kind,Name,Role,Summary,...}); `relationship.Repo.ListByProject(ctx, projectID) ([]Relationship,error)` (Relationship{ID,FromID,ToID,Label,Notes string; PairID *string}); `node.NewRepo(st) *node.Repo`.
- `paths.Home() (string, error)` (`engine/internal/paths/paths.go`) — Linetta 홈; 백업 트리는 `filepath.Join(home, "backups")`. 컴패니언은 `filepath.Join(home, "companion")`에 둔다(같은 home 트리 → 기존 backup 스케줄러 범위).
- main.go (`engine/cmd/linetta-engine/main.go`): repos 생성 블록(~64-89), `s.Notifier()`, `clock := func() int64 { return time.Now().UnixMilli() }`, `s.Handle(...)` 등록 블록(~124-179), `home, _ := paths.Home()`(~100).
- 핸들러 컨벤션 (`engine/internal/rpc/handlers/ai.go`): `func RunAI(...) rpc.Handler`; 에러는 `&rpc.MethodError{Code: rpc.CodeInvalidParams|CodeInternalError, Message: …}`. `type Clock func() int64`(handlers 패키지).
- 테스트 더블 (`engine/internal/ai/runner_test.go`): `fakeNotifier{ events []string; Notify(...) }`, `fakeClient{ chunks []string; failAt int; Chat(...) }`, `fixedProvider string` (`Provider()`), `func(_, _ string)(llm.Client,error){return fake,nil}`. 컴패니언 테스트에서 복제.
- dedup (`engine/internal/ai/stream_dedup.go`): 현재 ai 패키지 내부 비공개(`streamDedup`, `newStreamDedup()`, `Observe(text) (dedupAction, string)`, `Final() string`, consts `dedupEmit/dedupSkip/dedupReset`). Task 2에서 공유 패키지로 추출.

## File Structure
- Modify: `engine/go.mod`, `engine/go.sum` — tars v0.33.0.
- Create: `engine/internal/streamdedup/streamdedup.go` (+ `_test.go`) — ai에서 추출한 공유 dedup(공개 API).
- Modify: `engine/internal/ai/runner.go` — 추출된 streamdedup 사용. Delete: `engine/internal/ai/stream_dedup.go`(+그 test 이동).
- Create: `engine/internal/companion/proposal.go` (+ `_test.go`) — 제안 스키마 + 파서.
- Create: `engine/internal/companion/prompt.go` (+ `_test.go`) — 시스템/컨텍스트 프롬프트.
- Create: `engine/internal/companion/companion.go` — Service(조립 + 컨텍스트 수집).
- Create: `engine/internal/companion/runner.go` (+ `companion_test.go`) — 스트리밍·세션·제안 발행·취소.
- Create: `engine/internal/rpc/handlers/companion.go` (+ `_test.go`) — send/history/cancel 핸들러.
- Modify: `engine/cmd/linetta-engine/main.go` — companion 조립 + 등록.

---

## Task 1: tars v0.33.0 의존 범프

**Files:** `engine/go.mod`, `engine/go.sum`

- [ ] **Step 1: 범프 + tidy**

Run (repo root):
```bash
cd engine && go get github.com/devlikebear/tars@v0.33.0 && go mod tidy
```
Expected: `go.mod`의 `require github.com/devlikebear/tars` 가 `v0.33.0`로 갱신, go.sum 업데이트.

- [ ] **Step 2: pkg/session import 가능 확인**

Run: `cd engine && go list github.com/devlikebear/tars/pkg/session`
Expected: 패키지 경로 출력(에러 없음).

- [ ] **Step 3: 빌드/테스트 + 커밋**

Run: `cd engine && go build ./... && go test ./...`
Expected: 전 패키지 PASS.
```bash
git add engine/go.mod engine/go.sum
git commit -m "chore(engine): bump tars to v0.33.0 for pkg/session"
```

---

## Task 2: streamdedup 공유 패키지 추출

ai 패키지 내부 dedup을 공유 패키지로 옮겨 companion이 재사용한다. 순수 이동 + 공개 이름화 + 참조 갱신(동작 불변, 기존 ai 테스트가 가드).

**Files:**
- Create: `engine/internal/streamdedup/streamdedup.go`, `engine/internal/streamdedup/streamdedup_test.go`
- Modify: `engine/internal/ai/runner.go`
- Delete: `engine/internal/ai/stream_dedup.go` (+ 기존 `engine/internal/ai/stream_dedup_test.go`가 있으면 이동)

- [ ] **Step 1: 현재 dedup 읽기**

Run: `cat engine/internal/ai/stream_dedup.go` 및 `ls engine/internal/ai/stream_dedup_test.go 2>/dev/null && cat engine/internal/ai/stream_dedup_test.go`
목적: 정확한 로직/테스트를 그대로 옮기기 위해 원문 확보.

- [ ] **Step 2: 공유 패키지 생성**

`engine/internal/streamdedup/streamdedup.go` — `ai/stream_dedup.go`의 로직을 그대로 옮기되 식별자를 공개:
- `streamDedup` → `Dedup`
- `newStreamDedup()` → `New()`
- `dedupAction` → `Action`; `dedupEmit/dedupSkip/dedupReset` → `ActionEmit/ActionSkip/ActionReset`
- 메서드 `Observe(text string) (Action, string)`, `Final() string` 는 이름 유지(공개).
- 패키지 선언 `package streamdedup`. 내부 필드/헬퍼는 그대로(소문자 유지 가능).

`engine/internal/streamdedup/streamdedup_test.go` — 기존 ai dedup 테스트가 있으면 옮겨와 `package streamdedup`로 바꾸고 `New()/ActionEmit/...` 새 이름으로 호출 변경. 없으면 최소 테스트 1개 작성:
```go
package streamdedup

import "testing"

func TestEmitsNewContent(t *testing.T) {
	d := New()
	act, payload := d.Observe("안녕 ")
	if act != ActionEmit || payload != "안녕 " {
		t.Fatalf("got %v %q", act, payload)
	}
	d.Observe("세계")
	if got := d.Final(); got != "안녕 세계" {
		t.Fatalf("Final = %q", got)
	}
}
```

- [ ] **Step 3: ai/runner.go 참조 갱신 + 옛 파일 삭제**

`engine/internal/ai/runner.go`에서 dedup 사용부를 새 패키지로 교체:
- import에 `"github.com/devlikebear/linetta/engine/internal/streamdedup"` 추가.
- `dedup := newStreamDedup()` → `dedup := streamdedup.New()`.
- `dedupEmit/dedupReset/dedupSkip` → `streamdedup.ActionEmit/ActionReset/ActionSkip`.
- `dedup.Observe(...)`, `dedup.Final()` 호출은 그대로(메서드명 동일).
그런 다음:
```bash
git rm engine/internal/ai/stream_dedup.go
# 기존 stream_dedup_test.go가 있었다면 (Step2에서 옮겼으므로) git rm 으로 제거
```

- [ ] **Step 4: 빌드/테스트 + 커밋**

Run: `cd engine && go build ./... && go test ./internal/ai/... ./internal/streamdedup/... && go test ./...`
Expected: 전부 PASS (ai 동작 불변 — 기존 dedup 테스트가 새 위치에서 통과).
```bash
git add engine/internal/streamdedup engine/internal/ai/runner.go
git commit -m "refactor(engine): extract streamdedup to shared package"
```

---

## Task 3: companion proposal 스키마 + 파서

순수 로직(I/O 없음). TDD.

**Files:**
- Create: `engine/internal/companion/proposal.go`, `engine/internal/companion/proposal_test.go`

- [ ] **Step 1: 타입 + 파서 작성**

`engine/internal/companion/proposal.go`:
```go
// Package companion implements the conversational writing-companion backend:
// session persistence (tars pkg/session), context-injected streaming, and
// parsing of structured plot-edit proposals embedded in model output.
package companion

import (
	"encoding/json"
	"fmt"
	"strings"
)

// proposalFence is the fenced-block language tag the model must use to emit a
// structured plot-edit proposal.
const proposalFence = "linetta-proposal"

// Op is one proposed plot-core mutation. Only fields relevant to Type are set.
type Op struct {
	Type string `json:"op"`

	// create_thread
	Ref     string `json:"ref,omitempty"`
	Name    string `json:"name,omitempty"`
	Color   string `json:"color,omitempty"`
	Summary string `json:"summary,omitempty"`

	// update_thread / add_beat target
	ThreadID  string `json:"thread_id,omitempty"`
	ThreadRef string `json:"thread_ref,omitempty"`

	// add_beat / update_beat
	NodeID      string `json:"node_id,omitempty"`
	BeatID      string `json:"beat_id,omitempty"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	Intensity   int    `json:"intensity,omitempty"`

	// set_outline
	Outline string `json:"outline,omitempty"`
}

// Proposal is the parsed contents of a linetta-proposal block.
type Proposal struct {
	Summary string `json:"summary"`
	Ops     []Op   `json:"ops"`
}

// knownOps lists the plot-core op types accepted in Phase 1.
var knownOps = map[string]bool{
	"create_thread": true, "update_thread": true,
	"add_beat": true, "update_beat": true, "delete_beat": true,
	"set_outline": true,
}

// ParseProposal scans full model output for a linetta-proposal fenced block.
// Returns (proposal, blockPresent, error):
//   - no block:        (Proposal{}, false, nil)
//   - one valid block: (parsed, true, nil)
//   - invalid/>=2:     (best-effort, true, err)
func ParseProposal(full string) (Proposal, bool, error) {
	blocks := extractFencedBlocks(full, proposalFence)
	if len(blocks) == 0 {
		return Proposal{}, false, nil
	}
	if len(blocks) > 1 {
		p, _ := decodeProposal(blocks[0])
		return p, true, fmt.Errorf("multiple linetta-proposal blocks (%d); only one allowed", len(blocks))
	}
	p, err := decodeProposal(blocks[0])
	if err != nil {
		return p, true, err
	}
	if err := validateProposal(p); err != nil {
		return p, true, err
	}
	return p, true, nil
}

func decodeProposal(body string) (Proposal, error) {
	var p Proposal
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &p); err != nil {
		return Proposal{}, fmt.Errorf("invalid proposal JSON: %w", err)
	}
	return p, nil
}

func validateProposal(p Proposal) error {
	if len(p.Ops) == 0 {
		return fmt.Errorf("proposal has no ops")
	}
	refs := map[string]bool{}
	for _, op := range p.Ops {
		if op.Type == "create_thread" && op.Ref != "" {
			refs[op.Ref] = true
		}
	}
	for i, op := range p.Ops {
		if !knownOps[op.Type] {
			return fmt.Errorf("op[%d]: unknown op %q", i, op.Type)
		}
		switch op.Type {
		case "create_thread":
			if strings.TrimSpace(op.Name) == "" {
				return fmt.Errorf("op[%d] create_thread: name required", i)
			}
		case "update_thread":
			if op.ThreadID == "" {
				return fmt.Errorf("op[%d] update_thread: thread_id required", i)
			}
		case "add_beat":
			hasID := op.ThreadID != ""
			hasRef := op.ThreadRef != ""
			if hasID == hasRef {
				return fmt.Errorf("op[%d] add_beat: exactly one of thread_id/thread_ref required", i)
			}
			if hasRef && !refs[op.ThreadRef] {
				return fmt.Errorf("op[%d] add_beat: thread_ref %q not declared by any create_thread.ref", i, op.ThreadRef)
			}
			if op.NodeID == "" || strings.TrimSpace(op.Label) == "" {
				return fmt.Errorf("op[%d] add_beat: node_id and label required", i)
			}
		case "update_beat":
			if op.BeatID == "" {
				return fmt.Errorf("op[%d] update_beat: beat_id required", i)
			}
		case "delete_beat":
			if op.BeatID == "" {
				return fmt.Errorf("op[%d] delete_beat: beat_id required", i)
			}
		case "set_outline":
			// outline may be empty (clears); no required field
		}
	}
	return nil
}

// extractFencedBlocks returns the bodies of all ```<lang> ... ``` blocks whose
// info-string equals lang.
func extractFencedBlocks(s, lang string) []string {
	var out []string
	lines := strings.Split(s, "\n")
	inBlock := false
	var buf []string
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if !inBlock {
			if trimmed == "```"+lang {
				inBlock = true
				buf = nil
			}
			continue
		}
		if trimmed == "```" {
			out = append(out, strings.Join(buf, "\n"))
			inBlock = false
			continue
		}
		buf = append(buf, ln)
	}
	return out
}
```

- [ ] **Step 2: 테스트 작성**

`engine/internal/companion/proposal_test.go`:
```go
package companion

import "testing"

func block(body string) string {
	return "어쩌고\n```linetta-proposal\n" + body + "\n```\n저쩌고"
}

func TestParseProposal_NoBlock(t *testing.T) {
	_, present, err := ParseProposal("그냥 대화입니다. 블록 없음.")
	if present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
}

func TestParseProposal_Valid(t *testing.T) {
	body := `{"summary":"복수극 추가","ops":[
	  {"op":"create_thread","ref":"t1","name":"복수극"},
	  {"op":"add_beat","thread_ref":"t1","node_id":"n1","label":"결심"},
	  {"op":"set_outline","outline":"전체 개요"}
	]}`
	p, present, err := ParseProposal(block(body))
	if !present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if p.Summary != "복수극 추가" || len(p.Ops) != 3 {
		t.Fatalf("p=%+v", p)
	}
}

func TestParseProposal_UnknownOp(t *testing.T) {
	_, present, err := ParseProposal(block(`{"ops":[{"op":"frobnicate"}]}`))
	if !present || err == nil {
		t.Fatalf("expected invalid-op error, present=%v err=%v", present, err)
	}
}

func TestParseProposal_AddBeatXorThread(t *testing.T) {
	// both thread_id and thread_ref → error
	_, _, err := ParseProposal(block(`{"ops":[{"op":"add_beat","thread_id":"x","thread_ref":"y","node_id":"n","label":"l"}]}`))
	if err == nil {
		t.Fatal("expected XOR error")
	}
	// neither → error
	_, _, err = ParseProposal(block(`{"ops":[{"op":"add_beat","node_id":"n","label":"l"}]}`))
	if err == nil {
		t.Fatal("expected XOR error")
	}
}

func TestParseProposal_DanglingThreadRef(t *testing.T) {
	_, _, err := ParseProposal(block(`{"ops":[{"op":"add_beat","thread_ref":"nope","node_id":"n","label":"l"}]}`))
	if err == nil {
		t.Fatal("expected dangling thread_ref error")
	}
}

func TestParseProposal_BadJSON(t *testing.T) {
	_, present, err := ParseProposal(block(`{not json`))
	if !present || err == nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
}

func TestParseProposal_MultipleBlocks(t *testing.T) {
	two := block(`{"ops":[{"op":"set_outline","outline":"a"}]}`) + "\n" + block(`{"ops":[{"op":"set_outline","outline":"b"}]}`)
	_, present, err := ParseProposal(two)
	if !present || err == nil {
		t.Fatalf("expected multi-block error, present=%v err=%v", present, err)
	}
}
```

- [ ] **Step 3: 실행 + 커밋**

Run: `cd engine && go test ./internal/companion/...`
Expected: PASS.
```bash
git add engine/internal/companion/proposal.go engine/internal/companion/proposal_test.go
git commit -m "feat(companion): plot-proposal schema + parser"
```

---

## Task 4: companion prompt/context 빌더

순수 함수(데이터 → 문자열). 서비스가 repos에서 데이터를 모아 이 함수들에 넘긴다.

**Files:**
- Create: `engine/internal/companion/prompt.go`, `engine/internal/companion/prompt_test.go`

- [ ] **Step 1: 빌더 작성**

`engine/internal/companion/prompt.go`:
```go
package companion

import (
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// PromptData is everything buildContext needs, gathered by the service.
type PromptData struct {
	Outline       string
	Spine         plot.Spine
	HasSpine      bool
	Threads       []thread.Thread
	Entities      []entity.Entity
	Relationships []relationship.Relationship
}

// buildSystem returns the companion persona + proposal-format rules.
func buildSystem() string {
	var b strings.Builder
	b.WriteString("당신은 한국어 소설 작가의 집필 동료입니다. 작가와 자연스럽게 대화하며 플롯·인물·전개를 함께 구상합니다.\n\n")
	b.WriteString("구체적인 플롯 변경(스토리라인 생성/수정, 비트 추가/수정/삭제, 작품 개요 설정)을 제안할 때만, 응답에 다음 형식의 펜스드 블록을 **정확히 하나** 포함하세요. 단순 대화·질문 응답이면 블록을 넣지 마세요.\n\n")
	b.WriteString("```linetta-proposal\n")
	b.WriteString(`{"summary":"<한 줄 요약>","ops":[ ... ]}` + "\n")
	b.WriteString("```\n\n")
	b.WriteString("op 종류: create_thread{ref?,name,color?,summary?}, update_thread{thread_id,name?,color?,summary?}, add_beat{thread_id|thread_ref,node_id,label,description?,intensity?}, update_beat{beat_id,label?,description?,intensity?}, delete_beat{beat_id}, set_outline{outline}.\n")
	b.WriteString("기존 스토리라인·씬은 아래 컨텍스트에 주어진 id로 참조하세요. 같은 제안에서 새로 만든 스토리라인은 create_thread.ref 핸들을 add_beat.thread_ref로 참조하세요.\n")
	b.WriteString("당신은 변경을 직접 적용하지 않습니다 — 작가가 제안을 검토 후 적용합니다.\n")
	return b.String()
}

// buildContext renders the project state as a single user-role message body.
func buildContext(d PromptData) string {
	var b strings.Builder
	if s := strings.TrimSpace(d.Outline); s != "" {
		b.WriteString("## 작품 개요\n")
		b.WriteString(s)
		b.WriteString("\n\n")
	}
	if d.HasSpine && hasSpineBeats(d.Spine) {
		b.WriteString("## 플롯\n")
		writeScene(&b, "[이전 씬]", d.Spine.Prev)
		writeSceneVal(&b, "[현재 씬]", d.Spine.Current)
		writeScene(&b, "[다음 씬]", d.Spine.Next)
		b.WriteString("\n")
	}
	if len(d.Threads) > 0 {
		b.WriteString("## 스토리라인\n")
		for _, t := range d.Threads {
			line := fmt.Sprintf("- [%s] %s", t.ID, t.Name)
			if t.Summary != "" {
				line += " — " + t.Summary
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}
	if len(d.Entities) > 0 || len(d.Relationships) > 0 {
		b.WriteString("## 등장 인물·장소·관계\n")
		nameByID := map[string]string{}
		for _, e := range d.Entities {
			nameByID[e.ID] = e.Name
			line := fmt.Sprintf("- [%s] %s", e.ID, e.Name)
			if e.Role != "" {
				line += " / " + e.Role
			}
			if e.Summary != "" {
				line += ": " + e.Summary
			}
			b.WriteString(line + "\n")
		}
		seen := map[string]bool{}
		for _, r := range d.Relationships {
			if r.PairID != nil && *r.PairID != "" {
				if seen[*r.PairID] {
					continue
				}
				seen[*r.PairID] = true
			}
			from, to := nameByID[r.FromID], nameByID[r.ToID]
			if from == "" || to == "" {
				continue
			}
			b.WriteString(fmt.Sprintf("- %s ↔ %s: %s\n", from, to, r.Label))
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func hasSpineBeats(s plot.Spine) bool {
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

func writeScene(b *strings.Builder, tag string, s *plot.SceneBeats) {
	if s == nil || len(s.Beats) == 0 {
		return
	}
	writeSceneVal(b, tag, *s)
}

func writeSceneVal(b *strings.Builder, tag string, s plot.SceneBeats) {
	if len(s.Beats) == 0 {
		return
	}
	b.WriteString(tag + "\n")
	for _, bt := range s.Beats {
		line := fmt.Sprintf("  · [%s] %s", bt.ThreadName, bt.Label)
		if strings.TrimSpace(bt.Description) != "" {
			line += " — " + bt.Description
		}
		b.WriteString(line + "\n")
	}
}
```

- [ ] **Step 2: 테스트 작성**

`engine/internal/companion/prompt_test.go`:
```go
package companion

import (
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

func TestBuildSystem_HasProposalRules(t *testing.T) {
	s := buildSystem()
	for _, want := range []string{"집필 동료", "linetta-proposal", "create_thread", "add_beat", "직접 적용하지 않습니다"} {
		if !strings.Contains(s, want) {
			t.Fatalf("system missing %q", want)
		}
	}
}

func TestBuildContext_RendersSections(t *testing.T) {
	d := PromptData{
		Outline:  "전체 개요",
		HasSpine: true,
		Spine: plot.Spine{
			Current: plot.SceneBeats{NodeID: "n1", Beats: []plot.Beat{{ThreadName: "메인", Label: "발단", Description: "주인공 등장"}}},
		},
		Threads: []thread.Thread{{ID: "th1", Name: "메인플롯", Summary: "중심 줄기"}},
	}
	out := buildContext(d)
	for _, want := range []string{"## 작품 개요", "전체 개요", "## 플롯", "[현재 씬]", "메인", "## 스토리라인", "[th1] 메인플롯"} {
		if !strings.Contains(out, want) {
			t.Fatalf("context missing %q in:\n%s", want, out)
		}
	}
}

func TestBuildContext_EmptyIsBlank(t *testing.T) {
	if out := buildContext(PromptData{}); out != "" {
		t.Fatalf("empty data should yield empty context, got %q", out)
	}
}
```

- [ ] **Step 3: 실행 + 커밋**

Run: `cd engine && go test ./internal/companion/...`
Expected: PASS.
```bash
git add engine/internal/companion/prompt.go engine/internal/companion/prompt_test.go
git commit -m "feat(companion): system persona + context prompt builders"
```

---

## Task 5: companion Service + Runner (세션·스트리밍·제안·취소)

**Files:**
- Create: `engine/internal/companion/companion.go`, `engine/internal/companion/runner.go`, `engine/internal/companion/companion_test.go`

- [ ] **Step 1: Service (조립 + 컨텍스트 수집) 작성**

`engine/internal/companion/companion.go`:
```go
package companion

import (
	"context"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/thread"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/devlikebear/tars/pkg/session"
)

// historyTokenBudget caps how much prior transcript is replayed into context.
const historyTokenBudget = 6000

// entityContextLimit caps how many entities are injected.
const entityContextLimit = 40

// ClientFactory mirrors ai.ClientFactory (provider id + workDir → llm.Client).
type ClientFactory func(provider, workDir string) (llm.Client, error)

// ProviderSource yields the current provider id (settings.Store satisfies it).
type ProviderSource interface{ Provider() string }

// Service wires the companion backend.
type Service struct {
	sessions      *session.Store
	projects      *project.Repo
	threads       *thread.Repo
	entities      *entity.Repo
	relationships *relationship.Repo
	plot          *plot.Builder
	notify        rpc.Notifier
	factory       ClientFactory
	src           ProviderSource
	workDir       string
	runner        *Runner
}

// NewService constructs the companion service. sessionsDir is the directory
// passed to session.NewStore (e.g. <home>/companion).
func NewService(
	sessionsDir string,
	projects *project.Repo, threads *thread.Repo, entities *entity.Repo,
	relationships *relationship.Repo, plotBuilder *plot.Builder,
	notify rpc.Notifier, factory ClientFactory, src ProviderSource, workDir string,
) *Service {
	s := &Service{
		sessions:      session.NewStore(sessionsDir),
		projects:      projects, threads: threads, entities: entities,
		relationships: relationships, plot: plotBuilder,
		notify: notify, factory: factory, src: src, workDir: workDir,
	}
	s.runner = newRunner(s)
	return s
}

// gatherContext loads project state for prompt injection. nodeID may be "".
func (s *Service) gatherContext(ctx context.Context, projectID, nodeID string) (PromptData, error) {
	proj, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return PromptData{}, err
	}
	d := PromptData{Outline: proj.Outline}

	resolvedNode := nodeID
	if resolvedNode == "" && proj.LastOpenedNodeID != nil {
		resolvedNode = *proj.LastOpenedNodeID
	}
	if resolvedNode != "" {
		if sp, err := s.plot.Build(ctx, resolvedNode); err == nil {
			d.Spine = sp
			d.HasSpine = true
		}
	}
	if ths, err := s.threads.ListByProject(ctx, projectID, false); err == nil {
		d.Threads = ths
	}
	if ents, err := s.entities.Search(ctx, projectID, "", entityContextLimit); err == nil {
		d.Entities = ents
	}
	if rels, err := s.relationships.ListByProject(ctx, projectID); err == nil {
		d.Relationships = rels
	}
	return d, nil
}

// History returns the project's companion transcript messages.
func (s *Service) History(ctx context.Context, projectID string) ([]session.Message, error) {
	sess, err := s.sessions.EnsureWorker(projectID)
	if err != nil {
		return nil, err
	}
	return session.ReadMessages(s.sessions.TranscriptPath(sess.ID))
}

// Send starts a companion turn; returns the run id. Streaming + proposal arrive
// via notifications. Cancel cancels an in-flight run.
func (s *Service) Send(ctx context.Context, projectID, nodeID, text string, now func() int64) (string, error) {
	return s.runner.start(ctx, projectID, nodeID, text, now)
}

func (s *Service) Cancel(runID string) error { return s.runner.cancel(runID) }
```

- [ ] **Step 2: Runner (스트리밍·세션·제안·취소) 작성**

`engine/internal/companion/runner.go`:
```go
package companion

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/devlikebear/linetta/engine/internal/streamdedup"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/devlikebear/tars/pkg/session"
	"github.com/google/uuid"
)

// notification payloads
type deltaPayload struct {
	RunID string `json:"run_id"`
	Text  string `json:"text"`
}
type donePayload struct {
	RunID    string `json:"run_id"`
	FullText string `json:"full_text"`
}
type errorPayload struct {
	RunID   string `json:"run_id"`
	Message string `json:"message"`
}
type cancelledPayload struct {
	RunID string `json:"run_id"`
}
type proposalPayload struct {
	RunID   string `json:"run_id"`
	Valid   bool   `json:"valid"`
	Summary string `json:"summary,omitempty"`
	Ops     []Op   `json:"ops,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Runner manages companion run lifecycle + cancellation.
type Runner struct {
	svc    *Service
	mu     sync.Mutex
	active map[string]context.CancelFunc
}

func newRunner(svc *Service) *Runner {
	return &Runner{svc: svc, active: map[string]context.CancelFunc{}}
}

func (r *Runner) start(ctx context.Context, projectID, nodeID, text string, now func() int64) (string, error) {
	sess, err := r.svc.sessions.EnsureWorker(projectID)
	if err != nil {
		return "", err
	}
	path := r.svc.sessions.TranscriptPath(sess.ID)

	data, err := r.svc.gatherContext(ctx, projectID, nodeID)
	if err != nil {
		return "", err
	}

	// Persist the user turn before streaming.
	ts := time.UnixMilli(now())
	if err := session.AppendMessage(path, session.Message{Role: "user", Content: text, Timestamp: ts}); err != nil {
		// best-effort: continue even if persistence failed
		_ = err
	}

	// Build the message list: system + context + history + new user turn.
	msgs := []llm.ChatMessage{{Role: "system", Content: buildSystem()}}
	if cctx := buildContext(data); cctx != "" {
		msgs = append(msgs, llm.ChatMessage{Role: "user", Content: cctx})
	}
	if hist, err := session.LoadHistory(path, historyTokenBudget); err == nil {
		for _, m := range hist {
			msgs = append(msgs, llm.ChatMessage{Role: m.Role, Content: m.Content})
		}
	}
	// LoadHistory already includes the just-appended user turn as the last item.

	client, err := r.svc.factory(r.svc.src.Provider(), r.svc.workDir)
	if err != nil {
		return "", err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	runID := uuid.NewString()
	r.mu.Lock()
	r.active[runID] = cancel
	r.mu.Unlock()

	go r.run(runCtx, runID, path, msgs, client, now)
	return runID, nil
}

func (r *Runner) finish(runID string) {
	r.mu.Lock()
	if c, ok := r.active[runID]; ok {
		c()
		delete(r.active, runID)
	}
	r.mu.Unlock()
}

func (r *Runner) cancel(runID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.active[runID]
	if !ok {
		return errors.New("companion: run not found or already finished")
	}
	c()
	return nil
}

func (r *Runner) run(ctx context.Context, runID, path string, msgs []llm.ChatMessage, client llm.Client, now func() int64) {
	defer r.finish(runID)
	dedup := streamdedup.New()

	resp, err := client.Chat(ctx, msgs, llm.ChatOptions{
		OnDelta: func(text string) {
			switch act, payload := dedup.Observe(text); act {
			case streamdedup.ActionEmit:
				_ = r.svc.notify.Notify("companion.delta", deltaPayload{RunID: runID, Text: payload})
			case streamdedup.ActionReset:
				// companion has no reset event; re-emit as delta of full buffer is avoided.
				// Treat reset as emit of the corrected payload for simplicity.
				_ = r.svc.notify.Notify("companion.delta", deltaPayload{RunID: runID, Text: payload})
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

	// Persist assistant turn.
	_ = session.AppendMessage(path, session.Message{Role: "assistant", Content: full, Timestamp: time.UnixMilli(now())})

	// Parse + emit proposal if a block is present.
	if prop, present, perr := ParseProposal(full); present {
		pp := proposalPayload{RunID: runID, Valid: perr == nil, Summary: prop.Summary, Ops: prop.Ops}
		if perr != nil {
			pp.Error = perr.Error()
			pp.Ops = nil
		}
		_ = r.svc.notify.Notify("companion.proposal", pp)
	}

	_ = r.svc.notify.Notify("companion.done", donePayload{RunID: runID, FullText: full})
}
```
(주의: `github.com/google/uuid`는 이미 go.mod에 있음 — beat/repo.go 등에서 사용 중. import 미사용 경고가 나면 빌드 메시지에 따라 정리.)

- [ ] **Step 3: 테스트 작성 (fakes)**

`engine/internal/companion/companion_test.go` — fakeClient(제안 블록 든 full_text) + fakeNotifier + 임시 sessionsDir + 임시 store로 프로젝트 1개 생성. ai/runner_test.go의 더블 패턴 + 기존 repo 테스트의 store 헬퍼를 본떠 작성:
```go
package companion

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
	"github.com/devlikebear/tars/pkg/llm"
)

type fakeClient struct{ full string }

func (f *fakeClient) Ask(context.Context, string) (string, error) { return "", nil }
func (f *fakeClient) Chat(ctx context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	if opts.OnDelta != nil {
		opts.OnDelta(f.full)
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: f.full}}, nil
}

type fakeNotifier struct {
	mu     sync.Mutex
	events map[string]string // method → last params JSON
}

func (n *fakeNotifier) Notify(method string, params any) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.events == nil {
		n.events = map[string]string{}
	}
	b, _ := json.Marshal(params)
	n.events[method] = string(b)
	return nil
}

type fixedProvider string

func (p fixedProvider) Provider() string { return string(p) }

func newSvc(t *testing.T, full string) (*Service, *fakeNotifier, string) {
	t.Helper()
	st := store.OpenTestStore(t) // adjust to the repo's actual test-store helper
	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	threads := thread.NewRepo(st)
	beats := store.NewBeatRepoOrWhatever(st) // see note below
	_ = beats
	entities := entity.NewRepo(st)
	rels := relationship.NewRepo(st)
	pb := plot.NewBuilder(nodes, beatRepo(st), threads)
	notif := &fakeNotifier{}
	fc := &fakeClient{full: full}
	svc := NewService(t.TempDir(), projects, threads, entities, rels, pb, notif,
		func(_, _ string) (llm.Client, error) { return fc, nil }, fixedProvider("claude-code-cli"), "")
	p, err := projects.Create(context.Background(), 1, project.NewInput{Title: "t", Genres: []string{"f"}, LengthTarget: "novel", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	_ = p
	return svc, notif, p.ID
}
```
NOTE for implementer: import `beat` (`engine/internal/beat`) for `beat.NewRepo(st)` to build the plot.Builder; fix the helper names (`store.OpenTestStore` etc.) to whatever the repo's existing tests actually use (read `engine/internal/plot/builder_test.go` — it already constructs project/node/thread/beat repos + a temp store; copy that exact setup). Then add tests:
```go
func TestSend_StreamsDoneAndPersists(t *testing.T) {
	full := "좋아요! 복수극 라인을 제안할게요.\n```linetta-proposal\n{\"summary\":\"복수극\",\"ops\":[{\"op\":\"set_outline\",\"outline\":\"복수 서사\"}]}\n```"
	svc, notif, projectID := newSvc(t, full)
	done := make(chan struct{})
	// Send runs async; poll notifier for companion.done.
	runID, err := svc.Send(context.Background(), projectID, "", "복수극 어때?", func() int64 { return 1000 })
	if err != nil || runID == "" {
		t.Fatalf("Send err=%v runID=%q", err, runID)
	}
	// Wait for done (the fake client returns synchronously; the goroutine finishes fast).
	waitFor(t, notif, "companion.done")
	close(done)

	if !strings.Contains(notif.get("companion.done"), "복수 서사") {
		t.Fatalf("done payload missing full text: %s", notif.get("companion.done"))
	}
	if !strings.Contains(notif.get("companion.proposal"), "\"valid\":true") {
		t.Fatalf("expected valid proposal: %s", notif.get("companion.proposal"))
	}
	// transcript has user + assistant
	msgs, err := svc.History(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Fatalf("transcript = %+v", msgs)
	}
}
```
Add small test helpers `waitFor(t, notif, method)` (poll `notif.get(method) != ""` up to ~2s with short sleeps) and `(*fakeNotifier).get(method)` (locked read). Also add `TestSend_NoProposalWhenNoBlock` (full text without a block → `companion.done` present, `companion.proposal` absent).

- [ ] **Step 4: 실행 + 커밋**

Run: `cd engine && go test ./internal/companion/...`
Expected: PASS.
```bash
git add engine/internal/companion/companion.go engine/internal/companion/runner.go engine/internal/companion/companion_test.go
git commit -m "feat(companion): service + streaming runner (session, proposal emit, cancel)"
```

---

## Task 6: RPC 핸들러 + main.go 배선

**Files:**
- Create: `engine/internal/rpc/handlers/companion.go`, `engine/internal/rpc/handlers/companion_test.go`
- Modify: `engine/cmd/linetta-engine/main.go`

- [ ] **Step 1: 핸들러 작성**

`engine/internal/rpc/handlers/companion.go`:
```go
package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type companionSendParams struct {
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id"`
	Text      string `json:"text"`
}

// CompanionSend returns a handler for companion.send.
func CompanionSend(svc *companion.Service, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p companionSendParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" || p.Text == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id and text required"}
		}
		runID, err := svc.Send(ctx, p.ProjectID, p.NodeID, p.Text, func() int64 { return now() })
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(map[string]string{"run_id": runID})
	}
}

type companionHistoryParams struct {
	ProjectID string `json:"project_id"`
}

type companionMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// CompanionHistory returns a handler for companion.history.
func CompanionHistory(svc *companion.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p companionHistoryParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		msgs, err := svc.History(ctx, p.ProjectID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		out := make([]companionMessage, 0, len(msgs))
		for _, m := range msgs {
			out = append(out, companionMessage{Role: m.Role, Content: m.Content, Timestamp: m.Timestamp.UnixMilli()})
		}
		return json.Marshal(map[string][]companionMessage{"messages": out})
	}
}

type companionCancelParams struct {
	RunID string `json:"run_id"`
}

// CompanionCancel returns a handler for companion.cancel.
func CompanionCancel(svc *companion.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p companionCancelParams
		if err := json.Unmarshal(params, &p); err != nil || p.RunID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "run_id required"}
		}
		if err := svc.Cancel(p.RunID); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
```

- [ ] **Step 2: main.go 배선**

`engine/cmd/linetta-engine/main.go`:
- import에 `"github.com/devlikebear/linetta/engine/internal/companion"` 및 `"path/filepath"`(이미 있으면 생략) 추가.
- repos/plotBuilder 생성부 뒤(예: `plotBuilder := plot.NewBuilder(...)` 다음), `home` 확보 이후에 companion 서비스 생성. `home`은 main 후반(~100)에서 얻으므로, companion 생성은 `home` 확보 다음으로 배치:
```go
	companionSvc := companion.NewService(
		filepath.Join(home, "companion"),
		projects, threads, entities, relationships, plotBuilder,
		s.Notifier(), companion.ClientFactory(ai.DefaultClientFactory), settingsStore, home,
	)
```
  (`ai.DefaultClientFactory`의 타입은 `ai.ClientFactory` = `func(string,string)(llm.Client,error)`인데 companion은 동일 시그니처의 `companion.ClientFactory`를 받으므로 `companion.ClientFactory(ai.DefaultClientFactory)` 변환으로 전달. `settingsStore`는 `Provider() string`을 가져 `companion.ProviderSource` 만족. workDir은 `home` 사용.)
- 핸들러 등록(예: `s.Handle("ai.cancel", ...)` 다음):
```go
	s.Handle("companion.send", handlers.CompanionSend(companionSvc, clock))
	s.Handle("companion.history", handlers.CompanionHistory(companionSvc))
	s.Handle("companion.cancel", handlers.CompanionCancel(companionSvc))
```

- [ ] **Step 3: 핸들러 테스트 (있는 패턴 재사용)**

핸들러 테스트 하네스가 있으면 `companion_test.go` 추가: `companion.send` 빈 파라미터 → InvalidParams; 정상 → `{run_id}` 반환(서비스는 fake client로 구성). 하네스 구성이 번거로우면 companion 패키지 테스트로 갈음하고, 핸들러는 빌드 + 빈-파라미터 검증만 최소 테스트. 어떤 선택을 했는지 보고.

- [ ] **Step 4: 빌드/테스트 + 커밋**

Run: `cd engine && go build ./... && go test ./...`
Expected: 전 패키지 PASS.
```bash
git add engine/internal/rpc/handlers/companion.go engine/internal/rpc/handlers/companion_test.go engine/cmd/linetta-engine/main.go
git commit -m "feat(engine): companion.send/history/cancel RPC + wiring"
```

---

## 최종 검증 (모든 Task 후)

- [ ] `cd engine && go test ./...` 전 패키지 PASS
- [ ] repo root `bash scripts/build-engine.sh` → "ok"
- [ ] `engine/go.mod` tars `v0.33.0`
- [ ] companion 패키지: proposal 파서(7 케이스)/prompt 빌더/service(스트림·세션·제안)/핸들러 테스트 통과
- [ ] ai 동작 불변(streamdedup 추출 후 기존 ai 테스트 통과)
- [ ] FE 미변경(Phase 1 engine 전용)

## 범위 밖 (Phase 1 아님)

- FE: rpc 클라이언트(companion 네임스페이스)·채팅 패널·제안 검토 카드·적용 버튼 — Phase 2.
- 제안 적용(ref 해소 + 기존 RPC 호출) — Phase 2.
- 메모리 쓰기/회상 — Phase 3.
- 관계·엔티티 op, ai.reset 대응 companion 이벤트 — 후속.
