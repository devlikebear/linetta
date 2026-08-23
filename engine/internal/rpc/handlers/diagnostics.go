package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"

	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/paths"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/store"
)

// Capabilities describes build-variant feature availability surfaced to the UI.
type Capabilities struct {
	UnavailableProviders []string
	GitSyncAvailable     bool
	MCPAvailable         bool
}

type diagnosticsPayload struct {
	Version              string   `json:"version"`
	Home                 string   `json:"home"`
	DBPath               string   `json:"db_path"`
	MigrationVersion     int      `json:"migration_version"`
	MigrationCount       int      `json:"migration_count"`
	UnavailableProviders []string `json:"unavailable_providers,omitempty"`
	GitSyncAvailable     bool     `json:"git_sync_available"`
	MCPAvailable         bool     `json:"mcp_available"`
}

type diagnosticsGetPayload struct {
	diagnosticsPayload
	OpsStatus []opsstatus.Status `json:"ops_status"`
}

// DiagnosticsVersion returns side-effect-free runtime metadata for the shell
// startup gate and support screens.
func DiagnosticsVersion(st *store.Store, home string, version string, caps Capabilities) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		resolvedHome, err := diagnosticsHome(home)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		dbPath := filepath.Join(resolvedHome, "library.db")

		var latest, count sql.NullInt64
		if err := st.DB().QueryRowContext(ctx,
			`SELECT MAX(version), COUNT(*) FROM schema_migrations`).Scan(&latest, &count); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		payload := diagnosticsPayload{
			Version:              version,
			Home:                 resolvedHome,
			DBPath:               dbPath,
			MigrationVersion:     int(latest.Int64),
			MigrationCount:       int(count.Int64),
			UnavailableProviders: caps.UnavailableProviders,
			GitSyncAvailable:     caps.GitSyncAvailable,
			MCPAvailable:         caps.MCPAvailable,
		}
		return json.Marshal(payload)
	}
}

func DiagnosticsGet(st *store.Store, ops *opsstatus.Repo, home string, version string, caps Capabilities) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		raw, err := DiagnosticsVersion(st, home, version, caps)(ctx, params)
		if err != nil {
			return nil, err
		}
		var base diagnosticsPayload
		if err := json.Unmarshal(raw, &base); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		statuses, err := ops.Get(ctx)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(diagnosticsGetPayload{
			diagnosticsPayload: base,
			OpsStatus:          statuses,
		})
	}
}

func diagnosticsHome(home string) (string, error) {
	if home != "" {
		return home, nil
	}
	return paths.Home()
}
