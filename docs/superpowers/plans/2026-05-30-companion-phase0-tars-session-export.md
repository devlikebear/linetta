# Companion Phase 0 — tars `pkg/session` export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** tars의 채팅 세션 영속(`internal/session`)을 기존 `pkg/llm`·`pkg/memory`와 동일한 타입 별칭 패턴으로 `pkg/session`에 공개해 외부 모듈(Linetta)이 import할 수 있게 한다.

**Architecture:** 순수 별칭/위임 레이어. `internal/session`은 변경하지 않고, `pkg/session/exports.go`가 `Store`/`Session`/`Message`/`HistorySnapshot` 타입 별칭 + `NewStore` + transcript 위임 함수만 노출한다. 새 로직 없음.

**Tech Stack:** Go 1.25.6, tars 모듈 `github.com/devlikebear/tars` (파일 기반 저장, CGO/DB 의존 없음).

---

## 작업 레포 / 사전 지식 (구현자 필독)

- **모든 파일 경로는 tars 레포 기준이다: `/Users/changheonshin/workspace/myworks/tars`.** Linetta 레포가 아니다. 모든 git/go 명령은 tars 레포에서 실행한다.
- tars는 `main` 브랜치에서 작업한다(사용자 지시). `--no-verify` 금지, 훅 우회 금지. **push 및 릴리스 태그는 하지 않는다**(사용자가 직접 수행).
- 커스텀 git 훅/CI 없음. 검증은 `go build ./...` + `go test ./...`.
- export 패턴 레퍼런스(반드시 먼저 읽을 것): `pkg/llm/exports.go`, `pkg/memory/exports.go`. 둘 다 `import internal "…/internal/X"` + `type T = internal.T` + 얇은 함수 위임 구조다. `pkg/session`도 정확히 이 패턴을 따른다.
- `internal/session`은 **수정 금지**. 다른 internal 패키지도 건드리지 않는다.
- 확인된 internal/session 심볼(검증 완료):
  - `func NewStore(dir string) *Store` (`internal/session/session.go:276`)
  - `func (s *Store) EnsureWorker(projectID string) (Session, error)` (`session.go:761`)
  - `func (s *Store) TranscriptPath(id string) string` (`session.go:1489`)
  - `func (s *Store) Create(title string) (Session, error)` (`session.go:546`)
  - `type Session struct {…}` (`session.go:234`), `type Store struct {…}` (`session.go:265`)
  - `type Message struct { ID, Role, Content string; Timestamp time.Time; ToolName, ToolCallID, ToolArgs string; ToolIsError bool }` (`internal/session/message.go:7`)
  - `type HistorySnapshot struct { Messages []Message; Tokens int; CompactionUsed bool }` (`internal/session/transcript.go:158`)
  - transcript 함수(`internal/session/transcript.go`): `AppendMessage(path string, msg Message) error`, `ReadMessages(path string) ([]Message, error)`, `RewriteMessages(path string, msgs []Message) error`, `LoadHistory(path string, maxTokens int) ([]Message, error)`, `LoadHistorySnapshot(path string, maxTokens int) (HistorySnapshot, error)`
  - `func EstimateMessageTokenCost(msg Message) int` (`internal/session/compaction.go:384`)
- LSP가 테스트 작성 직후 stale "undefined" 진단을 보일 수 있다. **항상 실제 `go test` 출력만 신뢰**한다.

## File Structure

- Create: `pkg/session/exports.go` — 공개 표면(타입 별칭 + NewStore + transcript 위임). 단일 책임: internal/session의 공개 API.
- Create: `pkg/session/doc.go` — 패키지 docstring.
- Create: `pkg/session/exports_test.go` — 공개 표면이 외부에서 동작함을 입증(별도 _test 패키지에서 black-box로).
- Modify: `VERSION.txt` — 버전 범프.

---

## Task 1: `pkg/session` 공개 표면 + 테스트

**Files (tars 레포 기준):**
- Create: `pkg/session/exports.go`
- Create: `pkg/session/doc.go`
- Create: `pkg/session/exports_test.go`

- [ ] **Step 1: 먼저 패턴 확인**

Run: `cat pkg/memory/exports.go | head -20` 및 `cat pkg/llm/exports.go | head -15`
목적: `import internal "…"` + `type T = internal.T` + 얇은 함수 위임 패턴을 눈으로 확인한 뒤 동일하게 작성.

- [ ] **Step 2: `pkg/session/doc.go` 작성**

