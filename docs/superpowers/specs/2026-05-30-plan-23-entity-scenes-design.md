# Plan 23 — 등장 씬 타임라인 Design Spec

## 목적

EntitySheet(캐릭터/장소 시트)에 "이 엔티티가 등장하는 씬 목록"을 문서 순서대로 보여준다. "이 캐릭터 어디 나왔더라" 를 즉시 파악하고, 항목 클릭으로 그 씬으로 이동한다. mention 데이터가 이미 있으므로 저위험.

## Goals

1. Engine: 엔티티가 멘션된 씬들을 **문서 순서(트리 DFS)** 로, **breadcrumb 경로**(`부 / 장 / 씬`)와 함께 반환하는 쿼리 + RPC.
2. 같은 씬에서 여러 번 멘션돼도 씬 1개로 (중복 제거).
3. EntitySheet 에 `등장 씬 · N개` 섹션 — breadcrumb 리스트.
4. 항목 클릭 → 시트 닫고 해당 씬으로 이동.
5. 멘션 0 → "아직 등장한 씬이 없어요".

## Non-Goals

- 요약/스니펫 표시 (breadcrumb 경로만).
- 최근순 등 다른 정렬 (문서 순서 고정).
- 페이지네이션 (등장 씬은 보통 수십 개 이내).
- mentions/nodes schema 변경.

---

## 1. Engine

### 1.1 `mention.Repo.ScenesForEntity`

`engine/internal/mention/repo.go` 에 추가:

```go
// SceneMention is one scene where an entity appears.
type SceneMention struct {
	NodeID string `json:"node_id"`
	Label  string `json:"label"` // breadcrumb path, e.g. "1부 / 1장 / 씬 3"
}

// ScenesForEntity returns the distinct scenes (leaf nodes) where the entity is
// mentioned, in document order (tree DFS). Label is the breadcrumb path.
func (r *Repo) ScenesForEntity(ctx context.Context, entityID string) ([]SceneMention, error) {
	// 1) project_id of the entity (via any mention row → node → project, or
	//    via entities table). Use entities table for robustness even when the
	//    entity has zero mentions.
	var projectID string
	err := r.s.DB().QueryRowContext(ctx,
		`SELECT project_id FROM entities WHERE id = ?`, entityID).Scan(&projectID)
	if err != nil {
		return nil, err
	}

	// 2) set of node_ids mentioning this entity (distinct).
	rows, err := r.s.DB().QueryContext(ctx,
		`SELECT DISTINCT node_id FROM mentions WHERE entity_id = ?`, entityID)
	if err != nil {
		return nil, err
	}
	mentioned := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		mentioned[id] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(mentioned) == 0 {
		return []SceneMention{}, nil
	}

	// 3) load all project nodes, build id→node + parent→children (ordinal order).
	all, err := r.nodes.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
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

	// 4) DFS in document order; collect mentioned leaves with breadcrumb.
	out := []SceneMention{}
	var walk func(parent string)
	walk = func(parent string) {
		for _, c := range children[parent] {
			if c.Kind == "leaf" && mentioned[c.ID] {
				out = append(out, SceneMention{NodeID: c.ID, Label: breadcrumbForNode(byID, c)})
			}
			walk(c.ID)
		}
	}
	walk("")
	return out, nil
}
```

**의존성**: `mention.Repo` 가 `node.Repo` 에 접근해야 함. 현재 mention.Repo 가 nodes 를 안 들고 있으면 — 두 가지 선택:
- (a) `mention.NewRepo` 에 `*node.Repo` 주입 (생성자 시그니처 변경 + main.go 호출처 변경)
- (b) ScenesForEntity 를 별도 위치(예: handler 가 mentions + nodes 둘 다 받아 조합)

**선택 (b) 권장** — mention.Repo 시그니처를 안 건드리고, 핸들러가 mentions 의 "node_id 목록" + nodes 의 트리를 조합. 즉:
- `mention.Repo.MentionedNodeIDs(ctx, entityID) ([]string, projectID string, error)` — distinct node_id 집합 + project_id 반환 (가벼운 쿼리)
- 핸들러가 `nodes.ListByProject` 로 트리 DFS + breadcrumb 조합 → `[]SceneMention`

