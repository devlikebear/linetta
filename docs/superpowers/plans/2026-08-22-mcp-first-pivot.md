# MCP 우선 전환 — 구현 계획

> **에이전트 작업자에게:** 커밋은 기능 단위로, `feat/fix/chore` 메시지로 작성합니다. 동작이 바뀌는 작업은 실패하는 테스트를 먼저 쓰고, 각 단계 종료 시 `make test`를 통과시킵니다. 각 단계는 체크박스(`- [ ]`)로 추적합니다.
> **설계 문서:** `docs/superpowers/specs/2026-08-22-mcp-first-pivot-design.md`

**목표:** Linetta를 순수 창작 도구로 되돌리고, AI 협업은 Claude Desktop/Claude Code 같은 외부 MCP 클라이언트로 옮긴다. 앱은 LLM을 호출하지 않고 API 키를 요구하지 않으며, 외부 에이전트가 큐레이션된 스토리 컨텍스트를 읽고 원고를 쓸 수 있는 작업장이 된다.

**아키텍처:** 스토리 코어(컨텍스트 조립 + 스토리 옵 적용)를 LLM 코드에서 분리해 `internal/storycontext`, `internal/storyops`로 추출한다. 컨텍스트는 오늘 두 경로(`ai.ContextBuilder`의 씬 중심 브리프, 컴패니언 `gatherContext`의 팩트·메모리·레퍼런스)로 나뉘어 있으므로 추출은 이동이 아니라 **병합**이다. 그 위에 `internal/mcphost`가 공식 Go SDK의 Streamable HTTP 핸들러를 `127.0.0.1:7391`에 올린다. LLM 루프 제거는 검증이 끝난 뒤에만 한다.

**기술 스택:** Go 1.26 엔진, `github.com/modelcontextprotocol/go-sdk` v1.7.0, Tauri 2 / Rust 셸, React 18 + TypeScript + Vitest.

## 전역 제약

- 엔진 모듈은 `github.com/devlikebear/linetta/engine`. 빌드 태그 `mas`와 `mobile`은 서로 독립이며 둘 다 계속 빌드되어야 한다: `make test`, `make test-mobile-engine`, `cd engine && go build -tags mas ./...`.
- **MCP 게이트는 `//go:build !mobile`이다.** `mas`를 제외하지 않는다 — 컴패니언이 사라지면 MAS 빌드의 유일한 AI 경로가 MCP이기 때문이다.
- MCP는 **기본 꺼짐**. 사용자가 설정에서 켜기 전에는 어떤 리스너도 바인딩하지 않는다.
- 어떤 쓰기 경로도 `storyops.ApplyOps` / `manuscriptedit`를 우회하지 않는다. 스냅샷, 멘션 재동기화, 원고 재색인이 거기 있다.
- `storycontext`와 `storyops`는 `tars/pkg/llm`·`pkg/agentloop`·`pkg/session`을 import하지 않는다(`remember` 기록과 메모리 recall이 쓰는 `pkg/memory`는 두 패키지 모두 허용 — 또는 인터페이스 주입으로 tars 무관하게 유지). `go list -deps` 검증을 CI에 넣는다.
- UI가 호출하는 새 엔진 메서드는 `apps/desktop/src-tauri/src/lib.rs:16`의 `RENDERER_ENGINE_METHODS`에 추가해야 한다. 빠뜨리면 UI가 조용히 실패한다.
- 새 알림은 `apps/desktop/src-tauri/src/ffi.rs:215`의 `notification_event`에 추가하고 **동시에** `useEngineEvent` 리스너를 붙여야 한다.
- **1~4단계 동안 컴패니언은 그대로 동작한다.** 제거는 6단계에서만 일어난다.

---

## Phase 0 — 결정 확정

구현 전에 설계 문서 10절의 항목을 확정한다. 코드 작업 없음.

- [x] 컴패니언 제거 시점: **MCP 실사용 검증 후 단계적** (Phase 5 → 6)
- [x] 모바일에서 AI 기능이 완전히 사라지는 것을 **수용**
- [x] `web_search` 설정 **제거** (`web_fetch`는 유지)
- [x] 목표 버전 **1.0.0** 확정
- [x] 손으로 쓰는 요약 UI는 **미추가로 시작** (Phase 7에서 재검토)
- [x] 결정 결과를 설계 문서 10절에 반영

> **완료 (2026-08-22):** 다섯 항목 모두 권장안대로 확정. 이후 단계는 조건부 서술 없이 이 결정을 전제로 진행한다.

---

## Phase 1 — 스토리 코어 추출 (LLM에서 분리)

**이 단계에 사용자에게 보이는 새 기능은 없다.** 원칙은 "동작 변경 없는 이동"이고, 예외는 딱 두 가지 — 렌더러의 평문화(Task 1.1)와 팩트·메모리 병합(Task 1.3) — 이며 각각 명시적 작업으로 분리한다.

### Task 1.1 — `internal/storycontext` 추출과 렌더러 평문화

**파일:** `engine/internal/storycontext/*`(신규), `engine/internal/ai/*`(축소), 호출부

