# 회차 분량 Phase 1 — 하한·상한과 회차 게이지

_상위 로드맵: [`roadmap.md`](./roadmap.md)_
_작성일: 2026-11-09_
_목표: 회차 분량 기준을 숫자 하나에서 (최소, 목표, 최대)로 넓히고, 아웃라인 게이지가 미달·충족·초과를 구분해 보이는 수직 슬라이스_
_예상 소요: 2일_
_전제: `main` 최신. 마이그레이션 다음 번호 확인(작성 시점 0048)_

## Goal

- 작품마다 회차 최소·최대 글자 수를 정할 수 있다. 0은 기준 없음이고, 기존 작품은 지금과 똑같이 보인다.
- 새 웹소설 작품은 공백 포함 min 5,000 · target 5,300으로 시작한다(기본 프리셋 "최소 회차"와 같은 값).
- 기준이 없는 기존 웹소설 작품을 열면 한 번 제안하고, 거기서 이 작품이나 웹소설 작품 모두에 적용할 수 있다.
- 아웃라인의 회차 게이지가 세 상태로 보인다.
  - **미달**(min이 있고 합계 < min): 경고색, `3,925 / 5,000` 옆에 `−1,075`
  - **충족**: 지금의 완료 색
  - **초과**(max가 있고 합계 > max): 눈에 덜 띄는 색과 `+320`
  - min이 없으면 지금처럼 target 기준으로 채우고, 미달 표시는 하지 않는다.
- 분량 프리셋이 (기준, 최소, 목표, 최대)를 함께 들고, 고르면 한 번에 적용된다.
- 마크다운 내보내기·가져오기, 폴더/Git 동기화에서 새 값이 따라간다.

## Non-goals

- 에이전트 브리프·쓰기 경고 (Phase 2)
- 화 수·작품 전체 분량·공개 경고 (Phase 3)
- 미달을 이유로 무엇이든 거절하기

## Touch points

- `engine/internal/store/migrations/0048_project_episode_char_range.sql`(새)
- `engine/internal/project/project.go`, `repo.go` (+ `repo_test.go`)
- `engine/internal/rpc/handlers/projects.go` (오류 매핑)
- `engine/internal/settings/charcount_presets.go`, `settings.go` (+ 테스트)
- `engine/internal/mdmeta/metadata.go`, `engine/internal/export/project.go`, `engine/internal/importmd/extras.go` (+ 왕복 테스트)
- `apps/desktop/src/lib/types.ts`, `lib/charCountPresets.ts`(+ test), `lib/episodeLength.ts`(새, + test)
- `apps/desktop/src/components/CharCountPresetPicker.tsx`, `ContextPanel.tsx`, `OutlinePanel.tsx`(+ 각 test)
- `apps/desktop/src/lib/i18n.tsx` (ko·en·ja), 게이지 CSS

## Tasks

- [ ] **1.1 엔진 모델과 검사**
  - 마이그레이션: `ALTER TABLE projects ADD COLUMN episode_char_min INTEGER NOT NULL DEFAULT 0;` 와 `episode_char_max` 같은 모양
  - `Project`에 `EpisodeCharMin`, `EpisodeCharMax`(JSON `episode_char_min`, `episode_char_max`), `UpdateInput`에 포인터 필드
  - `repo.Update` 검사(갱신 뒤의 세 값으로 한 번에):
    - min, max는 0 이상
    - min > 0 && max > 0 이면 min ≤ max
    - target은 min 이상, max가 있으면 max 이하
    - 어기면 `ErrInvalidEpisodeCharRange`. RPC 핸들러가 기존 `ErrInvalidEpisodeCharTarget`과 같은 잘못된 입력 오류로 돌려준다
  - 한 번의 `Update`로 셋을 함께 바꿀 수 있어야 한다(프리셋 적용). 순서에 따라 중간 상태가 거절되면 안 된다
  - **새 웹소설 작품의 기본값**: `repo.Create`에서 `outline_preset`이 `webnovel`이면 `episode_char_min = 5000`,
    `episode_char_target = 5300`(공백 포함). 상수 `DefaultWebnovelEpisodeCharMin`, `DefaultWebnovelEpisodeCharTarget`으로 두고,
    `DefaultEpisodeCharTarget`(5,000)은 다른 작품과 마이그레이션 기본값으로 그대로 둔다. 컬럼 기본값은 0이라 기존 작품은 바뀌지 않는다
  - 테스트: 기본값 0, 새 웹소설 작품 5,000/5,300, 새 일반 작품 0/5,000, 세 값 함께 바꾸기, 범위 위반 각각, 기존 target만 바꾸기는 그대로 동작

