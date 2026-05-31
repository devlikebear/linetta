# Companion Phase 4 — Entity/Relationship Actions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`).

**Goal:** 컴패니언 제안 op에 `create_entity`/`update_entity`/`create_relationship`을 추가해, AI가 검토 모델 그대로 캐릭터·장소·관계를 생성·수정하게 한다.

**Architecture:** 기존 `linetta-proposal` JSON op 어휘 확장(제공자 무관, 루프 없음). 엔진은 파싱·검증만; 적용은 FE `applyProposal`이 기존 RPC(`entities.create/update`, 신설 `relationships.create_one`)로. `entityRefMap`으로 같은 제안 내 신규 엔티티 ref 해소.

**Tech Stack:** Go 1.26 engine, React/TS FE.

---

## 사전 지식 (구현자 필독)

- 루트 `/Users/changheonshin/workspace/myworks/linetta`. engine `engine/`(`go test ./...`), FE `apps/desktop`(`npx tsc --noEmit`), 빌드 `bash scripts/build-engine.sh`. `main`, no --no-verify, no push. LSP stale 무시.
- proposal `Op`(engine `companion/proposal.go`) 현재 필드: Type, Ref, Name, Color, Summary, ThreadID, ThreadRef, NodeID, BeatID, Label, Description, Intensity, Outline, Text, Category. `knownOps` map, `validateProposal` switch, `ParseProposal`. add_beat의 node_id는 이미 optional(Phase 2 fix).
- entity: FE `entities.create(input: NewEntityInput) → Entity`, `entities.update(input: UpdateEntityInput) → Entity` (이미 존재). `NewEntityInput{project_id, kind, name, role?}`, `UpdateEntityInput{id, kind?, name?, role?, summary?, attributes?}`, `EntityKind = "character"|"place"|"item"|"concept"`, `Entity{id, kind, name, role, summary, ...}`. (NewEntityInput에 summary 없음 → 생성 후 update로 보정.)
- relationship: 엔진 핸들러 `relationships.create_one` (params = `relationship.NewInput{ProjectID,FromID,ToID,Label,Notes}` → JSON `{project_id, from_id, to_id, label, notes}`, project_id/from_id/to_id/label 필수). FE 타입 `NewRelationshipInput{project_id, from_id, to_id, label, notes?}`, `Relationship{...}` 이미 존재. **FE `relationships` rpc 네임스페이스는 없음 → 신설.**
- FE `applyProposal(ops, projectId, currentNodeId)`에 thread `refMap` 존재. `ProposalOp`(types.ts), `ProposalCard.opLabel`(컴포넌트). prompt.go `buildContext`의 `## 등장 인물·장소·관계`는 엔티티를 `[id] name / role: summary`로 렌더.

## File Structure
- Modify: `engine/internal/companion/proposal.go` (+ proposal_test.go)
- Modify: `engine/internal/companion/prompt.go` (+ prompt_test.go)
- Modify: `apps/desktop/src/lib/types.ts`, `lib/rpc.ts`, `lib/applyProposal.ts`, `components/companion/ProposalCard.tsx`

---

## Task 1: 엔진 proposal — 엔티티·관계 op

**Files:** `engine/internal/companion/proposal.go`, `engine/internal/companion/proposal_test.go`

- [ ] **Step 1: Op 필드 추가**

`Op` 구조체에 필드 추가(기존 필드 뒤):
```go
	// create_entity / update_entity
	Kind     string `json:"kind,omitempty"`
	Role     string `json:"role,omitempty"`
	EntityID string `json:"entity_id,omitempty"`

	// create_relationship
	From    string `json:"from,omitempty"`
	FromRef string `json:"from_ref,omitempty"`
	To      string `json:"to,omitempty"`
	ToRef   string `json:"to_ref,omitempty"`
	Notes   string `json:"notes,omitempty"`
```

- [ ] **Step 2: knownOps + 검증**

`knownOps`에 추가: `"create_entity": true, "update_entity": true, "create_relationship": true,`.