- [x] `Context`, `ContextSelection`, `ContextBuilder`, 프롬프트 렌더링(`buildSystem`/`buildUser` 계열)을 `internal/storycontext`로 이동한다.
- [x] **렌더러는 평문 문자열을 반환하게 바꾼다** (`RenderSystem`/`RenderUser`). 현재 `ai.BuildMessages`는 반환 타입으로 `tars/pkg/llm.ChatMessage`를 쓰는데, 내부는 문자열 두 개를 만드는 것뿐이다. `internal/ai`에는 `llm.ChatMessage`로 감싸는 얇은 `BuildMessages` 어댑터만 남겨 컴패니언·AI 실행기가 전환 기간 동안 그대로 돌게 한다.
- [x] `storycontext`가 `tars/pkg/llm`을 import하지 않는지 `go list -deps`로 검증한다(테스트 또는 CI 스크립트).
- [x] 기존 `ai` 테스트를 함께 옮기고 전부 통과시킨다.

### Task 1.2 — `internal/storyops` 추출

**파일:** `engine/internal/storyops/*`(신규), `engine/internal/companion/*`(축소), 호출부

- [x] `Proposal`, `validateProposal`, `ApplyOps`, undo 배치, 메모리 기록(`remember`) 경로를 `internal/storyops`로 이동한다. `remember`가 쓰는 `tars/pkg/memory` 의존은 유지된다.
- [x] `companion.Service`는 새 `storyops`를 호출하도록 바꾼다. 컴패니언 동작은 변하지 않는다.
- [x] `companion.apply_ops` / `companion.undo_apply` 핸들러는 그대로 두되 내부적으로 `storyops`를 쓴다.
- [x] 기존 적용/되돌리기 테스트는 컴패니언에 남겨 위임 경로를 종단으로 검증하게 하고(이동보다 강한 검증), storyops에 적용/되돌리기/롤백/스냅샷/메모리 부재 가드를 직접 검증하는 단위 테스트를 새로 추가했다.

### Task 1.3 — 컨텍스트 병합: 팩트·메모리·레퍼런스

설계 문서 3.1절. `ai.ContextSelection`에는 `Facts`/`Memories`/`References` 토글이 이미 있지만 실제 수집은 컴패니언 `gatherContext`에만 있다. 이 작업이 빠지면 MCP 브리프에 팩트북과 메모리가 빠진다.

**파일:** `engine/internal/storycontext/*`, `+ 테스트`

- [x] 컴패니언 `gatherContext`의 팩트(씬 필터 포함)·메모리(recall)·레퍼런스 수집을 `storycontext` 빌더의 선택적 섹션으로 이식한다.
- [x] `Context` 구조체에 `Facts`/`Memories`/`References` 필드를 추가하고 렌더러가 해당 섹션을 출력하게 한다(빈 섹션은 생략 — 기존 관례).
- [x] 기존 토글(`ContextSelection`)이 실제로 이 섹션들을 켜고 끄는지 테스트한다.
- [x] 컴패니언의 기존 프롬프트 조립은 건드리지 않는다 — 이 병합은 MCP 툴을 위한 것이고, 컴패니언은 6단계까지 자기 경로를 유지한다.

### Task 1.4 — 요약기 경계 정리

**파일:** `engine/internal/summarizer/*`

- [x] 비-LLM 경로(`minRunesForLLM` 미만 평문 요약)와 LLM 경로를 파일 단위로 분리한다.
- [x] `nodes.SetSummary(id, summary, contentVersion)`를 외부에서 호출할 수 있는 형태로 정리한다(3단계의 `linetta_write_summary`가 쓴다).
- [x] `nodes.update_content`의 `postUpdate` 훅 구조는 유지한다 — 6단계에서 훅이 부르는 대상만 비-LLM 요약기로 바뀐다.

**1단계 종료 조건:** `make test` 통과, 사용자에게 보이는 동작 변화 0, `storycontext`/`storyops`가 LLM 코드에 의존하지 않음이 `go list -deps`로 확인됨.

> **완료 (2026-08-22):** 엔진 40개 패키지 테스트 전부 통과, `mas`/`mobile` 태그 빌드 통과, 두 신규 패키지의 tars 의존 0 확인. 렌더러는 `Render(c) (system, user string)`로 평문화됐고 `internal/ai`에는 `BuildMessages` 어댑터만 남았다. 이 머신의 Smart App Control이 cgo(gcc cc1)와 일부 신규 테스트 바이너리를 간헐 차단해 `cmd/linetta-ffi` 검증은 CI에 맡긴다.

---

## Phase 2 — MCP 호스트, 인증, 읽기 툴

읽기 전용 MCP 서버가 동작한다. 쓰기가 없으니 배관이 자리 잡는 동안 위험 반경은 0이다.

### Task 2.1 — SDK 의존성 추가

**파일:** `engine/go.mod`, `engine/go.sum`

- [x] `cd engine && go get github.com/modelcontextprotocol/go-sdk@v1.7.0`
- [x] `go build -tags mas ./...`가 SDK를 **링크하는지** 확인한다(MAS도 MCP를 쓴다).
- [x] `go test -tags mobile ./...`는 SDK를 링크하지 않아야 한다. `go list -deps -tags mobile ./... | grep modelcontextprotocol`이 비어야 한다. **확인: 0건.**

### Task 2.2 — 설정 키와 시크릿 토큰

**파일:** `engine/internal/settings/settings.go`, `secrets.go`, `+ 테스트`

