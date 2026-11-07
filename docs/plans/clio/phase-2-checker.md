# Phase 2: 검사기 — 작업지시서

_작성일: 2026-11-04_
_속한 로드맵: [Clio 로드맵](./roadmap.md)_
_예상 소요: 3일_
_상태: 구현 완료, 수동 확인 대기_

## 페이즈 목표

초안을 쓰는 동안 Clio가 **모델 호출 없이** 세 가지를 짚는다. 내 글에는 없는 AI 표현, 맥락과 맞지 않거나 한 글 안에서 섞인 존댓말/반말, 내 글의 평소 범위를 벗어난 습관이다. 지적마다 위치와 근거("내 X 글 112편 중 0편")가 붙는다. 기준은 Phase 1의 맥락별 프로필이다.

## 전제 조건

- [ ] Phase 1 Checkpoint 통과, 작가 승인
- [ ] 브랜치: `feat/clio-phase-2`
- [ ] Clio 마이그레이션 다음 번호: `0002`
- [ ] 실측 기준(2026-11-04, `x-posts.json`): 존댓말 비율은 X 글 0.08, X 답글 0.33. 답글도 반말이 더 많다. "답글이면 존댓말" 같은 고정 규칙은 틀린 지적을 많이 낸다
- [ ] 사전 계산(2026-11-04, `eval-voice.py`의 `features()`로 run-12를 셈): 이모지·느낌표·AI 표현만으로 짚히는 A는 12편 중 4편이다. "X 글에서 존댓말이 우세하면 지적"을 더해도 5편이다. 반대로 원문은 3편이 걸린다. X 글 중 존댓말 공지가 9편(8%)이라서다(#0, #11). #9는 인용문이라 걸린다. **표면 규칙은 소리 나는 AI 티만 잡고, 매끈한 AI 글은 못 잡는다.** 목표치는 이 사실에 맞춘다
- [ ] run-12 산출물 `~/Documents/voice-eval/run-12/rows.json`이 있다(저장소 밖)

## 포함 기능

1. 검사 규칙 4종과 세기(강, 중, 약)
2. 초안 저장
3. 편집기, 하이라이트, X 글자 수
4. run-12로 기준치 측정과 임계값 조정

## 이 페이즈에서 하지 않는 것

- 고치는 제안(Phase 3). 이 페이즈의 지적은 무엇이 왜 걸렸는지만 말한다
- 모델을 이용한 판정("AI 같은가?"를 모델에게 묻기). 결정론적 규칙만 쓴다
- 리네타 `style.Rules`(작품별 회피 표현) 편집 UI. Clio는 프로필에서 기준을 얻는다

## 작업 체크리스트

### 작업 그룹 A: 검사 규칙 (`engine/internal/clio/check`)

- [ ] **T2.A.1** — 지적 타입과 진입점
  - 파일: `engine/internal/clio/check/check.go`, `check_test.go`
  - 내용:
    ```go
    type Severity string // "strong" | "medium" | "weak"
    type Finding struct {
        Rule     string   // ai_phrase | ai_pattern | register_mismatch | register_mixed | out_of_range | unfamiliar_ending
        Severity Severity
        Start, End int    // 본문 rune 오프셋, 범위 규칙은 0,0(글 전체)
        Message  string   // 한국어 한 줄
        Evidence string   // "내 X 글 112편 중 0편에 있음" 같은 근거
    }
    func Run(body string, ctx voice.ContextProfile, corpus Corpus) []Finding
    ```
    `Corpus`는 맥락 글에서 AI 표현과 끝맺음이 몇 편에 나오는지 미리 세어 둔 값이다. 결과는 오프셋 순으로 정렬한다

- [ ] **T2.A.2** — AI 표현과 패턴 (강)
  - 파일: `check/aiphrase.go`, `aiphrase_test.go`
  - 내용: `eval-voice.py`의 `AI_TELLS`를 `voice.AITells`로 옮기고 위치를 찾는다. **내 맥락 글에서 나온 비율이 2% 이하일 때만** 짚는다. 작가가 원래 쓰는 표현은 짚지 않는다. 문장 패턴은 `style.Check([]style.Scene{{Paragraphs: lines}}, style.Rules{})`를 그대로 불러 `parallel_contrast`, `narrator_explains` 위반을 `ai_pattern`으로 바꾼다(오프셋은 줄 번호와 `Phrase`로 다시 찾는다)
  - 테스트: 작가가 0편 쓴 "결론적으로"는 짚고, 작가 글 10%에 있는 표현은 짚지 않는다

- [ ] **T2.A.3** — 존댓말/반말 (중)
  - 파일: `check/register.go`, `register_test.go`
  - 내용: 초안의 문장 조각마다 끝맺음이 존댓말인지 반말인지 판정한다(어미 목록은 `voice`와 같다)
    - `register_mismatch`: 맥락 글 중 초안과 같은 말투가 우세한 글이 2% 이하일 때만 짚는다. 예를 들어 반말 맥락에서 존댓말 글이 2% 이하인데 초안이 존댓말인 경우다. X 글은 존댓말 공지가 8%라서 걸리지 않는다. 작가가 가끔이라도 쓰는 말투는 작가의 선택으로 본다
    - `register_mixed`: 한 초안에 둘 다 있고, 프로필에서 한 글 안에 두 말투가 섞인 글의 비율이 10% 이하일 때. 소수 쪽 조각을 하나씩 짚는다. run-12 C의 "무거워졌다 … 갈아탔어요"가 이 경우다
    - 따옴표 안 조각은 세지 않는다(인용)
  - 테스트: 위의 두 경우, 인용 제외, 맥락 표본이 적어 물러난 경우 근거 문구에 "X 전체 기준"이 들어간다

- [ ] **T2.A.4** — 범위 이탈과 낯선 끝맺음 (약)
  - 파일: `check/range.go`, `range_test.go`
  - 내용: `emoji`, `laugh`, `hashtag`, `exclaim`, `ellipsis`, `lines`, `length`가 맥락의 P10~P90 밖이면 `out_of_range`를 낸다. 끝맺음이 맥락 글에 한 번도 없고, 그런 조각이 초안에 2개 이상이면 `unfamiliar_ending`을 낸다. 둘 다 약한 지적이다
  - 테스트: 이모지 3개(작가 P90=0) → 지적, 줄 수 2(작가 P50=2) → 통과

### 작업 그룹 B: 기준치 측정

- [ ] **T2.B.1** — run-12로 측정하는 CLI
  - 파일: `engine/cmd/clio-check-eval/main.go`, `main_test.go`(합성 rows)
  - 내용: `clio-check-eval <posts.json> <rows.json>`. `posts.json`으로 프로필과 Corpus를 만든다. 이때 rows의 `original` 본문과 같은 글은 빼서 시험 글이 기준에 섞이지 않게 한다. 그다음 `original`, `A`, `B`, `C`마다 강·중 지적이 1개 이상인 글 수와 규칙별 개수를 표로 출력한다. **글 본문은 출력하지 않는다**
  - 맥락: rows에는 kind가 없으니, `original` 본문으로 `posts.json`에서 kind를 찾는다

- [ ] **T2.B.2** — 임계값 조정과 기록
  - 파일: `docs/plans/clio/eval-check.md`(새 파일, 숫자만), 필요하면 `check/*.go`의 상수
  - 내용: 목표는 **원문 12편 중 강·중 지적 1편 이하**, A 12편 중 4편 이상이다. A 4편은 사전 계산의 표면 규칙 수준이다. 못 미치면 T2.A.2와 T2.A.3의 2%, 10%를 조정한다. 조정 전후 표와 이유를 `eval-check.md`에 적는다. 원문이 걸리면 규칙이 작가를 틀렸다고 하는 것이니, A 쪽 점수보다 이쪽을 우선한다
  - 함께 적을 것: 짚히지 않은 A가 왜 AI 같은지 작가가 한 줄씩 적는다(본문은 적지 않고 특징만, 예: "문장이 고르게 매끈함", "결론 문장으로 끝냄"). 이 목록이 다음 규칙 후보다. 한 번에 다 만들지 말고, 두 편 이상에서 반복되는 특징만 다음 규칙으로 올린다

### 작업 그룹 C: 초안과 편집기

- [ ] **T2.C.1** — 초안 저장소
  - 파일: `engine/internal/clio/migrations/0002_drafts.sql`, `engine/internal/clio/draft/draft.go`, `repo.go`, `repo_test.go`
  - 내용: 테이블 `drafts(id, body, platform, kind, content_version INTEGER NOT NULL DEFAULT 1, published_at INTEGER NOT NULL DEFAULT 0, created_at, updated_at)`. `Update`는 `content_version`이 맞을 때만 쓰고 1 올린다. 틀리면 `ErrConflict`(핸들러에서 `CodeContentConflict`)

- [ ] **T2.C.2** — RPC
  - 파일: `engine/internal/clio/handlers/drafts.go`, `check.go`(+ 테스트), `engine/internal/clio/app/app.go`
  - 내용: `drafts.create`, `drafts.get`, `drafts.list`, `drafts.update`, `drafts.delete`, `check.run`(`{body, platform, kind}` → `{findings, context_used}`). 프로필과 Corpus는 앱 안에 캐시하고, `posts.import` 뒤에 무효화한다. Tauri `RENDERER_ENGINE_METHODS`에 정렬해서 넣는다

- [ ] **T2.C.3** — 편집기 화면
  - 파일: `apps/clio/src/routes/Write.tsx`, `Write.css`, `components/HighlightedTextarea.tsx`, `components/FindingList.tsx`, `lib/xLength.ts`(+ 테스트)
  - 내용:
    - 맥락 선택(`X 글`, `X 답글`, `Threads`, `블로그`)
    - `HighlightedTextarea`: textarea 뒤에 같은 글꼴의 미러 div를 두고 지적 구간에 밑줄을 그린다(세기별 색: `--c-danger`, `--c-warn`, `--c-muted`). tiptap은 쓰지 않는다
    - 입력 300ms 뒤 `check.run`을 부르고, 1초 뒤 `drafts.update`로 저장한다. 충돌이 나면 다시 읽고 토스트를 띄운다
    - `FindingList`: 세기순으로 메시지와 근거를 보여 주고, 누르면 그 구간을 선택한다. "이 맥락에서 무시"는 Out of Scope
    - `xLength`: X 가중치 글자 수(한글, 한자, 일본어는 2, 나머지는 1, URL은 23)를 셈해 280을 넘으면 빨간색으로 표시한다
    - 왼쪽에 초안 목록(`drafts.list`), 새 초안 버튼

---

### ✅ Checkpoint: Phase 2 완료 확인

**구현 확인:**
- [ ] `check` 패키지가 `tars`도, 네트워크도 import하지 않는다: `go list -deps ./internal/clio/check | grep -E 'tars|net/http'`가 비어 있다
- [ ] `eval-check.md`에 run-12 표가 있고, 글 본문은 없다

**실행 확인:**
- [ ] `cd engine && CGO_ENABLED=0 go test ./...` 통과, `make test-clio` 통과
- [ ] 아래 명령의 결과에서 original은 12편 중 1편 이하, A는 12편 중 4편 이상이 강·중 지적을 받는다
  ```bash
  cd engine && go run ./cmd/clio-check-eval ~/Documents/voice-eval/x-posts.json ~/Documents/voice-eval/run-12/rows.json
  ```
- [ ] 같은 결과에서 C의 `register_mixed`가 1편 이상(#6의 섞인 말투) 잡힌다

**수동 확인:**
- [ ] X 글 맥락에서 "결론적으로 핵심은 꾸준함입니다 🚀"를 쓰면 AI 표현(강)과 이모지 범위(약)가 표시된다. 존댓말은 걸리지 않는다(작가의 X 글 8%가 존댓말)
- [ ] "써 보니 괜찮았다. 다음엔 다른 것도 해 볼게요."는 말투 섞임(중)으로 뒤 문장이 짚힌다
- [ ] X 답글 맥락에서 "좋네요. 저도 써 봐야겠어요"는 아무것도 걸리지 않는다
- [ ] 300자 한글을 붙여 넣으면 X 글자 수가 280을 넘었다고 빨갛게 표시된다
- [ ] 앱을 다시 켜도 초안이 남아 있다

**이 체크포인트를 통과하면 사용자에게 확인 요청 후 다음으로 진행.**
만약 실패하면: 실패 항목 보고 → 사용자와 함께 원인 파악 → 수정 → 재검증.

## 구현 메모 (2026-11-07)

- `ai_pattern`은 `internal/style`을 import하지 않는다. 이 패키지가 tars와 net/http를 끌어와서 체크포인트의 의존성 조건을 깨기 때문이다. 문장 패턴 코드만 `check/aipattern.go`로 옮겼다.
- 말투 판정은 `voice.Register` 하나로 모았다. 프로필의 `mixed_share`, Corpus, 검사가 모두 이것을 쓴다. 반말은 반말 종결 글자로 끝날 때만 세고, 명사로 끝나는 조각과 목록 항목에는 말투가 없다.
- AI 표현 기준은 1%(`tellRare`), 말투 기준은 2%(`rareShare`)다.
- `rare_habit`(중)을 새로 두었다. 작가가 거의 쓰지 않는 이모지, 느낌표 같은 습관을 짚는다. 글 길이는 길 때만 짚는다.
- 이유와 수치는 [eval-check.md](./eval-check.md)에 있다.
- `check.run`은 저장된 글과 프로필을 앱 안에 캐시하고, `posts.import`가 성공하면 캐시를 비운다. 응답의 `context_used`는 실제로 쓴 맥락(물러난 경우 `x/*` 등)이다.
- 초안이 충돌하면 서버 본문을 다시 읽어 화면을 바꾸고 토스트를 띄운다. 다른 창에서 고친 내용이 이긴다.
