# Plan 23 — 등장 씬 타임라인 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** EntitySheet 에 "이 엔티티가 등장하는 씬 목록"을 문서 순서대로 breadcrumb 경로로 보여주고, 클릭하면 그 씬으로 이동한다.

**Architecture:** mention.Repo 에 `MentionedNodeIDs` (distinct node_ids + project_id) 추가. 새 핸들러 `entities.scenes` 가 그것 + node.Repo 의 프로젝트 트리를 DFS 로 조합해 문서순 breadcrumb 목록(`[]SceneMention`)을 반환. FE 는 EntitySheet 에 "등장 씬" 섹션 추가 + Workspace onNavigate 연결. 엔진 schema 변경 없음.

**Tech Stack:** Go 1.26 (engine, sqlite), TypeScript / React 18 (frontend), Tauri JSONRPC.

---

## 파일 구조

**Engine:**
- `engine/internal/mention/repo.go` — `+MentionedNodeIDs`
- `engine/internal/mention/repo_test.go` — `+TestMentionedNodeIDs`
- `engine/internal/rpc/handlers/entities.go` — `+EntityScenes` + `SceneMention` + `breadcrumbLabel`
- `engine/internal/rpc/handlers/entities_test.go` — `+TestEntityScenesHandler`
- `engine/cmd/linetta-engine/main.go` — `entities.scenes` 등록

**Frontend:**
- `apps/desktop/src/lib/types.ts` — `+SceneMention`
- `apps/desktop/src/lib/rpc.ts` — `entities.scenes`
- `apps/desktop/src/components/EntitySheet.tsx` — `등장 씬` 섹션 + `onNavigate` prop
- `apps/desktop/src/components/EntitySheet.css` — `.entity-scenes` / `.entity-scene-link`
- `apps/desktop/src/routes/Workspace.tsx` — EntitySheet onNavigate 연결

엔진 schema 변경 없음 (mentions/nodes/entities 기존 테이블).

**FE 테스트 인프라:** vitest 미설치 — 타입체크 + 수동 스모크.

---

## Task 1: mention.Repo.MentionedNodeIDs

**Files:**
- Modify: `engine/internal/mention/repo.go`
- Modify: `engine/internal/mention/repo_test.go`

### Step 1: 실패 테스트 추가

`engine/internal/mention/repo_test.go` 의 기존 테스트 셋업 패턴을 먼저 확인 (store.Open + project + node + entity + ResyncForNode). 그 패턴으로 새 테스트 추가:

```go
func TestMentionedNodeIDs(t *testing.T) {
	ctx := context.Background()
	// 기존 mention repo 테스트와 동일 셋업: store, project, nodes, entity.
	// (이 파일의 다른 테스트가 store/project/node/entity/mention 을 어떻게 만드는지 따라할 것.)
	// 핵심:
	//   - project p
	//   - leaf 노드 두 개 n1, n2 (p 소속)
	//   - entity e (p 소속)
	//   - n1 에 e 멘션 2번 (같은 노드 중복), n2 에 e 멘션 1번
	// 그 뒤:
	ids, projectID, err := mr.MentionedNodeIDs(ctx, e.ID)
	if err != nil {
		t.Fatalf("MentionedNodeIDs: %v", err)
	}
	if projectID != p.ID {
		t.Fatalf("projectID=%q want %q", projectID, p.ID)
	}
	// distinct → n1, n2 (중복 제거)
	if len(ids) != 2 {
		t.Fatalf("ids=%v want 2 distinct", ids)
	}
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	if !set[n1.ID] || !set[n2.ID] {
		t.Fatalf("ids missing n1/n2: %v", ids)
	}
}

func TestMentionedNodeIDs_noMentions(t *testing.T) {
	ctx := context.Background()
	// project p + entity e (멘션 0).
	ids, projectID, err := mr.MentionedNodeIDs(ctx, e.ID)
	if err != nil {
		t.Fatalf("MentionedNodeIDs: %v", err)
	}
	if projectID != p.ID {
		t.Fatalf("projectID=%q want %q", projectID, p.ID)
	}
	if len(ids) != 0 {
		t.Fatalf("ids=%v want empty", ids)
	}
}
```

