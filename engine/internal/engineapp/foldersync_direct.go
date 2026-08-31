//go:build !mas

package engineapp

import (
	"context"

	"github.com/devlikebear/linetta/engine/internal/foldersync"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
)

func setupFolderSync(deps syncDeps) dailySyncer {
	syncer := foldersync.New(deps.settingsStore, deps.projects, deps.nodes, deps.entities, deps.relationships)
	syncer.Extras = deps.extras
	syncer.Ops = deps.ops
	deps.server.Handle("folder_sync.run", handlers.RunFolderSync(syncer))
	return realFolderSyncer{syncer}
}

type realFolderSyncer struct{ s *foldersync.Syncer }

func (r realFolderSyncer) RunOnce(ctx context.Context) (syncResult, error) {
	res, err := r.s.RunOnce(ctx)
	return syncResult{Error: res.Error}, err
}
