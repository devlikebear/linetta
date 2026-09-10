//go:build !mobile

package engineapp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/agent"
	"github.com/devlikebear/linetta/engine/internal/agentmemory"
	"github.com/devlikebear/linetta/engine/internal/agentskills"
	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/provider"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/storyops"
)

const agentAvailable = true

// agentDeps is what setupAgent needs from register().
type agentDeps struct {
	tools    mcphost.ToolDeps // the external host's deps, cloned below
	story    *storyops.Service
	history  *companion.HistoryRepo
	projects *project.Repo
	nodes    *node.Repo
	settings *settings.Store
	src      *provider.Source
	memory   *agentmemory.Repo
	skills   *agentskills.Store
	notify   func(method string, params any)
	clock    func() int64
}

// agentMemorySource adapts *agentmemory.Repo to agent.MemorySource. The two
// reads are independent and best-effort, matching
// companion.Service.CuratedMemory: a read failure leaves that document empty
// (at its correct budget) rather than failing the whole turn over one of two
// unrelated rows.
type agentMemorySource struct {
	repo *agentmemory.Repo
}

func (m agentMemorySource) Memories(ctx context.Context, projectID string) (writerProfile, workNotes agentmemory.Document) {
	writerProfile = agentmemory.Document{Scope: agentmemory.ScopeWriterProfile, CharsBudget: agentmemory.ScopeWriterProfile.Budget()}
	workNotes = agentmemory.Document{Scope: agentmemory.ScopeWorkNotes, CharsBudget: agentmemory.ScopeWorkNotes.Budget()}
	if m.repo == nil {
		return writerProfile, workNotes
	}
	if doc, err := m.repo.Load(ctx, agentmemory.ScopeWriterProfile, ""); err == nil {
		writerProfile = doc
	}
	if doc, err := m.repo.Load(ctx, agentmemory.ScopeWorkNotes, projectID); err == nil {
		workNotes = doc
	}
	return writerProfile, workNotes
}

// agentSkillSource adapts *agentskills.Store to agent.SkillSource. It reads
// both scopes a turn can see — the writer's global skills and this work's —
// and returns only what agent.SkillSource promises: enabled skills with
// Body left off. A nil store (the zero value) yields no skills rather than
// panicking, matching agentMemorySource's nil-repo case above — but
// setupAgent never builds one that way on purpose: it leaves agent.Deps.Skills
// nil instead, so an unwired store is a nil field a guard can see rather than
// a populated field that silently lists nothing. See setupAgent's comment.
//
// It does not call agentskills.Guard itself. Store.List already runs Guard
// on every entry it reads and reports a failure as a Diagnostic instead of
// returning the skill — see List's doc comment and agentskills'
// TestGuardFailureIsADiagnostic — so a skill an invisible character got
// hidden inside never leaves the store to begin with. Re-guarding here would
// be the same check run twice, not a second line of defence. The
// diagnostics themselves are dropped on the floor rather than surfaced to
// the model: a broken SKILL.md is the writer's problem to fix (visible in
// Settings' skills panel), not something worth spending the agent's turn
// explaining. That also answers the "skip vs. refuse the whole block"
// question the store's guard already decided: skip, so a single skill an
// invisible character got into does not go on to silence the other
// thirty-nine.
//
// A read failure for one scope (e.g. a permissions problem List would
// otherwise surface as an error) is swallowed the same way: best-effort,
// matching agentMemorySource's per-document reads — an agent turn should not
// fail over the skill list any more than over a memory document.
type agentSkillSource struct {
	store *agentskills.Store
}

func (a agentSkillSource) Skills(_ context.Context, projectID string) []agentskills.Skill {
	if a.store == nil {
		return nil
	}
	out := make([]agentskills.Skill, 0)
	out = appendEnabledSkills(out, a.store, agentskills.ScopeWriter, "")
	// Work-scoped skills need a work open to belong to; a fresh session
	// with no manuscript selected simply has none to offer.
	if id := strings.TrimSpace(projectID); id != "" {
		out = appendEnabledSkills(out, a.store, agentskills.ScopeWork, id)
	}
	return out
}

// appendEnabledSkills, which performs the reduction described above, lives in
// skill_context.go: the story brief's own skill source needs exactly the same
// one, and that file is untagged because the MCP brief is served on builds
// this file is not compiled into.

