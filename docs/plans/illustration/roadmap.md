# 삽화 생성·삽입 — 로드맵

_작성일: 2026-10-23_
_상태: 계획_
_관련: #154 인물 비주얼 시트(`engine/internal/visual`), #158 비주얼 MCP 쓰기 툴, #160 에디터 스키마 패리티_

## Overview

#154에서 인물 비주얼 시트(외형 고정 필드, 참조 이미지, 작품 화풍과 네거티브 프롬프트)와
`linetta_build_illustration_prompt`(프롬프트 조립, 이미지 생성은 하지 않음)를 만들었다.
그때 "나중에"로 미룬 두 가지를 이번에 한다.

- 앱 안에서 이미지 생성 모델을 직접 호출해 삽화를 만든다
- 만든 삽화를 씬 본문에 넣는다

연재 작가가 회차마다 올리는 삽화를 **같은 인물이 같은 얼굴로** 나오게 만들고, 원고와 같은 곳에 보관하는 것이 목표다.

## 가치 제안

> 웹소설 작가는 Linetta 안에서 장면을 고르고 등장인물의 비주얼 시트와 작품 화풍을 그대로 실은 삽화를 만들어,
> 작품 갤러리에 보관하고 씬 본문에 넣을 수 있다. 무엇이 어느 공급자로 가는지는 생성할 때마다 눈으로 확인한다.

## 결정 사항

- **공급자는 두 개만 우선 지원한다.**
  - **ChatGPT**: 기존 `openai-codex` 공급자(ChatGPT 계정 OAuth)를 그대로 쓴다. Codex Responses 백엔드에 호스티드
    `image_generation` 툴을 켠 요청을 보낸다. API 키는 필요 없다.
  - **Gemini**: 기존 `gemini-native` 공급자의 API 키로 `generateContent`에 `responseModalities: ["IMAGE"]`를 보낸다.
- **설정은 "이미지 생성" 항목으로 따로 둔다.** 에이전트 모델 설정(`ProviderSection`)과 같은 모양으로 공급자와 모델을 고른다.
  자격 증명(키, 로그인)은 provider 행의 것을 공유하고, 에이전트 쪽 활성 공급자와는 독립적으로 고른다.
  예를 들어 에이전트는 Claude를 쓰고 삽화는 Gemini로 만들 수 있다.
- **동의는 이미지 생성용으로 따로 받는다.** 기존 provider 동의 문장은 에이전트가 툴로 원고를 읽는 범위를 약속한다.
  삽화 생성은 보내는 것이 다르므로(작가가 확인한 프롬프트와 고른 참조 이미지) 공급자별로 한 번 따로 묻는다.
- **자동 생성은 없다.** 삽화는 작가가 버튼을 누를 때만 만든다. 생성 전에 보낼 프롬프트를 보여주고 고칠 수 있게 한다.
- **이미지는 `library.db`에 BLOB으로 저장한다.** 참조 이미지와 같은 이유다. 백업(`VACUUM INTO`)과 병합 복원에 그대로 실린다.
  대신 크기 상한을 두고 PNG가 아닌 JPEG나 WebP로 받는다.
- **본문에는 삽화 id만 넣는다.** TipTap `illustration` 노드의 attr로 id를 들고 있고 바이트는 갖지 않는다.
  그래야 스냅샷, 버전, 검색 인덱스가 커지지 않는다.
- **이미지 생성 API는 tars `pkg/llm`에 둔다.** Codex 자격 증명 갱신(`internal/auth`)이 tars 내부에 있어 Linetta에서 직접
  다룰 수 없다. 기존 클라이언트(`OpenAICodexClient`, `GeminiNativeClient`)에 `GenerateImage`를 더하는 편이 갱신, 재시도,
  rate limit 처리를 그대로 물려받는다.

## 확인되지 않은 가정 (Phase 1 첫 작업에서 스파이크로 검증)

- Codex 백엔드(`chatgpt.com/backend-api/codex/responses`)가 ChatGPT 로그인 토큰으로 `tools: [{"type":"image_generation"}]`를
  받아 `image_generation_call` 결과(base64)를 돌려준다. 공개 문서가 아닌 백엔드이므로 이벤트 모양, 스트리밍 필수 여부,
  툴의 `size`·`output_format`·`model` 파라미터를 받는지 직접 확인해야 한다.
- 어떤 라우팅 모델(요청의 `model`)이 이 툴을 쓸 수 있는지 확인해야 한다. 기본값 `gpt-5.3-codex-spark`가 안 되면
  이미지 생성용 기본 모델을 따로 정한다.
- Gemini 이미지 모델 id는 자주 바뀐다(`gemini-2.5-flash-image`, `gemini-3.1-flash-image` 등). 코드에 고정하지 않고
  모델 목록 조회 결과에서 이미지 출력 모델만 고른다.
- 참조 이미지(인물 시트의 이미지)를 두 공급자 모두 입력으로 받아 인물 일관성에 반영하는지 확인한다.

스파이크 결과가 가정과 다르면 Phase 1 문서의 해당 작업을 고치고 진행한다.

## 현재 코드베이스 근거

- 비주얼 시트·참조 이미지·화풍: `engine/internal/visual/visual.go`
  - `BuildPrompt(style, chars, scene, format)`가 결정적으로 프롬프트를 조립한다.
  - 참조 이미지는 BLOB, 장당 5 MiB, 인물당 8장. MIME 검사 패턴이 있다.