- [x] `MCPMode`(`off`|`read_only`|`full`, 기본 `off`), `MCPPort`(기본 `7391`), `MCPProjectID`, `MCPConsentVersion`, `MCPConsentedAt`를 `Settings`와 `SettingsPatch`에 추가한다.
- [x] `MCPTokenSet bool`(읽기용 존재 플래그)과 시크릿 저장소를 통해 쓰는 `RegenerateMCPToken()`을 추가한다. 토큰 값 자체는 `settings.get`이 절대 반환하지 않는다 — `api_key` 처리 방식과 동일하다.
- [x] 테스트: `settings.get`이 토큰을 가리고 존재 플래그만 노출한다. 모드가 왕복한다. 알 수 없는 모드는 `off`로 떨어진다.

### Task 2.3 — `mcphost` 골격, 인증, 수명 주기

**파일:** `engine/internal/mcphost/host.go`, `auth.go`, `discovery.go`, `+ 테스트`

- [x] `mcphost.New(deps)`가 `*mcp.Server`와 `http.Server`를 만들고 설정된 포트로 `net.Listen("tcp", "127.0.0.1:"+port)` 한다. 저장된 클라이언트 설정이 재시작을 견디도록 포트는 고정이다.
- [x] 포트가 이미 사용 중이면 설정 화면이 "7391 포트가 사용 중입니다 — 다른 포트를 선택하세요"로 렌더링할 수 있는 타입 에러를 반환한다. **다른 포트로 조용히 넘어가지 않는다.**
- [x] 인증 미들웨어: 상수 시간 베어러 비교, `Origin`이 있는데 루프백이 아니면 거부, `Host`가 루프백이 아니면 거부.
- [x] `Start()`가 `$LINETTA_HOME/mcp.json`(권한 0600, `{port, token, pid, started_at}`)을 쓰고, `Stop()`이 삭제하며 리스너를 내린다. 설정 파일 `settings.json`과는 별개 파일이다.
- [x] 테스트: 토큰 없음 → 401, 토큰 틀림 → 401, `Origin: https://evil.test` → 403, 포트 점유 → 타입 에러, POSIX에서 디스커버리 파일 권한 0600, `Stop` 후 파일 삭제.

### Task 2.4 — `engineapp` 연결

**파일:** `engine/internal/engineapp/mcp_enabled.go`(`//go:build !mobile`), `mcp_disabled.go`(`//go:build mobile`), `engineapp.go`, `+ 테스트`

- [x] `gitsync_enabled.go` / `gitsync_disabled.go` 패턴을 그대로 따른다: `const mcpAvailable`, `setupMCP(deps) mcpController`.
- [x] RPC `mcp.status`, `mcp.enable`, `mcp.disable`, `mcp.regenerate_token`, `mcp.activity`를 등록한다. 비활성 쌍둥이는 `CodeMethodNotFound`를 반환한다.
- [x] 호스트의 `Stop`을 `a.closers`에 넣어 앱과 함께 리스너가 죽게 한다.
- [x] `handlers.Capabilities`에 `MCPAvailable`을 추가하고 `diagnostics.version` / `diagnostics.get`으로 노출한다.
- [x] 테스트: 모드 `off`면 아무것도 바인딩하지 않음, `mcp.enable` 후 `mcp.status`가 포트를 보고함, `Close()`가 포트를 반납함.

### Task 2.5 — 읽기 툴 9개

**파일:** `engine/internal/mcphost/tools_read.go`, `+ 테스트`

- [x] 설계 문서 5절의 읽기 툴 9개를 `mcp.AddTool`로 등록한다. 입출력을 타입 구조체로 선언해 스키마가 생성되게 한다.
- [x] `linetta_get_story_context`는 병합된 `storycontext` 빌더(Task 1.3 완료가 전제)로 브리프를 조립하고, 평문 렌더러로 마크다운을 만들어 "무엇이 포함됐는지" 요약과 함께 반환한다.
- [x] `linetta_read_scene`은 `content_version`을 반환하고, 설명에 쓰기에는 이 값이 필요하다고 명시한다.
- [x] `MCPProjectID` 범위 제한은 툴마다가 아니라 공용 헬퍼 한 곳에서 강제한다.
- [x] 테스트: 씨드된 임시 스토어로 각 툴 검증, 범위 밖 `project_id` 차단, `read_only` 모드에서 정확히 이 9개만 등록됨, **LLM 프로바이더가 설정되지 않은 상태에서 `linetta_get_story_context`가 요약만 빈 채 팩트·메모리를 포함한 완전한 브리프를 에러 없이 반환함**(전환의 전제가 이 테스트에 달려 있다).

### Task 2.6 — 활동 로그

**파일:** `engine/internal/store/migrations/*`, `engine/internal/mcphost/activity.go`, `+ 테스트`

- [x] `mcp_activity` 테이블 마이그레이션(`id, at, tool, project_id, target_id, ok, detail`).
- [x] 성공·실패 관계없이 모든 툴 호출을 기록한다. **계획에서 이탈:** 보존 한도를 스냅샷 정리 잡에 얹지 않고 삽입 직후 자체 트리밍(500행)으로 처리했다 — 움직이는 부품이 하나 줄고, 앱이 유휴 상태가 되지 않아도 상한이 지켜진다.
- [x] `mcp.activity` RPC가 최근 기록을 반환한다.

