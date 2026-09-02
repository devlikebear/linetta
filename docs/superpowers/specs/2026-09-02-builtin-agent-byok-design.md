# 내장 AI 에이전트 (BYOK) — 설계 문서

**작성일:** 2026-09-02
**상태:** 제안
**로드맵 위치:** 서브프로젝트 1/4 (아래 3절)
**선행 문서:** `docs/superpowers/specs/2026-08-22-mcp-first-pivot-design.md`

## 1. 무엇을 만드는가

Linetta 안에서, 외부 도구 없이, 작가가 지시하고 에이전트가 씬을 쓰는 **대화형 에이전트 패널**을 만든다. 모델은 작가가 가져온 키 또는 Codex 구독(BYOK)으로 호출한다.

**전제 하나가 나머지 전부를 결정한다.** 내장 에이전트는 새 툴 레이어를 갖지 않는다. 1.0에서 만든 MCP 툴 16개를 **인프로세스로 호출하는 자기 MCP 서버의 클라이언트**다. 스냅샷, 되돌리기, 활동 로그, 동시 편집 보호, 호출 한도가 그대로 따라오고, 툴을 고치면 Claude Code와 내장 에이전트가 동시에 좋아진다.

이 문서가 끝나면 작가는 이렇게 쓴다.

1. 설정 → 연결 → **AI 프로바이더**에서 Codex 로그인 또는 API 키 입력, 동의 체크.
2. 작품을 열고 `Cmd/Ctrl+J`로 에이전트 패널을 연다.
3. "4-2 씬 컨텍스트 읽고 확립된 문체로 초고 써줘. 요약도 갱신해."
4. 씬이 바뀌는 걸 에디터에서 보고, 마음에 안 들면 패널의 **되돌리기** 한 번.

## 2. 왜 이 모양인가

**1.0 전환의 논리는 여전히 옳다.** 큐레이션된 브리프와 안전한 쓰기 경로가 제품의 본체이고, 그 위에 어떤 에이전트가 오든 같은 결과를 내야 한다. 이번 설계는 그 논리를 뒤집는 게 아니라 **에이전트 하나를 더 붙이는 것**이다. 다만 그 에이전트가 앱 안에 산다.

**옛 컴패니언을 되살리지 않는다.** 컴패니언은 자체 프롬프트 빌더, 자체 툴 레이어, 제안 펜스 파싱, 인텐트 게이팅을 따로 가진 병렬 경로였고, 그래서 9,800줄이 됐고, 그래서 지웠다. 이번에는 툴 레이어가 하나뿐이다. 에이전트가 하는 일은 "모델에 툴 스키마를 주고, 툴 호출이 오면 MCP 서버에 넘기고, 결과를 돌려주는" 루프뿐이다.

**1.0 설계서 12절 비목표의 정정.** "Linetta가 MCP 클라이언트가 되는 방향"은 **외부 서버를 소비하는 것**을 막은 문장이다. 자기 서버를 인프로세스로 소비하는 것은 그 반대가 아니라 그 연장이다. 외부 MCP 서버 소비는 여전히 비목표다.

**BYOK가 앱에 지우던 부담은 범위로 통제한다.** 1.0이 지운 것 중 되살리지 않는 것: 온보딩 마법사, OpenRouter OAuth, CLI 탐지, 프로바이더별 모델 카탈로그 큐레이션, 웹 검색 설정. 되살리는 것은 프로바이더 4개, 키 저장, 모델 목록 조회, 연결 테스트, 동의뿐이다.

## 3. 로드맵 — 서브프로젝트 넷

독립된 서브시스템이 넷이라 스펙을 나눈다. 이 문서는 1번이다.

| # | 서브프로젝트 | 내용 | 상태 |
| --- | --- | --- | --- |
| 1 | **BYOK 프로바이더 + 인프로세스 에이전트 + 채팅 패널** | 이 문서. 이것만으로 "Linetta 혼자 AI 드리븐 소설"이 성립한다 | 설계 완료 |
| 2 | 메모리 | 글자 수 제한된 작가 프로필(전역)과 작품 노트(작품별)를 시스템 프롬프트에 주입, 메모리 툴, 기존 `experiences.jsonl`은 무제한 로그로 유지 | 설계 예정 |
| 3 | 스킬과 자가개선 루프 | `SKILL.md` 저장소(전역/작품별), 읽기·관리 툴 2개, 툴 호출 N회 후 넛지, 응답 뒤 백그라운드 리뷰, 설정의 스킬 패널. Hermes 에이전트 모델 | 설계 예정 |
| 4 | 후속 | 세션 검색(FTS), BYOK 백그라운드 요약기, 모바일 내장 에이전트, MCP 프롬프트 | 미정 |

