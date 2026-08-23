# MCP 우선 전환 — 설계 문서

**작성일:** 2026-08-22
**상태:** 제안
**구현 계획:** `docs/superpowers/plans/2026-08-22-mcp-first-pivot.md`

## 1. 무엇을 바꾸려는가

지금 Linetta는 앱 안에 BYOK 방식으로 LLM 프로바이더를 설정하고, `Cmd/Ctrl+J` 컴패니언으로 AI 협업을 제공합니다. 이 문서는 그 구조를 뒤집는 전환을 설계합니다.

**전환 후:**

- **Linetta = 순수 창작 도구.** 사람이 직접 소설을 쓰는 것을 돕는 데 집중합니다. 앱 자체는 LLM을 호출하지 않고, API 키를 요구하지 않고, 토큰을 쓰지 않습니다.
- **AI 협업 = MCP 클라이언트.** Claude Desktop, Claude Code 등 사용자가 이미 쓰는 에이전트가 MCP로 Linetta에 접속해 원고를 읽고 씁니다.

즉 AI를 앱 안에 넣는 대신, **앱을 AI가 일할 수 있는 작업장으로 만드는** 방향입니다. 작가는 책상과 서류함과 거부권을 계속 쥐고 있고, 에이전트는 외부에서 고용된 집필자로 들어옵니다.

## 2. 왜 이 전환인가

**BYOK가 앱에 지우는 부담이 실제로 큽니다.** 프로바이더 설정 화면, 모델 카탈로그, OpenRouter OAuth, CLI 탐지, API 키 보관, 프로바이더별 실패 메시지 번역, AI 데이터 공유 동의, AI 온보딩 마법사 — 이 전부가 "소설 쓰기"가 아니라 "LLM 배관"입니다.

**사용자는 이미 에이전트를 갖고 있습니다.** Claude 구독자는 Claude Code/Desktop을 씁니다. Linetta 안에서 API 키를 또 넣고 토큰을 또 결제하는 건 중복입니다.

**외부 에이전트가 앱 내장 컴패니언보다 강합니다.** 최신 모델, 긴 컨텍스트, 서브에이전트, 파일 시스템 접근, 웹 검색을 이미 갖추고 있습니다. Linetta가 따라잡을 필요가 없습니다.

**Linetta만 줄 수 있는 것은 따로 있습니다.** 일반 파일시스템 MCP 서버도 마크다운은 읽고 씁니다. 못 하는 건 특정 씬 하나를 위해 조립된 큐레이션 컨텍스트입니다 — 아웃라인, 계층 요약, 직전 씬 요약, 등장인물·관계 브리프, 플롯 스파인, 팩트북 카드, 메모리, 문체·시점·분량 목표. 이 브리프가 이 제품의 본체이고, 외주 원고가 14화와 모순되지 않게 돌아오는 이유입니다.

## 3. 제거 경계 — 패키지가 아니라 기능으로 나눈다

**가장 중요한 설계 판단입니다.** MCP 서버의 핵심 툴들은 지금 제거 후보로 보이는 패키지 위에 서 있습니다.

- 스토리 컨텍스트 → `ai.ContextBuilder` (`internal/ai`)
- 스토리 옵 적용/되돌리기 → `companion.Service.ApplyOps` / `UndoApply` (`internal/companion`)

따라서 "`internal/ai`와 `internal/companion`을 지운다"는 계획은 **자기 발을 쏘는 계획**입니다. 경계는 패키지가 아니라 **LLM 루프 vs 스토리 오퍼레이션/컨텍스트**입니다.

### 3.1 컨텍스트 조립 경로가 지금 두 개라는 사실

코드를 확인한 결과, 오늘의 Linetta에는 컨텍스트 조립 경로가 **둘** 있고 서로 다른 것을 담습니다.

| 경로 | 담는 것 | 안 담는 것 |
| --- | --- | --- |
| `ai.ContextBuilder` → `ai.Context` (씬 중심) | 현재 씬 본문, 직전 씬 요약, 계층(부/장) 요약, 인접·관련 씬, 엔티티·관계 브리프, 플롯 스파인, 여백 노트, 문체 목표 | **팩트, 메모리, 레퍼런스** |
| `companion.gatherContext` → `PromptData` (작품 중심) | 아웃라인 노드 id, 씬 발췌, 스레드, 엔티티, 관계, **팩트, 메모리, 레퍼런스** | 계층 요약, 직전 씬 요약, 관련 씬 RAG |

