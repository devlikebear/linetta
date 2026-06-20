# 씬 편집기 히스토리 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 씬 편집기에 컴패니언 적용 전 체크포인트와 아이들(2분) 자동 체크포인트를 추가하고, 죽은 `ai-replace` reason을 제거해 스냅샷 reason 체계를 3개로 정리한다.

**Architecture:** 기존 `node_snapshots` 인프라를 확장한다. (1) 스냅샷 reason을 `manual`/`autosave`/`companion-before`로 정리. (2) 컴패니언 `set_scene_text`가 적용 직전 원본을 `companion-before`로 스냅샷. (3) 서버측 60초 간격 autosave를 제거하고, 자동 스냅샷 트리거를 프론트 2분 아이들 타이머 + 신규 `snapshots.create_auto` RPC로 일원화. 모든 자동/컴패니언 스냅샷은 내용 무변경 시 생성 skip(`CreateIfChanged`).

**Tech Stack:** Go (engine, `modernc.org/sqlite` 단일 커넥션), React + TypeScript + Vite (desktop renderer), TipTap 에디터, Tauri 셸. 테스트: Go `testing`, frontend `vitest`.

## Global Constraints

- SQLite는 **단일 커넥션**(`SetMaxOpenConns(1)`). 열린 `Rows`를 들고 다시 쿼리하지 말 것 — 데드락. (관련 메모리: linetta-sqlite-single-conn)
- 엔진 코드는 **Windows 크로스 컴파일** 가능해야 함. 플랫폼 syscall 금지 (이 작업은 순수 Go/SQL이라 해당 없음, 새 의존성 추가 금지).
- 스냅샷 reason은 `ValidReason()` 허용목록 기반. **reason 추가/제거 시 VersionSheet 라벨맵·i18n 키도 같은 변경에서 수정.** 라벨 없는 reason 금지.
- TipTap 세션 undo/redo는 그대로 둔다(영속화 신규 작업 없음). 본 플랜 범위 밖.
- 커밋 메시지는 기존 컨벤션(`feat(...)`, `refactor(...)`, `test(...)`) 사용.

---

## File Structure

**엔진 (Go):**
- `engine/internal/snapshot/snapshot.go` — reason 상수/`ValidReason` 정리
- `engine/internal/snapshot/repo.go` — `CreateIfChanged` 추가, `LatestAutosaveTime` 제거
- `engine/internal/snapshot/retention.go` — 주석만 갱신(동작 유지)
- `engine/internal/rpc/handlers/snapshots.go` — `CreateAutoSnapshot` 핸들러 추가
- `engine/internal/rpc/handlers/nodes.go` — `UpdateNodeContent`에서 autosave 블록·`AutosaveIntervalMillis` 제거
- `engine/internal/companion/companion.go` — `Service.snaps` 필드 + `WithSnapshots` 빌더
- `engine/internal/companion/tools.go` — `set_scene_text`에 `companion-before` 스냅샷
- `engine/cmd/linetta-engine/main.go` — 신규 핸들러 등록·companion 와이어링·`update_content` 시그니처 갱신

**프론트 (TypeScript):**
- `apps/desktop/src/lib/types.ts` — `SnapshotReason` 갱신
- `apps/desktop/src/lib/rpc.ts` — `snapshots.createAuto` 래퍼
- `apps/desktop/src/lib/i18n.tsx` — `version.reason.aiReplace` → `version.reason.companionBefore` (3개 로케일)
- `apps/desktop/src/components/VersionSheet.tsx` — reasonLabel 갱신
- `apps/desktop/src/hooks/useIdleTimer.ts` — 신규 아이들 타이머 훅
- `apps/desktop/src/routes/Workspace.tsx` — 아이들 체크포인트 와이어링

---

## Task 1: 스냅샷 reason 체계 정리 (ai-replace 제거 → companion-before)

**Files:**
- Modify: `engine/internal/snapshot/snapshot.go`
- Modify: `engine/internal/snapshot/retention.go:8` (주석)
- Test: `engine/internal/snapshot/repo_test.go:111,120`, `engine/internal/snapshot/retention_test.go:68` (기존 `ReasonAIReplace` 참조 교체)

**Interfaces:**
- Produces: `snapshot.ReasonCompanionBefore = "companion-before"` 상수. `snapshot.ReasonAIReplace` 삭제. `ValidReason("companion-before") == true`, `ValidReason("ai-replace") == false`.

- [ ] **Step 1: 기존 ai-replace 참조 위치 확인**

Run: `cd engine && grep -rn "ReasonAIReplace" internal/`
Expected: `snapshot.go:18`, `repo_test.go` (2곳), `retention_test.go` (1~2곳). retention.go는 주석만.

