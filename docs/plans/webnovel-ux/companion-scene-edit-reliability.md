# 컴패니언 씬 본문 적용 신뢰성 — 개선 기획서

_작성일: 2026-06-14_
_속한 로드맵: [`roadmap.md`](./roadmap.md), [`companion-writing-actions.md`](./companion-writing-actions.md)의 후속 안정화_
_예상 소요: 2~3일_
_구현 상태: 2026-06-14 구현 완료, v0.4.17 릴리즈 대상_

## Overview

컴패니언에게 "현재 씬을 써줘", "이 씬을 고쳐줘", "다음 씬 작성하자"라고 요청했을 때 사용자가 기대하는 결과는
대화가 아니라 **현재 보고 있는 씬 본문이 실제로 바뀌는 것**이다. 지금 구현은 `set_scene_text`와
`linetta_apply_ops`를 갖추고 있지만, 적용 여부를 모델의 협조에 많이 맡기고 있어 모델별로 다음 문제가 반복될 수 있다.

- 충분한 맥락이 있는데도 질문만 되돌려 보내며 티키타카를 만든다.
- "적용했습니다"라고 말하지만 실제 씬 본문이 비어 있거나 바뀌지 않는다.
- 현재 씬 본문 요청인데 플롯 비트, 기억, 아웃라인 요약만 저장하고 원고는 쓰지 않는다.
- 한번 실패하면 사용자가 같은 요청을 다시 써야 한다.

이 슬라이스의 목표는 프롬프트를 더 세게 쓰는 것이 아니라, **씬 본문 작성/편집 요청을 앱 레벨의 트랜잭션으로 다루는 것**이다.

## 현재 상태 분석

- `apps/desktop/src/components/companion/CompanionPanel.tsx`
  - 전송 직전 `beforeSend`를 호출해 현재 에디터 문서를 저장한다.
  - 선택 영역 퇴고/재작성 프롬프트는 "설명으로 끝내지 말고 `set_scene_text`로 반영"을 명시한다.
  - 일반 입력은 `companion.send`로 텍스트만 보내며, "이 요청은 씬 본문 변경이다"라는 구조화된 intent를 보내지 않는다.

- `apps/desktop/src/hooks/useCompanion.ts`
  - `companion-applied` 이벤트가 오면 `onApplied`를 호출한다.
  - `companion-done`은 모델의 최종 문장을 그대로 assistant 메시지로 추가한다.
  - 따라서 실제 적용 이벤트가 없거나 적용 검증이 약해도, 모델이 "적용했습니다"라고 말하면 사용자에게 성공처럼 보일 수 있다.

- `apps/desktop/src/routes/Workspace.tsx`
  - `onApplied`에서 현재 노드를 기준으로 트리와 언급을 새로고침한다.
  - 적용 이벤트 payload에 실제 대상 node/content_version 정보가 없어서, 어느 씬이 어떻게 바뀌었는지 프론트가 검증하거나 표시하기 어렵다.

- `engine/internal/companion/prompt.go`
  - 시스템 프롬프트는 현재 씬 본문 변경에 `set_scene_text`를 쓰라고 이미 안내한다.
  - 하지만 프롬프트는 권고일 뿐이고, 모델이 질문으로 빠지거나 잘못된 op를 쓰는 것을 완전히 막지 못한다.

- `engine/internal/companion/runner.go`
  - `companionForcedToolForUserText()`가 키워드 기반으로 직접 변경 요청을 감지해 첫 턴 tool choice를 강제한다.
  - 실패 시 한 번 `directApplyCorrectionPrompt()`로 보정한다.
  - 현재 감지는 범용 mutation 기준이라 "씬 본문 변경"만의 성공 조건을 갖지 않는다.

- `engine/internal/companion/tools.go`
  - `validateApplyOpsIntent()`는 아웃라인 요청이 플롯 비트만 저장되는 문제는 막는다.
  - 하지만 씬 본문 요청에 대해 `set_scene_text` 필수, 대상 노드 필수, 비어 있지 않은 본문 필수 같은 정책은 없다.
  - `ApplyOpsResult`는 applied count와 failures만 반환하며, 변경된 노드와 content_version, 적용 후 글자수, readback 결과를 담지 않는다.

