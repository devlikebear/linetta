//go:build mas || mobile

package engineapp

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

const gitSyncAvailable = false

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
			Message: "git sync is not available in this build",
		}
	}
	s.Handle("git_sync.run", unavailable)
	s.Handle("git_sync.init", unavailable)
	return noopGitSyncer{}
}

type noopGitSyncer struct{}

func (noopGitSyncer) RunOnce(context.Context) (syncResult, error) {
	return syncResult{}, nil
}
