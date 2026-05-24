# Phase 9: Live Run Stream + Long-form Editor — 작업지시서

_AI 실행 중 진행 상황을 실시간으로 보고, 장문 원고를 작가가 편하게 쓸 수 있는 편집 환경을 갖춘다. 백롭 두 주제를 묶었다._

_작성일: 2026-05-24_
_속한 로드맵: [`linetta-macos-app-completion-roadmap.md`](./linetta-macos-app-completion-roadmap.md)_
_예상 소요: 10~20시간_

## 페이즈 목표

`Run Agents`를 누르면 진행률, 현재 에이전트, 이벤트가 실시간으로 EpisodeWorkbenchView에 흘러나오고, 작가는 5000자 이상 장문 원고도 LongformEditor에서 끊김 없이 편집/검색/카운트한다. 끝나면 Linetta가 "Mac에서 매일 쓸 만한 창작 스튜디오"라는 phase-5 사용자 시나리오를 실질적으로 달성한다.

## 전제 조건

- [ ] Phase 8 완료 및 사용자 승인
- [ ] 현재 `handleEpisodeRun`이 동기 실행(블로킹)임을 인지: `agent.RunEpisode`가 끝나야 응답 반환. SSE를 의미 있게 쓰려면 비동기 실행 구조 필요
- [ ] Tessera run event 스키마 확인 (`tesserarun.Event` 구조)

## 포함 기능

### 라이브 Run Stream
1. **비동기 run 엔드포인트** `POST /api/works/{id}/episodes/{eid}/runs/async` — run_id 즉시 반환, 백그라운드 실행
2. **실시간 SSE** `/api/runs/{id}/events/stream` 가 진행 중 이벤트를 emit (현재는 동기 후 일괄)
3. **Swift EventStream** — URLSession bytesStream으로 SSE 파싱
4. **EpisodeWorkbenchView 라이브 패널** — 진행 단계, 현재 에이전트, 이벤트 리스트가 실시간 갱신

### 장문 편집기
5. **LongformEditor** — NSTextViewRepresentable wrapper
6. **편집 기능**: undo/redo (system standard), ⌘F find, word/character count footer, autosave indicator
7. **Dirty state 추적**: 변경 시 ● 표시, 닫기 전 확인 alert

## 이 페이즈에서 하지 않는 것

- Tessera 단계 시각화 다이어그램 → Out of Scope (그래프 라이브러리 도입 부담)
- Rich text / Markdown rendering 편집기 → Out of Scope (plain text + monospace 유지)
- Vim/Emacs 키바인딩 → Out of Scope
- Collaborative editing → Out of Scope

## 작업 체크리스트

### 작업 그룹 A: Async run + 실시간 SSE (Go side)

- [ ] **T9.A.1** — `agent.Runner`에 비동기 실행 지원
  - 파일: `internal/agent/runner.go`
  - 변경:
    - 현재 `RunEpisode(ctx, input) (RunResult, error)` 동기 메서드 유지
    - 신규 `StartEpisodeRun(ctx, input) (runID string, err error)` — DB에 run record를 `status=running`으로 즉시 insert하고 goroutine으로 실제 실행 spawn
    - 이벤트를 DB(`agent_run_events` 테이블)에 발생 시점에 즉시 insert (현재는 끝나고 일괄 저장 가정)
    - 완료 시 status를 `completed`/`failed`로 update
  - 검증: 단위 테스트 — StartEpisodeRun 호출 후 1초 내 run record 존재 + events가 시간 차이를 두고 누적

- [ ] **T9.A.2** — `events_stream`이 진짜 streaming
  - 파일: `internal/server/server.go` (`handleRunEventsStream`)
  - 현재 동작: `ListEvents` 한 번 호출 → 모두 write → 종료
  - 변경:
    - `ListEvents`로 기존 이벤트 먼저 flush
    - 그 뒤 short-poll(예: 500ms 간격)로 새 이벤트 확인 → 새 것만 write
    - run status가 completed/failed가 되면 마무리 이벤트 보내고 close
    - 클라이언트 close (request context cancel) 감지 시 즉시 종료
  - 검증: `curl -N http://127.0.0.1:.../api/runs/{id}/events/stream` 으로 실시간 stream 관찰

