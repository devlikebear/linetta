//go:build !mobile

package engineapp

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/devlikebear/linetta/engine/internal/backup"
	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/publish"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/syncauto"
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
	ops     *opsstatus.Repo
	// agentBusy says a turn is writing to a work.
	agentBusy func(projectID string) bool
}

// vaultSetup is what the rest of the app needs back: deleting a work has to tell
// the vault first, and the engine has to stop the runner on the way out.
type vaultSetup struct {
	queueDeletion handlers.ProjectDataCleaner
	stop          func() error
}

func setupVault(ctx context.Context, deps vaultDeps) vaultSetup {
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
	s.Handle("vault.items_get", handlers.VaultItemsGet(svc))
	s.Handle("vault.items_set", handlers.VaultItemsSet(svc))

	runner := &syncauto.Runner{
		Vault:   svc,
		Enabled: deps.settingsStore.SyncAutoEnabled,
		Busy:    deps.agentBusy,
		Now:     time.Now,
		Sleep: func(ctx context.Context, d time.Duration) error {
			backup.Wait(ctx, d)
			return ctx.Err()
		},
		Notify: func(status syncauto.Status) { _ = notifier.Notify("sync.status", status) },
		Record: func(started, finished time.Time, ok bool, reason string, last int64) {
			if err := deps.ops.Record(context.Background(), opsstatus.JobAutoSync, started.UnixMilli(), finished.UnixMilli(), ok, reason, autoSyncRecord{LastSuccessAt: last}); err != nil {
				log.Printf("sync: record: %v", err)
			}
		},
		LastSuccess: func() int64 { return lastAutoSync(deps.ops) },
	}
	control := syncControl{runner: runner, svc: svc, settings: deps.settingsStore}
	s.Handle("sync.status", handlers.SyncStatus(control))
	s.Handle("sync.preview", handlers.SyncPreview(control))
	s.Handle("sync.run_now", handlers.SyncRunNow(control))
	s.Handle("sync.set_auto", handlers.SyncSetAuto(control))
	s.Handle("sync.touch", handlers.SyncTouch(control))
	s.Handle("sync.set_active", handlers.SyncSetActive(control))

	stop := runner.Start(ctx)
	return vaultSetup{
		queueDeletion: svc.QueueDeletion,
		stop: func() error {
			stop()
			return nil
		},
	}
}

// autoSyncRecord is the job's metadata. ops_status keeps only the latest run, and
// "last synced" has to survive a run that failed.
type autoSyncRecord struct {
	LastSuccessAt int64 `json:"last_success_at"`
}

func lastAutoSync(ops *opsstatus.Repo) int64 {
	statuses, err := ops.Get(context.Background())
	if err != nil {
		return 0
	}
	for _, status := range statuses {
		if status.JobName == opsstatus.JobAutoSync {
			var record autoSyncRecord
			if err := json.Unmarshal([]byte(status.MetadataJSON), &record); err == nil {
				return record.LastSuccessAt
			}
		}
	}
	return 0
}

// syncControl adapts the runner for the handlers, which build on mobile too and
// so cannot name its types.
type syncControl struct {
	runner   *syncauto.Runner
	svc      *vault.Service
	settings *settings.Store
}

func (c syncControl) Status() any { return c.runner.Status() }

func (c syncControl) Preview(ctx context.Context) (any, error) { return c.svc.Preview(ctx) }

func (c syncControl) RunNow() { c.runner.RunNow() }

func (c syncControl) Flush(projectID string) { c.runner.Flush(projectID) }

func (c syncControl) Touch(projectID string) { c.runner.Touch(projectID) }

func (c syncControl) SetAuto(ctx context.Context, enabled bool) (any, error) {
	if _, err := c.settings.Set(ctx, settings.Patch{SyncAutoEnabled: &enabled}); err != nil {
		return nil, err
	}
	if enabled {
		c.runner.RunNow()
	}
	return c.runner.Status(), nil
}

func (c syncControl) SetActive(ctx context.Context, projectID string, active bool) (bool, error) {
	if active {
		return c.runner.Open(ctx, projectID)
	}
	c.runner.Close(projectID)
	return false, nil
}
