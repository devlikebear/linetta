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
		// Chain off the most-recently-inserted root so outline order is preserved
		// (CreateSibling inserts immediately after refID; without chaining, roots
		// would come out in reverse order relative to the seed).
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
