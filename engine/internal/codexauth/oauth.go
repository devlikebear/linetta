// Package codexauth logs a writer into their ChatGPT account so the built-in
// agent can call Codex on their subscription (#92).
//
// tars already reads the credential file and refreshes an expired token by
// itself; the only thing missing was the first login. So this package performs
// the OAuth PKCE dance and writes the same auth.json the Codex CLI writes —
// deliberately the same format, so the two tools can share one file.
//
// The protocol constants below come from the Codex CLI's own source
// (openai/codex, codex-rs/login), read on 2026-09-02. They are constants on
// purpose: an env-var override here would be a way to point Linetta's login at
// somebody else's endpoint.
package codexauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
)

const (
	// Issuer is OpenAI's OAuth host.
	Issuer = "https://auth.openai.com"
	// ClientID is the Codex CLI's registered public client.
	ClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	// CallbackPath is the redirect path registered with that client.
	CallbackPath = "/auth/callback"
	// PrimaryPort and FallbackPort are the only two ports the registered
	// redirect URIs allow. A third port would be rejected by the issuer, so a
	// busy pair is an error the writer has to resolve, not something to route
	// around.
	PrimaryPort  = 1455
	FallbackPort = 1457
	// Scope is what the Codex CLI requests.
	Scope = "openid profile email offline_access api.connectors.read api.connectors.invoke"

	authorizeEndpoint = Issuer + "/oauth/authorize"
	tokenEndpoint     = Issuer + "/oauth/token"
)

// randomURLSafe returns n random bytes as base64url text without padding.
func randomURLSafe(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("codexauth: read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// newPKCE returns a fresh verifier and its S256 challenge (RFC 7636). 32 bytes
// encode to 43 characters, the shortest length the RFC allows.
func newPKCE() (verifier, challenge string, err error) {
	verifier, err = randomURLSafe(32)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// newState returns the CSRF value the callback must echo back.
func newState() (string, error) { return randomURLSafe(32) }

// redirectURI is the address the issuer sends the browser back to. It says
// "localhost" rather than 127.0.0.1 because that is the string registered with
// the client, and the issuer compares it literally.
func redirectURI(port int) string {
	return fmt.Sprintf("http://localhost:%d%s", port, CallbackPath)
}

// authorizeURL builds the address the OS browser opens.
func authorizeURL(challenge, state string, port int) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", ClientID)
	q.Set("redirect_uri", redirectURI(port))
	q.Set("scope", Scope)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	// The Codex CLI sends these three; the issuer's simplified consent screen
	// and the organization claims in the id_token depend on them.
	q.Set("id_token_add_organizations", "true")
	q.Set("codex_cli_simplified_flow", "true")
	q.Set("originator", "linetta")
	return authorizeEndpoint + "?" + q.Encode()
}
