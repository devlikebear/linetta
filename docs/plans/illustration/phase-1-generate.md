# 삽화 Phase 1 — 삽화 생성과 갤러리

_상위 로드맵: [`roadmap.md`](./roadmap.md)_
_목표: 설정에서 이미지 생성 공급자(ChatGPT 또는 Gemini)를 고르고, 씬에서 삽화를 만들어 작품 갤러리에 보관하는 첫 수직 슬라이스_
_예상 소요: 4~5일 (tars 1.5일 포함)_

## Goal

작가는 설정 → 이미지 생성에서 ChatGPT(기존 Codex 로그인)나 Gemini(API 키)를 고른다.
씬 화면에서 "삽화 만들기"를 누르면 장면 설명, 등장인물, 형식을 고르고 조립된 프롬프트를 확인·수정한 뒤 생성한다.
결과는 작품 삽화 갤러리에 저장되고, 백업과 병합 복원에 함께 실린다.

이 단계에서는 원고 본문을 건드리지 않는다. 본문 삽입은 Phase 2에서 한다.

## Non-goals

- 씬 본문에 삽화 넣기 (Phase 2)
- Markdown 내보내기·게시에 이미지 싣기 (Phase 3)
- 에이전트가 삽화를 만드는 툴 (Later)
- `openai`(API 키), `anthropic`, `claude-code` 공급자의 이미지 생성
- 생성 취소, 여러 장 동시 생성

## Touch points

tars 저장소(`~/workspace/myprojects/tars`):

- `pkg/llm/image.go` 신규
- `pkg/llm/openai_codex_client.go`, `pkg/llm/openai_codex_image.go` 신규
- `pkg/llm/gemini_native.go`, `pkg/llm/gemini_native_image.go` 신규

Linetta 엔진:

- `engine/go.mod` (tars 버전)
- `engine/internal/settings/settings.go`, `engine/internal/settings/image.go` 신규
- `engine/internal/provider/image.go` 신규
- `engine/internal/store/migrations/0024_illustrations.sql` 신규
- `engine/internal/illustration/` 신규 (`repo.go`, `service.go`)
- `engine/internal/visual/compose.go` 신규 (MCP에서 옮겨 오는 인물 해석·프롬프트 조립)
- `engine/internal/mcphost/tools_visuals.go` (옮긴 함수를 쓰도록)
- `engine/internal/rpc/handlers/illustrations.go` 신규, `engine/internal/rpc/handlers/settings.go`
- `engine/internal/engineapp/engineapp.go`
- `engine/internal/restore/restore.go`

데스크톱:

- `apps/desktop/src/lib/types.ts`, `apps/desktop/src/lib/rpc.ts`, `apps/desktop/src/lib/i18n.tsx` (ko·en·ja)
- `apps/desktop/src/routes/Settings.tsx`
- `apps/desktop/src/components/settings/ImageGenSection.tsx` 신규
- `apps/desktop/src/components/illustration/IllustrationDialog.tsx` 신규
- `apps/desktop/src/components/illustration/IllustrationGallery.tsx` 신규
- `apps/desktop/src/routes/Workspace.tsx` (진입점과 패널 연결)

## UX

**설정 → 이미지 생성** (새 카테고리, `providers` 바로 아래)

- 공급자 선택: `사용 안 함` / `ChatGPT` / `Gemini`
  - 선택한 공급자의 자격 증명이 없으면 "에이전트 모델에서 먼저 로그인/키를 설정하세요"와 그 카테고리로 가는 버튼
- 모델 선택: 공급자별 목록 (Gemini는 조회 결과 중 이미지 출력 모델만)
- 이미지 생성 동의: 공급자별 한 번. 문장 예:
  "삽화를 만들 때 내가 확인한 프롬프트(장면 설명, 인물 외형, 화풍)와 고른 참조 이미지가 {provider}로 전송되고,
  Linetta는 그 밖의 어떤 곳에도 보내지 않는다는 데 동의합니다"
