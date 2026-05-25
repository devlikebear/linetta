# Linetta macOS App Completion — 개발 로드맵

_이 문서는 Phase 6~9의 총괄본. Claude Code는 이 로드맵을 먼저 읽고 전체 맥락을 파악한 뒤, 각 페이즈 작업지시서로 이동해서 실제 구현을 진행한다. 페이즈 간 전환 시 반드시 사용자 확인을 받는다._

_작성일: 2026-05-24_
_예상 전체 소요: 30~50시간 (4 페이즈)_
_페이즈 수: 4개 (Phase 6 ~ Phase 9)_
_선행 로드맵: [`linetta-pro-writer-roadmap.md`](./linetta-pro-writer-roadmap.md) (Phase 1~5 MVP 완료 기반)_

## Overview

Linetta MVP(Phase 1~5)는 Go engine과 SwiftUI macOS 앱이 분리된 채로 완성됐다. 두 컴포넌트가 잘 동작하지만 **macOS 앱만 띄우면 데이터가 안 보이는 결정적 UX 갭**이 있다 — 사용자가 별도 터미널에서 `make serve`를 직접 실행해야 한다.

이 로드맵은 그 갭을 메우고, macOS 앱이 "한 번 실행하면 모든 기능이 동작하는 진짜 데스크탑 앱"이 되도록 다듬는다. 핵심 축은 네 가지:

1. **Embedded Engine**: SwiftUI 앱이 Go 서버 라이프사이클을 직접 소유
2. **Settings Studio**: 빈약한 SettingsView를 실제 운영 도구로 보강 (백업/복구, config 편집, engine 제어 포함)
3. **App Polish**: 앱 메뉴, Canon 변경 이력 뷰, empty/error 상태, toast UX
4. **Live + Editor**: SSE 실시간 run 진행 표시 + NSTextView 기반 장문 원고 편집기

이 4개 페이즈가 끝나면 Linetta는 "Mac에서 매일 쓸 만한 창작 스튜디오"라는 phase-5의 사용자 시나리오를 실질적으로 달성한다.

## 완료 조건 (전체)

모든 페이즈 완료 시:

- [ ] `make macos-run` 한 번이면 Go engine과 SwiftUI 앱이 함께 뜨고, 앱 종료 시 engine도 정리된다.
- [ ] 앱 toolbar 또는 메뉴바에서 engine 연결 상태(healthy/starting/down)가 실시간 표시된다.
- [ ] Settings 탭만으로 engine 재시작, 백업 만들기/복원, Tessera config 편집, 데이터 디렉토리 열기까지 모두 가능하다.
- [ ] 앱 메뉴(File / Edit / View / Window)가 macOS 표준에 맞게 정리되어 키보드만으로 주요 작업이 가능하다.
- [ ] Canon Decisions History 뷰에서 누가/언제/왜 Canon을 바꿨는지 추적할 수 있다.
- [ ] Engine offline 시 명확한 복구 액션이 보이고 (Start engine / Open settings), 빈 상태도 매끄럽게 안내된다.
- [ ] `Run Agents` 실행 중 SSE로 progress가 실시간 갱신된다 (현재는 끝나야 한꺼번에 보임).
- [ ] 장문 원고 편집기가 undo/redo, find, word count, autosave 지표를 지원한다.

## 기술 스택 / 환경

- **Go**: 1.22+, `internal/server` REST/SSE, `cmd/linetta serve/export-library/import-library`
- **SwiftUI**: Swift 6, macOS 15, `swift-tools-version: 6.0` Package.swift
- **저장소**: SQLite via `modernc.org/sqlite`, 사용자 데이터는 `~/.linetta/`
- **IPC**: HTTP(JSON) + SSE. SwiftUI는 LLM provider나 DB를 직접 호출하지 않는다 (원칙 유지)
- **빌드 도구**: Makefile, `swift build`, `go build`
- **관련 외부 모듈**: `github.com/devlikebear/tessera` (오케스트레이션)

코드베이스 분석: 별도 문서 없이 본 로드맵에 통합 (Phase 1~5 산출물이 이미 그 역할을 함)

## Out of Scope (이번 4 페이즈 전체)

- 클라우드 동기화, 협업/공동 편집, 모바일 앱 (linetta-pro-writer-roadmap의 Out of Scope 그대로 유지)
- LLM provider 키 관리 GUI (Settings에서 가이드만 제공, Keychain 통합은 별도 페이즈 후보)
- 의존성 자동 업데이트, 자동 패치
- macOS 앱 코드 사이닝 / Notarization / 배포 패키징
- Universal Binary (현재 arm64 기준만)
- Tessera config의 grammar 수준 검증 (basic YAML parse만)
- 다국어 UI (영문 + 필요 시 한글 토큰만)

## 페이즈 구성

각 페이즈는 **수직 슬라이스** — 끝나면 demo 가능한 무언가가 나온다.

