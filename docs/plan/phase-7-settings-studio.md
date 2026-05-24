# Phase 7: Settings Studio — 작업지시서

_Settings 한 탭에서 Linetta 운영(engine 제어, 데이터 관리, config 편집)이 모두 가능하도록 보강한다. CLI에만 있던 백업/복구를 HTTP API로 노출하고 GUI를 붙인다._

_작성일: 2026-05-24_
_속한 로드맵: [`linetta-macos-app-completion-roadmap.md`](./linetta-macos-app-completion-roadmap.md)_
_예상 소요: 8~12시간_

## 페이즈 목표

Settings를 4 섹션(Engine / Storage / Tessera / About)으로 재구성한다. Engine 섹션은 EngineController를 직접 제어하고, Storage는 백업/복구를 GUI로 수행하며, Tessera는 config 파일을 직접 편집할 수 있게 한다. 결과적으로 사용자는 터미널 없이 Linetta의 거의 모든 운영 작업을 할 수 있다.

## 전제 조건

- [ ] Phase 6 완료 및 사용자 승인 (EngineController, AppState.client, EngineStatusBadge 동작)
- [ ] CLI `linetta export-library`, `linetta import-library` 동작 확인 (`make export-library`, `make import-library`)

## 포함 기능

1. **Settings 4 섹션 재구조화** (TabView 또는 Form sections)
2. **Engine 섹션**: status display, restart, stop, log tail, "Use external engine" 즉시 반영
3. **Storage 섹션**: DB path, Reveal in Finder, 백업 만들기 (Save dialog), 복원하기 (Open dialog)
4. **Tessera 섹션**: config path picker, monospace YAML 편집기, basic syntax check, provider secret 가이드 링크
5. **About 섹션**: 버전 정보, log directory, 외부 링크
6. **신규 HTTP API**: `POST /api/library/backup`, `POST /api/library/restore`
7. **`internal/library` 패키지 추출**: CLI와 HTTP가 공유하는 backup/restore 로직

## 이 페이즈에서 하지 않는 것

- 앱 메뉴(File/Edit/View) 정리 → Phase 8
- Canon decisions history view → Phase 8
- SSE 라이브 timeline → Phase 9
- Tessera config의 schema 수준 검증 → Out of Scope (basic YAML parse만)
- Keychain 통합 provider secret 관리 → Out of Scope (가이드만 제공)

## 작업 체크리스트

### 작업 그룹 A: `internal/library` 패키지 추출

- [ ] **T7.A.1** — backup/restore 코어 로직을 패키지로 분리
  - 파일: `internal/library/backup.go` (신규)
  - 함수:
    - `func Export(dbPath, configPath, outPath string) error` — 현재 `cmd/linetta/main.go:237-261` (`runExportLibrary`)의 zip 작성 로직 그대로 이동
    - `func Import(inPath, dbPath, configOut string, force bool) error` — `runImportLibrary` 로직 이동
    - `addZipFile`, `extractZipFile`는 unexported helper로 이동
  - 검증: 단위 테스트 `internal/library/backup_test.go` — 임시 디렉토리에 export → import → 파일 동일성 비교

- [ ] **T7.A.2** — `cmd/linetta` CLI에서 새 패키지 호출
  - 파일: `cmd/linetta/main.go`
  - 변경: `runExportLibrary` / `runImportLibrary` 본문을 `library.Export(...)`, `library.Import(...)` 호출로 교체
  - 기존 함수 signature와 동작 보존 (회귀 방지)
  - 검증: `go test ./cmd/linetta/...`, 그리고 수동: `make export-library && make import-library`

### 작업 그룹 B: HTTP backup/restore API

