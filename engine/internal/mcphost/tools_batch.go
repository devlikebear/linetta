//go:build !mobile

package mcphost

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/storyops"
)

// ---------- linetta_apply_story_ops ----------

type applyStoryOpsInput struct {
	ProjectID string        `json:"project_id" jsonschema:"id of the work to change"`
	NodeID    string        `json:"node_id,omitempty" jsonschema:"the scene ops without an explicit target apply to"`
	Summary   string        `json:"summary" jsonschema:"one line describing the change, shown to the writer"`
	Ops       []storyops.Op `json:"ops" jsonschema:"the mutations to apply as one batch"`
}

func (in applyStoryOpsInput) scope() (string, string) { return in.ProjectID, in.NodeID }

type applyStoryOpsOutput struct {
	Applied      int               `json:"applied"`
	Created      map[string]string `json:"created,omitempty"`
	Failures     []opFailure       `json:"failures,omitempty"`
	RolledBack   bool              `json:"rolled_back,omitempty"`
	UndoBatchID  string            `json:"undo_batch_id,omitempty"`
	ChangedNodes []string          `json:"changed_nodes,omitempty"`
}

type opFailure struct {
	Index int    `json:"index"`
	Op    string `json:"op,omitempty"`
	Error string `json:"error"`
}

// ---------- linetta_create_checkpoint ----------

type createCheckpointInput struct {
	NodeID    string `json:"node_id" jsonschema:"scene to checkpoint"`
	ProjectID string `json:"project_id,omitempty" jsonschema:"optional; checked against the scene's work when given"`
}

func (in createCheckpointInput) scope() (string, string) { return in.ProjectID, in.NodeID }

type createCheckpointOutput struct {
	SnapshotID string `json:"snapshot_id"`
	NodeID     string `json:"node_id"`
	Created    bool   `json:"created"`
}

// ---------- linetta_undo_last_change ----------

type undoInput struct {
	// Exactly one. A batch id undoes a structural change from
	// linetta_apply_story_ops; a snapshot id restores a scene's prose.
	BatchID    string `json:"batch_id,omitempty" jsonschema:"undo_batch_id from linetta_apply_story_ops"`
	SnapshotID string `json:"snapshot_id,omitempty" jsonschema:"snapshot_id from linetta_write_scene or linetta_create_checkpoint"`
}

func (in undoInput) scope() (string, string) { return "", in.SnapshotID }

type undoOutput struct {
	Reverted string `json:"reverted"` // outline | scene
	NodeID   string `json:"node_id,omitempty"`
}

func (d ToolDeps) registerBatchTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_apply_story_ops",
		Description: "Apply a batch of story changes: outline nodes, storylines and beats, characters, " +
			"places, relationships, Fact Book cards, and memories. Outline-structural batches are " +
			"all-or-nothing — if one op fails the outline is put back — and return undo_batch_id. " +
			"Entity, thread, beat, fact, and memory changes apply individually and are NOT batch-undoable; " +
			"revert those with follow-up ops. " +
			"Scene prose is NOT written here; use linetta_write_scene, which checks the version first.",
	}, record(d, "linetta_apply_story_ops", d.applyStoryOps))

	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_create_checkpoint",
		Description: "Save a restore point for a scene before a large rewrite. Returns a snapshot_id " +
			"linetta_undo_last_change can restore.",
	}, record(d, "linetta_create_checkpoint", d.createCheckpoint))

	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_undo_last_change",
		Description: "Undo a change you just made. Pass batch_id to revert a structural batch from " +
			"linetta_apply_story_ops, or snapshot_id to restore a scene's previous text. These are " +
			"different paths: undoing a batch restores the outline and leaves scene bodies alone.",
	}, record(d, "linetta_undo_last_change", d.undoLastChange))
}

func (d ToolDeps) applyStoryOps(ctx context.Context, _ *mcp.CallToolRequest, in applyStoryOpsInput) (*mcp.CallToolResult, applyStoryOpsOutput, error) {
	p, errResult := d.requireProject(ctx, in.ProjectID)
	if errResult != nil {
		return errResult, applyStoryOpsOutput{}, nil
	}
	if len(in.Ops) == 0 {
		return toolErr("ops is required"), applyStoryOpsOutput{}, nil
	}
	// One door per mutation type. The applier's set_scene_text overwrites
	// unconditionally, so letting it through here would route around
	// linetta_write_scene's version check entirely.
	for i, op := range in.Ops {
		if op.Type == "set_scene_text" {
			return toolErr(
					"ops[%d] set_scene_text is not accepted here; use linetta_write_scene, which requires the "+
						"content_version so the writer's own edits cannot be overwritten", i),
				applyStoryOpsOutput{}, nil
		}
	}
	nodeID := strings.TrimSpace(in.NodeID)
	if nodeID != "" {
		if _, errResult := d.requireNode(ctx, nodeID); errResult != nil {
			return errResult, applyStoryOpsOutput{}, nil
		}
	}
	if d.Story == nil {
		return toolErr("story operations are unavailable in this build"), applyStoryOpsOutput{}, nil
	}

	result := d.Story.ApplyOps(ctx, p.ID, nodeID, storyops.Proposal{
		Summary: strings.TrimSpace(in.Summary), Ops: in.Ops,
	}, d.now)

	out := applyStoryOpsOutput{
		Applied:     result.Applied,
		Created:     result.Created,
		RolledBack:  result.RolledBack,
		UndoBatchID: result.UndoBatchID,
	}
	for _, ch := range result.ChangedNodes {
		out.ChangedNodes = append(out.ChangedNodes, ch.NodeID)
	}
	for _, f := range result.Failures {
		out.Failures = append(out.Failures, opFailure{Index: f.Index, Op: f.Op, Error: f.Error})
	}
	if result.IsError() {
		// Reported as a tool error so the agent notices, with the structured
		// detail kept so it can see which op failed and why.
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: applyFailureText(result)}},
		}, out, nil
	}
	if result.Applied > 0 {
		d.notifyChanged(p.ID, "linetta_apply_story_ops", out.ChangedNodes, result.UndoBatchID)
	}
	return nil, out, nil
}