2·3번의 툴은 전부 MCP 툴로 노출한다. 그래야 Claude Code로 쓰든 내장 에이전트로 쓰든 같은 메모리가 쌓이고 같은 스킬이 자란다. 1번의 아키텍처가 그 전제를 만든다.

## 4. 아키텍처

### 4.1 내장 에이전트 = 자기 MCP 서버의 인프로세스 클라이언트

```
작가 ── 에이전트 패널 ── RPC agent.run ──▶ internal/agent
                                            │  llm.Client.Chat(messages, tools)
                                            │        │ tool_calls
                                            ▼        ▼
                                   mcp.Client ──in-memory──▶ mcp.Server
                                                              │ ToolDeps (mcphost)
                                                              ▼
                                       storycontext · storyops · manuscriptedit · snapshot
                                                              │
                                            mcp.changed ──▶ UI (기존 경로)
```

- `mcphost.ToolDeps.Register(server, MCPModeFull)`로 **두 번째 `mcp.Server` 인스턴스**를 만들고, go-sdk의 `mcp.NewInMemoryTransports()`로 클라이언트를 붙인다. HTTP 호스트와는 별개 인스턴스다. 포트도, 토큰도, Origin 검사도 없다. 외부 MCP가 꺼져 있어도 내장 에이전트는 동작한다.
- 내장 에이전트는 **항상 full 모드**다. 외부 `mcp_mode`의 `read_only`는 외부 클라이언트를 위한 것이다. 내장 에이전트가 원고를 못 쓰면 이 기능의 존재 이유가 없다.
- 툴 스키마는 `mcp.Client.ListTools`로 받아 `llm.ToolSchema`로 변환한다. 툴 설명에 이미 담긴 작업 흐름("초고 전에 `linetta_get_story_context`를 부르라", "쓴 뒤 요약을 갱신하라")이 그대로 모델에 전달된다. 설명을 두 벌 유지하지 않는다.
- `mcp_project_id` 작품 제한은 내장 에이전트에도 적용하지 않는다. 패널이 열린 작품이 곧 범위이고, 그 범위는 4.4절의 스코프 라인으로 전달한다. 다른 작품을 건드리는 것은 툴 입력의 `project_id`가 막지 않지만, 활동 로그에 남고 되돌릴 수 있다.

### 4.2 패키지

| 패키지 | 책임 | 빌드 태그 | tars 의존 |
| --- | --- | --- | --- |
| `internal/provider` (신규) | 설정 → `llm.Client` 해석, 모델 목록 조회, 연결 테스트, 프로바이더 가용성 | 없음 | `pkg/llm` |
| `internal/codexauth` (신규) | Codex OAuth PKCE 로그인, `auth.json` 보관, 로그아웃, 상태 | 없음 | 없음 (순수 HTTP) |
| `internal/agent` (신규) | 인프로세스 MCP 클라이언트, 에이전트 루프, 실행 관리, 스트리밍 알림, 트랜스크립트 | `!mobile` | `pkg/llm` |
| `internal/mcphost` (수정) | 활동 로그에 `source`/`run_id` 추가, `record` 래퍼가 컨텍스트에서 읽음 | `!mobile` | 없음 |
| `internal/settings` (수정) | `Provider`/`Providers` 필드 복원, 화이트리스트 갱신, 프로바이더별 동의 | 없음 | 없음 |
| `rpc/handlers/agent.go`, `providers.go`, `codex.go` (신규) | RPC 표면 | `!mobile` (agent만) | 없음 |

**의존성 게이트를 없애지 않고 좁힌다.** `scripts/validate-story-core-deps.sh`는 지금 엔진 전체에서 `pkg/llm`·`agentloop`·`session`을 금지한다. 바꾼 뒤 규칙:

- `pkg/agentloop`, `pkg/session`은 **엔진 전체에서 계속 금지**. 루프는 직접 쓴다(4.3절).
- `pkg/llm`은 `internal/provider`와 `internal/agent`에서만 허용. `go list -deps`로 `storycontext`, `storyops`, `mcphost`, `rpc/handlers`가 `pkg/llm`을 끌어오지 않음을 검증한다. 핸들러는 `provider`가 노출하는 인터페이스만 본다.

### 4.3 에이전트 루프

tars `pkg/agentloop`를 쓰지 않는 이유: 그 루프는 tars의 `tools.Registry`를 전제하므로 어차피 MCP 어댑터를 써야 하고, 넛지·백그라운드 리뷰(서브프로젝트 3)·취소·활동 로그 연동을 우리 손에 두는 게 낫다. 루프 자체는 200~300줄이다.