- [ ] **1.2 작가 프리셋**
  - `CharCountPreset`에 `EpisodeCharMin`, `EpisodeCharMax`(omitempty). `normalizeCharCountPreset`이 1.1과 같은 규칙으로 검사
  - 기존 `settings.json`의 프리셋(두 필드 없음)은 0으로 읽힌다
  - 테스트: 옛 프리셋 읽기, 범위 위반 거절, 저장 왕복

- [ ] **1.3 마크다운 메타데이터 왕복**
  - `mdmeta.Metadata`에 `episode_char_min`, `episode_char_max`(omitempty). 내보내기에서 쓰고 가져오기에서 0보다 크면 적용
  - **옛 파일 가져오기**: 가져오기는 새 작품을 만들므로 웹소설이면 1.1의 기본값(min 5,000)이 먼저 들어간다.
    파일에 `episode_char_target`이나 `char_count_basis`는 있는데 `episode_char_min`이 없으면 옛 작품의 내보내기이므로 min을 0으로 둔다.
    그래야 공백 제외 4,000자 같은 옛 규칙이 하한과 충돌하지 않고, 원래 작품과 같게 돌아온다
  - 폴더/Git 동기화가 같은 메타데이터를 쓰는지 확인하고, 다른 경로가 있으면 함께 고친다
  - 테스트: 내보내기 → 가져오기 왕복으로 세 값이 남는다. 두 필드가 없는 옛 파일(웹소설, 공백 제외 4,000)은 min 0으로 읽힌다

- [ ] **1.4 데스크톱 규칙과 프리셋**
  - `types.ts`의 `Project`, `CharCountPreset`에 두 필드
  - `charCountPresets.ts`: `LengthRule`에 `episode_char_min`, `episode_char_max`. `lengthRuleOf`, `sameRule`, `newCharCountPreset`이 네 값을 다룬다
  - 기본 프리셋에 하한형 하나를 더한다. 이름은 길이로 짓는다: `charPreset.builtin.minimumWithSpaces` = "최소 회차", 공백 포함, min 5,000 · target 5,300 · max 0
    - 기존 네 기본 프리셋은 min·max 0 그대로라, 지금 그 프리셋을 쓰는 작품의 "일치하는 프리셋" 표시가 바뀌지 않는다
  - `lib/episodeLength.ts`(새): `episodeLengthState(chars, rule)` → `{ state: "short" | "ok" | "long" | "progress", delta }`
    - min이 없을 때는 `"progress"`(지금 동작: target에 닿으면 완료 색)
    - 순수 함수로 두고 게이지와 이후 Phase들이 같이 쓴다
  - 테스트: 상태 판정 경계값(min과 같을 때 충족, max와 같을 때 충족), 옛 프리셋 일치, 새 기본 프리셋

- [ ] **1.5 입력 UI**
  - `ContextPanel`의 "회차 목표 ○자" 줄 아래에 "최소 ○자 · 최대 ○자"를 같은 `InlineEditableText`로 둔다. 비워 두거나 0이면 "없음"
  - 저장 오류(범위 위반)는 입력 칸에서 바로 보인다. 기존 `saveEpisodeTarget`처럼 throw하면 `InlineEditableText`가 되돌린다
  - `CharCountPresetPicker` 옵션 문구에 범위를 싣는다: 예) `최소 회차 · 공백 포함 5,000자 이상`, `표준 회차 · 공백 포함 5,000자`
    (`charPreset.option`을 min·max 유무에 따라 세 가지로 나눈다)
  - 프리셋 적용은 `projectsApi.update` 한 번으로 basis·min·target·max를 함께 보낸다
  - 테스트: 최소 입력·지우기, 범위 위반 되돌림, 프리셋 적용 시 한 번의 update, 옵션 문구