**2단계 종료 조건:** `claude mcp add --transport http linetta http://127.0.0.1:7391/mcp --header "Authorization: Bearer <token>"`로 연결되고, 실제 Claude Code 세션이 작품 구조를 설명할 수 있다.

---

> **완료 (2026-08-22):** 엔진 42개 패키지 통과, `mas`/`mobile` 빌드 통과, mobile의 SDK 의존 0건. 테스트가 실제 HTTP 엔드포인트를 외부 클라이언트처럼 구동한다(initialize → tools/list → tools/call, SSE 파싱). 툴 등록은 활동 로그 데코레이터로 감싸 Task 2.6을 같이 끝냈고, 범위 제한은 씬 id 우회까지 막는 것을 확인했다.
>
> **남은 종료 조건:** 실제 Claude Code에서 `claude mcp add`로 붙여 작품 구조를 읽는 왕복 — 사용자 기기에서 직접 해야 하는 단계다.

## Phase 3 — 쓰기 툴과 안전장치

### Task 3.1 — `linetta_write_scene`

**파일:** `engine/internal/mcphost/tools_write.go`, `+ 테스트`

- [x] 쓰기 전 스냅샷 저장소로 자동 스냅샷을 만들고 `nodes.UpdateContentIfVersion`을 호출한다.
- [x] `ErrContentConflict`는 "씬을 다시 읽고 최신 `content_version`으로 재시도하라"는 문구의 툴 에러가 된다.
- [x] 크기 상한을 넘는 본문은 명확한 메시지로 거부한다.
- [x] 테스트: 정상 경로가 쓰기와 스냅샷을 남김, 낡은 버전 → 충돌 에러이며 DB 불변, 초과 크기 → 거부.

### Task 3.2 — `linetta_revise_scene`

- [x] `manuscriptedit`의 미리보기 + 적용을 감싸 씬 전체를 재전송하지 않고 부분 수정한다.
- [x] 테스트: 정확한 범위에 적용되고 스냅샷이 남음, 일치 항목 없음은 쓸모 있는 에러를 반환.

### Task 3.3 — `linetta_apply_story_ops`

- [x] 기존 `Proposal` 옵 어휘를 받아 `storyops.ApplyOps`를 그대로 호출한다.
- [x] **`set_scene_text` 옵은 거부하고 `linetta_write_scene`으로 안내한다.** 적용기의 `set_scene_text`는 `nodes.UpdateContent`(무조건 덮어쓰기)를 쓰므로, 이 툴로 통과시키면 `write_scene`의 버전 검사 계약을 우회하게 된다. 변경 종류마다 문은 하나여야 한다.
- [x] `batch_id`, 생성된 id, 옵별 실패를 컴패니언 결과와 동일한 형태로 반환한다.
- [x] 테스트: 아웃라인 배치가 적용되고 되돌릴 수 있음, 잘못된 옵은 배치를 실패시키고 아웃라인을 복원함.

### Task 3.4 — `linetta_write_summary`

**전환의 급소다.** 설계 문서 6절 참조.

- [x] 대상을 셋 받는다: 씬(leaf) 요약, 컨테이너(부/장) 요약 — 계층 컨텍스트의 재료 — 그리고 작품 시놉시스(`project.Update`의 `Synopsis` 경유).
- [x] **버전 계약 확정:** 씬(leaf)만 `content_version`을 요구한다. 컨테이너와 시놉시스는 자식 편집을 추적하는 버전이 없으므로 요구하지 않고 마지막 쓰기가 이긴다 — 툴 설명에 명시한다.
- [x] 노드 요약은 에이전트가 읽은 시점의 `content_version`을 인자로 받아 `nodes.SetSummary(id, summary, contentVersion)`에 그대로 넘긴다. 이 낡음 감지는 **씬(leaf) 전용이다** — 컨테이너는 자식 편집을 추적하는 버전이 없다(기존 코드도 컨테이너에는 버전 0을 쓴다). 컨테이너 요약의 버전 의미는 구현 시 확정한다.
- [x] 테스트: 요약 저장 후 `SummaryForVersion == ContentVersion`, 이후 사람이 본문을 고치면 요약이 다시 낡은 것으로 표시됨, 낡은 `content_version`으로 온 요약은 거부됨, 시놉시스가 저장됨.

### Task 3.5 — 체크포인트와 되돌리기

- [x] `linetta_create_checkpoint`는 에이전트가 준 라벨로 `snapshots.create_manual`을 감싼다.
- [x] `linetta_undo_last_change`는 `batch_id`(구조 변경)와 `snapshot_id`(본문 변경) 두 가지를 받는다. 만료된 배치는 "되돌리기 기간이 지났습니다"라는 평이한 메시지를 반환한다.

> **구현 중 확정 (계획 정정):** `storyops.UndoApply` → `nodes.RestoreOutline`은 `parent_id·ordinal·label·title·status`만 되돌리고 **`content_doc`은 건드리지 않는다**(삭제됐다가 복원되는 노드만 예외). 즉 구조 변경 undo는 씬 본문을 되돌리지 못한다. 본문 되돌리기는 스냅샷(버전 기록) 경로다. 툴이 이 차이를 감추면 에이전트에게 거짓 약속을 하게 되므로, `linetta_write_scene`은 결과에 `snapshot_id`를 실어 보내고 `linetta_undo_last_change`가 두 종류를 모두 받는다.

### Task 3.6 — 호출 한도와 모드 강제

