# Phase 3: Linetta 앱 게시 — 작업지시서

_이 페이즈의 작업은 `linetta` 리포(`~/workspace/myprojects/linetta`)에서 한다. Claude Code가 위에서부터 순서대로 실행하고, Checkpoint에서 사용자 확인을 받는다._

_작성일: 2026-10-10 (원본: `linetta-artworks` 리포 `docs/plans/artworks-phase-3-linetta-publish.md`, 2026-10-18에 이 리포로 복사)_
_속한 로드맵: `linetta-artworks` 리포의 `docs/plans/artworks-roadmap.md`_
_예상 소요: 20~28시간_
_조사 기준: linetta v1.5.0 (커밋 `34cf973`). 아래의 파일 경로와 줄 번호는 그 시점 기준이므로 작업 전에 다시 확인한다._

## 페이즈 목표

데스크톱 Linetta에 게시 기능을 넣는다. 설정에서 브라우저 로그인으로 계정을 연결하고, 회차에서 게시 버튼으로 올리고, MCP 툴로도 게시할 수 있다. 이 페이즈가 끝나면 연재작의 새 회차를 앱에서 바로 사이트에 올릴 수 있다.

## 전제 조건

- [ ] Phase 2 완료 및 사용자 승인
- [ ] `linetta-artworks` 리포의 `docs/api.md`가 확정되어 있다
- [ ] 운영 사이트가 떠 있고 본인 계정의 `can_publish`가 켜져 있다
- [ ] 시작할 때 이 작업지시서를 `linetta` 리포의 `docs/plans/publish/`에 복사한다. 그 리포의 계획 문서 관례(주제별 폴더, 페이즈별 작업지시서, Checkpoint)와 같다

## linetta 리포에서 지킬 것

- 별도의 에이전트 규칙 파일은 없다. 규칙은 `docs/DEVELOPMENT.md`, `Makefile`, 보호 테스트에 있다.
- 검증 명령: `make test` (= `test-go` + `test-desktop` + `test-tauri`). 모바일 빌드 확인은 `make test-mobile-engine`.
- 커밋 제목은 범위를 붙인다: `feat(publish): …`, `fix(publish): …`.
- 화면 문구는 한국어, 영어, 일본어 세 벌을 모두 넣는다. `i18n.catalog.test.ts`가 키 집합이 같은지 검사한다.
- 엔진에서 화면으로 가는 오류는 번역된 문장이 아니라 사유 코드로 보낸다 (`engine/internal/rpc/reason.go`, `apps/desktop/src/lib/rpcMessage.ts`).
- 플랫폼에 따라 빠지는 기능은 `_enabled.go`와 `_disabled.go`를 짝으로 두고 진단 플래그를 낸다.
- 걸리기 쉬운 보호 테스트: `rpcAllowlist.test.ts`(Rust 허용 목록, 정렬 필수), `agentToolParity.test.ts`(MCP 툴 이름 목록), `validate-story-core-deps.sh`(`mcphost`가 LLM 클라이언트에 의존 금지), 명령 팔레트 섹션 8개 고정.
- 사용자에게 보이는 변경은 `CHANGELOG.md`에 적는다.

## 포함 기능

1. 엔진의 회차 판별과 회차 Markdown
2. 게시 클라이언트, 연결, 인증 정보 보관, 게시 상태 저장
3. 설정의 게시 항목, 회차의 게시 창과 상태 표시
4. MCP 게시 툴
5. 문서와 개인정보처리방침 갱신

## 이 페이즈에서 하지 않는 것

- iOS 게시 → Out of Scope. `mobile` 빌드에서는 게시가 꺼져 있고 화면에서 숨긴다
- 여러 회차 한꺼번에 게시, 예약 게시, 자동 게시 → 다음 단계
- 표지 이미지 올리기 → 다음 단계. API에 자리는 있지만 앱에서는 보내지 않는다
- 세계관·인물 시트 전송 → Out of Scope
- 수위 검사 → Phase 4 (서버 쪽). 단, 앱은 `pending`, `rejected`, `needs_review` 상태를 이 페이즈에서 표시할 수 있어야 한다

## 작업 체크리스트

### 작업 그룹 A: 회차를 엔진이 알게 하기

