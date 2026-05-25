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

	"github.com/devlikebear/linetta/engine/internal/paths"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
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

	repo := project.NewRepo(st)
	clock := func() int64 { return time.Now().UnixMilli() }

	s := rpc.NewServer()
	s.Handle("ping", handlers.Ping)
	s.Handle("projects.create", handlers.CreateProject(repo, clock))
	s.Handle("projects.list", handlers.ListProjects(repo))
	s.Handle("projects.get", handlers.GetProject(repo))
	s.Handle("projects.archive", handlers.ArchiveProject(repo, clock))

	if err := s.Serve(ctx, os.Stdin, os.Stdout); err != nil && !errors.Is(err, io.EOF) {
		fail("serve: %v", err)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "linetta-engine: "+format+"\n", args...)
	os.Exit(1)
}
