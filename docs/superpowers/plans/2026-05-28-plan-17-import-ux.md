# Plan 17 — Import UX 강화 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Markdown import가 leading-whitespace 헤딩에도 관대해지고, import 전 트리 미리보기 + 결과 토스트로 사용자가 무엇이 들어가는지/들어갔는지 확인할 수 있게 한다.

**Architecture:** (1) `tree.go`의 헤딩 정규식이 라인 앞 공백을 허용. (2) `BuildProject`가 컨테이너/leaf 카운트와 경고를 담은 `BuildResult`를 반환. (3) DB를 건드리지 않는 순수 `Preview()` 함수를 추가해 `imports.preview` RPC가 호출. (4) 프론트엔드는 파일 선택 후 **preview → 모달 확인 → 실제 import → 결과 토스트** 흐름으로 바꾼다.

**Tech Stack:** Go (engine), TypeScript / React (frontend), SQLite. 새 의존성 없음.

---

## 파일 구조

**Engine (Go):**
- `engine/internal/importmd/tree.go` — `headingRe`에 leading whitespace 허용
- `engine/internal/importmd/tree_test.go` — leading whitespace 케이스 추가
- `engine/internal/importmd/builder.go` — `BuildResult` 도입, count + warning 수집
- `engine/internal/importmd/builder_test.go` — `BuildResult` 검증
- `engine/internal/importmd/preview.go` (NEW) — outline → 직렬화 가능한 트리 (DB 미관여)
- `engine/internal/importmd/preview_test.go` (NEW)
- `engine/internal/rpc/handlers/imports.go` — result 확장 + `ImportPreview` 핸들러 추가
- `engine/internal/rpc/handlers/imports_test.go` — 새 결과/핸들러 검증
- `engine/cmd/linetta-engine/main.go:173` 부근 — `imports.preview` 등록

**Frontend (React):**
- `apps/desktop/src/lib/types.ts` — `ImportMarkdownResult` 확장, `ImportPreviewResult` 추가
- `apps/desktop/src/lib/rpc.ts` — `imports.preview` 클라이언트 추가
- `apps/desktop/src/components/ImportPreviewModal.tsx` (NEW) — 트리 + 경고 모달
- `apps/desktop/src/components/ImportPreviewModal.css` (NEW)
- `apps/desktop/src/routes/Library.tsx` — preview → 모달 → import → 토스트 흐름

---

## Task 1: 파서 leading whitespace 허용

**Files:**
- Modify: `engine/internal/importmd/tree.go:26`
- Test: `engine/internal/importmd/tree_test.go`

- [ ] **Step 1: 실패 테스트 추가**

`engine/internal/importmd/tree_test.go` 끝부분에 추가:

```go
func TestParseOutline_leadingWhitespaceOnHeading(t *testing.T) {
	md := "# Title\n" +
		"   ## 1부 어둠\n" +
		"\t### 1장 시작\n" +
		"#### 씬 1\n" +
		"본문이다."
	out := ParseOutline(md)
	if out.Title != "Title" {
		t.Fatalf("title=%q", out.Title)
	}
	if len(out.Roots) != 1 {
		t.Fatalf("want 1 root, got %d", len(out.Roots))
	}
	bu := out.Roots[0]
	if bu.Label != "1부 어둠" || bu.Level != 2 {
		t.Fatalf("part=%+v", bu)
	}
	if len(bu.Children) != 1 || bu.Children[0].Label != "1장 시작" {
		t.Fatalf("chapter mismatch: %+v", bu.Children)
	}
	ch := bu.Children[0]
	if len(ch.Children) != 1 || ch.Children[0].Label != "씬 1" {
		t.Fatalf("scene mismatch: %+v", ch.Children)
	}
}

func TestParseOutline_hashWithoutSpaceIsNotHeading(t *testing.T) {
	out := ParseOutline("##nospace text")
	if len(out.Roots) != 0 {
		t.Fatalf("want no headings, got %d", len(out.Roots))
	}
}
```

- [ ] **Step 2: 실패 확인**

```bash
cd engine && go test ./internal/importmd -run "TestParseOutline_leadingWhitespaceOnHeading|TestParseOutline_hashWithoutSpaceIsNotHeading" -v
```

기대: `TestParseOutline_leadingWhitespaceOnHeading` 실패 (현재는 leading whitespace 헤딩이 무시되어 root 0개).

- [ ] **Step 3: 정규식 수정**

`engine/internal/importmd/tree.go:26` 한 줄 교체:

