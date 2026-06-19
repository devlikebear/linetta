//go:build mas

package main

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
	// MAS: Tauri owns the privileged copy + the daily timer.
	s.Handle("folder_sync.stage", handlers.StageFolderSync(syncer))
	s.Handle("folder_sync.report", handlers.ReportFolderSync(syncer))
	return noopFolderSyncer{}
}

type noopFolderSyncer struct{}

func (noopFolderSyncer) RunOnce(context.Context) (syncResult, error) {
	return syncResult{}, nil
}
