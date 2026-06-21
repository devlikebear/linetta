//go:build mas

package engineapp

import (
	"github.com/devlikebear/linetta/engine/internal/foldersync"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
)

func setupFolderSync(deps syncDeps) dailySyncer {
	syncer := foldersync.New(deps.settingsStore, deps.projects, deps.nodes, deps.entities, deps.relationships)
	syncer.Ops = deps.ops
	deps.server.Handle("folder_sync.stage", handlers.StageFolderSync(syncer))
	deps.server.Handle("folder_sync.report", handlers.ReportFolderSync(syncer))
	return noopSyncer{}
}
