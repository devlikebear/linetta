package importmd

import (
	"context"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/mdmeta"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/ptrutil"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// Extras are the stores the completeness pass (#83) restores into. Nil repos
// skip their section, mirroring export.Extras.
type Extras struct {
	Threads *thread.Repo
	Beats   *beat.Repo
	Notes   *note.Repo
	Facts   *fact.Repo
}

// RestoreProjectDetails applies the exported project fields (genres, length
// target, POV, style notes, synopsis, plot outline, episode target) to the
// freshly built project. Invalid enum values are skipped with a warning
// instead of failing the whole import — the manuscript already landed.
func RestoreProjectDetails(ctx context.Context, pr *project.Repo, now int64, meta mdmeta.Metadata, res *BuildResult) error {
	pm := meta.Project
	if pm == nil {
		return nil
	}
	in := project.UpdateInput{ID: res.Project.ID}
	if pm.Genres != nil {
		in.Genres = &pm.Genres
	}
	if pm.LengthTarget != "" {
		if project.ValidLengthTarget(pm.LengthTarget) {
			in.LengthTarget = ptrutil.To(pm.LengthTarget)
		} else {
			res.Warnings = append(res.Warnings, WarnProjectFieldSkipped+":length_target")
		}
	}
	if pm.DefaultPOV != "" {
		if project.ValidDefaultPOV(pm.DefaultPOV) {
			in.DefaultPOV = ptrutil.To(pm.DefaultPOV)
		} else {
			res.Warnings = append(res.Warnings, WarnProjectFieldSkipped+":default_pov")
		}
	}
	if pm.StyleNotes != "" {
		in.StyleNotes = ptrutil.To(pm.StyleNotes)
	}
	if pm.Synopsis != "" {
		in.Synopsis = ptrutil.To(pm.Synopsis)
	}
	if pm.Outline != "" {
		in.Outline = ptrutil.To(pm.Outline)
	}
	if pm.EpisodeCharTarget > 0 {
		in.EpisodeCharTarget = ptrutil.To(pm.EpisodeCharTarget)
	}
	updated, err := pr.Update(ctx, now, in)
	if err != nil {
		return err
	}
	res.Project = updated
	return nil
}

// AlignNodes maps exported node ids onto the nodes the import just created.
//
// Export writes one heading per node and lists the nodes in the same pre-order
// in the frontmatter, so document order survives even when heading levels were
// clamped and the rebuilt tree has a different shape. The walk advances a
// cursor through the exported list only when the created node's label matches
// the exported heading text (label, or "label — title"), which tolerates
// synthetic leaves the import invents. Matched nodes get their label/title
// split, status, and summary back.
func AlignNodes(ctx context.Context, nr *node.Repo, now int64, projectID string, metaNodes []mdmeta.Node, res *BuildResult) (map[string]string, error) {
	idMap := map[string]string{}
	if len(metaNodes) == 0 {
		return idMap, nil
	}
	flat, err := nr.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	children := map[string][]node.Node{}
	for _, n := range flat {
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}
	var ordered []node.Node
	var walk func(parentKey string)
	walk = func(parentKey string) {
		for _, n := range children[parentKey] {
			ordered = append(ordered, n)
			walk(n.ID)
		}
	}
	walk("")

	cursor := 0
	for _, created := range ordered {
		if cursor >= len(metaNodes) {
			break
		}
		m := metaNodes[cursor]
		expected := m.Label
		if m.Title != "" {
			expected += " — " + m.Title
		}
		if created.Label != expected {
			continue
		}
		cursor++
		if m.ID != "" {
			idMap[m.ID] = created.ID
		}
		if m.Title != "" {
			if err := nr.Rename(ctx, created.ID, m.Label, m.Title, now); err != nil {
				return nil, err
			}
		}
		if m.Status != "" && m.Status != created.Status {
			if err := nr.SetStatus(ctx, created.ID, m.Status, now); err != nil {
				res.Warnings = appendOnce(res.Warnings, WarnNodesMetaPartial)
			}
		}
		if m.Summary != "" && created.Kind == "leaf" {
			if err := nr.SetSummary(ctx, created.ID, m.Summary, created.ContentVersion); err != nil {
				return nil, err
			}
		}
	}
	if cursor < len(metaNodes) {
		res.Warnings = appendOnce(res.Warnings, WarnNodesMetaPartial)
	}
	return idMap, nil
}

// RestoreExtras recreates threads/beats, margin notes, and fact cards.
// References to nodes that could not be aligned degrade instead of failing:
// beats and fact cards keep their content with no node link, notes without a
// node have nowhere to anchor and are skipped with a warning.
func RestoreExtras(ctx context.Context, ex Extras, now int64, projectID string, meta mdmeta.Metadata, nodeIDMap map[string]string, res *BuildResult) error {
	if ex.Threads != nil {
		for _, mt := range meta.Threads {
			if stringsTrim(mt.Name) == "" {
				continue
			}
			created, err := ex.Threads.Create(ctx, thread.NewInput{
				ProjectID: projectID,
				Name:      mt.Name,
				Color:     mt.Color,
			})
			if err != nil {
				return err
			}
			if mt.Summary != "" {
				if err := ex.Threads.Update(ctx, thread.UpdateInput{
					ID:      created.ID,
					Summary: ptrutil.To(mt.Summary),
				}); err != nil {
					return err
				}
			}
			if ex.Beats != nil {
				for _, mb := range mt.Beats {
					in := beat.NewInput{
						ThreadID:    created.ID,
						Label:       mb.Label,
						Description: mb.Description,
						Intensity:   mb.Intensity,
					}
					if newID, ok := nodeIDMap[mb.NodeID]; ok {
						in.NodeID = ptrutil.To(newID)
					} else if mb.NodeID != "" {
						res.Warnings = appendOnce(res.Warnings, WarnNodeLinksDropped)
					}
					if _, err := ex.Beats.Create(ctx, in); err != nil {
						return err
					}
				}
			}
			if mt.ClosedAt > 0 {
				if err := ex.Threads.Close(ctx, created.ID, mt.ClosedAt); err != nil {
					return err
				}
			}
		}
	}
	if ex.Notes != nil {
		for _, mn := range meta.Notes {
			newID, ok := nodeIDMap[mn.NodeID]
			if !ok {
				res.Warnings = appendOnce(res.Warnings, WarnNotesSkipped)
				continue
			}
			createdAt := mn.CreatedAt
			if createdAt <= 0 {
				createdAt = now
			}
			if _, err := ex.Notes.Create(ctx, note.NewInput{
				NodeID: newID,
				Anchor: mn.Anchor,
				Body:   mn.Body,
			}, createdAt); err != nil {
				return err
			}
		}
	}
	if ex.Facts != nil {
		for _, mc := range meta.FactCards {
			in := fact.NewInput{
				ProjectID: projectID,
				Claim:     mc.Claim,
				Result:    mc.Result,
				Status:    mc.Status,
				Category:  mc.Category,
			}
			if newID, ok := nodeIDMap[mc.NodeID]; ok {
				in.NodeID = ptrutil.To(newID)
			} else if mc.NodeID != "" {
				res.Warnings = appendOnce(res.Warnings, WarnNodeLinksDropped)
			}
			for _, s := range mc.Sources {
				in.Sources = append(in.Sources, fact.SourceInput{
					URL:        s.URL,
					Title:      s.Title,
					Snippet:    s.Snippet,
					AccessedAt: s.AccessedAt,
				})
			}
			if _, err := ex.Facts.Create(ctx, now, in); err != nil {
				// A card that no longer satisfies validation (unknown status,
				// missing source) should not kill the import of the manuscript.
				res.Warnings = appendOnce(res.Warnings, WarnFactCardsSkipped)
			}
		}
	}
	return nil
}

func appendOnce(warnings []string, code string) []string {
	for _, w := range warnings {
		if w == code {
			return warnings
		}
	}
	return append(warnings, code)
}