- 공급자별 안내: ChatGPT는 "ChatGPT 요금제 사용량에서 차감됩니다", Gemini는 "Google AI 요금이 이미지마다 청구됩니다"

**삽화 만들기 대화상자**

- 진입: 에디터 선택 영역 컨텍스트 메뉴 `이 장면으로 삽화 만들기`, 커맨드 팔레트 `삽화 만들기`, 갤러리의 `새 삽화`
- 입력
  - 장면 설명: 선택한 본문으로 미리 채움(없으면 빈칸). 작가가 고친다
  - 등장인물: 현재 씬의 멘션 노드에서 인물을 미리 고름. 추가·제거 가능. 시트가 없는 인물은 경고 표시
  - 형식: `한 장` / `4컷` (`visual.BuildPrompt`의 `single` / `four_panel`)
  - 비율: `정사각` / `세로` / `가로`
  - 참조 이미지 싣기: 켬/끔, 실릴 이미지 썸네일 표시 (최대 4장)
- `프롬프트 보기`: 조립된 프롬프트를 편집 가능한 텍스트로 보여준다. 고친 내용 그대로 보낸다
- `생성`: 진행 중 표시(보통 25~35초). 실패 시 이유(동의 없음, 로그인 만료, 사용량 초과, Gemini 결제 필요, 안전 정책 거절, 시간 초과)를 사람 말로
- 결과: 미리보기, `갤러리에 저장됨`, `다시 생성`(같은 입력으로 한 장 더)

**작품 삽화 갤러리**

- 워크스페이스 오른쪽 패널(FactBook 패널과 같은 자리)
- 썸네일 격자, 캡션 편집, 삭제, 원본 크게 보기, 프롬프트·공급자·모델·생성 시각 보기
- 하단에 장수와 총 용량("백업에 함께 저장됩니다")

## tars tasks

- [x] **1.0 스파이크: Codex·Gemini 이미지 요청 모양 확인** — 완료 (아래 "스파이크 기록")
  - tars에 `//go:build integration` 테스트 `pkg/llm/image_spike_test.go`를 임시로 만든다. 커밋하지 않는다.
  - Codex: 로그인된 `auth.json`으로 `responses`에 다음을 보낸다.
    ```json
    {"model":"<라우팅 모델>","stream":true,
     "input":[{"role":"user","content":[{"type":"input_text","text":"a red apple on a desk"}]}],
     "tools":[{"type":"image_generation","output_format":"jpeg","size":"1024x1024"}],
     "tool_choice":{"type":"image_generation"}}
    ```
  - 확인할 것: SSE 이벤트 이름(`response.output_item.done`의 `image_generation_call` 항목, `result` base64), 스트리밍 필수 여부,
    `size`·`output_format`·`quality`·`model` 툴 파라미터 수용 여부, `input_image` 참조 반영 여부,
    라우팅 모델별 성공 여부(`gpt-5.3-codex-spark`, 그 밖에 목록의 모델), 안전 정책 거절 시 응답 모양
  - Gemini: `models` 목록에서 이미지 출력 모델을 찾고 `generateContent`에
    `generationConfig: {responseModalities: ["IMAGE"], imageConfig: {aspectRatio: "2:3"}}`를 보낸다.
    응답 `candidates[0].content.parts[].inlineData`의 MIME과 크기, 참조 이미지 `inlineData` 입력 반영 여부를 확인한다.
  - 결과를 이 문서 끝 "스파이크 기록"에 적고, 아래 작업의 파라미터를 거기 맞춘다.

- [ ] **1.1 `pkg/llm/image.go` — 공통 타입**
  ```go
  type ImageAspect string // "square" | "portrait" | "landscape"

  type ImageRequest struct {
      Prompt     string
      References []ContentBlock // Type "image", base64 Data, MediaType
      Aspect     ImageAspect
  }

  type ImageResult struct {
      Data          []byte
      MediaType     string
      RevisedPrompt string // 공급자가 고쳐 쓴 프롬프트가 있으면
  }

  type ImageGenerator interface {
      GenerateImage(ctx context.Context, req ImageRequest) (ImageResult, error)
  }
  ```
  - 거절은 기존 `errors.go` 패턴에 `ErrImageRefused`(안전 정책)를 더한다.