```go
var headingRe = regexp.MustCompile(`^[ \t]*(#{1,6})\s+(.+?)\s*$`)
```

- [ ] **Step 4: 통과 확인**

```bash
cd engine && go test ./internal/importmd -v
```

기대: 모든 테스트 PASS.

- [ ] **Step 5: 커밋**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/internal/importmd/tree.go engine/internal/importmd/tree_test.go
git commit -m "fix(importmd): allow leading whitespace on heading lines"
```

---

## Task 2: BuildResult — count + warnings

**Files:**
- Modify: `engine/internal/importmd/builder.go`
- Modify: `engine/internal/importmd/builder_test.go`

- [ ] **Step 1: 실패 테스트 추가**

`engine/internal/importmd/builder_test.go` 끝부분에 추가:

```go
func TestBuildProject_returnsCountsAndWarnings(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	pr := project.NewRepo(db)
	nr := node.NewRepo(db)

	md := "# 작품\n" +
		"## 1부\n" +
		"### 1장\n" +
		"#### 씬 1\n본문 1\n" +
		"#### 씬 2\n본문 2\n" +
		"### 2장\n" +
		"#### 씬 1\n본문 3\n"
	out := ParseOutline(md)
	res, err := BuildProject(ctx, pr, nr, 1000, out, "fallback")
	if err != nil {
		t.Fatalf("BuildProject: %v", err)
	}
	if res.ContainerCount != 3 { // 1부, 1장, 2장
		t.Fatalf("ContainerCount=%d want 3", res.ContainerCount)
	}
	if res.LeafCount != 3 { // 씬 1, 씬 2, 씬 1
		t.Fatalf("LeafCount=%d want 3", res.LeafCount)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("Warnings=%v want empty", res.Warnings)
	}
}

func TestBuildProject_warnsWhenNoHeadings(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	pr := project.NewRepo(db)
	nr := node.NewRepo(db)

	out := ParseOutline("그냥 본문만 있고 헤딩이 없음.")
	res, err := BuildProject(ctx, pr, nr, 1000, out, "fallback")
	if err != nil {
		t.Fatalf("BuildProject: %v", err)
	}
	if res.LeafCount != 0 || res.ContainerCount != 0 {
		t.Fatalf("counts: c=%d l=%d want both 0", res.ContainerCount, res.LeafCount)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("want warning for no headings")
	}
}
```

- [ ] **Step 2: 실패 확인**

```bash
cd engine && go test ./internal/importmd -run "TestBuildProject_returnsCountsAndWarnings|TestBuildProject_warnsWhenNoHeadings" -v
```

기대: 컴파일 에러 (`res.ContainerCount` undefined). 의도된 실패.

- [ ] **Step 3: BuildResult 도입 + count 누적**

`engine/internal/importmd/builder.go` 를 다음으로 교체 (전체):

```go
package importmd

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
)

const emptyDoc = `{"type":"doc","content":[{"type":"paragraph"}]}`

// BuildResult is what BuildProject returns: the created project plus
// counts and human-readable warnings (e.g. "no headings found").
type BuildResult struct {
	Project        project.Project
	ContainerCount int
	LeafCount      int
	Warnings       []string
}

// BuildProject creates a new project from the parsed outline.
//
// Behavior:
//   - Title := outline.Title, or fallbackTitle, or "가져온 작품".
//   - Calls pr.Create which auto-seeds a "씬 1" leaf as the first node.
//   - If outline.Roots is empty, the seed is kept and a warning is recorded.
//   - Otherwise, each root is inserted as a sibling of the seed; once all roots
//     are inserted, the original seed is deleted. Containers with both body
//     and children get a synthetic 씬 1 leaf carrying the body first.
func BuildProject(ctx context.Context, pr *project.Repo, nr *node.Repo, now int64, outline Outline, fallbackTitle string) (BuildResult, error) {
	title := outline.Title
	if title == "" {
		title = fallbackTitle
	}
	if title == "" {
		title = "가져온 작품"
	}
	p, err := pr.Create(ctx, now, project.NewInput{
		Title:        title,
		Genres:       []string{},
		LengthTarget: "short",
		DefaultPOV:   "first",
	})
	if err != nil {
		return BuildResult{}, err
	}

	res := BuildResult{Project: p}

	if len(outline.Roots) == 0 {
		res.Warnings = append(res.Warnings,
			"헤딩(`#`, `##`, `###`, `####`)을 찾지 못해 비어있는 작품이 생성되었습니다. 마크다운에 헤딩을 추가한 뒤 다시 가져와 주세요.")
		return res, nil
	}
	if p.LastOpenedNodeID == nil {
		return res, nil
	}
	seedID := *p.LastOpenedNodeID

	refID := seedID
	for _, root := range outline.Roots {
		created, err := insertNode(ctx, nr, now, root, "", refID, &res)
		if err != nil {
			return BuildResult{}, err
		}
		refID = created.ID
	}
	if err := nr.Delete(ctx, seedID, now); err != nil {
		return BuildResult{}, err
	}
	final, err := pr.Get(ctx, p.ID)
	if err != nil {
		return BuildResult{}, err
	}
	res.Project = final
	return res, nil
}

