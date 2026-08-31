package handlers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/devlikebear/linetta/engine/internal/backup"
	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/restore"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/store"
)

// CreateRecoveryBackup creates a versioned full-library snapshot that the
// native startup recovery screen can restore.
func CreateRecoveryBackup(st *store.Store, home string, now Clock) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		result, err := backup.RunManualRecovery(ctx, st.DB(), home, time.UnixMilli(now()))
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(result)
	}
}

// ListBackups returns a handler for backup.list: every restorable snapshot
// under the backups folder, newest first.
func ListBackups(home string) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		entries, err := restore.ListBackups(home)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(map[string]any{"backups": entries})
	}
}

type backupPeekParams struct {
	Path string `json:"path"`
}

// PeekBackup returns a handler for backup.peek: the works inside one backup,
// so the UI can offer per-work restore.
func PeekBackup(home string) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p backupPeekParams
		if err := json.Unmarshal(params, &p); err != nil || p.Path == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "path required"}
		}
		if err := restore.ValidateBackupPath(home, p.Path); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		projects, err := restore.PeekProjects(ctx, p.Path, "")
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(map[string]any{"projects": projects})
	}
}

type backupRestoreParams struct {
	Path        string `json:"path"`
	ProjectID   string `json:"project_id"`
	TitleSuffix string `json:"title_suffix"`
}

// RestoreProjectFromBackup returns a handler for backup.restore_project.
//
// The merge is purely additive: the chosen work comes back as a NEW project
// and nothing already in the library is written to. A safety snapshot of the
// live library is still taken first, so even this path is undoable.
func RestoreProjectFromBackup(st *store.Store, home string, indexer *manuscript.Indexer, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p backupRestoreParams
		if err := json.Unmarshal(params, &p); err != nil || p.Path == "" || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "path and project_id required"}
		}
		if err := restore.ValidateBackupPath(home, p.Path); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		ts := time.UnixMilli(now())
		if _, err := backup.RunManualRecovery(ctx, st.DB(), home, ts); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: "pre-restore backup: " + err.Error()}
		}
		result, err := restore.MergeProject(ctx, st, p.Path, "", p.ProjectID, p.TitleSuffix, ts)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if indexer != nil {
			// Search index rebuild is best-effort; the searcher also lazily
			// rebuilds an empty project index on first query.
			_ = indexer.Rebuild(ctx, result.ProjectID)
		}
		return json.Marshal(result)
	}
}
