package storycontext

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/illustref"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

const prevSummaryMaxRunes = 300
const coreEntityContextLimit = 12

// Budgets for the parts of the brief that grow with the work, in runes: a
// Korean summary is three bytes a character, so a byte budget would give a
// Korean work a third of what it means to.
const (
	// nearbyMaxRunes bounds the adjacent-scene section.
	nearbyMaxRunes = 2500
	// partSummariesMaxRunes bounds the section that holds every part's arc.
	// The part being written keeps partSummaryCurrentMaxRunes of it; the rest
	// is shared among the others, so a long series still shows its whole shape.
	partSummariesMaxRunes      = 2500
	partSummaryCurrentMaxRunes = 1200
	partSummaryMinRunes        = 80
	// upcomingPlanLimit is how many unwritten scenes past the next one the
	// brief carries the plan of.
	upcomingPlanLimit = 2
	// An entity's dossier is a pointer back into the story, not a recap of it:
	// a few short lines, with a cap over all entities together. Without these
	// the dossier was four fifths of a ten-episode work's brief.
	entityRecentLimit        = 3
	entityRecentLineMaxRunes = 160
	entityRecentMaxRunes     = 6000
)

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
	factSource          FactSource
	memorySource        MemorySource
	curatedMemorySource CuratedMemorySource
	skillSource         SkillSource
	referenceSource     ReferenceSource
	directionSource     DirectionSource
	projects            *project.Repo
	nodes               *node.Repo
	mentions            *mention.Repo
	notes               *note.Repo
	relationships       *relationship.Repo
	plot                *plot.Builder
	refresher           SummaryRefresher
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
// FactSource supplies Fact Book cards for the brief. Optional: without it
// the Facts section stays empty. Today only the companion gathers facts; this
// interface completes the existing ContextSelection toggles so the MCP story
// brief carries them too (pivot #47, Task 1.3).
type FactSource interface {
	ContextFacts(ctx context.Context, projectID, nodeID string) ([]FactBrief, error)
}

// MemorySource supplies remembered writer/world facts for the brief. Optional.
type MemorySource interface {
	ContextMemories(projectID string) []string
}

// CuratedMemorySource supplies the two budgeted memory documents. Unlike
// MemorySource it takes a context: these come from the database, and a brief
// built during shutdown has to be able to give up.
type CuratedMemorySource interface {
	CuratedMemory(ctx context.Context, projectID string) (writerProfile, workNotes string)
}

// SkillSource supplies the skills list for the brief: the writer's own and
// this work's, already reduced to what may reach a model. Optional — without
// it the Skills section stays empty.
//
// The contract, which this package re-checks none of, matching
// agent.SkillSource's for the system-prompt list:
//
//   - Enabled only. A skill the writer switched off must not be listed.
//   - Guard-passed. agentskills.Store.List runs agentskills.Guard on every
//     entry and reports a failure as a Diagnostic instead of returning the
//     skill, so a guard-failing skill is SKIPPED rather than costing the
//     whole block — the decision the prompt's list already made, for the
//     same reason: one skill an invisible character got into must not
//     silence the other thirty-nine.
//   - Names and descriptions only. SkillBrief has no body to give.
//
// It takes a context for the same reason CuratedMemorySource does: these come
// off disk, and a brief built during shutdown has to be able to give up.
type SkillSource interface {
	ContextSkills(ctx context.Context, projectID string) []SkillBrief
}

// ReferenceSource supplies writer-attached reference material. Optional.
type ReferenceSource interface {
	ContextReferences(ctx context.Context, projectID, nodeID string) ([]ReferenceBrief, error)
}

// DirectionFocus is the work's current focus, as DirectionSource reports it.
type DirectionFocus struct {
	Body          string
	Stale         bool
	EpisodesSince int
}

// DirectionSceneIntent is one scene's intent, as DirectionSource reports it.
type DirectionSceneIntent struct {
	Goal  string
	Keep  []string
	Avoid []string
}

