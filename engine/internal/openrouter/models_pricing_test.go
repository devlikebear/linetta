package openrouter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// OpenRouter serves pricing objects whose values are not all strings: the
// "overrides" key carries an array of time-window price tables. A flat
// map[string]string rejected the whole response, so one model with overrides
// discarded the entire catalogue.
const modelsWithPricingOverrides = `{"data":[
  {"id":"openai/gpt-5.4","name":"GPT-5.4","context_length":400000,
   "pricing":{"prompt":"0.00000125","completion":"0.00001"}},
  {"id":"deepseek/deepseek-v4-flash-vision-exp","name":"DeepSeek V4",
   "pricing":{"prompt":"0.00000022","completion":"0.00000066",
     "overrides":[{"utc_start":1000,"utc_end":100,"prompt":"0.00000022"}]}},
  {"id":"openrouter/auto","name":"Auto Router","pricing":{"prompt":"-1"}}
]}`

func modelsServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write body: %v", err)
		}
	}))
}

func TestModels_keepsCatalogueWhenPricingHasStructuredValues(t *testing.T) {
	srv := modelsServer(t, modelsWithPricingOverrides)
	defer srv.Close()

	models, err := NewClient(srv.URL, srv.Client()).Models(context.Background(), "test-key")
	if err != nil {
		t.Fatalf("Models() error = %v, want nil", err)
	}
	if len(models) != 3 {
		t.Fatalf("len(models) = %d, want 3", len(models))
	}
	want := []string{"openai/gpt-5.4", "deepseek/deepseek-v4-flash-vision-exp", "openrouter/auto"}
	for i, id := range want {
		if models[i].ID != id {
			t.Errorf("models[%d].ID = %q, want %q", i, models[i].ID, id)
		}
	}
}

func TestModels_dropsOnlyTheNonStringPricingEntry(t *testing.T) {
	srv := modelsServer(t, modelsWithPricingOverrides)
	defer srv.Close()

	models, err := NewClient(srv.URL, srv.Client()).Models(context.Background(), "test-key")
	if err != nil {
		t.Fatalf("Models() error = %v", err)
	}
	pricing := models[1].Pricing
	if got := pricing["prompt"]; got != "0.00000022" {
		t.Errorf("pricing[prompt] = %q, want 0.00000022", got)
	}
	if got := pricing["completion"]; got != "0.00000066" {
		t.Errorf("pricing[completion] = %q, want 0.00000066", got)
	}
	if _, ok := pricing["overrides"]; ok {
		t.Error("pricing kept the structured overrides entry, want it skipped")
	}
}

func TestModels_stillFailsOnMalformedJSON(t *testing.T) {
	srv := modelsServer(t, `{"data":[{"id":"a","pricing":`)
	defer srv.Close()

	if _, err := NewClient(srv.URL, srv.Client()).Models(context.Background(), "test-key"); err == nil {
		t.Fatal("Models() error = nil, want a decode error")
	}
}
