package codexauth

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// fakeIDToken builds an unsigned JWT whose payload carries the claims the
// Codex issuer sends. Only the payload segment is ever read.
func fakeIDToken(t *testing.T, email, accountID string, exp int64) string {
	t.Helper()
	payload := map[string]any{
		"email": email,
		"exp":   exp,
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": accountID,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return "ignored-header." + base64.RawURLEncoding.EncodeToString(body) + ".ignored-signature"
}

func TestWriteAuthFile_matchesTheCodexCLIShape(t *testing.T) {
	home := t.TempDir()
	tok := Tokens{
		IDToken:      "id-token",
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		AccountID:    "acct-123",
	}
	at := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	if err := writeAuthFile(home, tok, at); err != nil {
		t.Fatalf("writeAuthFile: %v", err)
	}

	raw, err := os.ReadFile(AuthPath(home))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["auth_mode"] != "chatgpt" {
		t.Errorf("auth_mode = %v, want chatgpt", got["auth_mode"])
	}
	if got["last_refresh"] != "2026-09-03T10:00:00Z" {
		t.Errorf("last_refresh = %v", got["last_refresh"])
	}
	tokens, ok := got["tokens"].(map[string]any)
	if !ok {
		t.Fatalf("tokens is not an object: %v", got["tokens"])
	}
	for k, want := range map[string]string{
		"id_token": "id-token", "access_token": "access-token",
		"refresh_token": "refresh-token", "account_id": "acct-123",
	} {
		if tokens[k] != want {
			t.Errorf("tokens.%s = %v, want %q", k, tokens[k], want)
		}
	}
}

func TestWriteAuthFile_permissionsKeepTheTokenPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits do not apply on Windows")
	}
	home := filepath.Join(t.TempDir(), "codex")
	if err := writeAuthFile(home, Tokens{AccessToken: "a"}, time.Now()); err != nil {
		t.Fatalf("writeAuthFile: %v", err)
	}
	fi, err := os.Stat(AuthPath(home))
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("auth.json mode = %o, want 600", perm)
	}
	di, err := os.Stat(home)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := di.Mode().Perm(); perm != 0o700 {
		t.Errorf("codex dir mode = %o, want 700", perm)
	}
}

func TestWriteAuthFile_replacesAnExistingLogin(t *testing.T) {
	home := t.TempDir()
	if err := writeAuthFile(home, Tokens{AccessToken: "first"}, time.Now()); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := writeAuthFile(home, Tokens{AccessToken: "second"}, time.Now()); err != nil {
		t.Fatalf("second write: %v", err)
	}
	got, err := readAuthFile(home)
	if err != nil {
		t.Fatalf("readAuthFile: %v", err)
	}
	if got.Tokens.AccessToken != "second" {
		t.Errorf("access token = %q, want the second login", got.Tokens.AccessToken)
	}
}

func TestReadAuthFile_missingFileIsNotFound(t *testing.T) {
	if _, err := readAuthFile(t.TempDir()); !os.IsNotExist(err) {
		t.Fatalf("err = %v, want a not-exist error", err)
	}
}

func TestLogout_removesTheFileAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	if err := writeAuthFile(home, Tokens{AccessToken: "a"}, time.Now()); err != nil {
		t.Fatalf("writeAuthFile: %v", err)
	}
	if err := Logout(home); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := os.Stat(AuthPath(home)); !os.IsNotExist(err) {
		t.Errorf("auth.json survived logout: %v", err)
	}
	// Logging out twice is what a writer does when they are not sure it took.
	if err := Logout(home); err != nil {
		t.Errorf("second Logout: %v", err)
	}
}

func TestClaimsFromIDToken_readsEmailAccountAndExpiry(t *testing.T) {
	token := fakeIDToken(t, "writer@example.com", "acct-999", 1788000000)
	email, accountID, exp := claimsFromIDToken(token)
	if email != "writer@example.com" {
		t.Errorf("email = %q", email)
	}
	if accountID != "acct-999" {
		t.Errorf("accountID = %q", accountID)
	}
	if exp != 1788000000 {
		t.Errorf("exp = %d", exp)
	}
}

func TestClaimsFromIDToken_malformedInputYieldsEmptyValues(t *testing.T) {
	for _, tc := range []struct{ name, token string }{
		{"empty", ""},
		{"not a jwt", "nonsense"},
		{"two segments", "a.b"},
		{"payload is not base64", "a.!!!.c"},
		{"payload is not json", "a." + base64.RawURLEncoding.EncodeToString([]byte("nope")) + ".c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			email, accountID, exp := claimsFromIDToken(tc.token)
			if email != "" || accountID != "" || exp != 0 {
				t.Errorf("got (%q, %q, %d), want zero values", email, accountID, exp)
			}
		})
	}
}

func TestWriteAuthFile_enforcesDirectoryPermissionsEvenWhenPreexisting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits do not apply on Windows")
	}
	// Pre-create the directory with loose permissions (e.g., default 0755).
	home := filepath.Join(t.TempDir(), "codex")
	if err := os.Mkdir(home, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Verify it starts with 0755.
	di, err := os.Stat(home)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}
	if perm := di.Mode().Perm(); perm != 0o755 {
		t.Fatalf("pre-created directory has unexpected perm %o", perm)
	}
	// Write auth file; this should chmod the directory to 0700.
	if err := writeAuthFile(home, Tokens{AccessToken: "a"}, time.Now()); err != nil {
		t.Fatalf("writeAuthFile: %v", err)
	}
	// Verify directory is now 0700.
	di, err = os.Stat(home)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if perm := di.Mode().Perm(); perm != 0o700 {
		t.Errorf("codex dir mode = %o, want 700", perm)
	}
}

func TestReadAuthFile_corruptFileIsDistinguishableFromMissing(t *testing.T) {
	home := t.TempDir()
	authPath := AuthPath(home)
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write garbage to the file instead of valid JSON.
	if err := os.WriteFile(authPath, []byte("not json at all"), 0o600); err != nil {
		t.Fatalf("write garbage: %v", err)
	}
	// readAuthFile should return an error that is NOT os.IsNotExist.
	_, err := readAuthFile(home)
	if err == nil {
		t.Fatal("err = nil, want an error for corrupted JSON")
	}
	if os.IsNotExist(err) {
		t.Errorf("err satisfies os.IsNotExist, want a different error (e.g., JSON parse error)")
	}
}