- `engine/internal/companion/proposal.go`
  - `set_scene_text` 자체는 빈 문자열을 기본 거부한다.
  - 하지만 "이번 요청은 반드시 `set_scene_text`가 있어야 한다"는 요청 단위 검증은 없다.

## 핵심 문제 정의

현재 구조는 세 가지 응답 모드를 한 통로에서 섞어 처리한다.

1. 상담/브레인스토밍: 대답만 하면 된다.
2. 제안 카드: 사용자가 승인해야 적용된다.
3. 직접 적용: tool을 호출해 실제 프로젝트 상태가 바뀌어야 한다.

문제는 사용자의 "작성해줘/고쳐줘/반영해줘"가 3번이어야 할 때도 모델이 1번처럼 답하거나, 2번처럼 제안만 하거나,
3번을 했다고 말만 하는 것이다. 이 판단을 모델에게 맡기면 모델별 편차가 계속 남는다.

## 제품 원칙

- **작성/편집 명령은 대화가 아니라 거래다.**
  - 요청을 받으면 앱은 "적용됨", "승인 대기", "실패", "추가 정보 필요" 중 하나로 끝내야 한다.

- **성공 메시지는 모델이 아니라 앱이 만든다.**
  - 모델의 "적용했습니다" 문장은 신뢰하지 않는다.
  - 실제 `set_scene_text` 적용과 readback 검증이 끝난 뒤 앱이 성공 상태를 표시한다.

- **현재 씬 본문 변경은 별도 intent다.**
  - 플롯 비트 저장, 기억 저장, 아웃라인 생성이 성공해도 씬 본문 요청의 성공으로 보지 않는다.

- **불필요한 질문은 실패로 취급한다.**
  - 현재 씬이 있고 사용자의 명령이 구체적인 경우, 모델이 질문만 되돌리면 자동 보정 또는 실패 카드로 처리한다.

## MVP 범위

### 포함

- 씬 본문 작성/편집 intent 감지
- `set_scene_text` 필수 적용 계약
- 적용 후 readback 검증
- 적용 성공/실패 상태를 프론트에서 명확히 구분
- 실패 시 사용자가 재입력하지 않아도 되는 재시도 버튼
- 테스트와 실제 Tauri 수동 시나리오

## 구현 요약

- 백엔드가 `scene_write` / `scene_rewrite` / `generic_mutation` / `chat` intent를 분류한다.
- RPC send options는 선택적 `intent`를 받을 수 있으며, 명시 intent가 있으면 백엔드 추론보다 우선한다.
- scene intent에서는 `linetta_apply_ops`를 강제하고, `set_scene_text`가 없거나 빈 본문이면 실패시킨다.
- `set_scene_text` 적용 후 node를 다시 읽어 content_version, 글자수, plain text readback을 검증한다.
- 적용 이벤트는 `changed_nodes`를 포함한다.
- scene intent가 끝까지 본문 변경을 만들지 못하면 `companion.error`로 끝나며, 프론트는 마지막 요청을 재시도할 수 있게 보여준다.
- proposal 적용 결과가 0건이면 Workspace refresh를 성공처럼 호출하지 않는다.

### 제외

- 실시간 자동완성/ghost text
- 외부 맞춤법 검사기 연동
- 모델별 프롬프트 프로파일 관리 UI
- 긴 생성의 백그라운드 작업 큐 전면 개편
- diff/merge 에디터 전체 구현

## 설계 제안

### 1. Companion Intent Contract 추가

프론트와 백엔드 사이에 선택적 `intent`를 추가한다.

예시:

```ts
type CompanionIntent =
  | { kind: "scene_write"; target_node_id?: string; apply_policy: "direct" | "proposal" }
  | { kind: "scene_rewrite"; target_node_id?: string; apply_policy: "direct" | "proposal" }
  | { kind: "scene_proofread"; target_node_id?: string; apply_policy: "proposal" }
  | { kind: "outline_mutation"; apply_policy: "direct" | "proposal" }
  | { kind: "chat" };
```

- 액션 프리셋, 선택 영역 퇴고, 향후 "본문에 반영" 버튼은 프론트가 intent를 명시한다.
- 일반 채팅 입력은 백엔드가 규칙 기반으로 intent를 추론한다.
- 현재 입력창에서는 우선 기존 UX를 유지하되, "작성해줘/고쳐줘/반영해줘" 류 문장은 백엔드에서 `scene_write` 또는 `scene_rewrite`로 분류한다.