- [ ] **1.2 `OpenAICodexClient.GenerateImage`**
  - 파일: `pkg/llm/openai_codex_image.go`
  - `chatWithCredential`의 인증 헤더, 401 갱신 재시도, rate limit 관찰(`setLastRateLimit`)을 그대로 쓴다. 요청 본문만 다르다.
  - 툴은 `{"type":"image_generation","output_format":"jpeg"}`만 보낸다. `size`·`quality`는 무시되므로 보내지 않는다.
  - `Aspect`는 instructions에 비율 문장으로 넣는다 (예: portrait → "Vertical portrait orientation, 2:3 aspect ratio.").
  - instructions에 "이미지 툴이 실패하면 다시 시도하지 말 것"을 넣는다(거절 시 라우팅 모델이 스스로 재시도했다).
  - `stream: true`, `tool_choice: {"type":"image_generation"}`는 필수로 둔다.
  - SSE는 tars `createSSEScanner`(한 줄 최대 1 MiB)로 읽지 않는다. `result`가 담긴 줄은 JPEG로 약 225 KB지만
    WebP나 `partial_images`를 쓰면 2 MB를 넘었다. 이미지 경로는 한 줄 상한 16 MiB의 스캐너를 따로 쓴다.
  - 참조 이미지는 `toCodexUserContent`처럼 `input_image` data URL로 싣는다.
  - SSE에서 `response.output_item.done`의 `image_generation_call`을 본다. `status: "completed"`면 `result`를 디코드하고,
    `status: "failed"`면 `error.code`(예: `moderation_blocked`)를 담은 `ErrImageRefused`로 바로 끝낸다. 이미지 없이 끝나도 거절이다.
  - 테스트: `openai_codex_image_test.go`, `httptest` 서버로 성공 / 401 후 갱신 / `failed` 항목(거절) / 이미지 없음 / 잘린 스트림 5케이스.
    스트림 픽스처는 스파이크 기록의 이벤트 순서를 따른다(`keepalive` 포함).

- [ ] **1.3 `GeminiNativeClient.GenerateImage`**
  - 파일: `pkg/llm/gemini_native_image.go`
  - `x-goog-api-key` 헤더, `models/{model}:generateContent`, `responseModalities: ["IMAGE"]`, 비율은 `imageConfig.aspectRatio` (`1:1`, `2:3`, `3:2`).
    `imageConfig.imageSize: "1K"`는 3.x 모델에만 보낸다. `aspectRatio`는 늘 보낸다(빼면 가로가 나온다).
  - 응답은 채팅 경로의 10 MiB `LimitReader`를 쓰지 않고 이미지 전용 상한(32 MiB)으로 읽는다. 3.x 응답의 `thoughtSignature`는 버린다.
  - `finishReason`이 `STOP`이 아니거나(예: `PROHIBITED_CONTENT`) 이미지 part가 없으면 `finishReason`을 담은 `ErrImageRefused`.
  - 참조 이미지는 `inlineData` part로 프롬프트 앞에 싣는다.
  - 무료 등급 키의 `429 RESOURCE_EXHAUSTED`(`quotaId`에 `FreeTier`, `limit: 0`)는 `ErrImageBillingRequired`로 따로 구분한다.
  - 테스트: `gemini_native_image_test.go`, 성공 / 이미지 없음 / 무료 등급 429 / 그 밖의 4xx 오류 매핑 4케이스.

- [ ] **1.4 tars 반영과 Linetta 버전 올리기**
  - tars PR 머지 후 Linetta에서 `cd engine && go get github.com/devlikebear/tars@<commit>` 하고 `go mod tidy`
  - 커밋 메시지는 기존 관례대로 `chore(engine): tars를 이미지 생성 API(<short sha>)로`

