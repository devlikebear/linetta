package handlers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/semantic"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

// SemanticDeps is what the meaning-search methods (#384) work with.
type SemanticDeps struct {
	Service  *semantic.Service
	Models   *semantic.Models
	Settings *settings.Store
	// Supported is false on a build that cannot run the model.
	Supported bool
}

type semanticStatus struct {
	Supported bool                 `json:"supported"`
	Enabled   bool                 `json:"enabled"`
	Model     semantic.ModelStatus `json:"model"`
	Index     *semantic.Progress   `json:"index,omitempty"`
}

func (d SemanticDeps) status(ctx context.Context, projectID string) (semanticStatus, error) {
	out := semanticStatus{Supported: d.Supported, Enabled: d.Settings.SemanticSearchEnabled(), Model: d.Models.Status()}
	if strings.TrimSpace(projectID) != "" {
		p, err := d.Service.Progress(ctx, projectID)
		if err != nil {
			return out, err
		}
		out.Index = &p
	}
	return out, nil
}

// SemanticStatus handles search.semantic_status {project_id?}: whether meaning
// search is on, the model's state, and how far the work is embedded.
func SemanticStatus(d SemanticDeps) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id,omitempty"`
		}
		_ = json.Unmarshal(params, &in)
		out, err := d.status(ctx, in.ProjectID)
		if err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return json.Marshal(out)
	}
}

// SemanticEnable handles search.semantic_enable: switches meaning search on
// and fetches the model if it is not here yet. Every work is embedded in the
// background once the model is in place.
func SemanticEnable(d SemanticDeps) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		if !d.Supported {
			return nil, rpc.NotFound(rpc.ReasonSemanticUnsupported, semantic.ErrUnsupported.Error())
		}
		on := true
		if _, err := d.Settings.Set(ctx, settings.Patch{SemanticSearchEnabled: &on}); err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		kick := func(err error) {
			if err == nil {
				_ = d.Service.KickAll(context.Background())
			}
		}
		if d.Models.Status().Installed {
			kick(nil)
		} else {
			d.Models.Download(kick)
		}
		out, err := d.status(ctx, "")
		if err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return json.Marshal(out)
	}
}

// SemanticDisable handles search.semantic_disable {delete_model?}: switches
// meaning search off; with delete_model, also removes the model and every
// vector made with it.
func SemanticDisable(d SemanticDeps) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			DeleteModel bool `json:"delete_model,omitempty"`
		}
		_ = json.Unmarshal(params, &in)
		off := false
		if _, err := d.Settings.Set(ctx, settings.Patch{SemanticSearchEnabled: &off}); err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		if in.DeleteModel {
			if err := d.Models.Delete(); err != nil {
				return nil, rpc.MethodErrorFrom(err)
			}
			if err := d.Service.Forget(ctx); err != nil {
				return nil, rpc.MethodErrorFrom(err)
			}
		}
		out, err := d.status(ctx, "")
		if err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return json.Marshal(out)
	}
}