`validateProposal`: refs 수집 루프를 entity ref도 모으도록 확장(현재 create_thread.ref만 모음). 그 루프를 다음으로 교체:
```go
	refs := map[string]bool{}
	for _, op := range p.Ops {
		if (op.Type == "create_thread" || op.Type == "create_entity") && op.Ref != "" {
			refs[op.Ref] = true
		}
	}
```
switch에 케이스 추가:
```go
		case "create_entity":
			if strings.TrimSpace(op.Name) == "" {
				return fmt.Errorf("op[%d] create_entity: name required", i)
			}
			switch op.Kind {
			case "character", "place", "item", "concept":
			default:
				return fmt.Errorf("op[%d] create_entity: kind must be character|place|item|concept", i)
			}
		case "update_entity":
			if op.EntityID == "" {
				return fmt.Errorf("op[%d] update_entity: entity_id required", i)
			}
		case "create_relationship":
			if strings.TrimSpace(op.Label) == "" {
				return fmt.Errorf("op[%d] create_relationship: label required", i)
			}
			hasFrom, hasFromRef := op.From != "", op.FromRef != ""
			hasTo, hasToRef := op.To != "", op.ToRef != ""
			if hasFrom == hasFromRef {
				return fmt.Errorf("op[%d] create_relationship: exactly one of from/from_ref required", i)
			}
			if hasTo == hasToRef {
				return fmt.Errorf("op[%d] create_relationship: exactly one of to/to_ref required", i)
			}
			if hasFromRef && !refs[op.FromRef] {
				return fmt.Errorf("op[%d] create_relationship: from_ref %q not declared", i, op.FromRef)
			}
			if hasToRef && !refs[op.ToRef] {
				return fmt.Errorf("op[%d] create_relationship: to_ref %q not declared", i, op.ToRef)
			}
```

- [ ] **Step 3: 테스트**

`proposal_test.go`에 추가:
```go
func TestParseProposal_EntityAndRelationship(t *testing.T) {
	body := `{"ops":[
	  {"op":"create_entity","ref":"e1","kind":"character","name":"하나","role":"주인공"},
	  {"op":"create_entity","ref":"e2","kind":"character","name":"도윤"},
	  {"op":"create_relationship","from_ref":"e1","to_ref":"e2","label":"라이벌"}
	]}`
	p, present, err := ParseProposal(block(body))
	if !present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if len(p.Ops) != 3 || p.Ops[0].Name != "하나" || p.Ops[2].Label != "라이벌" {
		t.Fatalf("p=%+v", p)
	}
}

func TestParseProposal_CreateEntityRequiresKind(t *testing.T) {
	_, present, err := ParseProposal(block(`{"ops":[{"op":"create_entity","name":"X"}]}`))
	if !present || err == nil {
		t.Fatalf("expected kind error, present=%v err=%v", present, err)
	}
	_, _, err = ParseProposal(block(`{"ops":[{"op":"create_entity","name":"X","kind":"bogus"}]}`))
	if err == nil {
		t.Fatal("expected invalid-kind error")
	}
}

func TestParseProposal_RelationshipXorAndDanglingRef(t *testing.T) {
	// both from and from_ref → error
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"create_relationship","from":"a","from_ref":"x","to":"b","label":"L"}]}`)); err == nil {
		t.Fatal("expected from XOR error")
	}
	// dangling to_ref (no create_entity with that ref)
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"create_relationship","from":"a","to_ref":"nope","label":"L"}]}`)); err == nil {
		t.Fatal("expected dangling to_ref error")
	}
}

func TestParseProposal_UpdateEntityRequiresID(t *testing.T) {
	if _, _, err := ParseProposal(block(`{"ops":[{"op":"update_entity","name":"X"}]}`)); err == nil {
		t.Fatal("expected entity_id error")
	}
}
```

- [ ] **Step 4: 실행 + 커밋**

Run: `cd engine && go test ./internal/companion/...`
Expected: PASS(기존 + 신규). 기존 plot/remember op 테스트 회귀 없음 확인.
```bash
git add engine/internal/companion/proposal.go engine/internal/companion/proposal_test.go
git commit -m "feat(companion): add entity/relationship ops to proposal schema"
```

---

## Task 2: 엔진 prompt — kind 컨텍스트 + op 안내

