//go:build !mas

package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/gitsync"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// RunGitSync returns a handler for git_sync.run. The handler returns the
// structured ResultSummary as JSON. Hard RPC errors are reserved for
// catastrophic failures (e.g. cannot list projects); ordinary git errors
// (push rejected, no remote, etc.) are surfaced via summary.Error so the
// frontend can render a toast.
func RunGitSync(s *gitsync.Syncer) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		res, err := s.RunOnce(ctx)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(res)
	}
}

// InitGitSync returns a handler for git_sync.init. Creates GitSyncDir if
// missing and runs `git init` there if not already a repo. Safe to call
// repeatedly. Returns InitResult so the UI can show a differentiated toast.
func InitGitSync(s *gitsync.Syncer) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		res, err := s.Init(ctx)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(res)
	}
}
