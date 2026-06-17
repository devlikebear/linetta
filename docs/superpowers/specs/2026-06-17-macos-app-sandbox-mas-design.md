# Linetta macOS App Sandbox 호환화 (MAS 준비) 설계

- 작성일: 2026-06-17
- 상태: 설계 승인됨 (사용자 리뷰 대기)
- 대상: Linetta 데스크톱 앱(Tauri v2 + Go 사이드카), Mac App Store 배포 준비
- 선행: [Phase 1 — Developer ID 직접 배포](2026-06-16-macos-codesigning-distribution-design.md) (완료, main 머지됨)

## 배경

Phase 1에서 Developer ID 서명·공증·staple 배포(앱 + dmg, 로컬 + CI)를 완료했다.
Phase 2는 Mac App Store(MAS) 배포다. MAS는 **App Sandbox 필수**이며, 이는
Developer ID 배포와 근본적으로 다른 제약을 건다. 이 문서는 Phase 2의 첫 조각인
**App Sandbox 호환화**만 다룬다.

App Sandbox 호환성 감사 결과 요약:

- **호환 (그대로 통과)**: SQLite DB·백업·설정·companion 데이터는 모두
  `~/Library/Application Support/com.devlikebear.linetta/` 안에 있음.
  import/export(.md)는 Tauri `plugin-dialog` + `plugin-fs`를 거쳐 security-scoped
  bookmark가 자동 처리됨. Rust↔Go 사이드카는 stdin/stdout 파이프 통신(포트 없음).
  사이드카 바이너리는 앱 번들 내장.
- **엔타이틀먼트 선언 필요**: LLM/웹검색 아웃바운드 네트워크, Keychain 읽기,
  사용자 선택 파일 읽기/쓰기.
- **근본적 비호환 (코드 변경 필요)**: Git Sync — 엔진이 사용자가 설정한 임의
  경로(`GitSyncDir`)에 `os.WriteFile`하고 `git` 외부 프로세스를 spawn한다.
  샌드박스에서 둘 다 차단되며, `git` 실행은 엔타이틀먼트로도 풀 수 없다.

## 결정 사항 (확정)

- **범위**: App Sandbox 호환화만 먼저. 인증서/앱레코드/.pkg/업로드는 다음 spec.
- **Git Sync**: MAS 빌드에서 **컴파일 타임 빌드 태그(`mas`)로 코드 자체를 제외**.
  Developer ID 빌드는 Git Sync를 그대로 유지.
- **구조**: 전용 MAS 설정 오버레이 + 분리된 엔타이틀먼트 (접근 A).
- **검증**: MAS 인증서 없이 **기존 Developer ID 인증서 + app-sandbox 엔타이틀먼트**로
  로컬 서명해 샌드박스 동작을 먼저 확인.

## 목표 / 비범위

**목표**: App Sandbox에서 정상 동작하는 MAS 빌드 변형을 만들고, 로컬에서 샌드박스
서명된 앱을 실행해 모든 핵심 기능(DB·LLM 네트워크·Keychain·import/export)이
동작하며 Git Sync가 부재함을 검증한다.

**비범위**:

- Apple Distribution / Mac Installer Distribution 인증서 발급
- App Store Connect 앱 레코드 생성 (bundle id 등록)
- `.pkg` 생성·서명·업로드, 실제 심사 제출
- Intel(x86_64) 빌드 — Apple Silicon 전용 유지
- 프론트엔드 Git Sync UI 완전 제거 (엔진 핸들러가 미지원 에러를 반환하므로 안전성은
  확보됨; UI 정리는 후속 작업으로 분리)

---

## 1. 엔진 — `mas` 빌드 태그로 Git Sync 제외

`gitsync` 패키지가 `mas` 빌드에 **아예 컴파일되지 않도록** 분리한다. 기존 엔진은
이미 `secrets_darwin.go`(`//go:build darwin`) / `secrets_other.go`(`//go:build !darwin`)
패턴을 쓰므로 이를 따른다.

현재 배선 (`engine/cmd/linetta-engine/main.go`):

- L152: `syncer := gitsync.New(...)`, L153: `syncer.Ops = ops`
- L155-166: `retentionFn`이 `syncer.RunOnce(ctx)`를 호출 (backup retention 안에 얽힘)
- L291-292: `git_sync.run` / `git_sync.init` RPC 핸들러 등록