// DirectionPlannedBranch is the writer's selected forecast branch, as
// DirectionSource reports it (story-assist Phase 4).
type DirectionPlannedBranch struct {
	Title        string
	Premise      string
	Beats        []DirectionPlannedBeat
	Stale        bool
	StaleReasons []DirectionStaleReason
}

// DirectionPlannedBeat is one beat of DirectionPlannedBranch.
type DirectionPlannedBeat struct {
	EpisodeOffset int
	Summary       string
}

// DirectionSource supplies the writer's own direction for the brief: the
// work's current focus, this scene's intent, and the branch (if any) the
// writer picked out of a forecast. Optional — without it the Direction
// section stays empty. Declared as an interface, not a dependency on
// internal/direction's (or internal/forecast's) concrete types, so this
// package does not import either (matching FactSource/SkillSource's shape).
type DirectionSource interface {
	FocusFor(ctx context.Context, projectID string) (DirectionFocus, bool, error)
	SceneIntentFor(ctx context.Context, nodeID string) (DirectionSceneIntent, bool, error)
	// PlannedBranchFor returns the project's currently selected forecast
	// branch, if any. present is false when nothing is selected — never
	// treated as an error, the same convention FocusFor/SceneIntentFor use.
	PlannedBranchFor(ctx context.Context, projectID string) (DirectionPlannedBranch, bool, error)
}

// WithFactSource wires the optional Fact Book section.
func (b *ContextBuilder) WithFactSource(s FactSource) *ContextBuilder {
	b.factSource = s
	return b
}

// WithMemorySource wires the optional memories section.
func (b *ContextBuilder) WithMemorySource(s MemorySource) *ContextBuilder {
	b.memorySource = s
	return b
}

// WithCuratedMemorySource wires the optional curated writer-profile /
// work-notes section.
func (b *ContextBuilder) WithCuratedMemorySource(s CuratedMemorySource) *ContextBuilder {
	b.curatedMemorySource = s
	return b
}

// WithSkillSource wires the optional skills list.
func (b *ContextBuilder) WithSkillSource(s SkillSource) *ContextBuilder {
	b.skillSource = s
	return b
}

// WithReferenceSource wires the optional references section.
func (b *ContextBuilder) WithReferenceSource(s ReferenceSource) *ContextBuilder {
	b.referenceSource = s
	return b
}

