//go:build !mas

package engineapp

import (
	"context"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/foldersync"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

func setupFolderSync(
	s *rpc.Server,
	settingsStore *settings.Store,
	projects *project.Repo,
	nodes *node.Repo,
	entities *entity.Repo,
	relationships *relationship.Repo,
	ops *opsstatus.Repo,
) dailySyncer {
	syncer := foldersync.New(settingsStore, projects, nodes, entities, relationships)
	syncer.Ops = ops
	s.Handle("folder_sync.run", handlers.RunFolderSync(syncer))
	return realFolderSyncer{syncer}
}

type realFolderSyncer struct{ s *foldersync.Syncer }

func (r realFolderSyncer) RunOnce(ctx context.Context) (syncResult, error) {
	res, err := r.s.RunOnce(ctx)
	return syncResult{Error: res.Error}, err
}
