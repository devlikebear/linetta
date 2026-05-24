# Phase 6: Embedded Engine Lifecycle — 작업지시서

_Swift 앱이 Go engine 프로세스를 직접 소유하도록 한다. `make macos-run` 한 번으로 두 컴포넌트가 함께 뜨고, 앱 종료 시 engine이 정리된다._

_작성일: 2026-05-24_
_속한 로드맵: [`linetta-macos-app-completion-roadmap.md`](./linetta-macos-app-completion-roadmap.md)_
_예상 소요: 6~10시간_

## 페이즈 목표

`make macos-run`만 실행하면 (1) `bin/linetta`가 빌드되고, (2) SwiftUI 앱이 뜨면서 자동으로 `bin/linetta serve --addr 127.0.0.1:0 --db ...`를 spawn하고, (3) stdout에서 실제 주소를 받아 APIClient에 주입한다. 사용자는 별도 터미널을 띄울 필요가 없다. 앱을 닫으면 engine 프로세스도 SIGTERM으로 종료된다.

## 전제 조건

- [ ] Phase 5 완료 (MVP 동작 확인됨)
- [ ] `bin/linetta serve` 명령이 정상 동작 (`make serve` 수동 확인)
- [ ] Swift 6 / macOS 15 / Xcode 16 빌드 환경
- [ ] 현재 6개 View가 `private let client = APIClient()`로 직접 인스턴스화하고 있음을 인지

## 포함 기능

1. **EngineController** — Go engine 프로세스 라이프사이클 매니저 (Swift)
2. **동적 포트 핸들링** — `--addr 127.0.0.1:0` + stdout 파싱
3. **APIClient 공유** — `AppState.client`를 단일 진실 소스로, 모든 View 마이그레이션
4. **Engine status indicator** — Toolbar에 ● 색상으로 표시 (gray=starting / green=healthy / red=down)
5. **외부 engine 모드** — Settings 토글로 spawn 비활성화 (개발자 옵션)
6. **Makefile 정비** — `macos-run`이 `build-go` 의존, bin 경로를 env로 전달

## 이 페이즈에서 하지 않는 것

- Engine 재시작/정지 버튼 UI → Phase 7
- 백업/복구 → Phase 7
- 앱 메뉴 / Empty state polish → Phase 8
- SSE 라이브 timeline → Phase 9

## 작업 체크리스트

### 작업 그룹 A: Go 서버 측 준비

- [ ] **T6.A.1** — `cmd/linetta serve`에 ready 신호 stdout 출력 추가
  - 파일: `cmd/linetta/main.go`
  - 내용:
    - 현재 `fmt.Fprintf(stderr, "linetta serve listening on http://%s\n", addr)` 가 stderr로 나가는데, Swift가 stdout 파싱하기 쉽도록 **stdout으로도 머신 읽기용 한 줄 출력**:
      `fmt.Fprintf(stdout, "LINETTA_READY addr=%s pid=%d\n", addr, os.Getpid())`
    - human 로그(stderr)는 그대로 유지
  - 검증: `bin/linetta serve --addr 127.0.0.1:0 --db /tmp/x.db` 실행 시 stdout 첫 줄이 `LINETTA_READY addr=127.0.0.1:NNNNN pid=...` 형식

- [ ] **T6.A.2** — `serve_test.go`에 ready 출력 형식 테스트 추가
  - 파일: `cmd/linetta/main_test.go` (없으면 신규)
  - 내용: serveOptions의 `ready` 채널을 활용하는 기존 패턴(`opts.ready`)을 살리되, **stdout 출력 형식의 정규식**(`^LINETTA_READY addr=127\.0\.0\.1:\d+ pid=\d+$`)을 단위 테스트로 박는다
  - 검증: `go test ./cmd/linetta/...`

### 작업 그룹 B: Swift EngineController 구현

- [ ] **T6.B.1** — `EngineController` 신규 추가 (LinettaCore)
  - 파일: `macos/Linetta/Sources/LinettaCore/EngineController.swift`
  - 타입: `public final class EngineController: ObservableObject, @unchecked Sendable`
  - 상태: `enum EngineStatus { case stopped, starting, healthy, failed(String), external }`
  - 공개 API:
    - `@MainActor @Published public var status: EngineStatus`
    - `@MainActor @Published public var address: URL?`  ← APIClient base URL 소스
    - `public func startEmbedded(binaryPath: URL, dbPath: URL) async throws`
    - `public func attachExternal(address: URL)` (Settings 토글에서 사용)
    - `public func stop() async`
  - 내부:
    - `Process` 생성, `standardOutput = Pipe()`, `standardError = Pipe()`
    - 첫 stdout 줄을 정규식 `^LINETTA_READY addr=(.+) pid=(\d+)$`로 파싱 → `address` set + `status = .healthy`
    - stdout/stderr 후속 라인은 ring buffer(최근 200줄)에 보관 → Phase 7 log tail에서 사용
    - Process 종료 감지 시 `status = .failed(reason)` + `address = nil`
    - `stop()`은 SIGTERM → 1.5초 대기 → SIGKILL fallback
  - 참조: Foundation `Process` API. ProcessInfo의 `environment` 그대로 전달, `currentDirectoryURL`은 사용자 home으로
  - 검증: 단위 테스트 `EngineControllerTests`에서 echo 스크립트로 ready 라인 검출 시뮬레이션 (Bash trampoline 사용)

