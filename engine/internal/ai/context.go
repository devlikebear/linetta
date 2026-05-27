package ai

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

const prevSummaryMaxRunes = 300
const activeThreadsMaxChars = 1500
const recentBeatsPerThread = 5

// ContextBuilder gathers the Context payload from the repos.
type ContextBuilder struct {
	projects *project.Repo
	nodes    *node.Repo
	mentions *mention.Repo
	threads  *thread.Repo
	beats    *beat.Repo
	notes    *note.Repo
}

// NewContextBuilder returns a builder that reads from the supplied repos.
func NewContextBuilder(projects *project.Repo, nodes *node.Repo, mentions *mention.Repo, threads *thread.Repo, beats *beat.Repo, notes *note.Repo) *ContextBuilder {
	return &ContextBuilder{projects: projects, nodes: nodes, mentions: mentions, threads: threads, beats: beats, notes: notes}
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
	// Prefer the LLM-cached summary when fresh; fall back to the 300-rune trim.
	prevSummary := ""
	if prev != nil {
		if prev.Summary != "" && prev.SummaryForVersion == prev.ContentVersion {
			prevSummary = prev.Summary
		} else {
			prevSummary = trimRunes(docToPlainText(prev.ContentDoc), prevSummaryMaxRunes)
		}
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

	active, err := b.loadActiveThreads(ctx, nodeID)
	if err != nil {
		return Context{}, err
	}

	var noteBriefs []NoteBrief
	if b.notes != nil {
		ns, err := b.notes.ListForNode(ctx, nodeID)
		if err != nil {
			return Context{}, err
		}
		noteBriefs = make([]NoteBrief, 0, len(ns))
		for _, n := range ns {
			noteBriefs = append(noteBriefs, NoteBrief{Anchor: n.Anchor, Body: n.Body})
		}
	}

	return Context{
		ProjectID:     proj.ID,
		NodeID:        n.ID,
		SceneLabel:    n.Label,
		SceneText:     sceneText,
		PrevSummary:   prevSummary,
		Entities:      briefs,
		ActiveThreads: active,
		Notes:         noteBriefs,
		StyleNotes:    proj.StyleNotes,
		UserPrompt:    prompt,
		Options:       opts,
	}, nil
}

func (b *ContextBuilder) loadActiveThreads(ctx context.Context, nodeID string) ([]ActiveThread, error) {
	bs, err := b.beats.ListByNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	// Collect unique thread ids preserving first-seen order.
	seen := map[string]bool{}
	var threadIDs []string
	for _, bt := range bs {
		if !seen[bt.ThreadID] {
			seen[bt.ThreadID] = true
			threadIDs = append(threadIDs, bt.ThreadID)
		}
	}
	out := make([]ActiveThread, 0, len(threadIDs))
	for _, tid := range threadIDs {
		th, err := b.threads.Get(ctx, tid)
		if err != nil {
			continue // benign: stale row
		}
		if th.ClosedAt != nil {
			continue
		}
		all, err := b.beats.ListByThread(ctx, tid)
		if err != nil {
			return nil, err
		}
		// Take last N by ordinal.
		start := 0
		if len(all) > recentBeatsPerThread {
			start = len(all) - recentBeatsPerThread
		}
		recents := make([]BeatBrief, 0, len(all)-start)
		for _, x := range all[start:] {
			recents = append(recents, BeatBrief{Label: x.Label, Ordinal: x.Ordinal})
		}
		out = append(out, ActiveThread{
			Name: th.Name, Color: th.Color, Summary: th.Summary, RecentBeats: recents,
		})
	}
	return capActiveThreads(out, activeThreadsMaxChars), nil
}

// capActiveThreads drops trailing entries (whole) until the rough rendered
// size is under maxChars. Cheap approximation: name + summary + each beat label.
func capActiveThreads(in []ActiveThread, maxChars int) []ActiveThread {
	total := 0
	for i, t := range in {
		size := len(t.Name) + len(t.Summary) + 8
		for _, b := range t.RecentBeats {
			size += len(b.Label) + 8
		}
		if total+size > maxChars && i > 0 {
			return in[:i]
		}
		total += size
	}
	return in
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
