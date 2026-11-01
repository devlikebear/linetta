//go:build !mobile

package syncauto

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/vault"
)

type fakeVault struct {
	mu     sync.Mutex
	calls  []vault.SyncPolicy
	err    error
	report vault.SyncReport
	onCall func(vault.SyncPolicy)
}

func (f *fakeVault) SyncAll(_ context.Context, policy vault.SyncPolicy) (vault.SyncReport, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, policy)
	if f.onCall != nil {
		f.onCall(policy)
	}
	return f.report, f.err
}

func (f *fakeVault) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newRunner(v *fakeVault, c *clock) *Runner {
	return &Runner{
		Vault:   v,
		Enabled: func() bool { return true },
		Now:     c.Now,
		Random:  func() time.Duration { return 0 },
		Sleep: func(ctx context.Context, _ time.Duration) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
}

func reasonErr(code string) error {
	return &rpc.ReasonError{Reason: code, Err: errors.New(code)}
}

func newClock() *clock { return &clock{now: time.UnixMilli(1_800_000_000_000)} }

func TestOffSendsNothing(t *testing.T) {
	v := &fakeVault{}
	r := newRunner(v, newClock())
	r.Enabled = func() bool { return false }
	status, err := r.Tick(context.Background())
	if err != nil || status.State != StateOff || status.Enabled {
		t.Fatalf("status %+v, %v", status, err)
	}
	r.Touch("p")
	if _, err := r.Open(context.Background(), "p"); err != nil {
		t.Fatal(err)
	}
	r.Close("p")
	r.flush()
	if v.count() != 0 {
		t.Fatalf("%d calls while off", v.count())
	}
}

func TestSuccessKeepsTheTimeAndTheWorks(t *testing.T) {
	c := newClock()
	v := &fakeVault{report: vault.SyncReport{Works: []vault.WorkSync{{ProjectID: "p", State: vault.StateSynced}}, Archived: []string{"gone"}}}
	r := newRunner(v, c)
	var seen []string
	r.Notify = func(s Status) { seen = append(seen, s.State) }
	status, err := r.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.State != StateIdle || status.LastSuccessAt != c.Now().UnixMilli() || len(status.Works) != 1 || status.Archived[0] != "gone" {
		t.Fatalf("status %+v", status)
	}
	if len(seen) != 2 || seen[0] != StateRunning || seen[1] != StateIdle {
		t.Fatalf("notified %v", seen)
	}
}

func TestEveryStopConditionStopsAndSaysWhy(t *testing.T) {
	for code := range stopReasons {
		v := &fakeVault{err: reasonErr(code)}
		r := newRunner(v, newClock())
		status, _ := r.Tick(context.Background())
		if status.State != StateStopped || status.Reason != code {
			t.Errorf("%s: status %+v", code, status)
		}
		if got := r.nextWait(); got != MaxBackoff {
			t.Errorf("%s: waits %v", code, got)
		}
	}
}

func TestStoppedRecoversOnTheNextGoodRun(t *testing.T) {
	v := &fakeVault{err: reasonErr(rpc.ReasonVaultTermsOutdated)}
	r := newRunner(v, newClock())
	_, _ = r.Tick(context.Background())
	v.err = nil
	status, _ := r.Tick(context.Background())
	if status.State != StateIdle || status.Reason != "" {
		t.Fatalf("status %+v", status)
	}
}

func TestOfflineWaitsQuietlyAndKeepsTheLastSuccess(t *testing.T) {
	c := newClock()
	v := &fakeVault{}
	r := newRunner(v, c)
	_, _ = r.Tick(context.Background())
	last := r.Status().LastSuccessAt
	c.add(time.Hour)
	v.err = reasonErr(rpc.ReasonVaultUnreachable)
	status, _ := r.Tick(context.Background())
	if status.State != StateWaiting || status.LastSuccessAt != last {
		t.Fatalf("status %+v", status)
	}
}

func TestFailuresBackOffUpToThirtyMinutes(t *testing.T) {
	v := &fakeVault{err: reasonErr(rpc.ReasonVaultUnreachable)}
	r := newRunner(v, newClock())
	for _, want := range []time.Duration{10 * time.Minute, 20 * time.Minute, 30 * time.Minute, 30 * time.Minute, 30 * time.Minute} {
		_, _ = r.Tick(context.Background())
		if got := r.nextWait(); got != want {
			t.Fatalf("waits %v, want %v", got, want)
		}
	}
	v.err = nil
	_, _ = r.Tick(context.Background())
	if got := r.nextWait(); got != Period {
		t.Fatalf("waits %v after recovering", got)
	}
}

func TestJitterMovesThePeriod(t *testing.T) {
	r := newRunner(&fakeVault{}, newClock())
	r.Random = func() time.Duration { return -30 * time.Second }
	if got := r.nextWait(); got != Period-30*time.Second {
		t.Fatalf("waits %v", got)
	}
	r.Random = nil
	for range 200 {
		got := r.nextWait()
		if got < Period-Jitter || got > Period+Jitter {
			t.Fatalf("waits %v", got)
		}
	}
}

func TestAnEditHoldsTheUploadThreeMinutes(t *testing.T) {
	c := newClock()
	v := &fakeVault{}
	r := newRunner(v, c)
	r.Touch("p")
	settled := r.policy("").Settled
	if settled("p") {
		t.Fatal("settled at once")
	}
	if !settled("other") {
		t.Fatal("a work nobody touched has not settled")
	}
	if got := r.nextWait(); got != SettleAfter {
		t.Fatalf("waits %v", got)
	}
	c.add(2 * time.Minute)
	r.Touch("p")
	c.add(2 * time.Minute)
	if settled("p") {
		t.Fatal("settled while the writer kept going")
	}
	if got := r.nextWait(); got != time.Minute {
		t.Fatalf("waits %v", got)
	}
	c.add(time.Minute)
	if !settled("p") {
		t.Fatal("did not settle")
	}
	if _, err := r.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := r.nextWait(); got != Period {
		t.Fatalf("the settled edit still shortens the wait: %v", got)
	}
}

func TestOpenTakesTheWorkThenMarksItActive(t *testing.T) {
	v := &fakeVault{report: vault.SyncReport{Pulled: []string{"p"}}}
	r := newRunner(v, newClock())
	var wasOpen bool
	v.onCall = func(p vault.SyncPolicy) { wasOpen = p.Open("p") }
	pulled, err := r.Open(context.Background(), "p")
	if err != nil || !pulled {
		t.Fatalf("pulled %v, %v", pulled, err)
	}
	if wasOpen {
		t.Fatal("the work counted as open during its own pull")
	}
	if v.calls[0].Only != "p" {
		t.Fatalf("only = %q", v.calls[0].Only)
	}
	if !r.policy("").Open("p") {
		t.Fatal("not active after opening")
	}
}

func TestOpenSurvivesAFailure(t *testing.T) {
	v := &fakeVault{err: reasonErr(rpc.ReasonVaultUnreachable)}
	r := newRunner(v, newClock())
	if _, err := r.Open(context.Background(), "p"); err == nil {
		t.Fatal("the failure was hidden from the caller")
	}
	if !r.policy("").Open("p") {
		t.Fatal("a failed pull left the work closed")
	}
}

func TestCloseSendsWithoutWaitingToSettle(t *testing.T) {
	c := newClock()
	v := &fakeVault{}
	r := newRunner(v, c)
	r.Touch("p")
	if _, err := r.Open(context.Background(), "p"); err != nil {
		t.Fatal(err)
	}
	r.Close("p")
	policy := r.policy("")
	if policy.Open("p") || !policy.Settled("p") {
		t.Fatal("closing did not release the work")
	}
	if _, err := r.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.flushing) != 0 {
		t.Fatal("the flush stayed")
	}
}

