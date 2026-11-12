package publish

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
)

// What an agent asked to put on Linetta Works. Each is something readers see
// and nothing in the app can take back, so none of them happens on an
// agent's word alone (#238).
const (
	ApprovalRegisterWork   = "register_work"
	ApprovalPublishEpisode = "publish_episode"
	ApprovalPublishCover   = "publish_cover"
)

// DefaultApprovalTimeout is how long a publish tool waits for the writer. A
// call nobody answers is refused, so an agent working while the writer is
// away publishes nothing.
const DefaultApprovalTimeout = 2 * time.Minute

// ApprovalsChangedMethod is the notification the app's approval dialog
// listens on. Its params are approvalsChanged: every request still waiting.
const ApprovalsChangedMethod = "publish.approvals_changed"

var (
	// ErrApprovalDenied: the writer said no.
	ErrApprovalDenied = errors.New("the writer declined")
	// ErrApprovalTimeout: nobody answered in time.
	ErrApprovalTimeout = errors.New("the writer did not answer in time")
	// ErrApprovalUnknown: the answer names a request that is not waiting,
	// because it timed out, its caller gave up, or it was already answered.
	ErrApprovalUnknown = errors.New("no such approval request")
)

// ApprovalRequest is what the writer is shown. The tool fills in what it is
// about to send; Approvals sets ID and ExpiresAt.
type ApprovalRequest struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	ProjectID string `json:"project_id"`
	WorkTitle string `json:"work_title"`
	// NodeID and EpisodeTitle name the episode for publish_episode.
	NodeID       string `json:"node_id,omitempty"`
	EpisodeTitle string `json:"episode_title,omitempty"`
	// Republish is true when the episode is already on the site and its
	// text there would be replaced.
	Republish bool `json:"republish,omitempty"`
	// Blurb and SerialStatus are what register_work sends.
	Blurb        string `json:"blurb,omitempty"`
	SerialStatus string `json:"serial_status,omitempty"`
	// IllustrationID is the picture publish_cover uploads.
	IllustrationID string `json:"illustration_id,omitempty"`
	// Source is "agent" for the built-in panel and "external" for an MCP
	// client; Client is the name an external client gave itself, which it
	// chose, so it is shown as a label and never trusted.
	Source    string `json:"source"`
	Client    string `json:"client,omitempty"`
	ExpiresAt int64  `json:"expires_at"`
}

type approvalsChanged struct {
	Pending []ApprovalRequest `json:"pending"`
}

type pendingApproval struct {
	req    ApprovalRequest
	answer chan bool
}

// Approvals holds the publish requests waiting for the writer. A tool call
// blocks in Ask until the writer answers in the app, the call is cancelled,
// or the timeout passes.
type Approvals struct {
	mu      sync.Mutex
	pending []*pendingApproval
	notify  func(method string, params any)
	timeout time.Duration
	now     func() time.Time
}

// NewApprovals returns an empty queue. notify may be nil (tests); the
// requests then wait unseen until they time out.
func NewApprovals(notify func(method string, params any), timeout time.Duration) *Approvals {
	if timeout <= 0 {
		timeout = DefaultApprovalTimeout
	}
	return &Approvals{notify: notify, timeout: timeout, now: time.Now}
}

// Timeout is how long Ask waits.
func (a *Approvals) Timeout() time.Duration { return a.timeout }

// Ask puts req in front of the writer and waits. nil means approved;
// otherwise ErrApprovalDenied, ErrApprovalTimeout, or ctx's error.
func (a *Approvals) Ask(ctx context.Context, req ApprovalRequest) error {
	req.ID = uuid.NewString()
	req.ExpiresAt = a.now().Add(a.timeout).UnixMilli()
	p := &pendingApproval{req: req, answer: make(chan bool, 1)}

	a.mu.Lock()
	a.pending = append(a.pending, p)
	a.mu.Unlock()
	a.changed()

	timer := time.NewTimer(a.timeout)
	defer timer.Stop()
	var err error
	select {
	case ok := <-p.answer:
		if !ok {
			err = ErrApprovalDenied
		}
		// Answer already took it off the queue and announced it.
		return err
	case <-timer.C:
		err = ErrApprovalTimeout
	case <-ctx.Done():
		err = ctx.Err()
	}
	if a.remove(req.ID) != nil {
		a.changed()
		return err
	}
	// Answer won the race after all: honour what the writer said.
	if <-p.answer {
		return nil
	}
	return ErrApprovalDenied
}

// Answer settles one waiting request.
func (a *Approvals) Answer(id string, approved bool) error {
	p := a.remove(id)
	if p == nil {
		return ErrApprovalUnknown
	}
	p.answer <- approved
	a.changed()
	return nil
}

// Pending is every request still waiting, oldest first.
func (a *Approvals) Pending() []ApprovalRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]ApprovalRequest, 0, len(a.pending))
	for _, p := range a.pending {
		out = append(out, p.req)
	}
	return out
}

func (a *Approvals) remove(id string) *pendingApproval {
	a.mu.Lock()
	defer a.mu.Unlock()
	i := slices.IndexFunc(a.pending, func(p *pendingApproval) bool { return p.req.ID == id })
	if i < 0 {
		return nil
	}
	p := a.pending[i]
	a.pending = slices.Delete(a.pending, i, i+1)
	return p
}

func (a *Approvals) changed() {
	if a.notify == nil {
		return
	}
	a.notify(ApprovalsChangedMethod, approvalsChanged{Pending: a.Pending()})
}