mention 을 만드는 방법: 이 패키지의 `ResyncForNode(ctx, nodeID, []Found{...})` 사용. `Found` 구조와 entity 생성은 기존 테스트 (`TestResyncForNode`, `TestListEntitiesForNode` 등) 패턴을 그대로 따른다. entity 는 `entity.NewRepo(s).Create(...)` 로.

### Step 2: 실패 확인

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./internal/mention -run TestMentionedNodeIDs -v
```

기대: 컴파일 에러 (`MentionedNodeIDs` 미존재).

### Step 3: 구현

`engine/internal/mention/repo.go` 에 추가 (다른 메서드 옆):

```go
// MentionedNodeIDs returns the distinct node_ids where entityID is mentioned,
// plus the entity's project_id (resolved from the entities table so it works
// even when the entity has zero mentions).
func (r *Repo) MentionedNodeIDs(ctx context.Context, entityID string) (ids []string, projectID string, err error) {
	if err = r.s.DB().QueryRowContext(ctx,
		`SELECT project_id FROM entities WHERE id = ?`, entityID).Scan(&projectID); err != nil {
		return nil, "", err
	}
	rows, err := r.s.DB().QueryContext(ctx,
		`SELECT DISTINCT node_id FROM mentions WHERE entity_id = ?`, entityID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	ids = []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, "", err
		}
		ids = append(ids, id)
	}
	return ids, projectID, rows.Err()
}
```

(`r.s.DB()` 는 이 파일의 다른 메서드가 쓰는 store 접근 패턴 — `RecentSummariesForEntity` 가 `r.s.DB().QueryContext` 를 쓰므로 동일.)

### Step 4: 통과 확인

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./internal/mention -v && gofmt -l ./internal/mention
```

기대: 모든 mention 테스트 PASS, gofmt 빈 출력.

