//go:build !mas && !mobile

package engineapp

import (
	"context"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/gitsync"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
	"github.com/devlikebear/linetta/engine/internal/vault"
)

const gitSyncAvailable = true

func setupGitSync(deps syncDeps) (dailySyncer, targetHooks) {
	syncer := gitsync.New(deps.settingsStore, deps.projects, deps.nodes, deps.entities, deps.relationships)
	syncer.Extras = deps.extras
	syncer.Illustrations = deps.images
	syncer.Ops = deps.ops
	deps.server.Handle("git_sync.run", handlers.RunGitSync(syncer))
	deps.server.Handle("git_sync.init", handlers.InitGitSync(syncer))
	return realGitSyncer{syncer}, targetHooks{
		before: func(ctx context.Context) error {
			err := syncer.Prepare(ctx)
			if errors.Is(err, gitsync.ErrNotSetUp) {
				return &rpc.ReasonError{Reason: rpc.ReasonSyncTargetMissing, Err: err}
			}
			if err != nil {
				return &rpc.ReasonError{Reason: rpc.ReasonSyncTargetFailed, Err: err}
			}
			return nil
		},
		after: func(ctx context.Context) error {
			summary, refused, err := syncer.Publish(ctx)
			switch {
			case err != nil:
				return &rpc.ReasonError{Reason: rpc.ReasonSyncTargetFailed, Err: err}
			case refused:
				return vault.ErrRoundAgain
			case summary.Error != "":
				return &rpc.ReasonError{Reason: rpc.ReasonSyncTargetFailed, Err: errors.New(summary.Error)}
			}
			return nil
		},
	}
}

type realGitSyncer struct{ s *gitsync.Syncer }

func (r realGitSyncer) RunOnce(ctx context.Context) (syncResult, error) {
	res, err := r.s.RunOnce(ctx)
	return syncResult{Error: res.Error}, err
}
