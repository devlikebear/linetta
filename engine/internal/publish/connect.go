package publish

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

// Connection states the settings pane shows.
const (
	StateDisconnected = "disconnected"
	StateWaiting      = "waiting"
	StateConnected    = "connected"
	StateFailed       = "failed"
)

// Why the last connection attempt did not end connected.
const (
	FailureDenied   = "denied"  // the writer pressed "deny" in the browser
	FailureExpired  = "expired" // ten minutes passed without an answer
	FailureNetwork  = "network" // the site stopped answering while we waited
	FailureClaimed  = "claimed" // the token was already handed out (410)
	FailureStore    = "store"   // the token could not be saved on this device
	FailureFailed   = "failed"  // anything else the site said no with
	FailureRevoked  = "revoked" // the site stopped accepting the stored token
	maxPollFailures = 5
)

// ConnectStatus is what the pane shows. It never carries the token or the
// device secret.
type ConnectStatus struct {
	State         string `json:"state"`
	ConfirmCode   string `json:"confirm_code,omitempty"`
	VerifyURL     string `json:"verify_url,omitempty"`
	ExpiresAt     int64  `json:"expires_at,omitempty"`
	PenName       string `json:"pen_name,omitempty"`
	Handle        string `json:"handle,omitempty"`
	SiteURL       string `json:"site_url"`
	FailureReason string `json:"failure_reason,omitempty"`
}

type pairAttempt struct {
	pairing   Pairing
	expiresAt time.Time
	cancel    context.CancelFunc
}

// Connector runs the device-pairing flow (docs/api.md, "기기 연결"). It is
// shaped like codexauth.Service: Start answers at once with what the writer
// needs to see, the waiting happens in the background against a deadline,
// Status never returns a credential, and a second Start replaces the first.
// Unlike the Codex login nothing listens locally — the app polls pair/poll
// with the secret pair/start gave it until the writer answers in the browser.
type Connector struct {
	client     *Client
	creds      Credentials
	onSwitch   func(ctx context.Context) error
	now        func() time.Time
	deviceName string

	mu          sync.Mutex
	current     *pairAttempt
	lastFailure string
	// pollFloor keeps tests from waiting whole seconds between polls.
	pollFloor time.Duration
}

// NewConnector returns a Connector. onSwitch runs when the account approved
// is not the one this device was connected to before, so whatever the old
// account's works looked like is not shown as this one's.
func NewConnector(client *Client, creds Credentials, onSwitch func(ctx context.Context) error) *Connector {
	return &Connector{
		client:     client,
		creds:      creds,
		onSwitch:   onSwitch,
		now:        time.Now,
		deviceName: defaultDeviceName(),
		pollFloor:  time.Second,
	}
}

// defaultDeviceName is what the writer will see in their profile's device
// list on the site, so they can tell this computer from another.
func defaultDeviceName() string {
	host, err := os.Hostname()
	host = strings.TrimSuffix(strings.TrimSpace(host), ".local")
	if err != nil || host == "" {
		return "Linetta"
	}
	name := "Linetta (" + host + ")"
	if runes := []rune(name); len(runes) > 100 {
		name = string(runes[:100])
	}
	return name
}

// Start asks the site for a pairing and begins waiting for the writer to
// approve it. The returned status carries the confirm code to show and the
// URL to open in the browser.
func (c *Connector) Start(ctx context.Context) (ConnectStatus, error) {
	c.stopCurrent()
	c.mu.Lock()
	c.lastFailure = ""
	c.mu.Unlock()

	p, err := c.client.PairStart(ctx, c.deviceName)
	if err != nil {
		return ConnectStatus{}, classify(err)
	}
	if p.PairingID == "" || p.DeviceSecret == "" || p.VerifyURL == "" {
		return ConnectStatus{}, reason(rpc.ReasonPublishRejected, errors.New("publish: pair/start answered without a pairing"))
	}
	expiresIn := time.Duration(p.ExpiresIn) * time.Second
	if expiresIn <= 0 {
		expiresIn = 10 * time.Minute
	}
	// The window is the site's, not the request's: like the Codex login it
	// is derived from Background so a per-request context cannot cut the ten
	// minutes a writer has to find their browser down to one RPC.
	runCtx, cancel := context.WithTimeout(context.Background(), expiresIn)
	att := &pairAttempt{pairing: p, expiresAt: c.now().Add(expiresIn), cancel: cancel}
	c.mu.Lock()
	c.current = att
	c.mu.Unlock()
	go c.wait(runCtx, att)
	return c.Status(), nil
}

