package codexauth

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// tokenServer stands in for https://auth.openai.com/oauth/token. It records
// the form it was posted so the test can assert the PKCE verifier travelled.
type tokenServer struct {
	*httptest.Server
	gotForm url.Values
	status  int
	body    string
}

func newTokenServer(t *testing.T, idToken string) *tokenServer {
	t.Helper()
	ts := &tokenServer{status: http.StatusOK}
	ts.body = `{"access_token":"acc","refresh_token":"ref","id_token":"` + idToken + `"}`
	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ts.gotForm, _ = url.ParseQuery(string(body))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(ts.status)
		_, _ = io.WriteString(w, ts.body)
	}))
	t.Cleanup(ts.Close)
	return ts
}

// follow performs the browser's half: GET the callback with the code and state
// the issuer would have sent back.
func follow(t *testing.T, authURL, code string, overrideState string) *http.Response {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth url: %v", err)
	}
	q := u.Query()
	state := q.Get("state")
	if overrideState != "" {
		state = overrideState
	}
	redirect, err := url.Parse(q.Get("redirect_uri"))
	if err != nil {
		t.Fatalf("parse redirect uri: %v", err)
	}
	cb := *redirect
	cbq := url.Values{}
	cbq.Set("code", code)
	cbq.Set("state", state)
	cb.RawQuery = cbq.Encode()

	res, err := http.Get(cb.String())
	if err != nil {
		t.Fatalf("callback GET: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

// followAsync performs the same callback GET as follow, but — unlike follow —
// never fails the test on a transport error. It exists for a caller that
// launches the GET in its own goroutine while the exchange is deliberately
// blocked: a connection reset or refusal is one of the valid outcomes being
// tested for there, and t.Fatalf from a background goroutine only aborts
// that goroutine (via runtime.Goexit) — it never sends anything to a channel
// a caller might be blocked reading, which hangs the test instead of failing
// it cleanly.
func followAsync(authURL, code, overrideState string) (*http.Response, error) {
	u, err := url.Parse(authURL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	state := q.Get("state")
	if overrideState != "" {
		state = overrideState
	}
	redirect, err := url.Parse(q.Get("redirect_uri"))
	if err != nil {
		return nil, err
	}
	cb := *redirect
	cbq := url.Values{}
	cbq.Set("code", code)
	cbq.Set("state", state)
	cb.RawQuery = cbq.Encode()
	return http.Get(cb.String())
}

func newService(t *testing.T, idToken string) (*Service, *tokenServer, string) {
	t.Helper()
	home := t.TempDir()
	ts := newTokenServer(t, idToken)
	svc := NewService(home).WithTokenURL(ts.URL).WithHTTPClient(ts.Client())
	t.Cleanup(func() { _ = svc.Close() })
	return svc, ts, home
}

func TestStart_completesTheLoginAndWritesTheCredential(t *testing.T) {
	idToken := fakeIDToken(t, "writer@example.com", "acct-42", 1788000000)
	svc, ts, home := newService(t, idToken)

	authURL, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	res := follow(t, authURL, "the-code", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("callback status = %d, want 200", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "Linetta") {
		t.Errorf("success page does not mention Linetta: %s", body)
	}

	// The credential must land before Status reports a login.
	deadline := time.Now().Add(5 * time.Second)
	var st Status
	for time.Now().Before(deadline) {
		if st = svc.Status(); st.LoggedIn {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !st.LoggedIn {
		t.Fatal("Status still reports logged out after a successful callback")
	}
	if st.Email != "writer@example.com" || st.AccountID != "acct-42" || st.ExpiresAt != 1788000000 {
		t.Errorf("status = %+v", st)
	}

	// The exchange must carry the PKCE verifier and the registered redirect.
	if got := ts.gotForm.Get("grant_type"); got != "authorization_code" {
		t.Errorf("grant_type = %q", got)
	}
	if got := ts.gotForm.Get("code"); got != "the-code" {
		t.Errorf("code = %q", got)
	}
	if got := ts.gotForm.Get("client_id"); got != ClientID {
		t.Errorf("client_id = %q", got)
	}
	verifier := ts.gotForm.Get("code_verifier")
	if len(verifier) < 43 {
		t.Errorf("code_verifier = %q, want the PKCE verifier", verifier)
	}
	authQuery, _ := url.Parse(authURL)
	if got, want := ts.gotForm.Get("redirect_uri"), authQuery.Query().Get("redirect_uri"); got != want {
		t.Errorf("redirect_uri = %q, want the one sent to authorize (%q)", got, want)
	}

	// And the file must be the Codex CLI's shape, with the account id lifted
	// out of the id_token.
	f, err := readAuthFile(home)
	if err != nil {
		t.Fatalf("readAuthFile: %v", err)
	}
	if f.Tokens.AccessToken != "acc" || f.Tokens.RefreshToken != "ref" || f.Tokens.AccountID != "acct-42" {
		t.Errorf("stored tokens = %+v", f.Tokens)
	}
}

func TestStart_rejectsACallbackWithTheWrongState(t *testing.T) {
	svc, _, home := newService(t, fakeIDToken(t, "a@b.c", "acct", 1))
	authURL, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	res := follow(t, authURL, "the-code", "forged-state")
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a mismatched state", res.StatusCode)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := readAuthFile(home); err == nil {
		t.Fatal("a forged callback wrote a credential")
	}
	if svc.Status().LoggedIn {
		t.Fatal("a forged callback produced a login")
	}
}

func TestStart_reportsAnIssuerRejection(t *testing.T) {
	svc, ts, home := newService(t, "")
	ts.status = http.StatusBadRequest
	ts.body = `{"error":"invalid_grant"}`

	authURL, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	res := follow(t, authURL, "stale-code", "")
	if res.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502 when the exchange fails", res.StatusCode)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := readAuthFile(home); err == nil {
		t.Fatal("a failed exchange wrote a credential")
	}
}

// Status is the pane's only window onto a login attempt. Without a way to
// report "the last attempt failed," a refused exchange is indistinguishable
// from a writer who is still sitting in the browser (#92 review, finding 1).
func TestStatus_reportsAFailedExchangeUntilTheNextStart(t *testing.T) {
	svc, ts, _ := newService(t, "")
	ts.status = http.StatusBadRequest
	ts.body = `{"error":"invalid_grant"}`

	authURL, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	res := follow(t, authURL, "stale-code", "")
	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("callback status = %d, want 502", res.StatusCode)
	}

	// The failure is recorded inside the callback handler before it writes
	// the response, but poll rather than assume: the assertion should not be
	// coupled to exactly when that happens.
	deadline := time.Now().Add(5 * time.Second)
	var st Status
	for time.Now().Before(deadline) {
		if st = svc.Status(); st.LoginFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !st.LoginFailed {
		t.Fatal("Status never reported the failed exchange")
	}
	if st.LoggedIn {
		t.Error("a failed exchange must not also report a login")
	}

	// A fresh Start means the writer is trying again; the old failure must
	// not haunt the new attempt.
	ts.status = http.StatusOK
	ts.body = `{"access_token":"acc","refresh_token":"ref","id_token":""}`
	if _, err := svc.Start(context.Background()); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if st := svc.Status(); st.LoginFailed {
		t.Error("Start did not clear the previous failure")
	}
}

// TestStart_aStaleFailureFromTheSupersededAttemptDoesNotSurviveASecondStart
// guards the exact window a round-2 review found: Start clears the previous
// attempt's recorded failure and invalidates that attempt in two separate
// mutex sections, and if a stale write from the superseded attempt's
// callback lands between them, recordAttemptFailure's guard cannot tell it
// apart from a legitimate one — s.current still points at the superseded
// attempt until it is invalidated.
//
// Driving this through the real HTTP path is not practical: the two steps
// are back-to-back, sub-microsecond mutex sections with only pure,
// non-blocking work between them — nothing to hang a blocking token server
// on. Instead this uses a dedicated seam, startRaceSeamForTest (mirroring
// the existing afterExchangeForTest), to land a simulated stale write from
// the first attempt's callback deterministically inside that window, then
// checks the invariant that must hold once the second Start returns: no
// stale failure from a superseded attempt survives it.
func TestStart_aStaleFailureFromTheSupersededAttemptDoesNotSurviveASecondStart(t *testing.T) {
	svc, _, _ := newService(t, "")

	if _, err := svc.Start(context.Background()); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	svc.mu.Lock()
	att1 := svc.current
	svc.mu.Unlock()
	if att1 == nil {
		t.Fatal("first Start left no current attempt to supersede")
	}

	svc.startRaceSeamForTest = func() {
		// The first attempt's callback goroutine, concurrently, has decided
		// its exchange failed and is recording that — landing exactly in the
		// window this test exists to close.
		svc.recordAttemptFailure(att1, errors.New("a stale failure from the superseded attempt"))
	}

	if _, err := svc.Start(context.Background()); err != nil {
		t.Fatalf("second Start: %v", err)
	}

	if svc.hasLastFailure() {
		t.Error("a stale failure from the superseded first attempt survived the second Start")
	}
}

func TestStart_bothPortsBusyIsAnError(t *testing.T) {
	// Occupy both registered ports so no listener can bind.
	for _, port := range []int{PrimaryPort, FallbackPort} {
		ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err != nil {
			t.Skipf("port %d is already taken by something else; cannot run this case", port)
		}
		defer ln.Close() //nolint:revive // held for the length of the test on purpose
	}
	svc, _, _ := newService(t, "")
	_, err := svc.Start(context.Background())
	if !errors.Is(err, ErrPortInUse) {
		t.Fatalf("err = %v, want ErrPortInUse", err)
	}
}

func TestStart_secondCallCancelsTheFirst(t *testing.T) {
	svc, _, _ := newService(t, fakeIDToken(t, "a@b.c", "acct", 1))
	first, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("first Start: %v", err)
	}
	second, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if first == second {
		t.Fatal("the second login reused the first state; each attempt needs its own")
	}
	// The first attempt's callback must no longer be honoured.
	res := follow(t, first, "code", "")
	if res.StatusCode == http.StatusOK {
		t.Error("a superseded login attempt still accepted its callback")
	}
}

func TestStatus_reportsLoggedOutWithoutAFile(t *testing.T) {
	svc, _, _ := newService(t, "")
	if st := svc.Status(); st.LoggedIn || st.Email != "" {
		t.Errorf("status = %+v, want logged out", st)
	}
}

func TestLogoutMethod_clearsAStoredLogin(t *testing.T) {
	svc, _, home := newService(t, "")
	if err := writeAuthFile(home, Tokens{AccessToken: "a", IDToken: fakeIDToken(t, "x@y.z", "acct", 5)}, time.Now()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if !svc.Status().LoggedIn {
		t.Fatal("seeded credential did not read as a login")
	}
	if err := svc.Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if svc.Status().LoggedIn {
		t.Error("Logout left a login behind")
	}
}

func TestStart_timesOutAndStopsListening(t *testing.T) {
	svc, _, _ := newService(t, "")
	svc.loginWindow = 50 * time.Millisecond
	authURL, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	u, _ := url.Parse(authURL)
	redirect, _ := url.Parse(u.Query().Get("redirect_uri"))
	if _, err := http.Get(redirect.String()); err == nil {
		t.Error("the callback listener is still accepting connections after the window closed")
	}
}

// blockingTokenServer stands in for the token endpoint but holds every
// request open until the test releases it. It lets a test pin down exactly
// when, relative to an in-flight exchange, a supersession or logout happens —
// turning a scheduling race into a deterministic ordering.
type blockingTokenServer struct {
	*httptest.Server
	reached chan struct{}
	release chan struct{}
}

func newBlockingTokenServer(t *testing.T, idToken string) *blockingTokenServer {
	t.Helper()
	bs := &blockingTokenServer{
		reached: make(chan struct{}),
		release: make(chan struct{}),
	}
	var once sync.Once
	bs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(bs.reached) })
		<-bs.release
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"acc","refresh_token":"ref","id_token":"`+idToken+`"}`)
	}))
	t.Cleanup(bs.Close)
	return bs
}

// awaitReached blocks until a request has reached the handler (and is
// parked on <-bs.release), or fails the test if none arrives in time.
func (bs *blockingTokenServer) awaitReached(t *testing.T) {
	t.Helper()
	select {
	case <-bs.reached:
	case <-time.After(2 * time.Second):
		t.Fatal("exchange never reached the token endpoint")
	}
}

// TestStart_secondStartDuringExchangeDoesNotWriteCredential guards findings
// 1(b) and 2 from the Task 3 review: a second Start landing while the first
// attempt's callback is mid-exchange must leave that first attempt unable to
// write a credential, no matter how the exchange's own goroutine eventually
// resolves.
func TestStart_secondStartDuringExchangeDoesNotWriteCredential(t *testing.T) {
	idToken := fakeIDToken(t, "a@b.c", "acct", 1)
	home := t.TempDir()
	bs := newBlockingTokenServer(t, idToken)

	svc := NewService(home).WithTokenURL(bs.URL).WithHTTPClient(bs.Client())
	t.Cleanup(func() { _ = svc.Close() })

	first, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("first Start: %v", err)
	}

	type result struct {
		res *http.Response
		err error
	}
	done := make(chan result, 1)
	go func() {
		res, err := followAsync(first, "the-code", "")
		done <- result{res, err}
	}()
	bs.awaitReached(t)

	// Supersede the first attempt while its exchange is still in flight.
	if _, err := svc.Start(context.Background()); err != nil {
		t.Fatalf("second Start: %v", err)
	}

	close(bs.release)
	r := <-done
	// A transport error (the connection got reset or refused once the first
	// attempt was superseded) is an acceptable way for this callback to
	// fail — the only thing that must never happen is a 200 with a
	// credential behind it.
	if r.err == nil {
		defer r.res.Body.Close()
		if r.res.StatusCode == http.StatusOK {
			t.Errorf("a superseded attempt's callback still reports success (status %d)", r.res.StatusCode)
		}
	}
	if _, err := readAuthFile(home); err == nil {
		t.Fatal("a superseded attempt wrote a credential after its exchange returned")
	}
}

// TestLogoutMethod_duringAnInFlightExchangeIsNotUndone guards finding 2: a
// writer who signs out while a stray exchange is still running must not have
// that exchange resurrect the credential they just deleted.
func TestLogoutMethod_duringAnInFlightExchangeIsNotUndone(t *testing.T) {
	idToken := fakeIDToken(t, "a@b.c", "acct", 1)
	home := t.TempDir()
	bs := newBlockingTokenServer(t, idToken)

	svc := NewService(home).WithTokenURL(bs.URL).WithHTTPClient(bs.Client())
	t.Cleanup(func() { _ = svc.Close() })

	authURL, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	type result struct {
		res *http.Response
		err error
	}
	done := make(chan result, 1)
	go func() {
		res, err := followAsync(authURL, "the-code", "")
		done <- result{res, err}
	}()
	bs.awaitReached(t)

	if err := svc.Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	close(bs.release)
	r := <-done
	if r.err == nil {
		_ = r.res.Body.Close()
	}

	if _, err := readAuthFile(home); err == nil {
		t.Fatal("a stray exchange resurrected a credential after Logout deleted it")
	}
	if svc.Status().LoggedIn {
		t.Error("Status reports a login after Logout raced a stray exchange")
	}
}

// TestCallback_recheckAfterExchangeRefusesAStaleWrite guards finding 2
// directly, at the exact window the review named: not "during the HTTP
// round trip" (already covered above, and already closed by the exchange
// running on r.Context()), but the narrow gap strictly between the exchange
// returning and writeAuthFile being called. Blocking the token server can't
// land a test there — closing an attempt's listener cancels the request
// context and the exchange never returns at all — so this uses the
// package-private afterExchangeForTest seam to run a Logout at exactly that
// point, deterministically, instead of hoping a goroutine schedules there.
func TestCallback_recheckAfterExchangeRefusesAStaleWrite(t *testing.T) {
	svc, _, home := newService(t, fakeIDToken(t, "a@b.c", "acct", 1))

	authURL, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// hookDone carries the hook's Logout error back to the test goroutine.
	// Using a channel (rather than a plain variable the hook assigns) gives
	// the race detector a real happens-before edge: the hook runs on the
	// server's per-connection goroutine, and a bare variable read after the
	// client's HTTP round trip completes is not actually synchronized with
	// it — the socket carries bytes, not a Go memory barrier.
	hookDone := make(chan error, 1)
	svc.afterExchangeForTest = func() {
		// The exchange has already returned successfully; simulate a Logout
		// landing in the gap before the credential is written.
		hookDone <- svc.Logout()
	}

	// Logout closes this attempt's own listener from inside the hook, so
	// whether the client sees a clean refusal or a reset connection is
	// timing-dependent and is not what is under test — use the
	// non-fataling GET, run in the background, and ignore its outcome. What
	// matters is whether a credential lands on disk afterward.
	go func() {
		if res, err := followAsync(authURL, "the-code", ""); err == nil {
			_ = res.Body.Close()
		}
	}()

	if err := <-hookDone; err != nil {
		t.Fatalf("Logout (from the hook): %v", err)
	}

	// The channel receive above happened-after everything the handler did up
	// to and including the hook, but the handler keeps running afterward.
	// Give a stray writeAuthFile call a moment to land before asserting it
	// didn't: the fix refuses before ever calling it, but unfixed code races
	// the write against the connection Logout already closed, and this
	// makes that race resolve before the check runs instead of the check
	// beating a write that is still in flight.
	time.Sleep(300 * time.Millisecond)
	if _, err := readAuthFile(home); err == nil {
		t.Fatal("a credential was written after Logout ran between the exchange returning and the write")
	}
}

// TestStopAttempt_leavesAReplacementAttemptUntouched guards finding 1(b)
// directly: the deferred cleanup after a successful callback must be scoped
// to the attempt it belongs to. If it runs late — after a second Start has
// already replaced it — it must not tear down the replacement or null out
// s.current out from under it.
func TestStopAttempt_leavesAReplacementAttemptUntouched(t *testing.T) {
	svc, _, _ := newService(t, fakeIDToken(t, "a@b.c", "acct", 1))
	first, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("first Start: %v", err)
	}
	svc.mu.Lock()
	firstAttempt := svc.current
	svc.mu.Unlock()

	second, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if first == second {
		t.Fatal("the second login reused the first state")
	}

	// Simulate the first attempt's deferred success cleanup finally running
	// after it has already been superseded.
	svc.stopAttempt(firstAttempt)

	svc.mu.Lock()
	current := svc.current
	svc.mu.Unlock()
	if current == nil {
		t.Fatal("a stale cleanup for a superseded attempt nulled out the live attempt")
	}

	// The still-current second attempt must still answer its own callback.
	res := follow(t, second, "the-code", "")
	if res.StatusCode != http.StatusOK {
		t.Errorf("the live attempt's callback status = %d, want 200 after a stale cleanup for a different attempt", res.StatusCode)
	}
}
