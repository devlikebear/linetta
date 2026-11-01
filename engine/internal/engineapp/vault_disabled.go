//go:build mobile

package engineapp

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/publish"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store"
)

// The vault is not on iOS this phase. As with publishing, the methods refuse
// rather than vanish.
const vaultAvailable = false

type vaultDeps struct {
	server        *rpc.Server
	settingsStore *settings.Store
	store         *store.Store
	home          string
	indexer       *manuscript.Indexer
	clock         func() int64
	publish       *publish.Service
	ops           *opsstatus.Repo
	agentBusy     func(projectID string) bool
}

type vaultSetup struct {
	queueDeletion handlers.ProjectDataCleaner
	stop          func() error
}

func setupVault(_ context.Context, deps vaultDeps) vaultSetup {
	unavailable := func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return nil, &rpc.MethodError{
			Code:    rpc.CodeMethodNotFound,
			Message: "the vault is not available in this build",
		}
	}
	for _, method := range []string{
		"vault.status", "vault.work_status", "vault.setup",
		"vault.push", "vault.pull", "vault.resolve_conflict", "vault.delete_item",
		"vault.items_get", "vault.items_set",
		"sync.status", "sync.preview", "sync.run_now", "sync.set_auto", "sync.touch", "sync.set_active",
	} {
		deps.server.Handle(method, unavailable)
	}
	return vaultSetup{stop: func() error { return nil }}
}
