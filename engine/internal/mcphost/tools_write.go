//go:build !mobile

package mcphost

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
	"github.com/devlikebear/linetta/engine/internal/agentskills"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/storyops"
)

// WriteToolNames lists the write tools, in registration order. They are
// registered only in settings.MCPModeFull, so read_only does not merely refuse
// writes — the tools are absent from tools/list.
var WriteToolNames = []string{
	"linetta_create_work",
	"linetta_write_scene",
	"linetta_write_summary",
	"linetta_revise_scene",
	"linetta_apply_story_ops",
	"linetta_create_checkpoint",
	"linetta_undo_last_change",
	"linetta_edit_memory",
	"linetta_edit_skill",
}

// ---------- linetta_create_work ----------

type createWorkInput struct {
	Title         string   `json:"title" jsonschema:"the work's title"`
	Genres        []string `json:"genres,omitempty" jsonschema:"free-form genre labels, e.g. 회귀/빙의/환생"`
	LengthTarget  string   `json:"length_target,omitempty" jsonschema:"one of: flash, short, novella, novel, series; default series"`
	DefaultPOV    string   `json:"default_pov,omitempty" jsonschema:"one of: first, third_limited, omniscient; default first"`
	OutlinePreset string   `json:"outline_preset,omitempty" jsonschema:"one of: webnovel, novel; default webnovel"`
}

func (in createWorkInput) scope() (string, string) { return "", "" }

type createWorkOutput struct {
	ProjectID        string `json:"project_id"`
	Title            string `json:"title"`
	FirstSceneNodeID string `json:"first_scene_node_id" jsonschema:"the auto-created first scene; draft it with linetta_write_scene, passing expected_content_version 0"`
}

// maxSceneRunes caps one scene body. A runaway agent should hit a wall with a
// clear message rather than commit a megabyte to the manuscript.
const maxSceneRunes = 60000

// ---------- linetta_write_scene ----------

type writeSceneInput struct {
	NodeID    string `json:"node_id" jsonschema:"id of the scene to write"`
	ProjectID string `json:"project_id,omitempty" jsonschema:"optional; checked against the scene's work when given"`
	Text      string `json:"text" jsonschema:"the full scene body as plain prose; blank lines separate paragraphs"`
	// ExpectedContentVersion is the content_version from linetta_read_scene.
	// Required: it is what stops an agent from overwriting edits the writer
	// made after the agent last read the scene.
	// A pointer, not an int: a scene that has never been written has version 0,
	// so "absent" and "zero" must stay distinguishable or the first draft can
	// never be written.
	ExpectedContentVersion *int `json:"expected_content_version" jsonschema:"the content_version returned by linetta_read_scene"`
}

func (in writeSceneInput) scope() (string, string) { return in.ProjectID, in.NodeID }

type writeSceneOutput struct {
	NodeID         string `json:"node_id"`
	ContentVersion int    `json:"content_version"`
	WordCount      int    `json:"word_count"`
	// SnapshotID is the pre-write version. Reverting prose goes through this,
	// not through linetta_undo_last_change's batch id: undoing a structural
	// batch restores the outline and leaves scene bodies alone.
	SnapshotID string `json:"snapshot_id,omitempty"`
	// LinkedElements reports the registered story elements this write linked
	// automatically (#72), so the agent can spot a homonym gone wrong.
	LinkedElements []mention.Linked `json:"linked_elements,omitempty" jsonschema:"story elements named in the text, linked automatically"`
	// SummaryIsPlaceholder is always true after a write: Linetta fills the
	// scene summary with the opening lines cut to length, which is a
	// placeholder rather than a summary. Story briefs are built from these, so
	// leaving it costs the next brief its accuracy.
	SummaryIsPlaceholder bool `json:"summary_is_placeholder"`
}

// ---------- linetta_write_summary ----------