`ai.ContextSelection`에는 `Facts`/`Memories`/`References` 토글이 이미 있지만, 실제 수집은 컴패니언 쪽에만 있습니다. **추출은 이동이 아니라 병합입니다** — `internal/storycontext`는 `ai.ContextBuilder`를 기반으로 하되, 컴패니언의 팩트·메모리·레퍼런스 수집을 이식해 토글을 실제로 완성해야 합니다. 이걸 명시하지 않으면 구현자가 계획서 문면대로 따라가서 팩트북과 메모리가 빠진 브리프를 만들게 됩니다.

### 3.2 살려서 옮기는 것 (추출)

| 지금 위치 | 옮길 곳 | 내용 |
| --- | --- | --- |
| `internal/ai` — `Context`, `ContextSelection`, `ContextBuilder`, 프롬프트 렌더링 | `internal/storycontext` | 큐레이션된 스토리 브리프 조립. LLM 호출 없음 |
| `internal/companion` — 팩트·메모리·레퍼런스 수집 (`gatherContext`의 해당 부분) | `internal/storycontext` | 위 병합의 재료 |
| `internal/companion` — `Proposal`, `validateProposal`, `ApplyOps`, undo 배치, 메모리 기록(`remember`) 경로 | `internal/storyops` | 구조화된 스토리 변경 적용 + 되돌리기 |

**렌더러의 타입 의존성 주의:** `ai.BuildMessages`는 반환 타입으로 `tars/pkg/llm.ChatMessage`를 씁니다. 내부는 시스템/유저 문자열 두 개를 만드는 것뿐입니다(`buildSystem`/`buildUser`). 추출 시 `storycontext`는 **평문 문자열을 반환하는 렌더러**(`RenderSystem`/`RenderUser`)로 바꾸고, 전환 기간 동안 `internal/ai`에 `llm.ChatMessage`로 감싸는 얇은 어댑터만 남깁니다. 그래야 "storycontext는 LLM 코드를 import하지 않는다"는 검증이 성립하고, MCP 툴은 마크다운을 직접 렌더링할 수 있습니다.

이 추출이 끝나면 두 패키지의 나머지(프로바이더 글루, 에이전트 루프, 채팅 세션, 스트리밍)는 안전하게 제거할 수 있습니다.

### 3.3 제거하는 것

- `internal/ai`의 LLM 클라이언트 팩토리·실행기 (`client.go`, `runner.go`)와 프로바이더 글루
- `internal/companion` 잔여 — TARS 에이전트 루프, 채팅 히스토리, 스트리밍, 제안 대화 흐름
- `internal/modelcatalog`, `internal/openrouter`, `internal/clidetect`
- `internal/summarizer`의 LLM 경로 (짧은 씬용 비-LLM 경로는 유지 — 6절)
- RPC: `ai.*`, `companion.*`, `providers.*`, `openrouter.*`
- RPC: **`projects.rewrite_synopsis`** — 프로바이더가 사라진 뒤에는 이 메서드가 **파괴적으로 변합니다.** `DeriveProjectSynopsis(refresh=true)`는 컨테이너 요약을 먼저 지우고 요약기를 불러 다시 채우는데, 요약기가 없으면 지우기만 하고 빈 문자열을 돌려줍니다. 남겨두면 클릭 한 번으로 요약을 날리는 버튼이 됩니다. 시놉시스는 에이전트가 쓰기 툴로 채웁니다(5절).
- 설정: `provider`, `providers`, `ai_data_sharing_consent_*` (`web_search_*`는 Phase 0 결정에 따름)
- 프론트엔드: `components/ai/*`, `components/companion/*`, `hooks/useCompanion*`, Settings의 LLM 섹션, AI 온보딩 마법사

### 3.4 그대로 두는 것

