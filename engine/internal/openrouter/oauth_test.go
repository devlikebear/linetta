package openrouter

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCodeChallengeS256(t *testing.T) {
	got := codeChallengeS256("test-verifier")
	want := "JBbiqONGWPaAmwXk_8bT6UnlPfrn65D32eZlJS-zGG0"
	if got != want {
		t.Fatalf("challenge=%q, want %q", got, want)
	}
}

func TestOAuthManagerStartAndFinishExchangesCallbackCode(t *testing.T) {
	var postedBody string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/keys" {
			t.Fatalf("path=%q, want /auth/keys", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		postedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"key":"or-oauth-key"}`))
	}))
	defer api.Close()

	mgr := NewOAuthManager(OAuthConfig{
		APIBaseURL:  api.URL,
		AuthBaseURL: api.URL + "/auth",
		HTTPClient:  api.Client(),
	})
	start, err := mgr.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(start.AuthURL, "code_challenge_method=S256") {
		t.Fatalf("auth url missing challenge method: %s", start.AuthURL)
	}

	go func() {
		_, _ = http.Get(start.CallbackURL + "?code=callback-code")
	}()
	key, err := mgr.Finish(context.Background(), start.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if key != "or-oauth-key" {
		t.Fatalf("key=%q", key)
	}
	if !strings.Contains(postedBody, `"code":"callback-code"`) || !strings.Contains(postedBody, `"code_challenge_method":"S256"`) {
		t.Fatalf("exchange body mismatch: %s", postedBody)
	}
}
