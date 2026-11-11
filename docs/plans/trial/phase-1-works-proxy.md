# 체험 공급자 Phase 1 — 리네타웍스 체험 프록시와 한도 원장

_상위 로드맵: [`roadmap.md`](./roadmap.md)_
_작성일: 2026-11-11_
_저장소: `linetta-artworks` (이 문서만 linetta에 둔다. 동기화 계획과 같은 방식)_
_목표: 기기 토큰으로 인증한 요청을 Upstage Solar Mini 4로 흘려 보내고, 계정별 토큰 사용량을 세어 300만 토큰과 월 지출 상한을 지키는 서버_
_예상 소요: 3일_
_전제: 로드맵의 사전 조건(Upstage 서면 확인, 체험 전용 키). 개발은 키 없이 가짜 상위 서버로 한다_

## Goal

- 앱은 OpenAI 호환 요청을 `POST https://works.marvin-42.com/api/trial/v1/chat/completions`로 보낸다. 인증은 게시와 같은 `Authorization: Bearer lwk_…`다.
- 스트리밍(`stream: true`)은 상위 응답의 SSE를 그대로 흘려 보낸다. 끝나면 사용량을 계정 원장에 더한다.
- 계정마다 처음 요청한 순간 300만 토큰(운영 설정 값)을 받는다. 다 쓰면 다음 요청을 거절한다.
- 이번 달 추정 지출이 월 상한에 닿으면 체험 전체를 멈춘다.
- 앱은 `GET /api/artworks/trial/me`로 남은 한도와 상태를 본다.
- 운영 화면에서 체험을 켜고 끄고, 기본 한도, 월 상한, 이번 달 사용량을 보고, 계정에 한도를 더 줄 수 있다.
- 요청과 응답 본문은 어디에도 저장하지 않고 로그에도 남기지 않는다.

## Non-goals

- 앱 쪽 공급자와 화면 (Phase 2·3)
- 결제, 유료 전환
- 익명 체험
- 상위 공급자를 여럿 두고 고르기. 상위는 하나이고, 주소·모델·키를 환경 변수로 바꿀 수 있게만 한다

## 구성

```
Linetta 앱 ──Bearer lwk_──▶ Cloudflare Tunnel
                              ├─ ^/api/trial/  → artworks-trial (Node, 127.0.0.1:3005)
                              │      ├─ POST /api/artworks/trial/authorize ─▶ artworks-pb (서비스 계정)
                              │      ├─ POST https://api.upstage.ai/v1/chat/completions (체험 키)
                              │      └─ POST /api/artworks/trial/usage ─────▶ artworks-pb (서비스 계정)
                              └─ ^/api/        → artworks-pb (기존)
```

PocketBase JS 훅은 상위 응답을 다 받은 뒤에야 돌려주므로 스트리밍을 흘려 보낼 수 없다. 그래서 프록시만 Node 서비스로 두고,
인증, 한도, 원장은 PocketBase가 맡는다. 수위 검사 워커(`backend/agents/`)와 같은 이미지, 같은 서비스 계정 방식을 쓴다.

## Touch points

- `backend/pb_migrations/`: `trial_accounts`, `trial_usage` 컬렉션 (새 마이그레이션)
- `backend/pb_hooks/trial.pb.js`(새), `backend/pb_hooks/lib/trial.js`(새)
- `backend/pb_hooks/lib/settings.js`, `admin-settings.pb.js`: `site_settings`의 key `trial`
- `backend/agents/trial-proxy.js`(새), `backend/agents/lib/trial/*.js`(새), `backend/agents/lib/pb.js`(재사용)
- `deploy/compose.artworks.yml`, `deploy/tunnel-ingress.example.yml`, `deploy/artworks-trial.env.example`(새), `deploy/README.md`
- `frontend/src/app/admin/` 체험 탭, `frontend/src/content/privacy.ts`, `terms.ts`
- `docs/api.md` 체험 절
- `backend/tests/trial.test.mjs`(새), `backend/agents/lib/trial/*.test.js`(새)

## Tasks

