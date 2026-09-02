package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/tars/pkg/llm"
)

type fakeFetcher struct {
	models []string
	err    error
	seen   llm.ProviderOptions
}

func (f *fakeFetcher) FetchModels(_ context.Context, opts llm.ProviderOptions) ([]string, error) {
	f.seen = opts
	return f.models, f.err
}

func TestList_reportsTheFourInOrderWithState(t *testing.T) {
	src, st, _ := newSource(t)
	configure(t, st, "anthropic", "sk-ant-test", true)
	got := src.List()
	if len(got) != 4 {
		t.Fatalf("List = %d entries, want 4", len(got))
	}
	wantIDs := []string{"openai-codex", "anthropic", "gemini-native", "openai"}
	for i, id := range wantIDs {
		if got[i].ID != id {
			t.Errorf("List[%d].ID = %q, want %q", i, got[i].ID, id)
		}
	}
	if got[0].Auth != "oauth" || got[1].Auth != "api_key" {
		t.Errorf("auth kinds = %q/%q", got[0].Auth, got[1].Auth)
	}
	a := got[1]
	if !a.Active || !a.Configured || !a.Consented {
		t.Errorf("anthropic = %+v, want active+configured+consented", a)
	}
	if o := got[3]; o.Active || o.Configured || o.Consented {
		t.Errorf("openai = %+v, want nothing set", o)
	}
}

func TestListModels_needsACredentialButNotConsent(t *testing.T) {
	src, st, _ := newSource(t)
	f := &fakeFetcher{models: []string{"b-model", "a-model"}}
	src.WithFetcher(f)

	_, err := src.ListModels(context.Background(), "anthropic")
	if reasonOf(t, err) != rpc.ReasonProviderNotConfigured {
		t.Fatalf("without a key: %v", err)
	}

	configure(t, st, "anthropic", "sk-ant-test", false) // key, no consent
	models, err := src.ListModels(context.Background(), "anthropic")
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 || models[0] != "a-model" {
		t.Errorf("models = %v, want sorted", models)
	}
	if f.seen.Provider != "anthropic" || f.seen.APIKey != "sk-ant-test" {
		t.Errorf("fetcher saw %+v", f.seen)
	}
}

func TestListModels_classifiesFetchFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"401", &llm.ProviderError{Provider: "anthropic", StatusCode: 401, Message: `{"error":"bad key"}`}, rpc.ReasonProviderAuthFailed},
		{"429", &llm.ProviderError{Provider: "anthropic", StatusCode: 429, Message: "slow down"}, rpc.ReasonProviderRateLimited},
		{"network", errors.New("dial tcp: connection refused"), rpc.ReasonProviderUnreachable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, st, _ := newSource(t)
			configure(t, st, "anthropic", "sk-ant-test", false)
			src.WithFetcher(&fakeFetcher{err: tc.err})
			_, err := src.ListModels(context.Background(), "anthropic")
			if got := reasonOf(t, err); got != tc.want {
				t.Errorf("reason = %q, want %q (%v)", got, tc.want, err)
			}
			if got := err.Error(); len(got) > 260 {
				t.Errorf("message not capped: %d chars", len(got))
			}
		})
	}
}

func TestTest_refusesWithoutConsentAndNeverDials(t *testing.T) {
	src, st, _ := newSource(t)
	configure(t, st, "anthropic", "sk-ant-test", false)
	src.WithFactory(func(llm.ProviderOptions) (llm.Client, error) {
		t.Fatal("no client may be built before consent")
		return nil, nil
	})
	err := src.Test(context.Background(), "anthropic")
	if reasonOf(t, err) != rpc.ReasonProviderConsentRequired {
		t.Errorf("reason = %v", err)
	}
}

func TestTest_asksOnceAndClassifiesTheAnswer(t *testing.T) {
	src, st, _ := newSource(t)
	configure(t, st, "anthropic", "sk-ant-test", true)
	calls := 0
	src.WithFactory(func(llm.ProviderOptions) (llm.Client, error) {
		return fakeClient{ask: func(string) (string, error) {
			calls++
			return "OK", nil
		}}, nil
	})
	if err := src.Test(context.Background(), "anthropic"); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if calls != 1 {
		t.Errorf("Ask called %d times, want 1", calls)
	}

	src.WithFactory(func(llm.ProviderOptions) (llm.Client, error) {
		return fakeClient{ask: func(string) (string, error) {
			return "", &llm.ProviderError{Provider: "anthropic", StatusCode: 401, Message: "nope"}
		}}, nil
	})
	err := src.Test(context.Background(), "anthropic")
	if reasonOf(t, err) != rpc.ReasonProviderAuthFailed {
		t.Errorf("reason = %v", err)
	}
}

func TestClassify_passesReasonErrorsThroughAndNilStaysNil(t *testing.T) {
	in := &rpc.ReasonError{Reason: rpc.ReasonProviderConsentRequired}
	if out := Classify("anthropic", in); out != in {
		t.Errorf("Classify rewrapped a ReasonError: %v", out)
	}
	if Classify("anthropic", nil) != nil {
		t.Error("Classify(nil) must be nil")
	}
}
