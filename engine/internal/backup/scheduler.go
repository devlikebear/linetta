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
	onTick func(),
) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			now := nowFn()
			if _, _, err := RunDailyIfNeeded(ctx, db, home, now); err != nil {
				fmt.Fprintf(os.Stderr, "backup: %v\n", err)
			}
			if err := Prune(home, now); err != nil {
				fmt.Fprintf(os.Stderr, "backup prune: %v\n", err)
			}
			if retention != nil {
				if err := retention(ctx); err != nil {
					fmt.Fprintf(os.Stderr, "snapshot retention: %v\n", err)
				}
			}
			if onTick != nil {
				onTick()
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
