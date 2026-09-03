//go:build !mobile

package engineapp

import (
	"encoding/json"
	"testing"
)

// providerStatus mirrors provider.Status as it arrives over the wire. Decoded
// from JSON rather than reusing the struct on purpose: the field names are
// part of the contract the renderer reads.
type providerStatus struct {
	ID         string `json:"id"`
	Auth       string `json:"auth"`
	Active     bool   `json:"active"`
	Configured bool   `json:"configured"`
	Consented  bool   `json:"consented"`
	Model      string `json:"model"`
	BaseURL    string `json:"base_url"`
}

// The three providers.* registrations in engineapp are the one link nothing
// else covers: a typo in a method name compiles, passes every package test,
// and only fails when a writer clicks. This drives a real *App through
// app.Handle the way the renderer does.
func TestProvidersListIsRegisteredAndDescribesAFreshInstall(t *testing.T) {
	app := openApp(t)

	result, rpcErr := call(t, app, "providers.list", "")
	if rpcErr != nil {
		t.Fatalf("providers.list: %+v", rpcErr)
	}
	var got []providerStatus
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("decode providers.list: %v (%s)", err, result)
	}

	want := []providerStatus{
		{ID: "openai-codex", Auth: "oauth", Active: true},
		{ID: "anthropic", Auth: "api_key"},
		{ID: "gemini-native", Auth: "api_key"},
		{ID: "openai", Auth: "api_key"},
	}
	if len(got) != len(want) {
		t.Fatalf("providers.list returned %d entries, want %d: %s", len(got), len(want), result)
	}
	for i, w := range want {
		// Whitelist order is the order the settings pane renders, so it is
		// asserted positionally rather than by lookup.
		if got[i].ID != w.ID {
			t.Errorf("entry %d id = %q, want %q (whitelist order)", i, got[i].ID, w.ID)
		}
		if got[i].Auth != w.Auth {
			t.Errorf("%s auth = %q, want %q", w.ID, got[i].Auth, w.Auth)
		}
		if got[i].Active != w.Active {
			t.Errorf("%s active = %v, want %v", w.ID, got[i].Active, w.Active)
		}
		// A fresh install has no key and no login, and nothing is consented to.
		if got[i].Configured {
			t.Errorf("%s reports configured on a fresh install", w.ID)
		}
		if got[i].Consented {
			t.Errorf("%s reports consent on a fresh install", w.ID)
		}
	}
}

// providers.list_models and providers.test must be reachable too. Both refuse
// an unconfigured provider before any network activity, so a fresh install can
// assert the registration and the reason code without dialling anyone.
func TestProvidersListModelsAndTestAreRegistered(t *testing.T) {
	app := openApp(t)
	for _, method := range []string{"providers.list_models", "providers.test"} {
		_, rpcErr := call(t, app, method, `{"provider":"anthropic"}`)
		if rpcErr == nil {
			t.Errorf("%s: expected a refusal for an unconfigured provider", method)
			continue
		}
		// A missing registration answers "method not found" (-32601); this is
		// the handler running and refusing, which is what we are asserting.
		if got := string(rpcErr.Data); got != `{"reason":"provider_not_configured"}` {
			t.Errorf("%s: error data = %s (code %d, %q), want a provider_not_configured reason",
				method, got, rpcErr.Code, rpcErr.Message)
		}
	}
}