- [x] 분당 호출 상한과 호출당 본문 상한을 기본값이 있는 상수로 둔다.
- [x] `read_only` 모드는 3.1~3.5의 어떤 툴도 등록하지 않는다. 모드별 `tools/list` 길이를 테스트로 못 박는다.

### Task 3.7 — 변경 알림

**파일:** `engine/internal/mcphost/*`, `apps/desktop/src-tauri/src/ffi.rs`, `apps/desktop/src/hooks/*`

- [x] 적용된 모든 변경 후 `mcp.changed`(`{project_id, node_ids, tool, batch_id}`)를 발신한다.
- [x] `notification_event`에 `"mcp.changed" => Some("mcp-changed")`를 추가한다.
- [x] 프론트엔드 리스너가 아웃라인 트리를 다시 가져오고, 열려 있는 씬이 영향을 받았고 편집 버퍼가 깨끗하면 본문도 갱신한다. **버퍼가 더러우면 덮어쓰지 않고 "에이전트가 이 씬을 변경했습니다" 배너를 띄운다.**
- [x] 테스트: 매핑에 대한 Rust 단위 테스트, 깨끗/더러움 분기에 대한 Vitest.

- [x] 씬 쓰기 전 스냅샷은 당분간 `snapshot.ReasonCompanionBefore`를 재사용한다. 새 reason을 추가하려면 `ValidReason`과 프론트엔드 버전 시트 라벨을 함께 손봐야 하고, 컴패니언이 Phase 6에서 사라지면 이 reason은 사실상 "에이전트 변경 전"이 된다. 전용 reason 도입은 Phase 4의 UI 작업과 함께 판단한다.

**3단계 종료 조건:** 인메모리 종단 테스트가 `initialize` → `tools/call linetta_write_scene` → `tools/call linetta_undo_last_change`(반환된 `snapshot_id`로)를 구동하고 원고가 원래 바이트로 돌아온다. 구조 변경은 `linetta_apply_story_ops` → `undo_last_change`(`batch_id`)로 별도 검증한다.

---

> **완료 (2026-08-23):** 툴 15개 완성(읽기 9 + 쓰기 6). `make test` 전체 통과, 엔진 42/42, `mas`/`mobile` 빌드 통과, mobile SDK 의존 0건.
>
> **계획에서 이탈한 두 가지:**
> - `expected_content_version`을 `int`가 아니라 `*int`로 받는다. 새 씬은 버전이 0이라 "미제공"과 "0"을 구분하지 못하면 **첫 원고를 영영 쓸 수 없다.** 테스트가 잡았다.
> - 씬 쓰기 전 스냅샷 reason은 `companion-before`를 재사용했다(위 결정 참조).
>
> **아직 남은 것:** `linetta_where_does_appear`는 명시적 @멘션만 집계하므로, 에이전트가 쓴 평문 원고에서는 등장인물 추적이 비게 된다. 이슈 #32와 같은 문제이고 Phase 3 이후 별도로 다룬다.

## Phase 4 — 브리지, 설정 UX, MAS

### Task 4.1 — `cmd/linetta-mcp` 브리지

**파일:** `engine/cmd/linetta-mcp/main.go`, `+ 테스트`

- [x] `$LINETTA_HOME/mcp.json`을 읽고, 고급 설정을 위한 `--url` / `--token` 재정의를 지원한다.
- [x] SDK의 `StreamableClientTransport`(로컬 엔드포인트, 인증 헤더 부착)와 SDK의 `StdioTransport` 서버 쪽을 합성한다. **단순 바이트 펌프가 아니다** — SSE 응답 스트림, 서버 발신 메시지용 GET 스트림, `Mcp-Session-Id` 상태를 SDK가 처리하게 맡긴다.
- [x] `--print-headers`를 추가한다. **JSON 헤더 객체**를 출력하고 종료하며, 생성된 `.mcp.json`이 `headersHelper`로 이걸 호출한다. 그래야 설정 파일에 리터럴 토큰이 남지 않는다.

> **계획 정정:** 계획에는 "`Authorization` 헤더 값만 출력"이라고 적혀 있었으나, Claude Code 문서의 `headersHelper` 계약은 **"stdout에 문자열 키/값 JSON 객체"** 다(10초 제한, 절대 경로 권장). 헤더 값만 내보내면 셸은 통과하지만 인증이 조용히 실패한다. 플래그 이름도 복수형 `--print-headers`로 맞췄고, 출력 형태를 테스트로 고정했다.
- [x] Linetta가 실행 중이 아닐 때 사람이 읽을 수 있는 메시지로 종료한다("Linetta를 열고 설정에서 MCP를 켜세요"). 이 문자열이 사용자가 클라이언트에서 보게 될 문구다.
- [x] 테스트: 실제 MCP 서버(스텁) 대상으로 `tools/list` + `tools/call` 왕복, 디스커버리 파일 없음 → 안내 메시지, 헤더 헬퍼 출력이 JSON 객체로 파싱됨, RoundTripper가 원본 요청을 변형하지 않음.

### Task 4.2 — 빌드와 번들

**파일:** `scripts/build-mcp-bridge.sh`, `Makefile`, `apps/desktop/src-tauri/tauri.conf.json`, `.github/workflows/*`