```
run(ctx, projectID, nodeID, prompt):
  transcript.append(user)
  msgs = system + history(projectID, budget) + scopeLine + prompt
  for iter in 1..maxIters(24):
    resp = client.Chat(ctx, msgs, {Tools, OnDelta → notify agent.delta})
    transcript.append(assistant text)  (스트리밍 중 status=streaming, 끝나면 done)
    if no tool_calls: break
    for call in resp.ToolCalls:
      notify agent.tool {run_id, name, args 요약, state=started}
      result = mcp.CallTool(ctx with run_id, call)      ← ToolDeps.record가 활동 로그에 source=agent 기록
      transcript.append(tool event)
      notify agent.tool {…, state=done|error, batch_id?, node_ids?}
      msgs += assistant(tool_calls) + tool(result, 잘림 처리)
  notify agent.done {run_id, usage}
```

규칙:

- **턴당 툴 호출 상한 24회.** 넘으면 루프를 끊고 "한도에 닿았다"고 응답에 적는다. 폭주 에이전트는 씬 40개를 고쳐 쓰기 전에 벽에 부딪혀야 한다. `mcphost.limiter`(분당 한도)는 **내장 에이전트용 별도 인스턴스**를 둔다. 외부 클라이언트와 한도를 나눠 쓰다가 서로를 굶기면 안 된다.
- **툴 결과 잘림.** 결과 하나당 상한(초기값 24k 문자)을 넘으면 앞부분만 넘기고 잘렸다고 표시한다. `linetta_read_scene`이 긴 씬을 돌려주는 경우가 정상이라 상한은 넉넉하되 무한하지 않다.
- **취소.** `agent.cancel(run_id)`는 컨텍스트를 취소한다. 진행 중이던 툴 호출은 그 툴의 원자성에 맡긴다(`write_scene`은 스냅샷 후 단일 업데이트라 반쯤 쓰인 씬은 없다). 취소된 턴의 부분 응답은 status=cancelled로 남긴다.
- **동시 실행 금지.** 한 작품에 한 번에 하나의 run만. 두 번째 요청은 `-32011 agent_busy`.
- **대화 히스토리 예산.** 이전 턴은 user·assistant 텍스트만 다시 넣고 툴 결과는 넣지 않는다. 최근 턴부터 거꾸로 채워 약 40k 문자에서 자른다. 요약 압축은 하지 않는다(서브프로젝트 4에서 세션 검색과 함께 재검토).
- **모델 선택은 턴마다 설정에서 읽는다.** 설정을 바꾸면 다음 턴부터 적용된다. 엔진 재시작 없음.

### 4.4 시스템 프롬프트와 스코프

시스템 프롬프트는 짧다. 브리프는 프롬프트에 넣지 않는다. 외부 에이전트와 똑같이 `linetta_get_story_context`를 불러서 얻는다. 그래야 툴 설명의 작업 흐름이 실제로 검증된다.

시스템 프롬프트 구성:

1. 정체성 한 문단: Linetta 안에서 작가와 함께 소설을 쓰는 에이전트. 작가가 마지막 결정권을 가진다.
2. 언어: 앱 언어(ko/en/ja)로 대답하고, 원고는 작품의 언어로 쓴다.
3. 작업 규칙 세 줄: 초고 전에 컨텍스트를 읽는다. 쓴 뒤 요약을 갱신한다. 확신 없는 큰 개작은 먼저 `linetta_create_checkpoint`를 만든다.
4. (서브프로젝트 2·3이 붙으면) 메모리 블록, 스킬 목록.

유저 메시지 앞에 **스코프 라인**을 붙인다: `[작품: {project_id} "{제목}"] [열린 씬: {node_id} "{라벨}"]`. 패널이 열린 작품과 현재 에디터의 씬이다. 에이전트가 "이 씬"이 뭔지 묻지 않게 하는 것이 목적이고, 그 이상은 툴로 알아낸다.

### 4.5 UI 갱신 흐름

에이전트가 쓴 변경은 툴이 `mcp.changed`를 쏘므로 **워크스페이스 갱신 경로를 새로 만들지 않는다.** 편집 중인 씬 보호(더티 배너), 아웃라인 리프레시, Story World 갱신이 이미 그 알림에 붙어 있다. 패널 자체를 위한 알림만 추가한다(6절).

## 5. 프로바이더와 BYOK

### 5.1 목록 — 넷, 그 이상은 없다