변경:

1. **`handlers/gitsync.go`** → `//go:build !mas` 추가.
2. **`gitsync/gitsync_test.go`** → `//go:build !mas` 추가 (`go test -tags mas` 깨짐 방지).
3. **`main.go` 배선 추출** — 빌드태그 파일 2개 신설:
   - `engine/cmd/linetta-engine/gitsync_enabled.go` (`//go:build !mas`):
     - `dailySyncer` 인터페이스 정의 (`RunOnce(ctx) (gitsync.ResultSummary, error)` 수준)
     - `newSyncer(settingsStore, projects, nodes, entities, relationships, ops) dailySyncer`
       → 실제 `gitsync.New(...)` 반환 + `Ops` 설정
     - `registerGitSyncHandlers(s, syncer)` → `git_sync.run/init` 등록
   - `engine/cmd/linetta-engine/gitsync_disabled.go` (`//go:build mas`):
     - 동일 시그니처의 no-op 구현: `newSyncer` → no-op syncer(`RunOnce`가
       `Skipped: true` 반환), `registerGitSyncHandlers` → `git_sync.run/init`을
       "App Store 빌드에서 미지원" 에러(`rpc.MethodError`)로 등록
4. **`main.go` 본문**은 `newSyncer(...)` / `registerGitSyncHandlers(s, syncer)`만
   호출하도록 수정 → 태그와 무관하게 컴파일. `retentionFn`은 인터페이스의
   `RunOnce`를 호출하므로 그대로 동작 (mas에서는 no-op).
5. **`scripts/build-engine.sh`**: 환경변수 `LINETTA_BUILD_TAGS`를 받아
   `go build -tags "${LINETTA_BUILD_TAGS}"`로 전달 (미설정 시 현행과 동일).

검증: `go build -tags mas ./...` 와 `go test ./...`(태그 없음) 모두 통과.
mas 바이너리에 `gitsync` 패키지 심볼이 링크되지 않음을 `go list -deps -tags mas`로 확인.

## 2. 엔진 — Keychain 샌드박스 검증/교체

`engine/internal/settings/secrets_darwin.go`는 레거시 `SecKeychainFindGenericPassword`
/ `SecKeychainAddGenericPassword` (cgo, Security.framework)를 사용한다. App Sandbox에서
레거시 `SecKeychain*` API는 앱 전용 keychain access group으로 자동 스코프되지만 동작이
보장되지 않는다.

- **검증**: 샌드박스 서명된 앱에서 API 키 저장/조회가 동작하는지 확인.
- **실패 시 교체 (범위 내)**: 모던 `SecItemAdd` / `SecItemCopyMatching` /
  `SecItemDelete` (`kSecClassGenericPassword`)로 교체. 샌드박스에서 앱은
  `application-identifier` 기반 access group을 자동으로 받으므로, 별도 keychain
  access group 엔타이틀먼트 없이 동작하는 것이 기본. 필요 시 엔타이틀먼트에
  `keychain-access-groups` 추가.
- 동작은 `secrets_other.go`(non-darwin)와 동일한 인터페이스를 유지한다.

> 사용자 API 키 저장/조회가 깨지면 앱이 무용지물이므로, 검증 후 필요하면 교체까지
> 이 spec 범위에 포함한다.

## 3. Tauri 앱 — MAS 변형

### 3.1 설정 오버레이

`apps/desktop/src-tauri/tauri.mas.conf.json` 신설. `pnpm tauri build --config
tauri.mas.conf.json`로 기본 `tauri.conf.json`에 병합한다. 포함 내용:

- `bundle.macOS.entitlements`: 메인 앱 엔타이틀먼트 plist 경로
- (필요 시) `bundle.macOS.minimumSystemVersion` 등 MAS 요구 값

### 3.2 엔타이틀먼트 파일

- `apps/desktop/src-tauri/entitlements/linetta.entitlements` (메인 앱):
  - `com.apple.security.app-sandbox` = true
  - `com.apple.security.network.client` = true (LLM/웹검색 아웃바운드)
  - `com.apple.security.files.user-selected.read-write` = true (파일 다이얼로그)
  - (검증 결과에 따라) `keychain-access-groups`
