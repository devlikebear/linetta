# Linetta 컴패니언 — Phase 4: 확장 액션 (엔티티·관계 op) 설계

> 작성일: 2026-05-31
> 구현 레포: **Linetta** (engine + FE). 컴패니언의 제안 op 어휘를 캐릭터/장소(엔티티)·관계 생성·수정으로 확장.

## 상위 맥락

컴패니언(집필 동료)은 `linetta-proposal` 구조화 op를 제안하고 작가가 카드로 검토·적용한다(쓰기 = 사람 게이트). 이 op 집합이 사실상 "AI가 호출하는 제공자-무관 빌트인 툴 레지스트리"다. Phase 1~3에서 플롯(thread/beat/outline)·메모리를 다뤘다. **Phase 4(본 spec)**: 그 어휘를 **엔티티(캐릭터·장소·물건·개념)·관계**로 넓혀, AI가 "캐릭터도 생성하고 캐스트 관계도 구성하는" 등 더 많은 작업을 같은 검토 모델로 수행한다.

## 확정된 결정 (브레인스토밍)

1. **자율성:** 읽기 자율 / 쓰기 검토. 본 Phase는 **쓰기 op 확장**에 집중. (AI 능동 조회 루프는 후속.)
2. **메커니즘:** 제공자 무관 JSON op(루프 없음). `claude-code-cli`가 네이티브 tool-calling 불가하므로 네이티브 함수호출/agent loop는 쓰지 않는다.
3. **범위:** 엔티티(create/update) + 관계(create_one, 단방향). 씬(노드) 생성·문서 구조 변경 제외(후속). 양방향 관계 pair 제외(후속). 온디맨드 읽기 루프 제외(후속).

## 배경 / 현황 (조사 결과)

- proposal `Op`(engine `companion/proposal.go`) 현재 필드: Type, Ref, Name, Color, Summary, ThreadID, ThreadRef, NodeID, BeatID, Label, Description, Intensity, Outline, Text, Category. `knownOps`, `validateProposal` switch, `ParseProposal`.
- entity repo / FE: `entities.create(NewEntityInput{project_id, kind, name, role?}) → Entity`, `entities.update(UpdateEntityInput{id, kind?, name?, role?, summary?, attributes?}) → Entity` (FE rpc `entities` 네임스페이스 존재). `Entity{ID, Kind, Name, Role, Summary, ...}`. kind ∈ character|place|item|concept. (NewEntityInput에 summary 없음 → 생성 후 update로 보정.)
- relationship: 엔진 핸들러 `relationships.create_one`(params `{project_id, from_id, to_id, label, notes}`)·create_pair·update·delete 존재. **FE rpc에 `relationships` 네임스페이스 없음** → 신설 필요. `relationship.Relationship{ID, FromID, ToID, Label, Notes, PairID}`.
- FE `applyProposal(ops, projectId, currentNodeId)`에 thread refMap 존재. `ProposalOp`(types.ts), `ProposalCard.opLabel`(컴포넌트).
- prompt.go `buildContext`의 `## 등장 인물·장소·관계` 섹션은 엔티티를 `[id] name / role: summary`로(최대 40), 관계는 등장 엔티티 쌍만 렌더.

## 아키텍처 / 컴포넌트

**Engine:**
- `companion/proposal.go` — `Op`에 필드 추가: `Kind`(json `kind`), `Role`(`role`), `EntityID`(`entity_id`), `From`(`from`), `FromRef`(`from_ref`), `To`(`to`), `ToRef`(`to_ref`), `Notes`(`notes`). `knownOps`에 `create_entity`/`update_entity`/`create_relationship`. `validateProposal`에 케이스(아래 규칙).
- `companion/prompt.go` — `buildContext`의 엔티티 줄에 **kind 표시** 추가. `buildSystem`의 op 목록에 새 3개 + ref/ id 규칙 안내(이미 "id 지어내지 말 것" 블록 존재 — 엔티티 ref 한 줄 추가) + 예시 보강.

