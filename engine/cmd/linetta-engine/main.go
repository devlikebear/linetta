package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/devlikebear/tars/pkg/llm" // pin

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/backup"
	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/gitsync"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/paths"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/summarizer"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

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
	entities := entity.NewRepo(st)
	mentions := mention.NewRepo(st)
	threads := thread.NewRepo(st)
	beats := beat.NewRepo(st)
	notes := note.NewRepo(st)
	relationships := relationship.NewRepo(st)

	// Keep the mentions table in sync with each saved Tiptap doc.
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mentions.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})

	settingsStore, err := settings.New()
	if err != nil {
		fail("settings: %v", err)
	}

	aiRuns := store.NewAIRunsRepo(st)
	runner := ai.NewRunner(s.Notifier(), aiRuns, ai.DefaultClientFactory, settingsStore)

	summ := summarizer.New(nodes, settingsStore, ai.DefaultClientFactory)
	stopSummarizer := summ.Start(ctx)
	defer stopSummarizer()

	// Plan 16: ContextBuilder needs a SummaryRefresher so stale container
	// summaries can be filled on-demand. *summarizer.Summarizer satisfies
	// ai.SummaryRefresher via RefreshNow.
	contextBuilder := ai.NewContextBuilder(projects, nodes, mentions, threads, beats, notes).
		WithSummaryRefresher(summ)

	// Backup + retention scheduler. Runs once at boot, then daily at midnight+1m.
	home, err := paths.Home()
	if err != nil {
		fail("home: %v", err)
	}
	syncer := gitsync.New(settingsStore, projects, nodes, entities)
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
		time.Now, time.Sleep, nil /* onTick */)
	defer stopBackup()

	clock := func() int64 { return time.Now().UnixMilli() }
	s.Handle("ping", handlers.Ping)
	s.Handle("projects.create", handlers.CreateProject(projects, clock))
	s.Handle("projects.list", handlers.ListProjects(projects))
	s.Handle("projects.get", handlers.GetProject(projects))
	s.Handle("projects.archive", handlers.ArchiveProject(projects, clock))
	s.Handle("nodes.get", handlers.GetNode(nodes))
	s.Handle("nodes.update_content", handlers.UpdateNodeContent(nodes, snaps, clock, summ.Enqueue))
	s.Handle("nodes.set_last_opened", handlers.SetLastOpened(nodes, clock))
	s.Handle("snapshots.create_manual", handlers.CreateManualSnapshot(snaps, clock))
	s.Handle("nodes.list_tree", handlers.ListTree(nodes))
	s.Handle("nodes.create_sibling", handlers.CreateSibling(nodes, clock))
	s.Handle("nodes.create_child", handlers.CreateChild(nodes, clock))
	s.Handle("nodes.rename", handlers.RenameNode(nodes, clock))
	s.Handle("nodes.delete", handlers.DeleteNode(nodes, clock))
	s.Handle("nodes.move_up", handlers.MoveUp(nodes, clock))
	s.Handle("nodes.move_down", handlers.MoveDown(nodes, clock))
	s.Handle("entities.search", handlers.SearchEntities(entities))
	s.Handle("entities.get", handlers.GetEntity(entities))
	s.Handle("entities.create", handlers.CreateEntity(entities, clock))
	s.Handle("entities.update", handlers.UpdateEntity(entities, clock))
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
	s.Handle("mentions.list_for_node", handlers.ListMentionsForNode(mentions))
	s.Handle("ai.run", handlers.RunAI(contextBuilder, runner, clock))
	s.Handle("ai.preview_context", handlers.PreviewContext(contextBuilder))
	s.Handle("ai.cancel", handlers.CancelAI(runner))
	s.Handle("settings.get", handlers.GetSettings(settingsStore))
	s.Handle("settings.set", handlers.SetSettings(settingsStore))
	s.Handle("snapshots.list_for_node", handlers.ListSnapshotsForNode(snaps))
	s.Handle("snapshots.restore", handlers.RestoreSnapshot(nodes, snaps, clock))
	s.Handle("export.project", handlers.ExportProject(projects, nodes, entities))
	s.Handle("export.node", handlers.ExportNode(nodes))
	s.Handle("imports.markdown", handlers.ImportMarkdown(projects, nodes, clock))
	s.Handle("imports.preview", handlers.ImportPreview())
	s.Handle("git_sync.run", handlers.RunGitSync(syncer))
	s.Handle("git_sync.init", handlers.InitGitSync(syncer))

	if err := s.Serve(ctx, os.Stdin, os.Stdout); err != nil && !errors.Is(err, io.EOF) {
		fail("serve: %v", err)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "linetta-engine: "+format+"\n", args...)
	os.Exit(1)
}