이러면 mention.Repo 는 SQL 만, 트리 조합은 핸들러. `breadcrumbForNode` 헬퍼는 핸들러 패키지 또는 node 패키지에 둠 (Plan 16 `breadcrumbLabel` 이 ai 패키지에 private 이라 재활용 불가 — 핸들러용으로 작은 헬퍼 새로 작성).

**최종 채택 (b):**

`engine/internal/mention/repo.go`:
```go
// MentionedNodeIDs returns the distinct node_ids where entityID is mentioned,
// plus the entity's project_id (resolved from the entities table so it works
// even with zero mentions).
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

`SceneMention` 타입은 핸들러 패키지에 둔다 (RPC 결과 모양). breadcrumb DFS 도 핸들러.

### 1.2 핸들러 `entities.scenes`

`engine/internal/rpc/handlers/entities.go` (또는 mentions.go) 에:
```go
type entityScenesParams struct {
	EntityID string `json:"entity_id"`
}

type SceneMention struct {
	NodeID string `json:"node_id"`
	Label  string `json:"label"`
}

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

// breadcrumbLabel builds "부 / 장 / 씬" by walking parent_id up to root.
func breadcrumbLabel(byID map[string]node.Node, n node.Node) string {
	parts := []string{n.Label}
	cur := n
	for cur.ParentID != nil {
		p, ok := byID[*cur.ParentID]
		if !ok {
			break
		}
		parts = append([]string{p.Label}, parts...)
		cur = p
	}
	return strings.Join(parts, " / ")
}
```

`ListByProject` 가 ordinal 순으로 children 을 반환하는지 확인 — Plan 16 의 walk 가 그 순서를 신뢰하므로 동일하게 동작. (만약 미정렬이면 children[key] 를 ordinal 로 정렬하는 한 줄 추가.)

### 1.3 등록

`engine/cmd/linetta-engine/main.go` 의 entities 핸들러 옆:
```go
s.Handle("entities.scenes", handlers.EntityScenes(mentions, nodes))
```

### 1.4 테스트

`engine/internal/rpc/handlers/entities_test.go` (또는 신규 test 파일):
- 다층 트리(부/장/씬) + 엔티티 멘션 fixture
- `entities.scenes` 호출 → 문서순 SceneMention 배열
- 검증: (1) 문서 순서 (씬1, 씬2, ... DFS), (2) 같은 씬 중복 멘션 → 1개, (3) breadcrumb 정확 (`1부 / 1장 / 씬 1`), (4) 멘션 없는 엔티티 → `[]`, (5) entity_id 빈 값 → InvalidParams

`engine/internal/mention/repo_test.go`:
- `TestMentionedNodeIDs` — distinct node_ids + project_id (멘션 있음/없음 케이스)

---

## 2. Frontend

### 2.1 타입 + rpc

`apps/desktop/src/lib/types.ts`:
```ts
export interface SceneMention {
  node_id: string;
  label: string;
}
```

`apps/desktop/src/lib/rpc.ts` 의 entities 객체에:
```ts
scenes: (entityId: string) =>
  rpcCall<SceneMention[]>("entities.scenes", { entity_id: entityId }),
```

### 2.2 EntitySheet 섹션

`apps/desktop/src/components/EntitySheet.tsx`:
- Props 에 `onNavigate?: (nodeId: string) => void` 추가.
- state `const [scenes, setScenes] = useState<SceneMention[]>([])`.
- entityId 로드 시 `entities.scenes(entityId)` fetch (relationships 로드 패턴과 동일, useEffect).
- 새 섹션 (관계 섹션 옆):
```tsx
<section className="entity-section">
  <h5>등장 씬 · {scenes.length}개</h5>
  {scenes.length === 0 ? (
    <p className="entity-empty">아직 등장한 씬이 없어요</p>
  ) : (
    <ul className="entity-scenes">
      {scenes.map((s) => (
        <li key={s.node_id}>
          <button type="button" className="entity-scene-link" onClick={() => onNavigate?.(s.node_id)}>
            {s.label}
          </button>
        </li>
      ))}
    </ul>
  )}
