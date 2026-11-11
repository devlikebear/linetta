// Package storetest hands tests a migrated library database without making
// every test pay for the migration chain.
//
// The store is modernc.org/sqlite, SQLite compiled to Go, so the race detector
// instruments the whole SQL engine: running the chain on an empty file costs
// about 25 ms in a plain build and about 0.7 s under -race. Opening a fresh
// store per test made that the bulk of CI's `go test -race` run. Here the
// chain runs once per test binary and every later database starts as a copy
// of that result, which store.Open then finds already at the latest version.
package storetest

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/devlikebear/linetta/engine/internal/store"
)

var (
	buildOnce sync.Once
	template  []byte
	buildErr  error
)

// Open is store.Open for tests. A path with no file yet starts as a copy of a
// fully migrated database; a path that already holds one — a legacy fixture,
// or a store reopened by the test — is opened as it is, so migration and
// reopen behaviour is still exercised for real.
func Open(ctx context.Context, path string) (*store.Store, error) {
	if err := Seed(path); err != nil {
		return nil, err
	}
	return store.Open(ctx, path)
}

// Seed writes the migrated database to path unless a file is already there.
// It is for tests whose code under test opens the store itself, such as an
// engine started on a home directory: seed the library path before it starts.
func Seed(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	buildOnce.Do(build)
	if buildErr != nil {
		return buildErr
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, template, 0o600)
}

func build() {
	dir, err := os.MkdirTemp("", "linetta-storetest-")
	if err != nil {
		buildErr = fmt.Errorf("storetest: %w", err)
		return
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "template.db")
	st, err := store.Open(context.Background(), path)
	if err != nil {
		buildErr = fmt.Errorf("storetest: build template: %w", err)
		return
	}
	// Fold the WAL into the main file so the file alone is the whole database.
	if _, err := st.DB().Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		_ = st.Close()
		buildErr = fmt.Errorf("storetest: checkpoint template: %w", err)
		return
	}
	if err := st.Close(); err != nil {
		buildErr = fmt.Errorf("storetest: close template: %w", err)
		return
	}
	template, buildErr = os.ReadFile(path)
}