- [ ] **Step 2: reason 상수와 ValidReason 교체 (snapshot.go)**

`engine/internal/snapshot/snapshot.go` 1-2행 패키지 주석과 상수 블록을 교체:

```go
// Package snapshot persists node_snapshots — point-in-time copies of a leaf
// node's content_doc, tagged with a reason (manual, autosave, companion-before).
package snapshot
```

```go
// Reasons.
const (
	ReasonManual          = "manual"
	ReasonAutosave        = "autosave"
	ReasonCompanionBefore = "companion-before"
)

func ValidReason(reason string) bool {
	return reason == ReasonManual || reason == ReasonAutosave || reason == ReasonCompanionBefore
}
```

- [ ] **Step 3: 테스트의 ReasonAIReplace 참조를 ReasonCompanionBefore로 교체**

`repo_test.go`와 `retention_test.go`에서 `ReasonAIReplace` → `ReasonCompanionBefore` 치환 (companion-before도 비-autosave·영구 보존 reason이라 기존 테스트 의도 그대로 유지됨):

Run: `cd engine && grep -rln "ReasonAIReplace" internal/snapshot/ | xargs sed -i '' 's/ReasonAIReplace/ReasonCompanionBefore/g'`

- [ ] **Step 4: retention.go 주석 갱신**

`engine/internal/snapshot/retention.go:8`의 주석 한 줄을 교체:

```go
// Thin enforces autosave retention. Manual and companion-before snapshots are
// never touched. Autosaves:
```

- [ ] **Step 5: 패키지 빌드·테스트 통과 확인**

Run: `cd engine && go build ./... && go test ./internal/snapshot/...`
Expected: PASS, 컴파일 에러 없음.

- [ ] **Step 6: Commit**

```bash
git add engine/internal/snapshot/
git commit -m "refactor(snapshot): replace dormant ai-replace reason with companion-before"
```

---

## Task 2: CreateIfChanged 리포지토리 메서드 (내용 중복 제거)

**Files:**
- Modify: `engine/internal/snapshot/repo.go`
- Test: `engine/internal/snapshot/repo_test.go`

**Interfaces:**
- Consumes: `snapshot.ReasonCompanionBefore` (Task 1), 기존 `(*Repo).Create`, `(*Repo).LatestForNode`, `ErrNotFound`.
- Produces: `func (r *Repo) CreateIfChanged(ctx context.Context, nodeID, doc, reason string, now int64) (Snapshot, bool, error)` — 노드의 최신 스냅샷(아무 reason) content_doc이 `doc`과 정확히 같으면 `(Snapshot{}, false, nil)` 반환(생성 skip), 아니면 `Create` 후 `(snap, true, nil)`.

- [ ] **Step 1: 실패하는 테스트 작성 (repo_test.go에 추가)**

```go
func TestCreateIfChanged_skipsDuplicate(t *testing.T) {
	ctx := context.Background()
	r, nodeID := newRepoWithNode(t) // 기존 헬퍼 패턴 (repo_test 상단 참고)

	first, created, err := r.CreateIfChanged(ctx, nodeID, `{"v":1}`, ReasonAutosave, 1000)
	if err != nil || !created {
		t.Fatalf("first CreateIfChanged: created=%v err=%v", created, err)
	}
	if first.ID == "" {
		t.Fatalf("expected a created snapshot")
	}

	// 동일 내용 → skip.
	_, created2, err := r.CreateIfChanged(ctx, nodeID, `{"v":1}`, ReasonAutosave, 2000)
	if err != nil {
		t.Fatalf("second CreateIfChanged: %v", err)
	}
	if created2 {
		t.Errorf("expected skip on identical content, got created=true")
	}

	// 내용 변경 → 생성.
	_, created3, err := r.CreateIfChanged(ctx, nodeID, `{"v":2}`, ReasonAutosave, 3000)
	if err != nil || !created3 {
		t.Errorf("expected create on changed content: created=%v err=%v", created3, err)
	}
}
```