</section>
```
- CSS `.entity-scenes` / `.entity-scene-link` — 가벼운 리스트/링크 스타일 (기존 `entity-section` 패턴).

### 2.3 Workspace 연결

`apps/desktop/src/routes/Workspace.tsx` 의 EntitySheet 마운트에 `onNavigate` 추가:
```tsx
<EntitySheet
  entityId={entitySheetId}
  onClose={() => { setEntitySheetId(null); if (load) refreshMentioned(load.node.id); focusEditor(); }}
  onSaved={() => { if (load) refreshMentioned(load.node.id); }}
  onNavigate={(nodeId) => {
    setEntitySheetId(null);
    navigateToNode({ id: nodeId } as NodeRow);
  }}
/>
```
(`navigateToNode` 는 NodeRow 의 `.id` 만 읽어 `nodes.get(id)` 하므로 최소 객체로 호출 가능. 타입 안전 위해 `as NodeRow` 캐스팅 또는 nodes.get 후 전달. 구현 task 에서 정확히.)

---

## 3. 에러 처리

| 상황 | 처리 |
|---|---|
| entity_id 빈 값 | InvalidParams |
| 멘션 0 | `[]` → "아직 등장한 씬이 없어요" |
| scenes fetch 실패 | EntitySheet error state 표시 (기존 패턴) 또는 빈 목록 + 콘솔. 시트의 다른 섹션은 정상. |
| 멘션된 노드가 트리에 없음 (정합성 깨짐) | DFS 에서 자연히 누락 (byID 에 없으면 walk 안 함) — 안전 |

---

## 4. 테스트 전략

- Engine TDD: ScenesForEntity 흐름 (문서순/중복제거/breadcrumb/빈/InvalidParams), MentionedNodeIDs 단위.
- FE: 타입체크 + 수동 스모크.

### 수동 스모크
1. 다중 챕터 작품에서 한 캐릭터를 여러 씬에 @멘션.
2. 그 캐릭터 시트 열기 → `등장 씬 · N개` 섹션에 문서순 breadcrumb 목록.
3. 같은 씬에 2번 멘션 → 목록엔 1개.
4. 항목 클릭 → 시트 닫히고 그 씬으로 이동.
5. 멘션 안 한 새 엔티티 → "아직 등장한 씬이 없어요".

통과 시 `git tag plan-23-entity-scenes-done`.

---

## 5. 파일 변경 요약

```
engine/internal/mention/repo.go            # +MentionedNodeIDs + (SceneMention은 handler에)
engine/internal/mention/repo_test.go       # +TestMentionedNodeIDs
engine/internal/rpc/handlers/entities.go   # +EntityScenes + SceneMention + breadcrumbLabel
engine/internal/rpc/handlers/entities_test.go # +TestEntityScenes
engine/cmd/linetta-engine/main.go          # +entities.scenes 등록
apps/desktop/src/lib/types.ts              # +SceneMention
apps/desktop/src/lib/rpc.ts                # +entities.scenes
apps/desktop/src/components/EntitySheet.tsx # +등장 씬 섹션 + onNavigate
apps/desktop/src/components/EntitySheet.css # +.entity-scenes / .entity-scene-link
apps/desktop/src/routes/Workspace.tsx      # EntitySheet onNavigate 연결
```

엔진 schema 변경 없음.

---

## 6. 위험 / 미해결

- **mention.Repo ↔ node.Repo 결합 회피**: 핸들러에서 조합 (선택 b). mention.Repo 는 MentionedNodeIDs 만.
- **children ordinal 정렬**: `ListByProject` 반환 순서가 ordinal 인지 확인. 아니면 핸들러 DFS 에서 children 을 ordinal 정렬.
- **breadcrumbLabel 중복**: ai 패키지에 동일 로직 있으나 private. 핸들러에 작은 복제 — DRY 위반이나 패키지 경계상 수용 (또는 node 패키지에 공용 헬퍼 — 첫 버전은 핸들러 로컬).
- **navigateToNode 최소 객체 호출**: `.id` 만 읽으므로 안전하나, 더 견고히 하려면 핸들러 onNavigate 가 nodes.get 후 전달.