### Phase 6: Embedded Engine Lifecycle — Swift 앱이 Go 서버 라이프사이클을 소유

- **목표**: `make macos-run` 한 번으로 Go engine이 자동 기동/종료되고, 앱은 동적으로 할당된 engine 주소를 사용한다.
- **포함 기능**:
  - `bin/linetta` 사전 빌드 → SwiftUI 앱이 `Process`로 spawn
  - 동적 포트(`--addr 127.0.0.1:0`) + stdout 파싱으로 실제 주소 획득
  - APIClient 전역 공유(@EnvironmentObject 또는 ObservableObject)로 런타임 주소 반영
  - Toolbar에 engine status indicator (●starting / ●healthy / ●down)
  - Settings에 "Use external engine" 토글 (개발자 모드)
- **예상 소요**: 6~10시간
- **작업지시서**: [`phase-6-embedded-engine-lifecycle.md`](./phase-6-embedded-engine-lifecycle.md)
- **Checkpoint 요약**: `make macos-run`만 해도 갤러리에 works가 보이고, 앱 종료 후 `lsof -i :{port}` 로 engine 프로세스가 남지 않는다.

### Phase 7: Settings Studio — 설정 페이지 전수 보강 + 백업/복구 GUI

- **목표**: Settings 한 탭에서 Linetta 운영(engine 제어, 데이터 관리, config 편집)이 모두 가능하다.
- **포함 기능**:
  - Settings 4 섹션 구조 (Engine / Storage / Tessera / About)
  - Engine: status, restart, stop, log tail, "Use external engine" 토글
  - Storage: DB path, "Reveal in Finder", **백업 만들기 / 복원하기** (CLI 기능을 새 HTTP API로 노출)
  - Tessera: config path, **간단 YAML 편집기**(monospace TextEditor + syntax check), provider secret 가이드 링크
  - About: app version, Tessera version, log directory, GitHub 링크
  - 새 HTTP API: `POST /api/library/backup`, `POST /api/library/restore`
- **예상 소요**: 8~12시간
- **작업지시서**: [`phase-7-settings-studio.md`](./phase-7-settings-studio.md)
- **Checkpoint 요약**: Settings에서 백업 생성 → 복원 → 결과가 갤러리에 반영. Tessera config 편집 후 저장하면 다음 run이 새 설정으로 동작.

### Phase 8: App Polish & Workflow Completion — macOS 앱 다운 마무리

- **목표**: macOS 앱 표준에 맞는 메뉴/단축키/상태 UX를 갖추고, Canon 변경 이력을 추적할 수 있게 한다.
- **포함 기능**:
  - 앱 메뉴(File: New Work/New Episode/Export/Import; Edit: Find; View: Toggle Sidebar; Help)
  - Canon Decisions History 탭 (`listMemoryDecisions` 사용, WorkspaceView에 새 Tab)
  - Engine offline empty state: "Start engine" / "Open settings" 직접 액션
  - Error toast (현재 overlay 텍스트 → autohide HUD 스타일)
  - 갤러리 빈 상태: "Create work" + "Import backup" 두 액션
  - Run 실패 시 재시도 버튼 + 에러 상세 패널
- **예상 소요**: 6~8시간
- **작업지시서**: [`phase-8-app-polish-workflow.md`](./phase-8-app-polish-workflow.md)
- **Checkpoint 요약**: 키보드만으로 새 작품/에피소드 생성, Canon 변경 이력 조회, 백업 복원이 가능. Engine을 끄면 toast가 뜨고 Settings로 가는 액션이 보인다.

### Phase 9: Live Run Stream + Long-form Editor — 실시간 진행 + 작가용 편집기

- **목표**: AI 실행 중 진행 상황을 실시간으로 보고, 장문 원고를 작가가 편하게 쓸 수 있는 편집 환경을 갖춘다.
- **포함 기능**:
  - 서버: `POST /api/works/{id}/episodes/{eid}/runs/async` (비동기 run 생성, run_id 즉시 반환)
  - 서버: `/api/runs/{id}/events/stream` 진짜 streaming (run 진행과 동시에 emit)
  - APIClient: `EventStream` async sequence (URLSession bytesStream)
  - EpisodeWorkbenchView: `Run Agents` → 진행률, 현재 에이전트, 실시간 이벤트 리스트
  - `LongformEditor` (NSTextView wrapper): undo/redo, ⌘F find, word count footer, autosave indicator
  - 원고 dirty state 추적 + 닫기 전 확인
- **예상 소요**: 10~20시간
- **작업지시서**: [`phase-9-live-run-and-editor.md`](./phase-9-live-run-and-editor.md)
- **Checkpoint 요약**: Run Agents 누르면 이벤트가 실시간으로 흘러나오고, 5000자 이상 원고를 끊김 없이 편집/검색/카운트할 수 있다.

## 페이즈 간 의존성