- [ ] **T6.B.2** — Engine 바이너리 경로 탐색 헬퍼
  - 파일: `macos/Linetta/Sources/LinettaCore/EngineController.swift` (같은 파일에 helper로)
  - 함수: `public static func resolveBinaryPath(override: String?) -> URL?`
  - 탐색 순서:
    1. override 인자 (Settings에서 수동 지정 가능)
    2. 환경변수 `LINETTA_BIN`
    3. `Bundle.main.bundleURL/../bin/linetta` (.app 번들 옆 — 미래 대비)
    4. `$PWD/bin/linetta` (개발 시: `swift run` 작업 디렉토리는 macos/Linetta이므로 `../../bin/linetta`도 시도)
    5. `which linetta` (PATH 탐색)
  - 검증: `EngineControllerTests`에서 임시 디렉토리에 mock binary 만들고 각 케이스 확인

- [ ] **T6.B.3** — 데이터 디렉토리 / DB 경로 결정 헬퍼
  - 파일: `macos/Linetta/Sources/LinettaCore/StoragePaths.swift` (신규)
  - 내용: `public enum StoragePaths { static var defaultDB: URL { ~/.linetta/linetta.db }, static var dataDir: URL { ~/.linetta } }`
  - 기존 Go `defaultDBPath()`(cmd/linetta/main.go:325)와 동일한 경로 결정 로직 (Swift 쪽 일관성)
  - 검증: 단위 테스트로 경로 형식 확인

### 작업 그룹 C: APIClient 공유 마이그레이션

- [ ] **T6.C.1** — `AppState`를 단일 client owner로 격상
  - 파일: `macos/Linetta/Sources/Linetta/AppState.swift`
  - 변경:
    - `@Published var client: APIClient` 추가 (private이 아닌 public read, internal write)
    - `init(engine: EngineController)` — engine.address 변경을 Combine sink로 받아 client 재생성
    - 기존 `init(client: APIClient = APIClient())`는 제거 (Engine이 진실 소스)
  - 검증: 빌드 통과

- [ ] **T6.C.2** — `LinettaApp.swift`에서 EngineController/AppState 주입
  - 파일: `macos/Linetta/Sources/Linetta/LinettaApp.swift`
  - 변경:
    - `@StateObject var engine = EngineController()`
    - `@StateObject var appState = AppState(engine: engine)`
    - `WorkGalleryView().environmentObject(engine).environmentObject(appState)`
    - `applicationDidFinishLaunching`에서:
      - Settings의 `useExternalEngine` 읽기 (UserDefaults)
      - false면 `Task { try await engine.startEmbedded(binaryPath: ..., dbPath: ...) }`
      - true면 `engine.attachExternal(address: APIClient.defaultBaseURL)`
    - `applicationWillTerminate`에서 `await engine.stop()`
  - 검증: `make build && make macos-run` → 앱 실행되면 engine 프로세스 떠야 함 (Activity Monitor 또는 `pgrep -lf "bin/linetta serve"`)

- [ ] **T6.C.3** — 6개 View의 `APIClient()` 직접 사용을 제거
  - 파일들 (모두 동일 패턴):
    - `Views/WorkGalleryView.swift` (WorkRow의 `private let client = APIClient()`)
    - `Views/WorkspaceView.swift` (WorkOverview)
    - `Views/EpisodeWorkbenchView.swift`
    - `Views/CanonMemoryView.swift`
    - `Views/SettingsView.swift` (있다면)
  - 변경: `@EnvironmentObject private var appState: AppState` 추가 후 `appState.client.<method>` 호출
  - **주의**: `WorkRow`는 `List`의 행이라 EnvironmentObject 자동 주입됨. 별도 처리 불필요
  - 검증: grep으로 `APIClient()` 직접 인스턴스화가 0건이어야 함: `grep -rn "APIClient()" macos/Linetta/Sources/Linetta/`

### 작업 그룹 D: Status indicator + Toolbar

- [ ] **T6.D.1** — `EngineStatusBadge` 컴포넌트
  - 파일: `macos/Linetta/Sources/Linetta/Views/EngineStatusBadge.swift` (신규)
  - 내용:
    - `@EnvironmentObject var engine: EngineController`
    - Circle (8pt) + label, status별 색상: stopped=gray, starting=yellow(pulse animation), healthy=green, failed=red, external=blue
    - 클릭 시 tooltip 또는 popover로 현재 address, pid, 최근 로그 5줄 표시
  - 검증: 미리보기에서 5가지 status 모두 렌더 확인

- [ ] **T6.D.2** — `WorkGalleryView` toolbar에 badge 배치
  - 파일: `macos/Linetta/Sources/Linetta/Views/WorkGalleryView.swift`
  - 변경: 기존 toolbar의 `ToolbarItemGroup` 뒤에 `ToolbarItem(placement: .status) { EngineStatusBadge() }` 추가
  - 검증: 실행 시 우측 상단에 ● 표시