// insertNode creates the node (and its descendants) corresponding to n.
// If parentID is "", the new node becomes a sibling of seedRefID at root level.
// Otherwise it becomes a child of parentID.
func insertNode(ctx context.Context, nr *node.Repo, now int64, n *OutlineNode, parentID, seedRefID string, res *BuildResult) (node.Node, error) {
	hasChildren := len(n.Children) > 0
	hasBody := len(n.Body) > 0

	kind := "leaf"
	if hasChildren {
		kind = "container"
	}

	var created node.Node
	var err error
	if parentID != "" {
		created, err = nr.CreateChild(ctx, parentID, kind, n.Label, "", now)
	} else {
		created, err = nr.CreateSibling(ctx, seedRefID, kind, n.Label, "", now)
	}
	if err != nil {
		return node.Node{}, err
	}
	if kind == "container" {
		res.ContainerCount++
	} else {
		res.LeafCount++
	}

	if !hasChildren {
		if err := writeBody(ctx, nr, created.ID, n.Body, now); err != nil {
			return node.Node{}, err
		}
		return created, nil
	}

	if hasBody {
		synth, err := nr.CreateChild(ctx, created.ID, "leaf", "씬 1", "", now)
		if err != nil {
			return node.Node{}, err
		}
		if err := writeBody(ctx, nr, synth.ID, n.Body, now); err != nil {
			return node.Node{}, err
		}
		res.LeafCount++
	}
	for _, child := range n.Children {
		if _, err := insertNode(ctx, nr, now, child, created.ID, "", res); err != nil {
			return node.Node{}, err
		}
	}
	return created, nil
}

func writeBody(ctx context.Context, nr *node.Repo, leafID string, body []TiptapBlock, now int64) error {
	if len(body) == 0 {
		return nr.UpdateContent(ctx, leafID, emptyDoc, now)
	}
	doc := map[string]any{
		"type":    "doc",
		"content": toAnySlice(body),
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return nr.UpdateContent(ctx, leafID, string(raw), now)
}

func toAnySlice(blocks []TiptapBlock) []any {
	out := make([]any, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, b)
	}
	return out
}
```

- [ ] **Step 4: 기존 호출처 시그니처 맞추기**

`engine/internal/importmd/builder_test.go` 안의 기존 테스트에서 `BuildProject` 의 반환값을 사용하는 곳을 찾아 `.Project` 로 변환. 호출은 다음과 같이 바뀜:

```go
res, err := BuildProject(ctx, pr, nr, now, outline, "fallback")
// 기존: built, err := ...
// 사용처에서 built.ID, built.Title 등이면 res.Project.ID, res.Project.Title 로
```

기존 테스트가 `built.ID` 같은 형태로 쓰고 있다면 모두 `res.Project.ID` 로 갱신.

- [ ] **Step 5: 통과 확인**

```bash
cd engine && go test ./internal/importmd -v
```

기대: 모두 PASS.

- [ ] **Step 6: 커밋**

```bash
git add engine/internal/importmd/builder.go engine/internal/importmd/builder_test.go
git commit -m "feat(importmd): BuildProject returns counts and warnings"
```

---

## Task 3: Preview — outline에서 직렬화 가능한 트리 추출

**Files:**
- Create: `engine/internal/importmd/preview.go`
- Create: `engine/internal/importmd/preview_test.go`

- [ ] **Step 1: 실패 테스트 작성**

`engine/internal/importmd/preview_test.go` 신규 작성:

```go
package importmd

import (
	"testing"
)

func TestPreview_buildsTreeWithCountsAndWarnings(t *testing.T) {
	md := "# 작품\n" +
		"## 1부\n" +
		"### 1장\n" +
		"#### 씬 1\n본문\n" +
		"#### 씬 2\n본문\n" +
		"### 2장\n" +
		"#### 씬 1\n본문\n"
	out := ParseOutline(md)
	pv := Preview(out, "fallback.md")
	if pv.Title != "작품" {
		t.Fatalf("title=%q", pv.Title)
	}
	if pv.ContainerCount != 3 {
		t.Fatalf("ContainerCount=%d want 3", pv.ContainerCount)
	}
	if pv.LeafCount != 3 {
		t.Fatalf("LeafCount=%d want 3", pv.LeafCount)
	}
	if len(pv.Warnings) != 0 {
		t.Fatalf("warnings=%v", pv.Warnings)
	}
	if len(pv.Roots) != 1 || pv.Roots[0].Label != "1부" {
		t.Fatalf("roots=%+v", pv.Roots)
	}
	bu := pv.Roots[0]
	if bu.Kind != "container" || len(bu.Children) != 2 {
		t.Fatalf("part=%+v", bu)
	}
}

func TestPreview_emptyOutlineProducesWarning(t *testing.T) {
	pv := Preview(ParseOutline("그냥 본문만"), "x.md")
	if len(pv.Warnings) == 0 {
		t.Fatal("expected a warning for no headings")
	}
	if pv.ContainerCount != 0 || pv.LeafCount != 0 {
		t.Fatalf("counts: %+v", pv)
	}
}

func TestPreview_titleFallsBackToFileName(t *testing.T) {
	// markdown with no H1
	pv := Preview(ParseOutline("## 1부\n"), "novel.md")
	if pv.Title != "novel" {
		t.Fatalf("title=%q want 'novel'", pv.Title)
	}
}

func TestPreview_containerWithBodyAddsSyntheticLeaf(t *testing.T) {
	// container has both body lines and child headings.
	md := "# 작품\n" +
		"## 1부\n" +
		"이 부의 도입 문단.\n" +
		"### 1장\n#### 씬 1\n본문\n"
	pv := Preview(ParseOutline(md), "x.md")
	bu := pv.Roots[0]
	// children should be: synthetic "씬 1" leaf (for body) + "1장" container
	if len(bu.Children) != 2 {
		t.Fatalf("want 2 children (synth leaf + chapter), got %d", len(bu.Children))
	}
	if bu.Children[0].Kind != "leaf" || bu.Children[0].Label != "씬 1" {
		t.Fatalf("first child should be synthetic 씬 1 leaf, got %+v", bu.Children[0])
	}
}
```

- [ ] **Step 2: 실패 확인**

```bash
cd engine && go test ./internal/importmd -run TestPreview -v
```

기대: 컴파일 실패 (`Preview` undefined).

- [ ] **Step 3: 구현**

`engine/internal/importmd/preview.go` 신규 작성:

```go
package importmd

