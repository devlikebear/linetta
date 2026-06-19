# Linetta Folder Sync (iCloud / Google Drive 폴더 동기화) 설계

- 작성일: 2026-06-19
- 상태: 설계 승인됨 (사용자 리뷰 대기)
- 대상: Linetta 데스크톱 앱 (Tauri v2 + Go 사이드카), 전 빌드 (Developer ID / MAS / Linux / Windows)
- 관련 선행:
  - 기존 Git Sync (마크다운 export → git 디렉터리 commit/push, `!mas` 전용)
  - [Phase 2-A — App Sandbox 호환화](2026-06-17-macos-app-sandbox-mas-design.md)
  - [Phase 2-B — Mac App Store 제출](2026-06-18-mac-app-store-submission-design.md)

## 배경

Git Sync은 프로젝트를 마크다운으로 export 한 뒤 지정 git 디렉터리에 commit/push 한다.
하지만 git에 의존하고, MAS(샌드박스) 빌드에서는 빌드 태그로 완전히 제외되어 있다.

Folder Sync은 동일한 "마크다운 단방향 내보내기" 모델을, git 대신 **사용자가 지정한
클라우드 폴더**(iCloud Drive / Google Drive 동기화 폴더 / 임의 폴더)에 파일로 쓰는
방식으로 제공한다. OS(iCloud) 또는 데스크톱 클라이언트(Google Drive)가 실제 업로드를
담당한다. **전 빌드에서 동작**하며, 특히 MAS 사용자에게 Git Sync을 대체하는 sync 수단이
된다.

## 결정 사항 (확정)

- **동기화 모델**: 단방향 마크다운 내보내기 (Git Sync과 동일). 양방향/충돌해결/DB 백업 아님.
- **업로드 방식**: 폴더 방식 통일. iCloud/Google Drive 모두 "사용자가 고른 폴더"로 취급.
  OS/데스크톱 클라이언트가 업로드 담당. CloudKit/OAuth/REST API 미사용.
- **대상 빌드**: 전 빌드 (Developer ID, MAS, Linux, Windows).
- **트리거**: 자동 일일(엔진 스케줄러 재사용) + 수동 "지금 내보내기" 버튼. Git Sync과 동등.
- **전달 아키텍처 (접근법 A)**: 샌드박스(MAS)에서는 엔진이 컨테이너 내부 staging에 export
  하고, security-scoped bookmark를 가진 Tauri(Rust) 프로세스가 대상 폴더로 복사한다.
- **대상 폴더 수**: v1은 **단일 폴더 1개**(Git Sync과 동일). 사용자는 iCloud 또는 GDrive
  또는 임의 폴더 중 하나를 고른다. 동시 다중 타깃은 보류(YAGNI).

## 목표 / 비범위

**목표**: 사용자가 설정 화면에서 폴더를 한 번 지정하면, 매일 자동으로(그리고 수동 버튼으로)
모든 비-아카이브 프로젝트의 마크다운이 그 폴더에 기록된다. MAS 빌드 포함 전 빌드에서 동작.

**비범위**:
- 양방향 동기화 / 충돌 해결 / 머지
- SQLite DB 전체 백업·복원
- OAuth / Google Drive REST API / CloudKit 직접 통합
- 동시 다중 폴더 타깃
- CI 연동 (후속)

---

## 핵심 사실 (조사 결과)

- **출시 Developer ID 빌드는 비샌드박스**다. 베이스 `apps/desktop/src-tauri/tauri.conf.json`
  에 `bundle.macOS.entitlements`가 없어 sandbox 엔타이틀먼트가 적용되지 않는다
  (`linetta.entitlements`는 로컬 테스트용으로만 존재, 빌드에 미연결). Git Sync이 DevID
  Homebrew 빌드에서 동작하는 것과 일치. 따라서 **샌드박스 제약은 MAS 빌드에만** 해당된다.
- **Git Sync export 로직 재사용**: `export.ExportProject()`가 프로젝트별 마크다운을
  생성한다. Folder Sync도 이를 그대로 쓴다.
- **Tauri 플러그인 기존 구비**: `tauri-plugin-dialog`(폴더 피커), `tauri-plugin-fs`,
  `tauri-plugin-opener` 설치됨. `capabilities/default.json`에 `dialog:allow-open`,
  `fs:allow-write-text-file` 권한 존재. 폴더 피커는 `Settings.tsx`의 git_sync_dir에서
  이미 `openDialog({ directory: true })` 패턴 사용 중.
- **security-scoped bookmark는 net-new**. 코드베이스에 NSURL bookmark / ObjC FFI 선례
  없음. `com.apple.security.files.bookmarks.app-scope` 엔타이틀먼트 미존재.
