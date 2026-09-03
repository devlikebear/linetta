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
