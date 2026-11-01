//go:build !mas

package engineapp

import (
	"context"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/foldersync"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
)

// The engine can reach the writer's folder directly, so it can be read as well
// as written, and can be the two-way target.
const folderPrimaryAllowed = true

func setupFolderSync(deps syncDeps) (dailySyncer, targetHooks) {
	syncer := foldersync.New(deps.settingsStore, deps.projects, deps.nodes, deps.entities, deps.relationships)
	syncer.Extras = deps.extras
	syncer.Illustrations = deps.images
	syncer.Ops = deps.ops
	deps.server.Handle("folder_sync.run", handlers.RunFolderSync(syncer))
	return realFolderSyncer{syncer}, targetHooks{
		after: func(ctx context.Context) error {
			res, err := syncer.RunOnce(ctx)
			if err != nil {
				return err
			}
			if res.Error != "" {
				return errors.New(res.Error)
			}
			return nil
		},
	}
}

// folderStoreDir is where the folder target's sealed copy lives.
func folderStoreDir(_ string, configured string) string { return configured }

type realFolderSyncer struct{ s *foldersync.Syncer }

func (r realFolderSyncer) RunOnce(ctx context.Context) (syncResult, error) {
	res, err := r.s.RunOnce(ctx)
	return syncResult{Error: res.Error}, err
}
