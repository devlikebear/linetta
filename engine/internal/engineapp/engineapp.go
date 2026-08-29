package engineapp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/devlikebear/linetta/engine/internal/backup"
	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/contextualedit"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/manuscriptedit"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/paths"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
	"github.com/devlikebear/linetta/engine/internal/search"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/stats"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/storycontext"
	"github.com/devlikebear/linetta/engine/internal/storyops"
	"github.com/devlikebear/linetta/engine/internal/summarizer"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

const DefaultVersion = "0.9.6"

// Options configures an embedded engine instance.
type Options struct {
	Home string
}

// App owns the store, RPC server, and background jobs for one engine instance.
type App struct {
	server  *rpc.Server
	cancel  context.CancelFunc
	closers []func() error
	once    sync.Once
}

// Open constructs the full Linetta engine and registers every JSONRPC handler.
func Open(ctx context.Context, opts Options) (*App, error) {
	home := opts.Home
	if home == "" {
		h, err := paths.Home()
		if err != nil {
			return nil, err
		}
		home = h
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, fmt.Errorf("ensure home: %w", err)
	}

	appCtx, cancel := context.WithCancel(ctx)
	st, err := store.Open(appCtx, filepath.Join(home, "library.db"))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open store: %w", err)
	}

	app := &App{
		server:  rpc.NewServer(),
		cancel:  cancel,
		closers: []func() error{st.Close},
	}
	if err := app.register(appCtx, home, st); err != nil {
		_ = app.Close()
		return nil, err
	}
	return app, nil
}

