package openrouter

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

type OAuthConfig struct {
	APIBaseURL  string
	AuthBaseURL string
	HTTPClient  *http.Client
	Timeout     time.Duration
}

type OAuthStart struct {
	RequestID   string `json:"request_id"`
	AuthURL     string `json:"auth_url"`
	CallbackURL string `json:"callback_url"`
	ExpiresAt   int64  `json:"expires_at"`
}

type OAuthManager struct {
	mu          sync.Mutex
	sessions    map[string]*oauthSession
	apiBaseURL  string
	authBaseURL string
	httpClient  *http.Client
	timeout     time.Duration
}

type oauthSession struct {
	codeVerifier string
	callbackURL  string
	expiresAt    time.Time
	server       *http.Server
	listener     net.Listener
	callback     chan oauthCallback
}

type oauthCallback struct {
	code string
	err  error
}

func NewOAuthManager(cfg OAuthConfig) *OAuthManager {
	apiBaseURL := strings.TrimRight(strings.TrimSpace(cfg.APIBaseURL), "/")
	if apiBaseURL == "" {
		apiBaseURL = settings.OpenRouterBaseURL
	}
	authBaseURL := strings.TrimSpace(cfg.AuthBaseURL)
	if authBaseURL == "" {
		authBaseURL = "https://openrouter.ai/auth"
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &OAuthManager{
		sessions:    map[string]*oauthSession{},
		apiBaseURL:  apiBaseURL,
		authBaseURL: authBaseURL,
		httpClient:  httpClient,
		timeout:     timeout,
	}
}

func (m *OAuthManager) Start(ctx context.Context) (OAuthStart, error) {
	requestID, err := randomToken(18)
	if err != nil {
		return OAuthStart{}, err
	}
	state, err := randomToken(18)
	if err != nil {
		return OAuthStart{}, err
	}
	verifier, err := randomToken(48)
	if err != nil {
		return OAuthStart{}, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return OAuthStart{}, err
	}
	expiresAt := time.Now().Add(m.timeout)
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback/%s", ln.Addr().(*net.TCPAddr).Port, url.PathEscape(state))
	session := &oauthSession{
		codeVerifier: verifier,
		callbackURL:  callbackURL,
		expiresAt:    expiresAt,
		listener:     ln,
		callback:     make(chan oauthCallback, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/callback/", func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimPrefix(r.URL.Path, "/callback/") != state {
			session.sendCallback(oauthCallback{err: errors.New("openrouter oauth state mismatch")})
			http.Error(w, "Linetta OpenRouter 연결 상태가 맞지 않습니다. 앱에서 다시 시도하세요.", http.StatusForbidden)
			return
		}
		if msg := strings.TrimSpace(r.URL.Query().Get("error")); msg != "" {
			session.sendCallback(oauthCallback{err: errors.New(msg)})
			http.Error(w, "OpenRouter 연결이 취소되었습니다. Linetta로 돌아가 다시 시도하세요.", http.StatusBadRequest)
			return
		}
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		if code == "" {
			session.sendCallback(oauthCallback{err: errors.New("openrouter oauth code missing")})
			http.Error(w, "OpenRouter 인증 code가 없습니다. Linetta로 돌아가 다시 시도하세요.", http.StatusBadRequest)
			return
		}
		session.sendCallback(oauthCallback{code: code})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><title>Linetta</title><p>OpenRouter 연결을 받았습니다. 이제 Linetta로 돌아가세요.</p>"))
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	session.server = server

	m.mu.Lock()
	m.sessions[requestID] = session
	m.mu.Unlock()

	go func() {
		_ = server.Serve(ln)
	}()
	go func() {
		<-time.After(m.timeout)
		m.cleanup(requestID)
	}()

	authURL, err := m.authURL(callbackURL, verifier)
	if err != nil {
		m.cleanup(requestID)
		return OAuthStart{}, err
	}
	_ = ctx
	return OAuthStart{
		RequestID:   requestID,
		AuthURL:     authURL,
		CallbackURL: callbackURL,
		ExpiresAt:   expiresAt.UnixMilli(),
	}, nil
}

func (m *OAuthManager) Finish(ctx context.Context, requestID string) (string, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return "", errors.New("openrouter oauth request_id is required")
	}
	m.mu.Lock()
	session := m.sessions[requestID]
	m.mu.Unlock()
	if session == nil {
		return "", errors.New("openrouter oauth session not found or expired")
	}
	defer m.cleanup(requestID)

	timer := time.NewTimer(time.Until(session.expiresAt))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-timer.C:
		return "", errors.New("openrouter oauth timed out")
	case cb := <-session.callback:
		if cb.err != nil {
			return "", cb.err
		}
		return m.exchangeCode(ctx, cb.code, session.codeVerifier)
	}
}

func (m *OAuthManager) authURL(callbackURL, verifier string) (string, error) {
	u, err := url.Parse(m.authBaseURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("callback_url", callbackURL)
	q.Set("code_challenge", codeChallengeS256(verifier))
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (m *OAuthManager) exchangeCode(ctx context.Context, code, verifier string) (string, error) {
	body, err := json.Marshal(map[string]string{
		"code":                  code,
		"code_verifier":         verifier,
		"code_challenge_method": "S256",
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.apiBaseURL+"/auth/keys", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := m.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("openrouter oauth exchange failed: %s", res.Status)
	}
	var payload struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return "", err
	}
	key := strings.TrimSpace(payload.Key)
	if key == "" {
		return "", errors.New("openrouter oauth exchange returned empty key")
	}
	return key, nil
}

func (m *OAuthManager) cleanup(requestID string) {
	m.mu.Lock()
	session := m.sessions[requestID]
	delete(m.sessions, requestID)
	m.mu.Unlock()
	if session == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = session.server.Shutdown(ctx)
	_ = session.listener.Close()
}

func (s *oauthSession) sendCallback(cb oauthCallback) {
	select {
	case s.callback <- cb:
	default:
	}
}

func codeChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomToken(byteLen int) (string, error) {
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
