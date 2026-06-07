package ai

import (
	"context"
	"encoding/json"
	"fmt"
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
const coreEntityContextLimit = 12

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
// threads and beats are used only to construct the internal *plot.Builder (not
// stored on ContextBuilder), so the AI context and the plot spine-panel handler
// share the same neighbor logic without coupling the two callers.
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

// Build assembles the context for the given leaf node + user prompt + options,
// then removes sections disabled by Options.Context.
func (b *ContextBuilder) Build(ctx context.Context, nodeID, prompt, selectionText string, opts Options) (Context, error) {
	c, err := b.BuildFull(ctx, nodeID, prompt, selectionText, opts)
	if err != nil {
		return Context{}, err
	}
	return ApplyContextSelection(c), nil
}

// BuildFull assembles every available context section without applying
// Options.Context. Used by preview so disabled sections can still be inspected.
func (b *ContextBuilder) BuildFull(ctx context.Context, nodeID, prompt, selectionText string, opts Options) (Context, error) {
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
	coreEnts, err := b.mentions.ListCoreEntitiesForProject(ctx, n.ProjectID, coreEntityContextLimit)
	if err != nil {
		return Context{}, err
	}
	ents = mergeEntitiesByID(ents, coreEnts)
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
			Synopsis:     proj.Synopsis,
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

func mergeEntitiesByID(primary, extra []entity.Entity) []entity.Entity {
	if len(extra) == 0 {
		return primary
	}
	seen := make(map[string]bool, len(primary)+len(extra))
	out := make([]entity.Entity, 0, len(primary)+len(extra))
	for _, e := range primary {
		if e.ID == "" || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		out = append(out, e)
	}
	for _, e := range extra {
		if e.ID == "" || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		out = append(out, e)
	}
	return out
}

// ApplyContextSelection removes context sections that the writer unchecked in
// the AI panel. UserPrompt and SelectionText stay intact because they are the
// requested operation target, not ambient context.
func ApplyContextSelection(c Context) Context {
	s := c.Options.Context
	if !s.Enabled(ContextKeyCurrentScene) {
		c.SceneText = ""
	}
	if !s.Enabled(ContextKeyOverview) {
		c.Outline = ""
	}
	if !s.Enabled(ContextKeySynopsis) {
		c.Project.Synopsis = ""
		c.Hierarchical.ProjectSynopsis = ""
	}
	if !s.Enabled(ContextKeyNearbyScenes) {
		c.Hierarchical.NearbyLeafSummaries = nil
	}
	if !s.Enabled(ContextKeyRelatedScenes) {
		c.RelatedScenes = nil
	}
	if !s.Enabled(ContextKeyPlot) {
		c.Plot = plot.Spine{}
	}
	if !s.Enabled(ContextKeyEntities) {
		c.Entities = nil
	}
	if !s.Enabled(ContextKeyRelationships) {
		c.Relationships = nil
	}
	if !s.Enabled(ContextKeyNotes) {
		c.Notes = nil
	}
	if !s.Enabled(ContextKeyProjectMeta) {
		c.Project = ProjectMeta{Synopsis: c.Project.Synopsis}
	}
	if !s.Enabled(ContextKeyStyleNotes) {
		c.StyleNotes = ""
	}
	return c
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

// DeriveProjectSynopsis builds the rollup synopsis from root containers. When
// refresh is true, cached root summaries are invalidated first so the injected
// SummaryRefresher rewrites them before this method returns.
func (b *ContextBuilder) DeriveProjectSynopsis(ctx context.Context, projectID string, refresh bool) (string, error) {
	all, err := b.nodes.ListByProject(ctx, projectID)
	if err != nil {
		return "", err
	}
	var roots []node.Node
	for _, n := range all {
		if n.ParentID == nil && n.Kind == node.KindContainer {
			if refresh {
				_ = b.nodes.SetSummary(ctx, n.ID, "", 0)
				n.Summary = ""
				n.SummaryForVersion = 0
			}
			roots = append(roots, n)
		}
	}
	if len(roots) == 0 {
		return "", nil
	}
	if len(roots) == 1 {
		return b.refreshAndRead(ctx, roots[0]), nil
	}
	var sb strings.Builder
	for _, root := range roots {
		body := b.refreshAndRead(ctx, root)
		if body == "" {
			continue
		}
		sb.WriteString(root.Label)
		sb.WriteString(": ")
		sb.WriteString(body)
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String()), nil
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
	out := make([]RelationBrief, 0, len(rels))
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
		HasSynopsis:       strings.TrimSpace(c.Project.Synopsis) != "",
		RelatedScenes:     len(c.RelatedScenes),
		Entities:          len(c.Entities),
		Relationships:     len(c.Relationships),
		PlotBeats:         plotBeats,
		Notes:             len(c.Notes),
		ProjectMetaFields: projectMeta,
		HasStyleNotes:     strings.TrimSpace(c.StyleNotes) != "",
	}
}

// PreviewFromContext renders inspectable sections from an unfiltered Context.
// It does not mutate c, and selected state is derived from the supplied
// selection so the UI can preview disabled sections before a run.
func PreviewFromContext(c Context, selection ContextSelection) ContextPreview {
	sections := []PreviewSection{}
	add := func(id ContextKey, label string, count int, preview string, forcePresent bool) {
		present := forcePresent || count > 0 || strings.TrimSpace(preview) != ""
		selected := present && selection.Enabled(id)
		section := PreviewSection{
			ID:       id,
			Label:    label,
			Present:  present,
			Selected: selected,
			Count:    count,
			Preview:  trimRunes(strings.TrimSpace(preview), 1200),
		}
		sections = append(sections, section)
	}

	add(ContextKeyCurrentScene, "현재 씬 본문", 1, c.SceneText, true)

	overview := strings.TrimSpace(c.Outline)
	add(ContextKeyOverview, "작품 개요", boolCount(overview != ""), overview, false)

	synopsis := strings.TrimSpace(c.Project.Synopsis)
	add(ContextKeySynopsis, "작품 시놉시스", boolCount(synopsis != ""), synopsis, false)

	add(ContextKeyNearbyScenes, "직전·직후 씬 발췌", len(c.Hierarchical.NearbyLeafSummaries), renderSceneSummariesPreview(c.Hierarchical.NearbyLeafSummaries), false)
	add(ContextKeyRelatedScenes, "관련 과거 씬 (멘션 RAG)", len(c.RelatedScenes), renderSceneSummariesPreview(c.RelatedScenes), false)
	add(ContextKeyPlot, "플롯 (스토리라인&비트)", countPlotBeats(c.Plot), renderPlotPreview(c.Plot), false)
	add(ContextKeyEntities, "세계관 요소", len(c.Entities), renderEntitiesPreview(c.Entities), false)
	add(ContextKeyRelationships, "관계", len(c.Relationships), renderRelationshipsPreview(c.Relationships), false)
	add(ContextKeyNotes, "작가 주석", len(c.Notes), renderNotesPreview(c.Notes), false)
	if meta := renderProjectMeta(c.Project); meta != "" {
		add(ContextKeyProjectMeta, "작품 설정 (장르/분량/시점)", countProjectMeta(c.Project), meta, false)
	} else {
		add(ContextKeyProjectMeta, "작품 설정 (장르/분량/시점)", 0, "", false)
	}
	add(ContextKeyStyleNotes, "작가 style notes", boolCount(strings.TrimSpace(c.StyleNotes) != ""), c.StyleNotes, false)

	selectedCount := 0
	for _, section := range sections {
		if !section.Selected {
			continue
		}
		if section.Count > 0 {
			selectedCount += section.Count
		} else {
			selectedCount++
		}
	}

	return ContextPreview{
		PreviewCounts:     CountsFromContext(c),
		Sections:          sections,
		SelectedItemCount: selectedCount,
	}
}

func boolCount(ok bool) int {
	if ok {
		return 1
	}
	return 0
}

func countProjectMeta(m ProjectMeta) int {
	n := 0
	if len(m.Genres) > 0 {
		n++
	}
	if m.LengthTarget != "" {
		n++
	}
	if m.DefaultPOV != "" {
		n++
	}
	return n
}

func countPlotBeats(spine plot.Spine) int {
	n := len(spine.Current.Beats)
	if spine.Prev != nil {
		n += len(spine.Prev.Beats)
	}
	if spine.Next != nil {
		n += len(spine.Next.Beats)
	}
	return n
}

func renderSceneSummariesPreview(scenes []SceneSummary) string {
	var b strings.Builder
	for _, s := range scenes {
		b.WriteString(fmt.Sprintf("- [%s] %s\n", s.Label, s.Body))
	}
	return b.String()
}

func renderPlotPreview(spine plot.Spine) string {
	var b strings.Builder
	writeScene := func(tag string, s *plot.SceneBeats) {
		if s == nil || len(s.Beats) == 0 {
			return
		}
		b.WriteString(tag)
		if s.Label != "" {
			b.WriteString(" ")
			b.WriteString(s.Label)
		}
		b.WriteString("\n")
		for _, bt := range s.Beats {
			line := fmt.Sprintf("  · [%s] #%d %s", bt.ThreadName, bt.Ordinal, bt.Label)
			if strings.TrimSpace(bt.Description) != "" {
				line += " — " + bt.Description
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	writeScene("[이전 씬]", spine.Prev)
	writeScene("[현재 씬]", &spine.Current)
	writeScene("[다음 씬]", spine.Next)
	return b.String()
}

func renderEntitiesPreview(entities []EntityBrief) string {
	var b strings.Builder
	for _, e := range entities {
		b.WriteString(fmt.Sprintf("- @%s — %s", e.Name, kindLabel(e.Kind)))
		if e.Role != "" {
			b.WriteString(" / " + e.Role)
		}
		if e.Summary != "" {
			b.WriteString(": " + e.Summary)
		}
		if len(e.Recent) > 0 {
			b.WriteString("\n")
			for _, line := range e.Recent {
				b.WriteString("  · ")
				b.WriteString(line)
				b.WriteString("\n")
			}
		} else {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func renderRelationshipsPreview(relationships []RelationBrief) string {
	var b strings.Builder
	for _, r := range relationships {
		arrow := "→"
		if r.Bidirectional {
			arrow = "↔"
		}
		b.WriteString(fmt.Sprintf("- %s %s %s: %s", r.From, arrow, r.To, r.Label))
		if strings.TrimSpace(r.Notes) != "" {
			b.WriteString(" — ")
			b.WriteString(r.Notes)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func renderNotesPreview(notes []NoteBrief) string {
	var b strings.Builder
	for _, n := range notes {
		b.WriteString("- ")
		b.WriteString(n.Body)
		b.WriteString("\n")
	}
	return b.String()
}