```
Phase 5 (MVP 완료, 기존)
  └─→ Phase 6 (Embedded Engine)
        └─→ Phase 6.5 (UI Redesign)
              └─→ Phase 7 (Settings Studio)
                    └─→ Phase 8 (App Polish)
                          └─→ Phase 9 (Live + Editor)
```

**병렬 가능성**: 거의 없음. Phase 7은 Phase 6의 APIClient 공유 구조와 engine 제어 인터페이스에 의존. Phase 8은 Phase 7의 Settings 구조와 새 HTTP API에 의존. Phase 9는 Phase 6 APIClient 공유 구조가 있어야 EventStream 주입이 깔끔.

**예외**: Phase 9의 LongformEditor (T9.D.* 작업 그룹)는 Phase 6 완료 후 독립 작업 가능. 시간 압박 시 Phase 9의 SSE 부분과 분리 진행해도 됨.

## 페이즈 간 전환 규칙

각 페이즈는 다음 조건을 만족해야 완료로 간주:

1. 작업지시서의 모든 체크박스 완료
2. Checkpoint 블록의 모든 검증 통과
3. 사용자가 명시적으로 "Phase N 완료 확인, 다음 진행" 승인

Checkpoint 실패 시:
- Claude Code가 실패 원인을 보고
- 사용자와 함께 원인 파악
- 수정 후 재검증
- 재검증 통과 후 사용자 승인 → 다음 페이즈

## 최종 완료 체크리스트 (Phase 6~9 종료 후)

- [x] Phase 6.5 — UI Redesign: 3-column AppShell with warm-dark theme, Sidebar(Works/Memory/Episodes), Episode workspace(Blueprint + Run history + Review queue), Manuscript inspector, Command palette ⌘K, status footer. Legacy views removed.
- [ ] 모든 페이즈 Checkpoint 통과
- [ ] 전체 Go 테스트 통과: `go test ./...`
- [ ] Swift 패키지 테스트 통과: `cd macos/Linetta && swift test`
- [ ] `go vet ./...` 통과
- [ ] **E2E 시나리오 1 (Cold start)**: 새 머신 기준 `git clone` → `make build` → `make macos-run` → 새 작품 생성 → Run Agents → 결과 채택 → 백업 → 복원이 끊김 없이 가능
- [ ] **E2E 시나리오 2 (장문 연재)**: 5000자 이상 에피소드를 LongformEditor에서 편집 → SSE로 progress 확인 → Canon decisions에서 변경 이력 확인
- [ ] Out of Scope 항목이 잘못 끼어들지 않았는지 확인
- [ ] 기존 컨벤션 준수: Go는 표준 `_test.go`, Swift는 SwiftUI `View` + `@MainActor AppState` 패턴
- [ ] 문서 업데이트: `docs/plan/README.md`에 Phase 6~9 링크 추가, 루트 `README.md`의 실행 방법 단순화 (`make macos-run` 한 줄 강조)

## 참고 자료

- 선행 로드맵: [`linetta-pro-writer-roadmap.md`](./linetta-pro-writer-roadmap.md)
- Phase 5 작업지시서: [`phase-5-publication-polish.md`](./phase-5-publication-polish.md) (특히 [ ] 항목들이 Phase 6~9로 흡수됨)
- 서버 라우트 정의: `internal/server/server.go` `routes()` 메서드
- 현재 APIClient: `macos/Linetta/Sources/LinettaCore/APIClient.swift`
- 현재 Settings: `macos/Linetta/Sources/Linetta/Views/SettingsView.swift`

## 메모

- **APIClient 공유 패턴**: Phase 6에서 결정. 가장 간단한 방향은 `AppState`가 `var client: APIClient`를 가지고 engine 주소 변경 시 새 클라이언트로 교체. 모든 View가 `@EnvironmentObject AppState`를 통해 접근. 현재 6개 View가 직접 `APIClient()` 호출하는 것을 모두 마이그레이션해야 함.
- **bin/linetta 위치**: 배포 시 `.app` 번들 안에 포함되어야 하지만, 본 4 페이즈는 dev 시나리오(`swift run`)만 다룬다. Bundle 패키징은 별도 페이즈 후보.
- **포트 충돌**: 동적 포트(`:0`) 사용으로 회피. 사용자가 외부 engine을 동시에 쓰면 Settings의 "Use external engine" 토글로 spawn 비활성화 가능.
- **백업 API**: 기존 CLI 로직(`runExportLibrary`)을 그대로 `internal/library` 패키지로 추출하고 HTTP handler에서 호출. 중복 구현 금지.

---

_Claude Code 사용 시: 이 로드맵을 먼저 읽고 전체 맥락 파악. 그다음 [`phase-6-embedded-engine-lifecycle.md`](./phase-6-embedded-engine-lifecycle.md)로 이동해서 구현 시작. 페이즈 전환은 반드시 사용자 명시적 승인 후._
