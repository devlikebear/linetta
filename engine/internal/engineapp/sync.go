package engineapp

import (
	"context"
	"errors"

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

// targetHooks is what a folder or repository does around a sync run. A nil hook
// has nothing to do.
type targetHooks struct {
	before func(ctx context.Context) error
	after  func(ctx context.Context) error
}

type syncDeps struct {
	server        *rpc.Server
	settingsStore *settings.Store
	projects      *project.Repo
	nodes         *node.Repo
	entities      *entity.Repo
	relationships *relationship.Repo
	extras        export.Extras
	images        export.IllustrationSource
	ops           *opsstatus.Repo
}

func runExport(ctx context.Context, syncer dailySyncer) error {
	res, err := syncer.RunOnce(ctx)
	if err != nil {
		return err
	}
	if res.Error != "" {
		return errors.New(res.Error)
	}
	return nil
}

type noopSyncer struct{}

func (noopSyncer) RunOnce(context.Context) (syncResult, error) {
	return syncResult{}, nil
}