func applyFailureText(r storyops.ApplyOpsResult) string {
	var b strings.Builder
	if r.RolledBack {
		b.WriteString("the batch failed and the outline was put back; nothing was applied. ")
	}
	for _, f := range r.Failures {
		if f.Index >= 0 {
			fmt.Fprintf(&b, "ops[%d] %s: %s. ", f.Index, f.Op, f.Error)
			continue
		}
		fmt.Fprintf(&b, "%s. ", f.Error)
	}
	return strings.TrimSpace(b.String())
}

func (d ToolDeps) createCheckpoint(ctx context.Context, _ *mcp.CallToolRequest, in createCheckpointInput) (*mcp.CallToolResult, createCheckpointOutput, error) {
	n, errResult := d.requireNodeInProject(ctx, in.NodeID, in.ProjectID)
	if errResult != nil {
		return errResult, createCheckpointOutput{}, nil
	}
	if d.Snapshots == nil {
		return toolErr("version history is unavailable in this build"), createCheckpointOutput{}, nil
	}
	doc := ""
	if n.ContentDoc != nil {
		doc = *n.ContentDoc
	}
	snap, created, err := d.Snapshots.CreateIfChanged(ctx, n.ID, doc, snapshot.ReasonManual, d.now())
	if err != nil {
		return toolErr("could not save a checkpoint: %v", err), createCheckpointOutput{}, nil
	}
	if !created {
		// Nothing changed since the last snapshot, so the newest one already is
		// this checkpoint. Hand back its id rather than a confusing empty value.
		if latest, err := d.Snapshots.LatestForNode(ctx, n.ID); err == nil {
			return nil, createCheckpointOutput{SnapshotID: latest.ID, NodeID: n.ID, Created: false}, nil
		}
	}
	return nil, createCheckpointOutput{SnapshotID: snap.ID, NodeID: n.ID, Created: created}, nil
}

func (d ToolDeps) undoLastChange(ctx context.Context, _ *mcp.CallToolRequest, in undoInput) (*mcp.CallToolResult, undoOutput, error) {
	batchID := strings.TrimSpace(in.BatchID)
	snapshotID := strings.TrimSpace(in.SnapshotID)
	switch {
	case batchID == "" && snapshotID == "":
		return toolErr("pass batch_id to undo a structural batch, or snapshot_id to restore a scene's text"),
			undoOutput{}, nil
	case batchID != "" && snapshotID != "":
		return toolErr("pass either batch_id or snapshot_id, not both"), undoOutput{}, nil
	case batchID != "":
		if d.Story == nil {
			return toolErr("story operations are unavailable in this build"), undoOutput{}, nil
		}
		if err := d.Story.UndoApply(ctx, batchID, d.now); err != nil {
			if errors.Is(err, storyops.ErrUndoBatchNotFound) {
				return toolErr("undo is no longer available for that change"), undoOutput{}, nil
			}
			return toolErr("could not undo the change: %v", err), undoOutput{}, nil
		}
		d.notifyChanged("", "linetta_undo_last_change", nil, batchID)
		return nil, undoOutput{Reverted: "outline"}, nil
	}

	if d.Snapshots == nil {
		return toolErr("version history is unavailable in this build"), undoOutput{}, nil
	}
	snap, err := d.Snapshots.GetByID(ctx, snapshotID)
	if err != nil {
		return toolErr("snapshot %q not found", snapshotID), undoOutput{}, nil
	}
	n, errResult := d.requireNode(ctx, snap.NodeID)
	if errResult != nil {
		return errResult, undoOutput{}, nil
	}
	// Snapshot the current text first, so restoring is itself revertible.
	curDoc := ""
	if n.ContentDoc != nil {
		curDoc = *n.ContentDoc
	}
	if _, _, err := d.Snapshots.CreateIfChanged(ctx, n.ID, curDoc, snapshot.ReasonManual, d.now()); err != nil {
		return toolErr("could not snapshot before restoring: %v", err), undoOutput{}, nil
	}
	if err := d.Nodes.UpdateContent(ctx, n.ID, snap.ContentDoc, d.now()); err != nil {
		return toolErr("could not restore the scene: %v", err), undoOutput{}, nil
	}
	if d.EnqueueSummary != nil {
		d.EnqueueSummary(n.ID)
	}
	d.notifyChanged(n.ProjectID, "linetta_undo_last_change", []string{n.ID}, "")
	return nil, undoOutput{Reverted: "scene", NodeID: n.ID}, nil
}