## Engine tasks

- [ ] **1.5 이미지 생성 설정**
  - 파일: `engine/internal/settings/image.go` 신규, `settings.go`
  - `Settings`에 추가:
    ```go
    ImageProvider  string                         `json:"image_provider,omitempty"`  // "" = 사용 안 함
    ImageProviders map[string]ImageProviderConfig `json:"image_providers,omitempty"`
    ```
    `ImageProviderConfig{Model string; ConsentedAt int64}`. 키·로그인은 `Providers[id]`의 것을 쓰고 여기에 두지 않는다.
  - `ValidImageProviders() = [ProviderOpenAICodex, ProviderGeminiNative]`. 그 밖의 id를 patch하면 거절한다.
  - `SettingsPatch`에 `ImageProvider *string`, `ImageProviders map[string]ImageProviderPatch` (기존 `ProviderPatch` 모양을 따른다)
  - `HasImageConsent(id)`
  - 테스트: `image_test.go` — 기본값은 사용 안 함, 허용 밖 id 거절, 동의 기록, 디스크 왕복, 키가 이 구조체로 새지 않음

- [ ] **1.6 `provider.Source.ImageGenerator`**
  - 파일: `engine/internal/provider/image.go`
  - `ImageGenerator(id string) (llm.ImageGenerator, Resolved, error)`
    - `Resolve(id)`로 자격 증명을 얻고 `Configured()`가 아니면 `rpc.ReasonProviderNotConfigured`
    - 이미지 동의가 없으면 새 reason `rpc.ReasonImageConsentRequired` (`rpc/reason.go`)
    - 모델은 `ImageProviders[id].Model`. 비었으면 Codex는 Codex 모델 목록의 첫 `visibility: list` 모델,
      Gemini는 기본값 없이 "모델을 고르세요" 오류. `gpt-5.3-codex-spark`는 ChatGPT 계정에서 거절되므로 기본값으로 쓰지 않는다
    - 기존 `ClientFactory`로 만든 클라이언트가 `llm.ImageGenerator`를 구현하지 않으면 내부 오류
  - `ListImageModels(ctx, id)`: Gemini는 이름에 `-image`가 있고 `generateContent`를 지원하는 모델만, Codex는 라우팅 모델 목록
  - Gemini 무료 등급 오류는 새 reason `rpc.ReasonImageBillingRequired`로 바꾼다
  - 테스트: `image_test.go` — 미설정, 동의 없음, 모델 기본값, 허용 밖 id

- [ ] **1.7 삽화 저장소**
  - 마이그레이션 `0024_illustrations.sql`:
    ```sql
    CREATE TABLE illustrations (
      id          TEXT PRIMARY KEY,
      project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
      source_node_id TEXT REFERENCES nodes(id) ON DELETE SET NULL,
      caption     TEXT NOT NULL DEFAULT '',
      prompt      TEXT NOT NULL,
      provider    TEXT NOT NULL,
      model       TEXT NOT NULL,
      mime        TEXT NOT NULL,
      width       INTEGER NOT NULL DEFAULT 0,
      height      INTEGER NOT NULL DEFAULT 0,
      byte_size   INTEGER NOT NULL,
      data        BLOB NOT NULL,
      created_at  INTEGER NOT NULL
    );
    CREATE INDEX illustrations_project ON illustrations(project_id, created_at);
    ```
  - 패키지 `engine/internal/illustration/repo.go`: `visual.Repo`의 구조(`NewRepo`, `scanMeta`, `imageMutationError`)를 따른다.
    `Add`, `ListByProject`(바이트 없이), `Get`, `UpdateCaption`, `Delete`, `Usage(projectID) (count, bytes)`, `Restore`
  - 상한: `MaxIllustrationBytes = 5 MiB`, `MaxIllustrationsPerProject = 200`. MIME은 `http.DetectContentType`으로 확인(JPEG, PNG, WebP)
  - 공급자가 5 MiB를 넘는 PNG를 주면 `image/png` 디코드 후 `image/jpeg` 품질 90으로 다시 인코딩한다. 그래도 넘으면 거절
  - 테스트: `repo_test.go` — 추가·목록·삭제, 작품 삭제 시 함께 삭제, 상한 초과, 잘못된 MIME, 큰 PNG 재인코딩

