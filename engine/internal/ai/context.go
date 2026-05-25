package ai

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
)

const prevSummaryMaxRunes = 300

// ContextBuilder gathers the Context payload from the repos.
type ContextBuilder struct {
	projects *project.Repo
	nodes    *node.Repo
	mentions *mention.Repo
}

// NewContextBuilder returns a builder that reads from the supplied repos.
func NewContextBuilder(projects *project.Repo, nodes *node.Repo, mentions *mention.Repo) *ContextBuilder {
	return &ContextBuilder{projects: projects, nodes: nodes, mentions: mentions}
}

// Build assembles the context for the given leaf node + user prompt + options.
func (b *ContextBuilder) Build(ctx context.Context, nodeID, prompt string, opts Options) (Context, error) {
	n, err := b.nodes.Get(ctx, nodeID)
	if err != nil {
		return Context{}, err
	}
	proj, err := b.projects.Get(ctx, n.ProjectID)
	if err != nil {
		return Context{}, err
	}

	sceneText := docToPlainText(n.ContentDoc)

	prev, err := b.findPreviousLeaf(ctx, n)
	if err != nil {
		return Context{}, err
	}
	prevSummary := ""
	if prev != nil {
		prevSummary = trimRunes(docToPlainText(prev.ContentDoc), prevSummaryMaxRunes)
	}

	ents, err := b.mentions.ListEntitiesForNode(ctx, nodeID)
	if err != nil {
		return Context{}, err
	}
	briefs := make([]EntityBrief, 0, len(ents))
	for _, e := range ents {
		briefs = append(briefs, EntityBrief{
			Name: e.Name, Kind: e.Kind, Role: e.Role, Summary: e.Summary, Attributes: e.Attributes,
		})
	}

	return Context{
		ProjectID:   proj.ID,
		NodeID:      n.ID,
		SceneLabel:  n.Label,
		SceneText:   sceneText,
		PrevSummary: prevSummary,
		Entities:    briefs,
		StyleNotes:  proj.StyleNotes,
		UserPrompt:  prompt,
		Options:     opts,
	}, nil
}

// findPreviousLeaf returns the previous leaf in DFS order (within the project),
// or nil if `cur` is the first leaf.
func (b *ContextBuilder) findPreviousLeaf(ctx context.Context, cur node.Node) (*node.Node, error) {
	all, err := b.nodes.ListByProject(ctx, cur.ProjectID)
	if err != nil {
		return nil, err
	}
	// Build a parent_id → []children index, then DFS roots in ordinal order.
	children := map[string][]node.Node{}
	for _, n := range all {
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}
	var leaves []node.Node
	var walk func(parent string)
	walk = func(parent string) {
		for _, c := range children[parent] {
			if c.Kind == "leaf" {
				leaves = append(leaves, c)
			}
			walk(c.ID)
		}
	}
	walk("")
	for i, l := range leaves {
		if l.ID == cur.ID && i > 0 {
			return &leaves[i-1], nil
		}
	}
	return nil, nil
}

// docToPlainText walks a Tiptap doc and concatenates text content. Mentions are
// rendered as `@label`. Block boundaries become newlines.
func docToPlainText(rawDoc *string) string {
	if rawDoc == nil || *rawDoc == "" {
		return ""
	}
	var any interface{}
	if err := json.Unmarshal([]byte(*rawDoc), &any); err != nil {
		return ""
	}
	var sb stringBuilder
	walkDoc(any, &sb)
	return sb.String()
}

type stringBuilder struct{ buf []byte }

func (b *stringBuilder) WriteString(s string) { b.buf = append(b.buf, s...) }
func (b *stringBuilder) String() string       { return string(b.buf) }

func walkDoc(v interface{}, sb *stringBuilder) {
	switch t := v.(type) {
	case map[string]interface{}:
		kind, _ := t["type"].(string)
		if kind == "mention" {
			attrs, _ := t["attrs"].(map[string]interface{})
			label, _ := attrs["label"].(string)
			if label != "" {
				sb.WriteString("@")
				sb.WriteString(label)
			}
			return
		}
		if kind == "text" {
			if s, ok := t["text"].(string); ok {
				sb.WriteString(s)
			}
			return
		}
		// Block-level node: recurse children, then add a newline.
		if content, ok := t["content"].([]interface{}); ok {
			for _, c := range content {
				walkDoc(c, sb)
			}
		}
		if kind == "paragraph" || kind == "heading" || kind == "blockquote" {
			sb.WriteString("\n")
		}
	case []interface{}:
		for _, c := range t {
			walkDoc(c, sb)
		}
	}
}

func trimRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