- [ ] **1.1 원장 스키마**
  - `trial_accounts`: `user`(relation, unique), `granted_tokens`(number), `used_tokens`(number), `started_at`, `exhausted_at`
    - 처음 `authorize`할 때 만든다. `granted_tokens`는 그때의 운영 설정 기본값(300만)이다. 나중에 기본값을 바꿔도 이미 받은 계정은 그대로다
  - `trial_usage`: `user`, `device`, `input_tokens`, `cached_tokens`, `output_tokens`, `cost_usd`(추정), `status`(`ok` | `upstream_error` | `cancelled`), `model`, `created`
  - API 규칙은 둘 다 비공개(서비스 계정과 운영자만). 작가는 `/trial/me`로만 본다
  - `used_tokens`는 `UPDATE trial_accounts SET used_tokens = used_tokens + {:n}` 한 문장으로 더한다. 동시에 끝난 두 요청이 서로를 덮지 않게 한다

- [ ] **1.2 운영 설정** (`site_settings` key `trial`)
  - `enabled`(기본 false), `grant_tokens`(기본 3,000,000), `monthly_cap_usd`(기본 30), `max_output_tokens`(요청 하나의 출력 상한, 기본 16,000),
    `price_input_per_m` 0.10 · `price_cached_per_m` 0.01 · `price_output_per_m` 0.40 (추정 지출 계산용)
  - 범위 검사는 `lib/settings.js`의 수위 검사 설정과 같은 방식(`RANGES`)으로 한다
  - 상위 주소, 모델 이름, API 키는 운영 화면이 아니라 프록시의 환경 변수에 둔다(`TRIAL_UPSTREAM_BASE_URL`, `TRIAL_UPSTREAM_MODEL`, `TRIAL_UPSTREAM_API_KEY`)

- [ ] **1.3 PocketBase 경로** (`trial.pb.js`)
  - `GET /api/artworks/trial/me` — 기기 토큰(`authenticateDevice(e, false)`, 게시 권한 없어도 됨)
    - `{ state, granted_tokens, used_tokens, remaining_tokens }`. `state`는 `available` | `exhausted` | `paused`(꺼짐 또는 월 상한) | `not_started`(아직 받은 적 없음, 받을 수 있음)
  - `POST /api/artworks/trial/authorize` — 서비스 계정만. 본문 `{ token }`(프록시가 받은 기기 토큰 원문)
    - 토큰 해시로 기기와 계정을 찾고, 체험이 켜져 있는지, 이번 달 추정 지출이 상한 아래인지, 남은 토큰이 있는지 본다
    - 통과하면 계정 행이 없을 때 만들고 `{ user, device, remaining_tokens, max_output_tokens }`
    - 거절은 `{ code }`: `unauthorized` | `trial_paused` | `trial_exhausted`
  - `POST /api/artworks/trial/usage` — 서비스 계정만. `{ user, device, input_tokens, cached_tokens, output_tokens, status, model }`
    - `trial_usage` 한 행과 `used_tokens` 증가, 다 썼으면 `exhausted_at`
    - `cost_usd`는 설정의 단가로 계산해 저장한다. 월 지출은 이 열의 이번 달 합이다
  - `POST /api/artworks/admin/trial/grant` — 운영자. `{ user, add_tokens }`로 한도를 더 준다
  - `GET /api/artworks/admin/trial` — 운영자. 이번 달 요청 수, 토큰, 추정 지출, 체험을 받은 계정 수, 많이 쓴 계정 상위 20

