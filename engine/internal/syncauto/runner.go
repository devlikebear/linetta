//go:build !mobile

// Package syncauto keeps a library in step with the vault on its own. It decides
// when; internal/vault decides what. The engine runs it, so it keeps going with
// the window closed as long as the app stays in the background.
package syncauto

import (
	"context"
	"errors"
	"log"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/vault"
)

const (
	// Period is how often a healthy runner looks at the server.
	Period = 5 * time.Minute
	// Jitter keeps two devices from settling into the same moment.
	Jitter = 60 * time.Second
	// SettleAfter is how long after the last edit a work uploads.
	SettleAfter = 3 * time.Minute
	// MaxBackoff bounds the wait after failures, and how often a stopped runner
	// checks whether the writer fixed what stopped it.
	MaxBackoff = 30 * time.Minute
	// flushTimeout bounds the last upload as the engine shuts down.
	flushTimeout = 15 * time.Second
	// openTimeout bounds how long opening a work waits for a newer version.
	openTimeout = 8 * time.Second
)

// The state a runner is in, as a screen shows it.
const (
	StateOff     = "off"
	StateIdle    = "idle"
	StateRunning = "running"
	// StateWaiting is a connection problem: nothing for the writer to do, and it
	// retries by itself.
	StateWaiting = "waiting"
	// StateStopped needs the writer. Reason says what.
	StateStopped = "stopped"
)

// stopReasons are failures that will not pass by retrying.
var stopReasons = map[string]bool{
	rpc.ReasonVaultTermsOutdated:  true,
	rpc.ReasonVaultNotEnabled:     true,
	rpc.ReasonVaultQuotaExceeded:  true,
	rpc.ReasonVaultNoKey:          true,
	rpc.ReasonVaultWrongKey:       true,
	rpc.ReasonVaultKeyUnavailable: true,
	rpc.ReasonVaultNotConnected:   true,
	rpc.ReasonVaultUnauthorized:   true,
	rpc.ReasonSyncTargetMissing:   true,
	rpc.ReasonSyncTargetFailed:    true,
}

// Vault is the part of the vault service the runner uses.
type Vault interface {
	SyncAll(ctx context.Context, policy vault.SyncPolicy) (vault.SyncReport, error)
}

// Target is one place the library is kept, as settings have it now.
type Target struct {
	ID    string
	Vault Vault
	// Primary is the one two-way target. The others are mirrors.
	Primary bool
}

// Export is a Markdown export that is not part of a target: a folder or a
// repository the writer set up without choosing it for sync. It keeps its daily
// run whether or not automatic sync is on.
type Export struct {
	ID  string
	Run func(ctx context.Context) error
}

// How often Git runs. Its history is never rewritten and sealed blobs do not
// compress, so it runs far less than the other two.
const (
	GitPrimaryEvery = 30 * time.Minute
	GitMirrorEvery  = 24 * time.Hour
	ExportEvery     = 24 * time.Hour
)

// Status is what the sync screen shows. LastSuccessAt is unix millis, 0 for never.
type Status struct {
	Enabled       bool             `json:"enabled"`
	State         string           `json:"state"`
	Reason        string           `json:"reason,omitempty"`
	LastSuccessAt int64            `json:"last_success_at"`
	Works         []vault.WorkSync `json:"works"`
	// Archived names works a run moved to the archive because another device
	// deleted them. It holds until the next run that archives something.
	Archived []string `json:"archived"`
}