### 2. Scene Edit Intent Classifier

기존 `companionForcedToolForUserText()`를 확장하거나 별도 `classifyCompanionIntent()`로 분리한다.

감지 기준:

- 대상 단어: `현재 씬`, `씬`, `장면`, `본문`, `원고`, `문장`, `다음 씬`
- 변경 단어: `작성`, `써줘`, `고쳐`, `수정`, `재작성`, `다듬`, `확장`, `이어`, `반영`, `교체`
- 보수적 예외:
  - `어때`, `아이디어`, `추천`, `방법`, `작성법`만 있고 직접 적용 표현이 없으면 `chat`
  - 현재 노드가 없거나 leaf가 아니면 `needs_input`

결과는 tool-choice 강제뿐 아니라 tool 검증과 최종 응답 게이트에도 사용한다.

### 3. `set_scene_text` 필수 검증

`applyOpsIntent`를 다음처럼 확장한다.

```go
type applyOpsIntent struct {
    RequireOutlineTree bool
    RequireSceneText bool
    TargetNodeID string
    RequireNonEmptySceneText bool
}
```

검증 정책:

- `RequireSceneText`면 ops 안에 `set_scene_text`가 반드시 있어야 한다.
- `set_scene_text`가 현재 씬이 아닌 다른 노드를 대상으로 하면 명시적 node_id가 현재 요청과 맞는지 검증한다.
- `allow_empty`는 사용자가 "비워줘/초기화해줘"를 명확히 말한 경우에만 허용한다.
- `remember`, `create_thread`, `add_beat`, `set_outline`만 있는 응답은 실패로 처리한다.

### 4. 적용 후 Readback Verifier

`ApplyOpsResult`에 변경된 씬 정보를 추가한다.

예시:

```go
type AppliedNodeChange struct {
    NodeID string `json:"node_id"`
    Op string `json:"op"`
    ContentVersion int64 `json:"content_version"`
    CharCount int `json:"char_count"`
    TextPreview string `json:"text_preview"`
}
```

`set_scene_text` 후에는 다음을 수행한다.

1. `nodes.UpdateContent()` 실행
2. 같은 node를 다시 조회
3. plain text를 정규화해 op text와 비교
4. content_version 증가와 비어 있지 않은 char_count 확인

readback 실패 시 `Applied`를 성공으로 보지 않는다.

### 5. Runner 응답 게이트

scene edit intent에서는 최종 응답을 다음 규칙으로 통과시킨다.

- `direct` 정책:
  - `set_scene_text` 적용 검증 성공 → 앱 성공 메시지 표시
  - tool 미호출 또는 잘못된 op → 한 번 자동 보정
  - 보정 후에도 실패 → 실패 카드 표시, 재시도 버튼 제공

- `proposal` 정책:
  - 유효한 `set_scene_text` proposal 있음 → 승인 카드 표시
  - proposal 없음 → 한 번 "proposal만 반환" 보정
  - 보정 후에도 없음 → 실패 카드 표시

중요: 모델의 최종 prose가 "적용했습니다"라고 해도, 검증 상태가 없으면 성공으로 표시하지 않는다.

### 6. 프론트 상태 모델 개선

`useCompanion`은 run별로 다음 상태를 추적한다.

- `expectedIntent`
- `appliedChanges`
- `proposal`
- `terminalStatus: "done" | "applied" | "proposal" | "failed" | "needs_input"`

UX:

- 진행 중 상태:
  - `현재 씬 저장 중`
  - `원고 생성 중`
  - `본문 교체 중`
  - `적용 확인 중`
- 성공:
  - `현재 씬 본문을 교체했습니다 · {n}자`
- 실패:
  - `본문 변경이 만들어지지 않았습니다`
  - 버튼: `재시도`
  - 보조: `진단 복사`

`ProposalCard`는 `applyProposal()` 결과가 `applied === 0`이거나 failures가 있으면 성공 refresh를 호출하지 않는다.

## 작업 체크리스트

### 작업 그룹 A: intent 계약과 분류

- [x] **A.1** — RPC send options에 `intent` 추가
  - 파일: `apps/desktop/src/lib/types.ts`, `apps/desktop/src/lib/rpc.ts`, `engine/internal/rpc/handlers/companion.go`
  - 내용:
    - optional field라 기존 호출과 호환 유지
    - `node_id`와 intent target이 충돌하면 백엔드에서 에러 또는 현재 node 우선 정책 결정
  - 검증:
    - RPC marshal/unmarshal 테스트