- [ ] **T3.A.1** — 회차 판별을 Go로
  - 파일: `engine/internal/episode/episode.go`, `episode_test.go` (신규 패키지)
  - 내용:
    - 지금 회차 판별은 화면 쪽에만 있다: `apps/desktop/src/lib/outlineRepair.ts:96-140`(라벨 패턴), `apps/desktop/src/lib/outlineEpisode.ts:15-31`(`findEpisodeNode`). 같은 규칙을 Go로 옮긴다
    - `Resolve(ctx, nr *node.Repo, nodeID string) (episodeNodeID string, err error)`: 주어진 노드가 속한 회차 노드를 찾는다
    - `List(ctx, nr *node.Repo, projectID string) ([]Episode, error)`: 작품의 회차를 순서대로 준다. `Episode{NodeID, Label, Title, Ordinal}`
    - 회차는 권 바로 아래의 leaf일 수도 있고 씬 leaf들을 담은 컨테이너일 수도 있다. 두 경우를 모두 다룬다
    - 단편(회차 구분이 없는 작품)은 작품 전체를 회차 하나로 본다
  - 검증: TS 쪽 테스트의 사례를 Go 테스트로 옮겨 두 구현이 같은 답을 내는지 확인한다. `go test ./internal/episode/...`

- [ ] **T3.A.2** — 회차 단위 Markdown
  - 파일: `engine/internal/export/episode.go`, `episode_test.go`
  - 내용:
    - `ExportEpisodeMarkdown(ctx, nr *node.Repo, episodeNodeID string) (EpisodePayload, error)`. `EpisodePayload{Markdown, Title, CharCount, ContentHash}`
    - 지금은 컨테이너를 Markdown으로 내보내는 경로가 없다 (`ExportNode`는 leaf만, `ExportNodeText`는 일반 텍스트만). leaf 자손을 순서대로 `DocToMarkdown`으로 바꿔 장면 전환 표시로 잇는다
    - 게시용 본문에는 편집 흔적이 남으면 안 된다. 멘션은 `@라벨`이 아니라 라벨만 남기고, 노트 표시는 버린다. `DocToMarkdown`에 게시용 옵션을 추가하거나 감싸는 함수를 둔다. 기존 내보내기 동작은 바꾸지 않는다
    - 해시는 최종 Markdown의 SHA-256이다. 서버도 같은 방식으로 계산한다 (`docs/api.md`)
  - 검증: 씬 하나인 회차, 씬 여러 개인 회차, 멘션과 노트가 든 회차의 출력을 테스트한다. `export/schema_test.go`가 계속 통과한다

### 작업 그룹 B: 게시 엔진

- [ ] **T3.B.1** — 게시 상태 테이블
  - 파일: `engine/internal/store/migrations/0023_publish.sql` (작업 시점의 다음 번호를 쓴다), `engine/internal/publish/state.go`
  - 내용:
    - `publish_works(project_id PK, remote_url, slug, blurb, serial_status, last_synced_at)`
    - `publish_episodes(node_id PK, project_id, remote_url, number, content_hash, remote_status, published_at)`
    - 독자용 소개글(`blurb`)은 여기에 둔다. `projects.synopsis`는 AI 맥락용 요약이라 쓰지 않는다
  - 검증: 마이그레이션 테스트 통과

- [ ] **T3.B.2** — HTTP 클라이언트
  - 파일: `engine/internal/publish/client.go`, `client_test.go`
  - 내용:
    - `docs/api.md`의 경로를 그대로 구현한다. 기준 주소의 기본값은 운영 사이트이고, 개발용으로 환경변수 `LINETTA_PUBLISH_BASE_URL`로 바꿀 수 있다
    - 서버의 오류 사유 코드를 엔진의 사유 코드로 옮긴다. `rpc/reason.go`에 `ReasonPublish…` 상수를 추가한다
    - 요청 시간 제한, 한 번의 재시도(멱등한 PUT에 한해), User-Agent에 앱 버전
    - 토큰과 본문을 로그에 남기지 않는다
  - 검증: `httptest` 서버로 정상, 401, 403, 413, 429, 시간 초과를 테스트한다

- [ ] **T3.B.3** — 인증 정보 보관
  - 파일: `engine/internal/settings/secrets.go`, `engine/internal/publish/credential.go`
  - 내용:
    - 비밀값 이름 `publish.token`을 추가하고 기존 `SecretStore`(macOS 키체인, Windows 자격 증명 관리자)에 둔다
    - 보안 저장소가 없는 플랫폼(Linux)에서는 `<home>/publish-token`에 권한 0600으로 쓴다. MCP 토큰의 선례(`engine/internal/settings/mcp.go:77-140`)와 같은 방식이고, 그 이유를 주석으로 남긴다
    - 필명과 주소 같은 비밀이 아닌 연결 정보는 `settings.Config`에 둔다
    - 테스트에서는 `NewMemorySecretStore()`를 주입해 실제 키체인에 닿지 않게 한다
  - 검증: 저장, 조회, 삭제, 보안 저장소 미지원 시 파일 대체 경로를 테스트한다

