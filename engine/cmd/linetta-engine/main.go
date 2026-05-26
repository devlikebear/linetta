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
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/paths"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/store"
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

	// Keep the mentions table in sync with each saved Tiptap doc.
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mentions.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})

	settingsStore, err := settings.New()
	if err != nil {
		fail("settings: %v", err)
	}

	aiRuns := store.NewAIRunsRepo(st)
	contextBuilder := ai.NewContextBuilder(projects, nodes, mentions)
	runner := ai.NewRunner(s.Notifier(), aiRuns, ai.DefaultClientFactory, settingsStore)

	clock := func() int64 { return time.Now().UnixMilli() }
	s.Handle("ping", handlers.Ping)
	s.Handle("projects.create", handlers.CreateProject(projects, clock))
	s.Handle("projects.list", handlers.ListProjects(projects))
	s.Handle("projects.get", handlers.GetProject(projects))
	s.Handle("projects.archive", handlers.ArchiveProject(projects, clock))
	s.Handle("nodes.get", handlers.GetNode(nodes))
	s.Handle("nodes.update_content", handlers.UpdateNodeContent(nodes, snaps, clock))
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
	s.Handle("mentions.list_for_node", handlers.ListMentionsForNode(mentions))
	s.Handle("ai.run", handlers.RunAI(contextBuilder, runner, clock))
	s.Handle("ai.cancel", handlers.CancelAI(runner))

	if err := s.Serve(ctx, os.Stdin, os.Stdout); err != nil && !errors.Is(err, io.EOF) {
		fail("serve: %v", err)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "linetta-engine: "+format+"\n", args...)
	os.Exit(1)
}
