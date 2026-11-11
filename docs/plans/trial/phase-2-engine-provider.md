# 체험 공급자 Phase 2 — 엔진 체험 공급자

_상위 로드맵: [`roadmap.md`](./roadmap.md)_
_작성일: 2026-11-11_
_목표: 웍스에 연결된 기기라면 키 없이 고를 수 있는 공급자 `linetta-trial`. 체험 턴은 도구 20개로 돌고, 한도를 다 쓰면 그 사실을 오류 이유로 알린다_
_예상 소요: 2일_
_전제: Phase 1이 개발 서버에 떠 있음(`LINETTA_PUBLISH_BASE_URL`로 가리킨다). [`tool-profile.md`](./tool-profile.md) 구현(#360) 병합_

## Goal

- 공급자 목록에 `linetta-trial`이 모든 빌드(직배포, Mac App Store, iOS)에 있다.
- 웍스 기기 연결이 있으면 `configured`다. API 키, 주소, 모델은 작가가 넣지 않고 엔진이 정한다.
- 이 공급자로 도는 턴은 체험 도구 20개를 받고 백그라운드 패스가 돌지 않는다.
- 한도를 다 쓰면 `provider_trial_exhausted`, 체험이 멈췄으면 `provider_trial_paused`로 실패한다. 화면은 이 이유로 "내 키 연결" 안내를 띄운다.
- `trial.status` RPC가 남은 토큰과 상태를 돌려준다.

## Non-goals

- 화면, 동의 문구, 문서 (Phase 3)
- 웍스 연결 흐름 자체. 게시용 연결(`publish.connect`)을 그대로 쓴다
- 체험 한도를 엔진이 세기. 원장은 서버에만 있다

## 설계

### 자격 증명은 게시 연결에서 빌린다

`provider.Resolve("linetta-trial")`은 `settings.ProviderConfigFor`가 아니라 게시 연결에서 값을 만든다.

| 필드 | 값 |
|---|---|
| `APIKey` | `settings.PublishToken()` (`lwk_…`) |
| `BaseURL` | `PublishAccount.SiteURL + "/api/trial/v1"` (연결이 없으면 `publish.BaseURLFromEnv()` 기준, 그래도 `configured`는 false) |
| `Model` | `linetta-trial` 고정. 서버가 실제 모델로 바꾼다 |
| `ConsentedAt` | 이 공급자 몫의 동의 시각. 다른 공급자처럼 `providers.<id>.consented_at`에 둔다 |

`Configured()`는 `PublishAccount`가 있고 토큰이 비밀 저장소에 있을 때 참이다. 게시 연결을 끊으면 체험도 같이 끊긴다.
`settings.set`은 이 공급자의 `api_key`, `base_url`, `model`을 받지 않는다(`invalid_settings`). 받을 수 있는 것은 동의와 활성화뿐이다.

`Options`는 tars의 OpenAI 호환 공급자(`openai`)로 만든다. 상위가 OpenAI 호환이고, 프록시가 그 모양 그대로 흘려 보내기 때문이다.

### 체험 턴인지는 그 턴의 공급자로 정한다

#360은 `Deps.TrialToolsEnabled func() bool`을 두고 배선은 비워 두었다. 그런데 `Run`은 클라이언트를 먼저 얻고(`s.deps.Providers.Client(ctx, "")`가 `resolved`를 돌려준다) 그 뒤에 도구 그룹을 읽는다.
활성 공급자 설정을 따로 다시 읽으면, 그 사이에 작가가 공급자를 바꿀 경우 체험 키로 49개 도구를 보내거나 그 반대가 된다.
그래서 이 Phase에서 `TrialToolsEnabled`를 없애고, `withTrial`이 `resolved.ID == settings.ProviderLinettaTrial`을 받게 바꾼다. `loop.go`와 `pass.go` 두 곳 모두 그 턴의 `resolved`를 쓴다.

### 오류 이유

프록시는 OpenAI 오류 모양으로 `{"error":{"code":"trial_exhausted"}}`를 429와 함께 돌려준다. 지금 `Classify`는 429를 `provider_rate_limited`로 바꾸므로, 그보다 먼저 본문의 코드를 본다.

| 프록시 코드 | 상태 | 이유 |
|---|---|---|
| `trial_exhausted` | 429 | `provider_trial_exhausted` (새) |
| `trial_paused` | 503 | `provider_trial_paused` (새) |
| `trial_busy` | 429 | `provider_rate_limited` (기존) |
| `unauthorized` | 401 | `provider_auth_failed` (기존. 화면은 체험이면 "웍스 연결 다시"로 읽는다) |

코드 비교는 `id == linetta-trial`일 때만 한다. 다른 공급자의 응답 본문에 같은 낱말이 있어도 바뀌지 않게 하기 위해서다.

## Touch points

- `engine/internal/settings/settings.go`, `providers_cli.go`, `providers_nocli.go`, `providers_mobile.go`: `ProviderLinettaTrial`, `ValidProviders`, `settings.set` 검사
- `engine/internal/provider/provider.go`: `Resolve`, `Configured`, `Options`
- `engine/internal/provider/catalog.go`: `authKind`가 `"works"`, `ListModels`는 `["linetta-trial"]`를 그대로 돌려준다(상위를 부르지 않는다)
- `engine/internal/provider/errors.go`: 체험 코드 읽기
- `engine/internal/rpc/reason.go`: `ReasonProviderTrialExhausted`, `ReasonProviderTrialPaused`
- `engine/internal/agent/agent.go`, `loop.go`, `pass.go`, `prompt.go`: `TrialToolsEnabled` 제거, `withTrial(resolved.ID == …)`
- `engine/internal/engineapp/agent_enabled.go`: `TrialToolsEnabled` 배선 제거
- `engine/internal/publish/client.go`: `TrialStatus(ctx)` (`GET /api/artworks/trial/me`)
- `engine/internal/rpc/handlers/providers.go` 또는 `publish.go`: `trial.status`
- `apps/desktop/src/lib/types.ts`: `ProviderID`에 `"linetta-trial"`(화면 작업은 Phase 3지만 타입은 여기서 맞춘다)

## Tasks

- [ ] **2.1 공급자 id와 자격 증명**
  - `ProviderLinettaTrial = "linetta-trial"`. 세 빌드의 `ValidProviders` 맨 앞에 둔다(설정 화면에서 가장 먼저 보이게). `DefaultProvider`는 그대로다
  - `Resolve`, `Configured`, `Options` (위 표)
  - `settings.set`: 이 공급자의 키, 주소, 모델은 거절. 동의(`consented_at`)와 `provider` 활성화는 받는다
  - 테스트: 연결 없음은 미구성, 연결과 토큰이 있으면 구성, 토큰만 지워도 미구성, `Options`의 주소와 키, 키를 넣으려는 `settings.set` 거절, `String()`이 토큰을 가림

- [ ] **2.2 그 턴의 공급자로 체험 정하기**
  - `toolGroups.withTrial(bool)`로 바꾸고, `loop.go`와 `pass.go`에서 `resolved.ID == settings.ProviderLinettaTrial`을 넘긴다
  - `Deps.TrialToolsEnabled`와 engineapp 배선, 관련 테스트를 이 기준으로 고친다
  - 테스트: 체험 공급자로 도는 턴은 20개, 같은 서비스에서 다른 공급자로 도는 턴은 전체. 활성 공급자를 턴 도중에 바꿔도 그 턴은 시작할 때의 공급자를 따른다

- [ ] **2.3 오류 이유**
  - 새 이유 두 개, `Classify`가 체험 공급자일 때 본문의 `error.code`를 먼저 본다
  - 테스트: 429 + `trial_exhausted` → 소진, 503 + `trial_paused` → 멈춤, 429 + `trial_busy` → 속도 제한, 다른 공급자의 429 본문에 `trial_exhausted`가 있어도 속도 제한

- [ ] **2.4 `trial.status`**
  - `publish.Client.TrialStatus`가 `GET /api/artworks/trial/me`를 부른다. 연결이 없으면 부르지 않고 `{ state: "not_connected" }`
  - RPC `trial.status` → `{ state, granted_tokens, used_tokens, remaining_tokens }`. `state`는 서버 값에 `not_connected`, `unreachable`을 더한다
  - 테스트는 publish 패키지의 가짜 사이트(`fake_site_test.go`)에 경로를 더해 한다

- [ ] **2.5 엔진 끝까지**
  - engineapp 테스트: 가짜 사이트(게시 연결 + 체험 경로)와 기록용 가짜 OpenAI 호환 서버로 `agent.run` 한 턴
    - 요청이 `…/api/trial/v1/chat/completions`로, `Authorization: Bearer lwk_…`와 함께 간다
    - 도구가 20개다
    - 서버가 `trial_exhausted`로 답하면 `agent.error`의 이유가 `provider_trial_exhausted`다

## 확인

- `CGO_ENABLED=0 go test ./...` (ffi 둘 제외), `-tags mobile`, `-tags mas` 빌드
- Phase 1 개발 서버에 붙여 한 턴: `LINETTA_PUBLISH_BASE_URL=<개발 서버>`로 엔진을 띄우고 연결한 뒤 짧은 질문 하나. 서버 원장에 토큰이 적히는지