- [ ] **T3.B.4** — 연결 흐름
  - 파일: `engine/internal/publish/connect.go`, `connect_test.go`
  - 내용:
    - `codexauth.Service`(`engine/internal/codexauth/login.go`)의 구조를 본보기로 삼는다: `Start`가 주소를 바로 돌려주고, 백그라운드에서 기한을 두고 기다리고, `Status`는 토큰을 절대 돌려주지 않는다. 두 번째 `Start`가 첫 번째를 대체한다
    - 차이: 로컬 콜백 서버를 띄우지 않는다. `pair/start`로 받은 비밀값으로 `pair/poll`을 `interval` 간격으로 부른다
    - `Status{state: disconnected|waiting|connected|failed, confirm_code, pen_name, handle, failure_reason}`
    - `Disconnect`는 로컬 토큰을 지운다. 서버 쪽 기기 해제는 웹 프로필에서 한다는 안내를 화면에 둔다
  - 검증: 가짜 서버로 승인, 거절, 만료, 취소를 테스트한다

- [ ] **T3.B.5** — 게시 동작
  - 파일: `engine/internal/publish/service.go`, `service_test.go`
  - 내용:
    - `WorkStatus(projectID)`: 로컬 회차 목록(T3.A.1)과 서버 목록(`GET works/{project_id}`)을 합쳐 회차별 상태를 낸다: 미게시, 공개됨, 수정됨(로컬 해시가 서버와 다름), 검토 중, 반려, 운영자 검토 대기, 비공개, 사이트에만 있음
    - `UpsertWork(projectID, blurb, serialStatus)`: 제목과 장르는 `projects`에서, 언어는 앱 설정 언어에서 가져온다
    - `PublishEpisode(nodeID)`: 회차 판별 → Markdown → 작품이 서버에 없으면 먼저 등록을 요구 → 순서 맞춤(`episodes:reorder`) → 게시 → 로컬 상태 저장 → 성공하면 회차에 속한 노드의 상태를 `published`로 바꾼다 (`node.StatusPublished`)
    - `SetEpisodeVisibility(nodeID, visibility)`
    - 네트워크 실패 때 원고와 로컬 상태를 바꾸지 않는다
  - 검증: 가짜 서버로 첫 게시, 재게시, 순서 변경 뒤 게시, 작품 미등록, 실패 시 상태 불변을 테스트한다

- [ ] **T3.B.6** — RPC와 빌드 구분
  - 파일: `engine/internal/rpc/handlers/publish.go`, `engine/internal/engineapp/publish_enabled.go`, `publish_disabled.go`, `engine/internal/rpc/handlers/diagnostics.go`, `apps/desktop/src-tauri/src/lib.rs`
  - 내용:
    - RPC: `publish.connect_start`, `publish.connect_status`, `publish.disconnect`, `publish.work_status`, `publish.work_upsert`, `publish.episode_publish`, `publish.episode_set_visibility`
    - `publish_disabled.go`(`mobile` 태그)는 거절하는 대체 핸들러를 등록한다. `gitsync_disabled.go`와 같은 방식이다
    - 진단에 `publish_available` 플래그를 추가한다
    - Rust의 `RENDERER_ENGINE_METHODS`에 새 메서드를 정렬 순서에 맞게 넣는다
    - `EXTERNAL_URL_HOSTS`(`lib.rs:504`)에 사이트 도메인을 추가한다. 지금은 `auth.openai.com`만 허용된다
    - Mac App Store 빌드에서도 켠다. 두 권한 파일 모두 `network.client`가 이미 있다
  - 검증: `make test-go`, `make test-tauri`, `make test-mobile-engine`

### 작업 그룹 C: 화면

- [ ] **T3.C.1** — RPC 래퍼와 타입
  - 파일: `apps/desktop/src/lib/rpc.ts`, `apps/desktop/src/lib/types.ts`
  - 내용: `publishApi` 객체와 응답 타입. 사유 코드 문구를 `rpcMessage.ts`에 추가한다
  - 검증: `rpcAllowlist.test.ts` 통과