type writeSummaryInput struct {
	// Exactly one target. A scene or container summary feeds the story brief;
	// the synopsis is the work-level blurb.
	NodeID    string `json:"node_id,omitempty" jsonschema:"scene or container to summarize"`
	ProjectID string `json:"project_id,omitempty" jsonschema:"work whose synopsis to write; omit when node_id is set"`
	Summary   string `json:"summary" jsonschema:"3-5 sentences preserving characters, places, and key events"`
	// ExpectedContentVersion is required for scenes only — containers and the
	// synopsis have no version tracking their children's edits.
	ExpectedContentVersion *int `json:"expected_content_version,omitempty" jsonschema:"for a scene, the content_version from linetta_read_scene"`
}

func (in writeSummaryInput) scope() (string, string) { return in.ProjectID, in.NodeID }

type writeSummaryOutput struct {
	Target    string `json:"target"` // scene | container | synopsis
	NodeID    string `json:"node_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Summary   string `json:"summary"`
}

// registerWriteTools installs the mutating tools. Only called for
// settings.MCPModeFull.
func (d ToolDeps) registerWriteTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_create_work",
		Description: "Create a new work (novel) with its first chapter and scene, ready to draft. Returns " +
			"project_id and first_scene_node_id — write that scene with linetta_write_scene, passing " +
			"expected_content_version 0. Refused on a server restricted to a single work; the writer " +
			"lifts the restriction in Settings.",
	}, record(d, "linetta_create_work", d.createWork))

	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_write_scene",
		Description: "Replace a scene's body with new prose. Call linetta_read_scene first and pass the " +
			"content_version it returned: if the writer edited the scene since then the write is refused, " +
			"so their work is never silently overwritten. The previous text is snapshotted first and the " +
			"returned snapshot_id restores it. Registered story elements named in the text are linked " +
			"automatically and reported in linked_elements. Linetta then files the scene's opening lines " +
			"as a placeholder summary; call linetta_write_summary afterwards so the story brief carries a " +
			"real one.",
	}, record(d, "linetta_write_scene", d.writeScene))

	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_write_summary",
		Description: "Write the summary Linetta shows for a scene or chapter, or the work's synopsis. " +
			"Summaries feed linetta_get_story_context, so writing one after drafting keeps later briefs " +
			"accurate. Without one Linetta falls back to the opening lines cut to length, which says " +
			"what a scene opens on but not what happens in it — replacing that placeholder is what " +
			"this tool is for. " +
			"A scene summary needs the content_version from linetta_read_scene; chapters and the synopsis " +
			"do not.",
	}, record(d, "linetta_write_summary", d.writeSummary))

	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_edit_memory",
		Description: "Record something durable about how this writer works (writer_profile, which applies to every work) " +
			"or about this work (work_notes). Both are read back to you at the start of every session, so keep them " +
			"short and current: replace a line that changed rather than adding a second one. The result says how much " +
			"room is left.",
	}, record(d, "linetta_edit_memory", d.editMemory))

	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_edit_skill",
		Description: "Write one of your own skills: a durable how-to document you keep for next time — how " +
			"this writer wants fight scenes paced, how a recurring character speaks. Every skill's name and " +
			"description is put in front of you at the start of a session; linetta_read_skill opens one. " +
			"Scope writer makes it global, across every work; scope work ties it to the one work in " +
			"project_id. action create needs description and body; action patch takes either find and " +
			"replace for a targeted edit (find must appear exactly once in the body) or body to replace the " +
			"document wholesale, and can also change description or enabled on their own; action delete " +
			"removes it. Every change is versioned, so the writer can revert what you write. Keep a skill " +
			"specific and short — the description is one line saying when to reach for it, and bodies are " +
			"capped; the result says how much room is left.",
	}, record(d, "linetta_edit_skill", d.editSkill))

	d.registerReviseTool(s)
	d.registerBatchTools(s)
}

func (d ToolDeps) createWork(ctx context.Context, _ *mcp.CallToolRequest, in createWorkInput) (*mcp.CallToolResult, createWorkOutput, error) {
	// A restricted server promised the writer "this work only". A work the
	// agent creates but can never touch again would be a trap, so refuse
	// loudly instead.
	if restricted := d.allowedProjectID(); restricted != "" {
		return toolErr("this Linetta server is restricted to a single work; creating a new one is disabled — " +
			"the writer can lift the restriction in Settings"), createWorkOutput{}, nil
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return toolErr("title is required"), createWorkOutput{}, nil
	}
	// Defaults mirror the app's own new-work dialog: web-fiction series in
	// first person, webnovel outline.
	lengthTarget := in.LengthTarget
	if lengthTarget == "" {
		lengthTarget = project.LengthSeries
	}
	if !project.ValidLengthTarget(lengthTarget) {
		return toolErr("length_target %q is not valid; one of: flash, short, novella, novel, series", lengthTarget),
			createWorkOutput{}, nil
	}
	pov := in.DefaultPOV
	if pov == "" {
		pov = project.POVFirst
	}
	if !project.ValidDefaultPOV(pov) {
		return toolErr("default_pov %q is not valid; one of: first, third_limited, omniscient", pov),
			createWorkOutput{}, nil
	}
	preset := in.OutlinePreset
	if preset == "" {
		preset = project.OutlinePresetWebNovel
	}
	if !project.ValidOutlinePreset(preset) {
		return toolErr("outline_preset %q is not valid; one of: webnovel, novel", preset), createWorkOutput{}, nil
	}
	genres := in.Genres
	if genres == nil {
		genres = []string{}
	}
	p, err := d.Projects.Create(ctx, d.now(), project.NewInput{
		Title: title, Genres: genres, LengthTarget: lengthTarget, DefaultPOV: pov, OutlinePreset: preset,
	})
	if err != nil {
		return toolErr("could not create the work: %v", err), createWorkOutput{}, nil
	}
	first := ""
	if p.LastOpenedNodeID != nil {
		first = *p.LastOpenedNodeID
	}
	// The library view must learn a work appeared without being reopened.
	d.notifyChanged(p.ID, "linetta_create_work", nil, "")
	return nil, createWorkOutput{ProjectID: p.ID, Title: p.Title, FirstSceneNodeID: first}, nil
}

func (d ToolDeps) writeScene(ctx context.Context, _ *mcp.CallToolRequest, in writeSceneInput) (*mcp.CallToolResult, writeSceneOutput, error) {
	n, errResult := d.requireNodeInProject(ctx, in.NodeID, in.ProjectID)
	if errResult != nil {
		return errResult, writeSceneOutput{}, nil
	}
	if n.Kind != node.KindLeaf {
		return toolErr("node %q is a container (%s), not a scene; only scenes hold body text", n.ID, n.Label),
			writeSceneOutput{}, nil
	}
	if in.ExpectedContentVersion == nil {
		return toolErr("expected_content_version is required; call linetta_read_scene first and pass the value it returns"),
			writeSceneOutput{}, nil
	}
	expected := *in.ExpectedContentVersion
	if expected < 0 {
		return toolErr("expected_content_version must not be negative"), writeSceneOutput{}, nil
	}
	if count := len([]rune(in.Text)); count > maxSceneRunes {
		return toolErr("scene text is %d characters; the limit is %d — split it across scenes", count, maxSceneRunes),
			writeSceneOutput{}, nil
	}

	// Snapshot before touching anything, so the previous text survives even if
	// the write itself fails partway.
	snapshotID := ""
	if d.Snapshots != nil {
		beforeDoc := ""
		if n.ContentDoc != nil {
			beforeDoc = *n.ContentDoc
		}
		snap, created, err := d.Snapshots.CreateIfChanged(ctx, n.ID, beforeDoc, snapshot.ReasonCompanionBefore, d.now())
		if err != nil {
			return toolErr("could not snapshot the current text: %v", err), writeSceneOutput{}, nil
		}
		if created {
			snapshotID = snap.ID
		}
	}

	doc, err := storyops.PlainTextToTiptapDoc(in.Text)
	if err != nil {
		return toolErr("could not convert the text: %v", err), writeSceneOutput{}, nil
	}
	// Agent prose gets its registered names linked on the way in (#72):
	// without this, a scene written over MCP never reaches
	// linetta_where_does_appear or the story brief's mentioned-entities
	// section — the editor's scene-scan, same algorithm, is click-gated
	// because it rewrites what the writer typed; this body is machine-written
	// and snapshotted, a different consent regime. Linking is best-effort: a
	// failure here must not lose the prose.
	var linked []mention.Linked
	if d.Entities != nil {
		if ents, entErr := d.Entities.ListByProject(ctx, n.ProjectID); entErr == nil {
			if linkedDoc, l, linkErr := mention.AutoLinkDoc([]byte(doc), mention.BuildCandidates(ents)); linkErr == nil && len(l) > 0 {
				doc, linked = string(linkedDoc), l
			}
		}
	}
	if err := d.Nodes.UpdateContentIfVersion(ctx, n.ID, doc, expected, d.now()); err != nil {
		if errors.Is(err, node.ErrContentConflict) {
			return toolErr(
				"the scene changed since you read it (you passed version %d). Call linetta_read_scene again, "+
					"merge your changes into the current text, and retry with the fresh content_version.",
				expected), writeSceneOutput{}, nil
		}
		return toolErr("could not write the scene: %v", err), writeSceneOutput{}, nil
	}

	after, err := d.Nodes.Get(ctx, n.ID)
	if err != nil {
		return toolErr("wrote the scene but could not read it back: %v", err), writeSceneOutput{}, nil
	}
	// The summarizer keeps the story brief honest; without this an agent's
	// prose would never even get the short-scene plaintext summary.
	if d.EnqueueSummary != nil {
		d.EnqueueSummary(n.ID)
	}
	d.notifyChanged(n.ProjectID, "linetta_write_scene", []string{n.ID}, "")

	return nil, writeSceneOutput{
		NodeID:               after.ID,
		ContentVersion:       after.ContentVersion,
		WordCount:            after.WordCount,
		SnapshotID:           snapshotID,
		SummaryIsPlaceholder: true,
		LinkedElements:       linked,
	}, nil
}

func (d ToolDeps) writeSummary(ctx context.Context, _ *mcp.CallToolRequest, in writeSummaryInput) (*mcp.CallToolResult, writeSummaryOutput, error) {
	summary := strings.TrimSpace(in.Summary)
	if summary == "" {
		return toolErr("summary is required"), writeSummaryOutput{}, nil
	}
	nodeID := strings.TrimSpace(in.NodeID)
	projectID := strings.TrimSpace(in.ProjectID)
	if nodeID == "" && projectID == "" {
		return toolErr("pass node_id to summarize a scene or chapter, or project_id to write the work's synopsis"),
			writeSummaryOutput{}, nil
	}
	if nodeID != "" && projectID != "" {
		return toolErr("pass either node_id or project_id, not both"), writeSummaryOutput{}, nil
	}

	if nodeID == "" {
		p, errResult := d.requireProject(ctx, projectID)
		if errResult != nil {
			return errResult, writeSummaryOutput{}, nil
		}
		if _, err := d.Projects.Update(ctx, d.now(), project.UpdateInput{ID: p.ID, Synopsis: &summary}); err != nil {
			return toolErr("could not write the synopsis: %v", err), writeSummaryOutput{}, nil
		}
		d.notifyChanged(p.ID, "linetta_write_summary", nil, "")
		return nil, writeSummaryOutput{Target: "synopsis", ProjectID: p.ID, Summary: summary}, nil
	}

	n, errResult := d.requireNode(ctx, nodeID)
	if errResult != nil {
		return errResult, writeSummaryOutput{}, nil
	}
	target := "container"
	forVersion := n.ContentVersion
	if n.Kind == node.KindLeaf {
		target = "scene"
		// Scenes carry a version that tracks their own edits, so a summary
		// written against stale text must be refused — otherwise the brief
		// would report a fresh summary of prose that has since changed.
		if in.ExpectedContentVersion == nil {
			return toolErr("expected_content_version is required for a scene summary; call linetta_read_scene first"),
				writeSummaryOutput{}, nil
		}
		if *in.ExpectedContentVersion != n.ContentVersion {
			return toolErr(
				"the scene changed since you read it (you passed version %d, current is %d). "+
					"Re-read the scene and summarize the current text.",
				*in.ExpectedContentVersion, n.ContentVersion), writeSummaryOutput{}, nil
		}
		forVersion = *in.ExpectedContentVersion
	}

	if err := d.Nodes.SetSummary(ctx, n.ID, summary, forVersion); err != nil {
		return toolErr("could not write the summary: %v", err), writeSummaryOutput{}, nil
	}
	d.notifyChanged(n.ProjectID, "linetta_write_summary", []string{n.ID}, "")
	return nil, writeSummaryOutput{Target: target, NodeID: n.ID, Summary: summary}, nil
}

// ---------- linetta_edit_memory ----------

// memoryChangedPayload tells the app a memory moved under it. Settings holds
// an unsent textarea draft; without this, the writer's next blur would
// silently overwrite what the agent just recorded.
type memoryChangedPayload struct {
	Scope     string `json:"scope"`
	ProjectID string `json:"project_id,omitempty"`
	Source    string `json:"source"`
}

type editMemoryInput struct {
	Scope     string `json:"scope" jsonschema:"which memory: writer_profile (global - how this writer works, across every work) or work_notes (what you have learned about one work)"`
	Action    string `json:"action" jsonschema:"add, replace or remove"`
	Text      string `json:"text,omitempty" jsonschema:"the memory to record, one line; required for add and replace"`
	Find      string `json:"find,omitempty" jsonschema:"a short piece of the existing line you mean, unique among the lines; required for replace and remove"`
	ProjectID string `json:"project_id,omitempty" jsonschema:"the work whose notes to edit; required when scope is work_notes, and not accepted for writer_profile"`
}

// scope names the work the caller asked for (work notes only; the profile is
// global and belongs to no work) and, as the target, which of the two
// documents was edited.
//
// The target is what actually tells the two apart. It is tempting to say a
// work-notes row is the one carrying a work id, but that is not reliable:
// scope() sees the raw input, and only editMemory itself — through
// requireProject — resolves an omitted project_id to the work a restricted
// server is pinned to. A pinned client that omits the field therefore edits
// its one work and logs a row with no work id, exactly as it does for every
// other tool in this package (scope() is the same shape everywhere: raw
// input, no deps, no context). Without the target, that row would be
// indistinguishable from a writer-profile edit — and the scope string is
// these documents' only identity, since neither has an id of its own the way
// a node or a snapshot does.
func (in editMemoryInput) scope() (string, string) {
	return strings.TrimSpace(in.ProjectID), strings.TrimSpace(in.Scope)
}

type editMemoryOutput struct {
	Scope       string `json:"scope"`
	Body        string `json:"body"`
	CharsUsed   int    `json:"chars_used"`
	CharsBudget int    `json:"chars_budget"`
}

// editMemory is the whole memory surface: there is no read tool, because the
// documents are already in the story brief and every edit returns the result.
func (d ToolDeps) editMemory(ctx context.Context, _ *mcp.CallToolRequest, in editMemoryInput) (*mcp.CallToolResult, editMemoryOutput, error) {
	if d.Memory == nil {
		return toolErr("memory is unavailable in this build"), editMemoryOutput{}, nil
	}
	scope, err := agentmemory.ParseScope(in.Scope)
	if err != nil {
		return toolErr("%v", err), editMemoryOutput{}, nil
	}
	projectID := strings.TrimSpace(in.ProjectID)
	switch scope {
	case agentmemory.ScopeWorkNotes:
		p, errResult := d.requireProject(ctx, projectID)
		if errResult != nil {
			return errResult, editMemoryOutput{}, nil
		}
		// requireProject fills in the pinned work when the caller omitted one,
		// so take the id it resolved rather than the raw input.
		projectID = p.ID
	case agentmemory.ScopeWriterProfile:
		// The writer profile is global: it rides into the system prompt of
		// every work. A client the writer pinned to one work must not be able
		// to rewrite it — that is exactly what the pin exists to prevent, and
		// requireProject cannot enforce it here because there is no work to
		// check against. The built-in agent is unaffected: allowedProjectID
		// returns "" for SourceAgent.
		if restricted := d.allowedProjectID(); restricted != "" {
			return toolErr("this Linetta server is restricted to work %q, and the writer profile is "+
				"global — it applies to every work, so it is outside that restriction. Use scope "+
				"work_notes to record something about this work.", restricted), editMemoryOutput{}, nil
		}
	}
	// One transaction, not Load-then-Apply-then-Save: this document has two
	// writers by design (the panel's agent and a connected external client),
	// and a read-modify-write split across three calls loses whichever edit
	// lands second-to-last while telling its caller it succeeded.
	saved, err := d.Memory.Edit(ctx, scope, projectID, in.Action, in.Find, in.Text, d.now())
	if err != nil {
		return toolErr("%v", err), editMemoryOutput{}, nil
	}
	if d.Notify != nil {
		d.Notify("memory.changed", memoryChangedPayload{
			Scope: string(scope), ProjectID: projectID, Source: d.sourceOrExternal(),
		})
	}
	return nil, editMemoryOutput{
		Scope: string(saved.Scope), Body: saved.Body,
		CharsUsed: saved.CharsUsed, CharsBudget: saved.CharsBudget,
	}, nil
}

// ---------- linetta_edit_skill ----------

// Actions linetta_edit_skill accepts. Named rather than inlined so the
// handler's switch and its refusal message cannot drift apart.
const (
	skillActionCreate = "create"
	skillActionPatch  = "patch"
	skillActionDelete = "delete"
)

// skillChangedPayload tells the app a skill moved under it — Settings lists
// skills and holds an editor open on one, and the panel's next turn builds
// its prompt from the same documents.
//
// It matches memoryChangedPayload field for field, plus the name a skill has
// and a memory document does not, so a listener that already handles
// memory.changed does not have to learn a second shape for this one.
type skillChangedPayload struct {
	Scope     string `json:"scope"`
	ProjectID string `json:"project_id,omitempty"`
	Name      string `json:"name"`
	Source    string `json:"source"`
}

type editSkillInput struct {
	Action      string `json:"action" jsonschema:"create, patch or delete"`
	Name        string `json:"name" jsonschema:"the skill's name: a lowercase slug of letters, digits and hyphens, e.g. fight-scenes"`
	Scope       string `json:"scope" jsonschema:"writer (global - applies to every work) or work (belongs to one work)"`
	ProjectID   string `json:"project_id,omitempty" jsonschema:"the work the skill belongs to; required when scope is work, and not accepted for writer"`
	Description string `json:"description,omitempty" jsonschema:"one line saying when to reach for this skill; required for create, optional for patch"`
	Body        string `json:"body,omitempty" jsonschema:"the skill's markdown body; required for create, and for a patch that replaces the document wholesale"`
	Find        string `json:"find,omitempty" jsonschema:"for patch: a piece of the existing body that appears exactly once, to be replaced"`
	Replace     string `json:"replace,omitempty" jsonschema:"for patch: what to put in place of find; empty removes the found text"`
	// Enabled is a pointer so "the agent said nothing" stays distinguishable
	// from "the agent said false": a patch must leave the flag alone unless
	// it was asked to change it, and false is the interesting value here.
	Enabled *bool `json:"enabled,omitempty" jsonschema:"whether Linetta offers this skill to you; defaults to true on create, unchanged on patch"`
}

// scope names the work (work-scoped skills only; a writer skill is global and
// belongs to none) and, as the target, the skill written — so the activity
// row the writer reads says which document changed, not merely that some
// skill did. Like every other scope() in this package it sees the raw input
// and resolves nothing: a pinned client that omits project_id logs a row with
// no work id, exactly as it does for the other tools.
func (in editSkillInput) scope() (string, string) {
	return strings.TrimSpace(in.ProjectID), strings.TrimSpace(in.Name)
}

type editSkillOutput struct {
	Action      string `json:"action"`
	Name        string `json:"name"`
	Scope       string `json:"scope"`
	ProjectID   string `json:"project_id,omitempty"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	Body        string `json:"body,omitempty"`
	BodyRunes   int    `json:"body_runes"`
	BodyBudget  int    `json:"body_budget" jsonschema:"the cap a skill body is held to, so you can see how much room is left"`
	UpdatedAt   int64  `json:"updated_at,omitempty"`
}

