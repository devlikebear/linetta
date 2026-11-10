# 회차 분량 기준 — 로드맵

_작성일: 2026-11-09_
_상태: Phase 1·2 병합(2026-11-10, #347·#348), Phase 3 작업지시서 작성, Phase 4는 로드맵 수준. 2026-11-09 사용자 결정 반영(기본 하한 5,000, 초과 보조색, 기존 작품 제안)_
_관련: #239 분량 프리셋, 마이그레이션 0011(`episode_char_target`)·0045(`char_count_basis`), 장편 일관성 Phase 6(집필 문턱의 `Warnings`)_

## Overview

리네타의 회차 분량 기준은 **숫자 하나**다. `projects.episode_char_target`(기본 5,000)과 글자 수 기준(`char_count_basis`, 공백 포함/제외)뿐이다.
게이지는 목표에 닿으면 색만 바뀌고, 모자란 회차는 아무 표시가 없다.

연재처와 공모전의 요강은 다르게 말한다.

| 출처 | 요강 표현 | 뜻 |
|---|---|---|
| 네이버시리즈 공모전 2건(2026, JC미디어·엔리즈) | 회당 공백 포함 5,000자 **이상**, 최소 5~7화 | 하한 + 화 수 |
| 광주콘텐츠코리아랩 웹소설 공모전(2026-11) | 회차당 공백 포함 5,000자 **이상**, 1~7화 | 하한 + 화 수 |
| 카카오페이지 (공식 기준 없음, 업계 관행) | 공백 포함 5,000~5,500자 | 범위 |
| 다른 공모전 목록의 사례 | 공백 포함 4,500~5,500자 | 범위 |
| 신춘문예, SF스토리 공모전 | 원고지 80매 안팎, 1.5만~3만 자 | 작품 전체 분량 |

숫자 하나로는 "이상"과 "안팎"과 "범위"를 구분하지 못한다. 그래서 작가는 2화가 3,925자여도 요강 미달인 줄 모르고 공개한다.
에이전트 쪽은 더 나쁘다. 목표가 기본값(공백 포함 5,000)이면 브리프에 분량을 아예 싣지 않는다(`storycontext.chosenEpisodeCharTarget`).
네이버 기준에 딱 맞춘 작품일수록 에이전트가 분량 지시를 받지 못한다.

## 가치 제안

> 웹소설 작가는 연재처나 공모전의 분량 요강을 한 번 정해 두면, 어느 회차가 모자라거나 넘치는지 아웃라인에서 바로 보고,
> 에이전트도 같은 기준으로 회차를 쓴다. 제출 전에는 화 수와 전체 분량까지 한 번에 점검한다.

## 결정 사항

- **목표는 그대로 두고 하한·상한을 더한다.** `episode_char_target`은 "권장 목표"로 뜻을 좁히고, `episode_char_min`과
  `episode_char_max`를 새로 둔다. 0은 "기준 없음"이다. 기존 작품은 둘 다 0이라 지금과 똑같이 동작한다.
- **새 웹소설 작품은 하한 5,000자로 시작한다 (2026-11-09 사용자).** 공백 포함, min 5,000 · target 5,300 · max 0.
  기본 프리셋 "최소 회차"와 같은 값이다. target을 min보다 조금 높게 두는 것은 에이전트가 "5,000자 안팎"을 목표로 받아
  하한 밑으로 끝내지 않게 하려는 것이다. 웹소설이 아닌 작품은 지금처럼 기준 없음이다.
- **기존 작품은 마이그레이션으로 바꾸지 않는다.** 앱은 배포되어 있어서 다른 작가의 작품에 갑자기 미달 경고가 뜨면 안 된다.
  대신 기준이 없는 웹소설 작품을 열면 한 번 제안한다(Phase 1의 1.7). 제안에서 "웹소설 작품 모두에 적용"으로 한 번에 바꿀 수 있다.
  별도의 일괄 수정 화면은 만들지 않는다.
- **상한 초과는 눈에 덜 띄게 표시한다 (2026-11-09 사용자).** 유료 회차는 상한이 거의 없다. 범위형 요강을 위해 모델에는 두되,
  게이지의 초과 표시는 경고색이 아니라 보조색이다.
  - 하나의 "모드"(이상/안팎/범위) 필드 대신 두 숫자를 둔다. 요강의 세 표현이 모두 (min, max) 쌍으로 적히고, 검사가 단순하다.
  - 저장할 때 검사: min ≤ max(둘 다 있을 때), target은 [min, max] 안. 어기면 `ErrInvalidEpisodeCharRange`.
- **미달은 경고이고 막지 않는다.** 작가의 저장, 에이전트의 쓰기, Works 공개 어느 것도 분량 때문에 거절하지 않는다.
  집필 중인 회차는 당연히 모자라기 때문이다. 대신 표시를 분명히 하고, 에이전트에게는 쓰기 결과의 `warnings`로 알린다.
- **기본 프리셋 이름은 분량으로 짓는다.** `charCountPresets.ts`의 원칙(사이트 이름 대신 길이 이름, 사이트·공모전 숫자는
  작가의 프리셋으로)을 지킨다. "네이버 공모전" 같은 프리셋은 기본으로 넣지 않는다.
- **회차를 찾는 규칙은 엔진의 `episode` 패키지를 쓴다.** 데스크톱과 같은 규칙으로 옮겨 둔 것이다. 브리프와 쓰기 경고가 회차 합계를
  계산할 때 이것을 쓴다.
- **에이전트 브리프는 기준이 있으면 항상 싣는다.** min이나 max가 있으면 기본값 여부와 상관없이 싣는다. 목표만 있고
  기본값 그대로면 지금처럼 싣지 않는다(작가가 대화에서 말한 분량과 다투지 않게 하는 기존 판단을 지킨다).

## 현재 코드베이스 근거

- 모델과 저장: `engine/internal/project/project.go`(`DefaultEpisodeCharTarget = 5000`, `Project`, `UpdateInput`),
  `engine/internal/project/repo.go`(`Update`의 `EpisodeCharTarget` 검사), RPC 오류 매핑 `engine/internal/rpc/handlers/projects.go:106`
- 마이그레이션: `engine/internal/store/migrations/` (마지막 `0047_ledger_secrets.sql`)
- 작가 프리셋: `engine/internal/settings/charcount_presets.go`(`CharCountPreset`, `normalizeCharCountPreset`),
  `engine/internal/settings/settings.go`(`CharCountPresets`)
- 마크다운 메타데이터와 왕복: `engine/internal/mdmeta/metadata.go`, `engine/internal/export/project.go:324`, `engine/internal/importmd/extras.go:62`
- 에이전트 브리프: `engine/internal/storycontext/builder.go`(`chosenEpisodeCharTarget`), `types.go`(`ProjectMeta`), `render.go`(`회차 목표 분량` 줄)
- 회차 찾기: `engine/internal/episode/episode.go`(`Episode.LeafIDs`)
- 쓰기 결과 경고: `engine/internal/mcphost/tools_write.go`(`writeSceneOutput.Warnings`, `planWarning`, `settlementWarning`)
- 아웃라인 도구: `engine/internal/mcphost/tools_read.go`(`outlineRow`)
- 공개: `engine/internal/publish/service.go`(회차별 `CharCount`)
- 데스크톱: `apps/desktop/src/lib/charCountPresets.ts`, `lib/types.ts`, `components/CharCountPresetPicker.tsx`,
  `components/ContextPanel.tsx`(회차 목표 입력), `components/OutlinePanel.tsx:633`(회차 게이지), `components/StatsSection.tsx`,
  `routes/Workspace.tsx`(진행 문구·젠 모드 목표), `lib/i18n.tsx`(ko·en·ja)

## 기능 범위

| 기능 | 설명 | Phase |
|---|---|---:|
| 하한·상한 모델 | `episode_char_min`·`episode_char_max`, 검사, 프리셋·내보내기 왕복 | 1 |
| 회차 게이지 상태 | 미달(부족분 표시)·충족·초과 세 상태 | 1 |
| 분량 입력 UI | 회차 목표 옆 최소·최대, 프리셋에 범위 표시 | 1 |
| 새 웹소설 작품 기본 하한 | min 5,000 · target 5,300(공백 포함) | 1 |
| 기존 작품 기준 제안 | 기준 없는 웹소설 작품에 한 번 제안, 모두에 적용 | 1 |
| 브리프의 분량 기준 | "최소 N자, 약 M자 목표" 문구, 이 회차의 현재 분량과 부족분 | 2 |
| 쓰기 결과 분량 경고 | 회차 마지막 씬을 쓴 뒤 미달·초과면 `warnings` | 2 |
| 아웃라인 도구의 회차 분량 | `linetta_get_outline` 행에 회차 합계와 기준 판정 | 2 |
| 제출 기준과 점검 | 최소 화 수, 작품 전체 분량, 시놉시스 여부, 체크리스트 | 3 |
| 공개 전 미달 경고 | Works 공개 창에서 미달 회차 표시 | 3 |
| 원고지 매수 환산 | 200자 원고지 간이 환산(단편·장편 작품) | 4 |

## Now / Next / Later

### Done

1. ✅ **Phase 1 — 하한·상한과 회차 게이지** ([`phase-1-range-and-gauge.md`](./phase-1-range-and-gauge.md), #347, 2026-11-10 병합)
   - 끝나면: 새 웹소설 작품은 "공백 포함 5,000자 이상"으로 시작하고, 기존 작품은 열 때 한 번 제안받는다.
     아웃라인에서 모자란 회차가 부족분과 함께 경고색으로 보인다.
2. ✅ **Phase 2 — 에이전트가 분량 기준을 지키게** ([`phase-2-agent-length.md`](./phase-2-agent-length.md), #348, 2026-11-10 병합)
   - 끝나면: 에이전트 브리프에 기준과 이 회차의 부족분이 실리고, 회차를 짧게 끝내면 쓰기 결과의 `warnings`로 알게 된다.
   - 남은 수동 확인: 실제 에이전트가 경고를 받고 이어 쓰거나 모자라다고 말하는지(체크포인트의 수동 항목).

### Now

3. **Phase 3 — 제출 기준과 점검** ([`phase-3-submission-check.md`](./phase-3-submission-check.md))
   - 작품에 "제출 기준"을 둔다. 최소 화 수, 작품 전체 최소·최대 분량, 시놉시스 필요 여부.
   - 작업 공간에 체크리스트를 보인다. 예: "7화 중 5화 충족 · 2화 −1,075자 · 시놉시스 있음".
   - Works 공개 창에 지금 회차의 미달을 표시한다(막지 않는다).
   - 제출 기준은 분량 프리셋과 따로 둔다(2026-11-10 사용자).

### Later

4. **Phase 4 — 원고지 매수 환산**
   - 단편·장편 작품(웹소설 프리셋이 아닌 작품)에 "200자 원고지 약 N매"를 보인다.
   - 원고지는 문단 첫 칸 들여쓰기와 줄 끝 빈칸 때문에 글자 수 ÷ 200보다 매수가 많다. 정확한 원고지 규칙(문장부호 칸, 대화문 들여쓰기)은
     하지 않고, 문단별로 "들여쓰기 1칸 + 글자 수를 20칸 줄로 올림"하는 간이 환산이라고 표시한다.

## Out of Scope

- 연재처·공모전 요강을 자동으로 가져오거나 갱신하는 기능
- 분량 미달을 이유로 저장·쓰기·공개를 거절하는 문턱
- 회차를 자동으로 나누거나 합치는 기능(분량에 맞춰 씬을 다음 화로 옮기기 등)
- 공백 포함/제외 외의 글자 수 규칙(문장부호 제외, 원고지 칸 단위 등). 원고지는 Phase 4의 표시용 환산뿐이다.

## 정리된 질문 (2026-11-09 사용자)

- 새 웹소설 작품의 기본 하한 → **5,000자(공백 포함)**. 결정 사항 참고.
- 상한 초과 표시 → **눈에 덜 띄는 보조색**. 결정 사항 참고.
- 기존 작품 일괄 수정 → **별도 화면 없이**, 기준 없는 웹소설 작품을 열 때 한 번 제안하고 거기서 모두에 적용할 수 있게 한다(Phase 1의 1.7).
