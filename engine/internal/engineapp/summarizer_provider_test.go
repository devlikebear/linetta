//go:build !mobile

package engineapp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/agenttest"
)

// A scene long enough to leave the plain-text path.
var longSceneDoc = fmt.Sprintf(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":%q}]}]}`,
	strings.Repeat("비가 오는 밤, 해진은 탑의 문을 열었다. ", 20))

func saveAndWaitForSummary(t *testing.T, app *App, nodeID string) string {
	t.Helper()
	params, _ := json.Marshal(map[string]string{"id": nodeID, "doc": longSceneDoc})
	if _, rpcErr := call(t, app, "nodes.update_content", string(params)); rpcErr != nil {
		t.Fatalf("nodes.update_content: %+v", rpcErr)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		out, rpcErr := call(t, app, "nodes.get", `{"id":"`+nodeID+`"}`)
		if rpcErr != nil {
			t.Fatalf("nodes.get: %+v", rpcErr)
		}
		var n struct {
			Summary           string `json:"summary"`
			SummaryForVersion int    `json:"summary_for_version"`
			ContentVersion    int    `json:"content_version"`
		}
		_ = json.Unmarshal(out, &n)
		if n.Summary != "" && n.SummaryForVersion == n.ContentVersion {
			return n.Summary
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no summary after saving")
	return ""
}

func wantReason(t *testing.T, rpcErr *rpcError, reason string) {
	t.Helper()
	if rpcErr == nil || !strings.Contains(string(rpcErr.Data), reason) {
		t.Fatalf("error = %+v, want reason %s", rpcErr, reason)
	}
}

const consentedProvider = `{"provider":"anthropic","providers":{"anthropic":{"api_key":"sk-test","consented_at":1700000000000}}}`

// #135: switched off — the default — nothing reaches a provider by any path,
// even with one connected and consented to.
func TestSummarizerProvider_offByDefaultSendsNothing(t *testing.T) {
	app := openApp(t)
	projectID, nodeID := seedProjectWithScene(t, app)
	if _, rpcErr := call(t, app, "settings.set", consentedProvider); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}
	app.SetProviderFactoryForTest(agenttest.NewFailingClientFactory(t.Error))

	if got := saveAndWaitForSummary(t, app, nodeID); !strings.HasPrefix(got, "비가 오는 밤") {
		t.Fatalf("summary = %q, want the opening cut", got)
	}
	_, rpcErr := call(t, app, "projects.rewrite_synopsis", `{"id":"`+projectID+`"}`)
	wantReason(t, rpcErr, "summarizer_off")
}

// Switched on before consent: still nothing leaves.
func TestSummarizerProvider_onWithoutConsentSendsNothing(t *testing.T) {
	app := openApp(t)
	projectID, nodeID := seedProjectWithScene(t, app)
	if _, rpcErr := call(t, app, "settings.set",
		`{"summarizer_provider_enabled":true,"provider":"anthropic","providers":{"anthropic":{"api_key":"sk-test"}}}`); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}
	app.SetProviderFactoryForTest(agenttest.NewFailingClientFactory(t.Error))

	if got := saveAndWaitForSummary(t, app, nodeID); !strings.HasPrefix(got, "비가 오는 밤") {
		t.Fatalf("summary = %q, want the opening cut", got)
	}
	_, rpcErr := call(t, app, "projects.rewrite_synopsis", `{"id":"`+projectID+`"}`)
	wantReason(t, rpcErr, "provider_consent_required")
}

func TestSummarizerProvider_onAndConsentedSummarizesTheSavedScene(t *testing.T) {
	app := openApp(t)
	projectID, nodeID := seedProjectWithScene(t, app)
	if _, rpcErr := call(t, app, "settings.set", consentedProvider); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}
	if _, rpcErr := call(t, app, "settings.set", `{"summarizer_provider_enabled":true}`); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}
	app.SetProviderFactoryForTest(agenttest.NewScriptedClientFactory(agenttest.ScriptedTurn{Text: "해진이 탑의 문을 연다."}))

	if got := saveAndWaitForSummary(t, app, nodeID); got != "해진이 탑의 문을 연다." {
		t.Fatalf("summary = %q, want the provider's", got)
	}

	out, rpcErr := call(t, app, "projects.rewrite_synopsis", `{"id":"`+projectID+`"}`)
	if rpcErr != nil {
		t.Fatalf("projects.rewrite_synopsis: %+v", rpcErr)
	}
	var proj struct {
		Synopsis string `json:"synopsis"`
	}
	if err := json.Unmarshal(out, &proj); err != nil || proj.Synopsis != "해진이 탑의 문을 연다." {
		t.Fatalf("synopsis = %q, %v", proj.Synopsis, err)
	}

	raw, rpcErr := call(t, app, "summarizer.usage", "")
	if rpcErr != nil {
		t.Fatalf("summarizer.usage: %+v", rpcErr)
	}
	var usage struct{ Calls int }
	if err := json.Unmarshal(raw, &usage); err != nil || usage.Calls != 2 {
		t.Fatalf("usage = %s, want two calls", raw)
	}

	actRaw, rpcErr := call(t, app, "mcp.activity", `{"limit":20}`)
	if rpcErr != nil {
		t.Fatalf("mcp.activity: %+v", rpcErr)
	}
	var rows []struct{ Tool, Source string }
	_ = json.Unmarshal(actRaw, &rows)
	seen := map[string]bool{}
	for _, r := range rows {
		if r.Source == "summarizer" {
			seen[r.Tool] = true
		}
	}
	if !seen["summarize_scene"] || !seen["rewrite_synopsis"] {
		t.Fatalf("activity = %s, want both summarizer rows", actRaw)
	}
}
