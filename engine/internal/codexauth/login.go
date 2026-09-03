package codexauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ErrPortInUse means neither registered callback port could be bound. The
// redirect URI is registered per port, so there is no third port to try — a
// running Codex CLI login is the usual cause.
var ErrPortInUse = errors.New("codexauth: both callback ports are in use")

// ErrLoginFailed means the issuer refused the exchange.
var ErrLoginFailed = errors.New("codexauth: login failed")

// defaultLoginWindow bounds how long a callback listener stays open. Long
// enough to find a password, short enough that a forgotten attempt does not
// hold a port all day.
const defaultLoginWindow = 5 * time.Minute

// exchangeTimeout bounds the token request itself.
const exchangeTimeout = 30 * time.Second

// Status is what the settings pane shows about the stored login. It never
// carries a token — only whether one exists and whose account it is.
type Status struct {
	LoggedIn  bool   `json:"logged_in"`
	Email     string `json:"email,omitempty"`
	AccountID string `json:"account_id,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

// attempt is one in-flight login: the listener, the secrets that tie the
// callback to the authorize request, and the cancel that tears it all down.
type attempt struct {
	state    string
	verifier string
	srv      *http.Server
	cancel   context.CancelFunc
}

// Service owns the login flow for one Codex home directory.
type Service struct {
	codexHome string
	tokenURL  string
	client    *http.Client
	now       func() time.Time

	loginWindow time.Duration

	mu      sync.Mutex
	current *attempt

	// afterExchangeForTest, if set, runs synchronously right after a
	// successful exchange and before the pre-write recheck. Production code
	// never sets it; it exists only so a test can land a Logout or a second
	// Start deterministically inside the window between the exchange
	// returning and the credential write, instead of hoping a goroutine
	// schedules there.
	afterExchangeForTest func()
}

// NewService returns a Service writing to codexHome. Nothing binds or dials
// until Start is called.
func NewService(codexHome string) *Service {
	return &Service{
		codexHome:   codexHome,
		tokenURL:    tokenEndpoint,
		client:      &http.Client{Timeout: exchangeTimeout},
		now:         time.Now,
		loginWindow: defaultLoginWindow,
	}
}

// WithTokenURL points the exchange somewhere else (tests).
func (s *Service) WithTokenURL(u string) *Service { s.tokenURL = u; return s }

// WithHTTPClient replaces the exchange client (tests).
func (s *Service) WithHTTPClient(c *http.Client) *Service { s.client = c; return s }

// WithClock replaces the clock stamped into the credential file (tests).
func (s *Service) WithClock(fn func() time.Time) *Service { s.now = fn; return s }

// Start opens a callback listener and returns the URL the OS browser should
// open. It returns as soon as the listener is up: the exchange happens on the
// callback, and the writer learns the outcome from Status.
//
// A second Start cancels the first. A writer who abandoned one attempt and
// clicked again should not have a stale listener holding the port, nor two
// listeners racing for one callback.
//
// ctx is checked once, up front, for a caller that has already given up —
// see below — but does not bound the login window itself. Today's RPC server
// hands handlers a connection-lifetime context, so that would be harmless,
// but it is a trap for a future transport that hands handlers a per-request
// context: the 5-minute window a writer needs to find their password would
// silently collapse to however long that single request is allowed to run.
// The window is instead derived from context.Background(), and torn down
// deliberately by stopCurrent (via a second Start, Logout, or Close) or by
// its own timeout — never by the caller's request finishing.
func (s *Service) Start(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	verifier, challenge, err := newPKCE()
	if err != nil {
		return "", err
	}
	state, err := newState()
	if err != nil {
		return "", err
	}

	// Release the previous attempt's listener before binding: a second Start
	// must reclaim the primary port so that a stray callback for the
	// superseded attempt lands on the new server (and gets refused for state
	// mismatch) instead of finding nothing listening at all.
	s.stopCurrent()

	ln, port, err := listenOnRegisteredPort()
	if err != nil {
		return "", err
	}

	runCtx, cancel := context.WithTimeout(context.Background(), s.loginWindow)
	att := &attempt{state: state, verifier: verifier, cancel: cancel}

	mux := http.NewServeMux()
	mux.HandleFunc(CallbackPath, s.callbackHandler(att, port))
	att.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	s.mu.Lock()
	s.current = att
	s.mu.Unlock()

	go func() {
		if err := att.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("codexauth: callback server: %v", err)
		}
	}()
	go func() {
		<-runCtx.Done()
		// Closing the listener is what makes the timeout observable: an
		// abandoned attempt must not hold a registered port.
		_ = att.srv.Close()
	}()

	return authorizeURL(challenge, state, port), nil
}

// callbackHandler completes the login. It is deliberately strict: the state
// must match this attempt, and anything else is refused without touching the
// stored credential.
func (s *Service) callbackHandler(att *attempt, port int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		if !s.isCurrent(att) || q.Get("state") != att.state {
			writePage(w, http.StatusBadRequest, failurePage)
			return
		}
		if e := q.Get("error"); e != "" {
			log.Printf("codexauth: issuer refused the authorization: %s", e)
			writePage(w, http.StatusBadRequest, failurePage)
			return
		}
		code := q.Get("code")
		if code == "" {
			writePage(w, http.StatusBadRequest, failurePage)
			return
		}

		tok, err := s.exchange(r.Context(), code, att.verifier, port)
		if err != nil {
			log.Printf("codexauth: %v", err)
			writePage(w, http.StatusBadGateway, failurePage)
			return
		}

		if s.afterExchangeForTest != nil {
			s.afterExchangeForTest()
		}

		// The exchange can run for up to exchangeTimeout. Re-validate that
		// this attempt is still the one in flight immediately before writing
		// anything: a Logout or a second Start that landed while we were
		// waiting on the issuer must not have its outcome overwritten by a
		// credential this attempt no longer has any business writing.
		if !s.isCurrent(att) {
			writePage(w, http.StatusBadRequest, failurePage)
			return
		}

		_, accountID, _ := claimsFromIDToken(tok.IDToken)
		tok.AccountID = accountID
		if err := writeAuthFile(s.codexHome, tok, s.now()); err != nil {
			log.Printf("codexauth: %v", err)
			writePage(w, http.StatusInternalServerError, failurePage)
			return
		}
		writePage(w, http.StatusOK, successPage)

		// The writer has their answer; this attempt's job is done. stopAttempt
		// shuts down gracefully — draining the response already in flight
		// rather than resetting it, as Close would — and only touches
		// s.current if it is still this attempt: a second Start that already
		// replaced it is not this attempt's cleanup to perform.
		go s.stopAttempt(att)
	}
}

// isCurrent reports whether att is still the attempt in flight.
func (s *Service) isCurrent(att *attempt) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current == att
}

func writePage(w http.ResponseWriter, status int, page string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, page)
}

// exchange trades the authorization code for tokens.
func (s *Service) exchange(ctx context.Context, code, verifier string, port int) (Tokens, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI(port))
	form.Set("client_id", ClientID)
	form.Set("code_verifier", verifier)

	ctx, cancel := context.WithTimeout(ctx, exchangeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, fmt.Errorf("%w: build request: %v", ErrLoginFailed, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := s.client.Do(req)
	if err != nil {
		return Tokens{}, fmt.Errorf("%w: %v", ErrLoginFailed, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return Tokens{}, fmt.Errorf("%w: read response: %v", ErrLoginFailed, err)
	}
	if res.StatusCode != http.StatusOK {
		// The body can carry the issuer's own wording; keep only the status so
		// a provider message never rides into a log verbatim.
		return Tokens{}, fmt.Errorf("%w: token endpoint returned %d", ErrLoginFailed, res.StatusCode)
	}
	var tok Tokens
	if err := json.Unmarshal(body, &tok); err != nil {
		return Tokens{}, fmt.Errorf("%w: parse response: %v", ErrLoginFailed, err)
	}
	if tok.AccessToken == "" || tok.RefreshToken == "" {
		return Tokens{}, fmt.Errorf("%w: response was missing a token", ErrLoginFailed)
	}
	return tok, nil
}

// Status reports the stored login without reading any token value out to the
// caller.
func (s *Service) Status() Status {
	f, err := readAuthFile(s.codexHome)
	if err != nil {
		return Status{}
	}
	if strings.TrimSpace(f.Tokens.AccessToken) == "" {
		return Status{}
	}
	email, accountID, exp := claimsFromIDToken(f.Tokens.IDToken)
	if accountID == "" {
		accountID = f.Tokens.AccountID
	}
	return Status{LoggedIn: true, Email: email, AccountID: accountID, ExpiresAt: exp}
}

// Logout deletes the stored login and drops any attempt in flight.
func (s *Service) Logout() error {
	s.stopCurrent()
	return Logout(s.codexHome)
}

// Close releases a listener still waiting on a callback.
func (s *Service) Close() error {
	s.stopCurrent()
	return nil
}

// stopCurrent abruptly tears down whatever attempt is current, if any. It is
// deliberately unconditional and unscoped: the timeout, Logout and Close
// paths all mean "whatever is running now, stop it," and an abrupt Close is
// correct there — there is no response in flight worth draining.
func (s *Service) stopCurrent() {
	s.mu.Lock()
	att := s.current
	s.current = nil
	s.mu.Unlock()
	if att == nil {
		return
	}
	att.cancel()
	_ = att.srv.Close()
}

// stopAttempt gracefully tears down att, but only if att is still the
// current attempt. It is used solely by the success path in
// callbackHandler, and both properties matter: Shutdown (rather than Close)
// lets the success response already in flight finish writing instead of
// being reset mid-flush, and the identity check means a copy of this call
// that runs late — after a second Start has already replaced att — does
// nothing, rather than closing the replacement's listener and nulling out
// s.current out from under it.
//
// Shutdown runs before cancel is called: att.cancel() releases the timeout
// goroutine (see Start), which reacts by calling att.srv.Close(). Canceling
// first would let that abrupt Close race the graceful Shutdown and reset the
// very response Shutdown exists to drain. Once Shutdown has returned, the
// server is already fully stopped, so the later Close from that goroutine is
// a harmless no-op.
func (s *Service) stopAttempt(att *attempt) {
	s.mu.Lock()
	if s.current != att {
		s.mu.Unlock()
		return
	}
	s.current = nil
	s.mu.Unlock()
	_ = att.srv.Shutdown(context.Background())
	att.cancel()
}

// listenOnRegisteredPort binds the primary callback port, falling back to the
// secondary one. It never picks a third: the redirect URI is registered per
// port, so any other port would be refused by the issuer, and silently binding
// one would turn a clear error into a mystery in the browser.
func listenOnRegisteredPort() (net.Listener, int, error) {
	var lastErr error
	for _, port := range []int{PrimaryPort, FallbackPort} {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			return ln, port, nil
		}
		lastErr = err
		if !isAddrInUse(err) {
			return nil, 0, fmt.Errorf("codexauth: listen on 127.0.0.1:%d: %w", port, err)
		}
	}
	return nil, 0, fmt.Errorf("%w (%d, %d): %v", ErrPortInUse, PrimaryPort, FallbackPort, lastErr)
}

// isAddrInUse recognises a taken port across platforms. Windows reports
// WSAEADDRINUSE (10048) rather than EADDRINUSE, and some wrappers only leave
// the text behind.
func isAddrInUse(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) && uintptr(errno) == 10048 {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "address already in use") ||
		strings.Contains(msg, "only one usage of each socket address")
}
