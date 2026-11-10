# 회차 분량 Phase 2 — 에이전트가 분량 기준을 지키게

_상위 로드맵: [`roadmap.md`](./roadmap.md)_
_작성일: 2026-11-09_
_목표: 에이전트가 브리프에서 분량 기준과 이 회차의 부족분을 읽고, 회차를 짧게 끝내면 쓰기 결과에서 알게 되는 수직 슬라이스_
_예상 소요: 1일_
_전제: Phase 1 병합 (`episode_char_min`·`episode_char_max`, 데스크톱 `episodeLengthState`)_

## 배경

지금 브리프는 `회차 목표 분량: N자(공백 포함)` 한 줄을 싣는다. 그마저도 목표가 기본값(공백 포함 5,000)이면 싣지 않는다
(`storycontext.chosenEpisodeCharTarget`). 그리고 브리프는 **씬** 단위로 조립된다. 화 하나에 씬이 여럿이면, 에이전트는
이 씬을 몇 자로 써야 회차가 기준을 채우는지 알 수 없다.

LLM은 분량 지시를 받아도 짧게 끝내는 경향이 있다. 실사용 작품에서도 2~6화가 3,500~3,990자로 끝났다(목표 5,000).
그래서 숫자를 싣는 것만으로는 부족하고, 쓴 뒤에 결과로 다시 알려야 한다.

## Goal

- min이나 max가 있으면 브리프에 기준을 항상 싣는다. 문구는 요강의 말투를 따른다.
  - min만: `회차 분량: 최소 5,000자(공백 포함), 5,300자 안팎을 목표로`
  - 범위: `회차 분량: 4,500~5,500자(공백 포함), 5,000자 안팎을 목표로`
  - 목표만(작가가 고른 값): 지금 문구 그대로
- 웹소설 회차 안의 씬이면 브리프에 이 회차의 현재 분량을 싣는다.
  `이 회차: 지금 3,925자 (이 씬 제외 2,100자) · 최소까지 1,075자`
- 회차의 마지막 씬을 쓴 결과가 회차를 기준 밖에 두면 `writeSceneOutput.Warnings`에 한 줄을 더한다. 쓰기는 그대로 된다.
- `linetta_get_outline`의 회차 행에 회차 합계와 판정이 실린다. 외부 에이전트도 어느 화가 모자란지 한 번에 안다.

## Non-goals

- 분량 미달을 이유로 쓰기를 거절하는 문턱(로드맵 결정: 경고만)
- 에이전트가 스스로 이어 쓰게 하는 자동 반복. 경고를 받은 뒤 어떻게 할지는 에이전트 프롬프트의 지침과 작가의 지시를 따른다
- 회차 검토(`agent/episodereview.go`)에 분량 판정을 넣는 것. 검토는 문장을 판단하는 단계다. 분량은 숫자로 바로 알 수 있으니 경고로 충분하다

## Touch points

- `engine/internal/storycontext/types.go`(`ProjectMeta`, 새 `EpisodeLength`), `builder.go`, `render.go` (+ 테스트, 골든)
- `engine/internal/episode/episode.go` (씬 → 회차 찾기 도우미가 없으면 더한다)
- `engine/internal/mcphost/warnings.go`, `tools_write.go`, `tools_read.go` (+ 테스트)
- `engine/internal/agent/prompt.go` (경고를 읽는 지침 한 줄)
- 글자 수 기준: `engine/internal/charcount`

## Tasks

- [ ] **2.1 회차 분량 계산 도우미**
  - `episode` 패키지에 씬 id로 그 씬이 속한 `Episode`를 찾는 함수가 없으면 더한다(`Find(ctx, projectID) []Episode`를 한 번 돌려 `LeafIDs`로 찾는다)
  - 회차 합계는 각 `LeafIDs` 씬의 `word_count` 합(이미 작품의 글자 수 기준으로 센 값)
  - 판정은 데스크톱 `episodeLengthState`와 같은 규칙으로 Go에 둔다: `project.EpisodeLengthState(chars, min, target, max)`.
    두 구현이 같은 답을 내도록 경계값 표를 Go와 TS 테스트에 똑같이 둔다(`episode` 패키지가 이미 이 방식이다)
  - 웹소설 구조가 없는 작품(`Episode.Whole`)은 회차 분량을 계산하지 않는다
  - 테스트: 컨테이너 화, 리프 화, 구조 없는 작품, 빈 화