```go
// Package session exposes tars' file-backed chat session and transcript
// persistence for external agent applications. It is a thin alias layer over
// internal/session; the on-disk format (sessions/sessions.json index plus one
// sessions/{id}.jsonl transcript per session) is unchanged. Construct a Store
// with NewStore(dir); obtain a session's transcript path via
// (*Store).TranscriptPath(id); then append/read/load messages with the
// transcript helpers.
package session
```

- [ ] **Step 3: `pkg/session/exports.go` 작성**

```go
package session

import internal "github.com/devlikebear/tars/internal/session"

// Types — aliases so external callers get the full method set / struct fields.
type Store = internal.Store
type Session = internal.Session
type Message = internal.Message
type HistorySnapshot = internal.HistorySnapshot

// NewStore returns a Store rooted at dir (transcripts live under dir/sessions).
func NewStore(dir string) *Store { return internal.NewStore(dir) }

// AppendMessage appends one message as a JSON line to the transcript at path.
func AppendMessage(path string, msg Message) error { return internal.AppendMessage(path, msg) }

// ReadMessages reads all messages from the transcript at path (empty if absent).
func ReadMessages(path string) ([]Message, error) { return internal.ReadMessages(path) }

// RewriteMessages replaces the transcript contents with msgs.
func RewriteMessages(path string, msgs []Message) error { return internal.RewriteMessages(path, msgs) }

// LoadHistory returns the most recent messages fitting within maxTokens,
// oldest-first.
func LoadHistory(path string, maxTokens int) ([]Message, error) {
	return internal.LoadHistory(path, maxTokens)
}

// LoadHistorySnapshot is LoadHistory plus token/compaction metadata.
func LoadHistorySnapshot(path string, maxTokens int) (HistorySnapshot, error) {
	return internal.LoadHistorySnapshot(path, maxTokens)
}

// EstimateMessageTokenCost returns the token estimate used for history budgeting.
func EstimateMessageTokenCost(msg Message) int { return internal.EstimateMessageTokenCost(msg) }
```

- [ ] **Step 4: 빌드 확인(컴파일)**

Run: `go build ./pkg/session/...`
Expected: 성공(출력 없음). 실패 시 internal 심볼명을 실제 코드와 대조해 수정(예: 함수 시그니처가 다르면 맞춤).

- [ ] **Step 5: black-box 테스트 작성**

`pkg/session/exports_test.go` — 외부 소비자 관점을 위해 **`package session_test`** 로 작성(공개 표면만 사용):

```go
package session_test

import (
	"testing"
	"time"

	"github.com/devlikebear/tars/pkg/session"
)

func TestExportedSessionRoundTrip(t *testing.T) {
	store := session.NewStore(t.TempDir())

	sess, err := store.EnsureWorker("proj-1")
	if err != nil {
		t.Fatalf("EnsureWorker: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("expected non-empty session id")
	}

	path := store.TranscriptPath(sess.ID)
	if path == "" {
		t.Fatal("expected non-empty transcript path")
	}

	now := time.Now()
	if err := session.AppendMessage(path, session.Message{Role: "user", Content: "안녕", Timestamp: now}); err != nil {
		t.Fatalf("AppendMessage user: %v", err)
	}
	if err := session.AppendMessage(path, session.Message{Role: "assistant", Content: "반가워요", Timestamp: now.Add(time.Second)}); err != nil {
		t.Fatalf("AppendMessage assistant: %v", err)
	}

	msgs, err := session.ReadMessages(path)
	if err != nil {
		t.Fatalf("ReadMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "안녕" {
		t.Fatalf("msg0 = %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "반가워요" {
		t.Fatalf("msg1 = %+v", msgs[1])
	}
}

func TestExportedLoadHistoryTokenBudget(t *testing.T) {
	store := session.NewStore(t.TempDir())
	sess, err := store.EnsureWorker("proj-2")
	if err != nil {
		t.Fatalf("EnsureWorker: %v", err)
	}
	path := store.TranscriptPath(sess.ID)
	now := time.Now()
	for i := 0; i < 3; i++ {
		if err := session.AppendMessage(path, session.Message{
			Role:      "user",
			Content:   "메시지 본문이 토큰 예산 동작을 검증할 만큼 충분히 길어야 한다 " + time.Duration(i).String(),
			Timestamp: now.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("AppendMessage %d: %v", i, err)
		}
	}

	// Generous budget → all messages.
	all, err := session.LoadHistory(path, 100000)
	if err != nil {
		t.Fatalf("LoadHistory generous: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("generous budget want 3, got %d", len(all))
	}

	// Tiny budget → fewer than all (most recent kept). Asserts the budget is
	// actually applied, not ignored.
	few, err := session.LoadHistory(path, 1)
	if err != nil {
		t.Fatalf("LoadHistory tiny: %v", err)
	}
	if len(few) >= len(all) {
		t.Fatalf("tiny budget should truncate: got %d (all=%d)", len(few), len(all))
	}
}

func TestExportedReadMessagesMissingFile(t *testing.T) {
	msgs, err := session.ReadMessages(t.TempDir() + "/does-not-exist.jsonl")
	if err != nil {
		t.Fatalf("ReadMessages missing should not error: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("want 0 messages for missing file, got %d", len(msgs))
	}
}
```