### Step 5: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/internal/mention/repo.go engine/internal/mention/repo_test.go
git commit -m "feat(mention): MentionedNodeIDs — distinct node_ids + project_id for an entity"
```

## Context

Plan 23 Task 1. 엔티티 등장 씬 타임라인의 데이터 쿼리. `entities` 테이블에서 project_id 를 먼저 뽑아 (멘션 0 인 엔티티도 동작), `mentions` 에서 distinct node_id 를 가져온다. 트리 DFS + breadcrumb 조합은 Task 2 의 핸들러가 담당 (mention.Repo 는 node.Repo 와 결합 안 함).

`r.s` 는 `*store.Store`, `r.s.DB()` 로 `*sql.DB`. mentions 테이블 컬럼: `node_id`, `entity_id` (0001_init.sql). entities 테이블: `id`, `project_id`.

Working directory: `/Users/changheonshin/workspace/myworks/linetta`. Branch: `main`. Never push. Never skip hooks.

## Report Format

- Status DONE | DONE_WITH_CONCERNS | BLOCKED
- 추가 테스트 결과
- gofmt 출력
- 커밋 SHA
- 우려사항

---

## Task 2: entities.scenes 핸들러

**Files:**
- Modify: `engine/internal/rpc/handlers/entities.go`
- Modify: `engine/internal/rpc/handlers/entities_test.go`
- Modify: `engine/cmd/linetta-engine/main.go`

### Step 1: 실패 테스트 추가

`engine/internal/rpc/handlers/entities_test.go` 에 추가. 기존 `newEntityFixture(t)` 는 `(*entity.Repo, project.Project)` 만 주므로 — 이 테스트는 store/mention/node 까지 필요. 같은 파일의 fixture 가 내부적으로 store 를 만드는 방식을 보고, 트리 + 멘션을 만들 수 있는 확장 fixture 를 인라인으로 작성 (또는 store 를 노출하는 헬퍼). 가장 단순히: 새 인라인 셋업.

```go
func TestEntityScenesHandler(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "t.db")
	s, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	pr := project.NewRepo(s)
	nr := node.NewRepo(s)
	er := entity.NewRepo(s)
	mr := mention.NewRepo(s)

	now := int64(1000)
	p, err := pr.Create(ctx, now, project.NewInput{Title: "t", Genres: []string{}, LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	// pr.Create seeds a "씬 1" leaf. Build a small tree under it:
	//   부 (container) → 1장 (container) → 씬A (leaf), 씬B (leaf)
	// Use node.Repo CreateChild/CreateSibling. Exact methods: read node/repo.go.
	bu, _ := nr.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "1부", "", now)
	ch, _ := nr.CreateChild(ctx, bu.ID, "container", "1장", "", now)
	scA, _ := nr.CreateChild(ctx, ch.ID, "leaf", "씬A", "", now)
	scB, _ := nr.CreateChild(ctx, ch.ID, "leaf", "씬B", "", now)

	e, err := er.Create(ctx, now, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진"})
	if err != nil {
		t.Fatalf("entity: %v", err)
	}
	// Mention e in 씬B twice (dup), 씬A once. Document order should yield 씬A then 씬B.
	if err := mr.ResyncForNode(ctx, scA.ID, []mention.Found{{EntityID: e.ID}}); err != nil {
		t.Fatalf("resync A: %v", err)
	}
	if err := mr.ResyncForNode(ctx, scB.ID, []mention.Found{{EntityID: e.ID}, {EntityID: e.ID}}); err != nil {
		t.Fatalf("resync B: %v", err)
	}

	h := EntityScenes(mr, nr)
	params, _ := json.Marshal(map[string]any{"entity_id": e.ID})
	raw, err := h(ctx, params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got []SceneMention
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 scenes, got %d: %+v", len(got), got)
	}
	// document order: 씬A before 씬B
	if got[0].Label != "1부 / 1장 / 씬A" {
		t.Fatalf("got[0].Label=%q want '1부 / 1장 / 씬A'", got[0].Label)
	}
	if got[1].Label != "1부 / 1장 / 씬B" {
		t.Fatalf("got[1].Label=%q want '1부 / 1장 / 씬B'", got[1].Label)
	}
	if got[0].NodeID != scA.ID || got[1].NodeID != scB.ID {
		t.Fatalf("node ids mismatch: %+v", got)
	}
}

func TestEntityScenesHandler_noMentions(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "t.db")
	s, _ := store.Open(ctx, dbPath)
	defer s.Close()
	pr := project.NewRepo(s)
	er := entity.NewRepo(s)
	mr := mention.NewRepo(s)
	nr := node.NewRepo(s)
	p, _ := pr.Create(ctx, 1000, project.NewInput{Title: "t", Genres: []string{}, LengthTarget: "short", DefaultPOV: "first"})
	e, _ := er.Create(ctx, 1000, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "무명"})

	h := EntityScenes(mr, nr)
	params, _ := json.Marshal(map[string]any{"entity_id": e.ID})
	raw, err := h(ctx, params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got []SceneMention
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %+v", got)
	}
}

func TestEntityScenesHandler_emptyID(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "t.db")
	s, _ := store.Open(ctx, dbPath)
	defer s.Close()
	h := EntityScenes(mention.NewRepo(s), node.NewRepo(s))
	params, _ := json.Marshal(map[string]any{"entity_id": ""})
	_, err := h(ctx, params)
	if err == nil {
		t.Fatal("expected InvalidParams for empty entity_id")
	}
	var mErr *rpc.MethodError
	if !errors.As(err, &mErr) || mErr.Code != rpc.CodeInvalidParams {
		t.Fatalf("expected rpc.MethodError CodeInvalidParams, got %T %v", err, err)
	}
}
```

**중요 — 실제 API 확인:**
- `node.Repo.CreateChild` / `CreateSibling` 의 정확한 시그니처를 `engine/internal/node/repo.go` 에서 확인해 맞춘다 (위 호출은 `(ctx, parentOrRefID, kind, label, content, now)` 형태로 추정 — 실제와 다르면 맞출 것).
- `mention.Found` 구조 (EntityID 필드명) 를 `engine/internal/mention/repo.go` 에서 확인 (`ResyncForNode(ctx, nodeID, []Found)`). Found 가 다른 필드를 요구하면 그에 맞춤.
- import 에 `path/filepath`, `errors`, `store`, `project`, `node`, `entity`, `mention`, `encoding/json` 필요 — 기존 entities_test.go import 에 더한다.

### Step 2: 실패 확인

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./internal/rpc/handlers -run TestEntityScenes -v
```

