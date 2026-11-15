package publish

import (
	"context"
	"slices"
	"testing"
)

func TestTrialStatus_readsTheAccountFromTheSite(t *testing.T) {
	f := newServiceFixture(t, 1)
	f.site.mu.Lock()
	f.site.trial = TrialStatus{State: TrialAvailable, GrantedTokens: 3000000, UsedTokens: 412345, RemainingTokens: 2587655}
	f.site.mu.Unlock()
	got := f.svc.TrialStatus(context.Background())
	want := TrialStatus{State: TrialAvailable, GrantedTokens: 3000000, UsedTokens: 412345, RemainingTokens: 2587655}
	if got != want {
		t.Errorf("TrialStatus = %+v, want %+v", got, want)
	}
}

func TestTrialStatus_askingNothingWithoutAConnection(t *testing.T) {
	f := newServiceFixture(t, 1)
	if err := NewCredentials(f.settings).Clear(); err != nil {
		t.Fatal(err)
	}
	before := len(f.site.seen())
	if got := f.svc.TrialStatus(context.Background()); got.State != TrialNotConnected {
		t.Errorf("state = %q, want %q", got.State, TrialNotConnected)
	}
	if after := f.site.seen(); len(after) != before {
		t.Errorf("asked the site with no connection: %v", after[before:])
	}
}

func TestTrialStatus_aRefusedTokenIsForgotten(t *testing.T) {
	f := newServiceFixture(t, 1)
	f.site.failNext("GET /api/artworks/trial/me", 401, CodeUnauthorized)
	if got := f.svc.TrialStatus(context.Background()); got.State != TrialNotConnected {
		t.Errorf("state = %q, want %q", got.State, TrialNotConnected)
	}
	if _, ok := f.settings.PublishAccount(); ok {
		t.Error("the refused connection is still stored")
	}
}

func TestTrialStatus_anUnansweringSiteIsUnreachable(t *testing.T) {
	f := newServiceFixture(t, 1)
	f.site.failNext("GET /api/artworks/trial/me", 503, "trial_unavailable")
	if got := f.svc.TrialStatus(context.Background()); got.State != TrialUnreachable {
		t.Errorf("state = %q, want %q", got.State, TrialUnreachable)
	}
	if _, ok := f.settings.PublishAccount(); !ok {
		t.Error("an outage forgot the connection")
	}
	if !slices.Contains(f.site.seen(), "GET /api/artworks/trial/me") {
		t.Error("the site was never asked")
	}
}
