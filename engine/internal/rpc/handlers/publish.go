package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/publish"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// publish.* connect this device to Linetta Works and put episodes on it
// (Phase 3). Nothing here reaches the site before the writer connects.

// PublishConnector is the connection half of internal/publish.
type PublishConnector interface {
	Start(ctx context.Context) (publish.ConnectStatus, error)
	Status() publish.ConnectStatus
	Disconnect() error
}

// PublishService is the publishing half of internal/publish.
type PublishService interface {
	WorkStatus(ctx context.Context, projectID string) (publish.WorkStatus, error)
	UpsertWork(ctx context.Context, in publish.UpsertInput) (publish.WorkView, error)
	PublishEpisode(ctx context.Context, nodeID string) (publish.PublishResult, error)
	SetEpisodeVisibility(ctx context.Context, projectID, nodeID, visibility string) (publish.EpisodeView, error)
}

func publishError(err error) error {
	var me *rpc.MethodError
	if errors.As(err, &me) {
		return me
	}
	return rpc.MethodErrorFrom(err)
}

func invalidParams(message string) error {
	return &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: message}
}

// PublishConnectStart returns a handler for publish.connect_start. The pane
// sends consented only once the writer has ticked the box saying published
// text leaves the device; without it nothing is sent.
func PublishConnectStart(c PublishConnector) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			Consented bool `json:"consented"`
		}
		_ = json.Unmarshal(params, &in)
		if !in.Consented {
			return nil, rpc.MethodErrorFrom(&rpc.ReasonError{
				Reason: rpc.ReasonPublishConsentRequired,
				Err:    errors.New("publish: the writer has not agreed to send text to the site"),
			})
		}
		st, err := c.Start(ctx)
		if err != nil {
			return nil, publishError(err)
		}
		return json.Marshal(st)
	}
}

// PublishConnectStatus returns a handler for publish.connect_status. It
// never carries the token.
func PublishConnectStatus(c PublishConnector) rpc.Handler {
	return func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(c.Status())
	}
}

// PublishDisconnect returns a handler for publish.disconnect. It also
// abandons a connection attempt still waiting for the browser.
func PublishDisconnect(c PublishConnector) rpc.Handler {
	return func(context.Context, json.RawMessage) (json.RawMessage, error) {
		if err := c.Disconnect(); err != nil {
			return nil, publishError(err)
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

// PublishWorkStatus returns a handler for publish.work_status.
func PublishWorkStatus(svc PublishService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, invalidParams("project_id required")
		}
		out, err := svc.WorkStatus(ctx, in.ProjectID)
		if err != nil {
			return nil, publishError(err)
		}
		return json.Marshal(out)
	}
}

// PublishWorkUpsert returns a handler for publish.work_upsert.
func PublishWorkUpsert(svc PublishService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in publish.UpsertInput
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, invalidParams("project_id required")
		}
		out, err := svc.UpsertWork(ctx, in)
		if err != nil {
			return nil, publishError(err)
		}
		return json.Marshal(out)
	}
}

// PublishEpisode returns a handler for publish.episode_publish. node_id may
// be the episode or any scene inside it.
func PublishEpisode(svc PublishService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			NodeID string `json:"node_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.NodeID) == "" {
			return nil, invalidParams("node_id required")
		}
		out, err := svc.PublishEpisode(ctx, in.NodeID)
		if err != nil {
			return nil, publishError(err)
		}
		return json.Marshal(out)
	}
}

// PublishEpisodeSetVisibility returns a handler for
// publish.episode_set_visibility.
func PublishEpisodeSetVisibility(svc PublishService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID  string `json:"project_id"`
			NodeID     string `json:"node_id"`
			Visibility string `json:"visibility"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" || strings.TrimSpace(in.NodeID) == "" {
			return nil, invalidParams("project_id and node_id required")
		}
		out, err := svc.SetEpisodeVisibility(ctx, in.ProjectID, in.NodeID, in.Visibility)
		if err != nil {
			return nil, publishError(err)
		}
		return json.Marshal(out)
	}
}

// PublishApprovals is the queue of publish requests an agent made and the
// writer has not answered yet (#238).
type PublishApprovals interface {
	Pending() []publish.ApprovalRequest
	Answer(id string, approved bool) error
}

// PublishApprovalsPending returns a handler for publish.approvals_pending:
// every request still waiting, so a window that opens late shows them too.
func PublishApprovalsPending(a PublishApprovals) rpc.Handler {
	return func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(map[string][]publish.ApprovalRequest{"pending": a.Pending()})
	}
}

// PublishApprovalAnswer returns a handler for publish.approval_answer: the
// writer approves or declines one request. Only the app calls this; no MCP
// tool can reach it.
func PublishApprovalAnswer(a PublishApprovals) rpc.Handler {
	return func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ID       string `json:"id"`
			Approved *bool  `json:"approved"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ID) == "" || in.Approved == nil {
			return nil, invalidParams("id and approved required")
		}
		if err := a.Answer(in.ID, *in.Approved); err != nil {
			if errors.Is(err, publish.ErrApprovalUnknown) {
				return nil, rpc.NotFound(rpc.ReasonPublishApprovalGone, "the request is no longer waiting")
			}
			return nil, publishError(err)
		}
		return json.Marshal(map[string]bool{"ok": true})
	}
}