- **엔진↔Tauri 알림 라우팅 존재**: `engine.rs`가 엔진의 JSON-RPC 알림(`ai.delta` 등)을
  Tauri 이벤트로 라우팅한다. deliver 신호를 여기에 추가한다.
- **MAS conf 버그**: `tauri.mas.conf.json`이 sandbox 엔타이틀먼트로 `linetta.entitlements`
  (DevID 테스트용, application-identifier 없음)를 가리킨다. `linetta-mas.entitlements`를
  가리켜야 맞다. 본 작업에서 함께 수정한다.

---

## 아키텍처 & 데이터 흐름

기능 이름: **Folder Sync**. 빌드에 따라 **전달(delivery) 단계만** 달라진다(기존 `mas`
빌드 태그 패턴 그대로). 마크다운 생성 로직은 양 경로 공통.

### 비-MAS (DevID-mac 비샌드박스 / Linux / Windows)

```
엔진 스케줄러(일일) 또는 수동 RPC(folder_sync.run)
  → 엔진: 비-아카이브 프로젝트 목록
  → 각 프로젝트 export → 대상 폴더에 직접 파일 쓰기
  → ops_status(job=folder_sync) 기록. 끝. (Tauri 개입·북마크 없음)
```

### MAS (샌드박스)

**Tauri(Rust)가 오케스트레이션한다.** 엔진은 staging export(`folder_sync.stage`)와
ops 기록(`folder_sync.report`)만 제공하고, 스케줄링/복사는 Rust가 담당한다. 엔진→Tauri
알림(notification) 메커니즘은 쓰지 않는다.

```
일일 타이머(Rust, 앱 시작 시 + 24h 주기) 또는 수동 버튼
  → Rust: settings 확인(enabled? dir?) — 비활성/미설정이면 no-op
  → Rust → 엔진 `folder_sync.stage`: 각 프로젝트 export → 컨테이너 staging 폴더에 쓰기
      → {staging_dir, files[]} 반환
  → Rust: 저장된 security-scoped bookmark 해제 → startAccessingSecurityScopedResource
      → staging → 대상 폴더 복사 → stopAccessing
  → Rust → 엔진 `folder_sync.report` {started_at, finished_at, ok, files_copied, error}
      → 엔진: ops_status(job=folder_sync) 기록
```

엔진의 일일 스케줄러는 MAS 빌드에서 folder sync를 돌리지 않는다(no-op `dailySyncer`).
staging만 하고 전달이 안 되는 상황을 막기 위해 Rust 타이머가 유일한 구동원이다.

핵심 원칙: **샌드박스 밖 쓰기는 북마크를 가진 Tauri 프로세스에서만** 수행한다(접근법 A).

---

## 컴포넌트 & 파일 구조

### 엔진 (Go)

- `engine/internal/foldersync/foldersync.go` — `Syncer`. `gitsync.go`와 평행 구조.
  공통 헬퍼 `exportAll(ctx, destDir) (filesWritten int, err error)`로 비-아카이브 프로젝트를
  export. `RunOnce(ctx) (ResultSummary, error)` = `exportAll(FolderSyncDir)` 직접 쓰기 +
  ops 기록(비-MAS용). `Stage(ctx) (StageResult, error)` = `exportAll(stagingDir)` + 경로 목록
  반환(MAS용). `Report(ctx, ReportInput) error` = ops 기록(MAS 완료 후).
  `ResultSummary{Skipped, FilesWritten, FilesCopied, Message, Error}`.
- `engine/cmd/linetta-engine/foldersync_direct.go` (`//go:build !mas`) — `setupFolderSync`가
  `folder_sync.run` 핸들러(→ RunOnce 직접 쓰기) 등록, 실제 `dailySyncer` 반환(엔진 스케줄러가
  일일 구동).
- `engine/cmd/linetta-engine/foldersync_staged.go` (`//go:build mas`) — `setupFolderSync`가
  `folder_sync.stage`(→ Stage)·`folder_sync.report`(→ Report) 핸들러 등록, **no-op**
  `dailySyncer` 반환(Rust 타이머가 구동하므로 엔진 스케줄러는 folder sync 미실행).
- `engine/cmd/linetta-engine/sync.go` — 스케줄러가 git sync + folder sync 둘 다 순차
  실행하도록 `dailySyncer`를 다중화(현재 단일 syncer → 여러 syncer 순차 호출).
- `engine/internal/rpc/handlers/foldersync.go` — `RunFolderSync`(수동, 비-MAS),
  `StageFolderSync`·`ReportFolderSync`(MAS). 핸들러 본문은 빌드 태그 setup에서 등록 여부 결정.
- `engine/internal/settings/settings.go` — `Config`에 `FolderSyncDir string`,
  `FolderSyncEnabled bool` 추가. `Patch`에 대응 필드 + 검증.
