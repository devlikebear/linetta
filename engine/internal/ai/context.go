package ai

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

const prevSummaryMaxRunes = 300
const hierarchicalMaxChars = 2500

// SummaryRefresher is what ContextBuilder calls when an ancestor container has
// a stale summary. *summarizer.Summarizer satisfies this via its RefreshNow
// method (declared there to avoid an ai → summarizer import cycle).
type SummaryRefresher interface {
	RefreshNow(ctx context.Context, nodeID string)
}

type noopRefresher struct{}

func (noopRefresher) RefreshNow(context.Context, string) {}

// ContextBuilder gathers the Context payload from the repos.
type ContextBuilder struct {
	projects      *project.Repo
	nodes         *node.Repo
	mentions      *mention.Repo
	notes         *note.Repo
	relationships *relationship.Repo
	plot          *plot.Builder
	refresher     SummaryRefresher
}

// NewContextBuilder returns a builder that reads from the supplied repos.
func NewContextBuilder(projects *project.Repo, nodes *node.Repo, mentions *mention.Repo, threads *thread.Repo, beats *beat.Repo, notes *note.Repo, relationships *relationship.Repo) *ContextBuilder {
	return &ContextBuilder{
		projects:      projects,
		nodes:         nodes,
		mentions:      mentions,
		notes:         notes,
		relationships: relationships,
		plot:          plot.NewBuilder(nodes, beats, threads),
		refresher:     noopRefresher{},
	}
}

// WithSummaryRefresher returns a copy of b that synchronously refreshes stale
// container summaries on hierarchical context loads.
func (b *ContextBuilder) WithSummaryRefresher(r SummaryRefresher) *ContextBuilder {
	cp := *b
	if r == nil {
		cp.refresher = noopRefresher{}
	} else {
		cp.refresher = r
	}
	return &cp
}

// Build assembles the context for the given leaf node + user prompt + options.
func (b *ContextBuilder) Build(ctx context.Context, nodeID, prompt, selectionText string, opts Options) (Context, error) {
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
		recent, err := b.mentions.RecentSummariesForEntity(ctx, e.ID, nodeID, 5)
		if err != nil {
			return Context{}, err
		}
		briefs = append(briefs, EntityBrief{
			Name: e.Name, Kind: e.Kind, Role: e.Role, Summary: e.Summary, Attributes: e.Attributes,
			Recent: recent,
		})
	}

	hierarchical, nearbyIDs, err := b.loadHierarchicalContext(ctx, n)
	if err != nil {
		return Context{}, err
	}

	related, err := b.loadRelatedScenes(ctx, n, nearbyIDs)
	if err != nil {
		return Context{}, err
	}

	spine, err := b.plot.Build(ctx, nodeID)
	if err != nil {
		return Context{}, err
	}
	relations, err := b.loadRelationships(ctx, proj.ID, ents)
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
		ProjectID:   proj.ID,
		NodeID:      n.ID,
		SceneLabel:  n.Label,
		SceneText:   sceneText,
		PrevSummary: prevSummary,
		Project: ProjectMeta{
			Genres:       proj.Genres,
			LengthTarget: proj.LengthTarget,
			DefaultPOV:   proj.DefaultPOV,
		},
		Outline:       proj.Outline,
		Hierarchical:  hierarchical,
		RelatedScenes: related,
		Entities:      briefs,
		Relationships: relations,
		Plot:          spine,
		Notes:         noteBriefs,
		StyleNotes:    proj.StyleNotes,
		SelectionText: selectionText,
		UserPrompt:    prompt,
		Options:       opts,
	}, nil
}

