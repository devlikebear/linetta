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