| id | 표시명 | 인증 | tars 매핑 | 비고 |
| --- | --- | --- | --- | --- |
| `openai-codex` | ChatGPT (Codex) | OAuth (5.3절) | `openai-codex` | 구독으로 프론티어 모델. 기본 프로바이더 |
| `anthropic` | Anthropic | API 키 | `anthropic` | |
| `gemini-native` | Google Gemini | API 키 | `gemini-native` | 무료 키가 있어 가장 낮은 진입 장벽 |
| `openai` | OpenAI 호환 | API 키 + base URL | `openai` | base URL 비우면 OpenAI. OpenRouter, Ollama, LM Studio는 base URL로 |

빠지는 것과 이유:

- `claude-code-cli`: 외부 CLI가 필요하다. 이 기능의 목적은 외부 도구 없이 쓰는 것이다. Claude 구독 토큰을 직접 쓰는 것은 Anthropic ToS 위반이라 OAuth 경로도 없다.
- `openrouter` 별도 항목: OpenAI 호환의 base URL 프리셋 하나로 충분하다. OAuth PKCE 연결은 되살리지 않는다.
- `kimi`, `antigravity-cli`: 수요 없음.

**모델 선택**: 프로바이더별 기본 모델(tars 기본값을 그대로 씀) + `providers.list_models`로 받은 목록 + 자유 입력. 프론트엔드에 모델 카탈로그를 두지 않는다.

### 5.2 설정 데이터 모델

1.0이 지우지 않고 남겨둔 필드를 되살린다. `Config.Provider`, `Config.Providers`, `ProviderConfig`의 Deprecated 표기를 지우고 patch 표면에 다시 올린다.

```go
type ProviderConfig struct {
    Model       string `json:"model,omitempty"`
    APIKey      string `json:"api_key,omitempty"`       // patch 입력 전용, 저장은 SecretStore
    APIKeySet   bool   `json:"api_key_set,omitempty"`   // settings.get 응답의 존재 플래그
    ClearAPIKey bool   `json:"clear_api_key,omitempty"` // patch 입력 전용
    BaseURL     string `json:"base_url,omitempty"`      // openai 전용
    ConsentedAt int64  `json:"consented_at,omitempty"`  // 5.4절
}
```

- 화이트리스트: `openai-codex`, `anthropic`, `gemini-native`, `openai`. 디스크에 남아 있는 `claude-code-cli`/`openrouter` 항목은 **읽을 때 버리지 않고 쓸 수 없게만** 한다(1.0의 라운드트립 원칙 유지). `Provider`가 화이트리스트 밖이면 `openai-codex`로 본다.
- `CliPath`는 제거한다. 쓰는 프로바이더가 없다.
- API 키는 기존 `SecretStore`(macOS 키체인, Windows Credential Manager, Linux 파일)에 `providerAPIKeySecretName(provider)`로 저장한다. 1.0 마이그레이션 문서대로 지우지 않은 키가 있으면 **그대로 다시 보인다**. 이게 이번 설계에서 사용자가 얻는 첫 보상이다.
- `settings.json`에는 키가 절대 쓰이지 않는다. 기존 `migrateLegacySecrets` 경로가 이를 보장한다.

### 5.3 Codex OAuth (PKCE) — 앱 안에서 로그인한다

tars의 `openai-codex` 프로바이더는 `auth.json`을 읽고 만료되면 **토큰 갱신을 직접** 한다. 없는 것은 최초 로그인뿐이고, 그것을 `internal/codexauth`가 담당한다. tars를 바꾸지 않는다.

흐름:

1. `codex.login_start` → PKCE verifier/challenge(S256)와 state 생성, `127.0.0.1:1455`에 콜백 리스너를 연다. 인가 URL을 돌려주면 셸이 OS 브라우저로 연다(v0.9.6에서 만든 경로).
2. 브라우저에서 ChatGPT 로그인 → 콜백 → 코드를 토큰 엔드포인트에서 교환 → `access_token`, `refresh_token`, `id_token`. `account_id`는 `id_token`의 `https://api.openai.com/auth` 클레임 안 `chatgpt_account_id`에서 꺼낸다.
3. `$LINETTA_HOME/codex/auth.json`(0600)에 **Codex CLI와 같은 파일 형식**으로 쓴다. tars에는 `ProviderOptions.AuthConfig.CodexHome = $LINETTA_HOME/codex`를 넘긴다(`resolveCodexAuthPath`가 이 오버라이드를 첫 순위로 읽는 것을 확인했다).
4. `codex.login_status` → 로그인 여부, 계정 이메일(id_token 클레임), 만료.
5. `codex.logout` → 파일 삭제.

