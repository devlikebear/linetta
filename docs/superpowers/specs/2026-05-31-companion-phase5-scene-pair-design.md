# Linetta 컴패니언 — Phase 5: 쓰기 op 마무리 (씬 생성 + 양방향 관계) 설계

> 작성일: 2026-05-31
> 구현 레포: Linetta (engine + FE). 컴패니언 "완성"의 첫 조각 — 쓰기 op 어휘를 씬(노드) 생성과 양방향 관계로 마무리. (둘째 조각 = Phase 6 온디맨드 읽기 루프.)

## 상위 맥락

컴패니언은 `linetta-proposal` 구조화 op를 제안하고 작가가 검토·적용한다(제공자 무관, 루프 없음). Phase 1~4에서 플롯·메모리·엔티티·관계(단방향)를 다뤘다. **Phase 5(본 spec)**: 쓰기 op를 **씬 생성(`create_scene`)** 과 **양방향 관계(`create_relationship`의 `inverse_label`)** 로 마무리해, 컴패니언이 "새 씬을 만들고 거기에 비트를 넣고, 인물 관계를 양방향으로 묶는" 작업까지 한 제안으로 수행한다.

## 확정된 결정

1. 검토 모델 유지(쓰기=작가 승인), 제공자 무관 JSON op, 루프 없음.
2. **씬 생성:** `create_scene {ref?, label, title?, after_node_id?}` → 새 leaf 노드. 기본은 현재 씬 다음 형제. `ref`로 같은 제안 내 신규 씬 forward-참조 → `add_beat.node_ref`로 그 씬에 비트 부착.
3. **양방향 관계:** `create_relationship`에 `inverse_label?` 추가 — 있으면 `relationships.create_pair`(양방향), 없으면 기존 `create_one`(단방향).
4. 문서 구조 변경(씬 생성)은 위험 영역이라 **검토 게이트 유지** + 적용 후 트리/아웃라인 새로고침.

## 배경 / 현황 (조사 결과)

- proposal `Op`(engine `companion/proposal.go`) 현재 필드: Type, Ref, Name, Color, Summary, ThreadID, ThreadRef, NodeID, BeatID, Label, Description, Intensity, Outline, Text, Category, Kind, Role, EntityID, From, FromRef, To, ToRef, Notes. `knownOps`, `validateProposal`(refs는 create_thread+create_entity ref 수집).
- nodes: FE `nodes.createSibling(referenceId, kind, label, title)`, `nodes.createChild(parentId, kind, label, title)` 존재. 엔진 핸들러 `nodes.create_sibling`(params `{reference_id, kind, label, title}`), `nodes.create_child`. `node.Repo.CreateSibling(ctx, referenceID, kind, label, title, now)`.
- relationships: FE `relationships.createOne`/`createPair`(이미 존재)/`listByEntity`. `NewRelationshipPairInput{project_id, from_id, to_id, label, inverse_label, notes?}` 존재. 엔진 `relationships.create_pair` 핸들러 존재.
- FE `applyProposal(ops, projectId, currentNodeId)`에 `refMap`(thread), `entityRefMap`(entity). `ProposalOp`(types.ts), `ProposalCard.opLabel`. prompt.go `buildContext`의 `## 씬` 섹션이 전/현/후 씬 node_id+경로 노출(Phase 2 fix), `buildSystem`에 id-규칙 블록.

## 아키텍처 / 컴포넌트

**Engine `companion/proposal.go`:**
- `Op`에 필드 추가: `AfterNodeID`(json `after_node_id`), `Title`(`title`), `NodeRef`(`node_ref`), `InverseLabel`(`inverse_label`).
- `knownOps`에 `create_scene`.
- refs 수집 루프에 `create_scene` ref도 포함(노드 ref).
  - 단, 노드 ref와 thread/entity ref는 같은 `refs` 맵에 섞어도 무방(검증은 "선언된 ref인가"만 본다). 단순화를 위해 한 맵 공유.
- 검증 케이스:
  - `create_scene`: `label` 필수.
  - `add_beat`: 기존 thread XOR + label 유지에 더해, `node_id`와 `node_ref`는 동시 지정 불가(둘 다 있으면 에러). `node_ref`가 있으면 `refs`에 선언돼 있어야 함.
  - `create_relationship`: 변경 없음(`inverse_label`은 optional).