// Runner decides when to sync. Fields are set before Start.
type Runner struct {
	// Targets lists where the library is kept, as of now.
	Targets func() []Target
	// Exports lists the daily Markdown exports. Run skips one that is a target.
	Exports func() []Export
	Enabled func() bool
	// Busy says an agent is writing to the work.
	Busy func(projectID string) bool
	Now  func() time.Time
	// Sleep waits for d, or for ctx to end.
	Sleep func(ctx context.Context, d time.Duration) error
	// Notify hears every change of status. It may be nil.
	Notify func(Status)
	// Random returns a jitter in [-Jitter, Jitter]. Nil uses math/rand.
	Random func() time.Duration
	// Record keeps the last success across restarts. It may be nil.
	Record func(startedAt, finishedAt time.Time, ok bool, reason string, lastSuccess int64)
	// LastSuccess is what Record kept. It may be nil.
	LastSuccess func() int64

	mu       sync.Mutex
	active   map[string]bool
	edited   map[string]time.Time
	flushing map[string]bool
	status   Status
	failures int
	wake     context.CancelFunc
	woken    bool
	// lastRun says when a target or export last ran, for the ones with a period.
	lastRun map[string]time.Time
	// closed says a work was left, which is when the mirrors take it.
	closed bool
	// mirrors is how each mirror's last run went.
	mirrors map[string]MirrorStatus
}

// MirrorStatus is how a mirror's last run went. Reason is a code, empty when it
// went well.
type MirrorStatus struct {
	LastSuccessAt int64  `json:"last_success_at"`
	Reason        string `json:"reason,omitempty"`
}

func (r *Runner) init() {
	if r.active != nil {
		return
	}
	r.active = map[string]bool{}
	r.edited = map[string]time.Time{}
	r.flushing = map[string]bool{}
	r.lastRun = map[string]time.Time{}
	r.mirrors = map[string]MirrorStatus{}
	r.status = Status{State: StateOff, Works: []vault.WorkSync{}, Archived: []string{}}
	if r.LastSuccess != nil {
		r.status.LastSuccessAt = r.LastSuccess()
	}
}

func (r *Runner) enabled() bool { return r.Enabled != nil && r.Enabled() }

func (r *Runner) jitter() time.Duration {
	if r.Random != nil {
		return r.Random()
	}
	return time.Duration(rand.Int64N(int64(2*Jitter))) - Jitter
}

// Start runs the loop until stop, which waits for it and then uploads what is
// still waiting.
func (r *Runner) Start(ctx context.Context) (stop func()) {
	r.mu.Lock()
	r.init()
	r.mu.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			_, _ = r.Tick(ctx)
			if err := r.sleep(ctx, r.nextWait()); errors.Is(err, context.Canceled) && ctx.Err() != nil {
				return
			}
		}
	}()
	return func() {
		cancel()
		<-done
		r.flush()
	}
}

// sleep waits until the next run is due or something wakes the runner.
func (r *Runner) sleep(ctx context.Context, d time.Duration) error {
	waiting, cancel := context.WithCancel(ctx)
	defer cancel()
	r.mu.Lock()
	r.wake = cancel
	woken := r.woken
	r.woken = false
	r.mu.Unlock()
	if woken {
		return nil
	}
	return r.Sleep(waiting, d)
}

// RunNow wakes the runner for a run right away.
func (r *Runner) RunNow() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.init()
	r.woken = true
	if r.wake != nil {
		r.wake()
	}
}

// nextWait is the time to the next run: the period, longer after failures, or
// sooner if a work settles first.
func (r *Runner) nextWait() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	wait := Period + r.jitter()
	switch {
	case r.status.State == StateStopped:
		wait = MaxBackoff
	case r.failures > 0:
		wait = min(Period<<min(r.failures, 3), MaxBackoff)
	}
	now := r.Now()
	for _, at := range r.edited {
		if due := at.Add(SettleAfter).Sub(now); due < wait {
			wait = max(due, time.Second)
		}
	}
	return wait
}

// Touch records an edit, which holds the work's upload back until it settles.
func (r *Runner) Touch(projectID string) {
	r.mu.Lock()
	r.init()
	r.edited[projectID] = r.Now()
	r.mu.Unlock()
}