> 참고: `newRepoWithNode(t)` 헬퍼가 repo_test.go에 없으면, 같은 파일의 기존 테스트(예: `TestLatestForNode` 부근)가 노드를 만드는 방식을 그대로 따라 인라인으로 store/node를 세팅한다.

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd engine && go test ./internal/snapshot/ -run TestCreateIfChanged_skipsDuplicate`
Expected: FAIL — `r.CreateIfChanged undefined`.

- [ ] **Step 3: CreateIfChanged 구현 (repo.go의 Create 바로 아래에 추가)**

```go
// CreateIfChanged inserts a snapshot only when the node's most recent snapshot
// (any reason) differs from doc. Returns (snap, true, nil) when created, or
// (Snapshot{}, false, nil) when the latest snapshot already holds doc. This is
// the content-dedup guard shared by autosave and companion-before checkpoints.
func (r *Repo) CreateIfChanged(ctx context.Context, nodeID, doc, reason string, now int64) (Snapshot, bool, error) {
	latest, err := r.LatestForNode(ctx, nodeID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Snapshot{}, false, err
	}
	if err == nil && latest.ContentDoc == doc {
		return Snapshot{}, false, nil
	}
	snap, err := r.Create(ctx, nodeID, doc, reason, now)
	if err != nil {
		return Snapshot{}, false, err
	}
	return snap, true, nil
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd engine && go test ./internal/snapshot/ -run TestCreateIfChanged_skipsDuplicate`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add engine/internal/snapshot/repo.go engine/internal/snapshot/repo_test.go
git commit -m "feat(snapshot): add CreateIfChanged content-dedup helper"
```

---

## Task 3: snapshots.create_auto RPC 핸들러

**Files:**
- Modify: `engine/internal/rpc/handlers/snapshots.go`
- Modify: `engine/cmd/linetta-engine/main.go:220` 부근 (핸들러 등록)
- Test: `engine/internal/rpc/handlers/snapshots_test.go`

**Interfaces:**
- Consumes: `(*snapshot.Repo).CreateIfChanged` (Task 2), `snapshot.ReasonAutosave`, 기존 `handlers.Clock`.
- Produces: `handlers.CreateAutoSnapshot(snaps *snapshot.Repo, now Clock) rpc.Handler` — 메서드 `snapshots.create_auto`, params `{ "node_id": string, "doc": string }`. 응답: 생성 시 `Snapshot` JSON, skip 시 `{"skipped":true}`.

- [ ] **Step 1: 실패하는 테스트 작성 (snapshots_test.go에 추가)**

```go
func TestCreateAutoSnapshotHandler_dedups(t *testing.T) {
	f := newSnapFixture(t) // 기존 snapshots_test.go 헬퍼 (f.snaps, f.nID 제공)
	h := CreateAutoSnapshot(f.snaps, func() int64 { return 5000 })

	doc := `{"v":1}`
	raw, _ := json.Marshal(map[string]string{"node_id": f.nID, "doc": doc})

	if _, err := h(context.Background(), raw); err != nil {
		t.Fatalf("first create_auto: %v", err)
	}
	// 동일 내용 재호출 → 새 스냅샷 없음.
	if _, err := h(context.Background(), raw); err != nil {
		t.Fatalf("second create_auto: %v", err)
	}
	entries, _ := f.snaps.ListForNode(context.Background(), f.nID)
	autoCount := 0
	for _, e := range entries {
		if e.Reason == "autosave" {
			autoCount++
		}
	}
	if autoCount != 1 {
		t.Errorf("expected exactly 1 autosave snapshot after dedup, got %d", autoCount)
	}
}
```

> `newSnapFixture(t)`의 실제 이름은 snapshots_test.go 상단의 기존 헬퍼를 따른다(`TestCreateManualSnapshotHandler`가 쓰는 것과 동일). 필드명이 다르면 거기에 맞춘다.

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd engine && go test ./internal/rpc/handlers/ -run TestCreateAutoSnapshotHandler_dedups`
Expected: FAIL — `CreateAutoSnapshot undefined`.

- [ ] **Step 3: 핸들러 구현 (snapshots.go에 CreateManualSnapshot 아래 추가)**

```go
type autoSnapshotParams struct {
	NodeID string `json:"node_id"`
	Doc    string `json:"doc"`
}

// CreateAutoSnapshot returns a handler for snapshots.create_auto. It records an
// `autosave` snapshot only when the content changed since the node's last
// snapshot (idle-triggered from the renderer). Skips are reported, not errors.
func CreateAutoSnapshot(snaps *snapshot.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p autoSnapshotParams
		if err := json.Unmarshal(params, &p); err != nil || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id and doc required"}
		}
		got, created, err := snaps.CreateIfChanged(ctx, p.NodeID, p.Doc, snapshot.ReasonAutosave, now())
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if !created {
			return json.RawMessage(`{"skipped":true}`), nil
		}
		return json.Marshal(got)
	}
}
```

- [ ] **Step 4: 핸들러 등록 (main.go)**

`engine/cmd/linetta-engine/main.go`에서 `snapshots.create_manual` 등록(220행) 바로 아래에 추가:

```go
	s.Handle("snapshots.create_auto", handlers.CreateAutoSnapshot(snaps, clock))
