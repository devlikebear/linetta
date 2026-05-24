# Phase 8: App Polish & Workflow Completion — 작업지시서

_macOS 앱 표준에 맞는 메뉴/단축키/상태 UX를 갖추고, Canon 변경 이력을 추적할 수 있게 한다. 마지막 사용성 디테일을 정리한다._

_작성일: 2026-05-24_
_속한 로드맵: [`linetta-macos-app-completion-roadmap.md`](./linetta-macos-app-completion-roadmap.md)_
_예상 소요: 6~8시간_

## 페이즈 목표

키보드만으로 새 작품 생성, 새 에피소드, 내보내기, 설정 열기, Canon 검색이 가능하고, Engine offline 같은 비정상 상황에서도 명확한 복구 액션이 보인다. Canon Decisions History 뷰로 "누가 언제 왜 Canon을 바꿨는지" 추적 가능.

## 전제 조건

- [ ] Phase 7 완료 및 사용자 승인 (Settings 4섹션, 백업/복구 동작)
- [ ] `listMemoryDecisions` API는 이미 server/APIClient에 존재 (Phase 1~5에서 구현됨)

## 포함 기능

1. **앱 메뉴 정리** — File / Edit / View / Window / Help, 표준 단축키
2. **Canon Decisions History 탭** — WorkspaceView에 새 Tab 추가, 시간순 결정 리스트
3. **Engine offline empty state** — 갤러리/Workbench에 명시적 복구 액션
4. **Toast HUD** — 기존 overlay 텍스트를 autohide HUD로 격상
5. **갤러리 빈 상태 보강** — "Create work" + "Import backup" 두 액션 직접 노출
6. **Run 실패 복구** — 재시도 버튼 + 에러 상세 expand 패널

## 이 페이즈에서 하지 않는 것

- SSE 라이브 timeline → Phase 9
- NSTextView 장문 편집기 → Phase 9
- 다국어 UI → Out of Scope
- 앱 코드 사이닝/Notarization → Out of Scope

## 작업 체크리스트

### 작업 그룹 A: 앱 메뉴 정리

- [ ] **T8.A.1** — File 메뉴 commands
  - 파일: `macos/Linetta/Sources/Linetta/LinettaApp.swift`
  - 변경: `.commands { ... }` 안에
    - `CommandGroup(replacing: .newItem)`:
      - "New Work" (⌘N) → `appState.showingNewWork = true` (AppState에 published 추가)
      - "New Episode" (⌘⇧N) → 선택된 work가 있으면 EpisodeWorkbenchView로 routing + createEpisode 트리거
    - `CommandGroup(after: .newItem)`:
      - Divider
      - "Export Current Work as Markdown..." (⌘E) → 선택 work 있으면 export
      - "Import Library from Backup..." → Settings의 restore와 동일 동작 호출
  - 의존: AppState에 published intent (`@Published var pendingIntent: AppIntent?`) 도입. View들이 onChange로 소비
  - 검증: 메뉴바에서 항목 보이고 클릭 시 동작

- [ ] **T8.A.2** — Edit / View / Window 표준 메뉴 보강
  - 같은 파일
  - 추가:
    - `CommandGroup(after: .pasteboard)` — "Find in Memory..." (⌘F) → CanonMemoryView로 이동 + search field focus (AppIntent로 라우팅)
    - `CommandGroup(after: .sidebar)` — 기본 macOS 사이드바 토글이 자동 제공되도록 NavigationSplitView 유지
  - 검증: ⌘F 누르면 Memory 탭으로 이동

- [ ] **T8.A.3** — AppIntent enum 도입
  - 파일: `macos/Linetta/Sources/Linetta/AppIntent.swift` (신규)
  - 내용:
    ```swift
    enum AppIntent: Equatable {
      case openNewWork
      case openNewEpisode
      case exportCurrentWork
      case importLibrary
      case findInMemory
    }
    ```
  - AppState에 `@Published var intent: AppIntent?` 추가, 액션 후 nil로 reset
  - 각 관련 View가 `.onChange(of: appState.intent)` 으로 처리
  - 검증: 메뉴 → intent set → View가 반응

### 작업 그룹 B: Canon Decisions History

