package handlers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/vault"
)

// vault.* keep works sealed on Linetta Works (Phase 11). They share the publish
// connection and nothing else: nothing here reaches the public site's works.

// VaultService is internal/vault's service.
type VaultService interface {
	Status(ctx context.Context) (vault.Overview, error)
	WorkStatus(ctx context.Context, projectID string) (vault.WorkStatus, error)
	Setup(ctx context.Context, in vault.SetupInput) (vault.SetupResult, error)
	Push(ctx context.Context, projectID string) (vault.PushResult, error)
	Pull(ctx context.Context, in vault.PullInput) (vault.PullResult, error)
	ResolveConflict(ctx context.Context, in vault.ResolveInput) (vault.ResolveResult, error)
	DeleteItem(ctx context.Context, itemID string) error
}

func vaultResponse(out any, err error) (json.RawMessage, error) {
	if err != nil {
		return nil, publishError(err)
	}
	return json.Marshal(out)
}

// VaultStatus returns a handler for vault.status.
func VaultStatus(svc VaultService) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return vaultResponse(svc.Status(ctx))
	}
}

// VaultWorkStatus returns a handler for vault.work_status.
func VaultWorkStatus(svc VaultService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, invalidParams("project_id required")
		}
		return vaultResponse(svc.WorkStatus(ctx, in.ProjectID))
	}
}

// VaultSetup returns a handler for vault.setup. Its response is the one place
// the recovery key crosses the RPC boundary, and only on the call that made it.
func VaultSetup(svc VaultService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in vault.SetupInput
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, invalidParams("invalid params")
		}
		return vaultResponse(svc.Setup(ctx, in))
	}
}

// VaultPush returns a handler for vault.push.
func VaultPush(svc VaultService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, invalidParams("project_id required")
		}
		return vaultResponse(svc.Push(ctx, in.ProjectID))
	}
}

// VaultPull returns a handler for vault.pull. project_id names a work on this
// device and item_id one that is not yet; revision 0 is the head.
func VaultPull(svc VaultService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in vault.PullInput
		if err := json.Unmarshal(params, &in); err != nil || (strings.TrimSpace(in.ProjectID) == "" && strings.TrimSpace(in.ItemID) == "") {
			return nil, invalidParams("project_id or item_id required")
		}
		return vaultResponse(svc.Pull(ctx, in))
	}
}

// VaultResolveConflict returns a handler for vault.resolve_conflict.
func VaultResolveConflict(svc VaultService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in vault.ResolveInput
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, invalidParams("project_id required")
		}
		switch {
		case in.Choice == vault.ChoiceKeepLocal:
		case in.Choice == vault.ChoiceTakeRemote && strings.TrimSpace(in.TitleSuffix) != "":
		default:
			return nil, invalidParams("choice must be keep_local, or take_remote with title_suffix")
		}
		return vaultResponse(svc.ResolveConflict(ctx, in))
	}
}

// VaultDeleteItem returns a handler for vault.delete_item.
func VaultDeleteItem(svc VaultService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ItemID string `json:"item_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ItemID) == "" {
			return nil, invalidParams("item_id required")
		}
		if err := svc.DeleteItem(ctx, in.ItemID); err != nil {
			return nil, publishError(err)
		}
		return json.Marshal(map[string]bool{"ok": true})
	}
}