```

- [ ] **Step 5: 테스트·빌드 통과 확인**

Run: `cd engine && go build ./... && go test ./internal/rpc/handlers/ -run TestCreateAutoSnapshotHandler_dedups`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add engine/internal/rpc/handlers/snapshots.go engine/internal/rpc/handlers/snapshots_test.go engine/cmd/linetta-engine/main.go
git commit -m "feat(snapshot): add snapshots.create_auto idle-checkpoint RPC"
```

---

## Task 4: 서버측 60초 간격 autosave 제거

**Files:**
- Modify: `engine/internal/rpc/handlers/nodes.go` (autosave 블록·`AutosaveIntervalMillis` 제거, `snaps` 파라미터 제거)
- Modify: `engine/internal/snapshot/repo.go` (`LatestAutosaveTime` 제거)
- Modify: `engine/cmd/linetta-engine/main.go:215` (`update_content` 등록 시그니처)
- Test: `engine/internal/rpc/handlers/nodes_test.go` (autosave 테스트 2개 제거, 픽스처 갱신), `engine/internal/snapshot/repo_test.go` (`TestLatestAutosaveTime` 제거)

**Interfaces:**
- Produces: `handlers.UpdateNodeContent(nodes *node.Repo, now Clock, postUpdate func(nodeID string)) rpc.Handler` — `snaps` 파라미터 삭제됨. `AutosaveIntervalMillis` 상수와 `(*snapshot.Repo).LatestAutosaveTime` 삭제됨.

- [ ] **Step 1: 자동 스냅샷 의존 테스트 제거**

`engine/internal/rpc/handlers/nodes_test.go`에서 다음 두 테스트 함수를 삭제: `TestUpdateNodeContentHandler_createsAutosaveSnapshotOnFirstSave`, `TestUpdateNodeContentHandler_noSnapshotWithin60s`. (자동 스냅샷은 Task 3의 `create_auto`로 이동했으므로 여기서 검증하지 않는다.)

`engine/internal/snapshot/repo_test.go`에서 `TestLatestAutosaveTime` 함수를 삭제.

- [ ] **Step 2: UpdateNodeContent에서 autosave 블록·snaps 파라미터 제거 (nodes.go)**

`AutosaveIntervalMillis` 상수 블록(13-15행)을 삭제하고, `UpdateNodeContent`를 다음으로 교체:

```go
// UpdateNodeContent returns a handler for nodes.update_content. Version
// snapshots are no longer created here — autosave checkpoints are idle-triggered
// from the renderer via snapshots.create_auto.
func UpdateNodeContent(nodes *node.Repo, now Clock, postUpdate func(nodeID string)) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p updateContentParams
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id and doc required"}
		}
		t := now()
		if err := nodes.UpdateContent(ctx, p.ID, p.Doc, t); err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node not found"}
			}
			if errors.Is(err, node.ErrContentOnContainer) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		got, err := nodes.Get(ctx, p.ID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if postUpdate != nil {
			postUpdate(p.ID)
		}
		return json.Marshal(got)
	}
}
```

`snapshot` import가 nodes.go의 다른 핸들러에서 안 쓰이면 import도 제거(goimports가 처리). nodes_test.go 픽스처에서 `snaps` 필드가 `UpdateNodeContent` 호출에만 쓰였다면 호출부 `UpdateNodeContent(f.nodes, f.snaps, ...)` → `UpdateNodeContent(f.nodes, ...)`로 갱신.

- [ ] **Step 3: LatestAutosaveTime 제거 (repo.go)**

`repo.go`에서 `LatestAutosaveTime` 메서드(173-187행) 전체를 삭제. `database/sql`·`strings` 등 import가 다른 곳에서 여전히 쓰이는지 `go build`로 확인.

- [ ] **Step 4: main.go update_content 등록 갱신**

`engine/cmd/linetta-engine/main.go:215`:

```go
	s.Handle("nodes.update_content", handlers.UpdateNodeContent(nodes, clock, summ.Enqueue))
```

- [ ] **Step 5: 빌드·전체 엔진 테스트 통과 확인**

Run: `cd engine && go build ./... && go test ./...`
Expected: PASS. (autosave 관련 컴파일 잔재 없음.)

- [ ] **Step 6: Commit**

```bash
git add engine/
git commit -m "refactor(snapshot): drop server-side interval autosave in favor of idle checkpoints"
```

---

## Task 5: 컴패니언 적용 전 체크포인트 (companion-before)

