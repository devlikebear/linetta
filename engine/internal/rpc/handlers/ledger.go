package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/ledger"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// ledger.* and nodes.set_plan let the writer read and correct what agents keep
// about the work. Everything here is the writer's own edit.

func ledgerError(err error) error {
	if errors.Is(err, ledger.ErrNotFound) {
		return &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
	}
	return &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
}

// ListLedger returns a handler for ledger.list: every entry and part arc.
func ListLedger(repo *ledger.Repo, nodes *node.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		entries, err := repo.List(ctx, ledger.Filter{ProjectID: in.ProjectID})
		if err != nil {
			return nil, ledgerError(err)
		}
		arcs, err := repo.ListArcs(ctx, in.ProjectID)
		if err != nil {
			return nil, ledgerError(err)
		}
		all, err := nodes.ListByProject(ctx, in.ProjectID)
		if err != nil {
			return nil, err
		}
		type part struct {
			NodeID string `json:"node_id"`
			Label  string `json:"label"`
			ledger.Arc
		}
		labels := make(map[string]string, len(all))
		parts := []part{}
		for _, n := range all {
			labels[n.ID] = n.Label
			if n.Kind == node.KindContainer && n.ParentID == nil {
				arc := arcs[n.ID]
				arc.NodeID = n.ID
				parts = append(parts, part{NodeID: n.ID, Label: n.Label, Arc: arc})
			}
		}
		return json.Marshal(struct {
			Entries []ledger.Entry    `json:"entries"`
			Parts   []part            `json:"parts"`
			Labels  map[string]string `json:"labels"`
		}{entries, parts, labels})
	}
}

// SaveLedgerEntry returns a handler for ledger.save, which adds an entry when
// no id is given.
func SaveLedgerEntry(repo *ledger.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ID        string `json:"id"`
			ProjectID string `json:"project_id"`
			Kind      string `json:"kind"`
			ledger.Patch
		}
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if in.ID != "" {
			e, err := repo.Update(ctx, now(), in.ID, in.Patch)
			if err != nil {
				return nil, ledgerError(err)
			}
			return json.Marshal(e)
		}
		e := ledger.Entry{ProjectID: in.ProjectID, Kind: in.Kind, Source: ledger.SourceWriter}
		if in.Subject != nil {
			e.Subject = *in.Subject
		}
		if in.Body != nil {
			e.Body = *in.Body
		}
		saved, err := repo.Add(ctx, now(), e)
		if err != nil {
			return nil, ledgerError(err)
		}
		return json.Marshal(saved)
	}
}

// DeleteLedgerEntry returns a handler for ledger.delete.
func DeleteLedgerEntry(repo *ledger.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || in.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		if err := repo.Delete(ctx, in.ID); err != nil {
			return nil, ledgerError(err)
		}
		return json.Marshal(struct {
			OK bool `json:"ok"`
		}{true})
	}
}

// SetPartArc returns a handler for ledger.set_arc.
func SetPartArc(repo *ledger.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in ledger.Arc
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		a, err := repo.SetArc(ctx, now(), in)
		if err != nil {
			return nil, ledgerError(err)
		}
		return json.Marshal(a)
	}
}

// SetNodePlan returns a handler for nodes.set_plan.
func SetNodePlan(nodes *node.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ID   string `json:"id"`
			Plan string `json:"plan"`
		}
		if err := json.Unmarshal(params, &in); err != nil || in.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		if err := nodes.SetPlan(ctx, in.ID, strings.TrimSpace(in.Plan)); err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			OK bool `json:"ok"`
		}{true})
	}
}