- 인물 해석과 프롬프트 툴: `engine/internal/mcphost/tools_visuals.go`
  - `resolveVisualCharacters`, `buildIllustrationPrompt`가 MCP 안에만 있다. RPC에서도 써야 하므로 Phase 1에서 `visual`로 옮긴다.
- 공급자: `engine/internal/provider` (`Source.Resolve`, `Source.Client`, `ListModels`), `engine/internal/settings/providers.go`
- tars 클라이언트: `pkg/llm/openai_codex_client.go`, `pkg/llm/gemini_native*.go` (현재는 이미지를 입력으로만 받는다)
- RPC 등록: `engine/internal/engineapp/engineapp.go` (`visuals.*` 등록 블록), 핸들러 `engine/internal/rpc/handlers/visuals.go`
- 설정 UI: `apps/desktop/src/routes/Settings.tsx`, `apps/desktop/src/components/settings/ProviderSection.tsx`
- 비주얼 UI: `apps/desktop/src/components/CharacterVisualSection.tsx`, `ArtStyleSection.tsx`
- 에디터: `apps/desktop/src/components/editor/Tiptap.tsx`와 확장들(`MentionExtension.ts`, `NoteMarkerExtension.ts`)
- 스키마 패리티: `apps/desktop/src/components/editor/editorSchema.test.ts`와 `engine/internal/export/testdata/editor_schema.json`
- 본문 JSON 소비처: `export/markdown.go`·`text.go`·`episode.go`, `manuscript/indexer.go`, `manuscriptedit`, `contextualedit`,
  `mcphost/tools_read.go`·`tools_write.go`, `snapshot`, `restore`
- 마이그레이션: `engine/internal/store/migrations/` (마지막 `0023_publish.sql`)

## 기능 범위

| 기능 | 설명 | Phase |
|---|---|---:|
| tars 이미지 생성 API | `ImageGenerator` 인터페이스, Codex·Gemini 구현 | 1 |
| 이미지 생성 설정 | 공급자·모델 선택, 공급자별 이미지 동의 | 1 |
| 삽화 만들기 대화상자 | 장면·등장인물·형식 선택, 프롬프트 미리보기·수정, 생성 | 1 |
| 작품 삽화 갤러리 | 생성 결과 보관, 캡션, 삭제, 크기 합계 | 1 |
| 씬 본문 삽입 | TipTap `illustration` 노드, 갤러리에서 넣기 | 2 |
| 본문 소비처 대응 | 글자 수·검색·평문·에이전트 읽기/쓰기·스냅샷·복원 | 2 |
| Markdown 내보내기·폴더/Git 동기화 이미지 | `images/` 폴더와 상대 경로 | 3 |
| Linetta Works 게시 | 삽화 업로드 후 본문 참조 | 3 |
| 에이전트 삽화 생성 툴 | 승인 카드를 거치는 `linetta_generate_illustration` | Later |

## Now / Next / Later

### Now

1. **Phase 1 — 삽화 생성과 갤러리** ([`phase-1-generate.md`](./phase-1-generate.md))
   - 끝나면: 설정에서 ChatGPT나 Gemini를 고르고, 씬에서 삽화를 만들어 작품 갤러리에서 본다.
2. **Phase 2 — 씬 본문 삽입** ([`phase-2-insert.md`](./phase-2-insert.md))
   - 끝나면: 갤러리의 삽화를 씬에 넣고, 저장·스냅샷·에이전트 편집·복원 후에도 삽화가 남는다.

### Next

3. **Phase 3 — 내보내기·동기화·게시** ([`phase-3-export-publish.md`](./phase-3-export-publish.md))
   - 끝나면: Markdown 내보내기와 폴더/Git 동기화에 이미지 파일이 함께 나가고, Works에 게시한 회차에 삽화가 보인다.
   - Works 쪽 작업(`linetta-works` 저장소)이 함께 필요하다.

### Later

- 에이전트가 삽화를 만드는 MCP·내장 툴. 생성마다 작가 승인 카드를 거친다.
- 4컷만화 레이아웃(패널 분할, 말풍선). Phase 1의 `four_panel`은 프롬프트 형식까지만이다.
- 삽화 편집(크롭, 부분 재생성), 같은 프롬프트로 여러 장 뽑아 고르기.
- 장소·소품 비주얼 시트.
- OpenAI API 키(`openai`), Anthropic 등 다른 공급자.

## Out of Scope

- 이미지 편집, 크롭, 필터
- 생성 비용 계산과 사용량 대시보드 (Codex rate limit 오류와 Gemini 과금 안내 문구만 둔다)
- 백그라운드 일괄 생성
- 모바일(iOS) 전용 UI. 엔진은 `-tags mobile` 빌드가 깨지지 않게만 한다.

## 검증 명령 (모든 Phase 공통)

- 엔진: `cd engine && CGO_ENABLED=0 go test ./...` (로컬. cgo 테스트는 로그인 키체인을 건드린다), CI와 같은 확인은 `make test-go`
- 모바일 태그: `make test-mobile-engine`
- 데스크톱: `make test-desktop`
- tars: tars 저장소의 `go test ./pkg/llm/...`