- `internal/contextualedit` — **LLM을 전혀 쓰지 않습니다.** 생성자가 받는 건 엔티티·팩트·관계·원고 편집기·노드뿐입니다(`contextualedit.go:150`). 인물 설정을 바꾸면 관련 씬을 찾아 일괄 수정하는 이 기능은 오히려 "순수 창작 도구" 방향의 대표 기능입니다.
- `internal/manuscriptedit`, `snapshot`, `search`, `manuscript`(FTS), `plot`, `mention`, `stats`, `backup`, `gitsync`, `foldersync`
- **`tars` 의존성 자체는 남습니다.** `pkg/tools`의 `NewWebFetchTool`이 팩트북 URL 캡처에 쓰이고(`handlers/facts.go:108`), 메모리 기능이 `remember` 옵으로 유지되므로 `pkg/memory`도 남습니다. 빠지는 건 `pkg/llm`, `pkg/agentloop`, `pkg/session` 사용입니다.

> **규모에 대한 정직한 표기:** 제거 규모를 "패키지 LOC 합계"로 세면 과장됩니다. `internal/companion` 9,800줄 중 상당 부분이 `storyops`와 `storycontext`로 살아남습니다. 계획서는 규모를 기능 단위로 적고, 실제 삭제 줄 수는 추출이 끝난 뒤 측정합니다.

## 4. MCP 서버 아키텍처

### 4.1 실행 중인 앱 안에서 호스팅한다

새 패키지 `engine/internal/mcphost`를 `engineapp.register`에 연결하고, 공식 Go SDK(`github.com/modelcontextprotocol/go-sdk` v1.7.0)의 Streamable HTTP 핸들러를 `127.0.0.1`에 붙입니다.

**별도 프로세스가 `library.db`를 직접 여는 방식은 기각했습니다.**

1. `engineapp.Open`이 백업 루프, 스냅샷 정리, 요약기, 폴더/Git 동기화를 무조건 시작합니다. 프로세스가 둘이면 일일 동기화가 이중 실행되고 같은 행을 두 번 처리합니다.
2. `store.Open`이 열 때마다 `ApplyMigrations`를 돌립니다. 버전 업그레이드 직후 두 프로세스가 마이그레이션 전 백업과 마이그레이션 자체를 경합합니다.
3. **UI에 프로세스 간 갱신 경로가 없습니다.** 알림은 Go → C 콜백 → `notify_trampoline` → `app.emit` → `useEngineEvent`(`apps/desktop/src-tauri/src/ffi.rs:182`)로 흐르는 프로세스 내부 전용 경로입니다. 외부 프로세스가 고친 씬을 작가는 모른 채 계속 타이핑하게 됩니다.
4. SQLite WAL은 다중 프로세스를 견디지만, `store.Open`의 `db.SetMaxOpenConns(1)`은 **프로세스 안에서만** 직렬화합니다.

나중에 `engineapp.Options{DisableBackgroundJobs: true}`를 받는 명시적 `--headless` 모드로 되살릴 수 있습니다(후속 단계).

### 4.2 포트는 고정, 임의 포트 아님

설정 `mcp_port`, 기본값 **7391**. 클라이언트 설정은 한 번 쓰고 몇 달을 씁니다. 임의 포트(`:0`)를 쓰면 앱을 재시작할 때마다 저장된 설정이 전부 죽고, Claude Code에는 URL이 바뀌는 것을 흡수할 클라이언트 측 수단이 없습니다(`headersHelper`는 헤더 전용입니다). 포트가 이미 사용 중이면 조용히 다른 포트로 넘어가지 말고 **"7391 포트가 사용 중입니다 — 다른 포트를 선택하세요"** 라고 화면에 띄웁니다.

고정 포트가 보안을 약화시키지는 않습니다. 엔드포인트를 지키는 건 포트의 비밀성이 아니라 베어러 토큰과 `Origin` 검사입니다.

### 4.3 전송 방식이 두 개 필요한 이유

| 클라이언트 | 연결 경로 |
| --- | --- |
| Claude Code | HTTP 직접: `claude mcp add --transport http linetta http://127.0.0.1:7391/mcp --header "Authorization: Bearer <token>"` |
| Claude Desktop | 번들된 `linetta-mcp` 브리지를 통한 stdio |
| 기타 MCP 클라이언트 | 지원하는 전송 방식에 따라 둘 중 하나 |

