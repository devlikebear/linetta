# Companion Phase 5 — Scene + Bidirectional Relationship Ops Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`).

**Goal:** 컴패니언 제안 op에 `create_scene`(새 씬 생성, node_ref로 add_beat 부착)과 `create_relationship.inverse_label`(양방향 관계)을 추가한다.

**Architecture:** 기존 `linetta-proposal` JSON op 확장(제공자 무관, 루프 없음). 엔진 파싱·검증만; 적용은 FE `applyProposal`이 기존 RPC(`nodes.createSibling`, `relationships.createOne/createPair`)로. `nodeRefMap`으로 신규 씬 ref 해소.

**Tech Stack:** Go engine, React/TS FE.

---

## 사전 지식 (구현자 필독)

- 루트 `/Users/changheonshin/workspace/myworks/linetta`. engine `engine/`(`go test ./...`), FE `apps/desktop`(`npx tsc --noEmit`), 빌드 `bash scripts/build-engine.sh`(repo root). `main`, no --no-verify, no push. LSP stale 무시, 명령 출력만 신뢰.
- proposal `Op`(engine `companion/proposal.go`) 현재 필드: Type, Ref, Name, Color, Summary, ThreadID, ThreadRef, NodeID, BeatID, Label, Description, Intensity, Outline, Text, Category, Kind, Role, EntityID, From, FromRef, To, ToRef, Notes. `knownOps` map. `validateProposal`: `refs` 맵이 create_thread+create_entity의 ref를 수집, 그 뒤 per-op switch. `block(body)` 헬퍼는 proposal_test.go에 있음.
- FE `nodes.createSibling(referenceId, kind: "leaf"|"container", label, title) → NodeRow`. `relationships.createOne(NewRelationshipInput)`, `relationships.createPair(NewRelationshipPairInput{project_id,from_id,to_id,label,inverse_label,notes?})` — 둘 다 이미 존재. `applyProposal(ops, projectId, currentNodeId)`에 `refMap`(thread), `entityRefMap`. `ProposalOp`(types.ts), `ProposalCard.opLabel`.
- prompt.go: `buildSystem` op 목록 줄 + id-규칙 블록. `## 씬` 섹션은 node_id 노출(Phase 2 fix).

## File Structure
- Modify: `engine/internal/companion/proposal.go` (+ proposal_test.go), `prompt.go` (+ prompt_test.go)
- Modify: `apps/desktop/src/lib/types.ts`, `lib/applyProposal.ts`, `components/companion/ProposalCard.tsx`

---

## Task 1: 엔진 proposal — create_scene + node_ref + inverse_label

**Files:** `engine/internal/companion/proposal.go`, `proposal_test.go`

- [ ] **Step 1: Op 필드 추가**
```go
	// create_scene
	AfterNodeID string `json:"after_node_id,omitempty"`
	Title       string `json:"title,omitempty"`
	NodeRef     string `json:"node_ref,omitempty"`

	// create_relationship bidirectional
	InverseLabel string `json:"inverse_label,omitempty"`
```

- [ ] **Step 2: knownOps + refs + 검증**
- `knownOps`에 `"create_scene": true,`.
- refs 수집 루프에 create_scene 포함:
```go
	refs := map[string]bool{}
	for _, op := range p.Ops {
		if (op.Type == "create_thread" || op.Type == "create_entity" || op.Type == "create_scene") && op.Ref != "" {
			refs[op.Ref] = true
		}
	}
```
- switch에 create_scene 케이스 추가:
```go
		case "create_scene":
			if strings.TrimSpace(op.Label) == "" {
				return fmt.Errorf("op[%d] create_scene: label required", i)
			}
```
- `add_beat` 케이스에 node_id/node_ref 동시 금지 + dangling node_ref 검사 추가(기존 add_beat 케이스 본문 끝에 삽입):
```go
			if op.NodeID != "" && op.NodeRef != "" {
				return fmt.Errorf("op[%d] add_beat: node_id and node_ref are mutually exclusive", i)
			}
			if op.NodeRef != "" && !refs[op.NodeRef] {
				return fmt.Errorf("op[%d] add_beat: node_ref %q not declared by any create_scene.ref", i, op.NodeRef)
			}
```
(create_relationship 케이스는 변경 없음 — inverse_label은 optional.)