// WithDirectionSource wires the optional writer-direction section (the
// work's current focus and this scene's intent).
func (b *ContextBuilder) WithDirectionSource(s DirectionSource) *ContextBuilder {
	b.directionSource = s
	return b
}

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
	// Prefer the cached summary when there is one worth reading (see
	// node.SummaryUsable); fall back to the 300-rune trim.
	prevSummary := ""
	if prev != nil {
		if s, _ := prev.SummaryUsable(); s != "" {
			prevSummary = s
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

	hierarchical, nearbyIDs, err := b.loadHierarchicalContext(ctx, n)
	if err != nil {
		return Context{}, err
	}

	related, err := b.loadRelatedScenes(ctx, n, nearbyIDs)
	if err != nil {
		return Context{}, err
	}

	briefs, err := b.loadEntityBriefs(ctx, n, ents, hierarchical.NearbyLeafSummaries, related)
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

	// Optional sections are best-effort, matching the companion's rule that
	// partial context beats aborting the turn: per-source errors leave the
	// section empty.
	var facts []FactBrief
	if b.factSource != nil {
		if got, err := b.factSource.ContextFacts(ctx, n.ProjectID, nodeID); err == nil {
			facts = got
		}
	}
	var memories []string
	if b.memorySource != nil {
		memories = b.memorySource.ContextMemories(n.ProjectID)
	}
	var writerProfile, workNotes string
	if b.curatedMemorySource != nil {
		writerProfile, workNotes = b.curatedMemorySource.CuratedMemory(ctx, n.ProjectID)
	}
	var skills []SkillBrief
	if b.skillSource != nil {
		skills = b.skillSource.ContextSkills(ctx, n.ProjectID)
	}
	var references []ReferenceBrief
	if b.referenceSource != nil {
		if got, err := b.referenceSource.ContextReferences(ctx, n.ProjectID, nodeID); err == nil {
			references = got
		}
	}
	// An unwritten scene's summary is its plan; once the scene has a body the
	// body says more, and the summary is only a shorter copy of it.
	scenePlan := ""
	if strings.TrimSpace(sceneText) == "" {
		scenePlan = strings.TrimSpace(n.Summary)
	}
	var direction DirectionBrief
	if b.directionSource != nil {
		if focus, present, err := b.directionSource.FocusFor(ctx, n.ProjectID); err == nil && present {
			direction.Focus = focus.Body
			direction.FocusStale = focus.Stale
			direction.FocusEpisodesSince = focus.EpisodesSince
		}
		if intent, present, err := b.directionSource.SceneIntentFor(ctx, nodeID); err == nil && present {
			direction.SceneGoal = intent.Goal
			direction.Keep = intent.Keep
			direction.Avoid = intent.Avoid
		}
		if plan, present, err := b.directionSource.PlannedBranchFor(ctx, n.ProjectID); err == nil && present {
			beats := make([]PlannedBranchBeat, 0, len(plan.Beats))
			for _, bt := range plan.Beats {
				beats = append(beats, PlannedBranchBeat{EpisodeOffset: bt.EpisodeOffset, Summary: bt.Summary})
			}
			direction.PlannedBranch = &PlannedBranchBrief{
				Title: plan.Title, Premise: plan.Premise, Beats: beats,
				Stale: plan.Stale, StaleReasons: plan.StaleReasons,
			}
		}
	}

	return Context{
		ProjectID:   proj.ID,
		NodeID:      n.ID,
		SceneLabel:  n.Label,
		SceneText:   sceneText,
		ScenePlan:   scenePlan,
		PrevSummary: prevSummary,
		Project: ProjectMeta{
			Genres:       proj.Genres,
			LengthTarget: proj.LengthTarget,
			DefaultPOV:   proj.DefaultPOV,
			Synopsis:     proj.Synopsis,
			// Left at the default it says nothing the writer chose, and could
			// only argue with a length they gave in conversation.
			EpisodeCharTarget: chosenEpisodeCharTarget(proj.EpisodeCharTarget),
		},
		Outline:       proj.Outline,
		Hierarchical:  hierarchical,
		RelatedScenes: related,
		Entities:      briefs,
		Relationships: relations,
		Plot:          spine,
		Notes:         noteBriefs,
		Facts:         facts,
		Memories:      memories,
		WriterProfile: writerProfile,
		WorkNotes:     workNotes,
		Skills:        skills,
		References:    references,
		Direction:     direction,
		StyleNotes:    proj.StyleNotes,
		SelectionText: selectionText,
		UserPrompt:    prompt,
		Options:       opts,
	}, nil
}

func chosenEpisodeCharTarget(target int) int {
	if target == project.DefaultEpisodeCharTarget {
		return 0
	}
	return target
}

// loadEntityBriefs builds the entity section. An entity's Recent lines come
// from the scenes before cur, nearest first, one line per scene, skipping the
// scenes the brief already shows in full. A cur that is not a leaf has no story
// position, so every scene counts as before it.
func (b *ContextBuilder) loadEntityBriefs(ctx context.Context, cur node.Node, ents []entity.Entity, shown ...[]SceneSummary) ([]EntityBrief, error) {
	all, err := b.nodes.ListByProject(ctx, cur.ProjectID)
	if err != nil {
		return nil, err
	}
	earlier := leavesBefore(leavesInOrder(childrenByParent(all)), cur.ID)
	skip := map[string]bool{cur.ID: true}
	for _, scenes := range shown {
		for _, s := range scenes {
			skip[s.NodeID] = true
		}
	}

	briefs := make([]EntityBrief, 0, len(ents))
	budget := entityRecentMaxRunes
	// A scene's line appears once. The next entity it concerns gets the label
	// alone: a cast of twelve shares most of its scenes, and repeating the
	// summary under each of them is the bulk the dossier used to be.
	told := map[string]bool{}
	for _, e := range ents {
		brief := EntityBrief{Name: e.Name, Kind: e.Kind, Role: e.Role, Summary: e.Summary, Attributes: e.Attributes}
		ids, _, err := b.mentions.MentionedNodeIDs(ctx, e.ID)
		if err != nil {
			return nil, err
		}
		mentioned := make(map[string]bool, len(ids))
		for _, id := range ids {
			mentioned[id] = true
		}
		for i := len(earlier) - 1; i >= 0 && len(brief.Recent) < entityRecentLimit; i-- {
			lf := earlier[i]
			if !mentioned[lf.ID] || skip[lf.ID] {
				continue
			}
			first := strings.TrimSpace(strings.SplitN(lf.Summary, "\n", 2)[0])
			if first == "" {
				continue
			}
			line := "[" + lf.Label + "]"
			if !told[lf.ID] {
				line += " " + trimRunes(first, entityRecentLineMaxRunes)
				told[lf.ID] = true
			}
			budget -= utf8.RuneCountInString(line)
			if budget < 0 {
				break
			}
			brief.Recent = append(brief.Recent, line)
		}
		briefs = append(briefs, brief)
	}
	return briefs, nil
}

// leavesBefore returns the leaves ahead of curID in story order, or all of them
// when curID is not among them.
func leavesBefore(leaves []node.Node, curID string) []node.Node {
	for i, l := range leaves {
		if l.ID == curID {
			return leaves[:i]
		}
	}
	return leaves
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
		c.ScenePlan = ""
	}
	if !s.Enabled(ContextKeyOverview) {
		c.Outline = ""
	}
	if !s.Enabled(ContextKeySynopsis) {
		c.Project.Synopsis = ""
		c.Hierarchical.ProjectSynopsis = ""
		c.Hierarchical.PartSummaries = nil
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
	if !s.Enabled(ContextKeyFacts) {
		c.Facts = nil
	}
	if !s.Enabled(ContextKeyMemories) {
		c.Memories = nil
		c.WriterProfile = ""
		c.WorkNotes = ""
	}
	if !s.Enabled(ContextKeyReferences) {
		c.References = nil
	}
	// Its own key on purpose, not folded into ContextKeyMemories above — see
	// ContextKeySkills' comment in types.go.
	if !s.Enabled(ContextKeySkills) {
		c.Skills = nil
	}
	// Its own key too, for the same reason: see ContextKeyDirection's
	// comment in types.go.
	if !s.Enabled(ContextKeyDirection) {
		c.Direction = DirectionBrief{}
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
	for _, n := range all {
		byID[n.ID] = n
	}
	children := childrenByParent(all)
	leaves := leavesInOrder(children)

	curIdx := -1
	for i, l := range leaves {
		if l.ID == cur.ID {
			curIdx = i
			break
		}
	}

	out := HierarchicalContext{}
	nearbyIDs := make([]string, 0, 2)

	// Nearby: 1 prior + 1 next (in DFS-leaf order), then the plans of the
	// unwritten scenes after that, so an agent drafting from a plan need not
	// open each empty scene to see where the story is going.
	if curIdx >= 0 {
		neighbours := []int{curIdx - 1, curIdx + 1}
		for j := curIdx + 2; j < len(leaves) && j <= curIdx+1+upcomingPlanLimit; j++ {
			if strings.TrimSpace(docToPlainText(leaves[j].ContentDoc)) != "" {
				break
			}
			neighbours = append(neighbours, j)
		}
		for _, j := range neighbours {
			if j < 0 || j >= len(leaves) || j == curIdx {
				continue
			}
			lf := leaves[j]
			body, stale := freshLeafSummary(lf)
			if body == "" {
				continue
			}
			out.NearbyLeafSummaries = append(out.NearbyLeafSummaries, SceneSummary{
				NodeID: lf.ID, Label: node.BreadcrumbLabel(byID, lf), Body: body, Stale: stale,
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

	trimToBudget(&out, nearbyMaxRunes)
	kept := make(map[string]bool, len(out.NearbyLeafSummaries))
	for _, s := range out.NearbyLeafSummaries {
		kept[s.NodeID] = true
	}
	shown := nearbyIDs[:0]
	for _, id := range nearbyIDs {
		if kept[id] {
			shown = append(shown, id)
		}
	}

	out.PartSummaries = partSummaries(rootContainers, ancestorRoot(byID, cur), byID)

	return out, shown, nil
}

// ancestorRoot returns the id of the outermost node above n, or n's own id.
// The hop limit only guards against a cycle in the parent_id graph.
func ancestorRoot(byID map[string]node.Node, n node.Node) string {
	for hops := 0; n.ParentID != nil && hops < 64; hops++ {
		parent, ok := byID[*n.ParentID]
		if !ok {
			break
		}
		n = parent
	}
	return n.ID
}

// partSummaries is the work's arc, part by part, for a work divided into
// parts. Only summaries someone wrote count: a part's rollup is the openings
// of its scenes strung together, which tells the reader nothing the nearby
// scenes do not, and the brief has never presented derived text as the writer's
// own account of the work.
func partSummaries(roots []node.Node, currentRoot string, byID map[string]node.Node) []SceneSummary {
	written := make([]node.Node, 0, len(roots))
	for _, r := range roots {
		if r.SummaryWritten && strings.TrimSpace(r.Summary) != "" {
			written = append(written, r)
		}
	}
	if len(written) == 0 {
		return nil
	}
	remaining := partSummariesMaxRunes
	currentRunes := 0
	for _, r := range written {
		if r.ID == currentRoot {
			currentRunes = min(utf8.RuneCountInString(r.Summary), partSummaryCurrentMaxRunes)
		}
	}
	remaining -= currentRunes
	others := len(written)
	if currentRunes > 0 {
		others--
	}
	each := partSummaryMinRunes
	if others > 0 {
		each = max(remaining/others, partSummaryMinRunes)
	}
	out := make([]SceneSummary, 0, len(written))
	for _, r := range written {
		limit := each
		if r.ID == currentRoot {
			limit = partSummaryCurrentMaxRunes
		}
		out = append(out, SceneSummary{
			NodeID: r.ID, Label: node.BreadcrumbLabel(byID, r), Body: trimRunes(strings.TrimSpace(r.Summary), limit), Stale: !r.SummaryFresh(),
		})
	}
	return out
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
// the result set against excludeIDs (typically Nearby IDs + cur.ID) and against
// every leaf after cur, so a revision never reads what the story has not reached
// yet. A cur that is not a leaf has no such limit. Returns up to 2 SceneSummary
// entries with breadcrumb labels.
func (b *ContextBuilder) loadRelatedScenes(ctx context.Context, cur node.Node, excludeIDs []string) ([]SceneSummary, error) {
	all, err := b.nodes.ListByProject(ctx, cur.ProjectID)
	if err != nil {
		return nil, err
	}
	// Ask for every match: the filters below run in Go, and a limit here would
	// let scenes they reject push out the ones they keep.
	results, err := b.mentions.CoMentionLeaves(ctx, cur.ID, len(all))
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
	reachedCur := false
	for _, l := range leavesInOrder(childrenByParent(all)) {
		if reachedCur {
			excl[l.ID] = true
		}
		reachedCur = reachedCur || l.ID == cur.ID
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
			NodeID: n.ID, Label: node.BreadcrumbLabel(byID, n), Body: n.Summary, Stale: !n.SummaryFresh(),
		})
		if len(out) >= 2 {
			break
		}
	}
	return out, nil
}

// freshLeafSummary returns the cached summary when it is worth reading — fresh,
// or written and stale, which it reports (see node.SummaryUsable); otherwise
// it falls back to a 300-rune trim of the plaintext body. Empty string if no
// content at all (e.g. the leaf was just created and has no doc yet).
func freshLeafSummary(n node.Node) (string, bool) {
	if s, stale := n.SummaryUsable(); s != "" {
		return s, stale
	}
	if n.ContentDoc != nil {
		plain := docToPlainText(n.ContentDoc)
		if plain == "" {
			return "", false
		}
		return trimRunes(plain, prevSummaryMaxRunes), false
	}
	return "", false
}

// refreshAndRead returns n.Summary if fresh; otherwise it calls the refresher
// synchronously and re-reads the row. Empty string is returned if the refresh
// failed to land (better than a stale derived string). A written summary the
// refresher left alone is returned as it stands.
func (b *ContextBuilder) refreshAndRead(ctx context.Context, n node.Node) string {
	if n.SummaryFresh() {
		return n.Summary
	}
	if b.refresher != nil {
		b.refresher.RefreshNow(ctx, n.ID)
	}
	got, err := b.nodes.Get(ctx, n.ID)
	if err != nil {
		return ""
	}
	s, _ := got.SummaryUsable()
	return s
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
// is under maxRunes. The entries run outward from the scene, so the farthest go
// first.
func trimToBudget(h *HierarchicalContext, maxRunes int) {
	estimate := func() int {
		total := 0
		for _, s := range h.NearbyLeafSummaries {
			total += utf8.RuneCountInString(s.Label) + utf8.RuneCountInString(s.Body) + 4
		}
		return total
	}
	for estimate() > maxRunes && len(h.NearbyLeafSummaries) > 1 {
		h.NearbyLeafSummaries = h.NearbyLeafSummaries[:len(h.NearbyLeafSummaries)-1]
	}
	if len(h.NearbyLeafSummaries) == 1 {
		h.NearbyLeafSummaries[0].Body = trimRunes(h.NearbyLeafSummaries[0].Body, maxRunes)
	}
}

// findPreviousLeaf returns the previous leaf in DFS order (within the project),
// or nil if `cur` is the first leaf.
func (b *ContextBuilder) findPreviousLeaf(ctx context.Context, cur node.Node) (*node.Node, error) {
	all, err := b.nodes.ListByProject(ctx, cur.ProjectID)
	if err != nil {
		return nil, err
	}
	leaves := leavesInOrder(childrenByParent(all))
	for i, l := range leaves {
		if l.ID == cur.ID && i > 0 {
			return &leaves[i-1], nil
		}
	}
	return nil, nil
}

// childrenByParent builds a parent_id → []children index; roots sit under "".
func childrenByParent(all []node.Node) map[string][]node.Node {
	children := map[string][]node.Node{}
	for _, n := range all {
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}
	return children
}

// leavesInOrder walks the tree depth first and returns the leaves in document
// order.
func leavesInOrder(children map[string][]node.Node) []node.Node {
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
	return leaves
}

// docToPlainText walks a Tiptap doc and concatenates text content. Mentions are
// rendered as `@label`. Block boundaries become newlines.
// PlainText renders a stored Tiptap document as plain text, the same way the
// story brief does. Exported so MCP tools return prose rather than editor JSON
// without duplicating the walker.
func PlainText(rawDoc *string) string { return docToPlainText(rawDoc) }

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
		// An illustration travels as its placeholder line, so an agent that
		// rewrites the scene can keep the picture where it is.
		if id, caption, ok := illustref.Attrs(t); ok {
			sb.WriteString(illustref.Placeholder(id, caption))
			sb.WriteString("\n")
			return
		}
		// Block-level node: recurse children, then add a newline.
		if content, ok := t["content"].([]interface{}); ok {
			for _, c := range content {
				walkDoc(c, sb)
			}
		}
		if kind == "paragraph" || kind == "heading" || kind == "blockquote" || kind == "codeBlock" {
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
			ID:            id,
			Label:         label,
			Present:       present,
			Selected:      selected,
			Count:         count,
			Preview:       trimRunes(strings.TrimSpace(preview), 1200),
			CharCount:     EstimateChars(preview),
			TokenEstimate: EstimateTokens(preview),
		}
		sections = append(sections, section)
	}

	add(ContextKeyCurrentScene, "현재 씬 본문", 1, c.SceneText, true)

	overview := strings.TrimSpace(c.Outline)
	add(ContextKeyOverview, "작품 개요", boolCount(overview != ""), overview, false)

	synopsis := strings.TrimSpace(c.Project.Synopsis)
	if parts := renderSceneSummariesPreview(c.Hierarchical.PartSummaries, c.Options.Language); parts != "" {
		synopsis = strings.TrimSpace(synopsis + "\n\n" + parts)
	}
	add(ContextKeySynopsis, "작품 시놉시스", boolCount(synopsis != ""), synopsis, false)

	add(ContextKeyNearbyScenes, "직전·직후 씬 발췌", len(c.Hierarchical.NearbyLeafSummaries), renderSceneSummariesPreview(c.Hierarchical.NearbyLeafSummaries, c.Options.Language), false)
	add(ContextKeyRelatedScenes, "관련 과거 씬 (멘션 RAG)", len(c.RelatedScenes), renderSceneSummariesPreview(c.RelatedScenes, c.Options.Language), false)
	add(ContextKeyPlot, "플롯 (스토리라인&비트)", countPlotBeats(c.Plot), renderPlotPreview(c.Plot), false)
	add(ContextKeyEntities, "세계관 요소", len(c.Entities), renderEntitiesPreview(c.Entities), false)
	add(ContextKeyRelationships, "관계", len(c.Relationships), renderRelationshipsPreview(c.Relationships), false)
	add(ContextKeyNotes, "작가 주석", len(c.Notes), renderNotesPreview(c.Notes), false)
	if meta := renderProjectMeta(c.Project, ""); meta != "" {
		add(ContextKeyProjectMeta, "작품 설정 (장르/분량/시점)", countProjectMeta(c.Project), meta, false)
	} else {
		add(ContextKeyProjectMeta, "작품 설정 (장르/분량/시점)", 0, "", false)
	}
	add(ContextKeyStyleNotes, "작가 style notes", boolCount(strings.TrimSpace(c.StyleNotes) != ""), c.StyleNotes, false)

	selectedCount := 0
	selectedChars := 0
	selectedTokens := 0
	budgetTokens := 0
	for _, section := range sections {
		if section.Present {
			budgetTokens += section.TokenEstimate
		}
		if !section.Selected {
			continue
		}
		if section.Count > 0 {
			selectedCount += section.Count
		} else {
			selectedCount++
		}
		selectedChars += section.CharCount
		selectedTokens += section.TokenEstimate
	}

	return ContextPreview{
		PreviewCounts:         CountsFromContext(c),
		Sections:              sections,
		SelectedItemCount:     selectedCount,
		SelectedCharCount:     selectedChars,
		SelectedTokenEstimate: selectedTokens,
		BudgetTokenEstimate:   budgetTokens,
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

func renderSceneSummariesPreview(scenes []SceneSummary, lang string) string {
	var b strings.Builder
	for _, s := range scenes {
		b.WriteString(fmt.Sprintf("- [%s] %s%s\n", s.Label, staleMark(s.Stale, lang), s.Body))
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
		b.WriteString(fmt.Sprintf("- @%s — %s", e.Name, kindLabel(e.Kind, "")))
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
