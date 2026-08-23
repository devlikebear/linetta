//go:build !mobile

package mcphost

import (
	"context"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/storycontext"
)

// ReadToolNames lists the read tools, in registration order. Tests assert
// tools/list against this so the surface cannot drift silently.
var ReadToolNames = []string{
	"linetta_list_works",
	"linetta_get_outline",
	"linetta_get_story_context",
	"linetta_read_scene",
	"linetta_search_manuscript",
	"linetta_list_characters",
	"linetta_where_does_appear",
	"linetta_get_plot",
	"linetta_get_fact_cards",
}

const defaultSearchLimit = 20

// ---------- linetta_list_works ----------

type listWorksInput struct {
	IncludeArchived bool `json:"include_archived,omitempty" jsonschema:"include archived works as well as active ones"`
}

func (listWorksInput) scope() (string, string) { return "", "" }

type workSummary struct {
	ProjectID  string   `json:"project_id"`
	Title      string   `json:"title"`
	Status     string   `json:"status"`
	Synopsis   string   `json:"synopsis,omitempty"`
	Genres     []string `json:"genres,omitempty"`
	SceneCount int      `json:"scene_count"`
}

type listWorksOutput struct {
	Works []workSummary `json:"works"`
}

// ---------- linetta_get_outline ----------

type getOutlineInput struct {
	ProjectID string `json:"project_id" jsonschema:"id of the work, from linetta_list_works"`
}

func (in getOutlineInput) scope() (string, string) { return in.ProjectID, "" }

type outlineRow struct {
	NodeID          string `json:"node_id"`
	ParentID        string `json:"parent_id,omitempty"`
	Depth           int    `json:"depth"`
	Kind            string `json:"kind"`
	Label           string `json:"label"`
	Title           string `json:"title,omitempty"`
	Status          string `json:"status"`
	WordCount       int    `json:"word_count"`
	HasFreshSummary bool   `json:"has_fresh_summary"`
}

type getOutlineOutput struct {
	ProjectID string       `json:"project_id"`
	Title     string       `json:"title"`
	Outline   []outlineRow `json:"outline"`
}

// ---------- linetta_get_story_context ----------

type getStoryContextInput struct {
	NodeID string `json:"node_id" jsonschema:"id of the scene to build the brief for"`
	// Section toggles map onto the writer's own context checklist. Omit them
	// to get everything.
	IncludeFacts      *bool `json:"include_facts,omitempty"`
	IncludeMemories   *bool `json:"include_memories,omitempty"`
	IncludeReferences *bool `json:"include_references,omitempty"`
	IncludePlot       *bool `json:"include_plot,omitempty"`
}

func (in getStoryContextInput) scope() (string, string) { return "", in.NodeID }

type getStoryContextOutput struct {
	ProjectID        string   `json:"project_id"`
	NodeID           string   `json:"node_id"`
	SceneLabel       string   `json:"scene_label"`
	Brief            string   `json:"brief"`
	IncludedSections []string `json:"included_sections"`
	EmptySections    []string `json:"empty_sections"`
}

// ---------- linetta_read_scene ----------

type readSceneInput struct {
	NodeID string `json:"node_id" jsonschema:"id of the scene to read"`
}

func (in readSceneInput) scope() (string, string) { return "", in.NodeID }

type readSceneOutput struct {
	NodeID         string `json:"node_id"`
	ProjectID      string `json:"project_id"`
	Label          string `json:"label"`
	Title          string `json:"title,omitempty"`
	Status         string `json:"status"`
	WordCount      int    `json:"word_count"`
	ContentVersion int    `json:"content_version"`
	Body           string `json:"body"`
	Summary        string `json:"summary,omitempty"`
	SummaryIsStale bool   `json:"summary_is_stale"`
}

// ---------- linetta_search_manuscript ----------

type searchManuscriptInput struct {
	ProjectID string `json:"project_id" jsonschema:"id of the work to search"`
	Query     string `json:"query" jsonschema:"words or phrase to find in the manuscript"`
	Limit     int    `json:"limit,omitempty"`
}

func (in searchManuscriptInput) scope() (string, string) { return in.ProjectID, "" }

type searchHit struct {
	NodeID  string `json:"node_id"`
	Label   string `json:"label"`
	Snippet string `json:"snippet"`
}

type searchManuscriptOutput struct {
	Hits []searchHit `json:"hits"`
}

// ---------- linetta_list_characters ----------