프로토콜 상수 — Codex CLI 소스(`openai/codex`, `codex-rs/login/src/{server.rs, auth/manager.rs, token_data.rs}`, 2026-09-02 기준)에서 확인한 값이다. 구현은 이 값을 상수로 두고, 환경 변수 오버라이드를 두지 않는다.

| 항목 | 값 |
| --- | --- |
| 발급자 | `https://auth.openai.com` |
| 인가 | `{발급자}/oauth/authorize` |
| 토큰 교환 | `{발급자}/oauth/token` (POST, `grant_type=authorization_code`) |
| 클라이언트 id | `app_EMoamEEZ73f0CkXaXp7hrann` (tars 갱신 코드와 동일) |
| redirect_uri | `http://localhost:1455/auth/callback`, 1455가 막히면 `1457` 폴백 (Codex CLI와 같은 규칙) |
| scope | `openid profile email offline_access api.connectors.read api.connectors.invoke` |
| 추가 쿼리 | `id_token_add_organizations=true`, `codex_cli_simplified_flow=true`, `originator=linetta` |
| `auth.json` | `{"auth_mode":"chatgpt","tokens":{"id_token","access_token","refresh_token","account_id"},"last_refresh":<RFC3339>}` — tars는 `tokens.access_token/refresh_token/account_id`만 요구한다 |

세부:

- 1455와 1457이 모두 사용 중이면 조용히 다른 포트로 가지 않는다. redirect URI가 등록된 값이라 다른 포트는 어차피 실패한다. "로그인 포트(1455/1457)가 사용 중입니다. Codex CLI 로그인 창을 닫고 다시 시도하세요"를 띄운다(`codex_port_in_use`).
- 콜백은 `state`가 일치할 때만 받고, 성공 뒤 브라우저에는 "Linetta로 돌아가세요" 한 줄짜리 로컬 성공 페이지를 보여준다. Codex의 호스트된 성공 페이지로 리다이렉트하지 않는다.
- 로그인 대기는 5분에 끊는다(`codex_login_timeout`). 창을 닫으면 리스너도 닫힌다.
- 리프레시 토큰 저장소: tars는 macOS에서 `security` CLI로 키체인을 쓰려 한다. 샌드박스에서 실행 파일 호출이 막히므로 Linetta는 `TARS_OPENAI_CODEX_REFRESH_TOKEN_STORAGE=file`을 프로세스 환경에 고정해 **파일 모드**로 통일한다. 파일은 앱 데이터 디렉터리 안 0600이며, Codex CLI 자신도 같은 방식으로 보관한다. 이 결정은 개인정보 문서에 적는다.
- **폴백**: `$LINETTA_HOME/codex/auth.json`이 없고 `~/.codex/auth.json`이 있으면 그것을 쓴다(`!mas`). 이미 Codex CLI로 로그인한 사람은 클릭 하나 덜 한다. MAS는 컨테이너 밖을 못 읽으므로 폴백이 없다.
- ToS 메모: OpenAI는 Codex 구독 OAuth의 서드파티 재사용을 Anthropic처럼 명시 금지하지 않는다. tars가 같은 방식을 이미 쓴다. 정책이 바뀌면 이 프로바이더만 빼면 된다. 설계가 프로바이더 하나에 묶이지 않는 이유다.

### 5.4 동의 모델 — 프로바이더별

1.0이 "Linetta는 원고를 어디로도 보내지 않는다"고 약속했으므로, 보내기 시작하는 순간에 **작가가 그 사실을 문장으로 읽고 체크**해야 한다.

- 동의는 `ProviderConfig.ConsentedAt`에 프로바이더별로 기록한다. OpenAI에 동의한 것이 Anthropic 동의가 아니다(v0.9.3의 교훈).
- 동의 문장은 프로바이더 이름과 무엇이 전송되는지를 적는다: "이 프로바이더에 현재 씬 본문, 요약, 등장인물·플롯·팩트 카드 등 에이전트가 툴로 읽은 내용이 전송됩니다. Linetta는 그 밖의 어떤 곳에도 보내지 않습니다."
- 동의 없는 프로바이더로 `agent.run`이 오면 `-32012 provider_consent_required`. 패널은 설정으로 안내한다.
- 전역 `ai_data_sharing_consent_*` 필드는 그대로 두되 읽지 않는다(라운드트립만). 프로바이더별로 옮겼기 때문이다.

### 5.5 플랫폼

