# 체험 모드 — 에이전트 도구 20개 프로필

_작성일: 2026-11-11_
_목표: 체험 모드 턴에서 에이전트가 받는 도구를 49개에서 20개로 줄여, 요청마다 붙는 고정 토큰을 반으로 줄인다_
_예상 소요: 1.5일_
_전제: `main` 최신. 체험 공급자(리네타웍스 중앙 키)는 아직 없다. 이 작업은 프로필을 만들어 두기만 하고, 켜는 스위치는 체험 공급자 작업이 연결한다_

## 배경 (실측, 2026-11-11)

앱 DB 사본에 실제 엔진과 실제 도구를 붙이고 모델 응답만 각본으로 대신해, 요청마다 모델이 받는 내용을 o200k 토크나이저로 셌다.

- 요청마다 **도구 정의 13,148토큰**과 시스템 프롬프트 2.4천~4.6천 토큰이 붙는다. 짧은 작품에서는 이것이 입력의 70~85%다.
- 체험자와 같은 조건(짧은 작품, 대화 이력 없음)에서 턴당 입력은 질문 5.5만, 부분 퇴고 13.5만, 새 회차 20.7만 토큰이다.
- 지난 한 달 실사용 338턴의 도구 호출 3,616회 가운데 상위 11개 도구가 79%를 차지한다. 49개 중 8개는 한 번도 쓰이지 않았다.

아래 20개로 줄이면 도구 정의는 **5,477토큰(−58%)**이 된다. 같은 시나리오의 턴당 입력은 다음처럼 줄어든다.

| 시나리오 | 지금 | 체험 프로필 | 절감 |
|---|---|---|---|
| 질문 (짧은 작품) | 54,875 | 31,862 | −42% |
| 부분 퇴고 (짧은 작품) | 134,967 | 88,941 | −34% |
| 새 회차 (짧은 작품) | 207,409 | 137,094 | −34% |
| 새 회차 (30화 작품, 긴 이력) | 595,901 | 525,593 | −12% |

체험 1인분(질문 10, 퇴고 10, 새 회차 3)으로 치면 입력이 252만에서 162만 토큰으로 줄어 **−36%**다.
시스템 프롬프트에서 빠지는 문장과 꺼지는 백그라운드 검토는 이 표에 넣지 않았다. 실제 절감은 이보다 조금 더 크다.

## Goal

- 체험 프로필이 켜진 턴은 도구 20개만 받는다(아래 목록. `linetta_ask_writer`는 지금처럼 `loop.go`가 따로 붙이므로 mcphost 허용 목록은 19개).
- 프롬프트는 그 턴이 받은 도구만 이름으로 부른다. 프롬프트가 없는 도구를 부르는 일이 구조적으로 생기지 않는다.
- 체험 프로필에서는 백그라운드 패스(자체 검토, 정산, 회차 검토, 영향 검토)가 돌지 않는다.
- 프로필이 꺼져 있으면 도구 목록, 프롬프트, 패스가 지금과 바이트 단위로 같다.

### 남기는 도구 (20)

| 묶음 | 도구 | 이유 |
|---|---|---|
| 읽기 | `get_story_context`, `read_scene`, `get_outline`, `search_manuscript` | 실사용 상위. 쓰기 게이트가 `get_story_context`를 요구한다 |
| 이야기 세계 읽기 | `list_characters`, `get_plot`, `get_fact_cards`, `where_does_appear` | 인물·설정을 묻는 질문에 답하려면 필요하고, 정의가 짧다 |
| 원고 쓰기 | `write_scene`, `revise_scene`, `write_summary` | 체험의 핵심. 계획 게이트가 `write_summary as_plan`을 요구한다 |
| 구조 | `apply_story_ops` | 새 회차 자리를 만들고 줄거리 갈래를 기록한다. "다음 화 써줘"에 필요하다 |
| 안전 | `create_checkpoint`, `undo_last_change` | 체험자가 되돌릴 수 있어야 한다 |
| 문체 | `analyze_style`, `propose_style_profile` | "내 문체 분석해줘"가 체험에서 보여 줄 만한 기능이다 |
| 연속성 | `report_continuity`, `list_continuity`, `list_ledger` | 리네타의 차별점인 연속성 검토를 보여 준다. 기록은 하되 원장 편집은 뺀다 |
| 질문 카드 | `ask_writer` | 작가 결정이 필요한 곳을 카드로 묻는다 |

### 빼는 도구 (29)

