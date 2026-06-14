package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
)

// TestOpen_concurrentWriteTxns reproduces the SQLITE_BUSY error seen in
// production: multiple goroutines (RPC handlers + background jobs) each run a
// read-then-write transaction against the shared pool. Without serializing the
// pool to a single connection, two deferred transactions upgrade to writes on
// different connections and SQLite returns SQLITE_BUSY (BUSY_SNAPSHOT), which
// busy_timeout does not retry.
func TestOpen_concurrentWriteTxns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	ctx := context.Background()

	s, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	const workers = 8
	const perWorker = 25

	var wg sync.WaitGroup
	errs := make(chan error, workers*perWorker)
	for w := range workers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := range perWorker {
				tx, err := s.DB().BeginTx(ctx, nil)
				if err != nil {
					errs <- err
					return
				}
				// Read first so the transaction must upgrade read->write.
				var n int
				if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM projects`).Scan(&n); err != nil {
					_ = tx.Rollback()
					errs <- err
					return
				}
				id := "p-" + string(rune('a'+w)) + "-" + string(rune('0'+i%10)) + "-" + string(rune('0'+i/10))
				if _, err := tx.ExecContext(ctx, `
INSERT INTO projects (id, title, genres, length_target, default_pov, created_at, updated_at)
VALUES (?, 'x', '[]', 'novel', 'first', 0, 0)`, id); err != nil {
					_ = tx.Rollback()
					errs <- err
					return
				}
				if err := tx.Commit(); err != nil {
					errs <- err
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent write txn failed: %v", err)
	}
}
