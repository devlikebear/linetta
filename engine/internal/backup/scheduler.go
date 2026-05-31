package backup

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"
)

// RetentionFn is called after each successful backup run. Use it to also thin
// out node_snapshots, etc.
type RetentionFn func(ctx context.Context) error

type TickResult struct {
	StartedAt      int64  `json:"started_at"`
	FinishedAt     int64  `json:"finished_at"`
	BackupPath     string `json:"backup_path"`
	BackupRan      bool   `json:"backup_ran"`
	BackupError    string `json:"backup_error"`
	PruneError     string `json:"prune_error"`
	RetentionError string `json:"retention_error"`
}

func (r TickResult) OK() bool {
	return r.BackupError == "" && r.PruneError == "" && r.RetentionError == ""
}

func (r TickResult) Error() string {
	for _, msg := range []string{r.BackupError, r.PruneError, r.RetentionError} {
		if msg != "" {
			return msg
		}
	}
	return ""
}

// Start runs the daily backup loop. It performs one run immediately, then
// arranges the next run for local midnight + 1 minute (to avoid clock drift
// at exactly 00:00:00).
//
// The returned stop func cancels the loop and waits for the current iteration
// to finish.
//
// nowFn / sleepFn / onTick are injection points used by tests; production wires
// them to time.Now, time.Sleep, and a no-op.
func Start(
	ctx context.Context,
	db *sql.DB,
	home string,
	retention RetentionFn,
	nowFn func() time.Time,
	sleepFn func(time.Duration),
	onTick func(TickResult),
) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			now := nowFn()
			result := TickResult{StartedAt: now.UnixMilli()}
			if path, didRun, err := RunDailyIfNeeded(ctx, db, home, now); err != nil {
				result.BackupError = err.Error()
				fmt.Fprintf(os.Stderr, "backup: %v\n", err)
			} else {
				result.BackupPath = path
				result.BackupRan = didRun
			}
			if err := Prune(home, now); err != nil {
				result.PruneError = err.Error()
				fmt.Fprintf(os.Stderr, "backup prune: %v\n", err)
			}
			if retention != nil {
				if err := retention(ctx); err != nil {
					result.RetentionError = err.Error()
					fmt.Fprintf(os.Stderr, "snapshot retention: %v\n", err)
				}
			}
			result.FinishedAt = nowFn().UnixMilli()
			if onTick != nil {
				onTick(result)
			}
			next := nextLocalMidnight(now)
			wait := next.Sub(now) + time.Minute
			select {
			case <-ctx.Done():
				return
			default:
			}
			sleepFn(wait)
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

func nextLocalMidnight(t time.Time) time.Time {
	t = t.In(time.Local)
	return time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
}
