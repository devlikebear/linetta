package handlers

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/paths"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type diagnosticsPayload struct {
	Version          string `json:"version"`
	Home             string `json:"home"`
	DBPath           string `json:"db_path"`
	MigrationVersion int    `json:"migration_version"`
	MigrationCount   int    `json:"migration_count"`
}

// DiagnosticsVersion returns side-effect-free runtime metadata for the shell
// startup gate and support screens.
func DiagnosticsVersion(st *store.Store, version string) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		home, err := paths.Home()
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		dbPath, err := paths.DBPath()
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}

		var latest, count sql.NullInt64
		if err := st.DB().QueryRowContext(ctx,
			`SELECT MAX(version), COUNT(*) FROM schema_migrations`).Scan(&latest, &count); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		payload := diagnosticsPayload{
			Version:          version,
			Home:             home,
			DBPath:           dbPath,
			MigrationVersion: int(latest.Int64),
			MigrationCount:   int(count.Int64),
		}
		return json.Marshal(payload)
	}
}