// editSkill creates, patches and deletes skills.
//
// Both collaborators are required up front, not just the store: a write that
// landed on disk with no version row behind it is a change the writer cannot
// revert, which is worse than a refusal they can read.
func (d ToolDeps) editSkill(ctx context.Context, _ *mcp.CallToolRequest, in editSkillInput) (*mcp.CallToolResult, editSkillOutput, error) {
	if d.Skills == nil || d.SkillHistory == nil {
		return toolErr("skills are unavailable in this build"), editSkillOutput{}, nil
	}
	scope, projectID, name, errResult := d.requireSkillTarget(ctx, in.Scope, in.ProjectID, in.Name)
	if errResult != nil {
		return errResult, editSkillOutput{}, nil
	}
	// A writer skill is global: it is offered to the agent in every work. A
	// client the writer pinned to one work must not be able to write one —
	// that is exactly what the pin exists to prevent, and requireSkillTarget
	// cannot enforce it because there is no work to check against. The
	// built-in agent is unaffected: allowedProjectID returns "" for
	// SourceAgent. Reading such a skill stays allowed; only writing one
	// reaches past the pin.
	if scope == agentskills.ScopeWriter {
		if restricted := d.allowedProjectID(); restricted != "" {
			return toolErr("this Linetta server is restricted to work %q, and a %q skill is global — it "+
				"steers every work, so it is outside that restriction. Use scope %q to write a skill for "+
				"this work.", restricted, agentskills.ScopeWriter, agentskills.ScopeWork), editSkillOutput{}, nil
		}
	}

	now := d.now()
	var saved agentskills.Skill
	var reason string

	switch strings.TrimSpace(in.Action) {
	case skillActionCreate:
		res, s := d.createSkill(scope, projectID, name, in, now)
		if res != nil {
			return res, editSkillOutput{}, nil
		}
		saved, reason = s, agentskills.ReasonCreated

	case skillActionPatch:
		res, s := d.patchSkill(scope, projectID, name, in, now)
		if res != nil {
			return res, editSkillOutput{}, nil
		}
		saved, reason = s, agentskills.ReasonEdited

	case skillActionDelete:
		// The version row for a deletion carries the last body, so the writer
		// can restore straight from the row marked deleted rather than having
		// to reach for the one before it. That means reading the skill before
		// removing it, and refusing a name that is not there — an agent that
		// deleted the wrong name has to be told.
		cur, err := d.Skills.Read(scope, projectID, name)
		if err != nil {
			return skillReadErr(scope, name, err), editSkillOutput{}, nil
		}
		if err := d.Skills.Delete(scope, projectID, name); err != nil {
			return toolErr("could not delete skill %q: %v", name, err), editSkillOutput{}, nil
		}
		saved, reason = cur, agentskills.ReasonDeleted

	default:
		return toolErr("unknown action %q; use %s, %s or %s",
			in.Action, skillActionCreate, skillActionPatch, skillActionDelete), editSkillOutput{}, nil
	}

	// The version lands before the tool returns, so nothing an agent is told
	// succeeded is missing from the history the writer reverts through. A
	// failure here is logged rather than returned: the skill on disk has
	// already changed, and reporting an error for a change that DID happen
	// would send the agent off repairing something that is not broken.
	if err := d.SkillHistory.Record(ctx, saved, reason, now); err != nil {
		logf("skill history: %v", err)
	}
	if d.Notify != nil {
		d.Notify("skills.changed", skillChangedPayload{
			Scope: string(scope), ProjectID: projectID, Name: name, Source: d.sourceOrExternal(),
		})
	}

	out := editSkillOutput{
		Action: reason, Name: saved.Name, Scope: string(scope), ProjectID: projectID,
		Description: saved.Description, Enabled: saved.Enabled,
		BodyRunes: saved.BodyRunes, BodyBudget: agentskills.MaxBodyRunes,
	}
	if reason != agentskills.ReasonDeleted {
		out.Body = saved.Body
		out.UpdatedAt = saved.UpdatedAt
	}
	return nil, out, nil
}

