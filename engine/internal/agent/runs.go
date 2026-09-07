//go:build !mobile

package agent

import (
	"context"
	"errors"
	"sync"
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
}

func newRunRegistry() *runRegistry {
	return &runRegistry{
		byProject: map[string]string{},
		cancels:   map[string]context.CancelFunc{},
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
	return nil
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
