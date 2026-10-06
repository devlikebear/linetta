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