// createSkill writes a skill that must not already exist. A non-nil result is
// the refusal to return; the Skill is only meaningful when it is nil.
func (d ToolDeps) createSkill(scope agentskills.Scope, projectID, name string, in editSkillInput, now int64) (*mcp.CallToolResult, agentskills.Skill) {
	// Store.Write is an upsert — overwriting is normal there — so the plain
	// "already exists" meaning belongs to this action, and is checked here.
	// Without it, create silently destroys a skill the agent meant to extend.
	if _, err := d.Skills.Read(scope, projectID, name); err == nil {
		return toolErr("a %s skill named %q already exists; read it with linetta_read_skill and use "+
			"action %s to change it, or pick another name", scope, name, skillActionPatch), agentskills.Skill{}
	} else if !errors.Is(err, agentskills.ErrNotFound) {
		return toolErr("could not check whether skill %q exists: %v", name, err), agentskills.Skill{}
	}

	description := strings.TrimSpace(in.Description)
	if description == "" {
		return toolErr("description is required for %s: one line saying when to reach for this skill",
			skillActionCreate), agentskills.Skill{}
	}
	if strings.TrimSpace(in.Body) == "" {
		return toolErr("body is required for %s: the skill itself, as markdown", skillActionCreate),
			agentskills.Skill{}
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	saved, err := d.Skills.Write(agentskills.Skill{
		Name: name, Scope: scope, ProjectID: projectID,
		Description: description, Author: agentskills.AuthorAgent,
		Enabled: enabled, Body: in.Body,
	}, now)
	if err != nil {
		return toolErr("could not write skill %q: %v", name, err), agentskills.Skill{}
	}
	return nil, saved
}

// patchSkill changes a skill that must already exist. A non-nil result is the
// refusal to return; the Skill is only meaningful when it is nil.
func (d ToolDeps) patchSkill(scope agentskills.Scope, projectID, name string, in editSkillInput, now int64) (*mcp.CallToolResult, agentskills.Skill) {
	cur, err := d.Skills.Read(scope, projectID, name)
	if err != nil {
		return skillReadErr(scope, name, err), agentskills.Skill{}
	}

	// find/replace and a wholesale body are two different intentions, and
	// guessing which one a call carrying both meant is how an agent's
	// half-formed edit becomes a lost document.
	if in.Find != "" && in.Body != "" {
		return toolErr("give either find and replace, or body — not both: find makes a targeted edit, " +
			"body replaces the whole document"), agentskills.Skill{}
	}

	next := cur
	changed := false
	switch {
	case in.Find != "":
		// Exactly one match, the rule agentmemory's own find follows. Zero
		// means the agent is working from a body it has not read; more than
		// one means it has not said which, and editing the first would
		// corrupt a document it cannot see.
		switch n := strings.Count(cur.Body, in.Find); {
		case n == 0:
			return toolErr("find text does not appear in skill %q; read it with linetta_read_skill first",
				name), agentskills.Skill{}
		case n > 1:
			return toolErr("find text appears %d times in skill %q; give a longer piece that appears "+
				"exactly once", n, name), agentskills.Skill{}
		}
		next.Body = strings.Replace(cur.Body, in.Find, in.Replace, 1)
		changed = true
	case in.Body != "":
		next.Body = in.Body
		changed = true
	}
	if description := strings.TrimSpace(in.Description); description != "" {
		next.Description = description
		changed = true
	}
	if in.Enabled != nil && *in.Enabled != cur.Enabled {
		next.Enabled = *in.Enabled
		changed = true
	}
	if !changed {
		return toolErr("nothing to change: a %s needs find and replace, or body, or description, or enabled",
			skillActionPatch), agentskills.Skill{}
	}

	// The author becomes the agent because the agent is what wrote THIS
	// version, even when the writer authored the one before it.
	next.Author = agentskills.AuthorAgent
	saved, err := d.Skills.Write(next, now)
	if err != nil {
		return toolErr("could not write skill %q: %v", name, err), agentskills.Skill{}
	}
	return nil, saved
}

// skillReadErr turns a failed Store.Read into the refusal the agent reads. A
// missing skill is the common case and gets a sentence about what to do next;
// anything else says what actually went wrong.
func skillReadErr(scope agentskills.Scope, name string, err error) *mcp.CallToolResult {
	if errors.Is(err, agentskills.ErrNotFound) {
		return toolErr("no %s skill named %q; use one of the skills already listed for you, or create "+
			"this one with action %s", scope, name, skillActionCreate)
	}
	return toolErr("could not read skill %q: %v", name, err)
}
