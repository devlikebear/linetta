//go:build mas

package engineapp

import (
	"path/filepath"

	"github.com/devlikebear/linetta/engine/internal/foldersync"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
)

// The sandbox lets the engine write only its own container, and Tauri copies out
// through a bookmark. Nothing can be read back from the writer's folder, so here
// it can only be a mirror.
const folderPrimaryAllowed = false

// folderStoreDir keeps the sealed copy in the container for Tauri to copy out.
func folderStoreDir(home, _ string) string {
	return filepath.Join(home, foldersync.MirrorDirName)
}

func setupFolderSync(deps syncDeps) (dailySyncer, targetHooks) {
	syncer := foldersync.New(deps.settingsStore, deps.projects, deps.nodes, deps.entities, deps.relationships)
	syncer.Extras = deps.extras
	syncer.Illustrations = deps.images
	syncer.Ops = deps.ops
	deps.server.Handle("folder_sync.stage", handlers.StageFolderSync(syncer))
	deps.server.Handle("folder_sync.report", handlers.ReportFolderSync(syncer))
	return noopSyncer{}, targetHooks{}
}