기대: 컴파일 에러 (`EntityScenes`, `SceneMention` 미존재).

### Step 3: 핸들러 구현

`engine/internal/rpc/handlers/entities.go` 에 추가. import 에 `strings`, `github.com/devlikebear/linetta/engine/internal/mention`, `github.com/devlikebear/linetta/engine/internal/node` 추가:

```go
type entityScenesParams struct {
	EntityID string `json:"entity_id"`
}

// SceneMention is one scene where an entity appears (RPC result shape).
type SceneMention struct {
	NodeID string `json:"node_id"`
	Label  string `json:"label"`
}

// EntityScenes returns the distinct scenes (leaf nodes) where the entity is
// mentioned, in document order (tree DFS), each with a breadcrumb label.
func EntityScenes(mentions *mention.Repo, nodes *node.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p entityScenesParams
		if err := json.Unmarshal(params, &p); err != nil || p.EntityID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "entity_id required"}
		}
		ids, projectID, err := mentions.MentionedNodeIDs(ctx, p.EntityID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if len(ids) == 0 {
			return json.Marshal([]SceneMention{})
		}
		mentioned := make(map[string]bool, len(ids))
		for _, id := range ids {
			mentioned[id] = true
		}
		all, err := nodes.ListByProject(ctx, projectID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		byID := make(map[string]node.Node, len(all))
		children := map[string][]node.Node{}
		for _, n := range all {
			byID[n.ID] = n
			key := ""
			if n.ParentID != nil {
				key = *n.ParentID
			}
			children[key] = append(children[key], n)
		}
		out := []SceneMention{}
		var walk func(parent string)
		walk = func(parent string) {
			for _, c := range children[parent] {
				if c.Kind == "leaf" && mentioned[c.ID] {
					out = append(out, SceneMention{NodeID: c.ID, Label: breadcrumbLabel(byID, c)})
				}
				walk(c.ID)
			}
		}
		walk("")
		return json.Marshal(out)
	}
}

// breadcrumbLabel builds "부 / 장 / 씬" by walking parent_id up to the root.
func breadcrumbLabel(byID map[string]node.Node, n node.Node) string {
	parts := []string{n.Label}
	cur := n
	for cur.ParentID != nil {
		par, ok := byID[*cur.ParentID]
		if !ok {
			break
		}
		parts = append([]string{par.Label}, parts...)
		cur = par
	}
	return strings.Join(parts, " / ")
}
```

(`ListByProject` 는 `(parent_id IS NULL) DESC, parent_id, ordinal` 정렬 — 같은 부모의 children 이 ordinal 순으로 연속. children[key] 가 ordinal 순서를 받으므로 DFS 가 문서 순서를 생성. Plan 16 동일 패턴.)

### Step 4: main.go 등록

`engine/cmd/linetta-engine/main.go` 의 `entities.update` 등록 줄 다음에 추가:

```go
s.Handle("entities.scenes", handlers.EntityScenes(mentions, nodes))
```

(`mentions`, `nodes` 변수는 main.go 에서 이미 생성됨 — `entities.search` 등록부 근처에서 확인. mention repo 변수명이 `mentions` 인지 확인.)

### Step 5: 통과 확인 + 엔진 빌드

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./... && gofmt -l ./internal/rpc/handlers
cd /Users/changheonshin/workspace/myworks/linetta && ./scripts/build-engine.sh
```

기대: 모든 패키지 PASS, gofmt 빈 출력, 엔진 빌드 성공.

### Step 6: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/internal/rpc/handlers/entities.go engine/internal/rpc/handlers/entities_test.go engine/cmd/linetta-engine/main.go
git commit -m "feat(rpc): entities.scenes — document-ordered scenes where an entity appears"
```

