package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type plotSpinePanelParams struct {
	NodeID string `json:"node_id"`
}

// PlotSpinePanel returns a handler for plot.spine_panel. It returns the
// prev/current/next scene beats for the inline plot panel.
func PlotSpinePanel(builder *plot.Builder) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p plotSpinePanelParams
		if err := json.Unmarshal(params, &p); err != nil || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id required"}
		}
		spine, err := builder.Build(ctx, p.NodeID)
		if err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(spine)
	}
}
