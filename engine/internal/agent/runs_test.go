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

func TestStatusReportsTheLiveTurn(t *testing.T) {
	r := newRunRegistry()
	if st := r.status("p1"); st.Running {
		t.Fatalf("status before any run = %+v, want not running", st)
	}
	if err := r.start("p1", "run-1", func() {}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if seq := r.streamed("run-1", "Hel"); seq != 1 {
		t.Fatalf("first event seq = %d, want 1", seq)
	}
	r.streamed("run-1", "lo")
	st := r.status("p1")
	if !st.Running || st.RunID != "run-1" || st.Text != "Hello" || st.Seq != 2 || st.StartedAt == 0 {
		t.Fatalf("status mid-stream = %+v", st)
	}

	// The message became a transcript row; the tool it asked for is running.
	r.committed("run-1")
	r.inTool("run-1", "linetta_read_scene")
	st = r.status("p1")
	if st.Text != "" || st.Tool != "linetta_read_scene" || st.Seq != 3 {
		t.Fatalf("status inside a tool = %+v", st)
	}
	r.inTool("run-1", "")
	if st = r.status("p1"); st.Tool != "" || st.Seq != 4 {
		t.Fatalf("status after the tool = %+v", st)
	}

	// Another work's status is its own.
	if st := r.status("p2"); st.Running {
		t.Fatalf("status for an idle work = %+v", st)
	}

	r.finish("p1", "run-1")
	if st := r.status("p1"); st.Running {
		t.Fatalf("status after finish = %+v, want not running", st)
	}
	// A late event from the finished run numbers as 0: never ahead of any
	// status a panel could have read.
	if seq := r.next("run-1"); seq != 0 {
		t.Fatalf("seq after finish = %d, want 0", seq)
	}
}