- [x] **A.2** — 백엔드 intent classifier 추가
  - 파일: 신규 `engine/internal/companion/intent.go`(+`intent_test.go`) 또는 `runner.go`
  - 내용:
    - `scene_write`, `scene_rewrite`, `scene_proofread`, `outline_mutation`, `chat` 분류
    - 기존 `companionForcedToolForUserText()`는 classifier 결과를 사용하도록 단순화
  - 검증:
    - `"아니 1장 1씬 작성해달라고"` → `scene_write`
    - `"현재 씬 본문 써줘"` → `scene_write`
    - `"이 문장 다듬어줘"` → `scene_rewrite`
    - `"씬 작성법 알려줘"` → `chat`
    - `"이 설정 어때?"` → `chat`

### 작업 그룹 B: apply ops 검증 강화

- [x] **B.1** — `applyOpsIntent`에 scene text 필수 조건 추가
  - 파일: `engine/internal/companion/tools.go`
  - 내용:
    - `RequireSceneText`
    - `RequireNonEmptySceneText`
    - `AllowEmptySceneText`
    - `TargetNodeID`
  - 검증:
    - scene intent에서 `remember`만 있는 op는 실패
    - scene intent에서 `create_thread/add_beat`만 있는 op는 실패
    - scene intent에서 빈 `set_scene_text`는 실패
    - 명시적 "비워줘"일 때만 `allow_empty` 허용

- [x] **B.2** — 적용 결과에 changed node 추가
  - 파일: `engine/internal/companion/tools.go`, `apps/desktop/src/lib/types.ts`
  - 내용:
    - `changed_nodes` 배열 추가
    - `companion.applied` 이벤트에도 target node와 content_version 포함
  - 검증:
    - `set_scene_text` 적용 시 changed node payload가 포함된다.

- [x] **B.3** — readback verifier 추가
  - 파일: `engine/internal/companion/tools.go`
  - 내용:
    - `set_scene_text` 적용 후 node 재조회
    - plain text 정규화 비교
    - content_version/char_count 확인
  - 검증:
    - update 실패, readback 실패, empty 결과를 각각 실패로 반환한다.

### 작업 그룹 C: runner 응답 게이트

- [x] **C.1** — scene edit 직접 적용 게이트
  - 파일: `engine/internal/companion/runner.go`
  - 내용:
    - scene edit intent면 첫 턴 `linetta_apply_ops` required 유지
    - tool 미호출, wrong op, empty result면 한 번 보정
    - 보정 후 실패 시 `companion.error` 또는 신규 `companion.failed` 이벤트 발행
  - 검증:
    - 모델이 "적용했습니다"만 말해도 성공으로 끝나지 않는다.
    - 모델이 질문만 반환하면 보정 또는 실패 상태로 끝난다.

- [x] **C.2** — 성공 메시지를 앱 생성으로 전환
  - 파일: `engine/internal/companion/runner.go`, `apps/desktop/src/hooks/useCompanion.ts`
  - 내용:
    - scene edit 적용 성공 시 모델 prose보다 verified result를 우선 표시
    - transcript에는 모델 원문과 앱 상태를 어떻게 저장할지 결정한다.
  - 검증:
    - "적용했습니다" prose만 있는 응답이 성공 카드로 보이지 않는다.

### 작업 그룹 D: 프론트 UX 안정화

- [x] **D.1** — applied event payload 기반 refresh
  - 파일: `apps/desktop/src/hooks/useCompanion.ts`, `apps/desktop/src/routes/Workspace.tsx`
  - 내용:
    - changed node가 현재 열린 노드면 해당 노드를 갱신한다.
    - 현재 노드가 아니면 트리만 갱신하고 알림으로 대상 씬을 표시한다.
  - 검증:
    - 현재 씬 적용 시 에디터가 새 content_version으로 갱신된다.

- [x] **D.2** — 실패 카드와 재시도 버튼
  - 파일: `apps/desktop/src/components/companion/CompanionPanel.tsx`, `CompanionPanel.css`, `i18n.tsx`
  - 내용:
    - "본문 변경이 만들어지지 않았습니다" 상태 표시
    - 마지막 사용자 요청과 intent로 재시도
    - 진단 복사 버튼은 기존 진단 복사 UX와 톤 맞춤
  - 검증:
    - 실패 상태에서 사용자가 같은 문장을 다시 칠 필요가 없다.

