package publish

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type approvalNotes struct {
	mu   sync.Mutex
	seen [][]ApprovalRequest
	// next receives each non-empty list as it is announced.
	next chan []ApprovalRequest
}

func newApprovalNotes() *approvalNotes {
	return &approvalNotes{next: make(chan []ApprovalRequest, 8)}
}

func (n *approvalNotes) notify(method string, params any) {
	if method != ApprovalsChangedMethod {
		return
	}
	pending := params.(approvalsChanged).Pending
	n.mu.Lock()
	n.seen = append(n.seen, pending)
	n.mu.Unlock()
	if len(pending) > 0 {
		n.next <- pending
	}
}

func (n *approvalNotes) last() []ApprovalRequest {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.seen[len(n.seen)-1]
}

func askAsync(a *Approvals, ctx context.Context, req ApprovalRequest) <-chan error {
	done := make(chan error, 1)
	go func() { done <- a.Ask(ctx, req) }()
	return done
}

func waitErr(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Ask did not return")
		return nil
	}
}

func TestApprovals_approvedRequestReturnsNilAndLeavesTheQueue(t *testing.T) {
	notes := newApprovalNotes()
	a := NewApprovals(notes.notify, time.Minute)
	done := askAsync(a, context.Background(), ApprovalRequest{Kind: ApprovalPublishEpisode, ProjectID: "p", NodeID: "n"})

	shown := <-notes.next
	if len(shown) != 1 || shown[0].ID == "" || shown[0].Kind != ApprovalPublishEpisode || shown[0].ExpiresAt == 0 {
		t.Fatalf("announced %+v, want one publish_episode request with an id and expiry", shown)
	}
	if err := a.Answer(shown[0].ID, true); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if err := waitErr(t, done); err != nil {
		t.Fatalf("Ask = %v, want nil", err)
	}
	if got := a.Pending(); len(got) != 0 {
		t.Fatalf("Pending after answer = %+v", got)
	}
	if got := notes.last(); len(got) != 0 {
		t.Fatalf("last announcement = %+v, want an empty queue so the dialog closes", got)
	}
}

func TestApprovals_deniedRequestIsRefused(t *testing.T) {
	notes := newApprovalNotes()
	a := NewApprovals(notes.notify, time.Minute)
	done := askAsync(a, context.Background(), ApprovalRequest{Kind: ApprovalRegisterWork, ProjectID: "p"})
	shown := <-notes.next
	if err := a.Answer(shown[0].ID, false); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if err := waitErr(t, done); !errors.Is(err, ErrApprovalDenied) {
		t.Fatalf("Ask = %v, want ErrApprovalDenied", err)
	}
}

func TestApprovals_unansweredRequestTimesOutAndCannotBeApprovedLater(t *testing.T) {
	notes := newApprovalNotes()
	a := NewApprovals(notes.notify, 20*time.Millisecond)
	done := askAsync(a, context.Background(), ApprovalRequest{Kind: ApprovalPublishCover, ProjectID: "p"})
	shown := <-notes.next
	if err := waitErr(t, done); !errors.Is(err, ErrApprovalTimeout) {
		t.Fatalf("Ask = %v, want ErrApprovalTimeout", err)
	}
	if err := a.Answer(shown[0].ID, true); !errors.Is(err, ErrApprovalUnknown) {
		t.Fatalf("late Answer = %v, want ErrApprovalUnknown", err)
	}
	if got := notes.last(); len(got) != 0 {
		t.Fatalf("last announcement = %+v, want the timed-out request gone", got)
	}
}

func TestApprovals_cancelledCallerWithdrawsTheRequest(t *testing.T) {
	notes := newApprovalNotes()
	a := NewApprovals(notes.notify, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	done := askAsync(a, ctx, ApprovalRequest{Kind: ApprovalPublishEpisode, ProjectID: "p"})
	<-notes.next
	cancel()
	if err := waitErr(t, done); !errors.Is(err, context.Canceled) {
		t.Fatalf("Ask = %v, want context.Canceled", err)
	}
	if got := a.Pending(); len(got) != 0 {
		t.Fatalf("Pending after cancel = %+v", got)
	}
}

func TestApprovals_answersOnlyTheNamedRequest(t *testing.T) {
	notes := newApprovalNotes()
	a := NewApprovals(notes.notify, time.Minute)
	first := askAsync(a, context.Background(), ApprovalRequest{Kind: ApprovalPublishEpisode, ProjectID: "p", NodeID: "1"})
	<-notes.next
	second := askAsync(a, context.Background(), ApprovalRequest{Kind: ApprovalPublishEpisode, ProjectID: "p", NodeID: "2"})
	both := <-notes.next
	if len(both) != 2 || both[0].NodeID != "1" || both[1].NodeID != "2" {
		t.Fatalf("queue = %+v, want both requests oldest first", both)
	}
	if err := a.Answer(both[1].ID, true); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if err := waitErr(t, second); err != nil {
		t.Fatalf("second Ask = %v", err)
	}
	if got := a.Pending(); len(got) != 1 || got[0].NodeID != "1" {
		t.Fatalf("Pending = %+v, want the first request still waiting", got)
	}
	if err := a.Answer(both[0].ID, false); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if err := waitErr(t, first); !errors.Is(err, ErrApprovalDenied) {
		t.Fatalf("first Ask = %v", err)
	}
}