**Files:**
- Modify: `engine/internal/companion/companion.go` (`Service.snaps` 필드 + `WithSnapshots`)
- Modify: `engine/internal/companion/tools.go:285-295` (`set_scene_text` 경로)
- Modify: `engine/cmd/linetta-engine/main.go:177-185` 부근 (companion 빌더에 `.WithSnapshots(snaps)`)
- Test: `engine/internal/companion/tools_test.go`

**Interfaces:**
- Consumes: `(*snapshot.Repo).CreateIfChanged` (Task 2), `snapshot.ReasonCompanionBefore` (Task 1).
- Produces: `(*companion.Service).WithSnapshots(snaps *snapshot.Repo) *Service` (체이닝용 `*Service` 반환). `Service.snaps` 필드.

- [ ] **Step 1: 실패하는 테스트 작성 (tools_test.go에 추가)**

기존 `newToolSvc(t)` 헬퍼(38-60행)의 `svc := &Service{...}` 리터럴에 `snaps` 필드를 추가한다:

```go
	svc := &Service{
		projects: projects, threads: threads, entities: entities,
		relationships: rels, facts: facts, plot: pb, nodes: nodes, beats: beats,
		snaps: snapshot.NewRepo(st),
		src:   toolConfigSource{},
	}
```

(파일 상단 import에 `"github.com/devlikebear/linetta/engine/internal/snapshot"` 추가.)

그리고 새 테스트:

```go
func TestLinettaApplyOpsToolSnapshotsBeforeSceneText(t *testing.T) {
	ctx := context.Background()
	svc, projectID, nodeID := newToolSvc(t)

	// 원본 본문을 먼저 저장한다.
	originalDoc, err := plainTextToTiptapDoc("원본 본문")
	if err != nil {
		t.Fatalf("plainTextToTiptapDoc: %v", err)
	}
	if err := svc.nodes.UpdateContent(ctx, nodeID, originalDoc, 500); err != nil {
		t.Fatalf("seed content: %v", err)
	}

	// set_scene_text 적용.
	p := Proposal{Ops: []Op{{Type: "set_scene_text", Text: "AI가 바꾼 본문"}}}
	res := svc.ApplyOps(ctx, projectID, nodeID, p, func() int64 { return 1000 })
	if res.Applied != 1 || len(res.Failures) != 0 {
		t.Fatalf("apply set_scene_text failed: %+v", res)
	}

	// companion-before 스냅샷이 원본 내용을 담아야 한다.
	snap, err := svc.snaps.LatestForNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("LatestForNode: %v", err)
	}
	if snap.Reason != snapshot.ReasonCompanionBefore {
		t.Errorf("reason = %q, want companion-before", snap.Reason)
	}
	if snap.ContentDoc != originalDoc {
		t.Errorf("snapshot did not capture pre-AI content; got %q", snap.ContentDoc)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd engine && go test ./internal/companion/ -run TestLinettaApplyOpsToolSnapshotsBeforeSceneText`
Expected: FAIL — `svc.snaps undefined` (필드 미존재).

- [ ] **Step 3: Service에 snaps 필드 + WithSnapshots 추가 (companion.go)**

`type Service struct { ... }`에 필드 추가:

```go
	snaps *snapshot.Repo
```

기존 `WithHistory`/`WithReferences` 빌더 옆에 추가(같은 패턴):

```go
// WithSnapshots wires the node-snapshot repo so companion edits can record a
// companion-before checkpoint before mutating scene text.
func (s *Service) WithSnapshots(snaps *snapshot.Repo) *Service {
	s.snaps = snaps
	return s
}
```

companion.go import에 `"github.com/devlikebear/linetta/engine/internal/snapshot"` 추가.

- [ ] **Step 4: set_scene_text에 companion-before 스냅샷 삽입 (tools.go)**

`tools.go` `set_scene_text` 케이스에서 `before`를 읽은 직후(288행 뒤), `UpdateContent` 호출(293행) **이전**에 삽입:

```go
		before, err := s.nodes.Get(ctx, *targetNodeID)
		if err != nil {
			return err
		}
		// 적용 전 원본을 companion-before 체크포인트로 기록 (씬 텍스트만).
		// 직전 스냅샷과 동일하면 skip. snaps 미주입(테스트 등) 시 생략.
		if s.snaps != nil {
			beforeDoc := ""
			if before.ContentDoc != nil {
				beforeDoc = *before.ContentDoc
			}
			if _, _, err := s.snaps.CreateIfChanged(ctx, *targetNodeID, beforeDoc, snapshot.ReasonCompanionBefore, now()); err != nil {
				return fmt.Errorf("companion-before snapshot: %w", err)
			}
		}
		doc, err := plainTextToTiptapDoc(op.Text)
```