### 작업 그룹 E: Makefile 및 외부 engine 모드

- [ ] **T6.E.1** — `Makefile.macos-run` 타겟이 Go 빌드를 선행하도록 변경
  - 파일: `Makefile`
  - 변경:
    ```make
    macos-run: build-go ## Run the SwiftUI macOS app with embedded engine.
    	LINETTA_BIN=$(PWD)/$(BIN) cd $(MACOS_DIR) && swift run Linetta
    ```
  - 내용: env로 절대 경로 전달 → EngineController.resolveBinaryPath의 #2번 룰에 매칭
  - 검증: `make macos-run` → 빌드 후 앱 실행, engine 프로세스 자동 spawn

- [ ] **T6.E.2** — SettingsView에 "Use external engine" 토글
  - 파일: `macos/Linetta/Sources/Linetta/Views/SettingsView.swift`
  - 변경:
    - `@AppStorage("linetta.useExternalEngine") private var useExternalEngine = false` 추가
    - Engine section에 Toggle 추가. 도움말 텍스트: "When enabled, Linetta will not spawn its own engine. Run `make serve` separately."
  - 주의: 이번 페이즈는 토글 값 저장만. 토글 변경 시 즉시 engine 재시작은 Phase 7에서 처리 (현재는 앱 재시작 필요 안내)
  - 검증: 토글 후 앱 재시작 시 spawn 동작 변화

- [ ] **T6.E.3** — README / docs 업데이트
  - 파일: `README.md`, `docs/plan/README.md`
  - 변경:
    - README의 "Run" 섹션을 단순화: `make macos-run` 한 줄로 충분함을 강조. `make serve` 별도 실행은 외부 engine 모드 사용자만 필요
    - docs/plan/README.md의 읽는 순서 끝에 Phase 6~9 링크 추가
  - 검증: 사람이 읽어서 명확하면 통과

---

## ✅ Phase 6 Checkpoint

**구현 확인:**
- [ ] 모든 작업 체크박스 완료
- [ ] `grep -rn "APIClient()" macos/Linetta/Sources/Linetta/` 결과 0건
- [ ] `EngineController.status`가 stopped→starting→healthy로 전이됨
- [ ] App 종료 시 engine 프로세스가 사라짐

**자동 검증:**
- [ ] Go 테스트 통과: `go test ./...`
- [ ] Swift 테스트 통과: `cd macos/Linetta && swift test`
- [ ] Go vet 통과: `go vet ./...`

**수동 확인:**
- [ ] 콜드 스타트: `make clean && make macos-run` → 앱이 뜨고 갤러리에 works가 보임 (engine 별도 실행 안 함)
- [ ] 종료 정리: 앱 닫은 후 `pgrep -lf "bin/linetta serve"` 결과 비어 있음
- [ ] Status badge: toolbar에서 ● green 보임. Engine 강제 kill (`pkill -f "bin/linetta serve"`) 시 badge가 red로 전이
- [ ] 외부 engine 모드: Settings에서 toggle ON → 앱 재시작 → 직접 띄운 `make serve`에 연결됨

**완료 처리:**
1. 위 항목 모두 통과 시 Claude Code는 사용자에게 보고:
   - 완료된 작업 요약
   - 자동 검증 결과
   - 수동 확인 항목 결과 (특히 콜드 스타트와 종료 정리)
2. 사용자 명시적 승인 ("Phase 6 완료, 다음 진행") 후 Phase 7로 이동
3. 실패 시: 실패 항목 보고 → 원인 분석 → 수정 → 재검증

---

## 참고 자료

- 로드맵: [`linetta-macos-app-completion-roadmap.md`](./linetta-macos-app-completion-roadmap.md)
- 현재 Makefile `macos-run` 타겟: `Makefile:45-46`
- 현재 serve 진입점: `cmd/linetta/main.go:157-196`
- 현재 APIClient 초기화: `macos/Linetta/Sources/LinettaCore/APIClient.swift:5-12`

## 메모 / 주의

- `Process`의 stdout pipe를 비우지 않으면 4KB 버퍼가 차서 hang 위험. EngineController는 별도 Task로 지속 읽기 필요
- `swift run`은 `.app` 번들 없이 실행 → Bundle.main.bundleURL이 의미 없음. T6.B.2의 #3 룰은 미래 배포 대비 placeholder
- macOS Sandbox 안 켜져 있음(Package.swift에 entitlements 없음) → 임의 binary 실행 OK. 만약 나중에 Sandbox 켜면 spawn 막힘 — 그땐 별도 처리
- SIGTERM 후 1.5초는 경험적 값. Go server의 Shutdown timeout이 2초(main.go:181)라서 충분
- `useExternalEngine` 토글의 즉시 반영은 Phase 7에서 처리 — 이번엔 "앱 재시작 필요" 안내만

---
_다음 페이즈: Phase 7 — Settings Studio → [`phase-7-settings-studio.md`](./phase-7-settings-studio.md)_
