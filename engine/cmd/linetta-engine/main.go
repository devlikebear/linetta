package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	_ "github.com/devlikebear/tars/pkg/llm" // pin

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/backup"
	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/contextualedit"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/manuscriptedit"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/modelcatalog"
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
	"github.com/devlikebear/linetta/engine/internal/summarizer"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

const engineVersion = "0.4.24"

// providerSource adapts *settings.Store to ai.ProviderSource. The adapter lives
// here so the settings package has no dependency on ai.
type providerSource struct{ store *settings.Store }

func (p providerSource) Resolve() ai.ResolvedProvider {
	r := p.store.Resolve()
	return ai.ResolvedProvider{
		Provider: r.Provider,
		Model:    r.Model,
		APIKey:   r.APIKey,
		BaseURL:  r.BaseURL,
		CliPath:  r.CliPath,
	}
}

func (p providerSource) WebSearchProvider() string {
	return p.store.WebSearchProvider()
}

func (p providerSource) WebSearchAPIKey() string {
	return p.store.WebSearchAPIKey()
}

func main() {
	stdio := flag.Bool("stdio", false, "serve JSONRPC over stdin/stdout")
	flag.Parse()

	if !*stdio {
		fmt.Fprintln(os.Stderr, "linetta-engine: --stdio required (other modes land in later plans)")
		os.Exit(2)
	}

	if err := paths.EnsureHome(); err != nil {
		fail("ensure home: %v", err)
	}
	dbPath, err := paths.DBPath()
	if err != nil {
		fail("db path: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	st, err := store.Open(ctx, dbPath)
	if err != nil {
		fail("open store: %v", err)
	}
	defer st.Close()

	s := rpc.NewServer()

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

	// Keep the mentions table in sync with each saved Tiptap doc.
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mentions.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})
	nodes.SetWritingStatsRecorder(writingStats)
	nodes.SetManuscriptIndexer(manuscriptIndexer)

	settingsStore, err := settings.New()
	if err != nil {
		fail("settings: %v", err)
	}
	providerSrc := providerSource{store: settingsStore}

	aiRuns := store.NewAIRunsRepo(st)
	runner := ai.NewRunner(s.Notifier(), aiRuns, ai.DefaultClientFactory, providerSrc)

	summ := summarizer.New(nodes, providerSrc, ai.DefaultClientFactory).
		WithOpsStatus(ops, clock)
	stopSummarizer := summ.Start(ctx)
	defer stopSummarizer()

	// Plan 16: ContextBuilder needs a SummaryRefresher so stale container
	// summaries can be filled on-demand. *summarizer.Summarizer satisfies
	// ai.SummaryRefresher via RefreshNow.
	contextBuilder := ai.NewContextBuilder(projects, nodes, mentions, threads, beats, notes, relationships).
		WithSummaryRefresher(summ)

	// Backup + retention scheduler. Runs once at boot, then daily at midnight+1m.
	home, err := paths.Home()
	if err != nil {
		fail("home: %v", err)
	}
	syncer := setupGitSync(s, settingsStore, projects, nodes, entities, relationships, ops)
	retentionFn := func(ctx context.Context) error {
		if err := snapshot.Thin(ctx, st.DB(), time.Now().UnixMilli()); err != nil {
			return err
		}
		res, err := syncer.RunOnce(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gitsync (daily): %v\n", err)
			return nil // never block the scheduler
		}
		if res.Error != "" {
			fmt.Fprintf(os.Stderr, "gitsync (daily): %s\n", res.Error)
		}
		return nil
	}
	stopBackup := backup.Start(ctx, st.DB(), home, retentionFn,
		time.Now, time.Sleep, func(result backup.TickResult) {
			_ = ops.Record(ctx, opsstatus.JobBackup, result.StartedAt, result.FinishedAt,
				result.OK(), result.Error(), result)
		})
	defer stopBackup()

	companionSvc := companion.NewService(
		filepath.Join(home, "companion"),
		projects, threads, entities, relationships, plotBuilder,
		s.Notifier(), companion.ClientFactory(ai.DefaultClientFactory), providerSrc, home,
		nodes, beats,
	).WithFacts(facts).
		WithOpsStatus(ops).
		WithHistory(companion.NewHistoryRepo(st.DB())).
		WithReferences(companion.NewReferenceRepo(st.DB())).
		WithManuscript(manuscriptSearcher)

	caps := handlers.Capabilities{
		UnavailableProviders: ai.UnavailableProviders(),
		GitSyncAvailable:     gitSyncAvailable,
	}
	s.Handle("ping", handlers.Ping)
	s.Handle("diagnostics.version", handlers.DiagnosticsVersion(st, engineVersion, caps))
	s.Handle("diagnostics.get", handlers.DiagnosticsGet(st, ops, engineVersion, caps))
	s.Handle("ops_status.get", handlers.GetOpsStatus(ops))
	s.Handle("ops_status.clear_error", handlers.ClearOpsStatusError(ops))
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
	s.Handle("projects.rewrite_synopsis", handlers.RewriteProjectSynopsis(projects, contextBuilder, clock))
	s.Handle("projects.clear_synopsis", handlers.ClearProjectSynopsis(projects, clock))
	s.Handle("nodes.get", handlers.GetNode(nodes))
	s.Handle("nodes.update_content", handlers.UpdateNodeContent(nodes, snaps, clock, summ.Enqueue))
	s.Handle("stats.today", handlers.TodayStats(writingStats))
	s.Handle("stats.range", handlers.RangeStats(writingStats))
	s.Handle("stats.summary", handlers.SummaryStats(writingStats))
	s.Handle("nodes.set_last_opened", handlers.SetLastOpened(nodes, clock))
	s.Handle("snapshots.create_manual", handlers.CreateManualSnapshot(snaps, clock))
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
	s.Handle("ai.run", handlers.RunAI(contextBuilder, runner, clock))
	s.Handle("ai.preview_context", handlers.PreviewContext(contextBuilder))
	s.Handle("ai.cancel", handlers.CancelAI(runner))
	s.Handle("companion.preview_context", handlers.CompanionPreviewContext(companionSvc))
	s.Handle("companion.send", handlers.CompanionSend(companionSvc, clock))
	s.Handle("companion.history", handlers.CompanionHistory(companionSvc))
	s.Handle("companion.compact", handlers.CompanionCompact(companionSvc, clock))
	s.Handle("companion.clear", handlers.CompanionClear(companionSvc))
	s.Handle("companion.cancel", handlers.CompanionCancel(companionSvc))
	s.Handle("companion.remember", handlers.CompanionRemember(companionSvc))
	s.Handle("companion.apply_ops", handlers.CompanionApplyOps(companionSvc, clock))
	s.Handle("companion.references.list", handlers.CompanionReferencesList(companionSvc))
	s.Handle("companion.references.create", handlers.CompanionReferencesCreate(companionSvc, clock))
	s.Handle("companion.references.update", handlers.CompanionReferencesUpdate(companionSvc, clock))
	s.Handle("companion.references.delete", handlers.CompanionReferencesDelete(companionSvc))
	s.Handle("settings.get", handlers.GetSettings(settingsStore))
	s.Handle("settings.set", handlers.SetSettings(settingsStore))
	s.Handle("providers.list_models", handlers.ListModels(settingsStore, modelcatalog.Default()))
	s.Handle("providers.detect_cli", handlers.DetectCLI())
	s.Handle("providers.test", handlers.TestProvider(settingsStore, ai.DefaultClientFactory))
	s.Handle("web_search.test", handlers.TestWebSearch(settingsStore, handlers.DefaultWebSearchTester))
	s.Handle("snapshots.list_for_node", handlers.ListSnapshotsForNode(snaps))
	s.Handle("snapshots.restore", handlers.RestoreSnapshot(nodes, snaps, clock))
	s.Handle("export.project", handlers.ExportProject(projects, nodes, entities, relationships))
	s.Handle("export.node", handlers.ExportNode(nodes))
	s.Handle("export.nodeText", handlers.ExportNodeText(nodes))
	s.Handle("imports.markdown", handlers.ImportMarkdown(projects, nodes, entities, relationships, clock))
	s.Handle("imports.preview", handlers.ImportPreview())

	if err := s.Serve(ctx, os.Stdin, os.Stdout); err != nil && !errors.Is(err, io.EOF) {
		fail("serve: %v", err)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "linetta-engine: "+format+"\n", args...)
	os.Exit(1)
}
