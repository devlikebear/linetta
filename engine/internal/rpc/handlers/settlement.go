package handlers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/ledger"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// PendingSettlementItem is one scene waiting for settlement, with the labels
// the panel names it by: its own, and its parent's when it has one.
type PendingSettlementItem struct {
	NodeID      string `json:"node_id"`
	Label       string `json:"label"`
	ParentLabel string `json:"parent_label,omitempty"`
}

// PendingSettlement returns a handler for ledger.pending_settlement: whether
// settlement is on, and the scenes whose current prose it has not settled yet,
// in story order. before_node_id narrows it to the scenes ahead of one.
func PendingSettlement(settlements *ledger.Settlements, nodes *node.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID    string `json:"project_id"`
			BeforeNodeID string `json:"before_node_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		out := struct {
			Enabled bool                    `json:"enabled"`
			Items   []PendingSettlementItem `json:"items"`
		}{Enabled: settlements.Enabled(), Items: []PendingSettlementItem{}}
		ids, err := settlements.Pending(ctx, in.ProjectID, strings.TrimSpace(in.BeforeNodeID))
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return json.Marshal(out)
		}
		all, err := nodes.ListByProject(ctx, in.ProjectID)
		if err != nil {
			return nil, err
		}
		byID := make(map[string]node.Node, len(all))
		for _, n := range all {
			byID[n.ID] = n
		}
		for _, id := range ids {
			n := byID[id]
			item := PendingSettlementItem{NodeID: id, Label: n.Label}
			if n.ParentID != nil {
				item.ParentLabel = byID[*n.ParentID].Label
			}
			out.Items = append(out.Items, item)
		}
		return json.Marshal(out)
	}
}