import "strings"

// PreviewNode is a serializable mirror of the tree that BuildProject would
// produce. Kind is "container" or "leaf". Used by the frontend preview modal.
type PreviewNode struct {
	Label    string         `json:"label"`
	Kind     string         `json:"kind"`
	Children []*PreviewNode `json:"children,omitempty"`
}

// PreviewResult is the read-only summary of what an import would create.
// No database mutation. Mirrors BuildResult's counts so the UI can show
// the same totals before and after.
type PreviewResult struct {
	Title          string         `json:"title"`
	ContainerCount int            `json:"container_count"`
	LeafCount      int            `json:"leaf_count"`
	Warnings       []string       `json:"warnings"`
	Roots          []*PreviewNode `json:"roots"`
}

// Preview converts a parsed Outline into a PreviewResult. fallbackFileName is
// used to derive a title when outline.Title is empty (mirrors BuildProject).
// Pure function, no I/O.
func Preview(outline Outline, fallbackFileName string) PreviewResult {
	title := outline.Title
	if title == "" {
		title = stripMarkdownExt(fallbackFileName)
	}
	if title == "" {
		title = "가져온 작품"
	}

	res := PreviewResult{Title: title}
	if len(outline.Roots) == 0 {
		res.Warnings = append(res.Warnings,
			"헤딩(`#`, `##`, `###`, `####`)을 찾지 못해 비어있는 작품이 생성됩니다. 마크다운에 헤딩을 추가해 주세요.")
		return res
	}
	for _, r := range outline.Roots {
		res.Roots = append(res.Roots, walkPreview(r, &res))
	}
	return res
}

func walkPreview(n *OutlineNode, res *PreviewResult) *PreviewNode {
	hasChildren := len(n.Children) > 0
	hasBody := len(n.Body) > 0
	kind := "leaf"
	if hasChildren {
		kind = "container"
	}
	pn := &PreviewNode{Label: n.Label, Kind: kind}
	if kind == "container" {
		res.ContainerCount++
	} else {
		res.LeafCount++
	}
	if hasChildren && hasBody {
		// Synthetic 씬 1 leaf — same as BuildProject does.
		pn.Children = append(pn.Children, &PreviewNode{Label: "씬 1", Kind: "leaf"})
		res.LeafCount++
	}
	for _, c := range n.Children {
		pn.Children = append(pn.Children, walkPreview(c, res))
	}
	return pn
}