- [ ] **T8.B.1** — Decisions API 응답 모델 확인 + Models.swift 정합성
  - 파일: `macos/Linetta/Sources/LinettaCore/Models.swift`
  - 확인: `MemoryDecision` 모델이 이미 정의되어 있고 APIClient.listMemoryDecisions가 사용. id, item_id, action, actor, reason, before/after snapshot, timestamp 필드가 있는지 grep
  - 부족하면 server `memory.Decision` 구조와 맞춰 보강
  - 검증: `cd macos/Linetta && swift build`

- [ ] **T8.B.2** — `CanonDecisionsView` 신규
  - 파일: `macos/Linetta/Sources/Linetta/Views/CanonDecisionsView.swift` (신규)
  - 구조: NavigationSplitView (좌: 결정 리스트, 우: 상세)
  - 좌측: `client.listMemoryDecisions(workID:)` 결과를 timestamp 내림차순. 각 행: action badge + memory item title + actor + 상대 시간
  - 우측: action 종류, 이유, before/after diff (간단히 두 텍스트 블록 나란히)
  - 필터: action(create/update/archive), actor, kind
  - Refresh 버튼
  - 검증: 미리보기 + 실제 실행에서 결정 이력 표시

- [ ] **T8.B.3** — WorkspaceView에 "Decisions" Tab 추가
  - 파일: `macos/Linetta/Sources/Linetta/Views/WorkspaceView.swift`
  - 변경: 기존 TabView에 추가
    ```swift
    CanonDecisionsView(work: work)
      .tabItem { Label("Decisions", systemImage: "clock.arrow.circlepath") }
    ```
  - 검증: 작업실에서 Decisions 탭 확인

### 작업 그룹 C: Empty / Error State Polish

- [ ] **T8.C.1** — EngineOfflineEmptyState 컴포넌트
  - 파일: `macos/Linetta/Sources/Linetta/Views/EngineOfflineEmptyState.swift` (신규)
  - 표시 조건: `engine.status` 가 `.stopped`, `.failed`, 또는 `.starting`이 3초 이상 지속
  - 내용:
    - SF Symbol 큰 아이콘 + "Linetta engine isn't running"
    - 마지막 에러 메시지 (있으면)
    - 액션 버튼: "Start Engine" (engine.startEmbedded), "Open Settings", "Use External Engine"
  - 검증: engine 강제 kill 시 갤러리/workbench에 이 컴포넌트 표시

- [ ] **T8.C.2** — WorkGalleryView empty state 분기
  - 파일: `macos/Linetta/Sources/Linetta/Views/WorkGalleryView.swift`
  - 변경: `GalleryEmptyState`에서
    - works가 비어 있고 engine healthy → 기존 "Create or select a work" + "New Work" 버튼
    - works가 비어 있고 engine offline → `EngineOfflineEmptyState`
    - works가 있는데 healthy하지 않음 → toast로 알리고 캐시된 데이터는 계속 표시
  - 갤러리 빈 상태에 "Import Backup..." 보조 액션 추가 (intent로 Settings restore 트리거)
  - 검증: engine 끄고 새 머신 시뮬레이션 (다른 DB 경로) 후 양쪽 상태 확인

- [ ] **T8.C.3** — Toast HUD 컴포넌트
  - 파일: `macos/Linetta/Sources/Linetta/Views/ToastHUD.swift` (신규)
  - 동작:
    - `class ToastCenter: ObservableObject` — `@Published var current: Toast?`
    - `func show(_ message: String, kind: ToastKind, duration: TimeInterval = 3)`
    - `ToastKind`: info, success, warning, error (색상/아이콘)
    - 자동 autohide 후 nil로
  - AppState 또는 별도 EnvironmentObject로 주입
  - LinettaApp.swift root에 `.overlay(ToastHUD())`
  - 기존 `errorMessage` overlay 사용처(WorkGalleryView, EpisodeWorkbenchView 등) 모두 `toast.show(error.localizedDescription, kind: .error)`로 마이그레이션
  - 검증: 에러 발생 시 우측 하단 등에 자동 사라지는 HUD 표시

### 작업 그룹 D: Run 실패 복구

