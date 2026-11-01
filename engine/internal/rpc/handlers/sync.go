package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// sync.* drive automatic sync on this device. They answer in terms of the
// runner's status, which only the desktop build has, so the shapes stay opaque
// here.

// SyncControl is the runner as the handlers see it.
type SyncControl interface {
	Status() any
	Preview(ctx context.Context) (any, error)
	RunNow()
	Flush(projectID string)
	Touch(projectID string)
	SetAuto(ctx context.Context, enabled bool) (any, error)
	SetActive(ctx context.Context, projectID string, active bool) (pulled bool, err error)
}

// SyncStatus returns a handler for sync.status.
func SyncStatus(control SyncControl) rpc.Handler {
	return func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(control.Status())
	}
}

// SyncPreview returns a handler for sync.preview: what turning automatic sync on
// would send, read from this device alone.
func SyncPreview(control SyncControl) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return vaultResponse(control.Preview(ctx))
	}
}

// SyncRunNow returns a handler for sync.run_now. With project_id the work goes up
// without waiting to settle, for an app going to the background.
func SyncRunNow(control SyncControl) rpc.Handler {
	return func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if len(params) > 0 {
			if err := json.Unmarshal(params, &in); err != nil {
				return nil, invalidParams("invalid params")
			}
		}
		if in.ProjectID != "" {
			control.Flush(in.ProjectID)
		} else {
			control.RunNow()
		}
		return json.Marshal(control.Status())
	}
}

// SyncSetAuto returns a handler for sync.set_auto.
func SyncSetAuto(control SyncControl) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.Unmarshal(params, &in); err != nil || in.Enabled == nil {
			return nil, invalidParams("enabled required")
		}
		return vaultResponse(control.SetAuto(ctx, *in.Enabled))
	}
}

// SyncTouch returns a handler for sync.touch: an edit was saved, so the work's
// upload waits for the writer to pause.
func SyncTouch(control SyncControl) rpc.Handler {
	return func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, invalidParams("project_id required")
		}
		control.Touch(in.ProjectID)
		return json.Marshal(map[string]bool{"ok": true})
	}
}

// SyncSetActive returns a handler for sync.set_active. Opening a work takes a
// newer version first. A failure there comes back in the answer rather than as
// an error, because the work still opens: problem is a reason code to show.
func SyncSetActive(control SyncControl) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
			Active    *bool  `json:"active"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" || in.Active == nil {
			return nil, invalidParams("project_id and active required")
		}
		out := struct {
			Pulled  bool   `json:"pulled"`
			Problem string `json:"problem,omitempty"`
		}{}
		pulled, err := control.SetActive(ctx, in.ProjectID, *in.Active)
		out.Pulled = pulled
		var re *rpc.ReasonError
		if errors.As(err, &re) {
			out.Problem = re.Reason
		} else if err != nil {
			return nil, publishError(err)
		}
		return json.Marshal(out)
	}
}