func stripMarkdownExt(name string) string {
	if name == "" {
		return ""
	}
	lower := strings.ToLower(name)
	for _, suf := range []string{".markdown", ".md"} {
		if strings.HasSuffix(lower, suf) {
			return name[:len(name)-len(suf)]
		}
	}
	return name
}
```

- [ ] **Step 4: 통과 확인**

```bash
cd engine && go test ./internal/importmd -v
```

기대: 모두 PASS.

- [ ] **Step 5: 커밋**

```bash
git add engine/internal/importmd/preview.go engine/internal/importmd/preview_test.go
git commit -m "feat(importmd): pure Preview() — tree + counts + warnings without DB writes"
```

---

## Task 4: imports.markdown RPC 결과 확장

**Files:**
- Modify: `engine/internal/rpc/handlers/imports.go`
- Modify: `engine/internal/rpc/handlers/imports_test.go`

- [ ] **Step 1: 실패 테스트 추가**

`engine/internal/rpc/handlers/imports_test.go` 의 기존 `imports.markdown` 테스트 옆에 다음을 추가하거나, 기존 테스트가 결과를 검증하는 부분을 다음 형태로 강화:

```go
func TestImportMarkdown_resultIncludesCountsAndWarnings(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t) // 헬퍼 — 기존 테스트와 동일 패턴 사용
	pr := project.NewRepo(db)
	nr := node.NewRepo(db)
	h := ImportMarkdown(pr, nr, func() int64 { return 1000 })

	md := "# 작품\n## 1부\n### 1장\n#### 씬 1\n본문\n"
	params, _ := json.Marshal(map[string]any{
		"file_name": "x.md",
		"content":   md,
	})
	raw, err := h(ctx, params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got struct {
		ProjectID      string   `json:"project_id"`
		ContainerCount int      `json:"container_count"`
		LeafCount      int      `json:"leaf_count"`
		Warnings       []string `json:"warnings"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ProjectID == "" {
		t.Fatal("project_id empty")
	}
	if got.ContainerCount != 2 { // 1부, 1장
		t.Fatalf("ContainerCount=%d want 2", got.ContainerCount)
	}
	if got.LeafCount != 1 { // 씬 1
		t.Fatalf("LeafCount=%d want 1", got.LeafCount)
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("warnings=%v want none", got.Warnings)
	}
}
```

(기존 테스트 헬퍼 `newTestDB`가 이미 있다고 가정. 없다면 해당 파일에서 동일 패턴을 따른다.)

- [ ] **Step 2: 실패 확인**

```bash
cd engine && go test ./internal/rpc/handlers -run TestImportMarkdown_resultIncludesCountsAndWarnings -v
```

기대: 응답 JSON에 `container_count` 등이 없어 0/빈 값으로 unmarshal → 어서션 실패.

- [ ] **Step 3: 핸들러 결과 구조 확장**

`engine/internal/rpc/handlers/imports.go` 의 `importMarkdownResult` 와 핸들러 본문을 다음으로 교체:

```go
type importMarkdownResult struct {
	ProjectID      string   `json:"project_id"`
	ContainerCount int      `json:"container_count"`
	LeafCount      int      `json:"leaf_count"`
	Warnings       []string `json:"warnings"`
}

// ImportMarkdown returns a handler for imports.markdown.
func ImportMarkdown(pr *project.Repo, nr *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p importMarkdownParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		fallback := fallbackTitleFromFileName(p.FileName)
		outline := importmd.ParseOutline(p.Content)
		built, err := importmd.BuildProject(ctx, pr, nr, now(), outline, fallback)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		out := importMarkdownResult{
			ProjectID:      built.Project.ID,
			ContainerCount: built.ContainerCount,
			LeafCount:      built.LeafCount,
			Warnings:       built.Warnings,
		}
		if out.Warnings == nil {
			out.Warnings = []string{}
		}
		return json.Marshal(out)
	}
}
```

- [ ] **Step 4: 통과 확인**

```bash
cd engine && go test ./internal/rpc/handlers -v
```

기대: 모두 PASS. (다른 import 관련 테스트도 같이 봐서 회귀 없음 확인.)

- [ ] **Step 5: 커밋**

```bash
git add engine/internal/rpc/handlers/imports.go engine/internal/rpc/handlers/imports_test.go
git commit -m "feat(rpc): imports.markdown result carries counts + warnings"
```

---

## Task 5: imports.preview RPC 추가

**Files:**
- Modify: `engine/internal/rpc/handlers/imports.go`
- Modify: `engine/internal/rpc/handlers/imports_test.go`
- Modify: `engine/cmd/linetta-engine/main.go:173` 부근

- [ ] **Step 1: 실패 테스트 추가**

`engine/internal/rpc/handlers/imports_test.go` 끝에 추가:

```go
func TestImportPreview_returnsTreeNoDBWrite(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	pr := project.NewRepo(db)

	h := ImportPreview()

	md := "# 작품\n## 1부\n### 1장\n#### 씬 1\n본문\n"
	params, _ := json.Marshal(map[string]any{
		"file_name": "novel.md",
		"content":   md,
	})
	raw, err := h(ctx, params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got struct {
		Title          string   `json:"title"`
		ContainerCount int      `json:"container_count"`
		LeafCount      int      `json:"leaf_count"`
		Warnings       []string `json:"warnings"`
		Roots          []any    `json:"roots"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Title != "작품" {
		t.Fatalf("title=%q", got.Title)
	}
	if got.ContainerCount != 2 || got.LeafCount != 1 {
		t.Fatalf("counts: c=%d l=%d", got.ContainerCount, got.LeafCount)
	}
	if len(got.Roots) != 1 {
		t.Fatalf("roots len=%d", len(got.Roots))
	}
	// Confirm no project rows created.
	projects, err := pr.List(ctx, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Fatalf("preview must not write DB, found %d projects", len(projects))
	}
}
```

(만약 `pr.List` 의 시그니처가 다르다면 같은 파일 내 다른 테스트가 쓰는 표현으로 맞춘다.)

- [ ] **Step 2: 실패 확인**

```bash
cd engine && go test ./internal/rpc/handlers -run TestImportPreview -v
```

기대: 컴파일 실패 (`ImportPreview` undefined).

- [ ] **Step 3: 핸들러 추가**

`engine/internal/rpc/handlers/imports.go` 파일 끝에 다음 추가:

```go
// ImportPreview returns a handler for imports.preview. It parses the markdown
// and returns the would-be tree (label, kind, children) plus counts and
// warnings — without creating any project or node rows.
func ImportPreview() rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p importMarkdownParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		outline := importmd.ParseOutline(p.Content)
		pv := importmd.Preview(outline, p.FileName)
		if pv.Warnings == nil {
			pv.Warnings = []string{}
		}
		if pv.Roots == nil {
			pv.Roots = []*importmd.PreviewNode{}
		}
		return json.Marshal(pv)
	}
}
```

- [ ] **Step 4: main.go 등록**

`engine/cmd/linetta-engine/main.go:173` 근처 (`s.Handle("imports.markdown", ...)` 바로 아래에) 한 줄 추가:

```go
s.Handle("imports.preview", handlers.ImportPreview())
```

- [ ] **Step 5: 통과 확인 + 엔진 빌드**

```bash
cd engine && go test ./...
cd /Users/changheonshin/workspace/myworks/linetta && ./scripts/build-engine.sh
```

기대: 모든 패키지 PASS, 엔진 바이너리 재생성.

- [ ] **Step 6: 커밋**

```bash
git add engine/internal/rpc/handlers/imports.go engine/internal/rpc/handlers/imports_test.go engine/cmd/linetta-engine/main.go
git commit -m "feat(rpc): add imports.preview — read-only tree preview before commit"
```

---

## Task 6: 프론트엔드 — types + rpc 클라이언트

**Files:**
- Modify: `apps/desktop/src/lib/types.ts:181-183`
- Modify: `apps/desktop/src/lib/rpc.ts:91-97`

- [ ] **Step 1: 타입 정의 수정**

`apps/desktop/src/lib/types.ts:181-183` 의 `ImportMarkdownResult` 를 다음으로 교체하고, 바로 아래에 미리보기 타입을 추가:

```ts
export interface ImportMarkdownResult {
  project_id: string;
  container_count: number;
  leaf_count: number;
  warnings: string[];
}