- [ ] **1.6 회차 게이지**
  - `OutlinePanel`의 `episodeProgress`가 `episodeLengthState`를 쓴다. 클래스 `is-short`·`is-complete`·`is-long`
  - 미달·초과일 때 개수 뒤에 부족분·초과분(`−1,075`, `+320`)과 `title`(예: "최소 5,000자까지 1,075자 모자람")
  - 게이지 채움 비율의 기준은 min이 있으면 min, 없으면 target
  - 화(컨테이너)와 화 라벨의 리프 모두 같은 규칙. 씬(화 아래 리프)에는 붙이지 않는다(지금과 같음)
  - 세 언어 문구. 미달은 기존 경고 토큰, 충족은 완료 토큰, **초과는 보조(muted) 토큰**(2026-11-09 사용자 결정: 눈에 덜 띄게). 다크 모드 확인
  - 테스트: 세 상태 렌더, min 없는 작품은 지금과 같은 마크업

- [ ] **1.7 기존 작품에 기준 제안 (일괄 적용 포함)**
  - 별도 일괄 수정 화면은 만들지 않는다. 대신 웹소설 작품인데 min과 max가 모두 0인 작품을 열면 `ContextPanel` 분량 영역 위에 한 번 제안한다:
    "이 작품에는 회차 최소 분량이 없습니다. 공백 포함 5,000자 이상으로 둘까요?" [이 작품에 적용] [웹소설 작품 모두에 적용] [괜찮아요]
  - **이 작품에 적용**: 1.4의 "최소 회차" 규칙을 적용한다. 단 기존 target이 5,300보다 크면 target은 그대로 두고 min만 넣는다(6,000자 작품은 6,000 유지)
  - **웹소설 작품 모두에 적용**: 보관하지 않은 웹소설 작품 중 min·max가 0인 것에 같은 규칙을 `projects.update`로 하나씩 적용한다(새 RPC 없음)
    - 글자 수 기준이 공백 제외인 작품은 건너뛴다. 공백 제외 5,000자는 공백 포함으로 6,000자 안팎이라 다른 기준이 된다
    - 끝나면 "웹소설 작품 5개에 적용했습니다. 공백 제외 기준 1개는 건너뛰었습니다." 처럼 결과를 보이고, 건너뛴 작품 이름을 나열한다
  - **괜찮아요**: 그 작품에서 다시 묻지 않는다. 작품별로 `localStorage`에 기억한다(기기마다 한 번 묻는 정도로 충분하다). 적용하면 min이 생기므로 제안은 저절로 사라진다
  - 테스트: 제안 표시 조건(웹소설·min/max 0·닫지 않음), 이 작품 적용, target 6,000 유지, 모두 적용에서 공백 제외 건너뛰기와 결과 문구, 닫기 기억

---

### ✅ Checkpoint: Phase 1 완료 확인

**자동 확인:**
- [ ] `make test-go`, `make test-mobile-engine`, `make test-desktop`

**수동 확인 (사용자):**
- [ ] 기존 웹소설 작품을 열면 아웃라인은 전과 똑같고, 오른쪽 패널에 기준 제안이 한 번 보인다
- [ ] 제안에서 "웹소설 작품 모두에 적용"을 누르면 기존 웹소설 작품들에 최소 5,000자가 들어가고, 6,000자 목표 작품은 목표가 그대로다
- [ ] 새 웹소설 작품을 만들면 분량 프리셋이 "최소 회차 · 공백 포함 5,000자 이상"으로 보인다
- [ ] 분량 프리셋에서 "최소 회차 · 공백 포함 5,000자 이상"을 고르면, 5,000자 미만 회차가 경고색과 부족분으로 보인다
- [ ] 최소 6,000 · 최대 5,000처럼 맞지 않는 값을 넣으면 저장되지 않고 원래 값으로 돌아간다
- [ ] 마크다운으로 내보낸 뒤 새 작품으로 가져오면 최소·최대가 그대로다

**이 체크포인트를 통과하면 사용자에게 확인을 받고 로드맵에 완료를 표시한다.**
