# InkOS 소스 분석 — 리네타가 가져올 네 가지 아이디어

_작성일: 2026-10-24_
_대상: [Narcooo/inkos](https://github.com/Narcooo/inkos) main `56aa33b` (2026-10-19), AGPL-3.0_
_관련: [`roadmap.md`](./roadmap.md)_

이 문서는 로드맵의 근거 자료다. InkOS 저장소를 읽기만 했다. 실행·설치·빌드는 하지 않았다.
경로는 모두 InkOS 저장소 기준이다. 코드를 옮겨 오지 않는다. 설계만 참고한다.
InkOS와 리네타는 둘 다 AGPL-3.0이지만, 구조와 언어가 달라서 그대로 옮길 코드도 없다.

## 먼저 알아둘 것: README와 코드가 다르다

InkOS README(v2.0 설명)의 일부는 현재 코드와 맞지 않는다.
계획은 README가 아니라 코드를 기준으로 세운다.

| README 설명 | 실제 코드 |
|---|---|
| Observer가 9종 사실 추출, Reflector가 JSON delta 출력 | `observer.ts`·`reflector.ts`는 없다. **Settler** 호출 한 번(`agents/writer.ts` `settle()`)이 둘 다 한다. 9종 분류는 Skill 문서(`skills/inkos-long-writing/references/state-projection.md`)의 산문 지시로만 남았다 |
| Planner가 must-keep / must-avoid 산출 | 스키마는 `{chapter, goal}`과 memo `{goal, body(자유 Markdown), threadRefs}`뿐이다. `mustKeep: []`가 흔적으로만 남아 있다 |
| 갈래 예측은 2~5개 | 2~5개 범위는 도구 설명 문구에만 있다. 코드는 양의 정수면 다 받는다 |
| 최신 버전 2.0 | GitHub 최신 릴리스와 npm 버전은 1.8.0(2026-08-17)이다. 2.0은 main 브랜치에만 있다 |

---

## 1. 연속성 점검 (Settler · Continuity Auditor · 관찰 기록)

### 구성 요소

- **Settler**
  - 파일: `agents/writer.ts:271` `settle()`, `agents/settler-tool.ts` `SettlementToolSchema`
  - 하는 일: 챕터 본문과 기준 상태(JSON)를 받아 상태 **변경분(delta)만** 낸다.
- **Hook arbiter**
  - 파일: `utils/hook-arbiter.ts`
  - 하는 일: 떡밥(hook) id를 검증한다. 새 떡밥은 id 없는 후보로만 받고, id는 호스트가 붙인다.
- **Reducer**
  - 파일: `state/state-reducer.ts`
  - 하는 일: 결정적(deterministic) 코드로 변경분을 병합한다. 모델은 전체 상태를 다시 쓰지 못한다.
- **State validator**
  - 코드 검증 `state/state-validator.ts` 다음에 LLM 검증 `agents/state-validator.ts`를 거친다. LLM 검증은 temperature 0.1이다.
- **Continuity Auditor**
  - 파일: `agents/continuity.ts` `auditChapter`, `models/observation.ts`
  - 하는 일: 원고를 점검하고 관찰(observation)을 낸다. 점수를 매기거나, 원고를 거부하거나, 다시 쓰지 않는다.

### 데이터 모델

```ts
// 사실: 유효 기간이 있는 subject/predicate/object 삼중항
CurrentStateFactSchema = { subject, predicate, object,
  validFromChapter, validUntilChapter /* null = 유효 */, sourceChapter }

// 떡밥 장부
HookStatusSchema = "open" | "progressing" | "deferred" | "resolved" | "superseded"

// 관찰. 모델이 보내는 것은 {code, summary, assessment, category?, sourceRefs:[{sourceId,startLine,endLine}]}
ObservationSchema = { code, summary, evidence[],
  category?: "execution" | "quality" | "scope",
  assessment?: "issue" | "resolved" | "unavailable" | "observation",
  sourceRefs?: [{ sourceId, quote }] }
```

### 핵심 메커니즘: 근거 인용을 기계적으로 검증한다

1. 호스트가 원문에 줄 번호를 붙여 보낸다(`"<n>\t<line>"`).
2. 모델은 `{sourceId, startLine, endLine}`만 고른다. 인용문을 직접 쓰지 않는다.
3. 호스트가 그 범위의 원문을 복사해 `quote`로 만든다. 그다음 다음을 검사한다.
   - 범위가 원문 안에 있는가 (`REVIEW_SOURCE_RANGE_INVALID`)
   - 고른 줄이 공백만이 아닌가 (`REVIEW_SOURCE_REQUIRED`)
   - 인용문이 원문의 부분 문자열인가 (`REVIEW_SOURCE_MISMATCH`)
4. 오류는 도구 오류로 모델에게 돌려보낸다. 모델은 최대 3턴 안에 근거를 고치거나 관찰을 철회해야 한다.
5. 끝까지 실패하면 판정을 내리지 않고 "점검 불가(unavailable)" 관찰을 남긴다.

### 리네타에 맞는 교훈

- **모델은 연산(op)을 제안하고, 병합은 코드가 한다.** 그래서 상관없는 사실이 조용히 사라지는 일이 없다.
- **id는 호스트가 소유한다.** 모델은 존재하는 id만 참조할 수 있다. 새 항목은 id 없는 후보로 받는다.
- **인용은 모델이 쓰지 않고 호스트가 원문에서 잘라낸다.** 모델이 문장을 지어내거나 바꿔 말할 수 없다.
- **"추출 실수"와 "원고 자체의 모순"을 구분한다.** 작가에게는 원고 자체의 모순만 보고한다.
- **InkOS의 약점**
  - 사실 매칭이 정확한 문자열 비교다. "Lin Feng"과 "林枫"이 서로 다른 사실이 된다.
  - 심각도(severity)와 수정 방향이 스키마에 없다.
  - Studio 화면이 검증된 인용문(`sourceRefs`)을 표시하지 않는다.
  - 리네타는 entity id와 별칭으로 이 문제를 피할 수 있다.

## 2. 의도·초점 문서 (input governance)

### 구성 요소

- `story/author_intent.md`, `story/current_focus.md`
  - 기본 템플릿: `state/manager.ts:59-68`
  - 책을 만들 때 brief를 그대로 author_intent에 복사한다.
- Planner
  - 파일: `agents/planner.ts`, 도구 `submit_chapter_memo`
  - 산출물: `runtime/chapter-XXXX.intent.md`(읽기용)와 `.plan.json`(원본)
- Composer
  - 파일: `agents/composer.ts`
  - 하는 일: 맥락을 protected / compressible 두 단계로 나누고, `context.json`·`trace.json`을 남긴다.
  - protected 단계가 예산을 넘으면 압축하지 않고 실패한다(`composer.ts:202`).

### 리네타에 맞는 교훈

- **배치가 이름보다 중요하다.** 방향 문서를 일반 근거 목록 안에 묻어 두면 모델이 선택 사항으로 취급했다.
  그래서 Writer 프롬프트 맨 위 "User direction (overrides model defaults — must follow)" 블록으로 옮겼다(`writer.ts:463-466` 주석).
- **템플릿 문구가 그대로 주입된다.** 손대지 않은 템플릿도 빈 문자열이 아니어서 "반드시 따를 방향"으로 들어간다.
  비어 있거나 템플릿 그대로면 아예 넣지 말아야 한다.
- **초점 문서는 낡아도 표시가 없다.** current_focus에는 회전이나 낡음 표시가 없다.
  캐시된 plan.json은 초점이 바뀌어도 다시 쓰인다(`runner.ts:2168-2175`).
- **구조화된 keep/avoid가 자유 Markdown으로 무너졌다.** 이후로는 "피할 것"을 지켰는지 아무도 확인할 수 없다.
  리네타는 구조화된 필드로 되살릴 가치가 있다.
- **에이전트가 작가 소유 문서를 통째로 덮어쓸 수 있다**(`write_truth_file`). 리네타는 이 길을 열지 않는다.

## 3. 갈래 예측 (narrative forecast)

### 구성 요소

`forecast/` 아래 약 1,000줄이다.

- `schema.ts`, `context-builder.ts`, `agent.ts`, `runner.ts`, `store.ts`, `render.ts`
- 채팅 도구 `create_narrative_forecast`, `get_narrative_forecast`, `select_narrative_branch`
- Studio 화면 `NarrativeForecastPreview.tsx`

### 데이터 모델

```ts
ForecastBranchSchema = { branchId /* 러너가 branch-1..N 부여 */, title, premise,
  beats: [{ chapter, summary }], characterDecisions: [{ character, decision }],
  projectedChanges: { characters[], relationships[], world[], hooks[] },
  risks: [{ kind /* continuity|causality|character… */, description }],
  uncertainties[], intentRationale }
NarrativeForecastSchema = { forecastId, baseChapter, horizon, divergence,
  contextFingerprint, status: "active" | "stale", branches[] }
```

### 흐름

1. **맥락 수집.** 읽기 전용이다.
   - 정사 상태가 챕터 수와 어긋나면 예측을 거부한다.
2. **생성.** LLM 호출 한 번으로 모든 갈래를 강제 도구 호출로 받는다.
   - temperature 0.6. 검증에 실패하면 오류를 붙여 0.4로 한 번 재시도한다.
   - 갈래끼리 기술적으로 격리되지는 않는다. 프롬프트가 "서로 독립"이라고 말할 뿐이다.
3. **낡음 판정.** 입력 파일 내용의 SHA-256으로 판정한다. mtime을 쓰지 않고 순서와 무관하다.
   - **챕터 본문은 해시하지 않는다.** 본문만 고치면 예측이 낡았다고 표시되지 않는다.
   - 판정은 조회와 선택 때만 한다. 한 번 낡으면 되돌릴 수 없다.
4. **선택.** `selected-branch-plan.md`만 쓴다.
   - **이 계획을 읽는 코드가 하나도 없다.** 막다른 산출물이다.
   - Studio의 "이 갈래 선택" 버튼은 API를 부르지 않고 채팅 메시지를 보낸다. 선택 한 번에 LLM 한 턴을 쓴다.
5. **정사 보호 테스트.** 생성·조회·선택 전후에 책 디렉터리 전체를 스냅샷으로 비교한다(`forecast-fixture.ts`).

### 리네타가 더 잘할 수 있는 점

- **본문 버전도 지문에 넣는다.** 리네타는 작가가 본문을 직접 쓰므로 `content_version`을 넣어야 한다.
- **입력 묶음별 해시를 저장한다.** 그러면 "인물 2명, 3화 개요가 바뀜"처럼 왜 낡았는지 말할 수 있다.
- **선택은 RPC로 직접 처리한다.** LLM을 거치지 않는다.
- **선택한 갈래를 실제로 쓰이게 만든다.** 선택한 갈래를 브리프에 넣고, 개요 반영은 제안으로 만든다.
- **범위를 코드에서 강제한다.** 갈래 2~5개, 회차 1~10개.

## 4. 원고 가져오기 후 상태 재구성

### 구성 요소

- 회차 분할
  - `utils/chapter-splitter.ts`. 第N章 / 第N回 / Chapter N / 로마 숫자, 한자 숫자 파싱을 지원한다.
- 원고 불러오기
  - `agent/chapter-import-source.ts`
- 오케스트레이션
  - `pipeline/runner.ts` `importChapters()`(l.1662)
- 1단계 기반 생성
  - 원고 전체에서 세계관, 등장인물, 초기 떡밥을 만든다.
  - `agents/architect.ts` `generateFoundationFromImport()`. 구조화 호출 4번.
- 2단계 회차 재생
  - 회차마다 Settler를 한 번씩 **순차로** 부른다. 병렬 처리는 없다.

### 관찰한 점

- **추출 결과를 검토 없이 바로 적용한다.** 안전망은 "되돌릴 수 있음"(리비전 이력)이지 "검토함"이 아니다. 확인 절차가 없다.
- **이어 하기는 수동이다.** 회차별 원자적 커밋 자체가 체크포인트 역할을 한다.
  실패하면 `recovery: { resumeFrom }` 힌트를 돌려준다.
- **비용 장치가 없다.** 비용 미리보기도 상한도 없다.
  모델 맥락 창 크기를 모르면 원고를 나누지 않고 통째로 보낸다.
- **entity 표가 없고 별칭도 풀지 않는다.** 인물 카드는 1단계에서 한 번 만들 뿐 재생 중에는 갱신하지 않는다.
- **첫 제목 앞의 글이 버려진다.** 프롤로그와 서문이 조용히 사라지고, 회차 번호 누락도 감지하지 않는다.

### 리네타에 맞는 교훈

- **추출 결과는 제안함에 쌓는다.** 작가가 항목별로 수락·수정·거절하게 한다.
- **이름 후보는 기존 entity 이름과 별칭에 맞춰 본다.** NFKC 정규화, 대소문자와 공백 무시를 거쳐 비교하고,
  "같은 인물일 수 있음"을 검토 항목으로 띄운다.
- **AI 추출은 별도의 동의 단계로 둔다.** 리네타의 Markdown 가져오기는 이미 회차와 씬을 만든다.
  추출은 그 위에 얹는 선택 단계로 두고, 시작 전에 분량과 예상 비용을 보여준다.