| 빌드 | 결과 |
| --- | --- |
| 데스크톱 직접 배포 | 전부 |
| Mac App Store | 전부. `network.client`(API 호출)와 `network.server`(1455 콜백)가 이미 엔타이틀먼트에 있다. `~/.codex` 폴백만 없음 |
| 모바일 | 없음. `internal/agent`는 `!mobile`. `provider`·`codexauth`는 태그 없이 컴파일되지만 호출부가 없다. 서브프로젝트 4에서 툴 레이어를 HTTP 호스트에서 떼면 열린다 |

## 6. RPC와 알림

`apps/desktop/src-tauri/src/lib.rs`의 `RENDERER_ENGINE_METHODS`에 추가한다. 빠뜨리면 UI가 조용히 실패한다.

| 메서드 | 입력 | 출력 |
| --- | --- | --- |
| `providers.list` | — | 이 빌드에서 쓸 수 있는 프로바이더 목록과 각각의 상태(키 있음/로그인됨/동의됨/기본 모델) |
| `providers.list_models` | `{provider}` | `{models: string[]}`. 실패는 RPC 에러, UI는 자유 입력으로 폴백 |
| `providers.test` | `{provider}` | 짧은 챗 한 번. 성공/실패 이유 코드 |
| `codex.login_start` | — | `{auth_url}` |
| `codex.login_status` | — | `{logged_in, email?, expires_at?}` |
| `codex.logout` | — | — |
| `agent.run` | `{project_id, node_id?, prompt}` | `{run_id}` 즉시 반환, 진행은 알림 |
| `agent.cancel` | `{run_id}` | — |
| `agent.history` | `{project_id, limit}` | 최근 메시지와 툴 이벤트 |
| `agent.clear` | `{project_id}` | 트랜스크립트 삭제(활동 로그는 남는다) |
| `agent.undo` | `{batch_id}` | `storyops.UndoApply`. 패널의 되돌리기 버튼 |

프로바이더 설정 자체는 기존 `settings.set` patch로 한다(`provider`, `providers[p]`).

알림(`ffi.rs`의 `notification_event`와 `useEngineEvent` 리스너 **동시에** 추가):

| 이름 | 페이로드 |
| --- | --- |
| `agent.delta` | `{run_id, text}` |
| `agent.tool` | `{run_id, name, state: started/done/error, summary, batch_id?, node_ids?}` |
| `agent.done` | `{run_id, usage: {input, output}}` |
| `agent.error` | `{run_id, reason, message}` |
| `agent.cancelled` | `{run_id}` |

`ai.*` 이름은 다시 쓰지 않는다. 옛 리스너 잔재와 섞이지 않게 한다.

에러 이유 코드(기존 `rpcErrorMessage` 번역 경로): `provider_not_configured`, `provider_consent_required`, `provider_auth_failed`, `provider_rate_limited`, `provider_unreachable`, `agent_busy`, `agent_iteration_limit`, `codex_port_in_use`, `codex_login_timeout`.

## 7. 저장

**트랜스크립트는 `companion_messages`를 재사용한다.** `companion.HistoryRepo`가 살아 있고 필드(`ProjectID`, `NodeID`, `RunID`, `Role`, `Scope`, `Status`, `Content`)가 그대로 맞는다. 툴 이벤트는 `Role="tool"`, `Content`에 JSON(`{name, summary, ok, batch_id, node_ids}`)으로 넣는다. 새 테이블을 만들지 않는다. 1.0의 "기록 보존" 내보내기(`export.companion_history`)가 새 대화도 자동으로 담는다.

**활동 로그 확장** — 마이그레이션 `0017_mcp_activity_source.sql`: `mcp_activity`에 `source TEXT NOT NULL DEFAULT 'external'`, `run_id TEXT` 컬럼 추가. `ToolDeps.record`가 컨텍스트 값에서 읽는다. 설정의 활동 로그가 "내장 에이전트"와 "외부 클라이언트"를 구분해 보여주고, 패널은 `run_id`로 한 턴의 변경을 묶어 되돌리기 버튼을 붙인다.

## 8. UI

기억할 원칙: 플랫한 단일 사이드바, 중첩 없음, 여백. 옛 컴패니언의 액션 팔레트·인텐트 칩·스코프 선택기는 돌아오지 않는다.

### 8.1 설정 → 연결 → AI 프로바이더

`McpSection`과 같은 자리(연결 카테고리), 같은 결의 새 `ProviderSection`.

