package codexauth

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
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

// callbackURL builds the address the issuer would send the browser back to:
// the authorize URL's own redirect_uri, carrying params plus this attempt's
// state (or overrideState, to forge one).
func callbackURL(authURL string, params url.Values, overrideState string) (string, error) {
	u, err := url.Parse(authURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	state := q.Get("state")
	if overrideState != "" {
		state = overrideState
	}
	redirect, err := url.Parse(q.Get("redirect_uri"))
	if err != nil {
		return "", err
	}
	cb := *redirect
	cbq := url.Values{}
	for k, vs := range params {
		cbq[k] = vs
	}
	cbq.Set("state", state)
	cb.RawQuery = cbq.Encode()
	return cb.String(), nil
}

// follow performs the browser's half: GET the callback with the code and state
// the issuer would have sent back.
func follow(t *testing.T, authURL, code string, overrideState string) *http.Response {
	t.Helper()
	return followCallback(t, authURL, url.Values{"code": {code}}, overrideState)
}

// followCallback is follow for a callback that is not a plain success — a
// declined consent screen (?error=...), or one with no code at all.
func followCallback(t *testing.T, authURL string, params url.Values, overrideState string) *http.Response {
	t.Helper()
	cb, err := callbackURL(authURL, params, overrideState)
	if err != nil {
		t.Fatalf("build callback url: %v", err)
	}
	res, err := http.Get(cb)
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
	cb, err := callbackURL(authURL, url.Values{"code": {code}}, overrideState)
	if err != nil {
		return nil, err
	}
	return http.Get(cb)
}

// listenEphemeral is the test suite's stand-in for listenOnRegisteredPort. The
// production listener binds 1455 or 1457, which are shared machine state: a
// `codex login` sitting in someone's other terminal, or a second `go test`
// run, would make most of this file fail rather than skip. Nothing in these
// tests depends on the port number — the callback address travels in the
// authorize URL's redirect_uri, which is what follow() reads — so an
// ephemeral port makes the suite hermetic. Only
// TestStart_bothPortsBusyIsAnError, which is about the registered ports
// themselves, keeps the production listener.
func listenEphemeral() (net.Listener, int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, 0, err
	}
	return ln, ln.Addr().(*net.TCPAddr).Port, nil
}

func newService(t *testing.T, idToken string) (*Service, *tokenServer, string) {
	t.Helper()
	home := t.TempDir()
	ts := newTokenServer(t, idToken)
	svc := NewService(home).WithTokenURL(ts.URL).WithHTTPClient(ts.Client())
	svc.listen = listenEphemeral
	t.Cleanup(func() { _ = svc.Close() })
	return svc, ts, home
}

// TestNewService_wiresNoTestSeams guards the seams themselves. They are
// package-private fields a test assigns, which means a future constructor
// change could quietly wire one up in production and nothing would complain.
func TestNewService_wiresNoTestSeams(t *testing.T) {
	svc := NewService("x")
	if svc.startRaceSeamForTest != nil {
		t.Error("NewService wired a test seam into a production Service")
	}
	// The listener is not a test seam — it is a real dependency with a real
	// default — but the default has to be the registered-port listener, or
	// production would sign in on a port the issuer never redirects to.
	if svc.listen == nil {
		t.Fatal("NewService left the callback listener unset")
	}
	if reflect.ValueOf(svc.listen).Pointer() != reflect.ValueOf(listenOnRegisteredPort).Pointer() {
		t.Error("NewService does not default to the registered-port listener")
	}
}