// scopeLookup answers the two names the scope line carries. Failures are
// silent on purpose: a missing title must not cost the writer their turn.
type scopeLookup struct {
	projects *project.Repo
	nodes    *node.Repo
}

func (s scopeLookup) ProjectTitle(ctx context.Context, id string) string {
	p, err := s.projects.Get(ctx, id)
	if err != nil {
		return ""
	}
	return p.Title
}

func (s scopeLookup) NodeLabel(ctx context.Context, id string) string {
	if id == "" {
		return ""
	}
	n, err := s.nodes.Get(ctx, id)
	if err != nil {
		return ""
	}
	if n.Title != "" {
		return n.Title
	}
	return n.Label
}

// agentController adapts *agent.Service to handlers.AgentController.
type agentController struct {
	svc *agent.Service
	// tools is the composed ToolDeps the agent registers on its own server —
	// kept here (unexported, same package) purely so agent_wiring_test.go can
	// confirm the three deliberate differences from the external host's deps
	// (Source, Limiter, Story) actually landed. None of the five agent.*
	// methods surfaces them on the wire when no provider is configured, so
	// without this a regression that dropped one of the three would compile,
	// pass every other test, and only show up as a writer's agent starving
	// against — or reverting — an external client's work.
	tools mcphost.ToolDeps
	// deps is the agent.Deps literal setupAgent built, kept for the same
	// reason tools is: agent.Deps is a large struct of collaborators, none of
	// which is visible on the wire on a fresh install with no provider
	// configured, so a forgotten field there is exactly as silent as
	// ToolDeps.Memory was. TestProductionAgentDepsCarryEveryCollaborator
	// walks it reflectively.
	deps agent.Deps
	// notify is the same channel the tools themselves publish mcp.changed on.
	// Undo below is the one mutation in this file that does not go through a
	// tool, so it is the one place that has to emit the notification itself.
	notify func(method string, params any)
}

func setupAgent(deps agentDeps) (*agentController, func() error) {
	// The agent's own tool deps: same repos, three deliberate differences.
	//
	//  - Source marks every row it writes in the activity log, and exempts it
	//    from the mcp_project_id restriction meant for external clients.
	//  - Its own limiter: sharing the external one lets a busy Claude Desktop
	//    session starve the panel, and vice versa.
	//  - Its own storyops service: undo batches live in memory on the
	//    service, so this is what stops the panel's undo button reverting an
	//    external agent's batch.
	tools := deps.tools
	tools.Source = mcphost.SourceAgent
	tools.Limiter = mcphost.NewLimiter()
	tools.Story = deps.story

	// A nil deps.skills must leave Deps.Skills NIL, not an agentSkillSource
	// wrapping a nil store. Both answer "no skills" at runtime, but only one
	// of them can be noticed: a non-nil interface over a nil pointer looks
	// wired to every guard that checks the field, so dropping the
	// `skills: skillsStore` line in engineapp.go would compile, pass, and
	// ship an agent whose system prompt tells it every turn to read skills
	// from a list that is permanently empty. Nil here is what
	// TestProductionAgentDepsCarryEveryCollaborator can actually catch —
	// the same lesson as ToolDeps.Memory in #97.
	var skills agent.SkillSource
	if deps.skills != nil {
		skills = agentSkillSource{store: deps.skills}
	}

	built := agent.Deps{
		Providers: deps.src,
		History:   deps.history,
		Scope:     scopeLookup{projects: deps.projects, nodes: deps.nodes},
		Register: func(s *mcp.Server, groups mcphost.ToolGroups) {
			// Always full: an agent that cannot write the manuscript has no
			// reason to exist. settings.MCPMode governs external clients only.
			//
			// groups comes from the turn, not from the store: the two funcs
			// below are the only reading of the switches on this path.
			tools.Register(s, settings.MCPModeFull, groups)
		},
		Notify:   deps.notify,
		Language: deps.settings.Language,
		Memory:   agentMemorySource{repo: deps.memory},
		Skills:   skills,
		// Read per turn, like Language: switching the self-review off in
		// Settings has to take effect on the writer's very next message.
		SelfReviewEnabled: deps.settings.AgentSelfReviewEnabled,
		// The tool budget (#99), and the ONLY place it is read on the agent's
		// path. Run calls these once per turn and passes the one answer to
		// both halves: to the prompt, and to Register above as its groups
		// argument. That is what makes "the prompt and the tool list cannot
		// disagree" true rather than merely likely — while Register read the
		// store for itself, a settings.Set landing between the two readings
		// built a server whose tools and whose cache-key label came from
		// different answers, and the label being the key, every later turn
		// inherited it.
		MemoryToolsEnabled: deps.settings.MemoryToolsEnabled,
		SkillToolsEnabled:  deps.settings.SkillToolsEnabled,
		Undo: func(ctx context.Context, batchID, snapshotID string) (string, string, error) {
			if batchID != "" {
				return "", "", deps.story.UndoApply(ctx, batchID, deps.clock)
			}
			n, err := tools.RestoreSnapshot(ctx, snapshotID)
			if err != nil {
				return "", "", err
			}
			return n.ProjectID, n.ID, nil
		},
		Clock: deps.clock,
	}
	svc := agent.New(built)
	return &agentController{svc: svc, tools: tools, deps: built, notify: deps.notify}, svc.Close
}

