//go:build !mas && !mobile

package engineapp

import (
	"context"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/gitsync"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

const gitSyncAvailable = true

func setupGitSync(
	s *rpc.Server,
	settingsStore *settings.Store,
	projects *project.Repo,
	nodes *node.Repo,
	entities *entity.Repo,
	relationships *relationship.Repo,
	ops *opsstatus.Repo,
) dailySyncer {
	syncer := gitsync.New(settingsStore, projects, nodes, entities, relationships)
	syncer.Ops = ops
	s.Handle("git_sync.run", handlers.RunGitSync(syncer))
	s.Handle("git_sync.init", handlers.InitGitSync(syncer))
	return realGitSyncer{syncer}
}

type realGitSyncer struct{ s *gitsync.Syncer }

func (r realGitSyncer) RunOnce(ctx context.Context) (syncResult, error) {
	res, err := r.s.RunOnce(ctx)
	return syncResult{Error: res.Error}, err
}