- [ ] **2.2 브리프의 분량 기준과 이 회차**
  - `ProjectMeta`에 `EpisodeCharMin`, `EpisodeCharMax`. `chosenEpisodeCharTarget` 대신 `chosenEpisodeLengthRule`이 셋을 정한다:
    min·max 중 하나라도 있으면 target도 함께 싣고, 둘 다 없으면 지금 규칙(기본값이면 0)
  - `Context`에 `Episode *EpisodeLength`(`Label`, `Chars`, `OtherScenesChars`, `Min`, `Max`, `Target`, `State`, `Delta`). 회차 안의 씬일 때만
  - `render.go`: 위 Goal의 세 문구. 세 언어(`langPick`). 이 회차 줄은 프로젝트 줄 바로 아래
  - 브리프 용량(장편 Phase 1)의 고정 분량에 두 줄이 더해진다. 예산 계산에 반영되는지 확인한다
  - 테스트: 세 문구, 기본값만 있는 작품은 지금과 바이트 단위로 같은 출력, 회차 줄 유무, 세 언어

- [ ] **2.3 쓰기 결과의 분량 경고**
  - `warnings.go`에 `lengthWarning(ctx, nodeID)`. **쓰기 뒤에** 읽는다(`planWarning`과 반대). 내보내는 경우:
    - 쓴 씬이 회차의 마지막 씬이고 회차가 min 미달: `episode 2화 is 3,925 characters (including spaces); the minimum is 5,000 — 1,075 short. Continue the episode or tell the writer it is short.`
    - 회차가 max 초과(어느 씬이든): `… over the 5,500 maximum by 320 …`
    - min·max가 없으면 아무것도 내지 않는다
  - "마지막 씬"으로 한정하는 이유: 화의 앞 씬을 쓰는 중에는 회차가 당연히 모자라다. 매번 경고하면 잡음이 된다
  - `tools_write.go`에서 기존 두 경고 뒤에 붙인다. 문서 문구(`warnings:` 설명)에 분량을 더한다
  - 내장 에이전트의 쓰기도 같은 `Warnings`를 모델에게 돌려주는지 확인한다(계획 문턱은 내장 에이전트에게 거절로 동작하지만, 분량은 둘 다 경고다)
  - 테스트: 마지막 씬 미달, 앞 씬은 경고 없음, 초과, 기준 없음, 리프 화(씬 하나짜리 화)

- [ ] **2.4 아웃라인 도구의 회차 분량**
  - `outlineRow`에 회차 행에만 `episode_chars`, `length_state`(`short`·`ok`·`long`·`progress`), `length_delta`(omitempty)
  - `getOutlineOutput`에 작품의 규칙(`episode_char_min`, `episode_char_target`, `episode_char_max`, `char_count_basis`)을 한 번
  - 도구 설명에 "회차 행의 length_state가 short면 그 회차는 작품의 최소 분량에 못 미친다"
  - 테스트: 회차 행과 씬 행 구분, 규칙 없는 작품은 새 필드 없음

- [ ] **2.5 에이전트 지침**
  - `prompt.go`의 쓰기 지침에 한 줄: 쓰기 결과에 분량 경고가 오면, 작가가 짧게 쓰라고 하지 않은 한 회차를 이어 쓰거나 모자란 채로 끝낸다고 작가에게 말한다
  - 지침이 바뀌면 프롬프트 골든 테스트를 갱신한다

---

### ✅ Checkpoint: Phase 2 완료 확인

**자동 확인:**
- [ ] `make test-go`, `make test-mobile-engine`, `make test-desktop`

**수동 확인 (사용자):**
- [ ] "최소 회차 · 공백 포함 5,000자 이상"을 고른 작품에서 에이전트에게 새 회차를 쓰게 하면, 브리프(에이전트 로그)에 최소 분량과 이 회차 줄이 보인다
- [ ] 에이전트가 회차를 5,000자 미만으로 끝내면 이어 쓰거나, 모자라다고 작가에게 말한다
- [ ] 외부 MCP 클라이언트(Claude Code 등)에서 `linetta_get_outline`을 부르면 모자란 회차가 `short`로 나온다
- [ ] 기준을 정하지 않은 작품의 에이전트 동작과 브리프는 전과 같다

**이 체크포인트를 통과하면 사용자에게 확인을 받고 로드맵에 완료를 표시한다.**