- [ ] **1.4 프록시 서비스** (`backend/agents/trial-proxy.js`)
  - `POST /api/trial/v1/chat/completions`
    1. `Authorization` 헤더의 토큰으로 `authorize`를 부른다. 거절은 OpenAI 오류 모양 `{ error: { code, message } }`과 상태 코드로 돌려준다:
       `unauthorized` 401, `trial_exhausted` 429, `trial_paused` 503, `trial_busy` 429
    2. 계정당 동시에 하나만 받는다. 메모리의 사용자 id 집합으로 막고, 두 번째는 `trial_busy`다
    3. 본문은 4 MB까지 받는다. `model`은 무시하고 `TRIAL_UPSTREAM_MODEL`로 바꾼다. `max_tokens`는 `max_output_tokens`로 자른다.
       `stream`이면 `stream_options.include_usage = true`를 넣는다. 그 밖의 필드(`messages`, `tools`, `tool_choice`, `temperature`)는 그대로 보낸다
    4. 상위 응답을 그대로 흘려 보낸다(상태 코드, `content-type`, SSE 바이트). 마지막 청크의 `usage`를 읽어 둔다
    5. 끝나면 `usage`를 부른다. 앱이 중간에 끊으면 상위 요청도 끊고, 사용량을 모르면 입력을 본문 글자 수로 어림해 `cancelled`로 적는다(상위는 이미 과금했을 수 있으므로 0으로 두지 않는다)
    6. 상위 5xx와 연결 실패는 502로 돌려주고 `upstream_error`로 적는다. 토큰은 상위가 알려 준 만큼만 더한다
  - `GET /api/trial/v1/models` — `{ data: [{ id: "linetta-trial" }] }`. 앱의 연결 확인용이고 상위를 부르지 않는다
  - **본문을 로그에 남기지 않는다.** 로그는 요청 id, 사용자 id, 상태, 토큰 수, 걸린 시간뿐이다. 상위 오류 본문도 첫 줄 200자로 자르고 메시지 내용은 남기지 않는다
  - 한 요청이 남은 한도를 조금 넘을 수 있다(시작할 때만 검사한다). 요청 하나의 크기는 체험 도구 기준 최대 수십만 토큰이고, 넘친 만큼은 원장에 그대로 적는다
  - 헬스 확인 `:8096/health` (워커의 `:8095`와 같은 방식)

- [ ] **1.5 배포**
  - `compose.artworks.yml`에 `artworks-trial`(같은 agents 이미지, `command: node trial-proxy.js`, `127.0.0.1:3005`)
  - 터널 규칙: `^/api/trial/` → `127.0.0.1:3005`를 `^/api/` 규칙보다 **앞에** 둔다
  - `deploy/artworks-trial.env.example`: `PB_URL`, 서비스 계정, `TRIAL_UPSTREAM_*`, `PORT`
  - Upstage 콘솔에서 체험 키에 월 지출 한도를 따로 건다(서버 상한이 고장 나도 비용이 묶이게)

- [ ] **1.6 사이트 문서**
  - 개인정보처리방침: 체험을 쓰면 요청 내용이 리네타웍스를 거쳐 Upstage로 간다. 리네타웍스는 내용을 저장하지 않고 토큰 수만 적는다. 수탁자와 목적
  - 약관: 체험은 무료이고 한도가 있으며 예고 없이 멈출 수 있다
  - `docs/api.md`에 체험 경로와 오류 코드

## 테스트

- PocketBase(`backend/tests/trial.test.mjs`)
  - `me`: 기기 없음 401, 처음은 `not_started`, 받은 뒤 남은 양, 소진 `exhausted`, 꺼짐 `paused`
  - `authorize`: 서비스 계정이 아니면 403, 폐기된 기기 401, 월 상한 도달 `trial_paused`, 계정 행이 처음 한 번만 생김
  - `usage`: 두 요청을 동시에 더해도 합이 맞음, 소진 시각, 비용 계산
  - 운영자 경로: 운영자가 아니면 403, 한도 추가
- 프록시(`node --test`, 가짜 PocketBase와 가짜 상위 서버)
  - 스트리밍이 청크 단위로 그대로 흘러감(첫 청크가 마지막 청크 전에 도착)
  - `model` 치환, `max_tokens` 자르기, `include_usage` 추가, 도구 필드 보존
  - 소진, 꺼짐, 동시 요청 거절의 상태 코드와 오류 모양
  - 앱이 끊으면 상위도 끊기고 `cancelled`로 적힘
  - 로거에 넘어간 문자열 어디에도 메시지 본문이 없음
- `make test` 통과

## 배포 순서

1. 마이그레이션과 훅(`enabled: false`) 배포
2. 프록시 서비스와 터널 규칙 추가, 헬스 확인
3. 운영자 계정으로 `enabled: true`, 직접 요청 몇 번으로 원장과 Upstage 청구 화면이 맞는지 확인
4. Phase 2·3 앱 배포 전까지는 운영자 계정만 쓴다(앱에 체험 공급자가 없으므로 다른 사람이 부를 일이 없다)
