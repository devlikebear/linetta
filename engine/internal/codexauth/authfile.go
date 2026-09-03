package codexauth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// authFileName is what the Codex CLI calls its credential file, and what tars
// looks for under CodexHome.
const authFileName = "auth.json"

// Tokens is the credential set the issuer returns. The JSON names match the
// Codex CLI's file exactly so the two tools can read one another's login.
type Tokens struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id"`
}

// authFile is the whole on-disk document.
type authFile struct {
	AuthMode    string `json:"auth_mode"`
	Tokens      Tokens `json:"tokens"`
	LastRefresh string `json:"last_refresh"`
}

// AuthPath is where the credential lives under a Codex home directory.
func AuthPath(codexHome string) string {
	return filepath.Join(codexHome, authFileName)
}

// writeAuthFile stores a completed login. The write goes to a temporary file
// first and is then renamed, so a crash mid-write cannot leave a half-written
// credential that reads as a broken login.
//
// The temporary file is created with os.CreateTemp — a fresh, exclusively
// created, randomly named 0600 file — rather than written at a fixed
// auth.json.tmp path. A fixed name opens with O_CREATE|O_TRUNC, which follows
// a symlink and inherits an existing file's mode: a symlink planted while the
// directory was still world-writable would survive the Chmod above (the
// directory is tightened, the link is not) and then receive the token, and a
// pre-existing 0644 temp file would rename into a 0644 credential. It also
// gives two concurrent writers separate files instead of one shared one.
func writeAuthFile(codexHome string, tok Tokens, now time.Time) error {
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		return fmt.Errorf("codexauth: create %s: %w", codexHome, err)
	}
	// Ensure the directory is 0700 even if it already existed with looser perms.
	if err := os.Chmod(codexHome, 0o700); err != nil {
		return fmt.Errorf("codexauth: chmod %s: %w", codexHome, err)
	}
	body, err := json.MarshalIndent(authFile{
		AuthMode:    "chatgpt",
		Tokens:      tok,
		LastRefresh: now.UTC().Format(time.RFC3339),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("codexauth: encode auth file: %w", err)
	}
	f, err := os.CreateTemp(codexHome, "auth-*.json")
	if err != nil {
		return fmt.Errorf("codexauth: create temp auth file: %w", err)
	}
	tmp := f.Name()
	// Removing the temp file is a no-op once the rename below has moved it,
	// and the cleanup that matters on every path that did not get that far.
	defer func() { _ = os.Remove(tmp) }()
	// CreateTemp already opens at 0600, but a umask can only take bits away,
	// never add them — say the intended mode outright so the permission the
	// credential ends up with does not depend on reading that guarantee.
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return fmt.Errorf("codexauth: chmod temp auth file: %w", err)
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		return fmt.Errorf("codexauth: write auth file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("codexauth: write auth file: %w", err)
	}
	if err := os.Rename(tmp, AuthPath(codexHome)); err != nil {
		return fmt.Errorf("codexauth: install auth file: %w", err)
	}
	return nil
}

// readAuthFile loads the stored login. A missing file returns an error that
// satisfies os.IsNotExist, which is how callers tell "not logged in" apart
// from "the file is broken".
func readAuthFile(codexHome string) (authFile, error) {
	raw, err := os.ReadFile(AuthPath(codexHome))
	if err != nil {
		return authFile{}, err
	}
	var f authFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return authFile{}, fmt.Errorf("codexauth: parse auth file: %w", err)
	}
	return f, nil
}

// Logout deletes the stored login. Removing a file that is not there is a
// success: the writer asked to end up logged out, and they are.
func Logout(codexHome string) error {
	err := os.Remove(AuthPath(codexHome))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("codexauth: remove auth file: %w", err)
	}
	return nil
}

// idClaims is the subset of the id_token payload worth reading.
type idClaims struct {
	Email string `json:"email"`
	Exp   int64  `json:"exp"`
	Auth  struct {
		ChatGPTAccountID string `json:"chatgpt_account_id"`
	} `json:"https://api.openai.com/auth"`
}

// claimsFromIDToken decodes the payload segment of the id_token. The signature
// is not verified and does not need to be: this token came back over TLS from
// the token endpoint we called ourselves, so there is no untrusted hop to
// guard against, and verifying would drag in a JWKS client for nothing.
//
// Anything malformed yields zero values rather than an error — these claims
// are for display, and a login is not worth failing over an unreadable email.
func claimsFromIDToken(idToken string) (email string, accountID string, expiresAt int64) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return "", "", 0
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", 0
	}
	var c idClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		return "", "", 0
	}
	return c.Email, c.Auth.ChatGPTAccountID, c.Exp
}