**FE:**
- `lib/types.ts` — `ProposalOpType`에 `"create_entity" | "update_entity" | "create_relationship"`; `ProposalOp`에 `kind?`, `role?`, `entity_id?`, `from?`, `from_ref?`, `to?`, `to_ref?`, `notes?`.
- `lib/rpc.ts` — 신규 `relationships` 네임스페이스: `createOne(input: NewRelationshipInput) → Relationship`(method `relationships.create_one`, params `{project_id, from_id, to_id, label, notes}`). (`entities`는 이미 존재.) `NewRelationshipInput`/`Relationship` 타입은 types.ts에 이미 있음(확인).
- `lib/applyProposal.ts` — `entityRefMap`(두 번째 Map) 추가. case:
  - `create_entity` → `entities.create({project_id, kind, name, role})` → ref면 entityRefMap에 id; summary면 `entities.update({id, summary})`.
  - `update_entity` → `entities.update({id: entity_id, name?, role?, summary?, kind?})`.
  - `create_relationship` → `fromId = from ?? entityRefMap[from_ref]`, `toId = to ?? entityRefMap[to_ref]`(둘 다 해소 안 되면 실패) → `relationships.createOne({project_id: projectId, from_id, to_id, label, notes})`.
- `components/companion/ProposalCard.tsx` — `opLabel`에 새 3개 라벨(`캐릭터 생성: {name}`, `캐릭터 수정`, `관계 생성: {label}`).

## 제안 op 검증 규칙 (validateProposal)

```
create_entity:        name 필수; kind 필수이며 character|place|item|concept 중 하나(아니면 검증 실패)
update_entity:        entity_id 필수
create_relationship:  label 필수; (from XOR from_ref) 그리고 (to XOR to_ref); *_ref는 같은 제안의 create_entity.ref와 일치해야 함(dangling → 검증 실패)
```
(엔진은 검증만; 실제 id 해소·RPC 호출은 FE applyProposal.)

## 데이터 흐름 (예)

```
"주인공 '하나'와 라이벌 '도윤'을 만들고 둘을 라이벌로 묶어줘"
→ AI proposal:
  [{op:create_entity, ref:e1, kind:character, name:하나, role:주인공},
   {op:create_entity, ref:e2, kind:character, name:도윤, role:라이벌},
   {op:create_relationship, from_ref:e1, to_ref:e2, label:라이벌}]
→ 카드 검토 → 적용:
  entities.create(하나)→id A; entities.create(도윤)→id B;
  relationships.create_one({from_id:A, to_id:B, label:라이벌})
→ onApplied → 엔티티/관계 패널 새로고침
```

## 에러 처리

- 미해소 ref(entityRefMap 미존재)·빈 필수 필드 → 해당 op 실패 기록, 다음 op 진행(부분 적용, 기존 패턴). 카드가 op별 실패 사유 표시(Phase 2 fix로 이미 구현).
- create_entity 이름 중복 등 RPC 에러 → 실패 기록. (entities는 UNIQUE(project_id,name) — 중복 시 에러 표면화.)
- 무효 제안(`valid:false`) → 카드에 사유, 적용 비활성.
- 적용 후 `onApplied`로 멘션/엔티티 데이터 새로고침(기존 콜백 재사용; 관계는 EntitySheet 재오픈 시 반영).

## 테스트 전략

Engine(Go TDD):
- `proposal_test.go`: create_entity(name 필수·kind 유효), update_entity(entity_id 필수), create_relationship(label 필수, from/to XOR ref, dangling ref→실패), 기존 plot/remember op 회귀 없음.
- `prompt_test.go`: 엔티티 줄에 kind 렌더, buildSystem에 새 op 안내.

FE(테스트 인프라 없음): `tsc --noEmit` 클린(types/rpc/applyProposal/card). 수동 스모크: "캐릭터 둘 만들고 라이벌로 묶어줘" → 제안 카드 → 적용 → 엔티티 시트/검색에 반영.

검증: `go test ./...` 전 통과 + 엔진 빌드 + `tsc --noEmit`.

## 성공 기준

1. 컴패니언이 create_entity/update_entity/create_relationship를 제안하고, 적용 시 기존 RPC로 엔티티·관계가 생성/수정된다(ref 해소 포함).
2. FE에 `relationships` rpc 네임스페이스가 생겨 적용 경로가 동작한다.
3. 컨텍스트가 엔티티를 kind+id로 노출해 AI가 정확히 참조한다.
4. `go test ./...` 통과, 엔진 빌드 ok, `tsc --noEmit` 클린. 기존 플롯/메모리 op 무회귀.

## 범위 밖

- 씬(노드) 생성·문서 구조 변경 op — 후속.
- 양방향 관계 pair(create_pair) — 후속(우선 create_one).
- 온디맨드 읽기 조회 루프(linetta-query) — 후속.
- 엔티티 attributes/aliases 편집 op — 후속.
