package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
)

type manualSnapshotParams struct {
	NodeID string `json:"node_id"`
	Doc    string `json:"doc"`
}

// CreateManualSnapshot returns a handler for snapshots.create_manual.
func CreateManualSnapshot(snaps *snapshot.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p manualSnapshotParams
		if err := json.Unmarshal(params, &p); err != nil || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id and doc required"}
		}
		got, err := snaps.Create(ctx, p.NodeID, p.Doc, snapshot.ReasonManual, now())
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(got)
	}
}

type autoSnapshotParams struct {
	NodeID string `json:"node_id"`
	Doc    string `json:"doc"`
}

// CreateAutoSnapshot returns a handler for snapshots.create_auto. It records an
// autosave snapshot only when content changed since the node's latest snapshot.
func CreateAutoSnapshot(snaps *snapshot.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p autoSnapshotParams
		if err := json.Unmarshal(params, &p); err != nil || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id and doc required"}
		}
		got, created, err := snaps.CreateIfChanged(ctx, p.NodeID, p.Doc, snapshot.ReasonAutosave, now())
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if !created {
			return json.RawMessage(`{"skipped":true}`), nil
		}
		return json.Marshal(got)
	}
}

type listSnapshotsParams struct {
	NodeID string `json:"node_id"`
}

// ListSnapshotsForNode returns a handler for snapshots.list_for_node.
func ListSnapshotsForNode(snaps *snapshot.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p listSnapshotsParams
		if err := json.Unmarshal(params, &p); err != nil || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id required"}
		}
		entries, err := snaps.ListForNode(ctx, p.NodeID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if entries == nil {
			entries = []snapshot.Entry{}
		}
		return json.Marshal(entries)
	}
}

type compareSnapshotsParams struct {
	LeftID  string `json:"left_id"`
	RightID string `json:"right_id"`
}

// CompareSnapshots returns plaintext bodies for two snapshots from the same node.
func CompareSnapshots(snaps *snapshot.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p compareSnapshotsParams
		if err := json.Unmarshal(params, &p); err != nil || p.LeftID == "" || p.RightID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "left_id and right_id required"}
		}
		left, err := snaps.GetByID(ctx, p.LeftID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		right, err := snaps.GetByID(ctx, p.RightID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if left.NodeID != right.NodeID {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "snapshots belong to different nodes"}
		}
		got := snapshot.CompareResult{
			Left: snapshot.CompareSide{
				ID:        left.ID,
				Reason:    left.Reason,
				CreatedAt: left.CreatedAt,
				Plaintext: snapshot.PlaintextFromDoc(left.ContentDoc),
			},
			Right: snapshot.CompareSide{
				ID:        right.ID,
				Reason:    right.Reason,
				CreatedAt: right.CreatedAt,
				Plaintext: snapshot.PlaintextFromDoc(right.ContentDoc),
			},
		}
		return json.Marshal(got)
	}
}

type restoreSnapshotParams struct {
	SnapshotID string `json:"snapshot_id"`
}

// RestoreSnapshot snapshots the node's current body as `manual` (so the restore
// itself is undoable), then writes the snapshot's doc back into the node.
// Returns the updated node.
func RestoreSnapshot(nodes *node.Repo, snaps *snapshot.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p restoreSnapshotParams
		if err := json.Unmarshal(params, &p); err != nil || p.SnapshotID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "snapshot_id required"}
		}
		snap, err := snaps.GetByID(ctx, p.SnapshotID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		current, err := nodes.Get(ctx, snap.NodeID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		curDoc := ""
		if current.ContentDoc != nil {
			curDoc = *current.ContentDoc
		}
		if _, err := snaps.Create(ctx, snap.NodeID, curDoc, snapshot.ReasonManual, now()); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if err := nodes.UpdateContent(ctx, snap.NodeID, snap.ContentDoc, now()); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		updated, err := nodes.Get(ctx, snap.NodeID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(updated)
	}
}
