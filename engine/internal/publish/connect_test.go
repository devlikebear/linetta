package publish

import (
	"context"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

func newTestSettings(t *testing.T) *settings.Store {
	t.Helper()
	s, err := settings.NewForHomeWithSecretStore(t.TempDir(), settings.NewMemorySecretStore())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newTestConnector(t *testing.T, site *fakeSite, creds Credentials, onSwitch func(context.Context) error) *Connector {
	t.Helper()
	c := NewConnector(site.client(), creds, onSwitch)
	c.pollFloor = time.Millisecond
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// waitFor polls Status until it leaves "waiting".
func waitFor(t *testing.T, c *Connector) ConnectStatus {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st := c.Status(); st.State != StateWaiting {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the connection attempt never finished")
	return ConnectStatus{}
}

func TestConnect_approvedStoresTheTokenAndReportsThePenName(t *testing.T) {
	site := newFakeSite(t)
	site.poll = []string{PollPending, PollPending, PollApproved}
	st := newTestSettings(t)
	c := newTestConnector(t, site, NewCredentials(st), nil)

	started, err := c.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if started.State != StateWaiting || started.ConfirmCode != "NWCQ-BKWX" || started.VerifyURL == "" {
		t.Fatalf("start = %+v", started)
	}
	got := waitFor(t, c)
	if got.State != StateConnected || got.PenName != "마곰" || got.Handle != "devlikebear" {
		t.Fatalf("status = %+v", got)
	}
	if st.PublishToken() != fakeToken {
		t.Fatal("token not stored")
	}
	if acc, ok := st.PublishAccount(); !ok || acc.SiteURL != site.srv.URL {
		t.Fatalf("account = %+v", acc)
	}
}

func TestConnect_deniedAndExpiredEndFailedWithoutAToken(t *testing.T) {
	for _, answer := range []string{PollDenied, PollExpired} {
		site := newFakeSite(t)
		site.poll = []string{answer}
		st := newTestSettings(t)
		c := newTestConnector(t, site, NewCredentials(st), nil)
		if _, err := c.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		got := waitFor(t, c)
		want := map[string]string{PollDenied: FailureDenied, PollExpired: FailureExpired}[answer]
		if got.State != StateFailed || got.FailureReason != want {
			t.Fatalf("%s: status = %+v", answer, got)
		}
		if st.PublishTokenExists() {
			t.Fatalf("%s: a token was stored", answer)
		}
	}
}

func TestConnect_theSitesWindowRunningOutIsExpired(t *testing.T) {
	site := newFakeSite(t)
	site.poll = []string{PollPending}
	c := newTestConnector(t, site, NewCredentials(newTestSettings(t)), nil)
	if _, err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Shrink the attempt's window to a moment: the site's ten minutes are
	// not what is under test.
	c.mu.Lock()
	att := c.current
	c.mu.Unlock()
	att.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	att2 := &pairAttempt{pairing: att.pairing, cancel: cancel}
	c.mu.Lock()
	c.current = att2
	c.mu.Unlock()
	c.wait(ctx, att2)
	if got := c.Status(); got.State != StateFailed || got.FailureReason != FailureExpired {
		t.Fatalf("status = %+v", got)
	}
}

func TestConnect_cancelStopsWaitingAndASecondStartReplacesTheFirst(t *testing.T) {
	site := newFakeSite(t)
	site.poll = []string{PollPending}
	st := newTestSettings(t)
	c := newTestConnector(t, site, NewCredentials(st), nil)
	if _, err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	first := c.current
	c.mu.Unlock()
	if _, err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	second := c.current
	c.mu.Unlock()
	if first == second || second == nil {
		t.Fatal("the second Start did not replace the first")
	}
	c.Cancel()
	if got := c.Status(); got.State != StateDisconnected {
		t.Fatalf("after cancel: %+v", got)
	}
	// An approval landing for the cancelled attempt must not connect.
	c.approve(context.Background(), second, PollResult{Status: PollApproved, Token: "lwk_late", Handle: "x"})
	if st.PublishTokenExists() {
		t.Fatal("a cancelled attempt stored a token")
	}
}

func TestConnect_startFailingIsAReason(t *testing.T) {
	site := newFakeSite(t)
	site.failNext("POST /api/artworks/pair/start", 429, "")
	c := newTestConnector(t, site, NewCredentials(newTestSettings(t)), nil)
	if _, err := c.Start(context.Background()); reasonOf(err) != rpc.ReasonPublishRateLimited {
		t.Fatalf("err = %v", err)
	}
}

func TestConnect_switchingAccountsClearsTheOldState(t *testing.T) {
	site := newFakeSite(t)
	st := newTestSettings(t)
	if err := st.SetPublishAccount(&settings.PublishAccount{SiteURL: site.srv.URL, Handle: "someone-else"}); err != nil {
		t.Fatal(err)
	}
	cleared := false
	c := newTestConnector(t, site, NewCredentials(st), func(context.Context) error { cleared = true; return nil })
	if _, err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := waitFor(t, c); got.State != StateConnected {
		t.Fatalf("status = %+v", got)
	}
	if !cleared {
		t.Fatal("a different account kept the previous account's state")
	}
}

func TestConnect_disconnectAndRevoked(t *testing.T) {
	site := newFakeSite(t)
	st := newTestSettings(t)
	c := newTestConnector(t, site, NewCredentials(st), nil)
	if _, err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, c)
	if err := c.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if got := c.Status(); got.State != StateDisconnected || st.PublishTokenExists() {
		t.Fatalf("after disconnect: %+v", got)
	}

	if _, err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, c)
	c.Revoked()
	if got := c.Status(); got.State != StateFailed || got.FailureReason != FailureRevoked {
		t.Fatalf("after revoked: %+v", got)
	}
	if st.PublishTokenExists() {
		t.Fatal("a revoked token was kept")
	}
}

func TestConnect_statusNeverCarriesTheSecret(t *testing.T) {
	site := newFakeSite(t)
	site.poll = []string{PollPending}
	c := newTestConnector(t, site, NewCredentials(newTestSettings(t)), nil)
	st, err := c.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.ConfirmCode == "secret" || st.VerifyURL == "secret" {
		t.Fatalf("status leaked the device secret: %+v", st)
	}
}
