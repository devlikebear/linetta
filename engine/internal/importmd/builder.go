package importmd

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/mdmeta"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/ptrutil"
	"github.com/devlikebear/linetta/engine/internal/relationship"
)

const emptyDoc = `{"type":"doc","content":[{"type":"paragraph"}]}`

// BuildResult is what BuildProject returns: the created project plus
// counts and human-readable warnings (e.g. "no headings found").
type BuildResult struct {
	Project           project.Project
	ContainerCount    int
	LeafCount         int
	EntityCount       int
	RelationshipCount int
	Warnings          []string
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
		res.Warnings = append(res.Warnings, WarnNoHeadings)
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
			return res, err
		}
		// Chain off the most-recently-inserted root so outline order is preserved
		// (CreateSibling inserts immediately after refID; without chaining, roots
		// would come out in reverse order relative to the seed).
		refID = created.ID
	}
	if err := nr.Delete(ctx, seedID, now); err != nil {
		return res, err
	}
	final, err := pr.Get(ctx, p.ID)
	if err != nil {
		return res, err
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

// RestoreMetadata recreates exported entities and relationships inside the
// newly-built project. Relationships are resolved by exported entity id first,
// then by name for legacy appendices that had no stable ids.
func RestoreMetadata(ctx context.Context, er *entity.Repo, rr *relationship.Repo, now int64, projectID string, meta mdmeta.Metadata, res *BuildResult) error {
	if er == nil || meta.Empty() {
		return nil
	}

	idMap := map[string]string{}
	nameMap := map[string]string{}
	for _, in := range meta.Entities {
		name := stringsTrim(in.Name)
		if name == "" {
			continue
		}
		kind := stringsTrim(in.Kind)
		if kind == "" {
			kind = entity.KindCharacter
		}
		created, err := er.Create(ctx, now, entity.NewInput{
			ProjectID: projectID,
			Kind:      kind,
			Name:      name,
			Role:      stringsTrim(in.Role),
		})
		if err != nil {
			return err
		}
		attrs := in.Attributes
		if attrs == nil {
			attrs = map[string]string{}
		}
		if in.Summary != "" || len(attrs) > 0 || len(in.Aliases) > 0 {
			aliases := in.Aliases
			if aliases == nil {
				aliases = []string{}
			}
			if err := er.Update(ctx, now, entity.UpdateInput{
				ID:         created.ID,
				Kind:       ptrutil.To(kind),
				Name:       ptrutil.To(name),
				Aliases:    &aliases,
				Role:       ptrutil.To(stringsTrim(in.Role)),
				Summary:    ptrutil.To(stringsTrim(in.Summary)),
				Attributes: &attrs,
			}); err != nil {
				return err
			}
			created, err = er.Get(ctx, created.ID)
			if err != nil {
				return err
			}
		}
		if in.ID != "" {
			idMap[in.ID] = created.ID
		}
		nameMap[name] = created.ID
		res.EntityCount++
	}

	if rr == nil || len(meta.Relationships) == 0 {
		return nil
	}
	return restoreRelationships(ctx, rr, projectID, meta.Relationships, idMap, nameMap, res)
}

func restoreRelationships(ctx context.Context, rr *relationship.Repo, projectID string, rels []mdmeta.Relationship, idMap, nameMap map[string]string, res *BuildResult) error {
	groups := map[string][]mdmeta.Relationship{}
	singles := []mdmeta.Relationship{}
	for _, rel := range rels {
		if stringsTrim(rel.PairID) != "" {
			groups[rel.PairID] = append(groups[rel.PairID], rel)
			continue
		}
		singles = append(singles, rel)
	}

	for _, group := range groups {
		if len(group) == 2 {
			ok, err := restorePair(ctx, rr, projectID, group[0], group[1], idMap, nameMap, res)
			if err != nil {
				return err
			}
			if ok {
				continue
			}
		}
		for _, rel := range group {
			if err := restoreOne(ctx, rr, projectID, rel, idMap, nameMap, res); err != nil {
				return err
			}
		}
	}
	for _, rel := range singles {
		if err := restoreOne(ctx, rr, projectID, rel, idMap, nameMap, res); err != nil {
			return err
		}
	}
	return nil
}

func restorePair(ctx context.Context, rr *relationship.Repo, projectID string, a, b mdmeta.Relationship, idMap, nameMap map[string]string, res *BuildResult) (bool, error) {
	aFrom, aTo := resolveRelationshipEnds(a, idMap, nameMap)
	bFrom, bTo := resolveRelationshipEnds(b, idMap, nameMap)
	if aFrom == "" || aTo == "" || bFrom == "" || bTo == "" ||
		aFrom != bTo || aTo != bFrom || stringsTrim(a.Label) == "" || stringsTrim(b.Label) == "" {
		return false, nil
	}
	created, err := rr.CreatePair(ctx, relationship.NewPairInput{
		ProjectID:    projectID,
		FromID:       aFrom,
		ToID:         aTo,
		Label:        stringsTrim(a.Label),
		InverseLabel: stringsTrim(b.Label),
		Notes:        stringsTrim(a.Notes),
	})
	if err != nil {
		return true, err
	}
	if len(created) == 2 && stringsTrim(b.Notes) != "" {
		if err := rr.Update(ctx, relationship.UpdateInput{
			ID:    created[1].ID,
			Label: created[1].Label,
			Notes: stringsTrim(b.Notes),
		}); err != nil {
			return true, err
		}
	}
	res.RelationshipCount += len(created)
	return true, nil
}

func restoreOne(ctx context.Context, rr *relationship.Repo, projectID string, rel mdmeta.Relationship, idMap, nameMap map[string]string, res *BuildResult) error {
	fromID, toID := resolveRelationshipEnds(rel, idMap, nameMap)
	label := stringsTrim(rel.Label)
	if fromID == "" || toID == "" || label == "" {
		res.Warnings = append(res.Warnings, WarnRelationshipsSkipped)
		return nil
	}
	if _, err := rr.CreateOne(ctx, relationship.NewInput{
		ProjectID: projectID,
		FromID:    fromID,
		ToID:      toID,
		Label:     label,
		Notes:     stringsTrim(rel.Notes),
	}); err != nil {
		return err
	}
	res.RelationshipCount++
	return nil
}

func resolveRelationshipEnds(rel mdmeta.Relationship, idMap, nameMap map[string]string) (string, string) {
	fromID := idMap[stringsTrim(rel.FromID)]
	if fromID == "" {
		fromID = nameMap[stringsTrim(rel.FromName)]
	}
	toID := idMap[stringsTrim(rel.ToID)]
	if toID == "" {
		toID = nameMap[stringsTrim(rel.ToName)]
	}
	return fromID, toID
}

func stringsTrim(s string) string {
	return strings.TrimSpace(s)
}
