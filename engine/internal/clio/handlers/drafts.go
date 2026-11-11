package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/clio/draft"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// drafts.* serve Clio's editor. Updates are optimistic: the caller sends the
// content_version it last saw and a stale one yields CodeContentConflict.

func draftErr(err error) error {
	switch {
	case errors.Is(err, draft.ErrConflict):
		return &rpc.MethodError{Code: rpc.CodeContentConflict, Message: err.Error()}
	case errors.Is(err, draft.ErrNotFound):
		return clioInvalid(err.Error())
	default:
		return &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
	}
}

func draftContext(platform, kind string) error {
	if !clioPlatform(platform) || !clioKind(kind) {
		return clioInvalid("unknown platform or kind")
	}
	return nil
}

func parseDraftParams(params json.RawMessage, in any) error {
	if err := json.Unmarshal(params, in); err != nil {
		return clioInvalid("invalid params")
	}
	return nil
}

// CreateDraft returns a handler for drafts.create.
func CreateDraft(repo *draft.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			Platform string `json:"platform"`
			Kind     string `json:"kind"`
			Body     string `json:"body"`
		}
		if err := parseDraftParams(params, &in); err != nil {
			return nil, err
		}
		if err := draftContext(in.Platform, in.Kind); err != nil {
			return nil, err
		}
		d, err := repo.Create(ctx, in.Platform, in.Kind, in.Body)
		if err != nil {
			return nil, draftErr(err)
		}
		return json.Marshal(d)
	}
}

func draftIDHandler(fn func(ctx context.Context, id int64) (any, error)) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ID int64 `json:"id"`
		}
		if err := parseDraftParams(params, &in); err != nil {
			return nil, err
		}
		if in.ID <= 0 {
			return nil, clioInvalid("id is required")
		}
		out, err := fn(ctx, in.ID)
		if err != nil {
			return nil, draftErr(err)
		}
		return json.Marshal(out)
	}
}

// GetDraft returns a handler for drafts.get.
func GetDraft(repo *draft.Repo) rpc.Handler {
	return draftIDHandler(func(ctx context.Context, id int64) (any, error) { return repo.Get(ctx, id) })
}

// DeleteDraft returns a handler for drafts.delete.
func DeleteDraft(repo *draft.Repo) rpc.Handler {
	return draftIDHandler(func(ctx context.Context, id int64) (any, error) {
		return struct{}{}, repo.Delete(ctx, id)
	})
}

// ListDrafts returns a handler for drafts.list.
func ListDrafts(repo *draft.Repo) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		list, err := repo.List(ctx)
		if err != nil {
			return nil, draftErr(err)
		}
		return json.Marshal(list)
	}
}

// UpdateDraft returns a handler for drafts.update.
func UpdateDraft(repo *draft.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ID             int64  `json:"id"`
			ContentVersion int64  `json:"content_version"`
			Body           string `json:"body"`
			Platform       string `json:"platform"`
			Kind           string `json:"kind"`
		}
		if err := parseDraftParams(params, &in); err != nil {
			return nil, err
		}
		if in.ID <= 0 || in.ContentVersion <= 0 {
			return nil, clioInvalid("id and content_version are required")
		}
		if err := draftContext(in.Platform, in.Kind); err != nil {
			return nil, err
		}
		d, err := repo.Update(ctx, in.ID, in.ContentVersion, in.Body, in.Platform, in.Kind)
		if err != nil {
			return nil, draftErr(err)
		}
		return json.Marshal(d)
	}
}