- [ ] **T3.C.2** — 설정의 게시 항목
  - 파일: `apps/desktop/src/routes/Settings.tsx`, `apps/desktop/src/components/settings/PublishSection.tsx`, `PublishSection.test.tsx`
  - 내용:
    - `SETTINGS_CATEGORIES`에 `publish`를 추가하고 `navGroups`에 항목을 넣는다. `publish_available`이 거짓이면 숨긴다
    - 상태별 화면은 `linetta-artworks` 리포의 `docs/plans/ux-flow.md` "설정 > 게시"를 따른다
    - 연결 전에 안내와 동의 체크를 둔다: 게시한 원고는 사이트 서버에 저장되어 공개되고, 수위 검사를 위해 Anthropic API로 전송된다. 동의하지 않으면 연결 버튼이 눌리지 않는다. 제공자별 동의(`ProviderSection.tsx`)와 같은 방식이다
    - 상태 확인은 `ProviderSection.tsx:509-540`의 방식(1.5초 간격, 세대 카운터로 보호)을 따른다
    - 브라우저는 `openExternalUrl`로 연다
  - 검증: 컴포넌트 테스트에서 미연결, 대기(확인 코드 표시), 연결됨, 실패, 사용 불가 다섯 상태를 확인한다

- [ ] **T3.C.3** — 게시 창
  - 파일: `apps/desktop/src/components/PublishDialog.tsx`, `PublishDialog.test.tsx`, `apps/desktop/src/routes/Workspace.tsx`
  - 내용:
    - 진입점: 아웃라인의 회차 줄 메뉴, 그리고 명령 팔레트의 기존 섹션 안의 명령 하나. 팔레트에 섹션을 새로 만들지 않는다
    - 작품의 첫 게시면 소개글과 연재 상태를 입력받는다. 소개글이 비어 있으면 게시할 수 없다. 이후에는 "작품 정보 수정"으로 접어둔다
    - 회차 제목, 글자 수, 현재 상태, 게시 버튼. 결과로 주소와 복사·열기 버튼
    - 미연결이면 설정의 게시 항목으로 가는 안내를 보여준다
    - 반려와 검토 대기 상태에는 사유 요약과 재검토 요청 안내를 보여준다
    - "사이트에만 있는 회차"가 있으면 목록으로 보여주고 비공개 전환을 고르게 한다
  - 검증: 컴포넌트 테스트에서 첫 게시, 재게시, 미연결, 오류, 반려 상태를 확인한다

- [ ] **T3.C.4** — 회차의 게시 상태 표시
  - 파일: 아웃라인 컴포넌트 (작업 시 위치 확인)
  - 내용: 연결된 상태에서 게시된 작품의 회차 줄에 상태를 작게 표시한다 (공개됨, 수정됨, 검토 중, 반려). 작품을 열 때와 게시 직후에만 서버 상태를 가져온다. 주기적으로 묻지 않는다
  - 검증: 오프라인에서 작품을 열어도 오류 없이 마지막으로 저장된 상태가 보인다

- [ ] **T3.C.5** — 세 언어 문구
  - 파일: `apps/desktop/src/lib/i18n.tsx`
  - 내용: 새 키를 `ko`, `en`, `ja` 세 맵에 모두 넣는다. 한국어가 기준이다
  - 검증: `i18n.catalog.test.ts` 통과

### 작업 그룹 D: MCP 툴

- [ ] **T3.D.1** — 게시 툴
  - 파일: `engine/internal/mcphost/tools_publish.go`, `tools_publish_test.go`, `apps/desktop/src/lib/agentTools.ts`
  - 내용:
    - `linetta_publish_status`(읽기): 작품의 회차별 게시 상태
    - `linetta_publish_episode`(쓰기): 회차 하나를 게시한다. 연결되어 있지 않거나 작품이 서버에 등록되어 있지 않으면 거절하고, 사용자가 앱에서 먼저 연결과 작품 등록을 해야 한다고 알려준다. 에이전트가 계정 연결이나 작품 등록을 대신하지 않는다
    - 기존 패턴(`tools_read.go:299-304`의 `mcp.AddTool`과 `record`)을 따르고 이름을 `ReadToolNames`, `WriteToolNames`와 `agentTools.ts`에 넣는다
    - 쓰기 툴은 `MCPModeFull`에서만 등록된다
    - `docs/DEVELOPMENT.md`의 MCP 툴 목록과 개수를 고친다
    - 툴 예산(`settings/toolbudget.go`)이 넘지 않는지 확인한다
  - 검증: `agentToolParity.test.ts`, `tools_register_test.go`, `mcp_schema_test.go`, `validate-story-core-deps.sh` 통과