export interface ImportPreviewNode {
  label: string;
  kind: "container" | "leaf";
  children?: ImportPreviewNode[];
}

export interface ImportPreviewResult {
  title: string;
  container_count: number;
  leaf_count: number;
  warnings: string[];
  roots: ImportPreviewNode[];
}
```

- [ ] **Step 2: rpc 클라이언트 메서드 추가**

`apps/desktop/src/lib/rpc.ts:91-97` 의 `imports` 블록을 다음으로 교체:

```ts
export const imports = {
  markdown: (fileName: string, content: string) =>
    rpcCall<ImportMarkdownResult>("imports.markdown", {
      file_name: fileName,
      content,
    }),
  preview: (fileName: string, content: string) =>
    rpcCall<ImportPreviewResult>("imports.preview", {
      file_name: fileName,
      content,
    }),
};
```

같은 파일 상단의 `import type {...}` 줄에서 `ImportPreviewResult` 가 import 되는지 확인하고, 빠져 있으면 추가:

```ts
import type {
  // ... existing ...
  ImportMarkdownResult,
  ImportPreviewResult,
  // ... existing ...
} from "./types";
```

- [ ] **Step 3: 타입체크**

```bash
cd apps/desktop && npx tsc --noEmit
```

기대: 에러 없음.

- [ ] **Step 4: 커밋**

```bash
git add apps/desktop/src/lib/types.ts apps/desktop/src/lib/rpc.ts
git commit -m "feat(rpc-client): ImportPreviewResult + imports.preview"
```

---

## Task 7: ImportPreviewModal 컴포넌트

**Files:**
- Create: `apps/desktop/src/components/ImportPreviewModal.tsx`
- Create: `apps/desktop/src/components/ImportPreviewModal.css`

- [ ] **Step 1: CSS 작성**

`apps/desktop/src/components/ImportPreviewModal.css` 신규 작성:

```css
.import-preview-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
}

.import-preview-modal {
  background: var(--surface, #1d1d1f);
  color: var(--text, #e8e8ea);
  border-radius: 12px;
  padding: 1.5rem 1.75rem;
  width: min(560px, 92vw);
  max-height: 80vh;
  display: flex;
  flex-direction: column;
  gap: 1rem;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.55);
}

.import-preview-title {
  font-size: 1.1rem;
  font-weight: 600;
  margin: 0;
}

.import-preview-meta {
  font-size: 0.85rem;
  opacity: 0.7;
  margin: 0;
}

.import-preview-warnings {
  border: 1px solid #b9892f;
  background: rgba(185, 137, 47, 0.12);
  border-radius: 8px;
  padding: 0.6rem 0.8rem;
  font-size: 0.85rem;
}

.import-preview-warnings ul {
  margin: 0.3rem 0 0 1rem;
  padding: 0;
}

.import-preview-tree {
  flex: 1;
  overflow-y: auto;
  padding: 0.5rem 0;
  font-size: 0.9rem;
}

.import-preview-tree ul {
  list-style: none;
  margin: 0;
  padding-left: 1.1rem;
}

.import-preview-tree li {
  padding: 0.15rem 0;
}

.import-preview-tree .kind-leaf {
  opacity: 0.85;
}

.import-preview-tree .kind-container {
  font-weight: 600;
}

.import-preview-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}

.import-preview-actions button {
  padding: 0.4rem 1rem;
}

.import-preview-empty {
  opacity: 0.6;
  font-size: 0.9rem;
  padding: 1rem 0;
  text-align: center;
}
```

- [ ] **Step 2: 컴포넌트 작성**

`apps/desktop/src/components/ImportPreviewModal.tsx` 신규 작성:

```tsx
import type { ImportPreviewNode, ImportPreviewResult } from "../lib/types";
import "./ImportPreviewModal.css";

