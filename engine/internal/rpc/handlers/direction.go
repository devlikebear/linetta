package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/direction"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// direction.* serve the writer's own direction (story-assist Phase 1): the
// work's current focus and a scene's intent. The app's RPC surface is the
// only way to write either — there is deliberately no MCP tool for them,
// so an agent can read but never set either.

func directionError(err error) error {
	reasons := []struct {
		err    error
		reason string
	}{
		{direction.ErrProjectNotFound, rpc.ReasonProjectNotFound},
		{direction.ErrNodeNotFound, rpc.ReasonNodeNotFound},
		{direction.ErrFocusTooLong, rpc.ReasonDirectionFocusTooLong},
		{direction.ErrGoalTooLong, rpc.ReasonDirectionGoalTooLong},
		{direction.ErrTooManyItems, rpc.ReasonDirectionTooManyItems},
		{direction.ErrItemTooLong, rpc.ReasonDirectionItemTooLong},
		{direction.ErrNotScene, rpc.ReasonDirectionNotScene},
	}
	for _, r := range reasons {
		if errors.Is(err, r.err) {
			return rpc.NotFound(r.reason, err.Error())
		}
	}
	return &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
}

// GetFocus returns a handler for direction.get_focus.
func GetFocus(repo *direction.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		out, err := repo.GetFocus(ctx, in.ProjectID)
		if err != nil {
			return nil, directionError(err)
		}
		return json.Marshal(out)
	}
}

// SetFocus returns a handler for direction.set_focus. An empty body clears
// the focus; the engine, not the caller, picks the anchor episode (the last
// one with words in it right now) — see direction.Repo.SetFocus.
func SetFocus(repo *direction.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
			Body      string `json:"body"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		out, err := repo.SetFocus(ctx, now(), in.ProjectID, in.Body)
		if err != nil {
			return nil, directionError(err)
		}
		return json.Marshal(out)
	}
}

// getSceneIntentResult is direction.get_scene_intent's reply. Present is
// false — and Intent zero — when the scene has no intent saved.
type getSceneIntentResult struct {
	Present bool                  `json:"present"`
	Intent  direction.SceneIntent `json:"intent"`
}

// GetSceneIntent returns a handler for direction.get_scene_intent.
func GetSceneIntent(repo *direction.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			NodeID string `json:"node_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.NodeID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		intent, present, err := repo.GetSceneIntent(ctx, in.NodeID)
		if err != nil {
			return nil, directionError(err)
		}
		if !present {
			intent.NodeID = in.NodeID
			intent.Keep = []string{}
			intent.Avoid = []string{}
		}
		return json.Marshal(getSceneIntentResult{Present: present, Intent: intent})
	}
}

// SetSceneIntent returns a handler for direction.set_scene_intent. goal,
// keep and avoid all empty clears the scene's intent.
func SetSceneIntent(repo *direction.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			NodeID string   `json:"node_id"`
			Goal   string   `json:"goal"`
			Keep   []string `json:"keep"`
			Avoid  []string `json:"avoid"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.NodeID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		out, err := repo.SetSceneIntent(ctx, now(), direction.SceneIntent{
			NodeID: in.NodeID, Goal: in.Goal, Keep: in.Keep, Avoid: in.Avoid,
		})
		if err != nil {
			return nil, directionError(err)
		}
		return json.Marshal(out)
	}
}