func (c *Connector) wait(ctx context.Context, att *pairAttempt) {
	defer att.cancel()
	interval := time.Duration(att.pairing.Interval) * time.Second
	if interval < c.pollFloor {
		interval = c.pollFloor
	}
	failures := 0
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				c.finish(att, FailureExpired)
			}
			return
		case <-time.After(interval):
		}
		res, err := c.client.PairPoll(ctx, att.pairing.PairingID, att.pairing.DeviceSecret)
		if err != nil {
			if ctx.Err() != nil {
				continue // the select above reports it
			}
			switch {
			case IsCode(err, CodeAlreadyClaimed):
				c.finish(att, FailureClaimed)
				return
			case isOffline(err):
				// A dropped request or a busy site is not an answer. Keep
				// waiting, but not forever on a network that is gone.
				failures++
				if failures >= maxPollFailures {
					c.finish(att, FailureNetwork)
					return
				}
				continue
			default:
				log.Printf("publish: pair/poll: %v", err)
				c.finish(att, FailureFailed)
				return
			}
		}
		failures = 0
		switch res.Status {
		case PollPending:
			continue
		case PollDenied:
			c.finish(att, FailureDenied)
			return
		case PollExpired:
			c.finish(att, FailureExpired)
			return
		case PollApproved:
			c.approve(ctx, att, res)
			return
		default:
			c.finish(att, FailureFailed)
			return
		}
	}
}

// approve stores the token the site just handed over. The site hands it out
// once, so a failure to store it means this attempt is spent.
func (c *Connector) approve(ctx context.Context, att *pairAttempt, res PollResult) {
	if res.Token == "" {
		c.finish(att, FailureFailed)
		return
	}
	acc := settings.PublishAccount{
		SiteURL:     c.client.BaseURL(),
		Handle:      res.Handle,
		PenName:     res.PenName,
		ConnectedAt: c.now().UnixMilli(),
		ConsentedAt: c.now().UnixMilli(),
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current != att {
		return // cancelled or replaced while the poll was in flight
	}
	prev, had := c.creds.Account()
	if err := c.creds.Save(acc, res.Token); err != nil {
		log.Printf("publish: store token: %v", err)
		c.current, c.lastFailure = nil, FailureStore
		return
	}
	if had && (prev.SiteURL != acc.SiteURL || prev.Handle != acc.Handle) && c.onSwitch != nil {
		if err := c.onSwitch(ctx); err != nil {
			log.Printf("publish: clear the previous account's state: %v", err)
		}
	}
	c.current, c.lastFailure = nil, ""
}

func (c *Connector) finish(att *pairAttempt, failure string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current != att {
		return
	}
	c.current, c.lastFailure = nil, failure
}

// Status reports the connection without reading the token's value.
func (c *Connector) Status() ConnectStatus {
	c.mu.Lock()
	att, failure := c.current, c.lastFailure
	c.mu.Unlock()
	st := ConnectStatus{SiteURL: c.client.BaseURL()}
	if att != nil {
		st.State = StateWaiting
		st.ConfirmCode = att.pairing.ConfirmCode
		st.VerifyURL = att.pairing.VerifyURL
		st.ExpiresAt = att.expiresAt.UnixMilli()
		return st
	}
	if acc, ok := c.creds.Account(); ok && c.creds.HasToken() {
		st.State = StateConnected
		st.PenName, st.Handle = acc.PenName, acc.Handle
		return st
	}
	if failure != "" {
		st.State, st.FailureReason = StateFailed, failure
		return st
	}
	st.State = StateDisconnected
	return st
}

// Connected reports whether a token is stored, without reading it.
func (c *Connector) Connected() bool {
	_, ok := c.creds.Account()
	return ok && c.creds.HasToken()
}

// Cancel abandons the attempt in flight. The pairing on the site expires on
// its own.
func (c *Connector) Cancel() {
	c.stopCurrent()
	c.mu.Lock()
	c.lastFailure = ""
	c.mu.Unlock()
}

// Disconnect forgets the connection on this device. The device stays listed
// on the site until the writer removes it from their profile there; the pane
// says so.
func (c *Connector) Disconnect() error {
	c.Cancel()
	return c.creds.Clear()
}

// Revoked is called when the site refuses the stored token: the writer
// disconnected this device from the web. The token is forgotten so the pane
// shows a reconnect prompt instead of "connected".
func (c *Connector) Revoked() {
	if err := c.creds.Clear(); err != nil {
		log.Printf("publish: forget a revoked token: %v", err)
	}
	c.mu.Lock()
	if c.current == nil {
		c.lastFailure = FailureRevoked
	}
	c.mu.Unlock()
}

// Close stops an attempt still waiting.
func (c *Connector) Close() error {
	c.stopCurrent()
	return nil
}

func (c *Connector) stopCurrent() {
	c.mu.Lock()
	att := c.current
	c.current = nil
	c.mu.Unlock()
	if att != nil {
		att.cancel()
	}
}