- 프로바이더 4개를 세그먼트로 고른다. 선택한 것이 활성 프로바이더다.
- 선택에 따라 필드가 바뀐다: Codex는 **[ChatGPT로 로그인]** 버튼과 상태 줄(이메일, 로그아웃), 나머지는 API 키(비밀 입력, 저장됨 표시, 지우기), OpenAI 호환은 base URL 추가.
- 모델: 콤보박스(목록 새로고침 + 자유 입력). 비우면 기본 모델.
- 동의 체크박스 한 개, 프로바이더 이름이 들어간 문장. 체크 전엔 **[연결 테스트]**가 비활성.
- 연결 테스트 결과 한 줄.
- 활동 로그는 `McpSection`의 것을 공유하되 출처 열이 생긴다.

### 8.2 에이전트 패널

- `Cmd/Ctrl+J`로 토글(1.0에서 비워둔 바인딩). 워크스페이스 우측 패널 자리(팩트북·컨텍스트 패널과 같은 슬롯, 하나만 열림).
- 프로바이더가 없거나 동의가 없으면 패널 본문이 설정으로 가는 안내 하나로 대체된다.
- 메시지 목록: 사용자, 에이전트(스트리밍, `useSmoothStream` 재사용), 그리고 **툴 줄**. 툴 줄은 접힌 한 줄이다: "읽음 · 4-2 씬 / 스토리 컨텍스트", "씀 · 4-2 씬 **되돌리기**". 인자 전체를 보여주지 않는다.
- 되돌리기는 `batch_id`가 있는 툴 줄에만 붙고 `agent.undo`를 부른다. 이미 되돌린 것은 상태를 바꾼다.
- 작성기: 여러 줄, `Enter` 전송, `Shift+Enter` 줄바꿈. 실행 중엔 **정지** 버튼으로 바뀐다.
- 시작 칩 셋(작성기에 문장을 채우기만 한다): "현재 씬 초고", "연속성 점검", "다음 씬 제안". 서브프로젝트 4의 MCP 프롬프트가 오면 그것으로 대체한다.
- 턴이 끝나면 사용량 한 줄(입력/출력 토큰). 비용 계산은 하지 않는다. 숫자를 보여주는 것으로 충분하다.
- 패널을 닫아도 run은 계속되고, 다시 열면 `agent.history`로 복원한다.

## 9. 안전장치

1.0의 것을 전부 상속하고 둘을 더한다.

- 상속: 쓰기 전 자동 스냅샷, `expected_content_version` 충돌(에이전트는 다시 읽고 재시도하라는 툴 에러를 받는다), 편집 중 씬 보호 배너, 활동 로그, `linetta_undo_last_change`.
- 추가 1: 턴당 툴 호출 24회, 결과 잘림, 작품당 동시 실행 1.
- 추가 2: 프로바이더별 동의. 동의 없이는 단 한 바이트도 나가지 않는다. `providers.test`도 동의 뒤에만.

킬 스위치는 설정에서 프로바이더를 지우는 것이다. 별도 토글을 두지 않는다.

## 10. 에러 처리

- 프로바이더 에러(`llm.ProviderError`)는 이유 코드로 매핑해 알림 `agent.error`로 보낸다. 원문 JSON은 UI에 노출하지 않는다(v0.8.5의 교훈). 설정 화면의 연결 테스트도 같은 매핑을 쓴다.
- 인증 실패는 패널에 "설정에서 키/로그인을 확인하세요" 링크를 단다.
- Codex 토큰 갱신 실패는 `provider_auth_failed`로 오고, 패널이 "다시 로그인"을 안내한다.
- 툴 에러(예: 버전 충돌)는 **모델에게 돌려주는 것**이 기본이다. 모델이 다시 읽고 재시도한다. 같은 툴이 같은 에러를 3번 연속 내면 루프를 끊고 사용자에게 보여준다.
- 엔진이 죽거나 스트림이 끊기면 진행 중 메시지는 status=failed로 남기고 패널은 재시도 버튼을 보여준다.

## 11. 테스트

Go (`engine`):

- `provider`: 설정 → `ProviderOptions` 매핑(4개 프로바이더, base URL, CodexHome), 화이트리스트 밖 값의 관용 처리, 동의 없는 클라이언트 생성 거부.
- `settings`: `provider`/`providers` patch 라운드트립, 프로바이더별 동의 저장, 키 존재 플래그, `claude-code-cli` 잔재가 저장 시 살아남음.
- `codexauth`: `httptest`로 인가·토큰 엔드포인트 흉내, PKCE 검증, 파일 형식과 권한, 1455 점유 시 에러, 타임아웃, 폴백 경로(`!mas`).
- `agent`: 가짜 `llm.Client`(스크립트된 툴 호출 시퀀스) + **진짜 인메모리 MCP 서버 + 진짜 저장소**(`mcp_tools_test.go`의 픽스처 재사용). 씬을 읽고 쓰고 요약을 갱신하는 시나리오가 끝까지 돌고, 활동 로그에 `source=agent`와 `run_id`가 찍히고, `mcp.changed`가 나가는지 확인. 24회 상한, 취소, 결과 잘림, 동시 실행 거부, 히스토리 예산.
- `mcphost`: `record`가 컨텍스트의 source/run_id를 기록, 없으면 `external`.
- 의존성 게이트: 갱신된 스크립트가 `storycontext`/`storyops`/`mcphost`/`handlers`의 `pkg/llm` 유입을 잡고, `agentloop`/`session`을 전역에서 잡는다.
- 빌드: `make test`, `make test-mobile-engine`, `go build -tags mas ./...`, `GOOS=windows go build ./...`.