Claude Desktop은 현재 로컬 HTTP MCP 서버에 직접 붙지 못합니다. `claude_desktop_config.json`은 stdio 항목만 검증하고(`url` 필드는 조용히 버려집니다), 설정 → 커넥터 경로는 Anthropic 클라우드가 URL을 여는 구조라 공인 인증서가 달린 공개 HTTPS가 필요합니다. **따라서 Claude Desktop을 지원 대상으로 삼는 한 브리지는 선택이 아닙니다.**

브리지는 스토리 로직을 담지 않습니다. SDK의 `StreamableClientTransport`(로컬 엔드포인트로, 인증 헤더 부착)와 SDK의 `StdioTransport` 서버 쪽을 합성한 것뿐입니다. 단순 바이트 펌프가 아니라는 점이 중요합니다 — Streamable HTTP는 SSE 응답 스트림, 서버 발신 메시지를 위한 GET 스트림, `Mcp-Session-Id` 상태를 다룹니다. 그 처리는 전부 SDK에 맡깁니다. 툴이 바뀌어도 브리지는 함께 배포할 필요가 없습니다.

## 5. 툴 카탈로그 (15개)

엔진 RPC 100여 개를 1:1로 노출하지 않습니다. 클라이언트의 툴 예산은 유한하고, 툴이 늘수록 선택 정확도가 떨어집니다. 전부 `linetta_` 접두사를 씁니다.

### 읽기 (9)

| 툴 | 기반 | 비고 |
| --- | --- | --- |
| `linetta_list_works` | `projects.list` | 작품 id, 제목, 상태, 씬 수 |
| `linetta_get_outline` | `nodes.list_tree` | 트리 id, 라벨, 종류, 상태, 분량 |
| `linetta_get_story_context` | `storycontext` (병합된 빌더) | **핵심 툴.** 씬 하나를 위한 큐레이션 브리프 — 3.1절의 병합 완료가 전제 |
| `linetta_read_scene` | `nodes.get` | 본문과 **`content_version`** 반환 — 안전한 쓰기에 필수 |
| `linetta_search_manuscript` | `manuscript.search` | 원고 전문 검색 |
| `linetta_list_characters` | `entities.list` | `kind` 필터로 장소·사물·개념까지 |
| `linetta_where_does_appear` | `entities.scenes` | 특정 인물이 등장하는 씬 목록 |
| `linetta_get_plot` | `plot.spine_panel` | 스토리라인과 비트 |
| `linetta_get_fact_cards` | `facts.list` | 출처가 붙은 조사 노트 |

### 쓰기 (6)

| 툴 | 기반 | 비고 |
| --- | --- | --- |
| `linetta_write_scene` | `nodes.update_content` | `expected_content_version` 필수, 쓰기 전 자동 스냅샷 |
| `linetta_revise_scene` | `manuscript.replace_preview` + `replace_apply` | 씬 전체를 다시 보내지 않는 부분 수정 |
| `linetta_apply_story_ops` | `storyops.ApplyOps` | 기존 `Proposal` 옵 어휘를 배치로 |
| `linetta_write_summary` | `nodes.SetSummary` / `project.Update(Synopsis)` | **전환의 핵심.** 씬·컨테이너 요약과 작품 시놉시스를 모두 담당. 6절 참조 |
| `linetta_create_checkpoint` | `snapshots.create_manual` | 큰 개작 전 라벨 붙은 복원 지점 |
| `linetta_undo_last_change` | `storyops.UndoApply` | `batch_id`로 되돌리기 |

메모리는 툴 하나를 더 쓰지 않고 `linetta_apply_story_ops`의 기존 `remember` 옵으로 처리합니다.

### 설계 규칙

