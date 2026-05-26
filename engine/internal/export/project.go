package export

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
)

// Payload is the wire-shape returned by both ExportProject and ExportNode.
type Payload struct {
	Markdown          string `json:"markdown"`
	SuggestedFilename string `json:"suggested_filename"`
}

// ExportProject builds a single markdown document from the project tree plus an
// entities appendix. Heading levels are derived from depth (root containers = ##).
func ExportProject(ctx context.Context, pr *project.Repo, nr *node.Repo, er *entity.Repo, projectID string) (Payload, error) {
	p, err := pr.Get(ctx, projectID)
	if err != nil {
		return Payload{}, err
	}
	flat, err := nr.ListByProject(ctx, projectID)
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

	var sb strings.Builder
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

	// Entities appendix.
	ents, err := er.Search(ctx, projectID, "", 50)
	if err != nil {
		return Payload{}, err
	}
	if len(ents) > 0 {
		sb.WriteString("## 등장인물\n\n")
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

	return Payload{
		Markdown:          collapseBlankLines(sb.String()),
		SuggestedFilename: slugify(p.Title) + ".md",
	}, nil
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
