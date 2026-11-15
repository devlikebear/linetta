package provider

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

func connectWorks(t *testing.T, st *settings.Store, site, token string) {
	t.Helper()
	if err := st.SetPublishToken(token); err != nil {
		t.Fatalf("SetPublishToken: %v", err)
	}
	if err := st.SetPublishAccount(&settings.PublishAccount{SiteURL: site, Handle: "writer"}); err != nil {
		t.Fatalf("SetPublishAccount: %v", err)
	}
}

func TestResolveTrial_isUnconfiguredWithoutAWorksConnection(t *testing.T) {
	src, _, _ := newSource(t)
	r, err := src.Resolve(settings.ProviderLinettaTrial)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.Configured() {
		t.Errorf("Configured with no connection: %v", r)
	}
	if r.BaseURL != "" || r.APIKey != "" {
		t.Errorf("Resolved = %v, want no address and no key", r)
	}
}

func TestResolveTrial_borrowsThePublishConnection(t *testing.T) {
	src, st, _ := newSource(t)
	connectWorks(t, st, "https://works.example.com/", "lwk_secret")
	r, err := src.Resolve(settings.ProviderLinettaTrial)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !r.Configured() {
		t.Fatalf("not Configured with a connection: %v", r)
	}
	opts := Options(r)
	if opts.Provider != "openai" {
		t.Errorf("tars provider = %q, want openai", opts.Provider)
	}
	if opts.BaseURL != "https://works.example.com/api/trial/v1" {
		t.Errorf("BaseURL = %q", opts.BaseURL)
	}
	if opts.APIKey != "lwk_secret" {
		t.Errorf("APIKey = %q, want the publish token", opts.APIKey)
	}
	if opts.Model != TrialModel {
		t.Errorf("Model = %q, want %q", opts.Model, TrialModel)
	}
	if strings.Contains(r.String(), "lwk_secret") {
		t.Errorf("String leaks the token: %s", r.String())
	}
}

func TestResolveTrial_losingTheTokenAloneUnconfigures(t *testing.T) {
	src, st, _ := newSource(t)
	connectWorks(t, st, "https://works.example.com", "lwk_secret")
	if err := st.DeletePublishToken(); err != nil {
		t.Fatalf("DeletePublishToken: %v", err)
	}
	r, _ := src.Resolve(settings.ProviderLinettaTrial)
	if r.Configured() {
		t.Errorf("Configured with the account but no token: %v", r)
	}
}

func TestResolveTrial_keepsItsOwnConsent(t *testing.T) {
	src, st, _ := newSource(t)
	connectWorks(t, st, "https://works.example.com", "lwk_secret")
	if _, err := st.Set(context.Background(), settings.Patch{Providers: map[string]settings.ProviderPatch{
		settings.ProviderLinettaTrial: {ConsentedAt: int64Ptr(42)},
	}}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	r, _ := src.Resolve(settings.ProviderLinettaTrial)
	if r.ConsentedAt != 42 {
		t.Errorf("ConsentedAt = %d, want 42", r.ConsentedAt)
	}
}

func TestListTrial_isAWorksRowAndItsModelIsFixed(t *testing.T) {
	src, st, _ := newSource(t)
	connectWorks(t, st, "https://works.example.com", "lwk_secret")
	var row *Status
	for _, s := range src.List() {
		if s.ID == settings.ProviderLinettaTrial {
			row = &s
		}
	}
	if row == nil || row.Auth != "works" || !row.Configured {
		t.Fatalf("trial row = %+v, want a configured works row", row)
	}
	models, err := src.ListModels(context.Background(), settings.ProviderLinettaTrial)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if !slices.Equal(models, []string{TrialModel}) {
		t.Errorf("models = %v, want only %q", models, TrialModel)
	}
}
