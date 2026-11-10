# 회차 분량 Phase 3 — 제출 기준과 점검

_상위 로드맵: [`roadmap.md`](./roadmap.md)_
_작성일: 2026-11-10_
_목표: 작가가 공모전이나 연재처에 낼 작품의 제출 기준(화 수, 작품 전체 분량, 시놉시스)을 정해 두고, 작업 공간과 Works 공개 창에서 무엇이 모자란지 한눈에 보는 수직 슬라이스_
_예상 소요: 2일_
_전제: Phase 1·2 병합(#347, #348)_

## 배경

Phase 1·2로 회차 하나가 기준을 채우는지는 보인다. 그러나 공모전 요강은 회차 하나로 끝나지 않는다.

| 요강 항목 | 예 | 지금 리네타 |
|---|---|---|
| 최소 화 수 | 네이버 공모전 "최소 5~7화", 광주 "1~7화" | 없음. 아웃라인에서 직접 센다 |
| 회차당 분량 | 회당 공백 포함 5,000자 이상 | Phase 1 (회차 게이지) |
| 작품 전체 분량 | 신춘문예·SF 공모전 1.5만~3만 자, 원고지 80매 안팎 | 통계의 작품 글자 수뿐. 기준과 비교하지 않는다 |
| 시놉시스·기획서 | 대부분의 웹소설 공모전이 요구 | 시놉시스는 작품에 있다. 기획서는 앱 밖 |

작가가 알고 싶은 것은 "지금 내도 되나, 아니면 무엇이 모자라나"이다. 예: `7화 중 5화 썼음 · 기준 충족 3화 · 2화 −1,075자, 4화 −620자 · 시놉시스 있음`.

## 결정 (작업지시서 작성 시점의 제안. 시작 전에 사용자 확인)

- **제출 기준은 분량 프리셋과 따로 둔다 (제안).** 로드맵은 "분량 + 제출 기준"을 한 프리셋에 담을지 Phase 3 시작 때 정하기로 했다.
  - 따로 두는 이유:
    - 분량 프리셋은 연재 내내 쓰는 규칙이다. 제출 기준은 공모전 하나에 한 번 쓴다.
    - 프리셋에 화 수와 전체 분량까지 실으면 기본 프리셋 이름 원칙(사이트 이름이 아니라 길이 이름)과 맞지 않는다.
    - 공모전마다 숫자가 달라서 저장해 둔 프리셋을 다시 쓸 일이 드물다.
  - 그래서 작품마다 네 칸만 둔다. 공모전에 맞춘 묶음 프리셋은 Later로 미룬다.
- **점검은 막지 않는다.** Phase 1·2와 같다. 저장, 쓰기, 공개 어느 것도 점검 결과로 거절하지 않는다.
- **점검은 엔진이 계산한다.** 에이전트와 앱이 같은 답을 보도록 `episode` 패키지와 `charcount.JudgeEpisode`를 쓴다. 데스크톱은 결과를 그리기만 한다.
- **기획서는 다루지 않는다.** 앱 안에 기획서 필드가 없다. 체크리스트에 손으로 체크하는 항목을 만들면 작가가 저장 위치를 착각한다.

## Goal

- 작품 설정에 "제출 기준"을 둔다. 네 칸이고, 모두 비워 둘 수 있다.
  - 최소 화 수
  - 작품 전체 최소 분량
  - 작품 전체 최대 분량
  - 시놉시스 필요
- 기준이 하나라도 있으면 작업 공간 오른쪽 패널에 점검 목록이 보인다.
  - `화 수: 5 / 7화`
  - `회차 분량: 기준 충족 3화 · 미달 2화(2화 −1,075자, 4화 −620자)`
  - `작품 분량: 18,420 / 최소 30,000자(공백 포함)`
  - `시놉시스: 있음` 또는 `없음`
- Works 공개 창에서 지금 회차가 작품의 최소 분량에 못 미치면 `최소 5,000자에 1,075자 모자랍니다` 한 줄을 보인다. 공개는 그대로 된다.

## Non-goals

- 공모전 요강을 붙여 넣으면 숫자를 뽑아 주는 기능
- 마감일, 접수처, 제출 파일 형식 점검
- 원고지 매수 기준(Phase 4)
- 에이전트용 점검 도구(`linetta_check_submission`). 도구 스키마 예산이 빠듯하다(29,489 / 29,500B). 에이전트에게 쓸모가 확인되면 따로 계획한다
- 제출 기준 묶음 프리셋(위 결정 참고)

## Touch points

- `engine/internal/store/migrations/0049_project_submission.sql`(새)
- `engine/internal/project/` (`project.go`, `repo.go`, 테스트)
- `engine/internal/submission/`(새 패키지: 점검 계산)
- `engine/internal/rpc/handlers/projects.go`(`projects.submission_check`), `engine/internal/engineapp/engineapp.go`(등록)
- `engine/internal/mdmeta/metadata.go`, `engine/internal/export/project.go`, `engine/internal/importmd/extras.go`(왕복)
- `apps/desktop/src/lib/types.ts`, `lib/rpc.ts`, `lib/i18n.tsx`
- `apps/desktop/src/components/SubmissionCheck.tsx`(새), `ContextPanel.tsx`, `PublishDialog.tsx`, `App.css`

## Tasks

- [ ] **3.1 제출 기준 모델**
  - 마이그레이션: `projects`에 `submission_min_episodes`, `submission_total_min`, `submission_total_max`(모두 INTEGER NOT NULL DEFAULT 0), `submission_needs_synopsis`(INTEGER NOT NULL DEFAULT 0)
  - `Project`와 `UpdateInput`에 네 필드. 0과 false는 "기준 없음"
  - 검사: 음수 금지, 둘 다 있을 때 `total_min ≤ total_max`. 어기면 `ErrInvalidSubmissionRule`이고 RPC는 invalid input
  - 기존 작품과 새 작품 모두 기준 없음으로 시작한다(공모전에 낼 작품만 작가가 정한다)
  - 마크다운 메타데이터 왕복: `submission_min_episodes` 등 네 키(omitempty). 잘못된 값은 Phase 1의 `episode_char_range`처럼 경고와 함께 건너뛴다
  - 테스트: 기본값, 갱신, 잘못된 범위 거절, 내보내기·가져오기 왕복

- [ ] **3.2 점검 계산 (`submission` 패키지)**
  - `Check(nodes []node.Node, p project.Project) Report`. 순수 함수이고, RPC는 노드를 읽어 넘긴다
  - `Report`
    - `Episodes`: 회차 수. `episode.FromNodes`에서 본문이 있는 회차만 센다. 빈 "3화" 컨테이너는 아직 쓰지 않은 화다
    - `MinEpisodes`
    - `Meeting`: 회차 최소 분량을 채운 회차 수
    - `Short`: 회차 최소 분량에 못 미친 회차들. `{node_id, label, chars, delta}`, 아웃라인 순서
    - `Long`: 최대를 넘긴 회차들
    - `TotalChars`: 회차 합계의 합. 구조 없는 작품은 작품 전체 글자 수
    - `TotalMin`, `TotalMax`, `TotalState`: `charcount.JudgeEpisode`를 그대로 쓴다
    - `NeedsSynopsis`, `HasSynopsis`
    - `Ready`: 정한 기준을 모두 채웠는지
  - 회차 최소 분량이 없는 작품(min 0)은 `Meeting`과 `Short`가 비어 있고, 목록에서 그 줄을 빼라는 뜻으로 `EpisodeRule: false`를 둔다
  - 구조 없는 작품(단편)은 화 수 줄 없이 전체 분량과 시놉시스만 점검한다
  - 테스트: 화 수 미달·충족, 빈 회차 제외, 미달 회차 목록 순서, 전체 분량 미달·초과, 시놉시스, 단편, 기준 없음 → `Ready` 참

- [ ] **3.3 RPC**
  - `projects.submission_check { id }` → `Report`. 작품이 없으면 기존 not found
  - `engineapp`에 등록하고 RPC 왕복 테스트를 하나 둔다

- [ ] **3.4 작업 공간의 점검 목록**
  - `SubmissionCheck.tsx`: 오른쪽 패널, 회차 분량 입력 아래. 접을 수 있는 "제출 기준" 영역
    - 네 칸 입력. 숫자 칸은 비우면 0이고, 시놉시스는 체크박스. 저장은 `projects.update`, 잘못된 범위는 Phase 1처럼 알리고 되돌린다
    - 기준이 하나라도 있으면 점검 목록. 줄마다 충족(기본색)과 미달(경고색)
    - 미달 회차 이름을 누르면 그 회차로 간다(아웃라인 선택과 같은 동작). 세 개를 넘으면 `외 N화`
  - 갱신 시점: 작품을 열 때, 씬 저장 뒤(통계와 같은 `refreshKey`), 기준을 바꾼 뒤
  - 세 언어 문구
  - 테스트: 기준 없음이면 목록 없음, 줄마다 상태, 미달 회차 클릭, 잘못된 범위 알림

- [ ] **3.5 Works 공개 창의 미달 표시**
  - `PublishDialog`에 작품의 회차 규칙(min, max)을 넘기고, 지금 회차의 `char_count`로 `episodeLength`를 계산한다
  - short면 경고색 한 줄 `최소 5,000자에 1,075자 모자랍니다`, long이면 보조색 한 줄. 공개 단추는 그대로 둔다
  - 테스트: 미달 줄, 초과 줄, 기준 없음이면 줄 없음

---

### ✅ Checkpoint: Phase 3 완료 확인

**자동 확인:**
- [ ] `make test-go`, `make test-mobile-engine`, `make test-desktop`

**수동 확인 (사용자):**
- [ ] 작품에 "최소 7화, 시놉시스 필요"를 넣으면 오른쪽 패널에 `화 수: N / 7화`, 미달 회차 목록, 시놉시스 줄이 보인다
- [ ] 미달 회차를 누르면 그 회차가 열린다
- [ ] 모자란 회차를 Works 공개 창에서 열면 모자란 글자 수가 보이고, 공개는 그대로 된다
- [ ] 제출 기준을 비운 작품은 전과 같다

**이 체크포인트를 통과하면 사용자에게 확인을 받고 로드맵에 완료를 표시한다.**