- [ ] **T8.D.1** — Run 실패 상세 + 재시도
  - 파일: `macos/Linetta/Sources/Linetta/Views/EpisodeWorkbenchView.swift`
  - 변경:
    - runPanel의 artifactTimelinePanel에 "Last Run Status" 영역 추가
    - 성공: 실행 시간, 산출물 수
    - 실패: 에러 메시지, stack/details expand, "Retry" 버튼 (`runAgents()` 재호출)
  - 의존: `runEpisode` 응답에 에러 정보가 들어있는지 확인 (없으면 catch에서 보존)
  - 검증: Tessera config를 일부러 잘못 만들고 run → 실패 메시지 + retry 동작

- [ ] **T8.D.2** — 진행 중 표시 미세 개선
  - 같은 파일
  - 변경: isLoading일 때 "Running agents..." 텍스트 + 시간 경과 카운터 (Timer)
  - Phase 9에서 SSE로 진짜 progress 뜰 때 대체될 placeholder
  - 검증: Run 누르면 카운터 동작

---

## ✅ Phase 8 Checkpoint

**구현 확인:**
- [ ] 모든 작업 체크박스 완료
- [ ] 앱 메뉴바에 File/Edit/View/Window/Help가 macOS 표준에 맞게 표시됨
- [ ] WorkspaceView에 4 탭 (Overview, Memory, Workbench, Decisions)
- [ ] 모든 에러 처리가 ToastCenter를 거침 (`grep -rn "errorMessage =" macos/Linetta/Sources/Linetta/` 검토)

**자동 검증:**
- [ ] Go 테스트 통과: `go test ./...`
- [ ] Swift 테스트 통과: `cd macos/Linetta && swift test`
- [ ] 사용처 검사: `grep -rn "private let client = APIClient()" macos/Linetta/Sources/` 결과 0건 (Phase 6 회귀 방지)

**수동 확인:**
- [ ] **메뉴**: ⌘N → 새 작품 시트, ⌘⇧N → 새 에피소드, ⌘E → export, ⌘F → memory 검색
- [ ] **Engine offline**: Settings에서 Stop Engine → 갤러리에 EngineOfflineEmptyState 표시 → "Start Engine" 클릭으로 복구
- [ ] **Canon decisions**: Memory에서 항목 수정 → Decisions 탭에서 변경 이력 보임 (action, actor, before/after)
- [ ] **Toast**: 일부러 잘못된 work 생성 (genre 너무 김 등) → 우측 하단 HUD 빨간색 + 3초 후 사라짐
- [ ] **Run 실패 복구**: 잘못된 config로 run 실패 → 에러 details expand + Retry 누르면 재실행

**완료 처리:**
1. 위 항목 모두 통과 시 Claude Code는 사용자에게 보고
2. 사용자 명시적 승인 후 Phase 9로 이동
3. 실패 시: 실패 항목 보고 → 원인 분석 → 수정 → 재검증

---

## 참고 자료

- 로드맵: [`linetta-macos-app-completion-roadmap.md`](./linetta-macos-app-completion-roadmap.md)
- 현재 메뉴 구성: `LinettaApp.swift:15-22` (현재는 Refresh Works 하나뿐)
- Decisions API: `internal/server/server.go:756-767` (`handleMemoryDecisions`)
- APIClient.listMemoryDecisions: `APIClient.swift:150-152`

## 메모 / 주의

- AppIntent 패턴은 SwiftUI의 menu commands → view 통신 보일러플레이트 줄이기. 대안으로 NotificationCenter도 가능하지만 typed enum이 디버깅 쉬움
- ToastCenter를 새 EnvironmentObject로 추가하면 모든 View가 inject 받아야 함 → LinettaApp.swift root에서 `.environmentObject(toastCenter)` 한 번
- Canon decisions의 before/after JSON snapshot이 클 수 있음 → diff 라이브러리 도입 대신 단순 양쪽 텍스트 비교로 충분 (이번 페이즈 기준)
- 갤러리에서 engine offline 시 캐시된 works 표시 유지 vs 비우기 결정: 캐시 유지 + toast로 알림이 사용자 편의 ↑

---
_다음 페이즈: Phase 9 — Live Run Stream + Long-form Editor → [`phase-9-live-run-and-editor.md`](./phase-9-live-run-and-editor.md)_