- [ ] **1.8 인물 해석과 프롬프트 조립을 `visual`로 옮기기 (동작 변경 없음)**
  - `mcphost/tools_visuals.go`의 `resolveVisualCharacters`와 `buildIllustrationPrompt` 안의 조립 부분을
    `engine/internal/visual/compose.go`의 `Compose(ctx, repo, entities, projectID, ComposeInput) (Composition, error)`로 옮긴다.
    `Composition{Prompt, Characters, ReferenceImageIDs, MissingSheets}`
  - MCP 툴은 이 함수를 부르는 얇은 층이 된다. 두 경로가 따로 조립하지 않게 한다.
  - 검증: 기존 `tools_visuals_test.go`가 그대로 통과. `compose_test.go`에 없는 인물 이름 오류, 시트 없는 인물 경고 2케이스

- [ ] **1.9 `illustration.Service.Generate`**
  - 파일: `engine/internal/illustration/service.go`
  - 입력: `project_id`, `source_node_id`(선택), `prompt`(작가가 확인·수정한 최종 문자열), `character_ids`, `aspect`, `include_references`
  - 순서: 이미지 공급자 확인 → `ImageGenerator` → 참조 이미지 모으기(인물 순서대로 ordinal 0부터, 최대 4장) →
    `context.WithTimeout(180s)`로 생성 → 저장소 `Add` → 메타 반환
  - 프롬프트는 클라이언트가 보낸 것을 그대로 보낸다. 엔진이 몰래 덧붙이지 않는다(작가가 본 것이 보내는 것)
  - 오류를 reason으로 나눈다: 미설정, 동의 없음, Codex 로그인 만료, rate limit, Gemini 결제 필요, 거절(`ErrImageRefused`), 시간 초과, 상한 초과
  - 가로·세로는 요청한 비율이 아니라 결과 바이트에서 읽는다(`image.DecodeConfig`). Codex는 요청과 다른 크기(`1374x1145` 등)를 돌려준다
  - 테스트: `service_test.go`, 가짜 `ImageGenerator`로 성공 저장, 참조 4장 제한, 거절 시 저장 안 함, 시간 초과

- [ ] **1.10 RPC**
  - 파일: `engine/internal/rpc/handlers/illustrations.go`, 등록은 `engineapp.go`의 `visuals.*` 블록 옆
  - 설정 필드는 기존 `settings.get`/`settings.patch`로 다룬다
  - 새 메서드:
    - `image_providers.list` → 공급자별 `{id, configured, consented, model}` (`provider.Status`와 같은 모양, 비밀 없음)
    - `image_providers.models` `{id}` → `[]string`
    - `illustrations.compose` `{project_id, scene, character_ids, character_names, format}` → `visual.Composition`
    - `illustrations.generate` → `IllustrationMeta`
    - `illustrations.list` `{project_id}` → `{items: []IllustrationMeta, count, bytes}`
    - `illustrations.get` `{id}` → 메타 + `data_base64` (`visuals.get_image`와 같은 모양)
    - `illustrations.update` `{id, caption}`, `illustrations.delete` `{id}`
  - 테스트: `illustrations_test.go` — 빈 project_id `CodeInvalidParams`, 동의 없음 reason, list 합계

- [ ] **1.11 병합 복원**
  - `restore.MergeProject`가 `illustrations`를 새 작품 id로 옮긴다. `source_node_id`는 노드 id 재매핑 표로 바꾼다
  - 테스트: 기존 `restore` 병합 테스트에 삽화 한 장을 더해, 새 작품 id 아래에 바이트가 같은 삽화가 생기는지 확인