tools.go import에 `snapshot` 패키지가 없으면 추가.

- [ ] **Step 5: 테스트 통과 확인**

Run: `cd engine && go test ./internal/companion/ -run TestLinettaApplyOpsToolSnapshotsBeforeSceneText`
Expected: PASS.

- [ ] **Step 6: main.go companion 빌더에 WithSnapshots 와이어링**

`engine/cmd/linetta-engine/main.go`의 companion 빌더 체인(184-185행 `.WithHistory(...).WithReferences(...)`)에 추가:

```go
		WithSnapshots(snaps).
```

- [ ] **Step 7: 전체 엔진 빌드·테스트 통과 확인**

Run: `cd engine && go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add engine/
git commit -m "feat(companion): snapshot scene text as companion-before on set_scene_text"
```

---

## Task 6: 프론트 타입 + create_auto RPC 래퍼

**Files:**
- Modify: `apps/desktop/src/lib/types.ts:225,549`
- Modify: `apps/desktop/src/lib/rpc.ts:187-194`

**Interfaces:**
- Produces: `SnapshotReason = "manual" | "autosave" | "companion-before"`. `snapshots.createAuto(nodeId: string, doc: string): Promise<Snapshot | { skipped: true }>`.

- [ ] **Step 1: SnapshotReason 타입 갱신 (types.ts)**

`types.ts:225`:

```ts
export type SnapshotReason = "manual" | "autosave" | "companion-before";
```

`types.ts:549`의 `SnapshotEntry.reason` 인라인 유니온도 동일하게:

```ts
  reason: "manual" | "autosave" | "companion-before";
```

- [ ] **Step 2: createAuto 래퍼 추가 (rpc.ts)**

`rpc.ts`의 `snapshots` 객체(187행)에 추가:

```ts
  createAuto: (nodeId: string, doc: string) =>
    rpcCall<Snapshot | { skipped: true }>("snapshots.create_auto", { node_id: nodeId, doc }),
```

- [ ] **Step 3: 타입체크 통과 확인**

Run: `cd apps/desktop && npx tsc --noEmit`
Expected: 신규 에러 없음. (VersionSheet의 `ai-replace` 비교가 에러로 표면화될 수 있음 → Task 7에서 해결. 이 시점에 tsc가 `ai-replace` 관련 에러를 내면 Task 7과 함께 커밋해도 됨.)

- [ ] **Step 4: Commit**

```bash
git add apps/desktop/src/lib/types.ts apps/desktop/src/lib/rpc.ts
git commit -m "feat(desktop): add companion-before reason and snapshots.createAuto wrapper"
```

---

## Task 7: VersionSheet 라벨 + i18n 키 정리

**Files:**
- Modify: `apps/desktop/src/components/VersionSheet.tsx:46,65`
- Modify: `apps/desktop/src/lib/i18n.tsx` (3개 로케일: 778, 1688, 2598행 부근)

**Interfaces:**
- Consumes: `SnapshotReason` (Task 6).
- Produces: i18n 키 `version.reason.companionBefore` (ko/en/ja). `version.reason.aiReplace` 제거.

- [ ] **Step 1: i18n 키 교체 (i18n.tsx, 3개 로케일)**

각 로케일에서 `version.reason.aiReplace` 라인을 `companionBefore`로 교체(의미는 "AI 적용 전"):

```ts
// ko (778행 부근)
    "version.reason.companionBefore": "AI 적용 전",
// en (1688행 부근)
    "version.reason.companionBefore": "Before AI edit",
// ja (2598행 부근)
    "version.reason.companionBefore": "AI適用前",
```

- [ ] **Step 2: VersionSheet reasonLabel 갱신 (VersionSheet.tsx)**

`VersionSheet.tsx:46`:

```tsx
    if (reason === "companion-before") return t("version.reason.companionBefore");
    return t(`version.reason.${reason}`);
```

`VersionSheet.tsx:65`의 그룹핑 주석을 갱신(동작은 그대로 — 비-autosave는 모두 "주요" 그룹에 들어가므로 companion-before가 자동 포함됨):

```tsx
  // Group: "주요" (manual + companion-before) on top, then autosaves by YYYY-MM-DD.
```

- [ ] **Step 3: 타입체크·기존 테스트 통과 확인**

Run: `cd apps/desktop && npx tsc --noEmit && npx vitest run`
Expected: PASS, `ai-replace` 잔재 에러 없음.

- [ ] **Step 4: Commit**

```bash
git add apps/desktop/src/components/VersionSheet.tsx apps/desktop/src/lib/i18n.tsx
git commit -m "feat(desktop): label companion-before snapshots in version sheet"
```

---

