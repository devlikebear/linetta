# Linetta 컴패니언 — Phase 0: tars `pkg/session` export 설계

> 작성일: 2026-05-30
> 구현 레포: **tars** (`/Users/changheonshin/workspace/myworks/tars`) — Linetta와 별개 git 모듈. 본 spec은 컴패니언 기능의 일부라 Linetta 문서에 보관하되, Phase 0의 코드 변경은 전적으로 tars 레포에서 일어난다.

## 상위 맥락 — Linetta 컴패니언 (전체 비전)

Linetta 안에서 AI가 "집필 컴패니언"으로 동작하게 한다. 지속 대화 세션 + 메모리(작품·작가 취향 누적) + 자연어로 플롯/스레드/비트/개요를 함께 작성·수정하는 능력을 갖는다. 씬 본문 직접 집필은 범위 밖.

브레인스토밍에서 확정한 핵심 결정:
1. **정체성:** 집필 컴패니언 (세션 + 메모리 + 플롯 tool-calling + 브레인스토밍/질문응답; 씬 본문 제외).
2. **저장소:** tars 파일 스토어(sessions.json/`{id}.jsonl`, MEMORY.md/experiences.jsonl)를 그대로 쓰되 **Linetta 백업·gitsync 트리 아래**(`companion/{projectId}/`)에 둬 함께 백업.
3. **편집 적용:** **제안 → 검토 → 적용**. AI는 tool-call로 변경안만 내고, FE가 카드/diff로 보여주고, 작가가 [적용]/[부분]/[거절] 후 **기존 RPC**(threads/beats/projects.update)로 커밋.
4. **메모리 회상:** 키워드/구조 기반만(임베딩 없음). tars memory가 시맨틱 비활성 시 graceful fallback 제공.

### 단계 분해 (각각 별도 spec → plan → 구현)

| 단계 | 내용 | 레포 |
|---|---|---|
| **Phase 0 (본 spec)** | tars `pkg/session` export (internal/session 별칭) + 버전 범프 | tars |
| Phase 1 | 컴패니언 백엔드 — 세션/메모리 배선, `chat.send`/스트리밍 RPC, tool-call 제안 프로토콜 | Linetta engine |
| Phase 2 | 컴패니언 FE — 채팅 패널 + 제안 검토 카드 | Linetta FE |
| Phase 3 | 메모리 쓰기/회상 통합 | Linetta engine+FE |

이 순서인 이유: Phase 0가 없으면 세션 영속 자체가 불가(기반). Phase 1·2가 "제안→적용" 수직 슬라이스를 동작시키고, Phase 3에서 기억을 입힌다.

---

## Phase 0 목표 (한 문장)

tars의 채팅 세션 영속 기능(`internal/session`)을, 기존 `pkg/llm`·`pkg/memory`와 동일한 **타입 별칭 export 패턴**으로 `pkg/session`에 공개해 Linetta가 import할 수 있게 한다. (새 로직 없음 — 노출만.)

## 배경 / 현황 (조사 결과)

- tars 모듈 `github.com/devlikebear/tars`, Go 1.25.6, 현재 `v0.32.72`. 파일 기반 저장, **DB/CGO 의존 없음** → Linetta의 modernc.org/sqlite(순수 Go)와 직교, Tauri 사이드카 빌드 호환.
- `pkg/memory`는 **이미 export됨** (Backend/FileBackend/Service/Experience/Search… + 팩토리). Phase 3에서 그대로 import.
- `pkg/llm`은 **이미 사용 중**이며 `ChatOptions.Tools`/`ToolSchema`/`ToolChoice`/`ChatResponse.ToolCalls`로 tool-calling을 지원 → Phase 1에서 활용.
- `internal/session`(비공개)에 컴패니언이 필요한 표면이 이미 존재:
  - `func NewStore(dir string) *Store`
  - `Store` 메서드: `Create`/`CreateWithOptions`/`EnsureMain`/`EnsureWorker(projectID string)`/`Get`/`List`/`ListAll`/`Touch`/`SetTitle`/`Delete`/`TranscriptPath(id) string`/`Latest`/… (전부 메서드라 타입 별칭만으로 공개됨)
  - 타입: `Session`, `Message`(ID/Role/Content/Timestamp + Tool 필드), `HistorySnapshot`(Messages/Tokens/CompactionUsed)
  - transcript 패키지 함수(path 기반): `AppendMessage`, `ReadMessages`, `RewriteMessages`, `LoadHistory(path, maxTokens)`, `LoadHistorySnapshot(path, maxTokens)`, `EstimateMessageTokenCost(msg)`
- export 패턴 레퍼런스: `pkg/llm/exports.go`, `pkg/memory/exports.go` — `import internal "…/internal/X"` + `type T = internal.T` + 얇은 함수 위임.

## 아키텍처 / 무엇을 만드나

tars 레포에 새 패키지 `pkg/session` 디렉터리를 추가한다. 순수 별칭/위임 레이어이며 `internal/session`은 **변경하지 않는다**(125개 내부 사용처 무영향).

### 컴포넌트