- [ ] **Step 3: 테스트 (proposal_test.go 추가)**
```go
func TestParseProposal_CreateSceneAndNodeRef(t *testing.T) {
	body := `{"ops":[
	  {"op":"create_thread","ref":"t1","name":"메인"},
	  {"op":"create_scene","ref":"s1","label":"재회"},
	  {"op":"add_beat","thread_ref":"t1","node_ref":"s1","label":"첫 만남"}
	]}`
	p, present, err := ParseProposal(block(body))
	if !present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if len(p.Ops) != 3 || p.Ops[1].Label != "재회" || p.Ops[2].NodeRef != "s1" {
		t.Fatalf("p=%+v", p)
	}
}

func TestParseProposal_CreateSceneRequiresLabel(t *testing.T) {
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"create_scene"}]}`)); err == nil {
		t.Fatal("expected label error")
	}
}

func TestParseProposal_AddBeatNodeIDXorNodeRef(t *testing.T) {
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"add_beat","thread_id":"x","node_id":"n","node_ref":"s","label":"L"}]}`)); err == nil {
		t.Fatal("expected node_id/node_ref mutual-exclusion error")
	}
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"add_beat","thread_id":"x","node_ref":"nope","label":"L"}]}`)); err == nil {
		t.Fatal("expected dangling node_ref error")
	}
}

func TestParseProposal_RelationshipInverseLabel(t *testing.T) {
	_, present, err := ParseProposal(block(`{"ops":[{"op":"create_relationship","from":"a","to":"b","label":"라이벌","inverse_label":"라이벌"}]}`))
	if !present || err != nil {
		t.Fatalf("inverse_label should be valid: present=%v err=%v", present, err)
	}
}
```

- [ ] **Step 4: 실행 + 커밋**
Run: `cd engine && go test ./internal/companion/...` (PASS, 기존 회귀 없음)
```bash
git add engine/internal/companion/proposal.go engine/internal/companion/proposal_test.go
git commit -m "feat(companion): create_scene op + add_beat node_ref + bidirectional relationship"
```

---

## Task 2: 엔진 prompt — op 안내

**Files:** `engine/internal/companion/prompt.go`, `prompt_test.go`

- [ ] **Step 1: buildSystem op 목록 + 안내**
- op 목록 문자열에 추가: `, create_scene{ref?,label,title?,after_node_id?}`. `add_beat` 표기를 `add_beat{thread_id|thread_ref,node_id?|node_ref?,label,...}`로, `create_relationship`을 `create_relationship{from|from_ref,to|to_ref,label,inverse_label?,notes?}`로 갱신.
- 안내 한 줄 추가(id-규칙 블록 내):
```go
	b.WriteString("- 새 씬은 create_scene(ref 포함) 후 add_beat.node_ref로 그 씬에 비트를 붙입니다(node_id 생략 시 현재 씬). 관계를 양방향으로 만들려면 create_relationship에 inverse_label을 주세요.\n")