**Files:** `engine/internal/companion/prompt.go`, `engine/internal/companion/prompt_test.go`

- [ ] **Step 1: 엔티티 줄에 kind 표시**

`buildContext`의 엔티티 렌더 루프에서 `kindLabel`을 붙인다. 현재:
```go
			line := fmt.Sprintf("- [%s] %s", e.ID, e.Name)
			if e.Role != "" {
				line += " / " + e.Role
			}
```
를:
```go
			line := fmt.Sprintf("- [%s] (%s) %s", e.ID, kindLabel(e.Kind), e.Name)
			if e.Role != "" {
				line += " / " + e.Role
			}
```
그리고 파일에 헬퍼 추가(없으면):
```go
func kindLabel(k string) string {
	switch k {
	case "character":
		return "인물"
	case "place":
		return "장소"
	case "item":
		return "물건"
	case "concept":
		return "개념"
	}
	return k
}
```
(`entity.Entity`에 `Kind string` 필드 존재.)

- [ ] **Step 2: buildSystem op 목록 + 엔티티 ref 안내**

`buildSystem`의 op 목록 문자열에 새 3개 추가:
```
, create_entity{ref?,kind,name,role?,summary?}, update_entity{entity_id,name?,role?,summary?}, create_relationship{from|from_ref,to|to_ref,label,notes?}
```
(기존 op 나열 줄 끝에 이어붙임. kind는 character|place|item|concept.)

id-규칙 블록(Phase 2 fix에서 추가된 "id를 절대 지어내지 마세요" 부분)에 한 줄 추가:
```go
	b.WriteString("- 캐릭터·장소(엔티티)와 관계도 같은 규칙입니다. 기존 엔티티는 '등장 인물·장소·관계' 목록의 id로 참조하고, 새 엔티티는 create_entity(ref 포함) 후 create_relationship에서 from_ref/to_ref로 그 ref를 참조하세요.\n")
```

- [ ] **Step 3: 테스트**

`prompt_test.go`에 추가:
```go
func TestBuildContext_EntityShowsKind(t *testing.T) {
	d := PromptData{Entities: []entity.Entity{{ID: "e1", Kind: "character", Name: "하나", Role: "주인공"}}}
	out := buildContext(d)
	if !strings.Contains(out, "[e1] (인물) 하나") {
		t.Fatalf("entity kind not rendered:\n%s", out)
	}
}

func TestBuildSystem_MentionsEntityOps(t *testing.T) {
	s := buildSystem()
	for _, want := range []string{"create_entity", "create_relationship", "from_ref"} {
		if !strings.Contains(s, want) {
			t.Fatalf("buildSystem missing %q", want)
		}
	}
}
```
(`prompt_test.go`가 `entity` 패키지를 import 안 했으면 추가: `"github.com/devlikebear/linetta/engine/internal/entity"`. 기존 `TestBuildContext_RendersSections`가 엔티티를 안 쓰면 영향 없음; kind 변경으로 기존 엔티티 단언이 깨지면 그 단언을 `(인물)` 포함 형태로 갱신.)

- [ ] **Step 4: 빌드/테스트 + 커밋**

Run: `cd engine && go build ./... && go test ./internal/companion/...`
Expected: PASS.
```bash
git add engine/internal/companion/prompt.go engine/internal/companion/prompt_test.go
git commit -m "feat(companion): expose entity kind + entity/relationship op guidance in prompt"
```

---

## Task 3: FE — 타입 + relationships rpc + applyProposal + 카드

**Files:** `apps/desktop/src/lib/types.ts`, `lib/rpc.ts`, `lib/applyProposal.ts`, `components/companion/ProposalCard.tsx`

- [ ] **Step 1: types.ts**

- `ProposalOpType` 유니온에 추가: `| "create_entity" | "update_entity" | "create_relationship"`.
- `ProposalOp`에 필드 추가:
```ts
  kind?: string;
  role?: string;
  entity_id?: string;
  from?: string;
  from_ref?: string;
  to?: string;
  to_ref?: string;
  notes?: string;
```

- [ ] **Step 2: rpc.ts — relationships 네임스페이스**