func TestFlushOnShutdownSendsWhatIsWaiting(t *testing.T) {
	v := &fakeVault{}
	r := newRunner(v, newClock())
	r.flush()
	if v.count() != 0 {
		t.Fatal("nothing to send, and it ran anyway")
	}
	r.init()
	r.Touch("p")
	var settled bool
	v.onCall = func(p vault.SyncPolicy) { settled = p.Settled("p") }
	r.flush()
	if !settled {
		t.Fatal("the last upload waited for an edit to settle")
	}
}

func TestManualOperationSkipsTheRunQuietly(t *testing.T) {
	v := &fakeVault{err: reasonErr(rpc.ReasonVaultInProgress)}
	r := newRunner(v, newClock())
	status, _ := r.Tick(context.Background())
	if status.State != StateIdle || r.failures != 0 {
		t.Fatalf("status %+v, failures %d", status, r.failures)
	}
}

func TestBusyAsksTheAgent(t *testing.T) {
	r := newRunner(&fakeVault{}, newClock())
	r.Busy = func(id string) bool { return id == "p" }
	policy := r.policy("")
	if !policy.Busy("p") || policy.Busy("q") {
		t.Fatal("busy did not reach the policy")
	}
}

func TestStartRunsAtOnceAndWakesAndStops(t *testing.T) {
	c := newClock()
	v := &fakeVault{}
	r := newRunner(v, c)
	ran := make(chan struct{}, 8)
	v.onCall = func(vault.SyncPolicy) { ran <- struct{}{} }
	stop := r.Start(context.Background())
	<-ran
	r.RunNow()
	<-ran
	stop()
}

func TestLastSuccessSurvivesARestart(t *testing.T) {
	r := newRunner(&fakeVault{}, newClock())
	r.LastSuccess = func() int64 { return 42 }
	if got := r.Status().LastSuccessAt; got != 42 {
		t.Fatalf("last = %d", got)
	}
}

func TestRecordKeepsEveryRealRun(t *testing.T) {
	v := &fakeVault{}
	r := newRunner(v, newClock())
	var oks []bool
	r.Record = func(_, _ time.Time, ok bool, _ string, _ int64) { oks = append(oks, ok) }
	_, _ = r.Tick(context.Background())
	v.err = reasonErr(rpc.ReasonVaultUnreachable)
	_, _ = r.Tick(context.Background())
	v.err = reasonErr(rpc.ReasonVaultInProgress)
	_, _ = r.Tick(context.Background())
	if len(oks) != 2 || !oks[0] || oks[1] {
		t.Fatalf("recorded %v", oks)
	}
}