interface Props {
  preview: ImportPreviewResult;
  fileName: string;
  busy: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ImportPreviewModal({ preview, fileName, busy, onConfirm, onCancel }: Props) {
  const total = preview.container_count + preview.leaf_count;
  return (
    <div className="import-preview-backdrop" onMouseDown={onCancel}>
      <div className="import-preview-modal" onMouseDown={(e) => e.stopPropagation()}>
        <h3 className="import-preview-title">가져오기 미리보기</h3>
        <p className="import-preview-meta">
          {fileName} → <strong>{preview.title}</strong> · 컨테이너 {preview.container_count}개 · 씬 {preview.leaf_count}개
        </p>

        {preview.warnings.length > 0 && (
          <div className="import-preview-warnings" role="alert">
            <strong>경고</strong>
            <ul>
              {preview.warnings.map((w, i) => (
                <li key={i}>{w}</li>
              ))}
            </ul>
          </div>
        )}

        <div className="import-preview-tree">
          {total === 0 ? (
            <p className="import-preview-empty">가져올 노드가 없습니다.</p>
          ) : (
            <ul>
              {preview.roots.map((n, i) => (
                <PreviewItem key={i} node={n} />
              ))}
            </ul>
          )}
        </div>

        <div className="import-preview-actions">
          <button type="button" onClick={onCancel} disabled={busy}>
            취소
          </button>
          <button type="button" className="primary" onClick={onConfirm} disabled={busy || total === 0}>
            {busy ? "가져오는 중…" : "확인 후 가져오기"}
          </button>
        </div>
      </div>
    </div>
  );
}

function PreviewItem({ node }: { node: ImportPreviewNode }) {
  return (
    <li>
      <span className={`kind-${node.kind}`}>
        {node.kind === "container" ? "📁 " : "📄 "}
        {node.label || "(이름 없음)"}
      </span>
      {node.children && node.children.length > 0 && (
        <ul>
          {node.children.map((c, i) => (
            <PreviewItem key={i} node={c} />
          ))}
        </ul>
      )}
    </li>
  );
}
```

(이모지가 거슬리면 lucide `Folder` / `FileText` 아이콘으로 교체 가능. 일단 텍스트 fallback으로 진행.)

- [ ] **Step 3: 타입체크 + 빌드**

```bash
cd apps/desktop && npx tsc --noEmit
```

기대: 에러 없음.

- [ ] **Step 4: 커밋**

```bash
git add apps/desktop/src/components/ImportPreviewModal.tsx apps/desktop/src/components/ImportPreviewModal.css
git commit -m "feat(import): ImportPreviewModal — tree + warnings before commit"
```

---

## Task 8: Library 흐름 — preview → 모달 → import → 토스트

**Files:**
- Modify: `apps/desktop/src/routes/Library.tsx`

- [ ] **Step 1: handleImport 흐름 교체**

`apps/desktop/src/routes/Library.tsx` 를 다음으로 교체:

```tsx
import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { projects as projectsApi, imports as importsApi } from "../lib/rpc";
import type { ImportPreviewResult, NewProjectInput, Project } from "../lib/types";
import { ProjectCard } from "../components/ProjectCard";
import { NewProjectModal } from "../components/NewProjectModal";
import { ImportPreviewModal } from "../components/ImportPreviewModal";
import { pickAndReadMarkdown } from "../lib/importLoad";
import { MoreHorizontal, Settings, Plus, Upload } from "../lib/icons";
import { useToast } from "../components/ToastProvider";

const RECENT_LIMIT = 5;

interface PendingImport {
  fileName: string;
  content: string;
  preview: ImportPreviewResult;
}

export function Library() {
  const [recent, setRecent] = useState<Project[]>([]);
  const [totalRecent, setTotalRecent] = useState<number>(0);
  const [modalOpen, setModalOpen] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [importing, setImporting] = useState(false);
  const [pending, setPending] = useState<PendingImport | null>(null);
  const navigate = useNavigate();
  const { showToast } = useToast();

  const refresh = async () => {
    setLoading(true);
    setError(null);
    try {
      const all = await projectsApi.list({ limit: RECENT_LIMIT + 1 });
      setRecent(all.slice(0, RECENT_LIMIT));
      setTotalRecent(all.length);
      if (all.length === 0) setModalOpen(true);
    } catch (err) {
      setError(String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    refresh();
  }, []);

  const handleCreate = async (input: NewProjectInput) => {
    const created = await projectsApi.create(input);
    setModalOpen(false);
    navigate(`/workspace/${created.id}`);
  };

  const handleImport = async () => {
    setImporting(true);
    try {
      const picked = await pickAndReadMarkdown();
      if (!picked) return;
      const preview = await importsApi.preview(picked.fileName, picked.content);
      setPending({ fileName: picked.fileName, content: picked.content, preview });
    } catch (err) {
      showToast(`가져오기 실패: ${err}`);
    } finally {
      setImporting(false);
    }
  };

  const confirmImport = async () => {
    if (!pending) return;
    setImporting(true);
    try {
      const res = await importsApi.markdown(pending.fileName, pending.content);
      const total = res.container_count + res.leaf_count;
      let msg = `가져오기 완료 · 컨테이너 ${res.container_count}개 · 씬 ${res.leaf_count}개`;
      if (total === 0) msg = "가져오기 완료 · 빈 작품 (헤딩 없음)";
      if (res.warnings.length > 0) msg += ` · 경고 ${res.warnings.length}개`;
      showToast(msg);
      setPending(null);
      navigate(`/workspace/${res.project_id}`);
    } catch (err) {
      showToast(`가져오기 실패: ${err}`);
    } finally {
      setImporting(false);
    }
  };

  return (
    <main className="library">
      <header className="library-top">
        <button className="icon-btn" aria-label="라이브러리 옵션" disabled>
          <MoreHorizontal size={16} />
        </button>
        <Link to="/settings" className="icon-btn" aria-label="설정">
          <Settings size={16} />
        </Link>
      </header>

      <section className="library-center">
        <h1 className="library-heading">Linetta</h1>

        <div className="library-actions">
          <button className="new-button" onClick={() => setModalOpen(true)}>
            <Plus size={16} />
            <span>새 작품</span>
          </button>
          <button
            className="new-button"
            onClick={handleImport}
            disabled={importing}
          >
            <Upload size={16} />
            <span>{importing ? "가져오는 중…" : "가져오기 (.md)"}</span>
          </button>
        </div>

        {loading ? (
          <p className="hint">불러오는 중…</p>
        ) : error ? (
          <p className="error">{error}</p>
        ) : recent.length === 0 ? null : (
          <>
            <p className="library-label">최근 작품 · {recent.length}개</p>
            <div className="card-grid">
              {recent.map((p) => (
                <ProjectCard key={p.id} project={p} />
              ))}
            </div>
            {totalRecent > RECENT_LIMIT && (
              <Link to="/library/all" className="library-all-link">
                전체 라이브러리 →
              </Link>
            )}
          </>
        )}
      </section>

      <NewProjectModal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        onSubmit={handleCreate}
      />

      {pending && (
        <ImportPreviewModal
          preview={pending.preview}
          fileName={pending.fileName}
          busy={importing}
          onConfirm={confirmImport}
          onCancel={() => setPending(null)}
        />
      )}
    </main>
  );
}
```

- [ ] **Step 2: 타입체크 + 빌드**

```bash
cd apps/desktop && npx tsc --noEmit
```

기대: 에러 없음.

- [ ] **Step 3: 커밋**

```bash
git add apps/desktop/src/routes/Library.tsx
git commit -m "feat(import): preview modal flow + result toast in Library"
```

---

## 통합 검증 (수동 스모크)

플랜이 끝나면 다음을 직접 돌려본다:

1. `rm -rf /tmp/linetta-plan17 && LINETTA_HOME=/tmp/linetta-plan17 ./scripts/dev.sh`
2. Library → 가져오기 → 정상 markdown (`# 작품 / ## 1부 / ### 1장 / #### 씬 1`) 선택
   - 모달이 컨테이너 2 + 씬 1 표시 → "확인 후 가져오기" → 토스트 "가져오기 완료 · 컨테이너 2개 · 씬 1개"
3. **Leading whitespace 케이스** — `   ## 1부` 처럼 앞에 공백 있는 markdown:
   - 모달 트리에 1부가 보여야 함 (전엔 안 보였음)
4. **헤딩 없는 markdown** — 본문만 있는 .md:
   - 모달에 경고 배너 + "가져올 노드가 없습니다" + 확인 버튼 비활성화 → 취소
5. Plan 16 hierarchical도 살아있는지 확인:
   - 다층 트리 작품 안에서 AI 모드 → `sqlite3 /tmp/linetta-plan17/library.db "SELECT context_json FROM ai_runs ORDER BY started_at DESC LIMIT 1" | jq .hierarchical`
   - `nearby_leaf_summaries`, `other_chapter_summaries` 등이 채워져야 함

모두 통과하면:

```bash
git tag plan-17-import-ux-done
# 같은 시점에 Plan 16도 사실상 검증 완료 — 같이 tag
git tag plan-16-hierarchical-context-done
```

---

## Self-Review

**1. Spec 커버리지:**
- 파싱 관대화 → Task 1
- BuildResult + counts/warnings → Task 2
- 순수 Preview 함수 → Task 3
- 확장된 imports.markdown 결과 → Task 4
- 새 imports.preview RPC → Task 5
- FE 타입/RPC 클라이언트 → Task 6
- 미리보기 모달 → Task 7
- Library 흐름 + 토스트 → Task 8
모든 요구사항이 task로 매핑됨.

**2. Placeholder scan:** 전 task가 실제 코드/명령 포함, "TBD" 없음. Task 4의 `newTestDB` 헬퍼는 기존 imports_test.go가 이미 쓰는 것이라 가정 — 만약 없다면 가까운 다른 핸들러 테스트의 init 패턴을 그대로 차용.

**3. Type 일관성:**
- `BuildResult` (Task 2) → Task 4의 `built.Project.ID` / `built.ContainerCount` 등에서 정확히 사용
- `PreviewResult` (Task 3) → Task 5의 핸들러에서 `pv.Warnings` / `pv.Roots` 일치
- `ImportPreviewResult` / `ImportPreviewNode` (Task 6) → Task 7 모달이 그대로 소비
- `kind: "container" | "leaf"` 리터럴 일치 — Task 3의 Go 측("container"/"leaf") = Task 6 TS literal type
- 메서드명 `importsApi.preview` (Task 6) = Task 8 사용처 일치

**4. 위험 영역:** Task 2에서 기존 `BuildProject` 호출처가 시그니처 바뀌므로 (`project.Project` → `BuildResult`) `builder_test.go` 의 기존 어서션 갱신이 필요. Step 4에 명시.

자체 검토 통과.