**Engine `companion/prompt.go`:**
- `buildSystem` op 목록에 `create_scene{ref?,label,title?,after_node_id?}` 추가; `add_beat`에 `node_ref?` 표기; `create_relationship`에 `inverse_label?`(양방향) 표기.
- 안내 한 줄: "새 씬을 만들려면 create_scene(ref 포함) 후 add_beat.node_ref로 그 씬에 비트를 붙이세요. 관계를 양방향으로 만들려면 inverse_label을 주세요(예: 라이벌↔라이벌, 스승↔제자)."

**FE:**
- `lib/types.ts`: `ProposalOpType += "create_scene"`; `ProposalOp += after_node_id?, title?, node_ref?, inverse_label?`.
- `lib/applyProposal.ts`: `nodeRefMap` 추가. 
  - `create_scene` → `nodes.createSibling(op.after_node_id ?? currentNodeId, "leaf", op.label ?? "새 씬", op.title ?? "")`; ref면 nodeRefMap에 새 노드 id. (after_node_id/현재 씬 둘 다 없으면 실패.)
  - `add_beat` node 해소를 `op.node_id ?? (op.node_ref ? nodeRefMap.get(op.node_ref) : undefined) ?? currentNodeId`로 변경.
  - `create_relationship`: `op.inverse_label` 있으면 `relationships.createPair({project_id, from_id, to_id, label, inverse_label, notes})`, 없으면 기존 `createOne`.
- `components/companion/ProposalCard.tsx`: `opLabel`에 `create_scene: 씬 생성: {label}`.

## 적용 매핑 요약

```
create_scene   → nodes.createSibling(after_node_id ?? currentNodeId, "leaf", label, title); ref→nodeRefMap
add_beat       → node = node_id ?? nodeRefMap[node_ref] ?? currentNodeId; beats.create({thread, node, ...})
create_relationship(inverse_label?) → inverse_label ? relationships.createPair(...) : createOne(...)
```

## 데이터 흐름 (예)

```
"새 씬 '재회'를 만들고 거기에 두 비트를 넣어줘"
→ [{op:create_scene, ref:s1, label:재회},
   {op:add_beat, thread_id:<기존>, node_ref:s1, label:첫 만남},
   {op:add_beat, thread_id:<기존>, node_ref:s1, label:오해}]
→ 적용: nodes.createSibling(현재 씬, leaf, "재회")→id N; nodeRefMap[s1]=N;
        beats.create({thread, node:N, ...}) ×2 → onApplied로 트리/플롯 새로고침
```

## 에러 처리

- create_scene: after_node_id/현재 씬 모두 없음 → 해당 op 실패(부분 적용). 미해소 node_ref → add_beat 실패 기록.
- add_beat: node_id+node_ref 동시 → 엔진 검증 실패(제안 무효 표시).
- 적용 후 `onApplied`로 트리/아웃라인/플롯 패널 새로고침(기존 콜백 + 필요 시 트리 재로드). 씬은 구조 변경이므로 트리 갱신이 보이도록.
- 무효 제안 사유 카드 표시(기존).

## 테스트 전략

Engine: proposal_test — create_scene(label 필수), add_beat node_id+node_ref 동시 거부 + dangling node_ref 거부, create_relationship inverse_label 통과. 기존 op 회귀 없음. prompt_test — buildSystem에 create_scene/node_ref/inverse_label 안내.
FE: `tsc --noEmit` 클린.
검증: `go test ./...` + 엔진 빌드 + tsc.

## 성공 기준

1. 컴패니언이 create_scene + node_ref add_beat + 양방향 관계를 제안하고, 적용 시 새 씬·비트·양방향 관계가 생성된다.
2. add_beat가 node_id/node_ref/현재 씬 순으로 노드를 해소한다.
3. `go test ./...` 통과, 엔진 빌드 ok, `tsc --noEmit` 클린, 기존 op 무회귀.

## 범위 밖
- 온디맨드 읽기 루프 — Phase 6.
- 씬 삭제/이동/이름변경 op, 컨테이너(부/장) 생성 — 후속.
