package codexauth

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
)

func TestNewPKCE_challengeIsS256OfVerifier(t *testing.T) {
	verifier, challenge, err := newPKCE()
	if err != nil {
		t.Fatalf("newPKCE: %v", err)
	}
	// RFC 7636: the verifier is 43-128 chars from the unreserved set, and the
	// challenge is the base64url-without-padding SHA-256 of the verifier.
	if len(verifier) < 43 || len(verifier) > 128 {
		t.Errorf("verifier length %d out of the RFC 7636 range", len(verifier))
	}
	if strings.ContainsAny(verifier, "+/=") {
		t.Errorf("verifier %q is not base64url without padding", verifier)
	}
	sum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if challenge != want {
		t.Errorf("challenge = %q, want %q", challenge, want)
	}
}

func TestNewPKCE_isDifferentEveryCall(t *testing.T) {
	a, _, err := newPKCE()
	if err != nil {
		t.Fatalf("newPKCE: %v", err)
	}
	b, _, err := newPKCE()
	if err != nil {
		t.Fatalf("newPKCE: %v", err)
	}
	if a == b {
		t.Fatal("two verifiers were identical; the source is not random")
	}
}

func TestNewState_isRandomAndURLSafe(t *testing.T) {
	a, err := newState()
	if err != nil {
		t.Fatalf("newState: %v", err)
	}
	b, _ := newState()
	if a == b {
		t.Fatal("two states were identical")
	}
	if a == "" || strings.ContainsAny(a, "+/=") {
		t.Errorf("state %q is not base64url without padding", a)
	}
}

func TestRedirectURI_usesLocalhostAndTheCodexPath(t *testing.T) {
	if got := redirectURI(1455); got != "http://localhost:1455/auth/callback" {
		t.Errorf("redirectURI(1455) = %q", got)
	}
	if got := redirectURI(1457); got != "http://localhost:1457/auth/callback" {
		t.Errorf("redirectURI(1457) = %q", got)
	}
}

func TestAuthorizeURL_carriesEveryParameterCodexSends(t *testing.T) {
	raw := authorizeURL("test-challenge", "test-state", 1455)
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u.Scheme != "https" || u.Host != "auth.openai.com" || u.Path != "/oauth/authorize" {
		t.Fatalf("endpoint = %s://%s%s, want https://auth.openai.com/oauth/authorize", u.Scheme, u.Host, u.Path)
	}
	q := u.Query()
	want := map[string]string{
		"response_type":              "code",
		"client_id":                  "app_EMoamEEZ73f0CkXaXp7hrann",
		"redirect_uri":               "http://localhost:1455/auth/callback",
		"scope":                      "openid profile email offline_access api.connectors.read api.connectors.invoke",
		"code_challenge":             "test-challenge",
		"code_challenge_method":      "S256",
		"state":                      "test-state",
		"id_token_add_organizations": "true",
		"codex_cli_simplified_flow":  "true",
		"originator":                 "linetta",
	}
	for k, v := range want {
		if got := q.Get(k); got != v {
			t.Errorf("query %s = %q, want %q", k, got, v)
		}
	}
}
