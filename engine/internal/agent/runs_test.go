//go:build !mobile

package agent

import (
	"context"
	"errors"
	"testing"
)

func TestRuns_oneRunPerWork(t *testing.T) {
	r := newRunRegistry()
	if err := r.start("p1", "run-1", func() {}); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if err := r.start("p1", "run-2", func() {}); !errors.Is(err, ErrBusy) {
		t.Fatalf("second start on the same work = %v, want ErrBusy", err)
	}
}

// Two works are two conversations. Blocking one on the other would make the
// panel unusable for a writer who keeps two books open.
func TestRuns_differentWorksRunTogether(t *testing.T) {
	r := newRunRegistry()
	if err := r.start("p1", "run-1", func() {}); err != nil {
		t.Fatalf("p1: %v", err)
	}
	if err := r.start("p2", "run-2", func() {}); err != nil {
		t.Fatalf("p2: %v", err)
	}
}

func TestRuns_finishReleasesTheWork(t *testing.T) {
	r := newRunRegistry()
	_ = r.start("p1", "run-1", func() {})
	r.finish("p1", "run-1")
	if err := r.start("p1", "run-2", func() {}); err != nil {
		t.Fatalf("start after finish: %v", err)
	}
}

// A late finish from a run that was already replaced must not evict the run
// that replaced it — otherwise a slow teardown silently permits a third run.
func TestRuns_finishIgnoresAStaleRunID(t *testing.T) {
	r := newRunRegistry()
	_ = r.start("p1", "run-1", func() {})
	r.finish("p1", "run-old")
	if err := r.start("p1", "run-2", func() {}); !errors.Is(err, ErrBusy) {
		t.Fatalf("a stale finish released the work: %v", err)
	}
}

func TestRuns_cancelInvokesTheRunsCancelFunc(t *testing.T) {
	r := newRunRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	_ = r.start("p1", "run-1", cancel)
	if !r.cancel("run-1") {
		t.Fatal("cancel reported the run as unknown")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("the run's context was not cancelled")
	}
}

// The panel can ask to cancel a run that already finished — a click landing
// just after the last token. That is not an error.
func TestRuns_cancelUnknownRunIsFalseNotAPanic(t *testing.T) {
	r := newRunRegistry()
	if r.cancel("never-existed") {
		t.Fatal("cancel claimed to have stopped a run that never ran")
	}
}
