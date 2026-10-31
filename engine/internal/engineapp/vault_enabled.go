//go:build !mobile

package engineapp

import (
	"context"

	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/publish"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/vault"
)

// The private vault follows publishing: on in every desktop build, silent until
// the writer connects an account and the operator approves it.
const vaultAvailable = true

type vaultDeps struct {
	server        *rpc.Server
	settingsStore *settings.Store
	store         *store.Store
	home          string
	indexer       *manuscript.Indexer
	clock         func() int64
	// publish reconciles the site's state of a pulled work. Pulled works keep
	// their ids, so they are still the same works there.
	publish *publish.Service
}

func setupVault(deps vaultDeps) {
	notifier := deps.server.Notifier()
	svc := vault.NewService(vault.Deps{
		Client:   vault.NewClient(publish.BaseURLFromEnv(), DefaultVersion),
		Settings: deps.settingsStore,
		Store:    deps.store,
		Home:     deps.home,
		Index:    deps.indexer,
		Now:      deps.clock,
		Notify:   func(p vault.Progress) { _ = notifier.Notify("vault.progress", p) },
		AfterPull: func(ctx context.Context, projectID string) error {
			_, err := deps.publish.WorkStatus(ctx, projectID)
			return err
		},
	})
	s := deps.server
	s.Handle("vault.status", handlers.VaultStatus(svc))
	s.Handle("vault.work_status", handlers.VaultWorkStatus(svc))
	s.Handle("vault.setup", handlers.VaultSetup(svc))
	s.Handle("vault.push", handlers.VaultPush(svc))
	s.Handle("vault.pull", handlers.VaultPull(svc))
	s.Handle("vault.resolve_conflict", handlers.VaultResolveConflict(svc))
	s.Handle("vault.delete_item", handlers.VaultDeleteItem(svc))
}
