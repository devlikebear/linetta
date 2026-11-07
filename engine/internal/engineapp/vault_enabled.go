//go:build !mas

package engineapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/linetta/engine/internal/atomicfile"
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

// The private vault follows publishing: on in every build but the Mac App
// Store's, iOS included, and silent until the writer connects an account and the
// operator approves it.
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
	// The daily exports and the round each target runs around a sync.
	gitSyncer, folderSyncer dailySyncer
	gitHooks, folderHooks   targetHooks
}

// vaultSetup is what the rest of the app needs back: deleting a work has to tell
// the vault first, and the engine has to stop the runner on the way out.
type vaultSetup struct {
	queueDeletion handlers.ProjectDataCleaner
	stop          func() error
}

// The runner keeps the daily Markdown exports, alongside the targets they share
// a folder with.
const syncRunnerTakesExports = true

func setupVault(ctx context.Context, deps vaultDeps) vaultSetup {
	notifier := deps.server.Notifier()
	newService := func(target string, open func() (vault.Store, error), progress func(vault.Progress)) *vault.Service {
		return vault.NewService(vault.Deps{
			Client:    vault.NewClient(publish.BaseURLFromEnv(), DefaultVersion),
			OpenStore: open,
			Target:    target,
			Settings:  deps.settingsStore,
			Store:     deps.store,
			Home:      deps.home,
			Index:     deps.indexer,
			Now:       deps.clock,
			Notify:    progress,
			AfterPull: func(ctx context.Context, projectID string) error {
				_, err := deps.publish.WorkStatus(ctx, projectID)
				return err
			},
		})
	}
	svc := newService(vault.TargetWorks, nil, func(p vault.Progress) { _ = notifier.Notify("vault.progress", p) })
	device := deviceID(deps.home)
	directory := func(folder string, hooks targetHooks) (vault.Store, error) {
		return vault.NewDirStore(vault.DirConfig{Folder: folder, Device: device, Now: time.Now, Before: hooks.before, After: hooks.after}), nil
	}
	services := map[string]*vault.Service{
		vault.TargetWorks: svc,
	}
	// iOS has no folder of the writer's to keep a copy in, only the app's own
	// container, which the daily backup already covers.
	if foldersAvailable {
		services[vault.TargetFolder] = newService(vault.TargetFolder, func() (vault.Store, error) {
			cfg, err := deps.settingsStore.Get(context.Background())
			if err != nil {
				return nil, err
			}
			folder := folderStoreDir(deps.home, cfg.FolderSyncDir)
			if !folderPrimaryAllowed {
				if err := os.MkdirAll(folder, 0o700); err != nil {
					return nil, err
				}
			}
			return directory(folder, deps.folderHooks)
		}, nil)
	}
	if gitSyncAvailable {
		services[vault.TargetGit] = newService(vault.TargetGit, func() (vault.Store, error) {
			cfg, err := deps.settingsStore.Get(context.Background())
			if err != nil {
				return nil, err
			}
			return directory(cfg.GitSyncDir, deps.gitHooks)
		}, nil)
	}
	routed := routedVault{Service: svc, services: services, settings: deps.settingsStore}
	s := deps.server
	s.Handle("vault.status", handlers.VaultStatus(routed))
	s.Handle("vault.work_status", handlers.VaultWorkStatus(routed))
	s.Handle("vault.setup", handlers.VaultSetup(routed))
	s.Handle("vault.push", handlers.VaultPush(routed))
	s.Handle("vault.pull", handlers.VaultPull(routed))
	s.Handle("vault.resolve_conflict", handlers.VaultResolveConflict(routed))
	s.Handle("vault.delete_item", handlers.VaultDeleteItem(routed))
	s.Handle("vault.items_get", handlers.VaultItemsGet(routed))
	s.Handle("vault.items_set", handlers.VaultItemsSet(routed))

	targets := func() []syncauto.Target {
		primary, mirrors := deps.settingsStore.SyncTargets()
		var out []syncauto.Target
		if service, ok := services[primary]; ok {
			out = append(out, syncauto.Target{ID: primary, Vault: service, Primary: true})
		}
		for _, id := range mirrors {
			if service, ok := services[id]; ok && id != primary {
				out = append(out, syncauto.Target{ID: id, Vault: service})
			}
		}
		return out
	}
	exports := []syncauto.Export{
		{ID: vault.TargetGit, Run: func(ctx context.Context) error { return runExport(ctx, deps.gitSyncer) }},
		{ID: vault.TargetFolder, Run: func(ctx context.Context) error { return runExport(ctx, deps.folderSyncer) }},
	}
	runner := &syncauto.Runner{
		Targets: targets,
		Exports: func() []syncauto.Export { return exports },
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
	control := syncControl{runner: runner, services: services, settings: deps.settingsStore}
	s.Handle("sync.status", handlers.SyncStatus(control))
	s.Handle("sync.preview", handlers.SyncPreview(control))
	s.Handle("sync.run_now", handlers.SyncRunNow(control))
	s.Handle("sync.set_auto", handlers.SyncSetAuto(control))
	s.Handle("sync.touch", handlers.SyncTouch(control))
	s.Handle("sync.set_active", handlers.SyncSetActive(control))
	s.Handle("sync.targets", handlers.SyncTargets(control))
	s.Handle("sync.set_targets", handlers.SyncSetTargets(control))
	s.Handle("sync.restore", handlers.SyncRestore(control))

	stop := runner.Start(ctx)
	return vaultSetup{
		queueDeletion: svc.QueueDeletion,
		stop: func() error {
			stop()
			return nil
		},
	}
}

// routedVault is the Linetta Works service, except that a recovery key can be
// made against any target: GitHub and a folder need no account, only the key.
type routedVault struct {
	*vault.Service
	services map[string]*vault.Service
	settings *settings.Store
}

// primary is the target a work's own screens talk to: its state, its conflicts
// and its restore points live on the two-way target, whichever that is.
func (r routedVault) primary() *vault.Service {
	id, _ := r.settings.SyncTargets()
	if service, ok := r.services[id]; ok {
		return service
	}
	return r.Service
}

func (r routedVault) pick(target string) *vault.Service {
	if service, ok := r.services[target]; ok {
		return service
	}
	return r.primary()
}

func (r routedVault) Setup(ctx context.Context, in vault.SetupInput) (vault.SetupResult, error) {
	if service, ok := r.services[in.Target]; ok {
		return service.Setup(ctx, in)
	}
	return r.Service.Setup(ctx, in)
}

func (r routedVault) StatusOf(ctx context.Context, target string) (vault.Overview, error) {
	return r.pick(target).Status(ctx)
}

func (r routedVault) WorkStatus(ctx context.Context, projectID string) (vault.WorkStatus, error) {
	return r.primary().WorkStatus(ctx, projectID)
}

func (r routedVault) Push(ctx context.Context, projectID string) (vault.PushResult, error) {
	return r.primary().Push(ctx, projectID)
}

func (r routedVault) Pull(ctx context.Context, in vault.PullInput) (vault.PullResult, error) {
	return r.pick(in.Target).Pull(ctx, in)
}

func (r routedVault) ResolveConflict(ctx context.Context, in vault.ResolveInput) (vault.ResolveResult, error) {
	return r.primary().ResolveConflict(ctx, in)
}

// deviceID names this device in revision files. It stays on the device, and says
// nothing beyond telling two devices apart.
func deviceID(home string) string {
	file := filepath.Join(home, "sync-device-id")
	if data, err := os.ReadFile(file); err == nil {
		if id := strings.TrimSpace(string(data)); id != "" {
			return id
		}
	}
	raw := make([]byte, 4)
	_, _ = rand.Read(raw)
	id := hex.EncodeToString(raw)
	if err := atomicfile.Write(file, []byte(id), 0o600); err != nil {
		log.Printf("sync: keep device id: %v", err)
	}
	return id
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
	services map[string]*vault.Service
	settings *settings.Store
}

func (c syncControl) Status() any { return c.runner.Status() }

func (c syncControl) Preview(ctx context.Context) (any, error) {
	return c.services[vault.TargetWorks].Preview(ctx)
}

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

// targetView is one row of the sync screen.
type targetView struct {
	ID            string `json:"id"`
	Configured    bool   `json:"configured"`
	CanPrimary    bool   `json:"can_primary"`
	Bytes         int64  `json:"bytes"`
	LastSuccessAt int64  `json:"last_success_at"`
	Reason        string `json:"reason,omitempty"`
}

type targetsView struct {
	Primary   string       `json:"primary"`
	Mirrors   []string     `json:"mirrors"`
	KeyStored bool         `json:"key_stored"`
	Targets   []targetView `json:"targets"`
}

// Targets reads what the screen needs without a network call: Linetta Works
// reports its own usage in its section, and a folder is on this disk.
func (c syncControl) Targets(ctx context.Context) (any, error) {
	cfg, err := c.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	primary, mirrors := c.settings.SyncTargets()
	if mirrors == nil {
		mirrors = []string{}
	}
	keyStored, err := c.settings.VaultKeyExists()
	if err != nil {
		return nil, &rpc.ReasonError{Reason: rpc.ReasonVaultKeyUnavailable, Err: err}
	}
	reports := c.runner.Mirrors()
	out := targetsView{Primary: primary, Mirrors: mirrors, KeyStored: keyStored, Targets: []targetView{}}
	for _, id := range []string{vault.TargetWorks, vault.TargetGit, vault.TargetFolder} {
		service, ok := c.services[id]
		if !ok {
			continue
		}
		view := targetView{ID: id, CanPrimary: id != vault.TargetFolder || folderPrimaryAllowed}
		switch id {
		case vault.TargetWorks:
			view.Configured = c.settings.PublishToken() != ""
		case vault.TargetGit:
			view.Configured = strings.TrimSpace(cfg.GitSyncDir) != ""
		case vault.TargetFolder:
			view.Configured = strings.TrimSpace(cfg.FolderSyncDir) != ""
		}
		if view.Configured && id != vault.TargetWorks {
			view.Bytes = service.TargetBytes(ctx)
		}
		report := reports[id]
		view.LastSuccessAt, view.Reason = report.LastSuccessAt, report.Reason
		out.Targets = append(out.Targets, view)
	}
	return out, nil
}

// SetTargets changes where the library is kept. A new two-way target takes
// everything first, and only then replaces the old one, whose data stays put:
// the writer never has a moment with no complete copy.
func (c syncControl) SetTargets(ctx context.Context, primary string, mirrors []string) (any, error) {
	next, ok := c.services[primary]
	if !ok {
		return nil, fmt.Errorf("%w: %q", handlers.ErrBadTarget, primary)
	}
	if primary == vault.TargetFolder && !folderPrimaryAllowed {
		return nil, fmt.Errorf("%w: this build can only mirror to a folder", handlers.ErrBadTarget)
	}
	seen := map[string]bool{primary: true}
	kept := []string{}
	for _, id := range mirrors {
		if _, ok := c.services[id]; !ok {
			return nil, fmt.Errorf("%w: %q", handlers.ErrBadTarget, id)
		}
		if !seen[id] {
			seen[id] = true
			kept = append(kept, id)
		}
	}
	if current, _ := c.settings.SyncTargets(); current != primary {
		if _, err := next.SyncAll(ctx, vault.SyncPolicy{}); err != nil {
			return nil, err
		}
	}
	patch := settings.Patch{SyncPrimaryTarget: &primary, SyncMirrors: &kept}
	if seen[vault.TargetFolder] {
		on := true
		patch.FolderSyncEnabled = &on
	}
	if _, err := c.settings.Set(ctx, patch); err != nil {
		return nil, err
	}
	c.runner.RunNow()
	return c.Targets(ctx)
}

// Restore takes everything a target holds that this library does not have.
func (c syncControl) Restore(ctx context.Context, target string) (any, error) {
	service, ok := c.services[target]
	if !ok {
		return nil, fmt.Errorf("%w: %q", handlers.ErrBadTarget, target)
	}
	return service.SyncAll(ctx, vault.SyncPolicy{PullOnly: true})
}
