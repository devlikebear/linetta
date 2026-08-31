package engineapp

import (
	"context"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/export"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

type syncResult struct {
	Error string
}

type dailySyncer interface {
	RunOnce(ctx context.Context) (syncResult, error)
}

type syncDeps struct {
	server        *rpc.Server
	settingsStore *settings.Store
	projects      *project.Repo
	nodes         *node.Repo
	entities      *entity.Repo
	relationships *relationship.Repo
	extras        export.Extras
	ops           *opsstatus.Repo
}

type noopSyncer struct{}

func (noopSyncer) RunOnce(context.Context) (syncResult, error) {
	return syncResult{}, nil
}
