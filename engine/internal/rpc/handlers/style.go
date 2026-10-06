package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/style"
)

// style.* serve the style section of the canon panel (#162): the writer's
// checkable rules, and the check that holds the manuscript against them.

func styleError(err error) error {
	reasons := []struct {
		err    error
		reason string
	}{
		{style.ErrProjectNotFound, rpc.ReasonProjectNotFound},
		{style.ErrTooManyPhrases, rpc.ReasonStyleTooManyPhrases},
		{style.ErrPhraseTooLong, rpc.ReasonStylePhraseTooLong},
		{style.ErrSentenceLimit, rpc.ReasonStyleSentenceLimit},
		{style.ErrNoDraft, rpc.ReasonStyleNoDraft},
		{style.ErrDraftEmpty, rpc.ReasonStyleProfileEmpty},
		{style.ErrNotesTooLong, rpc.ReasonStyleNotesTooLong},
		{style.ErrDraftTooLong, rpc.ReasonStyleNotesTooLong},
	}
	for _, r := range reasons {
		if errors.Is(err, r.err) {
			return rpc.NotFound(r.reason, err.Error())
		}
	}
	return &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
}

// GetStyleRules returns a handler for style.get_rules.
func GetStyleRules(repo *style.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		out, err := repo.GetRules(ctx, in.ProjectID)
		if err != nil {
			return nil, styleError(err)
		}
		return json.Marshal(out)
	}
}

// SetStyleRules returns a handler for style.set_rules.
func SetStyleRules(repo *style.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in style.Rules
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		out, err := repo.SetRules(ctx, now(), in)
		if err != nil {
			return nil, styleError(err)
		}
		return json.Marshal(out)
	}
}

// styleCheckResult is style.check's reply: the rules the check ran against
// and what it found, so the panel never shows a report beside rules the
// writer has since changed.
type styleCheckResult struct {
	Rules style.Rules `json:"rules"`
	style.Report
}

// CheckStyle returns a handler for style.check. node_id narrows the check to
// one scene, or to the scenes under one chapter; without it the whole work is
// checked. The check reads the manuscript and writes nothing.
func CheckStyle(repo *style.Repo, nodes *node.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
			NodeID    string `json:"node_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		rules, err := repo.GetRules(ctx, in.ProjectID)
		if err != nil {
			return nil, styleError(err)
		}
		list, err := nodes.ListByProject(ctx, in.ProjectID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if in.NodeID != "" && !containsNode(list, in.NodeID) {
			return nil, rpc.NotFound(rpc.ReasonNodeNotFound, "node not found in this work")
		}
		return json.Marshal(styleCheckResult{Rules: rules, Report: style.Check(style.ScenesUnder(list, in.NodeID), rules)})
	}
}

func containsNode(list []node.Node, id string) bool {
	for _, n := range list {
		if n.ID == id {
			return true
		}
	}
	return false
}

// styleDraftResult is style.get_draft's reply. Pending is false — and Draft
// empty — when nothing is waiting, so the panel can tell "no draft" from a
// failed read.
type styleDraftResult struct {
	Pending bool        `json:"pending"`
	Draft   style.Draft `json:"draft"`
	// Limit is the size an approved profile may be, in characters.
	Limit int `json:"limit"`
}

// GetStyleDraft returns a handler for style.get_draft: the profile an agent
// proposed and the writer has not yet approved or discarded (#163).
func GetStyleDraft(repo *style.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		draft, pending, err := repo.GetDraft(ctx, in.ProjectID)
		if err != nil {
			return nil, styleError(err)
		}
		return json.Marshal(styleDraftResult{Pending: pending, Draft: draft, Limit: style.MaxProfileRunes})
	}
}

// ApproveStyleDraft returns a handler for style.approve_draft. This is the
// writer's act and the only way a draft becomes style notes; it is on the
// app's RPC surface and has no MCP counterpart, so an agent cannot approve
// its own proposal.
func ApproveStyleDraft(repo *style.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
			// Body is the text the writer approved: the draft, or their edit of it.
			Body string `json:"body"`
			// Mode is replace or append.
			Mode string `json:"mode"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		notes, err := repo.ApproveDraft(ctx, now(), in.ProjectID, in.Body, in.Mode)
		if errors.Is(err, style.ErrApproveMode) {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if err != nil {
			return nil, styleError(err)
		}
		return json.Marshal(map[string]string{"style_notes": notes})
	}
}

// DiscardStyleDraft returns a handler for style.discard_draft.
func DiscardStyleDraft(repo *style.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		if err := repo.DiscardDraft(ctx, in.ProjectID); err != nil {
			return nil, styleError(err)
		}
		return json.Marshal(map[string]bool{"ok": true})
	}
}
