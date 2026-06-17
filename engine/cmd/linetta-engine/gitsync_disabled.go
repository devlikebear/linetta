//go:build mas

package main

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

// setupGitSync registers git_sync handlers that report the feature is
// unavailable and returns a no-op syncer. The gitsync package (which shells out
// to git) is never compiled into the mas build.
func setupGitSync(
	s *rpc.Server,
	_ *settings.Store,
	_ *project.Repo,
	_ *node.Repo,
	_ *entity.Repo,
	_ *relationship.Repo,
	_ *opsstatus.Repo,
) dailySyncer {
	unavailable := func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return nil, &rpc.MethodError{
			Code:    rpc.CodeMethodNotFound,
			Message: "git sync is not available in the App Store build",
		}
	}
	s.Handle("git_sync.run", unavailable)
	s.Handle("git_sync.init", unavailable)
	return noopSyncer{}
}

type noopSyncer struct{}

func (noopSyncer) RunOnce(context.Context) (syncResult, error) { return syncResult{}, nil }