// Open marks a work as on screen. It takes a newer version first, so the writer
// opens the latest text, and reports whether one came in. Failing leaves the work
// as it is here: being offline must not stop anyone opening their own book.
func (r *Runner) Open(ctx context.Context, projectID string) (pulled bool, err error) {
	if primary, ok := r.primary(); ok && r.enabled() && primary.ID != targetGit {
		ctx, cancel := context.WithTimeout(ctx, openTimeout)
		defer cancel()
		report, syncErr := primary.Vault.SyncAll(ctx, r.policy(projectID))
		pulled, err = len(report.Pulled) > 0, syncErr
		if errors.Is(syncErr, context.DeadlineExceeded) {
			err = nil
		}
	}
	r.mu.Lock()
	r.init()
	r.active[projectID] = true
	r.mu.Unlock()
	return pulled, err
}

// Close marks a work as off screen, and sends it without waiting for it to
// settle: the writer has left it.
func (r *Runner) Close(projectID string) {
	r.mu.Lock()
	r.init()
	delete(r.active, projectID)
	r.flushing[projectID] = true
	r.closed = true
	r.mu.Unlock()
	r.RunNow()
}

// Flush sends a work at once, for when the app goes to the background.
func (r *Runner) Flush(projectID string) {
	r.mu.Lock()
	r.init()
	r.flushing[projectID] = true
	r.mu.Unlock()
	r.RunNow()
}

func (r *Runner) policy(only string) vault.SyncPolicy {
	return vault.SyncPolicy{
		Only: only,
		Open: func(id string) bool {
			r.mu.Lock()
			defer r.mu.Unlock()
			return r.active[id]
		},
		Busy: func(id string) bool {
			return r.Busy != nil && r.Busy(id)
		},
		Settled: func(id string) bool {
			r.mu.Lock()
			defer r.mu.Unlock()
			if r.flushing[id] {
				return true
			}
			at, edited := r.edited[id]
			return !edited || r.Now().Sub(at) >= SettleAfter
		},
	}
}

// Status is the last known state.
func (r *Runner) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.init()
	out := r.status
	out.Enabled = r.enabled()
	if !out.Enabled {
		out.State, out.Reason = StateOff, ""
	}
	return out
}

func (r *Runner) publish() {
	if r.Notify != nil {
		r.Notify(r.Status())
	}
}

func (r *Runner) setState(state string) {
	r.mu.Lock()
	r.status.State = state
	r.mu.Unlock()
	r.publish()
}

func (r *Runner) primary() (Target, bool) {
	if r.Targets == nil {
		return Target{}, false
	}
	for _, target := range r.Targets() {
		if target.Primary {
			return target, true
		}
	}
	return Target{}, false
}

// targetGit is vault.TargetGit, which has its own schedule.
const targetGit = vault.TargetGit

// due says a period has passed since id last ran.
func (r *Runner) due(id string, every time.Duration) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	last, ran := r.lastRun[id]
	return !ran || r.Now().Sub(last) >= every
}

func (r *Runner) ran(id string) {
	r.mu.Lock()
	r.lastRun[id] = r.Now()
	r.mu.Unlock()
}

// runExports keeps each folder or repository that is not a target on its daily
// Markdown export, as it always was.
func (r *Runner) runExports(ctx context.Context) {
	if r.Exports == nil {
		return
	}
	taken := map[string]bool{}
	if r.enabled() && r.Targets != nil {
		for _, target := range r.Targets() {
			taken[target.ID] = true
		}
	}
	for _, export := range r.Exports() {
		if taken[export.ID] || !r.due("export:"+export.ID, ExportEvery) {
			continue
		}
		r.ran("export:" + export.ID)
		if err := export.Run(ctx); err != nil {
			log.Printf("sync: export %s: %v", export.ID, err)
		}
	}
}

// Tick runs once. It sends nothing to a target while automatic sync is off.
func (r *Runner) Tick(ctx context.Context) (Status, error) {
	r.mu.Lock()
	r.init()
	r.mu.Unlock()
	r.runExports(ctx)
	if !r.enabled() {
		return r.Status(), nil
	}
	primary, ok := r.primary()
	if !ok {
		return r.Status(), nil
	}
	var err error
	r.mu.Lock()
	flushing := len(r.flushing) > 0
	r.mu.Unlock()
	if primary.ID != targetGit || flushing || r.due(primary.ID, GitPrimaryEvery) {
		started := r.Now()
		r.setState(StateRunning)
		r.ran(primary.ID)
		var report vault.SyncReport
		report, err = primary.Vault.SyncAll(ctx, r.policy(""))
		r.finish(started, report, err)
		r.publish()
	}
	r.runMirrors(ctx, false)
	return r.Status(), err
}