### 작업 그룹 E: 문서

- [ ] **T3.E.1** — 문서와 정책
  - 파일: `README.md`, `CHANGELOG.md`, `docs/privacy-policy.md`, `docs/DEVELOPMENT.md`
  - 내용:
    - README에 게시 기능 절을 추가한다: 선택 기능이라는 점, 무엇이 어디로 전송되는지, 연결 정보가 어디에 저장되는지(Linux 포함), 연결 해제 방법
    - README의 "Linetta has no cloud" 같은 문장이 게시 기능과 어긋나지 않게 고친다
    - 개인정보처리방침에 게시 기능을 켰을 때의 전송 내용을 추가한다
    - CHANGELOG에 사용자용 설명을 쓴다
  - 검증: 사용자가 문구를 읽고 승인한다

---

## ✅ Phase 3 Checkpoint

**구현 확인:**
- [ ] 모든 작업 체크박스 완료
- [ ] 게시하지 않는 사용자에게 달라지는 것이 없다: 연결하지 않은 상태에서 앱이 사이트로 요청을 보내지 않는다

**자동 검증:**
- [ ] `make test` 통과
- [ ] `make test-mobile-engine` 통과
- [ ] `make ci` 통과

**수동 확인 (사용자, 데스크톱 개발 빌드와 운영 사이트):**
- [ ] 설정 > 게시에서 연결: 브라우저의 확인 코드가 앱과 같고, 허용하면 앱이 필명과 함께 "연결됨"으로 바뀐다
- [ ] Phase 1에서 가져온 연재작을 연다: 이미 올라간 회차가 "공개됨"으로 보인다 (가져올 때 넣은 식별자가 맞는지 확인하는 항목이다)
- [ ] 새 회차를 게시: 주소가 표시되고 사이트에서 읽힌다. 멘션의 `@`나 편집 흔적이 본문에 없다
- [ ] 그 회차를 고친 뒤 상태가 "수정됨"으로 보이고, 다시 게시하면 같은 주소가 갱신된다
- [ ] 회차를 비공개로 전환하면 사이트에서 404가 된다
- [ ] 네트워크를 끊고 게시: 원고가 그대로이고 재시도 버튼이 보인다
- [ ] MCP로 연결한 에이전트에게 회차 게시를 시키면 게시되고 활동 로그에 남는다
- [ ] 웹 프로필에서 기기를 해제한 뒤 앱에서 게시하면 다시 연결하라는 안내가 보인다

**완료 처리:**
1. 위 항목이 모두 통과하면 완료한 작업, 자동 검증 결과, 수동 확인 항목을 사용자에게 보고한다.
2. 사용자가 "Phase 3 완료"라고 승인하면 릴리스 여부를 사용자가 정한다. 앱 릴리스는 이 작업지시서의 범위가 아니다.
3. 실패하면 실패 항목을 보고하고 원인을 찾아 고친 뒤 다시 검증한다.

---

## 메모 / 주의

- **가입 개방과의 순서**: 이 기능이 든 Linetta 버전이 배포되기 전에는 다른 작가가 게시할 방법이 없다. Phase 4의 가입 개방은 그 릴리스 뒤에 한다.
- **Mac App Store 심사**: 사용자가 만든 글을 공개 게시하는 기능이 생긴다. 심사에서 신고·차단 수단이나 계정 삭제 경로를 물을 수 있다. 신고 경로와 계정 삭제는 Phase 4에서 웹에 만든다. 스토어 제출은 Phase 4가 끝난 뒤에 하는 것을 권한다.
- **AGPL**: 이 리포는 공개 AGPL이다. 게시 클라이언트 코드는 공개된다. 서버 쪽 비밀값이나 남용 방지의 구체적인 한도 수치를 앱 코드에 넣지 않는다.
- **API 계약이 어긋날 때**: 구현하다가 `docs/api.md`를 바꿔야 하면 `linetta-artworks` 리포에서 먼저 고치고 사용자에게 알린다. 앱 쪽에서 임시로 맞추지 않는다.
- 회차 판별 규칙이 TS와 Go 두 곳에 생긴다. 이 페이즈에서는 화면 쪽 구현을 그대로 두고 Go 쪽을 추가한다. 한쪽으로 합치는 일은 별도 작업으로 남긴다.

---
_다음 페이즈: Phase 4 — 수위 검사와 공개 준비 (`linetta-artworks` 리포, 서버 쪽 작업)_