- **적용기를 재사용하고 병렬 쓰기 경로를 만들지 않습니다.** `ApplyOps`(`engine/internal/companion/tools.go:324`)는 제안 검증, 되돌리기용 아웃라인 캡처, 멘션 재동기화, 원고 재색인을 이미 수행합니다. 두 번째 쓰기 경로는 이 전부를 조용히 건너뜁니다.
- **동시 편집은 예외가 아니라 정상입니다.** 에이전트가 고쳐 쓰는 씬을 사람이 동시에 타이핑하는 상황이 기본 시나리오입니다. `nodes.update_content`에는 이미 `expected_content_version` → `ErrContentConflict` → JSON-RPC `-32009` 낙관적 동시성이 있습니다(`handlers/nodes.go:59`). MCP 툴은 이걸 "씬을 다시 읽고 최신 버전으로 재시도하라"는 툴 에러로 노출하며, 절대 조용히 덮어쓰지 않습니다.
- **툴 설명이 작업 흐름을 담습니다.** 쓰기 툴 설명에는 변경이 스냅샷되고 되돌릴 수 있다는 점, 초고를 쓰기 전에 `linetta_get_story_context`를 먼저 부르라는 점, 씬을 읽거나 쓴 뒤에는 `linetta_write_summary`로 요약을 갱신하라는 점을 적습니다.
- MCP **프롬프트**("다음 씬 초고", "연속성 점검")와 **리소스**(`linetta://work/{id}/scene/{id}`)는 후속입니다. 툴이 먼저입니다.

## 6. 요약 문제 — 이 전환의 급소

프로바이더를 제거하면 요약기가 멈춥니다. 그런데 요약은 `linetta_get_story_context`의 계층 요약과 직전 씬 요약을 채우는 재료입니다. 즉 **핵심 툴의 품질이 제거 대상에 의존합니다.**

**해법: 요약을 외부 에이전트가 써서 돌려보냅니다.** 에이전트는 어차피 다음 씬을 쓰기 위해 이전 씬을 읽습니다. `linetta_write_summary`로 그 결과를 저장하게 하면, 의존성이 협업 지점으로 바뀌고 요약 비용은 사용자의 기존 구독에 흡수됩니다.

구현 시 주의:

- `SetSummary(nodeID, summary, contentVersion)`에 **에이전트가 읽은 시점의 `content_version`을 넘겨야** 합니다. 그래야 사람이 이후에 본문을 고쳤을 때 `SummaryForVersion != ContentVersion`이 되어 요약이 다시 낡은 것으로 표시됩니다. 낡은 버전으로 온 요약은 거부합니다.
- 씬(leaf)뿐 아니라 **컨테이너(부/장) 요약도 같은 툴로** 씁니다 — 계층 컨텍스트가 컨테이너 요약에서 나옵니다. 작품 시놉시스도 이 툴이 담당합니다(`projects.rewrite_synopsis`는 제거되므로 — 3.3절).
- 짧은 씬은 지금도 LLM 없이 평문 요약이 저장됩니다(`summarizer.go`의 `minRunesForLLM` 분기). 이 경로와 `nodes.update_content`의 `postUpdate` 훅은 유지합니다 — 훅이 부르는 대상이 LLM 요약기에서 비-LLM 짧은 씬 요약기로 바뀔 뿐입니다.
- **정직한 폴백 고지:** MCP 클라이언트를 전혀 쓰지 않는 순수 수동 작가는 긴 씬에 대해 요약이 비어 있게 됩니다. 브리프의 나머지(아웃라인, 엔티티, 관계, 플롯 스파인, 팩트, 메모리, 문체 목표)는 전부 데이터베이스 상태라 항상 채워집니다. 손으로 요약을 적는 UI를 추가할지는 **결정 필요 사항**(10절)으로 남깁니다.

## 7. 보안과 동의

기본값 꺼짐. 설정 화면은 외부 에이전트가 무엇을 읽고 무엇을 바꿀 수 있는지 평이한 말로 설명합니다.