프론트엔드 (Vitest):

- `ProviderSection`: 프로바이더별 필드 전환, 동의 전 테스트 비활성, patch 형태, Codex 로그인 상태 표시.
- `AgentPanel`: 스트리밍 렌더, 툴 줄과 되돌리기, 정지, 미설정 상태의 안내, 히스토리 복원.
- 알림 이름과 `RENDERER_ENGINE_METHODS` 항목에 대한 allowlist 테스트(기존 `rpcAllowlist.test.ts` 확장).

## 12. 문서와 문구

1.0이 세 곳에 쓴 "Linetta는 모델을 호출하지 않고 키를 저장하지 않는다"를 고친다. 새 문장: "Linetta는 **작가가 연결한 프로바이더에만** 원고를 보내며, 그마저도 작가가 프로바이더별로 동의한 뒤에만 보낸다."

- `README.md` 상단 문단, "Writing with your own agent (MCP)" 절 옆에 "Writing with the built-in agent (BYOK)" 절.
- `docs/privacy-policy.md`(ko/en/ja): 전송 대상, 전송 내용, 동의, Codex 토큰 파일 보관.
- `apps/site`: 1.0 때 "코드가 지원하지 않는 MCP 주장 세 개"를 고친 이력이 있다. 같은 기준으로 이번엔 **구현 후에** 문구를 바꾼다.
- `CHANGELOG.md` 1.2.0. 스토어 설명은 릴리스 단계에서.
- `docs/migrating-to-1.0.md`에 "1.2에서 키가 다시 쓰인다"는 한 문단.

## 13. 비목표

- 온보딩 마법사, 첫 실행 넛지, 레스큐 카드(2026-06-21 스펙). 설정 한 화면으로 끝낸다.
- OpenRouter OAuth, CLI 탐지, `claude-code-cli`, Claude 구독 OAuth.
- 인라인 에디터 보조(고스트 텍스트, 선택 영역 고쳐쓰기). 대화형이 먼저다.
- 메모리·스킬 툴(서브프로젝트 2·3). 단, 4.1의 구조가 그것들을 MCP 툴로 붙일 자리를 만든다.
- 외부 MCP 서버 소비, 웹 검색 설정.
- 모바일 내장 에이전트, 백그라운드 요약기, 세션 검색(서브프로젝트 4).
- 비용 추정·예산 알림. 토큰 수만 보여준다.

## 14. 이 문서가 스스로 내린 결정 (거부 가능)

브레인스토밍에서 확정된 것 외에 설계자가 정한 것들이다. 하나라도 다르게 가고 싶으면 이 표를 고친다.

| 결정 | 선택 | 대안 |
| --- | --- | --- |
| 루프 구현 | 직접 작성 (`internal/agent`) | tars `pkg/agentloop` + 레지스트리 어댑터 |
| Codex 로그인 위치 | Linetta `internal/codexauth` (tars 무변경) | tars `internal/auth`에 추가 후 재노출 (릴리스 사이클 + Windows 체크 필요) |
| 트랜스크립트 저장 | `companion_messages` 재사용 | 새 `agent_messages` 테이블 |
| 툴 이벤트 저장 | 트랜스크립트의 `role=tool` 행 + 활동 로그 `run_id` | 활동 로그만 |
| 동의 단위 | 프로바이더별 | 전역 하나 |
| 내장 에이전트 모드 | 항상 full | 외부 `mcp_mode` 따라감 |
| 기본 프로바이더 | `openai-codex` | `gemini-native` (무료 키) |
| 히스토리 예산 | 최근 턴 텍스트만 40k 문자, 압축 없음 | LLM 요약 압축 |
| 패널 위치·단축키 | 우측 패널 슬롯, `Cmd/Ctrl+J` | 하단 드로어 |
| 버전 | 1.2.0 | 2.0.0 |