## Task 8: useIdleTimer 훅

**Files:**
- Create: `apps/desktop/src/hooks/useIdleTimer.ts`
- Test: `apps/desktop/src/hooks/useIdleTimer.test.ts`

**Interfaces:**
- Produces: `useIdleTimer(idleMs: number, onIdle: () => void): { markActivity: () => void; cancel: () => void }`. `markActivity()` 호출 시 타이머를 리셋, 마지막 호출 후 `idleMs` 동안 추가 호출이 없으면 `onIdle`을 1회 실행. `cancel()`은 대기 중 타이머를 취소. 언마운트 시 자동 정리.

- [ ] **Step 1: 실패하는 테스트 작성 (useIdleTimer.test.ts)**

```ts
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useIdleTimer } from "./useIdleTimer";

describe("useIdleTimer", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("fires onIdle after idleMs of no activity", () => {
    const onIdle = vi.fn();
    const { result } = renderHook(() => useIdleTimer(2000, onIdle));
    act(() => result.current.markActivity());
    act(() => vi.advanceTimersByTime(1999));
    expect(onIdle).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(1));
    expect(onIdle).toHaveBeenCalledTimes(1);
  });

  it("resets the timer on each markActivity", () => {
    const onIdle = vi.fn();
    const { result } = renderHook(() => useIdleTimer(2000, onIdle));
    act(() => result.current.markActivity());
    act(() => vi.advanceTimersByTime(1500));
    act(() => result.current.markActivity()); // reset
    act(() => vi.advanceTimersByTime(1500));
    expect(onIdle).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(500));
    expect(onIdle).toHaveBeenCalledTimes(1);
  });

  it("cancel() prevents a pending fire", () => {
    const onIdle = vi.fn();
    const { result } = renderHook(() => useIdleTimer(2000, onIdle));
    act(() => result.current.markActivity());
    act(() => result.current.cancel());
    act(() => vi.advanceTimersByTime(3000));
    expect(onIdle).not.toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd apps/desktop && npx vitest run src/hooks/useIdleTimer.test.ts`
Expected: FAIL — 모듈 없음.

- [ ] **Step 3: 훅 구현 (useIdleTimer.ts)**

```ts
import { useCallback, useEffect, useRef } from "react";

/**
 * Calls onIdle once after idleMs has elapsed since the last markActivity().
 * Each markActivity() resets the countdown. Latest onIdle is always used.
 */
export function useIdleTimer(idleMs: number, onIdle: () => void) {
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const onIdleRef = useRef(onIdle);
  onIdleRef.current = onIdle;

  const cancel = useCallback(() => {
    if (timerRef.current !== null) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  const markActivity = useCallback(() => {
    if (timerRef.current !== null) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => {
      timerRef.current = null;
      onIdleRef.current();
    }, idleMs);
  }, [idleMs]);

  useEffect(() => cancel, [cancel]);

  return { markActivity, cancel };
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd apps/desktop && npx vitest run src/hooks/useIdleTimer.test.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add apps/desktop/src/hooks/useIdleTimer.ts apps/desktop/src/hooks/useIdleTimer.test.ts
git commit -m "feat(desktop): add useIdleTimer hook"
```

---

## Task 9: Workspace에 아이들 체크포인트 와이어링

**Files:**
- Modify: `apps/desktop/src/routes/Workspace.tsx` (상수, dirty ref, 핸들러, 에디터 onChange 2곳: ~1604, ~1884)

**Interfaces:**
- Consumes: `useIdleTimer` (Task 8), `snapshots.createAuto` (Task 6), 기존 `loadRef`, `editorRef`, `debouncedSave`.

- [ ] **Step 1: 상수 + import 추가 (Workspace.tsx 상단)**

기존 `SAVE_DEBOUNCE_MS` 상수 부근에 추가:

```ts
const IDLE_CHECKPOINT_MS = 120_000; // 2분간 입력이 멈추면 자동 체크포인트
```

import에 `import { useIdleTimer } from "../hooks/useIdleTimer";` 추가 (경로는 Workspace.tsx 기준 상대경로 확인).

- [ ] **Step 2: dirty ref + 아이들 핸들러 추가 (saveNow/debouncedSave 정의 부근, ~744행)**

```ts
  const idleDirtyRef = useRef(false);
  const handleIdleCheckpoint = useCallback(async () => {
    if (!idleDirtyRef.current) return;
    const currentLoad = loadRef.current;
    const doc = editorRef.current?.getDoc();
    if (!currentLoad || !doc) return;
    idleDirtyRef.current = false;
    try {
      await snapshots.createAuto(currentLoad.node.id, JSON.stringify(doc));
    } catch {
      /* benign — autosave checkpoint is best-effort */
    }
  }, []);
  const { markActivity } = useIdleTimer(IDLE_CHECKPOINT_MS, handleIdleCheckpoint);
```

