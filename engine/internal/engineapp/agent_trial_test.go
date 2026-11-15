//go:build !mobile

package engineapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store/storetest"
)

// trialSite stands in for Linetta Works' trial proxy: an OpenAI-compatible
// chat endpoint under /api/trial/v1 that records what each request carried.
type trialSite struct {
	srv *httptest.Server

	mu        sync.Mutex
	exhausted bool
	requests  []trialRequest
}

type trialRequest struct {
	path  string
	auth  string
	model string
	tools []string
}

func newTrialSite(t *testing.T) *trialSite {
	t.Helper()
	s := &trialSite{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
			Tools  []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		_ = json.Unmarshal(raw, &body)
		req := trialRequest{path: r.URL.Path, auth: r.Header.Get("Authorization"), model: body.Model}
		for _, tool := range body.Tools {
			req.tools = append(req.tools, tool.Function.Name)
		}
		s.mu.Lock()
		s.requests = append(s.requests, req)
		exhausted := s.exhausted
		s.mu.Unlock()
		if exhausted {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"code":"trial_exhausted","message":"체험 한도를 다 썼습니다."}}`)
			return
		}
		if !body.Stream {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"네."},"finish_reason":"stop"}]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"index":0,"delta":{"role":"assistant","content":"네."},"finish_reason":null}]}`+"\n\n")
		_, _ = io.WriteString(w, `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`+"\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *trialSite) seen() []trialRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]trialRequest{}, s.requests...)
}

// openTrialApp opens an app whose device is connected to site with token and
// whose writer has chosen the trial and agreed to it.
func openTrialApp(t *testing.T, site, token string) *App {
	t.Helper()
	home := t.TempDir()
	t.Setenv("LINETTA_HOME", home)
	isolateHomeDir(t, t.TempDir())
	if err := storetest.Seed(filepath.Join(home, "library.db")); err != nil {
		t.Fatalf("seed library: %v", err)
	}
	secrets := settings.NewMemorySecretStore()
	st, err := settings.NewForHomeWithSecretStore(home, secrets)
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	if err := st.SetPublishToken(token); err != nil {
		t.Fatal(err)
	}
	if err := st.SetPublishAccount(&settings.PublishAccount{SiteURL: site, Handle: "writer"}); err != nil {
		t.Fatal(err)
	}
	app, err := Open(context.Background(), Options{Home: home, Secrets: secrets})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	choose := `{"provider":"linetta-trial","providers":{"linetta-trial":{"consented_at":1700000000000}}}`
	if _, rpcErr := call(t, app, "settings.set", choose); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}
	return app
}

func runTrialTurn(t *testing.T, app *App) *notificationLog {
	t.Helper()
	projectID, nodeID := seedProjectWithScene(t, app)
	log := app.CaptureNotificationsForTest()
	if _, rpcErr := call(t, app, "agent.run",
		`{"project_id":"`+projectID+`","node_id":"`+nodeID+`","prompt":"이 회차 주인공 이름이 뭐야?"}`); rpcErr != nil {
		t.Fatalf("agent.run: %+v", rpcErr)
	}
	return log
}

// A turn on the trial goes to the connected site's trial proxy with the
// publish token, through tars' real OpenAI-compatible client, holding the
// trial's twenty tools.
func TestAgentRun_onTheTrialGoesToTheSiteWithThePublishToken(t *testing.T) {
	site := newTrialSite(t)
	app := openTrialApp(t, site.srv.URL, "lwk_trial_token")
	log := runTrialTurn(t, app)
	waitForNotification(t, log, "agent.done")

	seen := site.seen()
	if len(seen) == 0 {
		t.Fatal("the trial turn never reached the site")
	}
	first := seen[0]
	if first.path != "/api/trial/v1/chat/completions" {
		t.Errorf("path = %q, want the trial proxy's chat endpoint", first.path)
	}
	if first.auth != "Bearer lwk_trial_token" {
		t.Errorf("Authorization = %q, want the publish token", first.auth)
	}
	if first.model != "linetta-trial" {
		t.Errorf("model = %q, want linetta-trial", first.model)
	}
	if len(first.tools) != 20 {
		t.Errorf("the trial turn held %d tools, want 20: %s", len(first.tools), strings.Join(first.tools, ", "))
	}
}

// The site's trial_exhausted reaches the panel as its own reason, not as a
// rate limit the writer could wait out.
func TestAgentRun_anExhaustedTrialEndsWithItsOwnReason(t *testing.T) {
	site := newTrialSite(t)
	site.exhausted = true
	app := openTrialApp(t, site.srv.URL, "lwk_trial_token")
	log := runTrialTurn(t, app)
	waitForNotification(t, log, "agent.error")
	var payload struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(log.paramsFor("agent.error"), &payload); err != nil {
		t.Fatalf("decode agent.error: %v", err)
	}
	if payload.Reason != "provider_trial_exhausted" {
		t.Errorf("reason = %q, want provider_trial_exhausted", payload.Reason)
	}
}

// trial.status is registered and asks the connected site.
func TestTrialStatus_isRegistered(t *testing.T) {
	app := openApp(t)
	res, rpcErr := call(t, app, "trial.status", "")
	if rpcErr != nil {
		t.Fatalf("trial.status: %+v", rpcErr)
	}
	if !strings.Contains(string(res), `"state":"not_connected"`) {
		t.Errorf("trial.status on a fresh install = %s, want not_connected", res)
	}
}
