package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/charcount"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/submission"
	"github.com/devlikebear/linetta/engine/internal/summarizer"
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
				errors.Is(err, project.ErrInvalidDefaultPOV) ||
				errors.Is(err, project.ErrInvalidOutlinePreset) {
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
			return nil, rpc.NotFound(rpc.ReasonProjectNotFound, "project not found")
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

// SubmissionCheck returns a handler for projects.submission_check: the work
// measured against its submission rule (episode-length Phase 3).
func SubmissionCheck(projects *project.Repo, nodes *node.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		proj, err := projects.Get(ctx, p.ID)
		if errors.Is(err, project.ErrNotFound) {
			return nil, rpc.NotFound(rpc.ReasonProjectNotFound, "project not found")
		}
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		all, err := nodes.ListByProject(ctx, proj.ID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(submission.Check(all, proj))
	}
}

// ManuscriptPagesResult is a work's rough length in 200-character manuscript
// sheets (episode-length Phase 4).
type ManuscriptPagesResult struct {
	Lines int `json:"lines"`
	Pages int `json:"pages"`
}

// ManuscriptPages returns a handler for projects.manuscript_pages: every
// scene's manuscript lines added up, then turned into sheets, so scenes run
// on from one another the way a submitted manuscript does.
func ManuscriptPages(projects *project.Repo, nodes *node.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		proj, err := projects.Get(ctx, p.ID)
		if errors.Is(err, project.ErrNotFound) {
			return nil, rpc.NotFound(rpc.ReasonProjectNotFound, "project not found")
		}
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		all, err := nodes.ListByProject(ctx, proj.ID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		lines := 0
		for _, n := range all {
			if n.Kind == node.KindLeaf && n.ContentDoc != nil {
				lines += charcount.ManuscriptLines([]byte(*n.ContentDoc))
			}
		}
		return json.Marshal(ManuscriptPagesResult{Lines: lines, Pages: charcount.ManuscriptPages(lines)})
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
			return nil, rpc.NotFound(rpc.ReasonProjectNotFound, "project not found")
		}
		if errors.Is(err, project.ErrInvalidInput) ||
			errors.Is(err, project.ErrInvalidOutlinePreset) ||
			errors.Is(err, project.ErrInvalidEpisodeCharTarget) ||
			errors.Is(err, project.ErrInvalidEpisodeCharRange) ||
			errors.Is(err, project.ErrInvalidSubmissionRule) ||
			errors.Is(err, project.ErrInvalidCharCountBasis) {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(p)
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
			return nil, rpc.NotFound(rpc.ReasonProjectNotFound, "project not found")
		}
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(got)
	}
}

// RewriteProjectSynopsis asks the writer's provider for a synopsis of the
// work and saves it as the editable synopsis (#135). write does the asking —
// summarizer.WriteSynopsis — and refuses unless the writer switched provider
// summaries on and the provider is consented to.
func RewriteProjectSynopsis(repo *project.Repo, write func(ctx context.Context, projectID string) (string, error), now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if _, err := repo.Get(ctx, p.ID); errors.Is(err, project.ErrNotFound) {
			return nil, rpc.NotFound(rpc.ReasonProjectNotFound, "project not found")
		} else if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		synopsis, err := write(ctx, p.ID)
		switch {
		case errors.Is(err, summarizer.ErrModelOff):
			return nil, rpc.MethodErrorFrom(&rpc.ReasonError{Reason: rpc.ReasonSummarizerOff, Err: err})
		case errors.Is(err, summarizer.ErrNothingToSummarize):
			return nil, rpc.MethodErrorFrom(&rpc.ReasonError{Reason: rpc.ReasonSynopsisNothingToSummarize, Err: err})
		case err != nil:
			return nil, rpc.MethodErrorFrom(err)
		}
		got, err := repo.Update(ctx, now(), project.UpdateInput{ID: p.ID, Synopsis: &synopsis})
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(got)
	}
}

// SummarizerUsage returns what provider summaries have cost so far.
func SummarizerUsage(usage *summarizer.UsageRepo) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		u, err := usage.Get(ctx)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(u)
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
				return nil, rpc.NotFound(rpc.ReasonProjectNotFound, "project not found")
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
				return nil, rpc.NotFound(rpc.ReasonProjectNotFound, "project not found")
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
				return nil, rpc.NotFound(rpc.ReasonProjectNotFound, "project not found")
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
				return nil, rpc.NotFound(rpc.ReasonProjectNotFound, "project not found")
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
