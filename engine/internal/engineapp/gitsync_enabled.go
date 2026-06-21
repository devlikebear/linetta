//go:build !mas && !mobile

package engineapp

import (
	"context"

	"github.com/devlikebear/linetta/engine/internal/gitsync"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
)

const gitSyncAvailable = true

func setupGitSync(deps syncDeps) dailySyncer {
	syncer := gitsync.New(deps.settingsStore, deps.projects, deps.nodes, deps.entities, deps.relationships)
	syncer.Ops = deps.ops
	deps.server.Handle("git_sync.run", handlers.RunGitSync(syncer))
	deps.server.Handle("git_sync.init", handlers.InitGitSync(syncer))
	return realGitSyncer{syncer}
}

type realGitSyncer struct{ s *gitsync.Syncer }

func (r realGitSyncer) RunOnce(ctx context.Context) (syncResult, error) {
	res, err := r.s.RunOnce(ctx)
	return syncResult{Error: res.Error}, err
}