type listCharactersInput struct {
	ProjectID string `json:"project_id" jsonschema:"id of the work"`
	Kind      string `json:"kind,omitempty" jsonschema:"filter by character, place, item, or concept; omit for all"`
}

func (in listCharactersInput) scope() (string, string) { return in.ProjectID, "" }

type entityRow struct {
	EntityID   string            `json:"entity_id"`
	Kind       string            `json:"kind"`
	Name       string            `json:"name"`
	Role       string            `json:"role,omitempty"`
	Summary    string            `json:"summary,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type listCharactersOutput struct {
	Entities []entityRow `json:"entities"`
}

// ---------- linetta_where_does_appear ----------

type whereAppearsInput struct {
	EntityID string `json:"entity_id" jsonschema:"id of the character, place, item, or concept"`
}

func (in whereAppearsInput) scope() (string, string) { return "", in.EntityID }

type appearanceRow struct {
	NodeID string `json:"node_id"`
	Label  string `json:"label"`
	Status string `json:"status"`
}

type whereAppearsOutput struct {
	EntityID string          `json:"entity_id"`
	Scenes   []appearanceRow `json:"scenes"`
}

// ---------- linetta_get_plot ----------

type getPlotInput struct {
	NodeID string `json:"node_id" jsonschema:"a scene in the work; the plot spine is built around it"`
}

func (in getPlotInput) scope() (string, string) { return "", in.NodeID }

type getPlotOutput struct {
	NodeID string     `json:"node_id"`
	Spine  plot.Spine `json:"spine"`
}

// ---------- linetta_get_fact_cards ----------

type getFactCardsInput struct {
	ProjectID string `json:"project_id" jsonschema:"id of the work"`
	NodeID    string `json:"node_id,omitempty" jsonschema:"restrict to cards attached to this scene"`
	Limit     int    `json:"limit,omitempty"`
}

func (in getFactCardsInput) scope() (string, string) { return in.ProjectID, in.NodeID }

type factRow struct {
	FactID   string   `json:"fact_id"`
	Status   string   `json:"status"`
	Claim    string   `json:"claim"`
	Result   string   `json:"result,omitempty"`
	Category string   `json:"category,omitempty"`
	Sources  []string `json:"sources,omitempty"`
}

type getFactCardsOutput struct {
	Cards []factRow `json:"cards"`
}

// registerReadTools installs every read tool, each wrapped so the call lands
// in the activity log.
func (d ToolDeps) registerReadTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_list_works",
		Description: "List the writer's works (novels) with their ids, titles, and scene counts. " +
			"Start here to find the project_id other tools need.",
	}, record(d, "linetta_list_works", d.listWorks))

	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_get_outline",
		Description: "Return the work's outline tree: parts, chapters, and scenes with their node ids, " +
			"status, and word counts. Use it to locate the scene you need to read or write.",
	}, record(d, "linetta_get_outline", d.getOutline))

	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_get_story_context",
		Description: "Build the curated brief for one scene: outline, chapter summaries, the previous " +
			"scene's summary, character and relationship briefs, plot beats, fact cards, memories, and the " +
			"writer's style and POV targets. Call this before drafting or revising so the text stays " +
			"consistent with the rest of the work. Empty summary sections mean nobody has summarized those " +
			"scenes yet.",
	}, record(d, "linetta_get_story_context", d.getStoryContext))

	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_read_scene",
		Description: "Read one scene's text as plain prose, with its content_version. Any later write to " +
			"this scene must pass the content_version you got here, so the writer's own edits are never " +
			"silently overwritten.",
	}, record(d, "linetta_read_scene", d.readScene))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "linetta_search_manuscript",
		Description: "Full-text search across the work's manuscript. Returns matching scenes with snippets.",
	}, record(d, "linetta_search_manuscript", d.searchManuscript))

	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_list_characters",
		Description: "List the work's story elements — characters by default, or places, items, and " +
			"concepts via the kind filter — with their roles, summaries, and attributes.",
	}, record(d, "linetta_list_characters", d.listCharacters))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "linetta_where_does_appear",
		Description: "List the scenes where one character, place, item, or concept is mentioned.",
	}, record(d, "linetta_where_does_appear", d.whereAppears))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "linetta_get_plot",
		Description: "Return the plot spine around a scene: storylines and their beats, in order.",
	}, record(d, "linetta_get_plot", d.getPlot))

	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_get_fact_cards",
		Description: "Return the work's Fact Book cards: source-backed research notes with their " +
			"verification status. Use them for real-world details instead of inventing facts.",
	}, record(d, "linetta_get_fact_cards", d.getFactCards))
}

func (d ToolDeps) listWorks(ctx context.Context, _ *mcp.CallToolRequest, in listWorksInput) (*mcp.CallToolResult, listWorksOutput, error) {
	projects, err := d.Projects.List(ctx, project.ListFilter{IncludeArchived: in.IncludeArchived})
	if err != nil {
		return toolErr("could not list works: %v", err), listWorksOutput{}, nil
	}
	restricted := d.allowedProjectID()
	out := listWorksOutput{Works: []workSummary{}}
	for _, p := range projects {
		if restricted != "" && p.ID != restricted {
			continue
		}
		scenes := 0
		if all, err := d.Nodes.ListByProject(ctx, p.ID); err == nil {
			for _, n := range all {
				if n.Kind == node.KindLeaf {
					scenes++
				}
			}
		}
		status := "active"
		if p.ArchivedAt != nil {
			status = "archived"
		}
		out.Works = append(out.Works, workSummary{
			ProjectID:  p.ID,
			Title:      p.Title,
			Status:     status,
			Synopsis:   p.Synopsis,
			Genres:     p.Genres,
			SceneCount: scenes,
		})
	}
	return nil, out, nil
}

func (d ToolDeps) getOutline(ctx context.Context, _ *mcp.CallToolRequest, in getOutlineInput) (*mcp.CallToolResult, getOutlineOutput, error) {
	p, errResult := d.requireProject(ctx, in.ProjectID)
	if errResult != nil {
		return errResult, getOutlineOutput{}, nil
	}
	all, err := d.Nodes.ListByProject(ctx, p.ID)
	if err != nil {
		return toolErr("could not read the outline: %v", err), getOutlineOutput{}, nil
	}
	out := getOutlineOutput{ProjectID: p.ID, Title: p.Title, Outline: outlineRows(all)}
	return nil, out, nil
}

// outlineRows flattens the tree in document order with a depth column, which
// is what an agent needs to understand structure without reconstructing it.
func outlineRows(all []node.Node) []outlineRow {
	children := map[string][]node.Node{}
	for _, n := range all {
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}
	for key := range children {
		sort.SliceStable(children[key], func(i, j int) bool {
			return children[key][i].Ordinal < children[key][j].Ordinal
		})
	}
	rows := []outlineRow{}
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		for _, n := range children[parent] {
			parentID := ""
			if n.ParentID != nil {
				parentID = *n.ParentID
			}
			rows = append(rows, outlineRow{
				NodeID:          n.ID,
				ParentID:        parentID,
				Depth:           depth,
				Kind:            n.Kind,
				Label:           n.Label,
				Title:           n.Title,
				Status:          n.Status,
				WordCount:       n.WordCount,
				HasFreshSummary: n.Summary != "" && n.SummaryForVersion == n.ContentVersion,
			})
			walk(n.ID, depth+1)
		}
	}
	walk("", 0)
	return rows
}

func (d ToolDeps) getStoryContext(ctx context.Context, _ *mcp.CallToolRequest, in getStoryContextInput) (*mcp.CallToolResult, getStoryContextOutput, error) {
	n, errResult := d.requireNode(ctx, in.NodeID)
	if errResult != nil {
		return errResult, getStoryContextOutput{}, nil
	}
	if d.Context == nil {
		return toolErr("story context is unavailable in this build"), getStoryContextOutput{}, nil
	}
	opts := storycontext.Options{
		Context: storycontext.ContextSelection{
			Facts:      in.IncludeFacts,
			Memories:   in.IncludeMemories,
			References: in.IncludeReferences,
			Plot:       in.IncludePlot,
		},
	}
	c, err := d.Context.BuildFull(ctx, n.ID, "", "", opts)
	if err != nil {
		return toolErr("could not build the story brief: %v", err), getStoryContextOutput{}, nil
	}
	// Only the user half of the render is the brief; the system half carries
	// tone and instruction scaffolding meant for Linetta's own runner.
	_, brief := storycontext.Render(c)
	included, empty := sectionReport(c)
	return nil, getStoryContextOutput{
		ProjectID:        n.ProjectID,
		NodeID:           n.ID,
		SceneLabel:       c.SceneLabel,
		Brief:            brief,
		IncludedSections: included,
		EmptySections:    empty,
	}, nil
}

// sectionReport tells the agent what the brief actually carries. An empty
// summary section is the signal to go write one with linetta_write_summary.
func sectionReport(c storycontext.Context) (included, empty []string) {
	c = storycontext.ApplyContextSelection(c)
	checks := []struct {
		name    string
		present bool
	}{
		{"current_scene", strings.TrimSpace(c.SceneText) != ""},
		{"overview", strings.TrimSpace(c.Outline) != ""},
		{"synopsis", strings.TrimSpace(c.Hierarchical.ProjectSynopsis) != "" || strings.TrimSpace(c.Project.Synopsis) != ""},
		{"nearby_scene_summaries", len(c.Hierarchical.NearbyLeafSummaries) > 0},
		{"related_scenes", len(c.RelatedScenes) > 0},
		{"entities", len(c.Entities) > 0},
		{"relationships", len(c.Relationships) > 0},
		{"plot", spineHasBeats(c.Plot)},
		{"notes", len(c.Notes) > 0},
		{"facts", len(c.Facts) > 0},
		{"memories", len(c.Memories) > 0},
		{"references", len(c.References) > 0},
		{"style_notes", strings.TrimSpace(c.StyleNotes) != ""},
	}
	included, empty = []string{}, []string{}
	for _, ch := range checks {
		if ch.present {
			included = append(included, ch.name)
		} else {
			empty = append(empty, ch.name)
		}
	}
	return included, empty
}

func (d ToolDeps) readScene(ctx context.Context, _ *mcp.CallToolRequest, in readSceneInput) (*mcp.CallToolResult, readSceneOutput, error) {
	n, errResult := d.requireNode(ctx, in.NodeID)
	if errResult != nil {
		return errResult, readSceneOutput{}, nil
	}
	if n.Kind != node.KindLeaf {
		return toolErr("node %q is a container (%s), not a scene; only scenes have body text", n.ID, n.Label),
			readSceneOutput{}, nil
	}
	return nil, readSceneOutput{
		NodeID:         n.ID,
		ProjectID:      n.ProjectID,
		Label:          n.Label,
		Title:          n.Title,
		Status:         n.Status,
		WordCount:      n.WordCount,
		ContentVersion: n.ContentVersion,
		// Trimmed at the tool boundary, not in PlainText: the brief's renderer
		// depends on that function's exact output. An untouched empty scene
		// otherwise arrives as "\n", which an agent can misread as content.
		Body:           strings.TrimSpace(storycontext.PlainText(n.ContentDoc)),
		Summary:        n.Summary,
		SummaryIsStale: n.Summary == "" || n.SummaryForVersion != n.ContentVersion,
	}, nil
}

func (d ToolDeps) searchManuscript(ctx context.Context, _ *mcp.CallToolRequest, in searchManuscriptInput) (*mcp.CallToolResult, searchManuscriptOutput, error) {
	p, errResult := d.requireProject(ctx, in.ProjectID)
	if errResult != nil {
		return errResult, searchManuscriptOutput{}, nil
	}
	q := strings.TrimSpace(in.Query)
	if q == "" {
		return toolErr("query is required"), searchManuscriptOutput{}, nil
	}
	if d.Manuscript == nil {
		return toolErr("manuscript search is unavailable in this build"), searchManuscriptOutput{}, nil
	}
	limit := in.Limit
	if limit <= 0 || limit > 100 {
		limit = defaultSearchLimit
	}
	hits, err := d.Manuscript.Query(ctx, p.ID, q, limit)
	if err != nil {
		return toolErr("search failed: %v", err), searchManuscriptOutput{}, nil
	}
	out := searchManuscriptOutput{Hits: []searchHit{}}
	for _, h := range hits {
		out.Hits = append(out.Hits, searchHit{NodeID: h.NodeID, Label: h.Breadcrumb, Snippet: h.Snippet})
	}
	return nil, out, nil
}

func (d ToolDeps) listCharacters(ctx context.Context, _ *mcp.CallToolRequest, in listCharactersInput) (*mcp.CallToolResult, listCharactersOutput, error) {
	p, errResult := d.requireProject(ctx, in.ProjectID)
	if errResult != nil {
		return errResult, listCharactersOutput{}, nil
	}
	all, err := d.Entities.ListByProject(ctx, p.ID)
	if err != nil {
		return toolErr("could not list story elements: %v", err), listCharactersOutput{}, nil
	}
	kind := entityKindFilter(in.Kind)
	out := listCharactersOutput{Entities: []entityRow{}}
	for _, e := range all {
		if kind != "" && !strings.EqualFold(e.Kind, kind) {
			continue
		}
		out.Entities = append(out.Entities, entityRow{
			EntityID:   e.ID,
			Kind:       e.Kind,
			Name:       e.Name,
			Role:       e.Role,
			Summary:    e.Summary,
			Attributes: e.Attributes,
		})
	}
	return nil, out, nil
}

func (d ToolDeps) whereAppears(ctx context.Context, _ *mcp.CallToolRequest, in whereAppearsInput) (*mcp.CallToolResult, whereAppearsOutput, error) {
	entityID := strings.TrimSpace(in.EntityID)
	if entityID == "" {
		return toolErr("entity_id is required; call linetta_list_characters first"), whereAppearsOutput{}, nil
	}
	ent, err := d.Entities.Get(ctx, entityID)
	if err != nil {
		return toolErr("story element %q not found", entityID), whereAppearsOutput{}, nil
	}
	if _, errResult := d.requireProject(ctx, ent.ProjectID); errResult != nil {
		return errResult, whereAppearsOutput{}, nil
	}
	ids, _, err := d.Mentions.MentionedNodeIDs(ctx, entityID)
	if err != nil {
		return toolErr("could not read mentions: %v", err), whereAppearsOutput{}, nil
	}
	out := whereAppearsOutput{EntityID: entityID, Scenes: []appearanceRow{}}
	if len(ids) == 0 {
		return nil, out, nil
	}
	mentioned := make(map[string]bool, len(ids))
	for _, id := range ids {
		mentioned[id] = true
	}
	all, err := d.Nodes.ListByProject(ctx, ent.ProjectID)
	if err != nil {
		return toolErr("could not read the outline: %v", err), whereAppearsOutput{}, nil
	}
	for _, row := range outlineRows(all) {
		if !mentioned[row.NodeID] {
			continue
		}
		out.Scenes = append(out.Scenes, appearanceRow{NodeID: row.NodeID, Label: row.Label, Status: row.Status})
	}
	return nil, out, nil
}

func (d ToolDeps) getPlot(ctx context.Context, _ *mcp.CallToolRequest, in getPlotInput) (*mcp.CallToolResult, getPlotOutput, error) {
	n, errResult := d.requireNode(ctx, in.NodeID)
	if errResult != nil {
		return errResult, getPlotOutput{}, nil
	}
	if d.Plot == nil {
		return toolErr("plot is unavailable in this build"), getPlotOutput{}, nil
	}
	spine, err := d.Plot.Build(ctx, n.ID)
	if err != nil {
		return toolErr("could not build the plot spine: %v", err), getPlotOutput{}, nil
	}
	return nil, getPlotOutput{NodeID: n.ID, Spine: spine}, nil
}

func (d ToolDeps) getFactCards(ctx context.Context, _ *mcp.CallToolRequest, in getFactCardsInput) (*mcp.CallToolResult, getFactCardsOutput, error) {
	p, errResult := d.requireProject(ctx, in.ProjectID)
	if errResult != nil {
		return errResult, getFactCardsOutput{}, nil
	}
	if d.Facts == nil {
		return toolErr("the Fact Book is unavailable in this build"), getFactCardsOutput{}, nil
	}
	filter := fact.ListFilter{ProjectID: p.ID, Limit: in.Limit}
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 50
	}
	if nodeID := strings.TrimSpace(in.NodeID); nodeID != "" {
		if _, errResult := d.requireNode(ctx, nodeID); errResult != nil {
			return errResult, getFactCardsOutput{}, nil
		}
		filter.NodeID = &nodeID
	}
	cards, err := d.Facts.List(ctx, filter)
	if err != nil {
		return toolErr("could not read the Fact Book: %v", err), getFactCardsOutput{}, nil
	}
	out := getFactCardsOutput{Cards: []factRow{}}
	for _, c := range cards {
		row := factRow{
			FactID:   c.ID,
			Status:   c.Status,
			Claim:    c.Claim,
			Result:   c.Result,
			Category: c.Category,
		}
		for _, src := range c.Sources {
			if strings.TrimSpace(src.URL) != "" {
				row.Sources = append(row.Sources, src.URL)
			}
		}
		out.Cards = append(out.Cards, row)
	}
	return nil, out, nil
}

// spineHasBeats mirrors the renderer's own emptiness check so the section
// report agrees with what the brief actually contains.
func spineHasBeats(s plot.Spine) bool {
	if len(s.Current.Beats) > 0 {
		return true
	}
	if s.Prev != nil && len(s.Prev.Beats) > 0 {
		return true
	}
	return s.Next != nil && len(s.Next.Beats) > 0
}
