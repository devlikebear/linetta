package handlers

import (
	"context"
	"encoding/json"

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
