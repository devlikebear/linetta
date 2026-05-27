package importmd

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
)

// emptyDoc is the placeholder Tiptap doc for a leaf with no body.
const emptyDoc = `{"type":"doc","content":[{"type":"paragraph"}]}`

// BuildProject creates a new project from the parsed outline.
//
// Behavior:
//   - Title := outline.Title, or fallbackTitle, or "가져온 작품".
//   - Calls pr.Create which auto-seeds a "씬 1" leaf as the first node.
//   - If outline.Roots is empty, the seed is kept as-is (acceptable degradation
//     for malformed markdown).
//   - Otherwise, each root is inserted as a sibling of the seed; once all roots
//     are inserted, the original seed is deleted. Containers with both body
//     and children get a synthetic 씬 1 leaf carrying the body first.
func BuildProject(ctx context.Context, pr *project.Repo, nr *node.Repo, now int64, outline Outline, fallbackTitle string) (project.Project, error) {
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
		return project.Project{}, err
	}
	if len(outline.Roots) == 0 {
		return p, nil
	}
	if p.LastOpenedNodeID == nil {
		return p, nil
	}
	seedID := *p.LastOpenedNodeID

	// Insert roots as siblings of the seed, in order. After the first sibling
	// is inserted, subsequent ones use the previously-inserted root as their
	// reference so they end up adjacent and in the correct order.
	refID := seedID
	for _, root := range outline.Roots {
		created, err := insertNode(ctx, nr, now, root, "", refID)
		if err != nil {
			return project.Project{}, err
		}
		// Note: CreateSibling places the new node immediately AFTER refID,
		// so to preserve outline order when we walk roots[0..n], we keep
		// inserting after the seed (which means roots end up in REVERSE
		// order relative to the seed). To get correct order we instead chain
		// off the most recently inserted root.
		refID = created.ID
	}
	// Now delete the seed.
	if err := nr.Delete(ctx, seedID, now); err != nil {
		return project.Project{}, err
	}
	return pr.Get(ctx, p.ID)
}

// insertNode creates the node (and its descendants) corresponding to n.
// If parentID is "", the new node becomes a sibling of seedRefID at root level.
// Otherwise it becomes a child of parentID.
func insertNode(ctx context.Context, nr *node.Repo, now int64, n *OutlineNode, parentID, seedRefID string) (node.Node, error) {
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

	if !hasChildren {
		// Leaf — write body directly.
		if err := writeBody(ctx, nr, created.ID, n.Body, now); err != nil {
			return node.Node{}, err
		}
		return created, nil
	}

	// Container.
	if hasBody {
		// Synthetic "씬 1" carries the body.
		synth, err := nr.CreateChild(ctx, created.ID, "leaf", "씬 1", "", now)
		if err != nil {
			return node.Node{}, err
		}
		if err := writeBody(ctx, nr, synth.ID, n.Body, now); err != nil {
			return node.Node{}, err
		}
	}
	for _, child := range n.Children {
		if _, err := insertNode(ctx, nr, now, child, created.ID, ""); err != nil {
			return node.Node{}, err
		}
	}
	return created, nil
}

// writeBody serializes body blocks to Tiptap doc JSON and updates the leaf.
// Empty body → emptyDoc placeholder.
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