- [ ] **T7.B.1** — server.go에 `/api/library/*` 라우트 추가
  - 파일: `internal/server/server.go`
  - `routes()`에 추가:
    - `s.mux.HandleFunc("/api/library/backup", s.handleLibraryBackup)`
    - `s.mux.HandleFunc("/api/library/restore", s.handleLibraryRestore)`
    - `s.mux.HandleFunc("/api/library/info", s.handleLibraryInfo)` (DB path, size, last modified, work count)
  - **주의**: 현재 Server 구조체에 dbPath가 없음. `Options`에 `DBPath string` 추가하고 `cmd/linetta` serve가 전달
  - 검증: `go test ./internal/server/...`

- [ ] **T7.B.2** — handleLibraryBackup 구현
  - 파일: `internal/server/server.go`
  - 동작:
    - POST 요청 받음. body는 `{"include_config": true/false, "config_path": "..."}` (옵셔널)
    - response: `application/zip` 바이너리 직접 스트림 (메모리 안 거치게 임시 파일 → io.Copy)
    - Content-Disposition: `attachment; filename="linetta-backup-YYYYMMDD-HHMMSS.zip"`
    - 임시 파일은 `os.MkdirTemp` + defer cleanup
  - 검증: 단위 테스트 — POST 후 zip 헤더(`PK\x03\x04`) 검사

- [ ] **T7.B.3** — handleLibraryRestore 구현
  - 파일: `internal/server/server.go`
  - 동작:
    - POST `multipart/form-data` 받음 (`file` 필드에 zip, `force` 필드)
    - 임시 디렉토리에 풀고 library.Import 호출
    - **중요**: 복원은 현재 실행 중인 server가 DB 핸들을 쥐고 있어 충돌 가능 → 응답 후 server가 자기 자신을 graceful restart 해야 함. 대안: 복원 결과만 별도 경로에 만들고 사용자에게 "복원 완료, 앱 재시작 필요" 안내
    - **이번 페이즈는 후자 채택**: 복원은 새 DB 경로에 풀고 응답에 `{"restored_db_path": "..."}` 반환. UI가 "재시작 필요" 안내
  - 검증: 단위 테스트로 zip 업로드 + restore 결과 확인

- [ ] **T7.B.4** — handleLibraryInfo 구현
  - 파일: `internal/server/server.go`
  - response JSON: `{"db_path": "...", "db_size_bytes": N, "work_count": N, "last_modified": "RFC3339"}`
  - 검증: 단위 테스트

### 작업 그룹 C: APIClient 확장

- [ ] **T7.C.1** — `LibraryAPI` 메서드 추가
  - 파일: `macos/Linetta/Sources/LinettaCore/APIClient.swift`
  - 추가:
    - `public func libraryInfo() async throws -> LibraryInfo`
    - `public func downloadBackup(includeConfig: Bool, configPath: String?) async throws -> Data` (zip 바이너리)
    - `public func uploadRestore(zipData: Data, force: Bool) async throws -> RestoreResult` (multipart)
  - 새 모델: `Models.swift`에 `LibraryInfo`, `RestoreResult` 추가
  - 검증: 빌드 통과 + 가능하면 mock URLSession으로 단위 테스트

### 작업 그룹 D: Settings 4 섹션 재구조화

- [ ] **T7.D.1** — Settings 컨테이너를 TabView로 전환
  - 파일: `macos/Linetta/Sources/Linetta/Views/SettingsView.swift` (전면 개편)
  - 구조:
    ```swift
    TabView {
      EngineSettingsView().tabItem { Label("Engine", systemImage: "bolt.fill") }
      StorageSettingsView().tabItem { Label("Storage", systemImage: "externaldrive") }
      TesseraSettingsView().tabItem { Label("Tessera", systemImage: "wand.and.stars") }
      AboutSettingsView().tabItem { Label("About", systemImage: "info.circle") }
    }
    .frame(width: 560, height: 480)
    ```
  - 각 섹션은 별도 파일로 분리: `Views/Settings/EngineSettingsView.swift` 등
  - 검증: Settings 창에서 4 탭 모두 클릭 가능

