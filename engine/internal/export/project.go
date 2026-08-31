package export

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/mdmeta"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// Payload is the wire-shape returned by both ExportProject and ExportNode.
type Payload struct {
	Markdown          string `json:"markdown"`
	SuggestedFilename string `json:"suggested_filename"`
}

// Sources is everything ExportProject reads. Relationships and Extras tolerate
// nil repos: the corresponding frontmatter sections are simply absent, which
// keeps older callers and tests working while they wire the new stores.
type Sources struct {
	Projects      *project.Repo
	Nodes         *node.Repo
	Entities      *entity.Repo
	Relationships *relationship.Repo
	Extras        Extras
}

// Extras are the metadata stores added by the completeness pass (#83):
// plot threads with beats, margin notes, and fact-book cards.
type Extras struct {
	Threads *thread.Repo
	Beats   *beat.Repo
	Notes   *note.Repo
	Facts   *fact.Repo
}

// appendixHeadings names the two human-readable appendices in the reader's
// language.
//
// These used to be Korean for everyone, so an English writer exporting their
// own novel found "## 등장인물" in the middle of it (#45). The frontmatter above
// them is what import actually restores from; these headings are for a person
// reading the file, which is exactly why they have to be readable.
func appendixHeadings(language string) (characters, relationships string) {
	switch {
	case strings.HasPrefix(language, "en"):
		return "Characters", "Relationships"
	case strings.HasPrefix(language, "ja"):
		return "登場人物", "関係"
	default:
		return "등장인물", "관계"
	}
}