| 묶음 | 도구 | 빼는 이유 |
|---|---|---|
| 작품 관리 | `list_works`, `register_work`, `create_work` | 체험은 열린 작품 하나에서만 한다 |
| 삽화·시각 설정 | `get_character_visuals`, `build_illustration_prompt`, `list_illustrations`, `request_illustration`, `set_representative_illustration`, `set_character_visuals`, `set_art_style`, `add_reference_image`, `delete_reference_image` | 이미지 생성은 텍스트 체험 범위 밖이다 |
| 게시 | `publish_episode`, `publish_status`, `publish_cover` | 리네타웍스 계정이 있어야 한다 |
| 기억·스킬 | `edit_memory`, `read_skill`, `edit_skill` | 체험자에게는 쌓인 기억과 스킬이 없다. 스킬이 빠지면 자체 검토도 함께 꺼진다 |
| 대화 이력 | `search_history`, `read_history` | 체험 대화는 짧다 |
| 제안함 | `propose_story_ops`, `list_proposals`, `accept_proposals`, `reject_proposals` | 원고에서 이야기 세계를 대량으로 뽑는 기능이라 체험에는 무겁다(`propose_story_ops` 1,217토큰) |
| 기타 검토 | `save_forecast`, `find_impact`, `update_continuity`, `set_part_arc`, `edit_ledger` | 장편 운영 도구다. `edit_ledger`가 빠지면 정산 패스도 꺼진다 |

## Non-goals

- 체험 공급자, 리네타웍스 프록시, 사용량 상한 (별도 작업)
- 설정 화면에 체험 프로필 스위치 노출. 작가가 고르는 값이 아니라 체험 공급자가 정하는 값이다
- 외부 MCP 클라이언트의 도구 목록. 외부 클라이언트는 자기 LLM을 쓰므로 체험과 상관없다
- 도구 설명문 다듬기. `apply_story_ops`(957)와 `report_continuity`(816)가 남은 5,477토큰의 1/3이라 다음에 볼 만하지만, 이번 범위는 아니다

## Touch points

- `engine/internal/mcphost/budget.go`: `ToolGroups.Trial`, `TrialToolNames`, `excludedByGroups`, `ToolNames`, `MeasureToolBudget`
- `engine/internal/mcphost/budget_test.go`: `TestToolGroupNamesAreAllRealTools`, `TestToolNamesMatchesWhatTheServerActuallyServes`
- `engine/internal/agent/prompt.go`, `prompt_test.go`: `toolGroups.trial`, 문장별 도구 표시
- `engine/internal/agent/tools.go`: `hostGroups`
- `engine/internal/agent/agent.go`: `Deps.TrialToolsEnabled`
- `engine/internal/agent/loop.go`, `pass.go`: 그룹을 한 번 읽는 곳 두 군데
- `engine/internal/agent/episodereview.go`, `selfreview.go`: 패스 끄기
- `engine/internal/engineapp/agent_enabled.go`: 배선 (이번에는 항상 false)

## Tasks

- [ ] **1. mcphost 허용 목록**
  - `ToolGroups`에 `Trial bool`을 더한다. 주석: 체험 공급자가 켜는 값이고 작가의 스위치가 아니며, 켜지면 아래 허용 목록 밖의 도구를 모두 뺀다
  - `TrialToolNames`(19개, `ask_writer` 제외)를 둔다. 다른 그룹 목록처럼 `ReadToolNames`·`WriteToolNames`의 부분집합이다
  - `excludedByGroups` 맨 앞에 `if groups.Trial && !slices.Contains(TrialToolNames, name) { return true }`를 넣는다. 다른 그룹 스위치는 그 뒤에 그대로 적용된다(허용 목록 ∩ 스위치)
  - `ToolGroupsFrom`은 `Trial`을 읽지 않는다. 설정 저장소에 그런 값이 없고, 외부 서버는 체험과 상관없다
  - 테스트
    - `TestToolGroupNamesAreAllRealTools`에 `TrialToolNames`를 더한다
    - `TestToolNamesMatchesWhatTheServerActuallyServes`의 조합에 `Trial: true`를 더한다
    - `ToolNames(full, {Trial: true, 모든 그룹 on})`이 정확히 19개이고 `TrialToolNames`와 같은 집합인지
    - 기존 조합의 결과가 바뀌지 않았는지