- [x] `make build-mcp-bridge`가 호스트 OS용으로 빌드하고, CI가 세 플랫폼 모두에서 같은 스크립트를 돌린다. 브리지는 cgo 없는 순수 Go라 `GOOS`/`GOARCH`만 넘기면 크로스 컴파일된다.
- [x] 직접 배포 빌드에서는 Tauri 리소스로 번들하고, `mcp_bridge_path` 커맨드가 절대 경로를 프론트엔드에 노출한다.
- [x] **MAS 빌드에서는 번들하지 않는다.** `tauri.mas.conf.json`이 `"resources": []`로 덮어쓴다. MAS 사용자는 릴리스 자산이나 Homebrew로 따로 받는다.
- [x] CI가 `linetta-mcp-macos` / `linetta-mcp-linux` / `linetta-mcp-windows.exe`를 릴리스 자산으로 올린다. 복사가 실패하면 잡이 죽으므로, 이것 자체가 브리지 없는 릴리스를 막는 게이트다.
- [x] `scripts/validate-distribution.sh`를 확장해 브리지 빌드/번들/자산 업로드 경로가 하나라도 빠지면 게이트에서 실패한다.

> **계획 정정 1 — 플랫폼별 설정은 리소스 목록을 *합치지 않고 통째로 갈아끼운다.** Tauri는 `tauri.<platform>.conf.json`을 RFC 7386(JSON Merge Patch)로 병합하는데, 이 규칙에서 배열은 병합이 아니라 **교체**다. `tauri.windows.conf.json`이 이미 `bundle.resources`를 엔진 DLL 맵으로 정의하고 있었으므로, 베이스의 `"resources/*"`는 **윈도우에서 죽은 설정**이었다. 즉 개발 주력 플랫폼에서 브리지가 아예 번들되지 않는다. 윈도우 맵에 `"resources/*": "resources"`를 함께 넣어 다른 OS와 배치가 같아지도록 맞췄다. 반대로 이 성질 덕분에 MAS 제외는 `"resources": []` 한 줄로 끝난다.

> **계획 정정 2 — 빈 글롭은 빌드 에러다.** `tauri-build`의 `copy_resources`는 `cargo build` 시점(build.rs)에 돌고, 매칭되는 파일이 하나도 없는 글롭은 `GlobPathNotFound`로 **실패**한다. 브리지를 아직 빌드하지 않은 새 체크아웃에서는 `pnpm tauri dev`조차 컴파일되지 않는다는 뜻이다. 그래서 `apps/desktop/src-tauri/resources/README.md`를 **의도적으로 커밋**해 글롭이 항상 하나는 잡게 했다. 파일 없이 체크아웃한 상태로 `cargo check`가 통과하는 것을 실제로 확인했다.

> **계획 정정 3 — macOS 번들 안의 브리지는 서명해야 한다.** 공증(notarytool)은 아카이브 안의 모든 Mach-O를 훑는데, Go가 darwin에 붙이는 ad-hoc 서명은 hardened runtime이 없어 공증에서 거부된다. CI의 브리지 빌드 단계에서 앱과 같은 Developer ID로 `--options runtime` 서명을 붙였다. 이 문제는 태그를 밀기 전까지는 드러나지 않는 종류라 미리 막았다.

> **알려진 한계 — 리눅스 AppImage.** AppImage는 실행할 때마다 임시 마운트 지점에 풀리므로 설정 화면이 찍어 주는 브리지 절대 경로가 다음 실행에서 무효가 된다. deb/rpm/직접 다운로드는 문제없다. AppImage 사용자는 릴리스 자산의 `linetta-mcp-linux`를 고정 경로에 두고 쓰면 된다. Phase 4의 블로커는 아니다.

### Task 4.3 — MAS 엔타이틀먼트

**파일:** `apps/desktop/src-tauri/*.entitlements`, `packaging/*`, `docs/`

> **이 태스크는 macOS 실기 검증이 필요하다.** 현재 개발 환경이 윈도우라 아래 항목과 함께, Task 4.2에서 만든 MAS의 `"resources": []` 병합이 실제 `make build-mas-local` 산출물에서 브리지를 빼는지도 macOS에서 확인해야 한다.

- [x] MAS 엔타이틀먼트에 `com.apple.security.network.server`를 추가한다.
- [ ] 샌드박스 빌드에서 루프백 리스너가 실제로 뜨는지 로컬 검증한다(`make build-mas-local`). **macOS 실기 필요 — 남은 유일한 Phase 4 항목.**
- [x] MAS에서는 브리지 없이 Claude Code HTTP 직접 연결 경로만 안내한다. `mcp_bridge_path`가 null을 돌려주면 설정 화면이 "이 빌드에는 브리지가 없다, 위의 `claude mcp add` 한 줄은 그대로 동작한다, Desktop을 쓰려면 릴리스나 Homebrew에서 받아라"를 3개 언어로 띄운다. 경로를 아직 물어보지 않은 상태(undefined)와 브리지가 없는 상태(null)를 구분해서, 로딩 중에 잘못된 안내가 깜빡이지 않는다.
- [x] 심사 설명 문구를 준비한다: 로컬 루프백 전용, 사용자가 명시적으로 켜야 함, 원격 접속 없음. `packaging/README.md`의 "Mac App Store review notes"에 정리했다.

### Task 4.4 — 설정 화면

**파일:** `apps/desktop/src/routes/Settings.tsx`, 신규 컴포넌트, `apps/desktop/src-tauri/src/lib.rs`, i18n 리소스

