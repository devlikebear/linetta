package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/foldersync"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// RunFolderSync exports directly to the configured folder (non-MAS builds).
func RunFolderSync(s *foldersync.Syncer) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		res, err := s.RunOnce(ctx)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(res)
	}
}

// StageFolderSync exports to a container staging dir (MAS builds).
func StageFolderSync(s *foldersync.Syncer) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		res, err := s.Stage(ctx)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(res)
	}
}

// ReportFolderSync records ops status after Tauri completes the MAS copy.
func ReportFolderSync(s *foldersync.Syncer) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in foldersync.ReportInput
		if len(params) > 0 {
			if err := json.Unmarshal(params, &in); err != nil {
				return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
			}
		}
		if err := s.Report(ctx, in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(map[string]bool{"ok": true})
	}
}