func TestStart_completesTheLoginAndWritesTheCredential(t *testing.T) {
	idToken := fakeIDToken(t, "writer@example.com", "acct-42", 1788000000)
	svc, ts, home := newService(t, idToken)

	authURL, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Capture the attempt's own PKCE verifier before the callback retires it,
	// so the exchange assertion below can compare against the real value
	// rather than settling for "looks long enough".
	svc.mu.Lock()
	wantVerifier := svc.current.verifier
	svc.mu.Unlock()

	res := follow(t, authURL, "the-code", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("callback status = %d, want 200", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	// Assert on wording unique to the success page: both pages say "Linetta",
	// so matching that would pass on the failure page too.
	if !strings.Contains(string(body), "Signed in to ChatGPT") {
		t.Errorf("the callback did not serve the success page: %s", body)
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
	if got := ts.gotForm.Get("code_verifier"); got != wantVerifier {
		t.Errorf("code_verifier = %q, want this attempt's own verifier %q", got, wantVerifier)
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
	svc, ts, home := newService(t, fakeIDToken(t, "a@b.c", "acct", 1))
	authURL, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	res := follow(t, authURL, "the-code", "forged-state")
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a mismatched state", res.StatusCode)
	}
	time.Sleep(100 * time.Millisecond)
	// The state gate's real property is stronger than "no credential landed":
	// a forged callback must not reach the exchange at all, so the forged
	// code is never presented to the issuer.
	if len(ts.gotForm) != 0 {
		t.Errorf("a forged callback reached the token endpoint: %v", ts.gotForm)
	}
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

// awaitLoginFailed polls until Status reports a terminal failure. The failure
// is recorded inside the callback handler before it writes the response, but
// polling keeps the assertion from being coupled to exactly when that happens.
func awaitLoginFailed(t *testing.T, svc *Service) Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var st Status
	for time.Now().Before(deadline) {
		if st = svc.Status(); st.LoginFailed {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	return st
}

// TestCallback_aDeclinedConsentScreenIsAReportedFailure guards the gap
// LoginFailed exists to close, at its most common cause. A writer who clicks
// Cancel on OpenAI's consent screen comes back as ?error=access_denied with a
// perfectly valid state: past the state gate, terminal, and — before this —
// silent, so the pane went on saying "waiting for your browser" for the rest
// of the five-minute window while the writer looked at a failure page saying
// the opposite.
func TestCallback_aDeclinedConsentScreenIsAReportedFailure(t *testing.T) {
	svc, ts, _ := newService(t, "")

	authURL, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	res := followCallback(t, authURL, url.Values{
		"error":             {"access_denied"},
		"error_description": {"The user denied the request"},
	}, "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("callback status = %d, want 400 for a declined consent", res.StatusCode)
	}
	if len(ts.gotForm) != 0 {
		t.Errorf("a declined consent still reached the token endpoint: %v", ts.gotForm)
	}

	st := awaitLoginFailed(t, svc)
	if !st.LoginFailed {
		t.Fatal("Status never reported the declined consent; the pane would spin until the window closed")
	}
	if st.LoggedIn {
		t.Error("a declined consent must not also report a login")
	}

	// And the attempt must be over, not merely marked failed: an answer the
	// writer has already given must not hold a callback port for the rest of
	// the window.
	assertCallbackListenerStopped(t, authURL)
}

// TestCallback_aCallbackWithNoCodeIsAReportedFailure covers the other branch
// past the state gate that used to end in silence: the issuer sent the browser
// back with neither an error nor a code, which is terminal for this attempt
// just the same.
func TestCallback_aCallbackWithNoCodeIsAReportedFailure(t *testing.T) {
	svc, _, _ := newService(t, "")

	authURL, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	res := followCallback(t, authURL, url.Values{}, "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("callback status = %d, want 400 for a callback with no code", res.StatusCode)
	}
	if st := awaitLoginFailed(t, svc); !st.LoginFailed {
		t.Fatal("Status never reported a callback that carried no code")
	}
	assertCallbackListenerStopped(t, authURL)
}

// assertCallbackListenerStopped checks that nothing is still listening on the
// attempt's callback address. The teardown is asynchronous (Shutdown waits for
// the handler that scheduled it to return), so poll rather than assume.
func assertCallbackListenerStopped(t *testing.T, authURL string) {
	t.Helper()
	// The probe carries a forged state on purpose: it must bounce off the
	// state gate rather than reach the exchange, or the probe itself would
	// complete a login and stop the very listener it is checking on.
	cb, err := callbackURL(authURL, url.Values{"code": {"late"}}, "a-forged-probe-state")
	if err != nil {
		t.Fatalf("build callback url: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		res, err := http.Get(cb)
		if err != nil {
			return
		}
		_ = res.Body.Close()
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("the callback listener is still holding its port after a terminal failure")
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
	// This is the one case that is about the registered ports themselves, so
	// it is the one case that uses the production listener.
	svc.listen = listenOnRegisteredPort
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
	// The first attempt's callback must no longer be honoured. Either outcome
	// counts: on the registered ports the second Start rebinds the same port
	// and the new server refuses the stale state, and on the ephemeral ports
	// this suite uses the first listener is simply gone and the connection is
	// refused. Accepting both is also what retires the rebind flake this test
	// used to have — it no longer depends on the second bind winning a race
	// with the first close.
	res, err := followAsync(first, "code", "")
	if err == nil {
		defer res.Body.Close()
		if res.StatusCode == http.StatusOK {
			t.Error("a superseded login attempt still accepted its callback")
		}
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
	// A reply at all means the listener outlived its window, which is the
	// failure. Close the body on that path anyway: a test that leaks a
	// connection while reporting a leaked listener is a poor witness.
	res, err := http.Get(redirect.String())
	if err == nil {
		_ = res.Body.Close()
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
	svc.listen = listenEphemeral
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
	svc.listen = listenEphemeral
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

// TestFinishLogin_refusesToWriteForASupersededAttempt guards finding 2
// directly, at the exact window the review named: not "during the HTTP round
// trip" (already covered above, and already closed by the exchange running on
// r.Context()), but the narrow gap strictly between the exchange returning and
// the credential being written. Blocking the token server cannot land a test
// there — closing an attempt's listener cancels the request context and the
// exchange never returns at all — and this used to need a production seam
// planted mid-handler. It does not any more: finishLogin *is* that half of the
// callback, so the test signs out first and then runs the whole post-code path
// against a recorder, with the attempt already stale before the exchange even
// starts. Deterministic, and no seam.
func TestFinishLogin_refusesToWriteForASupersededAttempt(t *testing.T) {
	svc, _, home := newService(t, fakeIDToken(t, "a@b.c", "acct", 1))

	authURL, err := svc.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	svc.mu.Lock()
	att := svc.current
	svc.mu.Unlock()
	if att == nil {
		t.Fatal("Start left no current attempt")
	}

	// The writer signs out while this attempt's exchange is still in flight.
	if err := svc.Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	cb, err := callbackURL(authURL, url.Values{"code": {"the-code"}}, "")
	if err != nil {
		t.Fatalf("build callback url: %v", err)
	}
	rec := httptest.NewRecorder()
	svc.finishLogin(rec, httptest.NewRequest(http.MethodGet, cb, nil), att, "the-code", PrimaryPort)

	if rec.Code == http.StatusOK {
		t.Errorf("a superseded attempt's callback reported success (status %d)", rec.Code)
	}
	if _, err := readAuthFile(home); err == nil {
		t.Fatal("a credential was written for an attempt that was superseded before the write")
	}
	if svc.Status().LoggedIn {
		t.Error("Status reports a login after a superseded attempt finished its exchange")
	}
}

// TestLogout_waitsForAnInFlightCredentialWrite guards the check-to-write
// window the recheck above cannot close on its own. The recheck and the write
// are two steps; so are Logout's stop and delete. Interleave them —
// storeCredential's recheck passes, Logout stops the attempt and deletes the
// file, storeCredential writes — and a credential survives a successful
// sign-out, with Status reporting signed in.
//
// The interleaving is a handful of instructions wide and cannot be scheduled
// deliberately, so this test asserts the property that closes it instead: the
// two pairs are mutually exclusive under fileMu. Holding fileMu stands in for
// a callback that is inside storeCredential; Logout must not proceed until it
// is released.
func TestLogout_waitsForAnInFlightCredentialWrite(t *testing.T) {
	svc, _, home := newService(t, "")
	if err := writeAuthFile(home, Tokens{AccessToken: "a"}, time.Now()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	svc.fileMu.Lock()
	done := make(chan error, 1)
	go func() { done <- svc.Logout() }()

	select {
	case err := <-done:
		svc.fileMu.Unlock()
		t.Fatalf("Logout ran while a credential write held the file lock (err = %v)", err)
	case <-time.After(200 * time.Millisecond):
	}

	svc.fileMu.Unlock()
	if err := <-done; err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := readAuthFile(home); err == nil {
		t.Error("Logout left the credential behind")
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