- **루프백 전용 바인딩**(`127.0.0.1`). LAN 바인딩, 터널, 원격 접속은 어느 단계에서도 없습니다.
- **베어러 토큰** 32바이트 난수. 기존 설정 시크릿 저장소(`engine/internal/settings/secrets*.go`)를 통해 보관합니다. 설정에서 재발급·폐기할 수 있고, 재발급하면 기존 클라이언트 설정은 전부 무효가 됩니다.
- **`Origin`/`Host` 검증.** `Origin`이 있는데 루프백이 아니면 거부합니다(MCP 명세의 DNS 리바인딩 방어). 그렇지 않으면 임의 웹페이지가 `127.0.0.1`로 POST할 수 있습니다.
- **토큰이 Git 원격으로 새면 안 됩니다.** 폴더 동기화 디렉터리에 놓인 `.mcp.json`은 Git sync가 그대로 커밋·푸시합니다. 방어 두 겹을 모두 적용합니다: 생성되는 `.mcp.json` 스니펫은 리터럴 토큰 대신 Claude Code의 `headersHelper`(`linetta-mcp --print-header`, 0600 디스커버리 파일을 읽음)를 쓰고, Git/폴더 동기화 내보내기에서 `.mcp.json`을 제외합니다.
- **디스커버리 파일** `$LINETTA_HOME/mcp.json`(설정 파일 `settings.json`과 별개), 권한 0600, `{port, token, pid, started_at}`. 종료 시 삭제합니다. 신뢰 경계: 같은 사용자로 실행되는 프로세스는 이미 `library.db`와 시크릿 저장소를 직접 읽을 수 있으므로 이 파일이 기준을 낮추지 않습니다.
- **모드 설정** — `off`(기본) / `read_only` / `full`. `read_only`에서는 쓰기 툴을 아예 등록하지 않으므로 `tools/list`에 나타나지도, 호출되지도 않습니다.
- **작품 제한** — `mcp_project_id`가 설정되면 모든 툴이 한 작품으로 제한됩니다.
- **모든 변경은 되돌릴 수 있습니다** — 씬 쓰기 전 자동 스냅샷, 구조 변경 전 아웃라인 캡처, `batch_id` 반환, `linetta_undo_last_change`.
- **활동 로그.** 모든 툴 호출(시각, 툴, 작품, 대상, 결과)을 기록하고 설정에서 보여줍니다. "내가 자는 동안 뭘 했나"에 대한 답입니다.
- **한도.** 호출당 본문 크기 상한, 분당 호출 상한. 폭주하는 에이전트 루프는 벽에 부딪혀야지 씬 40개를 고쳐 쓰면 안 됩니다.

**동의 모델은 오히려 단순해집니다.** 전환이 끝나면 `ai_data_sharing_consent_*`는 프로바이더와 함께 사라지고, MCP 동의 하나만 남습니다. Linetta가 원고를 어디로도 보내지 않고, 외부 클라이언트가 가져가는 구조이기 때문입니다.

## 8. 플랫폼별 결말

**데스크톱(직접 배포 macOS/Windows/Linux):** 전체 기능. 브리지 번들.

**Mac App Store:** 컴패니언이 사라지면 MAS 빌드에는 AI 경로가 **하나도** 남지 않습니다. 따라서 MAS의 MCP 지원은 선택이 아니라 필수 범위입니다. 빌드 게이트는 `!mas && !mobile`이 아니라 **`!mobile`**입니다.

- 샌드박스에 `com.apple.security.network.server` 엔타이틀먼트를 추가하면 루프백 리스너는 허용됩니다.
- 브리지 번들은 다릅니다. 외부 앱이 실행하는 두 번째 실행 파일을 샌드박스 앱 번들에 넣는 것은 심사 위험이 있고, 컨테이너 안 디스커버리 파일을 외부 프로세스가 읽을 수 있는지도 확실하지 않습니다. **확인되지 않은 것을 확인된 것처럼 쓰지 않습니다.**
- MAS 대응: Claude Code는 HTTP로 직접 붙습니다(포트 + 토큰 수동 붙여넣기). Claude Desktop용 브리지는 앱 번들이 아니라 Homebrew/GitHub 릴리스로 별도 배포합니다.

**모바일(iOS/iPad/Android):** MCP 서버를 호스팅하지 않습니다. 즉 **컴패니언 제거는 모바일에서 AI 기능이 완전히 사라지고 대체 경로가 없다는 뜻입니다.** 최근 iPad UX에 투자한 것을 고려하면 이건 각주가 아니라 명시적으로 확인받아야 할 결과입니다(10절).

## 9. 권장 작업 흐름 (문서화할 내용)