- [ ] **2. 에이전트 그룹과 배선**
  - `agent.toolGroups`에 `trial bool`, `hostGroups()`가 `Trial`로 넘긴다
  - `Deps.TrialToolsEnabled func() bool`을 더하고, `loop.go:100`과 `pass.go:110`에서 다른 스위치와 같은 자리에서 **한 번만** 읽는다. nil이면 false다(다른 스위치의 nil=켜짐과 반대이니 주석으로 이유를 남긴다: 꺼진 것이 기본 제품이다)
  - 세션 캐시는 `toolGroups` 값으로 구분되므로 필드만 더하면 체험 턴과 일반 턴이 다른 세션을 쓴다. 이 동작을 테스트로 고정한다
  - `engineapp/agent_enabled.go`는 `TrialToolsEnabled`를 비워 둔다. 체험 공급자 작업이 "현재 공급자가 체험 공급자인가"로 채운다

- [ ] **3. 프롬프트가 받은 도구만 부르게**
  - 지금 `systemPrompt`의 기본 문장들은 삽화 읽기 도구, `create_checkpoint`, `propose_style_profile`, `apply_story_ops`, `ask_writer`를 그룹과 상관없이 부른다. 체험 프로필에서 이 중 삽화 문장은 빠져야 하고, 기억 문장의 `search_history`도 빠져야 한다
  - 방법: 문장마다 그 문장이 부르는 도구를 함께 적고, 그 도구가 모두 이 턴의 목록에 있을 때만 싣는다. 판단 기준은 `toolGroups`에서 계산한 이름 집합(= `mcphost.ToolNames(full, groups.hostGroups())` + `ask_writer`)이다. 문장 안의 도구 이름을 문자열에서 뽑아 쓰지 않는다
  - 체험 프로필에서는 기억 블록의 용량 줄과 기록 지시를 빼고(지금 `memory` off와 같다), 스킬 블록도 뺀다(`skills` off와 같다). 체험 프로필이면 이 두 그룹을 off로 취급하는 것과 같다
  - 테스트 (일반화)
    - `TestSystemPrompt_withBothGroupsOffNamesNoRemovedTool`를 일반화해, 모든 그룹 조합 × 체험 on/off에서 프롬프트에 나오는 `linetta_[a-z_]+`가 모두 그 턴의 도구 목록에 있는지 본다. 앞으로 문장이 늘어도 이 테스트 하나가 지킨다
    - 체험 off의 프롬프트가 지금과 같은지 (기존 프롬프트 테스트가 그대로 통과)
    - 체험 on에서도 `get_story_context`, `revise_scene`, `<selection>` 안내, `ask_writer` 안내는 남는지

- [ ] **4. 백그라운드 패스 끄기**
  - 자체 검토는 스킬 도구가 없으면 이미 돌지 않는다(`offers` 검사). 정산(`edit_ledger`)과 영향 검토(`find_impact`)도 도구가 빠지면 이미 꺼진다
  - 회차 검토는 `report_continuity`가 남으므로 돈다. 체험에서는 끈다: `maybeEpisodeReview`에서 `st.groups.trial`이면 패스를 만들지 않는다
  - 테스트: 체험 턴에서 `write_scene` 뒤에 패스 Chat 호출이 0회인지 (기존 `episodereview_test.go`의 각본 클라이언트로)

- [ ] **5. 도구 예산 측정값**
  - `MeasureToolBudget`은 설정 화면용이라 체험 프로필을 보여 줄 곳이 없다. 대신 `budget_test.go`에 체험 프로필의 바이트 상한을 하나 둔다(이번 측정에서 19개의 함수 스키마가 약 19.3 kB였다. 상한은 구현 시점에 `MeasureToolBudget`과 같은 방법으로 잰 값 +10%). 도구 설명이 길어져 체험 비용이 몰래 늘어나는 것을 막는다

## 검증

- `CGO_ENABLED=0 go test ./internal/mcphost/... ./internal/agent/... ./internal/engineapp/...`
- 실측 재현: 이번 측정과 같은 방법으로(DB 사본, 기록용 각본 클라이언트, o200k) 체험 on/off의 턴당 입력을 다시 재서 PR 본문에 표로 남긴다. 기대값은 짧은 작품에서 −34~42%다
- 체험 on으로 실제 공급자 한 번: 질문, 선택 문단 퇴고, 새 회차 쓰기 세 턴이 "없는 도구를 불렀다"는 오류 없이 끝나는지. 체험 공급자가 없으므로 테스트 전용 스위치로 켠다

## 이후

- 체험 공급자 작업이 `Deps.TrialToolsEnabled`를 채운다
- 남은 큰 도구 두 개(`apply_story_ops`, `report_continuity`)의 설명을 체험용으로 줄일지는 체험 공급자 단가가 정해진 뒤 판단한다