- [x] 토글, 모드 선택, 포트 입력, 작품 제한, 동의 문구, 토큰 재발급, 킬 스위치, 활동 목록.
- [x] 복사 가능한 스니펫 3종을 설정된 포트와 실제 브리지 경로로 생성한다. **리터럴 토큰은 `claude mcp add` 한 줄에만 담는다**(사용자 자기 기기에 쓰이므로). `.mcp.json` 스니펫은 `headersHelper` + `linetta-mcp --print-header`를 쓴다 — `.mcp.json`은 사람들이 커밋하는 파일이다.
- [x] 포트 점유 상태를 Task 2.3의 타입 에러로 렌더링하고 다른 포트를 고르게 한다. 엔진이 붙이는 `mcp_port_in_use` / `mcp_consent_required` reason 코드를 `rpc.Reason*` 상수로 등록하고, 렌더러의 `rpcErrorMessage`가 ko/en/ja 문장으로 옮긴다. 서버가 꺼져 있는 동안 포트 입력이 계속 활성이라 고치는 데 키 입력 한 번이면 된다.
- [x] `mcp.status`, `mcp.enable`, `mcp.disable`, `mcp.regenerate_token`, `mcp.activity`를 `RENDERER_ENGINE_METHODS`에 추가한다.
- [x] `capabilities.mcp_available`이 false면 화면 전체를 숨긴다.
- [x] 3개 언어 번역.
- [x] Vitest: 스니펫 렌더링, 동의 전에는 활성화가 막힘, 킬 스위치가 `mcp.disable`을 호출함, 포트 점유가 실행 가능한 문장으로 바뀜.

### Task 4.5 — 연결 표시등

- [x] 세션이 활성인 동안 워크스페이스에 눈에 띄지 않는 표시등을 띄우고, 클릭하면 활동 로그로 간다. 작가가 "뭔가 다른 것이 내 원고를 고칠 수 있다"는 사실에 놀라는 일이 없어야 한다.

### Task 4.6 — `.mcp.json`을 동기화에서 제외

**파일:** `engine/internal/gitsync/*`, `engine/internal/foldersync/*`, `+ 테스트`

- [x] gitsync의 `git add -A`에 `:(exclude).mcp.json` 경로 지정을 붙인다.
- [x] 테스트: 동기화 디렉터리의 `.mcp.json`이 스테이징되지 않고 살아남는다.

> **계획 정정:** "두 내보내기 모두"라고 적었지만 코드를 읽어보니 **foldersync는 제외할 것이 없다.** `exportAll`은 자기가 만든 원고 파일과 매니페스트만 쓰고 디렉터리의 다른 파일은 읽지도 복사하지도 않는다. 실제 유출 경로는 gitsync의 `git add -A` 하나뿐이고, 거기만 막으면 된다. 작가의 `.gitignore`를 대신 수정하지 않은 것은 그 파일이 작가가 관리하는 파일이기 때문이다.

**4단계 종료 조건:** 처음 쓰는 사용자가 MCP를 켜고 명령 한 줄을 붙여넣어 Claude Code 또는 Claude Desktop으로 자기 작품에 초고를 쓸 수 있다.

**상태:** 직접 배포 빌드(macOS / Windows / Linux)에서는 충족. 설정에서 MCP를 켜면 실제 브리지 절대 경로가 박힌 스니펫 3종이 나오고, Claude Code는 `claude mcp add` 한 줄, Claude Desktop은 번들된 브리지로 바로 붙는다. MAS 빌드만 macOS 실기에서 샌드박스 리스너 확인이 남아 있다(Task 4.3).

---

## Phase 5 — 컴패니언 강등과 검증

**아직 지우지 않는다.** MCP 경로가 실제로 컴패니언을 대체할 수 있는지 확인하는 단계다.

### Task 5.1 — 신규 사용자 기본값 전환

- [ ] 신규 설치의 온보딩에서 AI 설정 마법사 대신 **MCP 연결 안내**를 보여준다.
- [ ] 컴패니언과 프로바이더 설정은 기존 사용자에게만 보이게 하고, 설정 화면에서 "레거시" 섹션으로 내린다.
- [ ] 레거시 섹션에 전환 안내와 MCP 설정으로 가는 링크를 넣는다.

### Task 5.2 — 실사용 검증

- [ ] 실제 작품으로 씬 10개 이상을 MCP 경로만으로 집필하고, 컴패니언으로만 가능했던 작업이 있는지 기록한다.
- [ ] 요약 쓰기 흐름이 실제로 돌아가는지 확인한다 — 에이전트가 `linetta_write_summary`를 자발적으로 부르는가, 아니면 툴 설명을 고쳐야 하는가.
- [ ] 빠진 툴을 목록화하고, 15개 예산 안에서 추가할지 판단한다.

### Task 5.3 — 데이터 보존 경로

- [ ] 컴패니언 대화 히스토리와 메모리를 내보낼 수 있는 경로를 만든다(마크다운 내보내기 또는 읽기 전용 뷰).
- [ ] **테이블을 조용히 드롭하지 않는다.** 제거 후에도 데이터는 남고, 필요하면 읽을 수 있어야 한다.

**5단계 종료 조건:** MCP만으로 한 작품 분량의 실제 집필이 가능하다는 것이 확인됐고, 빠진 기능 목록이 비었거나 수용 가능하다.

