package ai

import (
	"context"
	"encoding/json"
	"strings"

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
	projects  *project.Repo
	nodes     *node.Repo
	mentions  *mention.Repo
	threads   *thread.Repo
	beats     *beat.Repo
	notes     *note.Repo
	refresher SummaryRefresher
}

// NewContextBuilder returns a builder that reads from the supplied repos.
func NewContextBuilder(projects *project.Repo, nodes *node.Repo, mentions *mention.Repo, threads *thread.Repo, beats *beat.Repo, notes *note.Repo) *ContextBuilder {
	return &ContextBuilder{
		projects: projects, nodes: nodes, mentions: mentions,
		threads: threads, beats: beats, notes: notes,
		refresher: noopRefresher{},
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

	active, err := b.loadActiveThreads(ctx, nodeID)
	if err != nil {
		return Context{}, err
	}
	_ = nearbyIDs // consumed by Task 5

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
		Hierarchical:  hierarchical,
		Entities:      briefs,
		ActiveThreads: active,
		Notes:         noteBriefs,
		StyleNotes:    proj.StyleNotes,
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
	nearbyIDs := make([]string, 0, 3)

	// Nearby: 2 prior + 1 next (in DFS-leaf order), all excluding cur.
	if curIdx >= 0 {
		for _, j := range []int{curIdx - 2, curIdx - 1, curIdx + 1} {
			if j < 0 || j >= len(leaves) || j == curIdx {
				continue
			}
			lf := leaves[j]
			body := freshLeafSummary(lf)
			if body == "" {
				continue
			}
			out.NearbyLeafSummaries = append(out.NearbyLeafSummaries, SceneSummary{
				NodeID: lf.ID, Label: breadcrumbLabel(byID, lf), Body: body,
			})
			nearbyIDs = append(nearbyIDs, lf.ID)
		}
	}

	// Same chapter: every other leaf under cur.parent_id, minus the nearby IDs
	// and cur itself.
	if cur.ParentID != nil {
		exclude := map[string]bool{cur.ID: true}
		for _, id := range nearbyIDs {
			exclude[id] = true
		}
		for _, sib := range children[*cur.ParentID] {
			if sib.Kind != "leaf" || exclude[sib.ID] {
				continue
			}
			body := freshLeafSummary(sib)
			if body == "" {
				continue
			}
			out.SameChapterSummaries = append(out.SameChapterSummaries, SceneSummary{
				NodeID: sib.ID, Label: breadcrumbLabel(byID, sib), Body: body,
			})
		}
	}

	// Other chapters within same 부: sibling containers of cur's parent.
	if cur.ParentID != nil {
		curChap, hasChap := byID[*cur.ParentID]
		if hasChap && curChap.ParentID != nil {
			for _, sibChap := range children[*curChap.ParentID] {
				if sibChap.Kind != "container" || sibChap.ID == curChap.ID {
					continue
				}
				body := b.refreshAndRead(ctx, sibChap)
				if body == "" {
					continue
				}
				out.OtherChapterSummaries = append(out.OtherChapterSummaries, ChapterSummary{
					NodeID: sibChap.ID, Label: breadcrumbLabel(byID, sibChap), Body: body,
				})
			}
			// Other parts: siblings of curChap's parent (top-level containers).
			curPart := byID[*curChap.ParentID]
			for _, sibPart := range children[parentKeyOf(curPart)] {
				if sibPart.Kind != "container" || sibPart.ID == curPart.ID {
					continue
				}
				body := b.refreshAndRead(ctx, sibPart)
				if body == "" {
					continue
				}
				out.OtherPartSummaries = append(out.OtherPartSummaries, PartSummary{
					NodeID: sibPart.ID, Label: breadcrumbLabel(byID, sibPart), Body: body,
				})
			}
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

func parentKeyOf(n node.Node) string {
	if n.ParentID == nil {
		return ""
	}
	return *n.ParentID
}

func breadcrumbLabel(byID map[string]node.Node, n node.Node) string {
	parts := []string{n.Label}
	cur := n
	for cur.ParentID != nil {
		p, ok := byID[*cur.ParentID]
		if !ok {
			break
		}
		parts = append([]string{p.Label}, parts...)
		cur = p
	}
	return strings.Join(parts, " / ")
}

// trimToBudget drops trailing entries from the lowest-priority sections first
// (other_part → other_chapter → same_chapter → nearby) until the estimated
// rendered size is under maxChars.
func trimToBudget(h *HierarchicalContext, maxChars int) {
	estimate := func() int {
		total := len(h.ProjectSynopsis)
		for _, s := range h.NearbyLeafSummaries {
			total += len(s.Label) + len(s.Body) + 4
		}
		for _, s := range h.SameChapterSummaries {
			total += len(s.Label) + len(s.Body) + 4
		}
		for _, s := range h.OtherChapterSummaries {
			total += len(s.Label) + len(s.Body) + 4
		}
		for _, s := range h.OtherPartSummaries {
			total += len(s.Label) + len(s.Body) + 4
		}
		return total
	}
	for estimate() > maxChars && len(h.OtherPartSummaries) > 0 {
		h.OtherPartSummaries = h.OtherPartSummaries[:len(h.OtherPartSummaries)-1]
	}
	for estimate() > maxChars && len(h.OtherChapterSummaries) > 0 {
		h.OtherChapterSummaries = h.OtherChapterSummaries[:len(h.OtherChapterSummaries)-1]
	}
	for estimate() > maxChars && len(h.SameChapterSummaries) > 0 {
		h.SameChapterSummaries = h.SameChapterSummaries[:len(h.SameChapterSummaries)-1]
	}
	for estimate() > maxChars && len(h.NearbyLeafSummaries) > 0 {
		h.NearbyLeafSummaries = h.NearbyLeafSummaries[:len(h.NearbyLeafSummaries)-1]
	}
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