// loadHierarchicalContext gathers layer-1 (container rollup) data for cur.
// On stale container hits it calls b.refresher.RefreshNow synchronously so the
// returned summaries are fresh. The second return value is the set of leaf
// node IDs placed in NearbyLeafSummaries, used by Task 5's topology RAG for
// set-based exclusion.
func (b *ContextBuilder) loadHierarchicalContext(ctx context.Context, cur node.Node) (HierarchicalContext, []string, error) {
	all, err := b.nodes.ListByProject(ctx, cur.ProjectID)
	if err != nil {
		return HierarchicalContext{}, nil, err
	}
	byID := make(map[string]node.Node, len(all))
	children := map[string][]node.Node{}
	for _, n := range all {
		byID[n.ID] = n
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}
	// DFS leaves in document order.
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

	curIdx := -1
	for i, l := range leaves {
		if l.ID == cur.ID {
			curIdx = i
			break
		}
	}

	out := HierarchicalContext{}
	nearbyIDs := make([]string, 0, 2)

	// Nearby: 1 prior + 1 next (in DFS-leaf order), all excluding cur.
	if curIdx >= 0 {
		for _, j := range []int{curIdx - 1, curIdx + 1} {
			if j < 0 || j >= len(leaves) || j == curIdx {
				continue
			}
			lf := leaves[j]
			body := freshLeafSummary(lf)
			if body == "" {
				continue
			}
			out.NearbyLeafSummaries = append(out.NearbyLeafSummaries, SceneSummary{
				NodeID: lf.ID, Label: node.BreadcrumbLabel(byID, lf), Body: body,
			})
			nearbyIDs = append(nearbyIDs, lf.ID)
		}
	}

	// Project synopsis: if there's exactly one root container, use its summary.
	// If there are multiple, concatenate "Label: body" lines as a virtual root.
	rootContainers := []node.Node{}
	for _, n := range children[""] {
		if n.Kind == "container" {
			rootContainers = append(rootContainers, n)
		}
	}
	if len(rootContainers) == 1 {
		out.ProjectSynopsis = b.refreshAndRead(ctx, rootContainers[0])
	} else if len(rootContainers) > 1 {
		var sb strings.Builder
		for _, rc := range rootContainers {
			body := b.refreshAndRead(ctx, rc)
			if body == "" {
				continue
			}
			sb.WriteString(rc.Label)
			sb.WriteString(": ")
			sb.WriteString(body)
			sb.WriteString("\n")
		}
		out.ProjectSynopsis = strings.TrimSpace(sb.String())
	}

	trimToBudget(&out, hierarchicalMaxChars)

	return out, nearbyIDs, nil
}

// loadRelatedScenes runs the co-mention topology-RAG query and post-filters
// the result set against excludeIDs (typically Nearby IDs + cur.ID). Returns
// up to 3 SceneSummary entries with breadcrumb labels.
func (b *ContextBuilder) loadRelatedScenes(ctx context.Context, cur node.Node, excludeIDs []string) ([]SceneSummary, error) {
	// Fetch top-K + small buffer so the post-filter can still yield 3.
	results, err := b.mentions.CoMentionLeaves(ctx, cur.ID, 2+len(excludeIDs))
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	excl := make(map[string]bool, len(excludeIDs)+1)
	excl[cur.ID] = true
	for _, id := range excludeIDs {
		excl[id] = true
	}
	all, err := b.nodes.ListByProject(ctx, cur.ProjectID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]node.Node, len(all))
	for _, n := range all {
		byID[n.ID] = n
	}
	out := make([]SceneSummary, 0, 2)
	for _, r := range results {
		if excl[r.NodeID] {
			continue
		}
		n, ok := byID[r.NodeID]
		if !ok {
			continue
		}
		if n.Summary == "" {
			continue
		}
		out = append(out, SceneSummary{
			NodeID: n.ID, Label: node.BreadcrumbLabel(byID, n), Body: n.Summary,
		})
		if len(out) >= 2 {
			break
		}
	}
	return out, nil
}

// freshLeafSummary returns the cached summary if the version matches; otherwise
// it falls back to a 300-rune trim of the plaintext body. Empty string if no
// content at all (e.g. the leaf was just created and has no doc yet).
func freshLeafSummary(n node.Node) string {
	if n.Summary != "" && n.SummaryForVersion == n.ContentVersion {
		return n.Summary
	}
	if n.ContentDoc != nil {
		plain := docToPlainText(n.ContentDoc)
		if plain == "" {
			return ""
		}
		return trimRunes(plain, prevSummaryMaxRunes)
	}
	return ""
}