func (c *agentController) Run(ctx context.Context, projectID, nodeID, prompt string) (string, error) {
	return c.svc.Run(ctx, agent.RunRequest{ProjectID: projectID, NodeID: nodeID, Prompt: prompt})
}

func (c *agentController) Cancel(_ context.Context, runID string) error {
	return c.svc.Cancel(runID)
}

func (c *agentController) History(ctx context.Context, projectID string, limit int) (json.RawMessage, error) {
	msgs, err := c.svc.History(ctx, projectID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]agentHistoryRow, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, agentHistoryRow{
			ID: m.ID, ProjectID: m.ProjectID, NodeID: m.NodeID, NodeLabel: m.NodeLabel,
			RunID: m.RunID, Role: m.Role, Status: m.Status, Content: m.Content, CreatedAt: m.CreatedAt,
		})
	}
	return json.Marshal(out)
}

// agentHistoryRow is the panel's shape. companion.HistoryMessage has no JSON
// tags — it is an internal row — so it is projected rather than marshalled.
type agentHistoryRow struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id,omitempty"`
	NodeLabel string `json:"node_label,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
}

func (c *agentController) Clear(ctx context.Context, projectID string) error {
	return c.svc.Clear(ctx, projectID)
}

// Undo reverts what the agent last did: a structural batch (batchID) or a
// scene write's pre-write snapshot (snapshotID) — see rpc/handlers.AgentUndo
// for the exactly-one-of validation this relies on. A batch that has aged out
// of storyops' in-memory undo window, or a snapshot the writer's own cleanup
// has since dropped, is not the writer's mistake — it is the ordinary result
// of a restart, a few more turns, or normal retention — so both map to the
// same reason code rather than surfacing the collaborator's English sentence
// verbatim.
//
// A successful revert emits mcp.changed, exactly as the equivalent tool paths
// do (mcphost.ToolDeps.undoLastChange, via UndoApply/RestoreSnapshot). It has
// to: a batch undo's RestoreOutline deletes the nodes the batch created, and a
// snapshot undo overwrites a scene's body outside the write path the panel
// otherwise learns from — mcp.changed is the ONLY signal the workspace
// refreshes from either way. Without it the sidebar and open editor keep
// showing stale content immediately after the writer's own undo reported
// success.
//
// The batch path's empty project id matches undoLastChange's own call: the
// batch id does not carry a work, and useMcpChanges treats an empty
// project_id as "refresh regardless" rather than filtering the event away.
// The snapshot path always has a project id and node id — svc.Undo hands
// back the restored scene's — so those are carried instead.
func (c *agentController) Undo(ctx context.Context, batchID, snapshotID string) error {
	projectID, nodeID, err := c.svc.Undo(ctx, batchID, snapshotID)
	if err != nil {
		if errors.Is(err, storyops.ErrUndoBatchNotFound) || errors.Is(err, snapshot.ErrNotFound) {
			return &rpc.ReasonError{Reason: rpc.ReasonAgentUndoUnavailable, Err: err}
		}
		return err
	}
	if c.notify != nil {
		payload := mcphost.ChangedPayload{
			Tool:    "linetta_undo_last_change",
			BatchID: batchID,
			Source:  mcphost.SourceAgent,
		}
		if nodeID != "" {
			payload.ProjectID = projectID
			payload.NodeIDs = []string{nodeID}
		}
		c.notify("mcp.changed", payload)
	}
	return nil
}