- `engine/internal/opsstatus/` — job 상수 `JobFolderSync = "folder_sync"` 추가.

### Tauri (Rust)

- `apps/desktop/src-tauri/src/folder_sync.rs` — 커맨드 `set_folder_sync_dir(path)`(항상 엔진
  settings에 경로 저장; MAS 빌드면 추가로 북마크 생성·저장)와 `folder_sync_now()`
  (비-MAS: 엔진 `folder_sync.run` 포워드 / MAS: settings 확인 → `folder_sync.stage` →
  북마크로 staging→대상 복사 → `folder_sync.report`). 복사 헬퍼 + (MAS) 일일 타이머.
- `apps/desktop/src-tauri/src/macos_bookmarks.rs` (`#[cfg(all(target_os="macos", feature="mas"))]`)
  — ObjC FFI로 `bookmarkData(options: .withSecurityScope)` 생성, resolve +
  `startAccessingSecurityScopedResource`/`stopAccessingSecurityScopedResource`. 북마크 blob은
  앱 컨테이너 내 파일(`folder-sync.bookmark`)에 저장.
- `apps/desktop/src-tauri/src/lib.rs` — `set_folder_sync_dir`·`folder_sync_now` 커맨드 등록
  (`generate_handler!`), (MAS) 일일 타이머 spawn. 엔진 알림 라우팅 변경 없음.
- `apps/desktop/src-tauri/Cargo.toml` — cargo feature `mas` 추가(+ ObjC FFI용 `objc2`/`block2`
  등 크레이트, MAS 빌드에서만). `objc2` 계열 사용.
- MAS 빌드 배선:
  - `scripts/release-mas-local.sh` — `pnpm tauri build`에 `--features mas` 전달.
  - `apps/desktop/src-tauri/entitlements/linetta-mas.entitlements` —
    `com.apple.security.files.bookmarks.app-scope` = true 추가.
  - `apps/desktop/src-tauri/tauri.mas.conf.json` — sandbox 엔타이틀먼트를
    `entitlements/linetta-mas.entitlements`로 수정(현재 `linetta.entitlements` 가리킴).

### 프론트엔드

- `apps/desktop/src/lib/rpc.ts` — `folderSyncNow()`·`setFolderSyncDir(path)`(Tauri invoke),
  settings patch에 `folder_sync_dir`/`folder_sync_enabled`. 프론트는 빌드 종류를 모르고 항상
  이 Tauri 커맨드만 호출한다(빌드 분기는 Rust cfg가 흡수).
- `apps/desktop/src/lib/types.ts` — `FolderSyncResult`, Config 타입 확장.
- `apps/desktop/src/routes/Settings.tsx` — Git Sync 섹션 옆에 "Folder Sync" 섹션:
  폴더 선택 버튼(`pickFolderSyncDir`), 활성 토글, "지금 내보내기" 버튼,
  ops_status(job=folder_sync) 행(마지막 성공/실패/에러 + clearError). Git Sync UI 패턴 복제.
  iCloud/Google Drive 퀵픽 힌트(다이얼로그 기본 위치 점프)는 선택적 편의.

---

## 폴더 선택 · 북마크 · 설정

- 폴더 선택: 프론트가 기존 패턴(`openDialog({ directory: true })`)으로 다이얼로그를 열어
  경로를 얻은 뒤, **Rust 커맨드 `set_folder_sync_dir(path)`** 를 호출(프론트가 빌드 종류를
  몰라도 됨):
  - 모든 빌드: 엔진 settings `folder_sync_dir`에 저장(표시 및 비-MAS 직접 쓰기용).
  - MAS만: 추가로 방금 선택해 접근 권한이 있는 경로로부터 security-scoped bookmark 생성 →
    컨테이너에 저장. 이후 `folder_sync_now`/타이머가 복사 시 resolve→start/stop.
- **iCloud/Google Drive 편의(선택)**: 다이얼로그 기본 위치를 감지된 클라우드 경로로 점프.
  - iCloud Drive: `~/Library/Mobile Documents/com~apple~CloudDocs/`
  - Google Drive(신형 macOS): `~/Library/CloudStorage/GoogleDrive-*`
  - 결국 "고른 폴더" 한 개로 통일.
- 파일 레이아웃: 프로젝트당 마크다운 1개, 매 실행 덮어쓰기(Git Sync과 동일).

---

## 에러 처리 & 관찰성

- 폴더 미설정 또는 `folder_sync_enabled=false` → no-op(에러 아님).
- MAS 북마크 해제 실패(폴더 이동/삭제/권한 회수) →
  `ResultSummary.Error = "폴더 접근 권한을 잃었습니다. 다시 선택하세요"` + ops_status 에러.