// refreshAndRead returns n.Summary if fresh; otherwise it calls the refresher
// synchronously and re-reads the row. Empty string is returned if the refresh
// failed to land (better than a stale string).
func (b *ContextBuilder) refreshAndRead(ctx context.Context, n node.Node) string {
	if n.Summary != "" && n.SummaryForVersion == n.ContentVersion {
		return n.Summary
	}
	if b.refresher != nil {
		b.refresher.RefreshNow(ctx, n.ID)
	}
	got, err := b.nodes.Get(ctx, n.ID)
	if err != nil {
		return ""
	}
	if got.Summary != "" && got.SummaryForVersion == got.ContentVersion {
		return got.Summary
	}
	return ""
}

// loadRelationships returns relationships whose both endpoints appear in the
// current scene's mentioned entities. Pairs are deduped by pair_id (first wins).
func (b *ContextBuilder) loadRelationships(ctx context.Context, projectID string, ents []entity.Entity) ([]RelationBrief, error) {
	if b.relationships == nil || len(ents) == 0 {
		return nil, nil
	}
	rels, err := b.relationships.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	nameByID := make(map[string]string, len(ents))
	present := make(map[string]bool, len(ents))
	for _, e := range ents {
		nameByID[e.ID] = e.Name
		present[e.ID] = true
	}
	seenPair := map[string]bool{}
	out := make([]RelationBrief, 0)
	for _, r := range rels {
		if !present[r.FromID] || !present[r.ToID] {
			continue
		}
		bidir := false
		if r.PairID != nil && *r.PairID != "" {
			if seenPair[*r.PairID] {
				continue
			}
			seenPair[*r.PairID] = true
			bidir = true
		}
		out = append(out, RelationBrief{
			From: nameByID[r.FromID], To: nameByID[r.ToID],
			Label: r.Label, Notes: r.Notes, Bidirectional: bidir,
		})
	}
	return out, nil
}

// trimToBudget drops trailing nearby entries until the estimated rendered size
// is under maxChars.
func trimToBudget(h *HierarchicalContext, maxChars int) {
	estimate := func() int {
		total := len(h.ProjectSynopsis)
		for _, s := range h.NearbyLeafSummaries {
			total += len(s.Label) + len(s.Body) + 4
		}
		return total
	}
	for estimate() > maxChars && len(h.NearbyLeafSummaries) > 0 {
		h.NearbyLeafSummaries = h.NearbyLeafSummaries[:len(h.NearbyLeafSummaries)-1]
	}
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

// CountsFromContext extracts a PreviewCounts from a fully-built Context.
// Pure function — no I/O. Whitespace-only strings are treated as empty.
func CountsFromContext(c Context) PreviewCounts {
	projectMeta := 0
	if len(c.Project.Genres) > 0 {
		projectMeta++
	}
	if c.Project.LengthTarget != "" {
		projectMeta++
	}
	if c.Project.DefaultPOV != "" {
		projectMeta++
	}
	plotBeats := len(c.Plot.Current.Beats)
	if c.Plot.Prev != nil {
		plotBeats += len(c.Plot.Prev.Beats)
	}
	if c.Plot.Next != nil {
		plotBeats += len(c.Plot.Next.Beats)
	}
	return PreviewCounts{
		NearbyScenes:      len(c.Hierarchical.NearbyLeafSummaries),
		HasOutline:        strings.TrimSpace(c.Outline) != "",
		HasSynopsis:       strings.TrimSpace(c.Hierarchical.ProjectSynopsis) != "",
		RelatedScenes:     len(c.RelatedScenes),
		Entities:          len(c.Entities),
		Relationships:     len(c.Relationships),
		PlotBeats:         plotBeats,
		Notes:             len(c.Notes),
		ProjectMetaFields: projectMeta,
		HasStyleNotes:     strings.TrimSpace(c.StyleNotes) != "",
	}
}