> `snapshots`는 이미 `rpc.ts`에서 import되어 있음(`handleManualSave`가 `snapshots.createManual` 사용). 없으면 import 추가.

- [ ] **Step 3: 에디터 onChange에서 markActivity + dirty 마킹 (~1604, ~1884행)**

두 곳의 에디터 onChange 콜백(메인 에디터·Zen 모드)에서 `debouncedSave(doc)` 호출 옆에 추가:

```ts
            onChange={(doc) => {
              debouncedSave(doc);
              idleDirtyRef.current = true;
              markActivity();
            }}
```

(기존 onChange 본문에 다른 호출이 있으면 유지하고 두 줄만 덧붙인다.)

- [ ] **Step 4: 타입체크 + 전체 프론트 테스트 통과 확인**

Run: `cd apps/desktop && npx tsc --noEmit && npx vitest run`
Expected: PASS.

- [ ] **Step 5: 빌드 확인**

Run: `cd apps/desktop && npm run build`
Expected: 빌드 성공.

- [ ] **Step 6: Commit**

```bash
git add apps/desktop/src/routes/Workspace.tsx
git commit -m "feat(desktop): create idle autosave checkpoint after 2 min of inactivity"
```

---

## Task 10: 통합 검증 (수동)

**Files:** 없음 (실행 검증)

- [ ] **Step 1: 엔진 + 프론트 전체 테스트**

Run: `cd engine && go test ./... && cd ../apps/desktop && npx vitest run`
Expected: 모두 PASS.

- [ ] **Step 2: 앱 실행 수동 시나리오** (`/run` 또는 `npm run tauri dev`)

1. 씬을 편집하고 800ms 후 저장됨 확인 (saveStatus "saved").
2. 입력을 멈추고 2분 대기 → 버전 시트(VersionSheet)에 "자동 저장" 항목 1개 추가됨.
3. 다시 같은 내용 그대로 2분 대기 → 중복 항목 생성 안 됨 (dedup).
4. 컴패니언으로 씬 텍스트를 바꾸는 제안을 Apply → 버전 시트에 "AI 적용 전" 항목 생성, 그 내용이 적용 전 원본임.
5. "AI 적용 전" 항목 복원(restore) → 씬이 원본으로 되돌아오고, 복원 직전 상태가 "수동 저장"으로 보존됨(기존 restore 동작).
6. Ctrl+Z / Ctrl+Shift+Z 세션 undo/redo 동작 확인.

- [ ] **Step 3: 검증 결과를 사실대로 보고** (실패 시 출력과 함께)

---

## Self-Review (작성자 점검 결과)

**1. Spec coverage:**
- 롤백 → 기존 `snapshots.restore` 유지 + VersionSheet 라벨 정리(Task 7) + 통합검증(Task 10). ✓
- 컴패니언 전 체크포인트 → Task 5. ✓ ("후"는 스펙대로 드롭.)
- 아이들 2분 자동 체크포인트 → Task 3(RPC)+Task 8(훅)+Task 9(와이어링). ✓
- autosave→idle 통합(중복 제거) → Task 4(기존 제거). ✓
- 죽은 ai-replace 삭제 → Task 1. ✓
- 가드레일: 내용 중복 제거 → Task 2(`CreateIfChanged`), 적용처 Task 3·5. dirty 플래그 → Task 9. enum-UI 동기화 → Task 1·6·7. ✓
- undo 세션 휘발(TipTap) → 범위 밖 명시 + Task 10 Step 2-6 확인. ✓

**2. Placeholder scan:** "TBD/TODO/적절히 처리" 없음. 모든 코드 step에 실제 코드 포함. 픽스처 헬퍼명(`newSnapFixture`, `newRepoWithNode`)은 "기존 파일 패턴을 따른다"고 명시 — 구현자가 해당 테스트 파일 상단에서 확인.

**3. Type consistency:** `CreateIfChanged(ctx, nodeID, doc, reason, now) (Snapshot, bool, error)` — Task 2 정의, Task 3·5에서 동일 시그니처 사용. `ReasonCompanionBefore = "companion-before"` — Task 1 정의, Task 5·6·7에서 동일 문자열. `useIdleTimer(idleMs, onIdle) → {markActivity, cancel}` — Task 8 정의, Task 9에서 `markActivity` 사용. `UpdateNodeContent`(snaps 제거) — Task 4 정의, main.go(Task 4) 일치. 일관됨.