- [ ] **T9.A.3** — async run 엔드포인트 추가
  - 파일: `internal/server/server.go`
  - 새 경로: `POST /api/works/{id}/episodes/{eid}/runs/async`
  - response: `{"run_id": "..."}` 즉시 반환
  - 기존 동기 엔드포인트(`runs`)는 호환성 유지 (CLI/테스트가 사용)
  - 검증: 단위 테스트

### 작업 그룹 B: Swift EventStream

- [ ] **T9.B.1** — `RunEventStream` async sequence
  - 파일: `macos/Linetta/Sources/LinettaCore/RunEventStream.swift` (신규)
  - 타입: `public struct RunEventStream: AsyncSequence` with `Element = RunEvent`
  - 구현:
    - 내부에서 `URLSession.shared.bytes(for: request)` 사용
    - line-by-line 읽어서 `event:` / `data:` 헤더 파싱 → `RunEvent` decode → yield
    - 빈 줄(이벤트 종료) 처리
  - APIClient에 추가: `public func streamRunEvents(runID: String) -> RunEventStream`
  - 검증: 단위 테스트 (mock server 또는 임시 SSE producer)

- [ ] **T9.B.2** — APIClient에 async run 메서드 추가
  - 파일: `macos/Linetta/Sources/LinettaCore/APIClient.swift`
  - 추가:
    - `public func startEpisodeRun(workID:, episodeID:, request:) async throws -> RunStartResponse`
    - `RunStartResponse { let runID: String }` 모델
  - 검증: 빌드

### 작업 그룹 C: EpisodeWorkbench 라이브 패널

- [ ] **T9.C.1** — runAgents를 비동기 흐름으로 재작성
  - 파일: `macos/Linetta/Sources/Linetta/Views/EpisodeWorkbenchView.swift`
  - 변경:
    - 기존 `client.runEpisode(...)` 대신 `client.startEpisodeRun(...)` 호출 → run_id 받음
    - `for try await event in client.streamRunEvents(runID:)` 루프 → events에 append + 마지막 이벤트 기준 UI 상태 업데이트
    - 완료 이벤트 수신 시 `loadVersions`, `loadReview` 호출
    - 도중 사용자가 다른 에피소드로 전환 시 stream cancel (Task handle 보관)
  - 검증: 실제 run 중 events 리스트가 시간 따라 차오름

- [ ] **T9.C.2** — Live progress 표시
  - 같은 파일
  - 변경: artifactTimelinePanel에 상단 영역 추가
    - 현재 단계 (예: `planning`, `mandate`, `executing`, `reviewing`) — event.type에서 추출
    - 현재 활성 에이전트 (event.role)
    - 경과 시간 (시작부터 카운터)
    - "Cancel Run" 버튼 (Task cancel + 서버에 cancel signal — 이번엔 client side cancel만, 서버 cancel은 향후 작업)
  - 검증: Run 중 시각적으로 진행 상황 보임

### 작업 그룹 D: LongformEditor

- [ ] **T9.D.1** — `LongformEditor` NSViewRepresentable
  - 파일: `macos/Linetta/Sources/Linetta/Views/LongformEditor.swift` (신규)
  - 타입: `struct LongformEditor: NSViewRepresentable`
  - 내부: `NSScrollView` 안에 `NSTextView`
  - 설정:
    - isRichText = false
    - usesFontPanel = false
    - allowsUndo = true (system undo/redo)
    - font = `.monospacedSystemFont(ofSize: 14)` (옵션으로 .body도 토글)
    - typingAttributes에 lineSpacing
    - automaticQuoteSubstitutionEnabled = false (작가는 직접 쓰는 따옴표 선호)
  - Binding: `@Binding var text: String`
  - Coordinator: NSTextViewDelegate로 textDidChange → text 갱신, dirty 감지
  - 검증: 미리보기 + 5000자 lorem ipsum 입력 후 끊김 없음

- [ ] **T9.D.2** — Find UI (⌘F)
  - 같은 파일
  - NSTextView의 standard Find Bar 활성화: `usesFindBar = true`, `isIncrementalSearchingEnabled = true`
  - ⌘F는 `performTextFinderAction(.showFindInterface)` 호출
  - 검증: ⌘F → 상단에 NSTextFinder bar 표시

- [ ] **T9.D.3** — Word/Character Count Footer
  - 파일: `macos/Linetta/Sources/Linetta/Views/LongformEditor.swift` 또는 별도 `EditorFooter`
  - 표시: words: N · characters: N · last saved: 2m ago
  - words count: `text.split(separator: " " 또는 whitespace).count` (한글은 character 기반이 적절 → 옵션 토글)
  - last saved: 부모에서 last save time을 binding으로 전달
  - 검증: 입력하면 실시간 카운트 갱신