- `apps/desktop/src-tauri/entitlements/linetta-sidecar.entitlements` (사이드카):
  - `com.apple.security.inherit` = true (부모 샌드박스 상속) — **이것만**

### 3.3 서명 순서 제어

Tauri 기본 서명은 사이드카에도 앱 엔타이틀먼트를 적용할 수 있어 MAS에서 부적합하다.
빌드 스크립트(§5)가 codesign을 명시적으로 제어한다:

1. 사이드카(`Contents/MacOS/linetta-engine`)를 `linetta-sidecar.entitlements`
   (inherit)로 서명
2. 메인 앱 번들을 `linetta.entitlements` (sandbox)로 서명
3. `codesign --options runtime`는 Developer ID 경로용; MAS 제출용은 후속 spec에서
   Apple Distribution 인증서로 재서명

## 4. 사소한 샌드박스 조정

- `apps/desktop/src-tauri/src/lib.rs`의 `open_path` 커맨드: 임의 경로에 대한 `open`
  호출이 샌드박스에서 막힐 수 있음. App Support 하위 경로로 제한하거나, 실패 시
  graceful 처리(에러를 UI에 노출하되 크래시 없음). 영향 작음(백업 폴더 Finder 열기).
- `LINETTA_HOME` env override (`engine/internal/paths/paths.go`): 샌드박스에서
  컨테이너 밖 경로면 쓰기 실패. 코드 변경 불필요, 동작만 문서화.

## 5. 빌드 & 로컬 검증 플로우

### 5.1 빌드 스크립트

`scripts/build-mas-local.sh` 신설:

1. `LINETTA_BUILD_TAGS=mas bash scripts/build-engine.sh` (엔진 사이드카, git 제외)
2. `pnpm tauri build --config src-tauri/tauri.mas.conf.json --bundles app`
3. 사이드카를 inherit 엔타이틀먼트로 서명 → 앱을 sandbox 엔타이틀먼트로 서명
   (기존 Developer ID 인증서 사용, `~/.linetta/apple/config.env`에서 식별)

`Makefile`에 `build-mas-local` 타깃 추가.

### 5.2 검증 (완료 기준)

1. `codesign -d --entitlements - <app>` → app-sandbox 등 엔타이틀먼트 적용 확인
2. `codesign -d --entitlements - <app>/Contents/MacOS/linetta-engine` → inherit 확인
3. 샌드박스 서명된 앱 실행:
   - 프로젝트/노드 생성·편집 → DB 읽기/쓰기 OK (컨테이너 경로)
   - LLM 호출 1회 성공 (네트워크 엔타이틀먼트)
   - API 키 저장 후 재시작 → Keychain 조회 OK
   - .md import / export 다이얼로그 동작 OK
4. Git Sync 기능 부재 확인: `git_sync.run` 호출 시 미지원 에러, `git` 프로세스 미실행
5. `log stream --predicate 'sender == "sandboxd"'`로 실행 중 sandbox violation 없음 확인

## 산출물

- `mas` 태그로 빌드되어 Git Sync가 제외된 엔진 사이드카
- App Sandbox 엔타이틀먼트가 적용된 `Linetta.app` (Developer ID 서명, 로컬 검증용)
- `tauri.mas.conf.json`, 엔타이틀먼트 2종, `scripts/build-mas-local.sh`,
  `make build-mas-local`
- (필요 시) 모던 SecItem API 기반 Keychain 구현

## 주요 리스크

- **Keychain**: 레거시 API가 샌드박스에서 실패하면 모던 API 교체 필요 (범위 내 흡수)
- **사이드카 inherit 서명**: Tauri 기본 서명이 덮어쓸 수 있어 빌드 스크립트가 서명 제어
- **검증 한계**: Developer ID + app-sandbox 서명으로 로컬 샌드박스 동작은 확인 가능하나,
  MAS 고유 제약(provisioning profile, receipt validation 등)은 다음 spec에서 다룸

## 다음 spec (Phase 2-B 예고)

- Apple Distribution + Mac Installer Distribution 인증서 발급
- App Store Connect 앱 레코드 생성
- `productbuild`로 `.pkg` 생성·서명, `notarytool`/Transporter 업로드
- 프론트엔드 Git Sync UI 정리, MAS 빌드 CI 연동
