// Package app wires Clio's engine: its own database, and only the RPC methods
// Clio serves. It follows engineapp.Open but creates none of Linetta's repos.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/devlikebear/linetta/engine/internal/clio/handlers"
	cliomigrations "github.com/devlikebear/linetta/engine/internal/clio/migrations"
	"github.com/devlikebear/linetta/engine/internal/clio/post"
	"github.com/devlikebear/linetta/engine/internal/paths"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store"
)

// Options configures an embedded Clio engine.
type Options struct {
	// Home is the data directory. Empty means paths.HomeFor for Clio.
	Home string
	// Secrets overrides the credential backend; nil means the OS keychain.
	// Phase 1 stores no secrets, but the hook is part of the app contract.
	Secrets settings.SecretStore
}

// App owns the store and RPC server of one Clio engine instance.
type App struct {
	server *rpc.Server
	cancel context.CancelFunc
	st     *store.Store
	once   sync.Once
	err    error
}

// Open creates the home, opens <home>/clio.db with Clio's migrations and
// registers the Clio methods.
func Open(ctx context.Context, opts Options) (*App, error) {
	home := opts.Home
	if home == "" {
		h, err := paths.HomeFor("com.devlikebear.clio", "CLIO_HOME")
		if err != nil {
			return nil, err
		}
		home = h
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, fmt.Errorf("ensure home: %w", err)
	}
	appCtx, cancel := context.WithCancel(ctx)
	st, err := store.OpenWith(appCtx, filepath.Join(home, "clio.db"), cliomigrations.FS)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open store: %w", err)
	}
	a := &App{server: rpc.NewServer(), cancel: cancel, st: st}
	posts := post.NewRepo(st)
	a.server.Handle("ping", handlers.Ping)
	a.server.Handle("posts.import_preview", handlers.ImportPreview())
	a.server.Handle("posts.import", handlers.Import(posts))
	a.server.Handle("posts.list", handlers.ListPosts(posts))
	a.server.Handle("posts.counts", handlers.CountPosts(posts))
	a.server.Handle("voice.profile", handlers.VoiceProfile(posts))
	return a, nil
}

// Handle dispatches one JSONRPC request. Notifications return nil, nil.
func (a *App) Handle(ctx context.Context, request []byte) ([]byte, error) {
	return a.server.HandleMessage(ctx, request)
}

// SetNotifier routes JSONRPC notifications to fn.
func (a *App) SetNotifier(fn func(method string, params json.RawMessage)) {
	a.server.SetNotifier(fn)
}

// Close stops the engine and closes the database.
func (a *App) Close() error {
	a.once.Do(func() {
		a.cancel()
		a.err = a.st.Close()
	})
	return a.err
}