**`pkg/session/exports.go`** — 공개 표면. 정확한 내용:
```go
package session

import internal "github.com/devlikebear/tars/internal/session"

// Types
type Store = internal.Store
type Session = internal.Session
type Message = internal.Message
type HistorySnapshot = internal.HistorySnapshot

// Constructor
func NewStore(dir string) *Store { return internal.NewStore(dir) }

// Transcript helpers (path-based; path comes from (*Store).TranscriptPath(id))
func AppendMessage(path string, msg Message) error                  { return internal.AppendMessage(path, msg) }
func ReadMessages(path string) ([]Message, error)                   { return internal.ReadMessages(path) }
func RewriteMessages(path string, msgs []Message) error             { return internal.RewriteMessages(path, msgs) }
func LoadHistory(path string, maxTokens int) ([]Message, error)     { return internal.LoadHistory(path, maxTokens) }
func LoadHistorySnapshot(path string, maxTokens int) (HistorySnapshot, error) { return internal.LoadHistorySnapshot(path, maxTokens) }
func EstimateMessageTokenCost(msg Message) int                      { return internal.EstimateMessageTokenCost(msg) }
```

근거:
- `Store` 타입 별칭 → 모든 공개 메서드(특히 per-project 세션에 쓸 `EnsureWorker`)가 자동 공개. 컴패니언용 신규 메서드 불필요.
- `Session`의 부가 서브타입(SessionGoal/SessionCritic/SessionToolConfig 등)은 컴패니언이 사용하지 않으므로 별칭하지 않는다(최소 표면 유지, YAGNI). 필요해지면 후속 단계에서 추가.
- transcript 함수는 path 인자를 받으며, path는 `store.TranscriptPath(sessionID)`로 얻는다 → Phase 1이 이 흐름으로 메시지를 적재/로드.

**`pkg/session/doc.go`** — 패키지 docstring 한 단락: "외부 모듈이 tars의 파일 기반 채팅 세션·트랜스크립트 영속을 재사용하기 위한 공개 표면. internal/session에 대한 얇은 별칭이며 저장 포맷(sessions.json + `{id}.jsonl`)은 그대로다."

### 데이터 흐름 (Phase 1이 이 표면을 어떻게 쓸지 — 참고용, 본 Phase 구현 아님)
```
store := session.NewStore(companionDir)         // companionDir = Linetta 백업 트리 하위
sess, _ := store.EnsureWorker(projectID)         // 프로젝트별 컴패니언 세션
path := store.TranscriptPath(sess.ID)
session.AppendMessage(path, session.Message{Role:"user", Content: text, Timestamp: now})
hist, _ := session.LoadHistory(path, budget)     // 토큰 예산 내 최근 히스토리
```

## 버전 / 배포

- `VERSION.txt`를 다음 마이너로 범프(현재 `0.32.72` → 제안 `0.33.0`; 실제 값은 tars의 버전 규칙에 맞춰 플랜에서 확정).
- tars 레포에 커밋. **원격 push 및 릴리스 태그는 사용자가 수행**(Go 모듈 프록시가 새 태그를 해석하려면 원격 반영 필요).
- **Linetta 소비는 Phase 1로 분리.** 개발 중에는 Linetta `engine/go.mod`에 머신-로컬 `replace github.com/devlikebear/tars => /Users/changheonshin/workspace/myworks/tars`를 걸어 새 `pkg/session`을 빌드 검증(이 replace는 커밋하지 않음). 릴리스 시 `require`를 새 태그로 범프.

## tars 레포 작업 규약 (플랜에서 확정)

- 구현 시작 전 tars의 현재 브랜치·청결 상태를 확인하고, 사용자의 일반 규약(메인에서 작업하되 안전장치, `--no-verify` 금지, push는 명시 요청 시에만)을 동일하게 적용한다. tars에 별도 pre-commit/CI 훅이 있으면 존중한다.
- `internal/session` 및 다른 internal 패키지는 수정하지 않는다.

## 에러 처리

순수 별칭/위임 레이어라 런타임 분기 없음. 위임 함수는 internal의 에러를 그대로 전파한다. 실패 모드는 internal/session이 이미 처리(파일 없음 → 빈 결과, 잠금 등).

## 테스트 전략

`pkg/session/exports_test.go` (tars, Go TDD):
- `NewStore(t.TempDir())` → `EnsureWorker("proj-1")` 성공, 반환 Session.ID 비어있지 않음.
- `TranscriptPath(id)`가 store 디렉터리 하위 경로를 반환.
- `AppendMessage`로 user/assistant 2건 적재 → `ReadMessages`가 순서대로 2건, Role/Content 일치.
- `LoadHistory(path, 큰 예산)`이 2건 모두 반환; `LoadHistory(path, 아주 작은 예산)`이 최신 위주로 잘려 반환(토큰 예산 동작 확인).
- 존재하지 않는 transcript path에 `ReadMessages` → 빈 결과, 에러 없음.

전체 검증: tars 레포에서 `go build ./... && go test ./...` 그린(회귀 없음 — internal 미변경). `go vet ./pkg/session/...` 클린.

## 성공 기준

1. `pkg/session`가 존재하고 `Store`/`Session`/`Message`/`HistorySnapshot` + `NewStore` + transcript 함수들을 공개한다.
2. 외부에서 `import "github.com/devlikebear/tars/pkg/session"` 후 `NewStore`/`EnsureWorker`/`TranscriptPath`/`AppendMessage`/`LoadHistory`를 호출할 수 있다(테스트가 입증).
3. `internal/session` 무변경, tars 기존 테스트 전부 통과.
4. `VERSION.txt` 범프 + tars 커밋 완료(push/태그는 사용자 몫).

## 범위 밖 (Phase 0 아님)

- 컴패니언 세션 생성 정책, 메시지 적재 로직, 스트리밍, tool-call 제안 — Phase 1.
- Linetta `go.mod` 영구 변경(require 태그 범프) — Phase 1/릴리스.
- `pkg/memory` 변경 — 불필요(이미 export됨).
- `Session` 부가 서브타입 별칭 — 필요 시 후속.