- 복사/쓰기 실패(디스크 풀, 권한 등) → `ResultSummary.Error`.
- **소프트 에러(폴더 접근, 디스크)는 RPC 에러가 아니라 결과 payload(`Error` 필드)로**
  표면화(Git Sync과 동일). RPC 에러는 진짜 프로토콜 오류에만.
- 모든 시도는 `ops_status`(job=`folder_sync`)에 기록: started/finished/ok/error +
  metadata(files_written, delivered). Settings에서 마지막 성공/실패 표시 + clearError.

---

## 테스트

- 엔진 `Syncer.RunOnce`(`!mas`): 임시 디렉터리에 프로젝트 export·직접 쓰기 검증. projects
  repo 모킹/인메모리. files_written 카운트, 덮어쓰기, 폴더 미설정 no-op.
- 엔진 `mas` 빌드: `Stage`가 staging 디렉터리에 쓰고 {staging_dir, files[]}를 반환하는지,
  `Report`가 ops_status를 기록하는지.
- 엔진 핸들러: `folder_sync.run`(비-MAS)·`folder_sync.stage`/`folder_sync.report`(MAS) 결과
  직렬화.
- Rust: staging→target 복사 로직 임시 디렉터리 round-trip 테스트. 북마크 ObjC 래퍼는
  단위테스트 어려움 → 얇은 래퍼로 격리 + 수동 QA(실제 MAS 빌드에서 iCloud/GDrive 폴더로
  내보내기 확인).
- 프론트: Folder Sync 섹션 렌더, 설정 저장 경로(folder_sync_dir/enabled), ops_status 행 표시.

---

## 산출물

- 엔진 `foldersync` 패키지 + 빌드 태그 분기(`foldersync_direct.go` / `foldersync_staged.go`)
- `folder_sync.run`(비-MAS) / `folder_sync.stage` · `folder_sync.report`(MAS) RPC 핸들러
- settings `FolderSyncDir` / `FolderSyncEnabled`
- Tauri `set_folder_sync_dir` · `folder_sync_now` 커맨드 + `macos_bookmarks.rs`(cfg mas) +
  (MAS) 일일 타이머
- cargo feature `mas` 배선 + `bookmarks.app-scope` 엔타이틀먼트 + MAS conf 엔타이틀먼트 수정
- Settings UI "Folder Sync" 섹션
- 테스트(엔진 direct/staged, Rust 복사, 프론트 렌더)

## 검증 (완료 기준)

- 비-MAS: 폴더 지정 → 수동/일일 실행 → 대상 폴더에 프로젝트 마크다운이 생성·갱신됨.
- MAS: 폴더 선택 시 북마크 생성 → 앱 재시작 후에도 자동/수동 실행이 iCloud/GDrive 폴더에
  파일을 씀(security-scoped 접근으로). spctl/검증 통과한 MAS 빌드에서 수동 QA.
- ops_status(job=folder_sync)에 성공/실패가 기록되고 Settings에 표시됨.
- 폴더 이동/삭제 시 명확한 에러 메시지 표면화 + 재선택으로 복구.

## 주요 리스크

- **security-scoped bookmark FFI 정확성** — 가장 위험. resolve/start/stop 순서, 만료/staleness
  처리. ObjC 래퍼를 작게 유지하고 수동 QA로 검증.
- **MAS 일일 타이머 신뢰성** — Rust 타이머는 앱 실행 중에만 동작(엔진 스케줄러와 동일 제약).
  앱 시작 시 1회 + 24h 주기로 구동해 "하루 한 번"을 보장. ops_status는 `folder_sync.report`
  에서만 기록되므로 stage 후 복사 실패 시에도 report로 에러를 남긴다.
- **MAS cargo feature 배선** — Rust `mas` feature와 Go `mas` 빌드 태그가 함께 켜져야 함.
  release-mas-local.sh에서 둘 다 보장.
- **Google Drive 폴더 경로 가변성** — 신/구 macOS, Windows 드라이브 문자 등. 자동 감지는
  편의일 뿐 필수 아님 — 사용자가 직접 고르면 됨.

## 다음 (후속, 별도 spec 가능)

- 동시 다중 폴더 타깃 (iCloud + GDrive 동시)
- Folder Sync CI 연동
- Google Drive REST API 직접 업로드(데스크톱 클라이언트 불필요)
- **Stale bookmark 자동 갱신** (MAS): 사용자가 폴더를 이동/리네임하면 security-scoped
  bookmark가 stale 상태가 된다. 현재는 resolve는 되지만 갱신본을 다시 저장하지 않아 장기적으로
  접근이 끊길 수 있다. `URLByResolvingBookmarkData`의 `isStale`가 true면 FFI로 갱신된 bookmark
  데이터를 반환해 재저장하는 처리를 후속에 추가한다. (접근 완전 실패 시엔 이미 "다시 선택" 안내됨.)