## Context

Plan 23 Task 2. Task 1 의 `MentionedNodeIDs` 를 써서 멘션 node_id 집합 + project_id 를 얻고, node.Repo 의 프로젝트 트리를 DFS 해 문서순 breadcrumb 목록 반환. mention.Repo 와 node.Repo 의 결합은 핸들러에서만 (repo 시그니처 무변경).

`breadcrumbLabel` 은 Plan 16 ai/context.go 의 동일 로직 — 패키지 경계상 private 재활용 불가라 핸들러에 작은 복제. 첫 버전 수용 (위험 섹션 명시).

main.go 변수: `entities.search` 등록부에서 mention/node repo 변수명 확인 후 정확히 전달.

Working directory: `/Users/changheonshin/workspace/myworks/linetta`. Branch: `main`. Never push. Never skip hooks.

## Before You Begin

`node.Repo.CreateChild`/`CreateSibling` 시그니처, `mention.Found` 구조, main.go 의 mention/node 변수명을 먼저 확인. 테스트 코드의 호출을 실제 API 에 맞춰 조정.

## Report Format

- Status DONE | DONE_WITH_CONCERNS | BLOCKED
- node.Repo / mention.Found 실제 API 확인 결과
- `go test ./...` 결과
- gofmt / 엔진 빌드 결과
- 커밋 SHA
- 우려사항

---

## Task 3: FE 타입 + rpc 클라이언트

**Files:**
- Modify: `apps/desktop/src/lib/types.ts`
- Modify: `apps/desktop/src/lib/rpc.ts`

### Step 1: 타입 추가

`apps/desktop/src/lib/types.ts` 의 다른 entity 관련 타입 옆에 추가:

```ts
export interface SceneMention {
  node_id: string;
  label: string;
}
```

### Step 2: rpc 클라이언트

`apps/desktop/src/lib/rpc.ts` 의 `entities` 객체에 `scenes` 추가:

```ts
export const entities = {
  search: (projectId: string, query: string, limit = 20) =>
    rpcCall<Entity[]>("entities.search", { project_id: projectId, query, limit }),
  get: (id: string) => rpcCall<Entity>("entities.get", { id }),
  create: (input: NewEntityInput) => rpcCall<Entity>("entities.create", input),
  update: (input: UpdateEntityInput) => rpcCall<Entity>("entities.update", input),
  scenes: (entityId: string) => rpcCall<SceneMention[]>("entities.scenes", { entity_id: entityId }),
};
```

`rpc.ts` 상단 import 블록의 `./types` 에서 `SceneMention` 가 import 되는지 확인하고 없으면 추가.

### Step 3: 타입체크

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
```

기대: 에러 없음.

### Step 4: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/lib/types.ts apps/desktop/src/lib/rpc.ts
git commit -m "feat(rpc-client): entities.scenes + SceneMention type"
```

## Context

Plan 23 Task 3. Task 2 의 `entities.scenes` RPC 의 FE 클라이언트. wire 모양 `{ node_id, label }` 그대로 (snake_case — EntitySheet 가 직접 소비).

Working directory: `/Users/changheonshin/workspace/myworks/linetta`. Branch: `main`. Never push. Never skip hooks.

## Report Format

- Status DONE | BLOCKED
- 타입체크 결과
- 커밋 SHA

---

## Task 4: EntitySheet 등장 씬 섹션 + Workspace 연결

**Files:**
- Modify: `apps/desktop/src/components/EntitySheet.tsx`
- Modify: `apps/desktop/src/components/EntitySheet.css`
- Modify: `apps/desktop/src/routes/Workspace.tsx`

### Step 1: EntitySheet — import + state + fetch

`apps/desktop/src/components/EntitySheet.tsx`:

1. import 에 `SceneMention` 추가:
```tsx
import type { Entity, EntityKind, Relationship, SceneMention, UpdateEntityInput } from "../lib/types";
```

