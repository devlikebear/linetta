//go:build !mobile

package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

// ErrBusy means the work already has a turn in flight. One run per work is
// the rule: two loops writing the same manuscript would interleave scene
// updates the writer never asked for, and the second would spend its budget
// fighting version conflicts with the first.
var ErrBusy = errors.New("agent: this work already has a turn running")

type runRegistry struct {
	mu        sync.Mutex
	byProject map[string]string             // projectID -> runID
	cancels   map[string]context.CancelFunc // runID -> cancel
	live      map[string]*liveRun           // runID -> what the turn is doing now
}

// liveRun is the part of a turn that exists nowhere else while it runs: when
// it started, how many notifications it has emitted, the reply text that has
// streamed but is not yet a transcript row, and the tool it is inside.
//
// It is what agent.status answers from. The panel is unmounted whenever the
// writer leaves the workspace, and a panel that mounts mid-turn has only the
// transcript to go on — which cannot say whether a turn is still running, and
// holds none of the reply currently streaming.
type liveRun struct {
	startedAt int64
	seq       int64
	text      strings.Builder
	tool      string
}

// RunStatus is one work's turn as agent.status reports it. Running false means
// no turn holds the work, and every other field is zero.
type RunStatus struct {
	Running   bool   `json:"running"`
	RunID     string `json:"run_id,omitempty"`
	StartedAt int64  `json:"started_at,omitempty"`
	// Seq is the number of the last notification the turn emitted. Every
	// agent.* payload carries its own, so a panel that reads this status and
	// then the transcript can tell which of the events it is holding are
	// already reflected in what it read.
	Seq int64 `json:"seq"`
	// Text is the reply streamed so far that has not been written to the
	// transcript yet — the half-finished message a panel mounting mid-turn
	// would otherwise never see.
	Text string `json:"text,omitempty"`
	// Tool is the tool the turn is inside right now, if any.
	Tool string `json:"tool,omitempty"`
}

func newRunRegistry() *runRegistry {
	return &runRegistry{
		byProject: map[string]string{},
		cancels:   map[string]context.CancelFunc{},
		live:      map[string]*liveRun{},
	}
}

// start claims the work for runID. It returns ErrBusy when another run holds it.
func (r *runRegistry) start(projectID, runID string, cancel context.CancelFunc) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, taken := r.byProject[projectID]; taken {
		return ErrBusy
	}
	r.byProject[projectID] = runID
	r.cancels[runID] = cancel
	r.live[runID] = &liveRun{startedAt: time.Now().UnixMilli()}
	return nil
}

// status reports the turn holding projectID, if one does.
func (r *runRegistry) status(projectID string) RunStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	runID, ok := r.byProject[projectID]
	if !ok {
		return RunStatus{}
	}
	st := RunStatus{Running: true, RunID: runID}
	if run := r.live[runID]; run != nil {
		st.StartedAt, st.Seq, st.Text, st.Tool = run.startedAt, run.seq, run.text.String(), run.tool
	}
	return st
}

// next numbers one notification of runID. A run the registry no longer holds
// answers 0, which is never greater than a status the panel read — its events
// are not ones a reattaching panel could be waiting on.
func (r *runRegistry) next(runID string) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bump(runID)
}

func (r *runRegistry) bump(runID string) int64 {
	run := r.live[runID]
	if run == nil {
		return 0
	}
	run.seq++
	return run.seq
}

// streamed records reply text as it leaves the model and numbers its event.
func (r *runRegistry) streamed(runID, text string) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run := r.live[runID]; run != nil {
		run.text.WriteString(text)
	}
	return r.bump(runID)
}

// committed forgets the streamed text: the message it belonged to is over,
// and what there was of it is the transcript's to report from here.
func (r *runRegistry) committed(runID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run := r.live[runID]; run != nil {
		run.text.Reset()
	}
}

// inTool marks the tool the turn is inside ("" on the way out) and numbers
// the event announcing it.
func (r *runRegistry) inTool(runID, name string) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run := r.live[runID]; run != nil {
		run.tool = name
	}
	return r.bump(runID)
}

// cancel stops the named run and reports whether it was still running. A
// false is not a failure: the writer's stop click can land after the last
// token, and the panel should not show an error for that.
func (r *runRegistry) cancel(runID string) bool {
	r.mu.Lock()
	cancel, ok := r.cancels[runID]
	r.mu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

// finish releases the work. The runID is checked against the current holder
// so a late teardown from a run that has already been replaced cannot evict
// its successor and let a third run start.
func (r *runRegistry) finish(projectID, runID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byProject[projectID] == runID {
		delete(r.byProject, projectID)
	}
	delete(r.cancels, runID)
	delete(r.live, runID)
}

// cancelAll stops every run currently in flight. Used when the service is
// closing: a turn that outlives Close would keep calling the provider and
// writing transcript rows against a store the caller is free to close the
// moment Close returns.
func (r *runRegistry) cancelAll() {
	r.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(r.cancels))
	for _, c := range r.cancels {
		cancels = append(cancels, c)
	}
	r.mu.Unlock()
	for _, c := range cancels {
		c()
	}
}

// track registers a cancel func WITHOUT claiming a work, and untrack releases
// it. This is the whole of what the background self-review needs from the
// registry, and the "without" is the point.
//
// start refuses a second run per work (ErrBusy), which is right for turns: two
// loops writing the same manuscript would interleave scene updates the writer
// never asked for. A self-review keyed on the same project id would inherit
// that rule and get it exactly backwards — the review is a janitor that runs
// after the reply has already gone, so claiming the work would make the
// writer's very next message wait on it (or, if the writer is quick, make the
// review lose to that message and never happen at all). The writer's next
// message must never wait on a review.
//
// cancelAll still reaches it, which is the half that must NOT be lost: Close
// cancels everything in cancels, and a review that outlived Close would keep
// calling the provider and writing skill files while the caller is free to
// close the store underneath it.
func (r *runRegistry) track(runID string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancels[runID] = cancel
}

// untrack releases a cancel func registered with track. It touches byProject
// not at all, because track never wrote to it — so a review tearing down can
// never evict the turn that is holding the work.
func (r *runRegistry) untrack(runID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cancels, runID)
}