- [ ] **T7.D.2** — EngineSettingsView
  - 파일: `macos/Linetta/Sources/Linetta/Views/Settings/EngineSettingsView.swift` (신규)
  - 요소:
    - 현재 status (`EngineStatusBadge` 재사용) + address + pid
    - Restart button (engine.stop() → engine.startEmbedded(...))
    - Stop button (engine.stop())
    - "Use external engine" toggle (즉시 반영: ON 시 stop + attachExternal, OFF 시 startEmbedded)
    - "External engine address" TextField (toggle ON일 때만 enabled, `@AppStorage`)
    - Log tail: EngineController가 보관 중인 최근 50줄을 monospace로 표시 (auto refresh 1초마다)
  - 검증: Restart 버튼 누르면 status starting → healthy 전이

- [ ] **T7.D.3** — StorageSettingsView
  - 파일: `macos/Linetta/Sources/Linetta/Views/Settings/StorageSettingsView.swift` (신규)
  - 요소:
    - LibraryInfo 표시 (DB path, size, work count, last modified) — onAppear에서 `client.libraryInfo()` 호출
    - "Reveal in Finder" 버튼 → `NSWorkspace.shared.selectFile(...)`
    - "Create Backup" 버튼:
      - `NSSavePanel`로 저장 위치 선택 (기본 파일명: `linetta-backup-YYYYMMDD-HHMMSS.zip`)
      - `client.downloadBackup(...)` → `try Data.write(to:)`
      - 성공 시 toast (간단히 message text로)
    - "Restore from Backup" 버튼:
      - `NSOpenPanel`로 zip 선택
      - 경고 alert: "This will replace your library. Continue?"
      - `client.uploadRestore(...)` → 응답 받고 "Restart Linetta to use the restored library" alert
  - 검증: 백업 만들기 → zip 파일 생성 확인 (`unzip -l`로 library.db 포함)

- [ ] **T7.D.4** — TesseraSettingsView
  - 파일: `macos/Linetta/Sources/Linetta/Views/Settings/TesseraSettingsView.swift` (신규)
  - 요소:
    - Config path picker (TextField + "Choose..." 버튼 → NSOpenPanel)
    - "Open in Editor" 버튼 → `NSWorkspace.shared.open(url)` (외부 에디터)
    - **Inline editor**: monospace TextEditor (`.font(.system(.body, design: .monospaced))`), 파일 내용 로드/저장
    - "Validate" 버튼: 기본 YAML 파싱만 (`Yams` 의존성 추가 또는 간단히 `Process`로 `yq` 호출. Yams 권장)
    - "Reset to Default" 버튼: 기존 `defaultTesseraConfig` 문자열로 덮어쓰기 (현재 SettingsView에 있는 string literal 재사용)
    - Provider secret 가이드 링크 (`docs/` 또는 외부 URL)
  - 의존성: `Yams` 추가. Package.swift에 `.package(url: "https://github.com/jpsim/Yams", from: "5.1.0")`, target dependencies에 `.product(name: "Yams", package: "Yams")`
  - 검증: 잘못된 YAML 입력 시 Validate가 에러 메시지 표시

- [ ] **T7.D.5** — AboutSettingsView
  - 파일: `macos/Linetta/Sources/Linetta/Views/Settings/AboutSettingsView.swift` (신규)
  - 요소:
    - 앱 이름, 버전 (`Bundle.main.infoDictionary?["CFBundleShortVersionString"]`)
    - Build date (CI 환경변수 또는 빌드 시 주입 — 이번엔 placeholder)
    - "Reveal data directory" 버튼 (`~/.linetta`)
    - "Reveal log directory" 버튼 (Phase 6에서 결정한 로그 경로 — 없으면 stderr 안내)
    - GitHub 링크 (`https://github.com/devlikebear/linetta`)
    - Tessera version (Go 쪽 `/health`를 확장해서 version 반환 — 또는 build time embed)
  - 검증: 모든 버튼이 안전하게 동작 (없는 디렉토리 클릭 시 alert)