1. Linetta 설정에서 MCP를 켜고 `claude mcp add` 한 줄을 복사합니다.
2. Linetta 폴더 동기화를 어떤 디렉터리로 지정하고, **그 디렉터리에서** Claude Code를 실행합니다. 에이전트는 내보내진 마크다운(grep 가능한 산문)과 Linetta의 구조화된 툴(정본 스토리 상태)을 동시에 갖게 됩니다. 그 디렉터리에 `.mcp.json`을 두려면 `headersHelper`를 쓰고 동기화에서 제외해야 합니다 — Git sync가 푸시하는 바로 그 디렉터리이기 때문입니다.
3. 씬 단위로 지시합니다. "4-2 씬 컨텍스트를 읽고, 확립된 문체로 초고를 쓰고, 해소된 비트와 요약을 갱신해줘."
4. Linetta에서 검토합니다. 되돌리기는 툴 한 번 또는 클릭 한 번입니다.

## 10. 확정된 결정

**2026-08-22 확정.** 아래 다섯 항목은 전부 권장안대로 결정됐습니다. 이후 단계는 이 결정을 전제로 진행합니다.

| 항목 | 결정 | 근거 |
| --- | --- | --- |
| 컴패니언 제거 시점 | **MCP 실사용 검증 후 단계적** (Phase 5 → 6) | MCP 경로가 실사용에서 컴패니언을 대체함이 확인되기 전에 지우면, 사용자 손에는 AI 없는 앱만 남습니다 |
| 모바일에서 AI 완전 소멸 | **수용** | 모바일은 MCP 서버를 호스팅할 수 없어 대체 경로가 없지만, 두 갈래 유지는 전환의 목적을 무너뜨립니다. 모바일은 순수 집필 도구가 됩니다 |
| `web_search` 설정 | **제거** (`web_fetch`는 유지) | Brave/Perplexity 키도 결국 BYOK입니다. 검색은 에이전트가 더 잘합니다. `web_fetch`는 키가 필요 없고 팩트북 URL 캡처에 쓰이므로 남깁니다 |
| 손으로 쓰는 요약 UI | **미추가로 시작** (Phase 7에서 재검토) | MCP 없이 쓰는 사용자 비중을 보고 판단합니다 |
| 목표 버전 | **1.0.0** | 파괴적 변경이자 제품 정체성 전환입니다 |

## 11. 리스크

| 리스크 | 완화 |
| --- | --- |
| MCP 검증 전에 컴패니언을 지워 사용자가 빈손이 됨 | 단계 순서를 강제: 추출 → 구축 → 강등 → 검증 → 제거 |
| 추출 중 `ApplyOps`/컨텍스트 빌더 동작이 미묘하게 깨짐 | 추출은 **동작 변경 없는 이동**으로만 진행하고 기존 테스트를 그대로 통과시킴. 유일한 예외(팩트·메모리 병합, 렌더러 평문화)는 명시적 작업으로 분리 |
| 폭주 에이전트가 원고를 뒤엎음 | 모드 설정, 호출·크기 한도, 호출별 스냅샷, 되돌리기, 활동 로그, 킬 스위치 |
| 사람과 에이전트가 같은 씬을 편집 | `expected_content_version`, `-32009` 노출, 에디터 배너 |
| 에이전트가 요약 갱신을 빼먹어 브리프 품질 저하 | 쓰기 툴 설명에 요약 갱신 지시 포함, Phase 5 실사용 검증에서 실제 호출 여부 측정 |
| MAS 심사에서 로컬 서버가 문제됨 | 엔타이틀먼트만 사용하고 브리지는 번들 밖으로. 문제 시 MAS는 HTTP 직접 연결만 지원 |
| 기존 컴패니언 사용자의 데이터 | 히스토리·메모리 데이터는 **삭제하지 않고** 읽기 또는 내보내기로 보존 |
| Claude Desktop의 로컬 서버 정책 변화 | 브리지는 얇고 유지 비용이 낮음. HTTP 경로는 명세 표준 |

## 12. 비목표

- 원격/LAN 접속, 터널, OAuth, 다중 사용자.
- Linetta가 MCP **클라이언트**가 되는 방향(외부 MCP 서버를 앱이 소비). 이 전환의 정반대입니다.
- 모바일 MCP 호스팅.
- 집필 기능 자체의 재설계. 이 전환은 AI 경계를 옮기는 것이지 에디터를 다시 만드는 것이 아닙니다.