## Desktop tasks

- [ ] **1.12 타입·RPC·문구**
  - `types.ts`: `IllustrationMeta`, `Composition`, `ImageProviderStatus`, 설정 타입에 `image_provider`, `image_providers`
  - `rpc.ts`: `illustrationsApi`, `imageProvidersApi` (`visualsApi` 모양을 따른다)
  - `i18n.tsx`: ko·en·ja 세 언어 모두

- [ ] **1.13 설정 → 이미지 생성**
  - `ImageGenSection.tsx`: `ProviderSection.tsx`의 행·모델 선택·동의 패턴을 따른다
  - `Settings.tsx`: `agentAvailable`일 때만 카테고리 노출 (공급자 설정과 같은 조건)
  - 테스트: `ImageGenSection.test.tsx` — 자격 증명 없는 공급자 안내, 동의 전 생성 불가 표시, 모델 저장

- [ ] **1.14 삽화 만들기 대화상자**
  - `IllustrationDialog.tsx`: 입력 → `illustrations.compose` → 프롬프트 편집 → `illustrations.generate`
  - 등장인물 미리 고르기: 현재 씬 문서의 `mention` 노드 중 인물 종류
  - 진입점: `Tiptap.tsx` 선택 영역 메뉴, 커맨드 팔레트, 갤러리
  - 이미지 공급자가 `사용 안 함`이면 설정으로 가는 안내만 보인다
  - 테스트: `IllustrationDialog.test.tsx` — 선택 텍스트로 미리 채움, 편집한 프롬프트가 그대로 전송됨, 거절 오류 문구

- [ ] **1.15 갤러리 패널**
  - `IllustrationGallery.tsx`: 목록, 썸네일은 `illustrations.get`으로 받아 object URL 캐시(작품 전환 시 해제)
  - 캡션 편집, 삭제 확인, 크게 보기, 생성 정보, 하단 장수·용량
  - 테스트: `IllustrationGallery.test.tsx` — 목록 표시, 삭제 후 갱신, 용량 표시

---

### ✅ Checkpoint: Phase 1 완료 확인

**구현 확인:**
- [ ] 설정에서 이미지 공급자를 `ChatGPT`, `Gemini`, `사용 안 함`으로 바꿀 수 있고, 키·토큰이 `image_providers`에 저장되지 않는다
- [ ] 동의 전에는 생성 버튼이 막히고, 엔진도 `ReasonImageConsentRequired`로 거절한다
- [ ] MCP `linetta_build_illustration_prompt`와 대화상자의 프롬프트가 같은 `visual.Compose`에서 나온다

**실행 확인:**
- [ ] tars: `go test ./pkg/llm/...` 통과
- [ ] `cd engine && CGO_ENABLED=0 go test ./...` 통과, `make test-mobile-engine` 통과
- [ ] `make test-desktop` 통과

**수동 확인:**
- [ ] `make dev`로 띄우고, ChatGPT 로그인 상태에서 인물 시트가 있는 씬의 한 단락을 골라 세로 삽화를 만든다. 갤러리에 저장되고 인물 참조가 반영된다
- [ ] Gemini 키로 같은 장면을 만든다
- [ ] 백업 후 그 작품을 병합 복원하면 새 작품 갤러리에 삽화가 있다

**이 체크포인트를 통과하면 사용자에게 확인 요청 후 Phase 2로 진행.**
실패하면 실패 항목을 보고하고, 사용자와 원인을 파악해 고친 뒤 다시 확인한다.

---

## 스파이크 기록

_2026-10-23. 스크래치 폴더의 Python 스크립트로 실제 요청을 보냈다. tars에 통합 테스트를 만들지 않았고 저장소에는 아무것도 남기지 않았다._

### Codex (ChatGPT Plus 로그인, `~/.codex/auth.json`) — 실제 호출로 확인