func (a *App) register(ctx context.Context, home string, st *store.Store) error {
	s := a.server
	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	snaps := snapshot.NewRepo(st)
	writingStats := stats.NewRepo(st)
	entities := entity.NewRepo(st)
	mentions := mention.NewRepo(st)
	threads := thread.NewRepo(st)
	beats := beat.NewRepo(st)
	notes := note.NewRepo(st)
	relationships := relationship.NewRepo(st)
	facts := fact.NewRepo(st)
	manuscriptIndexer := manuscript.NewIndexer(st.DB())
	manuscriptSearcher := manuscript.NewSearcher(st.DB(), nodes, manuscriptIndexer)
	manuscriptEditor := manuscriptedit.NewService(nodes, snaps)
	contextualEditor := contextualedit.NewService(entities, facts, relationships, manuscriptEditor, nodes)
	plotBuilder := plot.NewBuilder(nodes, beats, threads)
	ops := opsstatus.NewRepo(st)
	searchRepo := search.NewRepo(st)
	clock := func() int64 { return time.Now().UnixMilli() }

	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mentions.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})
	nodes.SetWritingStatsRecorder(writingStats)
	nodes.SetManuscriptIndexer(manuscriptIndexer)
	if err := manuscriptIndexer.RebuildAll(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "manuscript index rebuild: %v\n", err)
	}
	if err := mentions.RebuildAll(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "mention rebuild: %v\n", err)
	}

	settingsStore, err := settings.NewForHome(home)
	if err != nil {
		return fmt.Errorf("settings: %w", err)
	}

	summ := summarizer.New(nodes).WithOpsStatus(ops, clock)
	stopSummarizer := summ.Start(ctx)
	a.closers = append(a.closers, func() error {
		stopSummarizer()
		return nil
	})

	syncDeps := syncDeps{
		server:        s,
		settingsStore: settingsStore,
		projects:      projects,
		nodes:         nodes,
		entities:      entities,
		relationships: relationships,
		ops:           ops,
	}
	gitSyncer := setupGitSync(syncDeps)
	folderSyncer := setupFolderSync(syncDeps)
	syncers := []dailySyncer{gitSyncer, folderSyncer}
	retentionFn := func(ctx context.Context) error {
		if err := snapshot.Thin(ctx, st.DB(), time.Now().UnixMilli()); err != nil {
			return err
		}
		for _, sy := range syncers {
			res, err := sy.RunOnce(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "daily sync: %v\n", err)
				continue
			}
			if res.Error != "" {
				fmt.Fprintf(os.Stderr, "daily sync: %s\n", res.Error)
			}
		}
		return nil
	}
	stopBackup := backup.Start(ctx, st.DB(), home, retentionFn,
		time.Now, backup.Wait, func(result backup.TickResult) {
			_ = ops.Record(ctx, opsstatus.JobBackup, result.StartedAt, result.FinishedAt,
				result.OK(), result.Error(), result)
		})
	a.closers = append(a.closers, func() error {
		stopBackup()
		return nil
	})

	// Named rather than inlined: the archive export reads the same transcript,
	// and two repos over one table would be a needless second source.
	companionHistory := companion.NewHistoryRepo(st.DB())
	companionSvc := companion.NewService(home).
		WithFacts(facts).
		WithHistory(companionHistory).
		WithReferences(companion.NewReferenceRepo(st.DB()))

	// The MCP host serves story tools to external agents. It binds only when
	// the writer has turned MCP on and accepted its consent.
	//
	// A storyops instance of its own: undo batches live in memory on the
	// service, so an agent can undo only what it applied — never the writer's
	// own companion batch.
	mcpStory := storyops.New(projects, nodes, threads, beats, entities, relationships).
		WithFacts(facts).
		WithSnapshots(snaps).
		WithMemory(companionSvc)

	mcpContextBuilder := storycontext.NewContextBuilder(projects, nodes, mentions, threads, beats, notes, relationships).
		WithSummaryRefresher(summ).
		WithFactSource(companionSvc).
		WithMemorySource(companionSvc).
		WithReferenceSource(companionSvc)

	mcpCtrl, stopMCP := setupMCP(mcpDeps{
		settingsStore: settingsStore,
		home:          home,
		repos: mcpToolRepos{
			projects:   projects,
			nodes:      nodes,
			entities:   entities,
			mentions:   mentions,
			facts:      facts,
			plot:       plotBuilder,
			manuscript: manuscriptSearcher,
			context:    mcpContextBuilder,
			snapshots:  snaps,
			story:      mcpStory,
			msEdit:     manuscriptEditor,
			enqueue:    summ.Enqueue,
			notify:     func(method string, params any) { _ = s.Notifier().Notify(method, params) },
			clock:      clock,
			db:         st.DB(),
		},
	})
	a.closers = append(a.closers, stopMCP)

	caps := handlers.Capabilities{
		GitSyncAvailable: gitSyncAvailable,
		MCPAvailable:     mcpAvailable,
	}
	s.Handle("ping", handlers.Ping)
	s.Handle("diagnostics.version", handlers.DiagnosticsVersion(st, home, DefaultVersion, caps))
	s.Handle("diagnostics.get", handlers.DiagnosticsGet(st, ops, home, DefaultVersion, caps))
	s.Handle("ops_status.get", handlers.GetOpsStatus(ops))
	s.Handle("ops_status.clear_error", handlers.ClearOpsStatusError(ops))
	s.Handle("backup.create_recovery", handlers.CreateRecoveryBackup(st, home, clock))
	s.Handle("search.query", handlers.Search(searchRepo))
	s.Handle("manuscript.search", handlers.SearchManuscript(manuscriptSearcher))
	s.Handle("manuscript.replace_preview", handlers.ReplacePreview(manuscriptEditor))
	s.Handle("manuscript.replace_apply", handlers.ReplaceApply(manuscriptEditor, clock))
	s.Handle("contextual.resolve_target", handlers.ContextualResolveTarget(contextualEditor))
	s.Handle("contextual.plan_change", handlers.ContextualPlanChange(contextualEditor))
	s.Handle("contextual.apply_change", handlers.ContextualApplyChange(contextualEditor, clock))
	s.Handle("contextual.check_consistency", handlers.ContextualCheckConsistency(contextualEditor))
	s.Handle("projects.create", handlers.CreateProject(projects, clock))
	s.Handle("projects.list", handlers.ListProjects(projects))
	s.Handle("projects.get", handlers.GetProject(projects))
	s.Handle("projects.archive", handlers.ArchiveProject(projects, clock))
	s.Handle("projects.restore", handlers.RestoreProject(projects, clock))
	s.Handle("projects.delete", handlers.DeleteProject(projects, companionSvc.DeleteProjectData))
	s.Handle("projects.update", handlers.UpdateProject(projects, clock))
	s.Handle("projects.clear_synopsis", handlers.ClearProjectSynopsis(projects, clock))
	s.Handle("nodes.get", handlers.GetNode(nodes))
	s.Handle("nodes.update_content", handlers.UpdateNodeContent(nodes, clock, summ.Enqueue))
	s.Handle("stats.today", handlers.TodayStats(writingStats))
	s.Handle("stats.range", handlers.RangeStats(writingStats))
	s.Handle("stats.summary", handlers.SummaryStats(writingStats))
	s.Handle("nodes.set_last_opened", handlers.SetLastOpened(nodes, clock))
	s.Handle("snapshots.create_manual", handlers.CreateManualSnapshot(snaps, clock))
	s.Handle("snapshots.create_auto", handlers.CreateAutoSnapshot(snaps, clock))
	s.Handle("nodes.list_tree", handlers.ListTree(nodes))
	s.Handle("nodes.create_sibling", handlers.CreateSibling(nodes, clock))
	s.Handle("nodes.create_child", handlers.CreateChild(nodes, clock))
	s.Handle("nodes.rename", handlers.RenameNode(nodes, clock))
	s.Handle("nodes.delete", handlers.DeleteNode(nodes, clock))
	s.Handle("nodes.move_to", handlers.MoveTo(nodes, clock))
	s.Handle("nodes.move_to_parent", handlers.MoveToParent(nodes, clock))
	s.Handle("nodes.move_to_root", handlers.MoveToRoot(nodes, clock))
	s.Handle("nodes.convert_to_container", handlers.ConvertToContainer(nodes, clock))
	s.Handle("nodes.restore_outline", handlers.RestoreOutline(nodes, clock))
	s.Handle("nodes.move_up", handlers.MoveUp(nodes, clock))
	s.Handle("nodes.move_down", handlers.MoveDown(nodes, clock))
	s.Handle("nodes.set_status", handlers.SetNodeStatus(nodes, clock))
	s.Handle("entities.search", handlers.SearchEntities(entities))
	s.Handle("entities.list", handlers.ListEntities(entities))
	s.Handle("entities.get", handlers.GetEntity(entities))
	s.Handle("entities.create", handlers.CreateEntity(entities, clock))
	s.Handle("entities.update", handlers.UpdateEntity(entities, clock))
	s.Handle("entities.scenes", handlers.EntityScenes(mentions, nodes))
	s.Handle("threads.create", handlers.CreateThread(threads))
	s.Handle("threads.list", handlers.ListThreads(threads))
	s.Handle("threads.get", handlers.GetThread(threads))
	s.Handle("threads.update", handlers.UpdateThread(threads))
	s.Handle("threads.close", handlers.CloseThread(threads, clock))
	s.Handle("threads.reopen", handlers.ReopenThread(threads))
	s.Handle("beats.create", handlers.CreateBeat(beats))
	s.Handle("beats.list_by_thread", handlers.ListBeatsByThread(beats))
	s.Handle("beats.list_by_node", handlers.ListBeatsByNode(beats))
	s.Handle("beats.update", handlers.UpdateBeat(beats))
	s.Handle("beats.reorder", handlers.ReorderBeats(beats))
	s.Handle("beats.delete", handlers.DeleteBeat(beats))
	s.Handle("plot.spine_panel", handlers.PlotSpinePanel(plotBuilder))
	s.Handle("relationships.create_one", handlers.CreateOneRelationship(relationships))
	s.Handle("relationships.create_pair", handlers.CreatePairRelationship(relationships))
	s.Handle("relationships.list_by_entity", handlers.ListRelationshipsByEntity(relationships))
	s.Handle("relationships.update", handlers.UpdateRelationship(relationships))
	s.Handle("relationships.delete", handlers.DeleteRelationship(relationships))
	s.Handle("notes.create", handlers.CreateNote(notes, clock))
	s.Handle("notes.list_for_node", handlers.ListNotesForNode(notes))
	s.Handle("notes.get", handlers.GetNote(notes))
	s.Handle("notes.update", handlers.UpdateNote(notes))
	s.Handle("notes.delete", handlers.DeleteNote(notes))
	s.Handle("facts.create", handlers.CreateFact(facts, clock))
	s.Handle("facts.create_from_url", handlers.CreateFactFromURL(facts, clock, nil))
	s.Handle("facts.list", handlers.ListFacts(facts))
	s.Handle("facts.update", handlers.UpdateFact(facts, clock))
	s.Handle("facts.delete", handlers.DeleteFact(facts))
	s.Handle("mentions.list_for_node", handlers.ListMentionsForNode(mentions))
	s.Handle("settings.get", handlers.GetSettings(settingsStore))
	s.Handle("settings.set", handlers.SetSettings(settingsStore))
	s.Handle("mcp.status", handlers.MCPStatus(mcpCtrl))
	s.Handle("mcp.enable", handlers.MCPEnable(mcpCtrl))
	s.Handle("mcp.disable", handlers.MCPDisable(mcpCtrl))
	s.Handle("mcp.regenerate_token", handlers.MCPRegenerateToken(mcpCtrl))
	s.Handle("mcp.activity", handlers.MCPActivity(mcpCtrl))
	s.Handle("snapshots.list_for_node", handlers.ListSnapshotsForNode(snaps))
	s.Handle("snapshots.compare", handlers.CompareSnapshots(snaps))
	s.Handle("snapshots.restore", handlers.RestoreSnapshot(nodes, snaps, clock))
	s.Handle("export.project", handlers.ExportProject(projects, nodes, entities, relationships))
	s.Handle("export.node", handlers.ExportNode(nodes))
	s.Handle("export.nodeText", handlers.ExportNodeText(nodes))
	s.Handle("export.companion_history", handlers.ExportCompanionHistory(projects, companionHistory, companionSvc))
	s.Handle("imports.markdown", handlers.ImportMarkdown(projects, nodes, entities, relationships, clock))
	s.Handle("imports.preview", handlers.ImportPreview())
	return nil
}

// Handle dispatches one JSONRPC request envelope.
func (a *App) Handle(ctx context.Context, request []byte) ([]byte, error) {
	return a.server.HandleMessage(ctx, request)
}

// SetNotifier routes JSONRPC notifications to fn.
func (a *App) SetNotifier(fn func(method string, params json.RawMessage)) {
	a.server.SetNotifier(fn)
}

// Close stops background jobs and closes durable resources.
func (a *App) Close() error {
	var first error
	a.once.Do(func() {
		a.cancel()
		for i := len(a.closers) - 1; i >= 0; i-- {
			if err := a.closers[i](); err != nil && first == nil {
				first = err
			}
		}
	})
	return first
}