---

## Phase 6 — 제거와 1.0.0

5단계 검증이 끝난 뒤에만 진행한다.

### Task 6.1 — 엔진 제거

- [ ] `internal/ai`의 LLM 클라이언트/실행기와 `BuildMessages` 어댑터, `internal/companion` 잔여(에이전트 루프·세션·스트리밍), `internal/modelcatalog`, `internal/openrouter`, `internal/clidetect` 삭제.
- [ ] `internal/summarizer`의 LLM 경로 삭제. 비-LLM 짧은 씬 경로와 `nodes.update_content`의 `postUpdate` 훅은 유지 — 훅이 부르는 대상만 바뀐다.
- [ ] RPC `ai.*`, `companion.*`, `providers.*`, `openrouter.*` 제거. `handlers.Capabilities`의 `UnavailableProviders` 정리.
- [ ] **RPC `projects.rewrite_synopsis` 제거.** 프로바이더 없는 상태에서 이 메서드는 컨테이너 요약을 지우고 빈 문자열을 돌려주는 파괴적 동작이 된다(설계 문서 3.3절). `projects.clear_synopsis`는 무해하므로 유지 여부만 판단.
- [ ] 설정에서 `provider`, `providers`, `ai_data_sharing_consent_*` 제거. 마이그레이션은 기존 값을 무시하되 파괴하지 않는다.
- [ ] `tars` 의존성은 **유지한다** — `pkg/tools`의 `web_fetch`가 팩트북 URL 캡처에 쓰이고(`handlers/facts.go:108`), `storyops`의 `remember`가 `pkg/memory`를 쓴다. `pkg/llm`, `pkg/agentloop`, `pkg/session` 사용만 사라진다.
- [ ] `handlers/websearch.go`, `web_search.test` RPC, `web_search_*` 설정을 제거한다(Phase 0 확정). `web_fetch`(`handlers/facts.go:108`, 키 불필요)는 남긴다.

### Task 6.2 — 프론트엔드 제거

- [ ] `components/ai/*`, `components/companion/*`, `hooks/useCompanion*` 삭제.
- [ ] Settings의 LLM 섹션, AI 온보딩 마법사, 관련 단축키·명령 팔레트 항목 삭제.
- [ ] `ffi.rs`의 `notification_event`에서 `ai.*` / `companion.*` 매핑 제거, `lib.rs`의 `RENDERER_ENGINE_METHODS`에서 해당 메서드 제거.
- [ ] `Cmd/Ctrl+J`를 비우거나 다른 집필 기능에 재할당한다.

### Task 6.3 — 문서와 스토어 문구

- [ ] README를 다시 쓴다: "AI is optional" 섹션을 "당신의 에이전트와 함께 쓰기(MCP)"로 교체.
- [ ] `docs/privacy-policy.md`에서 AI 프로바이더로 원고를 전송한다는 문구를 제거하고, MCP를 켠 경우 외부 클라이언트가 가져간다는 설명으로 바꾼다.
- [ ] `docs/DEVELOPMENT.md`의 "AI companion tools" 절을 MCP 툴 카탈로그로 교체.
- [ ] Mac App Store 설명, 스크린샷, CHANGELOG 갱신.

### Task 6.4 — 1.0.0

- [ ] `make bump-version VERSION=1.0.0`
- [ ] 마이그레이션 안내 문서: 기존 컴패니언 사용자가 MCP로 넘어오는 방법, 히스토리 데이터가 어떻게 되는지.

**6단계 종료 조건:** 앱이 어떤 LLM도 직접 호출하지 않고, 프로바이더 설정 화면이 존재하지 않으며, `make test` / `make test-mobile-engine` / `go build -tags mas ./...`가 전부 통과하고, `go list -deps`에 `tars/pkg/llm`·`pkg/agentloop`·`pkg/session`이 나타나지 않는다.

---

## Phase 7 — 순수 창작 도구 강화 (후속)

제거로 확보한 여력을 집필 기능에 투자한다. 이 계획의 범위 밖이지만 방향을 적어 둔다.

- [ ] `contextualedit`(설정 변경 → 관련 씬 일괄 수정) 같은 결정론적 파워 기능 확장 — LLM 없이 동작하며 이 제품 방향의 대표 기능이다.
- [ ] 손으로 쓰는 요약 UI — Phase 0에서 미추가로 시작하기로 확정했으므로, MCP 없이 쓰는 사용자 비중을 보고 여기서 재검토한다
- [ ] 집필 통계, 원고 진행 관리, 퇴고 워크플로
- [ ] MCP 프롬프트("다음 씬 초고", "연속성 점검")와 리소스(`linetta://work/{id}/scene/{id}`)
- [ ] `--headless` 엔진 모드 — 앱을 열지 않고도 에이전트가 작업

---

## 검증 계약

각 단계 종료 시:

```bash
make test
make test-mobile-engine
cd engine && go build -tags mas ./...
```

2·3단계는 인메모리 MCP 종단 테스트를 추가로 통과해야 한다. 4단계는 개발자 기기에서 Claude Code와 Claude Desktop 양쪽 수동 왕복을 추가로 요구한다. 6단계는 위 세 명령에 더해, LLM 호출 경로가 하나도 남지 않았음을 `go list -deps`로 확인한다.