- **엔드포인트와 헤더**: tars `chatWithCredential`과 같다.
  `POST https://chatgpt.com/backend-api/codex/responses`, `Authorization: Bearer`, `chatgpt-account-id`,
  `OpenAI-Beta: responses=experimental`, `originator`, `Accept: text/event-stream`
- **스트리밍 필수**: `stream: false`는 `400 {"detail":"Stream must be set to true"}`
- **라우팅 모델**: `gpt-5.3-codex-spark`는
  `400 {"detail":"The 'gpt-5.3-codex-spark' model is not supported when using Codex with a ChatGPT account."}`.
  Codex 모델 목록(`visibility: list`)에 있는 `gpt-5.6-luna`로는 성공했다. 목록에는 `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna`가 있다
- **요청 본문** (성공한 모양):
  ```json
  {"model":"gpt-5.6-luna","instructions":"...","stream":true,"store":false,
   "input":[{"type":"message","role":"user","content":[
     {"type":"input_text","text":"<prompt>"},
     {"type":"input_image","image_url":"data:image/jpeg;base64,..."}]}],
   "tools":[{"type":"image_generation","output_format":"jpeg"}],
   "tool_choice":{"type":"image_generation"}}
  ```
- **결과**: `response.output_item.done`의 `item.type == "image_generation_call"`,
  `status: "completed"`, `result`(base64), `revised_prompt`, `output_format`, `size`, `quality`, `action`(`generate`/`edit`).
  그 뒤에 빈 `message` 항목과 `response.completed`가 온다. `response.completed.response.tool_usage.image_gen`에 이미지 토큰 수가 있다.
  이벤트 순서: `response.created` → `image_generation_call.in_progress` → `.generating` → (`.partial_image`) → `.completed`
  → `output_item.done` → `response.completed`. 중간에 `keepalive`가 낄 수 있다
- **툴 파라미터**:
  - `output_format`: 지킨다(`jpeg`, `webp` 확인). JPEG 결과는 160~200 KB, WebP는 1.3 MB였다 → JPEG로 받는다
  - `size`: 무시된다. `1024x1024`, `1024x1536`을 보내도 `1536x1024`가 나왔다. 프롬프트에
    "Vertical portrait orientation, 2:3"을 넣으면 `1024x1536`이 나왔다. 그 밖에 `1374x1145`, `1672x941`도 나왔다
  - `quality`: 무시된다. `high`를 보내도 결과는 늘 `quality: "low"`였다 (Plus 계정)
  - `model: "gpt-image-2"`: 오류는 없었지만 반영 여부를 알 수 없다 → 보내지 않는다
  - `partial_images: 1`: 지킨다. `response.image_generation_call.partial_image`(`partial_image_b64`)가 온다. 진행 미리보기는 Later
- **참조 이미지**: `input_image`로 보낸 그림의 책상과 소품을 그대로 둔 채 요청대로 고쳐 그렸다
  (`action: "edit"`, 이미지 입력 1536토큰). 인물 일관성에 쓸 수 있다
- **`tool_choice`**: 없어도 생성했지만, 텍스트만 답하는 일을 막으려고 강제한다
- **거절**: HTTP 200, `image_generation_call`이 `status: "failed"`,
  `error: {"type":"image_generation_user_error","code":"moderation_blocked","moderation_details":{"moderation_stage":"output"}}`.
  라우팅 모델이 스스로 한 번 더 시도해 두 번 실패했고 59초가 걸렸다
- **소요 시간**: 성공 25~34초, 거절 59초. 180초 시간 제한이면 충분하다
- **사용량 헤더**: 이미지 7장 뒤에도 `x-codex-primary-used-percent`가 0이었다. 이미지가 이 한도에 잡히는지는 알 수 없다

### Gemini (`gemini-native` 키) — 실제 호출로 확인 (2026-10-24, 결제가 설정된 프로젝트의 키)