- [x] **D.3** — `ProposalCard` 적용 실패 처리
  - 파일: `apps/desktop/src/components/companion/ProposalCard.tsx`
  - 내용:
    - `applied === 0`이면 `onApplied()`를 호출하지 않는다.
    - failures가 있으면 성공처럼 보이는 문구를 피한다.
  - 검증:
    - 실패 결과에서 에디터 refresh가 발생하지 않는다.

### 작업 그룹 E: 검증

- [x] **E.1** — Go 테스트
  - 명령:
    - `go test ./engine/internal/companion ./engine/internal/rpc/handlers`
  - 케이스:
    - scene write intent 강제 tool-choice
    - claim-only 모델 응답 보정
    - question-only 모델 응답 실패 처리
    - wrong-op 응답 거부
    - readback verifier

- [x] **E.2** — 프론트 테스트
  - 명령:
    - `cd apps/desktop && pnpm test -- CompanionPanel.test.tsx useCompanion.events.test.tsx ProposalCard.test.tsx --run`
  - 케이스:
    - applied event payload 수신
    - success/failure terminal status
    - proposal 실패 적용 처리

- [x] **E.3** — 전체 게이트와 수동 검증
  - 명령:
    - `make test`
    - `git diff --check`
    - `make tauri-dev`
  - 수동 시나리오:
    - 빈 씬에서 `아니 1장 1씬 작성해달라고` 전송 → 질문 없이 본문 생성/적용/검증 표시
    - 기존 본문 있는 씬에서 `현재 씬을 감정선 중심으로 다듬어줘` → 본문 변경 확인
    - 모델이 proposal만 만들도록 유도되는 요청 → 승인 전 본문 불변, 승인 후 반영
    - 일부러 실패 mock provider로 "적용했습니다"만 응답 → 성공으로 표시되지 않음

## 완료 조건

- [x] 씬 본문 작성/편집 요청은 검증된 적용 또는 실패 상태로 끝난다.
- [x] 직접 적용 성공은 readback 검증을 통과해야만 성공으로 표시된다.
- [x] 모델이 말로만 "적용했다"고 한 경우 성공으로 표시되지 않는다.
- [x] 현재 씬 요청에서 `remember/create_thread/add_beat/set_outline`만 실행된 경우 실패 처리된다.
- [x] 실패 시 사용자는 같은 요청을 다시 타이핑하지 않고 재시도할 수 있다.
- [x] `make test`와 실제 Tauri 앱 smoke 시나리오가 통과한다.

## 구현 순서

1. `git status --short --branch`로 시작 상태를 확인한다.
2. A/B를 먼저 구현해 "씬 본문 intent와 apply 검증"을 만든다.
3. C로 runner의 최종 응답 게이트를 추가한다.
4. D로 프론트 성공/실패 상태를 정리한다.
5. E의 테스트를 추가하고 `make test`까지 통과시킨다.
6. 실제 Tauri 앱에서 빈 씬 작성, 기존 씬 다듬기, 실패 mock 시나리오를 확인한다.

## 리스크와 대응

- **긴 원고를 direct apply로 바로 교체하는 부담**
  - 단기: 명확한 작성/편집 명령만 direct, 퇴고/대규모 재작성은 proposal 정책을 기본으로 둔다.
  - 후속: diff preview를 추가해 사용자가 바뀐 부분만 승인하게 한다.

- **분류기가 과하게 적용하는 문제**
  - 직접 적용 표현이 없는 상담/아이디어/작성법 요청은 `chat`으로 유지한다.
  - 사용자가 "본문에 반영" 같은 명령을 명확히 쓸 때만 direct로 보낸다.

- **모델이 tool schema를 어기는 문제**
  - schema, intent validation, corrective turn, terminal failure card 순으로 방어한다.

- **화면 전환/패널 닫힘 중 진행 상태 유실**
  - 이 계획은 run 상태를 store에 유지하는 기존 구조를 활용한다.
  - 별도 백그라운드 job queue는 Out of Scope지만, applied/failed terminal event는 패널 재개 시 복원 가능하게 설계한다.