import 타입에 `NewRelationshipInput`, `Relationship` 추가(없으면). 그리고:
```ts
export const relationships = {
  createOne: (input: NewRelationshipInput) =>
    rpcCall<Relationship>("relationships.create_one", input),
};
```
(`NewRelationshipInput{project_id, from_id, to_id, label, notes?}` 이미 types.ts에 존재.)

- [ ] **Step 3: applyProposal.ts — 3 케이스 + entityRefMap**

- import에 entities/relationships 추가:
```ts
import { threads as threadsApi, beats as beatsApi, projects as projectsApi, companion as companionApi, entities as entitiesApi, relationships as relationshipsApi } from "./rpc";
```
- 함수 상단에 `const entityRefMap = new Map<string, string>();` (threads refMap 옆).
- switch에 케이스 추가(`default` 앞):
```ts
        case "create_entity": {
          if (!op.kind || !op.name) throw new Error("kind/name 없음");
          const ent = await entitiesApi.create({
            project_id: projectId,
            kind: op.kind as never,
            name: op.name,
            role: op.role,
          });
          if (op.ref) entityRefMap.set(op.ref, ent.id);
          if (op.summary) {
            await entitiesApi.update({ id: ent.id, summary: op.summary });
          }
          break;
        }
        case "update_entity": {
          if (!op.entity_id) throw new Error("entity_id 없음");
          await entitiesApi.update({
            id: op.entity_id,
            name: op.name,
            role: op.role,
            summary: op.summary,
            kind: op.kind as never,
          });
          break;
        }
        case "create_relationship": {
          const fromId = op.from ?? (op.from_ref ? entityRefMap.get(op.from_ref) : undefined);
          const toId = op.to ?? (op.to_ref ? entityRefMap.get(op.to_ref) : undefined);
          if (!fromId || !toId) throw new Error("관계 양쪽 엔티티 참조를 해소할 수 없음");
          if (!op.label) throw new Error("관계 라벨 없음");
          await relationshipsApi.createOne({
            project_id: projectId,
            from_id: fromId,
            to_id: toId,
            label: op.label,
            notes: op.notes,
          });
          break;
        }
```
NOTE: `op.kind as never` — `ProposalOp.kind` is `string` but `NewEntityInput.kind`/`UpdateEntityInput.kind` is the `EntityKind` union. The engine already validated kind ∈ the 4 values, so cast is safe. If `as never` triggers a lint complaint, use `as EntityKind` and import the `EntityKind` type from `./types`. Prefer `as EntityKind` (cleaner) — import it.

- [ ] **Step 4: ProposalCard.tsx — opLabel**

`opLabel` switch에 추가(`default` 앞):
```tsx
    case "create_entity": return `${op.kind === "place" ? "장소" : "캐릭터"} 생성: ${op.name ?? ""}`;
    case "update_entity": return `엔티티 수정`;
    case "create_relationship": return `관계 생성: ${op.label ?? ""}`;
```

- [ ] **Step 5: tsc + 커밋**

Run: `cd apps/desktop && npx tsc --noEmit`
Expected: 클린. (`as EntityKind` 캐스트 쓰면 `import type { EntityKind } from "../lib/types"` 또는 applyProposal에서 `./types` import에 추가.)
```bash
git add apps/desktop/src/lib/types.ts apps/desktop/src/lib/rpc.ts apps/desktop/src/lib/applyProposal.ts apps/desktop/src/components/companion/ProposalCard.tsx
git commit -m "feat(desktop): apply entity/relationship proposal ops"
```

---

## Task 4: 최종 검증

- [ ] `cd engine && go test ./...` 전 패키지 PASS
- [ ] `cd apps/desktop && npx tsc --noEmit` 클린
- [ ] repo root `bash scripts/build-engine.sh` → ok
- [ ] 수동 스모크(사용자): "주인공 하나와 라이벌 도윤을 만들고 둘을 라이벌로 묶어줘" → 제안 카드(캐릭터 생성×2 + 관계 생성) → 적용 → 엔티티 검색/시트에 하나·도윤 + 관계 반영

## 범위 밖
- 씬 생성·문서구조 op, 양방향 pair, 온디맨드 읽기 루프, attributes/aliases 편집 — 후속.