// runMirrors sends what changed to the mirrors. A mirror of Linetta Works or a
// folder goes when a work is left and on the way out, and a Git mirror once a
// day. One that fails says so and takes nothing else down with it.
func (r *Runner) runMirrors(ctx context.Context, last bool) {
	if r.Targets == nil {
		return
	}
	r.mu.Lock()
	closed := r.closed
	r.closed = false
	r.mu.Unlock()
	for _, target := range r.Targets() {
		if target.Primary {
			continue
		}
		due := closed || last
		if target.ID == targetGit {
			due = r.due(target.ID, GitMirrorEvery)
		}
		if !due {
			continue
		}
		r.ran(target.ID)
		policy := r.policy("")
		policy.Mirror = true
		_, err := target.Vault.SyncAll(ctx, policy)
		r.mu.Lock()
		state := r.mirrors[target.ID]
		state.Reason = reasonOf(err)
		if err == nil {
			state.LastSuccessAt = r.Now().UnixMilli()
		} else if state.Reason == "" {
			state.Reason = rpc.ReasonSyncTargetFailed
			log.Printf("sync: mirror %s: %v", target.ID, err)
		}
		r.mirrors[target.ID] = state
		r.mu.Unlock()
	}
}

// Mirrors is how each mirror's last run went.
func (r *Runner) Mirrors() map[string]MirrorStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.init()
	out := make(map[string]MirrorStatus, len(r.mirrors))
	for id, state := range r.mirrors {
		out[id] = state
	}
	return out
}

func (r *Runner) finish(started time.Time, report vault.SyncReport, err error) {
	code := reasonOf(err)
	if code == rpc.ReasonVaultInProgress {
		// A manual upload holds the lock; the next run does this one's work.
		r.mu.Lock()
		r.status.State = StateIdle
		r.mu.Unlock()
		return
	}
	now := r.Now()
	r.mu.Lock()
	r.status.Reason = code
	switch {
	case err == nil:
		r.failures = 0
		r.status.State = StateIdle
		r.status.LastSuccessAt = now.UnixMilli()
		r.status.Works = report.Works
		if len(report.Archived) > 0 {
			r.status.Archived = report.Archived
		}
		for id := range r.flushing {
			delete(r.flushing, id)
		}
		for id, at := range r.edited {
			if now.Sub(at) >= SettleAfter {
				delete(r.edited, id)
			}
		}
	case errors.Is(err, context.Canceled):
		r.status.State = StateIdle
	case stopReasons[code]:
		r.status.State = StateStopped
	default:
		r.failures++
		r.status.State = StateWaiting
		log.Printf("sync: %v", err)
	}
	last := r.status.LastSuccessAt
	r.mu.Unlock()
	if r.Record != nil && !errors.Is(err, context.Canceled) {
		r.Record(started, now, err == nil, code, last)
	}
}

func reasonOf(err error) string {
	var re *rpc.ReasonError
	if errors.As(err, &re) {
		return re.Reason
	}
	return ""
}

// flush is the last run as the engine shuts down: what the writer has not seen
// uploaded yet goes now, not when they next open the app.
func (r *Runner) flush() {
	if !r.enabled() {
		return
	}
	r.mu.Lock()
	pending := len(r.edited) > 0 || len(r.flushing) > 0
	for id := range r.edited {
		r.flushing[id] = true
	}
	r.mu.Unlock()
	if !pending {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	if primary, ok := r.primary(); ok {
		if _, err := primary.Vault.SyncAll(ctx, r.policy("")); err != nil {
			log.Printf("sync: last upload: %v", err)
		}
	}
	r.runMirrors(ctx, true)
}