2. Props 에 `onNavigate` 추가:
```tsx
interface Props {
  entityId: string | null;
  onClose: () => void;
  onSaved?: (entity: Entity) => void;
  onNavigate?: (nodeId: string) => void;
}
```
그리고 구조분해에 추가:
```tsx
export function EntitySheet({ entityId, onClose, onSaved, onNavigate }: Props) {
```

3. state 추가 (다른 useState 옆):
```tsx
const [scenes, setScenes] = useState<SceneMention[]>([]);
```

4. entityId 로드 시 scenes fetch. 기존에 entity/relationships 를 로드하는 useEffect (entityId 의존) 를 찾아, 그 안 또는 옆에 추가. relationships 와 동일 패턴 — entityId 가 바뀌면:
```tsx
useEffect(() => {
  if (!entityId) {
    setScenes([]);
    return;
  }
  let cancelled = false;
  entities.scenes(entityId)
    .then((s) => { if (!cancelled) setScenes(s); })
    .catch(() => { if (!cancelled) setScenes([]); });
  return () => { cancelled = true; };
}, [entityId]);
```
(`entities` 는 이미 `import { entities, relationships } from "../lib/rpc";` 로 import 됨.)

### Step 2: EntitySheet — 섹션 렌더

관계 섹션(`<section className="entity-section relations">`) 다음에 새 섹션 추가:

```tsx
<section className="entity-section">
  <h5>등장 씬 · {scenes.length}개</h5>
  {scenes.length === 0 ? (
    <p className="entity-empty">아직 등장한 씬이 없어요</p>
  ) : (
    <ul className="entity-scenes">
      {scenes.map((s) => (
        <li key={s.node_id}>
          <button
            type="button"
            className="entity-scene-link"
            onClick={() => onNavigate?.(s.node_id)}
          >
            {s.label}
          </button>
        </li>
      ))}
    </ul>
  )}
</section>
```

### Step 3: EntitySheet.css

`apps/desktop/src/components/EntitySheet.css` 끝에 추가:

```css
.entity-scenes {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}

.entity-scene-link {
  background: none;
  border: none;
  padding: 0.25rem 0;
  text-align: left;
  color: inherit;
  cursor: pointer;
  font-size: 0.85rem;
  width: 100%;
  border-radius: 4px;
}

.entity-scene-link:hover {
  color: #2980b9;
  text-decoration: underline;
}
```

### Step 4: Workspace — onNavigate 연결

`apps/desktop/src/routes/Workspace.tsx` 의 EntitySheet 마운트 (entitySheetId 분기) 에 `onNavigate` 추가:

```tsx
<EntitySheet
  entityId={entitySheetId}
  onClose={() => {
    setEntitySheetId(null);
    if (load) refreshMentioned(load.node.id);
    focusEditor();
  }}
  onSaved={() => {
    if (load) refreshMentioned(load.node.id);
  }}
  onNavigate={(nodeId) => {
    setEntitySheetId(null);
    navigateToNode({ id: nodeId } as NodeRow);
  }}
/>
```

`navigateToNode` 는 `TreeNode | NodeRow` 를 받아 `.id` 로 `nodes.get` 한다 (`"children" in target` 체크 후 NodeRow 면 그대로 leaf 로 사용). `{ id: nodeId } as NodeRow` 는 `children` 키가 없으므로 NodeRow 분기를 타고 `nodes.get(nodeId)` 실행 → 정상 이동. `NodeRow` 타입은 Workspace 가 이미 import 함 (`import type { NodeRow, ... }`).

### Step 5: 타입체크 + 엔진 회귀

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./...
```

기대: 타입체크 clean, 엔진 테스트 PASS.

### Step 6: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/components/EntitySheet.tsx apps/desktop/src/components/EntitySheet.css apps/desktop/src/routes/Workspace.tsx
git commit -m "feat(entity-sheet): 등장 씬 timeline section + navigate to scene"
```

## Context

Plan 23 Task 4 (final). EntitySheet 에 "등장 씬" 섹션을 더하고, 항목 클릭 시 시트 닫고 그 씬으로 이동. relationships fetch 패턴(entityId 의존 useEffect)을 그대로 따라 scenes fetch.