- [ ] **Step 6: 테스트 실행**

Run: `go test ./pkg/session/...`
Expected: PASS (3 tests).
주의: `LoadHistory(path, 1)`가 0건을 반환할 수도 있다(예산이 한 메시지보다 작으면). 테스트는 `len(few) >= len(all)`만 실패로 보므로 0건도 통과한다. 만약 internal 구현이 "최소 1건 보장"이라 항상 전부 반환한다면 이 단언이 깨질 수 있는데, 그 경우 토큰 예산 단언을 `len(few) <= len(all)`로 완화하지 말고 — 대신 `LoadHistorySnapshot`의 `Tokens` 필드가 예산 내인지 검증하는 형태로 바꾼다(실제 동작을 먼저 `go test -run TestExportedLoadHistoryTokenBudget -v`로 관찰해 맞춘다).

- [ ] **Step 7: 전체 회귀 확인**

Run: `go build ./... && go test ./...`
Expected: 전 패키지 PASS (internal 미변경이므로 회귀 없음). `go vet ./pkg/session/...` 도 클린.

- [ ] **Step 8: 커밋**

```bash
git add pkg/session
git commit -m "feat(session): export pkg/session (alias over internal/session)"
```
(tars 레포에서. push/tag 하지 않음.)

---

## Task 2: 버전 범프

**Files (tars 레포 기준):**
- Modify: `VERSION.txt`

- [ ] **Step 1: 현재 버전 확인**

Run: `cat VERSION.txt`
Expected: `0.32.72`

- [ ] **Step 2: 버전 범프**

`VERSION.txt` 전체 내용을 다음으로 교체(새 공개 패키지 추가 → 마이너 범프):
```
0.33.0
```
(만약 tars의 버전 도구/스크립트가 별도 형식을 요구하면 — `cat VERSION.txt`로 확인한 기존 포맷이 단순 `X.Y.Z` 한 줄이면 위 그대로. 끝에 개행 유무는 기존 파일과 동일하게 맞춘다.)

- [ ] **Step 3: 버전 참조 잔여 확인(선택)**

Run: `grep -rn "0.32.72" --include="*.go" . | head`
빌드된 바이너리에 버전을 박는 곳이 있어 `0.32.72`가 하드코딩돼 있으면 보고만 하고 임의 변경하지 않는다(보통 VERSION.txt가 단일 소스). 잔여가 없으면 그대로 진행.

- [ ] **Step 4: 최종 빌드/테스트 + 커밋**

Run: `go build ./... && go test ./...`
Expected: 전 패키지 PASS.
```bash
git add VERSION.txt
git commit -m "chore(release): bump version to 0.33.0 for pkg/session export"
```
(push/tag 하지 않음. 사용자가 직접 push + 릴리스 태그.)

---

## 최종 검증 (모든 Task 후)

- [ ] tars 레포에서 `go build ./...` 성공
- [ ] tars 레포에서 `go test ./...` 전 패키지 PASS
- [ ] `go vet ./pkg/session/...` 클린
- [ ] `pkg/session`가 `Store`/`Session`/`Message`/`HistorySnapshot` + `NewStore` + transcript 함수(`AppendMessage`/`ReadMessages`/`RewriteMessages`/`LoadHistory`/`LoadHistorySnapshot`/`EstimateMessageTokenCost`)를 공개
- [ ] `internal/session` 무변경(diff로 확인)
- [ ] `VERSION.txt` = `0.33.0`, 두 커밋 생성, **push/태그 미수행**

## 범위 밖 (Phase 0 아님)

- Linetta 측 소비(go.mod `replace`/`require` 범프, 컴패니언 배선) — Phase 1.
- 세션 생성 정책·스트리밍·tool-call 제안 — Phase 1.
- `Session` 부가 서브타입(SessionGoal/Critic 등) 별칭 — 필요 시 후속.
- `pkg/memory` 변경 — 불필요(이미 export됨).
