package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// Clock is an injected millisecond-precision source. Tests pass deterministic
// values; production passes time.Now().UnixMilli wrappers.
type Clock func() int64

// CreateProject returns a handler for projects.create.
func CreateProject(repo *project.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in project.NewInput
		if len(params) > 0 {
			if err := json.Unmarshal(params, &in); err != nil {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
			}
		}
		p, err := repo.Create(ctx, now(), in)
		if err != nil {
			if errors.Is(err, project.ErrInvalidInput) ||
				errors.Is(err, project.ErrInvalidLengthTarget) ||
				errors.Is(err, project.ErrInvalidDefaultPOV) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(p)
	}
}

// listParams mirrors project.ListFilter on the wire.
type listParams struct {
	IncludeArchived bool `json:"include_archived"`
	Limit           int  `json:"limit"`
}

// ListProjects returns a handler for projects.list.
func ListProjects(repo *project.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p listParams
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p) // tolerate empty / partial
		}
		list, err := repo.List(ctx, project.ListFilter{IncludeArchived: p.IncludeArchived, Limit: p.Limit})
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		// Always emit an array, never null.
		if list == nil {
			list = []project.Project{}
		}
		return json.Marshal(list)
	}
}

// idParam is the shared shape for handlers that take a single project id.
type idParam struct {
	ID string `json:"id"`
}

// ProjectDataCleaner removes non-SQLite project data such as companion files.
type ProjectDataCleaner func(context.Context, string) error

// GetProject returns a handler for projects.get.
func GetProject(repo *project.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		got, err := repo.Get(ctx, p.ID)
		if errors.Is(err, project.ErrNotFound) {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project not found"}
		}
		if errors.Is(err, project.ErrInvalidInput) {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(got)
	}
}

// UpdateProject returns a handler for projects.update.
func UpdateProject(repo *project.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in project.UpdateInput
		if err := json.Unmarshal(params, &in); err != nil || in.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		p, err := repo.Update(ctx, now(), in)
		if errors.Is(err, project.ErrNotFound) {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project not found"}
		}
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(p)
	}
}

// RewriteProjectSynopsis derives a fresh synopsis from the current root
// container summaries and stores it on the project as the editable synopsis.
func RewriteProjectSynopsis(repo *project.Repo, builder *ai.ContextBuilder, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		synopsis, err := builder.DeriveProjectSynopsis(ctx, p.ID, true)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		got, err := repo.Update(ctx, now(), project.UpdateInput{ID: p.ID, Synopsis: &synopsis})
		if errors.Is(err, project.ErrNotFound) {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project not found"}
		}
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(got)
	}
}

// ClearProjectSynopsis removes the editable synopsis while preserving outline.
func ClearProjectSynopsis(repo *project.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		empty := ""
		got, err := repo.Update(ctx, now(), project.UpdateInput{ID: p.ID, Synopsis: &empty})
		if errors.Is(err, project.ErrNotFound) {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project not found"}
		}
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(got)
	}
}

// ArchiveProject returns a handler for projects.archive.
func ArchiveProject(repo *project.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Archive(ctx, p.ID, now()); err != nil {
			if errors.Is(err, project.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

// RestoreProject returns a handler for projects.restore.
func RestoreProject(repo *project.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Restore(ctx, p.ID, now()); err != nil {
			if errors.Is(err, project.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

// DeleteProject returns a handler for projects.delete.
func DeleteProject(repo *project.Repo, cleaners ...ProjectDataCleaner) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if _, err := repo.Get(ctx, p.ID); err != nil {
			if errors.Is(err, project.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		for _, clean := range cleaners {
			if clean == nil {
				continue
			}
			if err := clean(ctx, p.ID); err != nil {
				return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
			}
		}
		if err := repo.Delete(ctx, p.ID); err != nil {
			if errors.Is(err, project.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