### 작업 그룹 E: 통합

- [ ] **T7.E.1** — 기존 `defaultTesseraConfig` 문자열을 재사용 가능한 위치로 이동
  - 파일: `macos/Linetta/Sources/LinettaCore/TesseraDefaults.swift` (신규)
  - 내용: `public let defaultTesseraConfig = "..."` (현재 SettingsView.swift 끝 부분)
  - 기존 SettingsView에서 삭제
  - 검증: 빌드

- [ ] **T7.E.2** — `cmd/linetta serve`가 server.Options.DBPath 전달
  - 파일: `cmd/linetta/main.go:157-196` (runServe)
  - 변경: `server.Options{Memory: ..., Agent: ..., DBPath: opts.DBPath}`
  - 검증: `go test ./cmd/linetta/...`

---

## ✅ Phase 7 Checkpoint

**구현 확인:**
- [ ] 모든 작업 체크박스 완료
- [ ] Settings 창이 4 탭으로 표시됨
- [ ] `internal/library` 패키지가 CLI와 HTTP 둘 다에서 사용됨 (코드 중복 없음)

**자동 검증:**
- [ ] Go 테스트 통과: `go test ./...`
- [ ] Swift 테스트 통과: `cd macos/Linetta && swift test`
- [ ] `internal/library` 패키지 단독 테스트: `go test ./internal/library/...`
- [ ] `internal/server` HTTP backup/restore 테스트 통과

**수동 확인:**
- [ ] **백업 E2E**: Settings → Storage → Create Backup → 저장 위치 선택 → zip 생성 → `unzip -l linetta-backup-*.zip`에 `library.db` 포함
- [ ] **복원 E2E**: 위 zip을 Restore → 알림 따라 앱 재시작 → 갤러리에 동일 works 보임
- [ ] **Engine restart**: Settings → Engine → Restart → status가 starting→healthy 전이, address 갱신
- [ ] **External engine toggle**: ON → 자체 spawn 정지, 외부 `make serve`와 연결. OFF → 다시 자체 spawn
- [ ] **Tessera config 편집**: 잘못된 YAML 입력 시 Validate가 에러 표시, 올바른 YAML 저장 후 다음 Run Agents가 새 설정으로 동작

**완료 처리:**
1. 위 항목 모두 통과 시 Claude Code는 사용자에게 보고
2. 사용자 명시적 승인 후 Phase 8로 이동
3. 실패 시: 실패 항목 보고 → 원인 분석 → 수정 → 재검증

---

## 참고 자료

- 로드맵: [`linetta-macos-app-completion-roadmap.md`](./linetta-macos-app-completion-roadmap.md)
- 현재 export/import CLI: `cmd/linetta/main.go:237-323`
- 현재 SettingsView: `macos/Linetta/Sources/Linetta/Views/SettingsView.swift`
- Yams: https://github.com/jpsim/Yams

## 메모 / 주의

- Restore는 DB 핸들 충돌 때문에 in-place로 못함 → 새 경로로 풀고 "재시작 필요" 안내. 자동 재시작은 Phase 9에서 EngineController.restart로 가능하지만 이번엔 사용자 액션으로 충분
- Yams는 macOS only일 수도 → Package.swift에서 platforms: .macOS(.v15)이므로 OK
- NSSavePanel/NSOpenPanel은 SwiftUI의 fileImporter/fileExporter modifier로도 가능. 일관성 위해 fileImporter 우선
- backup HTTP 응답을 메모리에 다 올리면 대용량 DB에 부담 → 임시 파일 + io.Copy 스트림
- Tessera config 편집 시 file watcher는 도입하지 않음 (사용자가 외부에서 수정해도 다음 열 때 다시 로드)

---
_다음 페이즈: Phase 8 — App Polish & Workflow Completion → [`phase-8-app-polish-workflow.md`](./phase-8-app-polish-workflow.md)_