- [ ] **T9.D.4** — ManuscriptVersionView 마이그레이션
  - 파일: `macos/Linetta/Sources/Linetta/Views/ManuscriptVersionView.swift`
  - 변경: 기존 TextEditor를 LongformEditor로 교체
  - dirty 상태 binding 추가, 부모(EpisodeWorkbenchView)에 전파
  - 검증: Manuscript 탭에서 LongformEditor가 보이고 undo/redo 동작

- [ ] **T9.D.5** — Dirty state + 종료 확인
  - 파일: `macos/Linetta/Sources/Linetta/Views/EpisodeWorkbenchView.swift`
  - 변경:
    - `@State private var manuscriptDirty = false`
    - 에피소드 전환 / 워크 전환 / 윈도우 닫기 전에 dirty면 alert: "Discard unsaved changes?"
    - Save (T9.D.3의 last saved 갱신)
  - 검증: dirty 상태에서 다른 에피소드 선택 시 alert

---

## ✅ Phase 9 Checkpoint

**구현 확인:**
- [ ] 모든 작업 체크박스 완료
- [ ] async run + SSE 엔드포인트 동작
- [ ] LongformEditor가 Manuscript 탭에서 사용됨

**자동 검증:**
- [ ] Go 테스트 통과: `go test ./...`
- [ ] Swift 테스트 통과: `cd macos/Linetta && swift test`
- [ ] `internal/agent` 비동기 테스트 통과: `go test ./internal/agent/...`

**수동 확인:**
- [ ] **실시간 progress**: Run Agents 클릭 → 1초 내 첫 이벤트 도착, 단계/에이전트가 시간 따라 갱신
- [ ] **장문 편집**: Manuscript 탭에 5000자 이상 텍스트 입력/스크롤/편집이 끊김 없음
- [ ] **Find**: ⌘F → Find Bar 표시 → 단어 검색/하이라이트
- [ ] **Undo/Redo**: ⌘Z / ⌘⇧Z로 편집 되돌리기
- [ ] **Dirty 보호**: 편집 중 다른 에피소드 선택 → "Discard?" alert
- [ ] **Word count**: footer가 입력에 맞춰 실시간 갱신

**완료 처리:**
1. 위 항목 모두 통과 시 Claude Code는 사용자에게 보고:
   - 완료된 작업 요약
   - 자동 검증 결과
   - 수동 확인 항목 결과
2. **로드맵 최종 완료 체크리스트 (E2E 시나리오 1, 2) 진행 안내**
3. 모든 E2E 통과 후 사용자 명시적 승인으로 **로드맵 전체 완료**

---

## 참고 자료

- 로드맵: [`linetta-macos-app-completion-roadmap.md`](./linetta-macos-app-completion-roadmap.md)
- 현재 동기 run handler: `internal/server/server.go:383-409`
- 현재 SSE handler (단순): `internal/server/server.go:608-626`
- 현재 ManuscriptVersionView: `macos/Linetta/Sources/Linetta/Views/ManuscriptVersionView.swift`
- NSTextView 가이드: Apple Developer Documentation > NSTextView

## 메모 / 주의

- async run 도입은 DB schema 변경 가능성 — `agent_runs.status` 컬럼이 없으면 추가 필요. 마이그레이션은 `internal/store`의 기존 패턴 따름
- SSE polling 500ms는 trade-off. 너무 짧으면 DB 부담, 너무 길면 체감 지연. 향후 condition variable로 개선 가능
- NSTextView wrapper는 SwiftUI focus 관리와 까다로움. 첫 구현은 always-focused로 단순화
- 한글 단어 카운트는 모호 → 옵션 제공 ("Words" 기본 / "Characters" 토글)
- LongformEditor의 자동 저장은 명시적 ⌘S만. 부분 자동 저장은 별도 페이즈 (Phase 10 후보)
- Phase 9 완료 후 남는 자연스러운 후속 작업: (1) Server cancel for runs (2) Phase 10 후보 — App 번들 빌드 + Notarization (3) Cloud sync 기획

---
_이 페이즈가 로드맵의 마지막. 완료 후 로드맵 [최종 완료 체크리스트](./linetta-macos-app-completion-roadmap.md#최종-완료-체크리스트-phase-69-종료-후) 진행._