```

- [ ] **Step 2: 테스트 (prompt_test.go)**
```go
func TestBuildSystem_MentionsSceneAndPair(t *testing.T) {
	s := buildSystem()
	for _, want := range []string{"create_scene", "node_ref", "inverse_label"} {
		if !strings.Contains(s, want) {
			t.Fatalf("buildSystem missing %q", want)
		}
	}
}
```

- [ ] **Step 3: 빌드/테스트 + 커밋**
Run: `cd engine && go build ./... && go test ./internal/companion/...` (PASS)
```bash
git add engine/internal/companion/prompt.go engine/internal/companion/prompt_test.go
git commit -m "feat(companion): prompt guidance for create_scene + bidirectional relationship"
```

---

## Task 3: FE — types + applyProposal + 카드

**Files:** `apps/desktop/src/lib/types.ts`, `lib/applyProposal.ts`, `components/companion/ProposalCard.tsx`

- [ ] **Step 1: types.ts**
- `ProposalOpType += "create_scene"`.
- `ProposalOp += after_node_id?: string; title?: string; node_ref?: string; inverse_label?: string;`

- [ ] **Step 2: applyProposal.ts**
- import에 `nodes as nodesApi` 추가(기존 rpc import 줄에): `import { ... , nodes as nodesApi } from "./rpc";`
- 함수 상단에 `const nodeRefMap = new Map<string, string>();` (refMap/entityRefMap 옆).
- `create_scene` 케이스 추가(`default` 앞):
```ts
        case "create_scene": {
          if (!op.label) throw new Error("씬 라벨 없음");
          const ref = op.after_node_id ?? currentNodeId;
          if (!ref) throw new Error("씬을 만들 기준 위치(현재 씬)가 없음");
          const node = await nodesApi.createSibling(ref, "leaf", op.label, op.title ?? "");
          if (op.ref) nodeRefMap.set(op.ref, node.id);
          break;
        }
```
- `add_beat` 케이스의 node 해소를 변경:
```ts
        case "add_beat": {
          const tid = op.thread_id ?? (op.thread_ref ? refMap.get(op.thread_ref) : undefined);
          if (!tid) throw new Error("스토리라인 참조를 해소할 수 없음");
          const nodeId = op.node_id ?? (op.node_ref ? nodeRefMap.get(op.node_ref) : undefined) ?? currentNodeId ?? undefined;
          await beatsApi.create({
            thread_id: tid,
            node_id: nodeId ?? undefined,
            label: op.label,
            description: op.description,
            intensity: op.intensity,
          });
          break;
        }
```
- `create_relationship` 케이스: inverse_label 분기 추가(기존 케이스 교체):
```ts
        case "create_relationship": {
          const fromId = op.from ?? (op.from_ref ? entityRefMap.get(op.from_ref) : undefined);
          const toId = op.to ?? (op.to_ref ? entityRefMap.get(op.to_ref) : undefined);
          if (!fromId || !toId) throw new Error("관계 양쪽 엔티티 참조를 해소할 수 없음");
          if (!op.label) throw new Error("관계 라벨 없음");
          if (op.inverse_label) {
            await relationshipsApi.createPair({
              project_id: projectId, from_id: fromId, to_id: toId,
              label: op.label, inverse_label: op.inverse_label, notes: op.notes,
            });
          } else {
            await relationshipsApi.createOne({
              project_id: projectId, from_id: fromId, to_id: toId, label: op.label, notes: op.notes,
            });
          }
          break;
        }
```

- [ ] **Step 3: ProposalCard.tsx — opLabel**
`opLabel` switch에 추가(`default` 앞): `case "create_scene": return \`씬 생성: ${op.label ?? ""}\`;`

- [ ] **Step 4: tsc + 커밋**
Run: `cd apps/desktop && npx tsc --noEmit` (클린)
```bash
git add apps/desktop/src/lib/types.ts apps/desktop/src/lib/applyProposal.ts apps/desktop/src/components/companion/ProposalCard.tsx
git commit -m "feat(desktop): apply create_scene + node_ref beats + bidirectional relationship"
```

---

## Task 4: 최종 검증
- [ ] `cd engine && go test ./...` 전 패키지 PASS
- [ ] `cd apps/desktop && npx tsc --noEmit` 클린
- [ ] repo root `bash scripts/build-engine.sh` → ok
- [ ] 수동 스모크(사용자): "새 씬 '재회' 만들고 메인플롯 비트 2개 넣어줘" → 카드(씬 생성 + 비트×2) → 적용 → 트리에 새 씬 + 플롯에 비트. "A와 B를 라이벌로 양방향 묶어줘" → 관계 pair 생성.

## 범위 밖
- 온디맨드 읽기 루프 — Phase 6. 씬 삭제/이동/컨테이너 생성 — 후속.