// ExportProject builds a single markdown document from the project tree plus
// frontmatter metadata and readable appendices. Heading levels are derived from
// depth (root containers = ##).
//
// `language` names the appendix headings only; nothing the writer typed is
// translated. Import accepts all three, so a file exported in one language
// still round-trips after the writer switches.
func ExportProject(ctx context.Context, src Sources, projectID, language string) (Payload, error) {
	p, err := src.Projects.Get(ctx, projectID)
	if err != nil {
		return Payload{}, err
	}
	flat, err := src.Nodes.ListByProject(ctx, projectID)
	if err != nil {
		return Payload{}, err
	}
	children := map[string][]node.Node{}
	for _, n := range flat {
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}

	ents, err := src.Entities.ListByProject(ctx, projectID)
	if err != nil {
		return Payload{}, err
	}
	rels := []relationship.Relationship{}
	if src.Relationships != nil {
		rels, err = src.Relationships.ListByProject(ctx, projectID)
		if err != nil {
			return Payload{}, err
		}
	}
	meta, entityNames := buildMetadata(p, ents, rels)

	// Nodes in the exact pre-order the headings are written below. Import
	// aligns positionally against this list to give beats/notes/fact cards
	// their node back after ids are regenerated.
	orderedNodes := collectNodes(children)
	for _, n := range orderedNodes {
		meta.Nodes = append(meta.Nodes, mdmeta.Node{
			ID:      n.ID,
			Label:   n.Label,
			Title:   n.Title,
			Status:  n.Status,
			Summary: n.Summary,
		})
	}

	if err := appendExtras(ctx, src.Extras, projectID, orderedNodes, &meta); err != nil {
		return Payload{}, err
	}

	frontmatter, err := mdmeta.RenderFrontMatter(meta)
	if err != nil {
		return Payload{}, err
	}

	var sb strings.Builder
	sb.WriteString(frontmatter)
	sb.WriteString("# ")
	sb.WriteString(p.Title)
	sb.WriteString("\n\n")

	var walk func(parentKey string, depth int) error
	walk = func(parentKey string, depth int) error {
		for _, n := range children[parentKey] {
			level := depth + 2 // depth 0 → ##
			if level > 3 {
				level = 3
			}
			heading := strings.Repeat("#", level) + " " + n.Label
			if n.Title != "" {
				heading += " — " + n.Title
			}
			sb.WriteString(heading)
			sb.WriteString("\n\n")
			if n.Kind == "leaf" && n.ContentDoc != nil {
				body, err := DocToMarkdown([]byte(*n.ContentDoc))
				if err != nil {
					return fmt.Errorf("export node %s: %w", n.ID, err)
				}
				sb.WriteString(body)
			}
			if err := walk(n.ID, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk("", 0); err != nil {
		return Payload{}, err
	}

	// Human-readable appendices. Import strips these and uses the frontmatter
	// above for exact restoration.
	charactersHeading, relationshipsHeading := appendixHeadings(language)
	if len(ents) > 0 {
		sb.WriteString("## " + charactersHeading + "\n\n")
		for _, e := range ents {
			line := fmt.Sprintf("- **%s** (%s)", e.Name, e.Kind)
			if e.Role != "" {
				line += " · " + e.Role
			}
			if e.Summary != "" {
				line += " — " + e.Summary
			}
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	if len(rels) > 0 {
		sb.WriteString("## " + relationshipsHeading + "\n\n")
		for _, rel := range rels {
			line := fmt.Sprintf("- **%s** → **%s**: %s",
				entityNames[rel.FromID], entityNames[rel.ToID], rel.Label)
			if rel.Notes != "" {
				line += " — " + rel.Notes
			}
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	return Payload{
		Markdown:          collapseBlankLines(sb.String()),
		SuggestedFilename: slugify(p.Title) + ".md",
	}, nil
}

// collectNodes flattens the tree in the same pre-order the markdown walk uses.
func collectNodes(children map[string][]node.Node) []node.Node {
	var out []node.Node
	var walk func(parentKey string)
	walk = func(parentKey string) {
		for _, n := range children[parentKey] {
			out = append(out, n)
			walk(n.ID)
		}
	}
	walk("")
	return out
}

// appendExtras adds threads/beats, margin notes, and fact cards to the
// frontmatter. A nil repo skips its section; node_snapshots, mentions, and
// companion/stat history intentionally stay backup-only.
func appendExtras(ctx context.Context, ex Extras, projectID string, orderedNodes []node.Node, meta *mdmeta.Metadata) error {
	if ex.Threads != nil {
		threads, err := ex.Threads.ListByProject(ctx, projectID, true)
		if err != nil {
			return err
		}
		for _, t := range threads {
			mt := mdmeta.Thread{Name: t.Name, Color: t.Color, Summary: t.Summary}
			if t.ClosedAt != nil {
				mt.ClosedAt = *t.ClosedAt
			}
			if ex.Beats != nil {
				beats, err := ex.Beats.ListByThread(ctx, t.ID)
				if err != nil {
					return err
				}
				for _, b := range beats {
					mb := mdmeta.Beat{Label: b.Label, Description: b.Description, Intensity: b.Intensity}
					if b.NodeID != nil {
						mb.NodeID = *b.NodeID
					}
					mt.Beats = append(mt.Beats, mb)
				}
			}
			meta.Threads = append(meta.Threads, mt)
		}
	}
	if ex.Notes != nil {
		for _, n := range orderedNodes {
			notes, err := ex.Notes.ListForNode(ctx, n.ID)
			if err != nil {
				return err
			}
			for _, nt := range notes {
				meta.Notes = append(meta.Notes, mdmeta.Note{
					NodeID:    nt.NodeID,
					Anchor:    nt.Anchor,
					Body:      nt.Body,
					CreatedAt: nt.CreatedAt,
				})
			}
		}
	}
	if ex.Facts != nil {
		cards, err := ex.Facts.List(ctx, fact.ListFilter{ProjectID: projectID})
		if err != nil {
			return err
		}
		for _, c := range cards {
			mc := mdmeta.FactCard{Claim: c.Claim, Result: c.Result, Status: c.Status, Category: c.Category}
			if c.NodeID != nil {
				mc.NodeID = *c.NodeID
			}
			for _, s := range c.Sources {
				mc.Sources = append(mc.Sources, mdmeta.FactSource{
					URL:        s.URL,
					Title:      s.Title,
					Snippet:    s.Snippet,
					AccessedAt: s.AccessedAt,
				})
			}
			meta.FactCards = append(meta.FactCards, mc)
		}
	}
	return nil
}

func buildMetadata(p project.Project, ents []entity.Entity, rels []relationship.Relationship) (mdmeta.Metadata, map[string]string) {
	meta := mdmeta.Metadata{Version: mdmeta.Version, OutlinePreset: p.OutlinePreset}
	meta.Project = &mdmeta.ProjectMeta{
		Genres:            p.Genres,
		LengthTarget:      p.LengthTarget,
		DefaultPOV:        p.DefaultPOV,
		StyleNotes:        p.StyleNotes,
		Synopsis:          p.Synopsis,
		Outline:           p.Outline,
		EpisodeCharTarget: p.EpisodeCharTarget,
	}
	entityNames := map[string]string{}
	for _, e := range ents {
		entityNames[e.ID] = e.Name
		meta.Entities = append(meta.Entities, mdmeta.Entity{
			ID:         e.ID,
			Kind:       e.Kind,
			Name:       e.Name,
			Aliases:    e.Aliases,
			Role:       e.Role,
			Summary:    e.Summary,
			Attributes: e.Attributes,
		})
	}
	for _, rel := range rels {
		pairID := ""
		if rel.PairID != nil {
			pairID = *rel.PairID
		}
		meta.Relationships = append(meta.Relationships, mdmeta.Relationship{
			ID:       rel.ID,
			PairID:   pairID,
			FromID:   rel.FromID,
			ToID:     rel.ToID,
			FromName: entityNames[rel.FromID],
			ToName:   entityNames[rel.ToID],
			Label:    rel.Label,
			Notes:    rel.Notes,
		})
	}
	return mdmeta.Normalize(meta), entityNames
}

// ExportNode renders a single leaf's body (no heading).
func ExportNode(ctx context.Context, nr *node.Repo, nodeID string) (Payload, error) {
	n, err := nr.Get(ctx, nodeID)
	if err != nil {
		return Payload{}, err
	}
	if n.Kind != "leaf" || n.ContentDoc == nil {
		return Payload{}, fmt.Errorf("export: node %s has no content_doc", nodeID)
	}
	md, err := DocToMarkdown([]byte(*n.ContentDoc))
	if err != nil {
		return Payload{}, err
	}
	name := n.Label
	if name == "" {
		name = "scene"
	}
	return Payload{
		Markdown:          md,
		SuggestedFilename: slugify(name) + ".md",
	}, nil
}

// slugify converts a title to a filename-safe slug. Lowercases ASCII letters,
// replaces runs of whitespace/punctuation with `-`. Korean and other letter
// runes are kept verbatim.
func slugify(s string) string {
	var b strings.Builder
	lastDash := true
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastDash = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		case r > 127: // keep non-ASCII letters (Korean / CJK) verbatim
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "untitled"
	}
	return out
}

// SyncFilename returns the collision-resistant filename used by unattended
// folder and Git sync. Manual exports keep the shorter SuggestedFilename.
func SyncFilename(title, projectID string) string {
	return slugify(title) + "--" + slugify(projectID) + ".md"
}