`navigateToNode` 는 `.id` 만 읽으므로 `{ id: nodeId } as NodeRow` 최소 객체로 호출 가능 (`"children" in target` false → NodeRow 분기 → `nodes.get(id)`).

Working directory: `/Users/changheonshin/workspace/myworks/linetta`. Branch: `main`. Never push. Never skip hooks.

## Before You Begin

EntitySheet 의 기존 entityId-의존 useEffect 위치와 relationships fetch 패턴을 먼저 확인. scenes fetch 를 동일하게.

## Report Format

- Status DONE | DONE_WITH_CONCERNS | BLOCKED
- 타입체크 / 엔진 테스트 결과
- 커밋 SHA
- 우려사항

---

## 통합 검증 (Task 4 직후 수동 스모크)

```bash
rm -rf /tmp/linetta-plan23 && LINETTA_HOME=/tmp/linetta-plan23 ./scripts/dev.sh
```

1. 다중 챕터 작품, 한 캐릭터를 여러 씬에 @멘션 (한 씬엔 2번).
2. 그 캐릭터 더블클릭 → 시트 `등장 씬 · N개` 섹션, 문서순 breadcrumb 목록. 2번 멘션한 씬은 1개로.
3. 항목 클릭 → 시트 닫히고 그 씬으로 이동 (에디터가 그 씬 표시).
4. 멘션 안 한 새 엔티티 시트 → "아직 등장한 씬이 없어요".
5. 회귀: 관계/속성/요약 섹션 정상, 멘션 더블클릭 정상.

통과 시:
```bash
git tag plan-23-entity-scenes-done
```

---

## Self-Review

**1. Spec 커버리지:**

| Spec 요구 | Task |
|---|---|
| MentionedNodeIDs (distinct + project_id) | Task 1 |
| entities.scenes 핸들러 (문서순 + breadcrumb + 중복제거) | Task 2 |
| breadcrumbLabel | Task 2 |
| main.go 등록 | Task 2 |
| 멘션 0 → [] | Task 2 (len(ids)==0 분기) |
| entity_id 빈 값 → InvalidParams | Task 2 |
| FE SceneMention 타입 + rpc client | Task 3 |
| EntitySheet 등장 씬 섹션 | Task 4 |
| 클릭 → 씬 이동 (onNavigate) | Task 4 |
| 빈 경우 메시지 | Task 4 |
| 수동 스모크 5 | Task 4 직후 |

모든 spec 요구 매핑.

**2. Placeholder scan:** Task 1/2 의 테스트는 "기존 fixture 패턴 확인" 지시 + 완전한 테스트 코드 제공 (실제 API 확인은 명시적 검증 단계). "TBD"/"TODO" 없음.

**3. Type 일관성:**
- `SceneMention { node_id, label }` — Task 2 Go struct (`NodeID json:"node_id"`, `Label json:"label"`) ↔ Task 3 TS interface (`node_id`, `label`) snake_case 일치 ↔ Task 4 EntitySheet 소비 (`s.node_id`, `s.label`).
- `MentionedNodeIDs(ctx, entityID) ([]string, string, error)` — Task 1 정의, Task 2 호출 (`ids, projectID, err`).
- `EntityScenes(mentions, nodes)` — Task 2 정의, main.go 등록.
- `entities.scenes(entityId)` — Task 3 클라이언트, Task 4 호출.
- `onNavigate(nodeId)` — Task 4 EntitySheet prop ↔ Workspace 핸들러.

체크 통과.

**4. 위험:**
- Task 2 테스트의 node.Repo CreateChild/CreateSibling 시그니처 + mention.Found 필드 — implementer 가 실제 API 확인 후 조정 (Before You Begin 명시).
- breadcrumbLabel 핸들러 로컬 복제 (ai 패키지 private 와 중복) — 첫 버전 수용.
- navigateToNode 최소 객체 호출 — `.id` 만 읽음, NodeRow 분기 확인됨.