- **무료 등급 키**: 처음 쓴 키(무료 등급 프로젝트)는 이미지 모델 6개 모두 `429 RESOURCE_EXHAUSTED`,
  `quotaId`가 `...-FreeTier`, `limit: 0`이었다. 결제를 설정한 뒤에도 그 키는 계속 429였고,
  결제를 설정한 프로젝트의 키로 바꾸자 성공했다.
  UI 문구 예: "이 키는 무료 등급 프로젝트의 키라 이미지를 만들 수 없습니다. 결제를 설정한 프로젝트의 키를 넣으세요"
- **모델 목록**(`GET /v1beta/models`, 60개 중 이미지 출력 모델 6개):
  `gemini-2.5-flash-image`, `gemini-3-pro-image-preview`, `gemini-3-pro-image`, `gemini-3.1-flash-image-preview`,
  `gemini-3.1-flash-image`, `gemini-3.1-flash-lite-image`. 이미지 출력을 알리는 필드가 따로 없어 이름으로 거른다.
  `gemini-3-pro-image`는 비용 때문에 생성까지는 시험하지 않았다
- **요청 본문** (성공한 모양):
  ```json
  {"contents":[{"role":"user","parts":[
     {"inlineData":{"mimeType":"image/jpeg","data":"..."}},
     {"text":"<prompt>"}]}],
   "generationConfig":{"responseModalities":["IMAGE"],
     "imageConfig":{"aspectRatio":"2:3","imageSize":"1K"}}}
  ```
- **결과**: HTTP 200, `candidates[0].finishReason: "STOP"`, `candidates[0].content.parts[]`에 `inlineData{mimeType, data}` 한 개.
  3.x 모델은 같은 part에 `thoughtSignature`(이미지보다 큰 문자열)가 붙는다. 저장하지 않고 버린다.
  `usageMetadata.candidatesTokensDetails`에 이미지 토큰 수(1K 1120, 2K 1680)

  | 모델 | 설정 | MIME | 크기 | 바이트 | 시간 |
  |---|---|---|---|---:|---:|
  | `gemini-3.1-flash-image` | 2:3, 1K | `image/jpeg` | 848×1264 | 886 KB | 9.3 s |
  | `gemini-3.1-flash-image` | 2:3, 2K | `image/jpeg` | 1696×2528 | 3.66 MB | 16.7 s |
  | `gemini-3.1-flash-image` | 비율 없음, TEXT+IMAGE | `image/jpeg` | 1408×768 | 887 KB | 9.7 s |
  | `gemini-3.1-flash-lite-image` | 3:2 | `image/jpeg` | 1264×848 | 749 KB | 3.6 s |
  | `gemini-2.5-flash-image` | 2:3 (`imageSize` 없음) | `image/png` | 832×1248 | 1.55 MB | 8.7 s |
  | `gemini-3.1-flash-image` | 16:9, 1K, 참조 1장 | `image/jpeg` | 1376×768 | 652 KB | 9.2 s |

- **비율**: `aspectRatio`를 지킨다. 지정하지 않으면 문서와 달리 정사각이 아니라 가로(1408×768)가 나왔다 → 늘 보낸다
- **`responseModalities: ["TEXT","IMAGE"]`**: 텍스트 part 없이 이미지만 왔다. `["IMAGE"]`로 고정한다
- **참조 이미지**: Codex 결과 그림을 넣고 "반으로 잘라 접시에"라고 하자 책상·소품·화풍을 유지한 채 고쳐 그렸다(입력 이미지 258토큰)
- **거절**: HTTP 200, `candidates[0].finishReason: "PROHIBITED_CONTENT"`, `content: {}`(part 없음), 8.3초.
  `STOP`이 아니고 이미지 part가 없으면 `finishReason`을 담아 `ErrImageRefused`로 끝낸다
- **응답 크기**: 2K 응답 JSON은 base64 이미지 4.9 MB + `thoughtSignature` 5.4 MB로 10 MB가 넘는다.
  tars `gemini_native_chat.go`는 응답을 `io.LimitReader(resp.Body, 10*1024*1024)`로 읽으므로 이미지 경로는 따로 더 큰 상한을 둬야 한다
