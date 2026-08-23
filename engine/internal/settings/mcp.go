package settings

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MCP access modes. off is the default: no listener binds until the writer
// turns it on. read_only registers only the read tools, so a misbehaving agent
// cannot reach a write tool at all — the guarantee is "not registered", not
// "registered and refused".
const (
	MCPModeOff      = "off"
	MCPModeReadOnly = "read_only"
	MCPModeFull     = "full"
)

// DefaultMCPPort is fixed rather than ephemeral: a client config is written
// once and reused for months, and Claude Code has no client-side way to absorb
// a moving URL. A busy port surfaces as a visible error instead of a silent
// fallback.
const DefaultMCPPort = 7391

// MCPConsentVersion is the current MCP data-sharing consent revision. Separate
// from the AI provider consent: that one covers text Linetta sends to a
// provider it configured, this one covers a third-party client Linetta does
// not control.
const MCPConsentVersion = 1

// ValidMCPModes returns the accepted mcp_mode values.
func ValidMCPModes() []string {
	return []string{MCPModeOff, MCPModeReadOnly, MCPModeFull}
}

// MCPMode returns the configured access mode, normalized.
func (s *Store) MCPMode() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	mode := s.cfg.MCPMode
	for _, valid := range ValidMCPModes() {
		if mode == valid {
			return mode
		}
	}
	return MCPModeOff
}

// MCPPort returns the configured loopback port, normalized.
func (s *Store) MCPPort() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.MCPPort < 1024 || s.cfg.MCPPort > 65535 {
		return DefaultMCPPort
	}
	return s.cfg.MCPPort
}

// MCPProjectID returns the work the server is restricted to, or "" for all.
func (s *Store) MCPProjectID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.MCPProjectID
}

// HasMCPConsent reports whether the writer accepted the current MCP consent
// revision. The host refuses to start without it.
func (s *Store) HasMCPConsent() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.MCPConsentVersion >= MCPConsentVersion
}

// mcpTokenFileName holds the bearer token on platforms with no secure secret
// backend (Linux, as of today — see secrets_unsupported.go).
//
// Plaintext at 0600 is acceptable for THIS secret specifically: while the
// server runs, mcp.json already carries the same token at 0600 so the bridge
// can find it, and any process running as this user can read library.db
// directly. Provider API keys deliberately do NOT get this fallback — they are
// long-lived credentials for third-party accounts, and quietly starting to
// store them in plaintext is not a change a writer opted into.
const mcpTokenFileName = "mcp-token"

func (s *Store) mcpTokenPath() string {
	return filepath.Join(s.dir, mcpTokenFileName)
}

// MCPToken returns the bearer token, or "" when none has been generated.
func (s *Store) MCPToken() string {
	if secret, ok, err := s.secrets.Get(mcpTokenSecretName); err == nil && ok {
		return secret
	}
	raw, err := os.ReadFile(s.mcpTokenPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// MCPTokenExists reports whether a token has been minted, WITHOUT reading its
// value. settings.get must never read secret values — on macOS that can prompt
// the Keychain, and the redacted view only needs presence.
func (s *Store) MCPTokenExists() bool {
	if ok, err := s.secrets.Exists(mcpTokenSecretName); err == nil && ok {
		return true
	}
	_, err := os.Stat(s.mcpTokenPath())
	return err == nil
}

// EnsureMCPToken returns the existing token, generating one on first use so
// enabling MCP never leaves the server unauthenticated.
func (s *Store) EnsureMCPToken() (string, error) {
	if token := s.MCPToken(); token != "" {
		return token, nil
	}
	return s.RegenerateMCPToken()
}

// RegenerateMCPToken issues a fresh token, invalidating every client config
// that carried the old one.
func (s *Store) RegenerateMCPToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate mcp token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	if err := s.secrets.Set(mcpTokenSecretName, token); err != nil {
		// No secure backend on this platform. Fall back to a 0600 file rather
		// than leaving MCP unusable — Linux is a shipping platform, and a
		// server that cannot mint a token cannot start at all.
		if writeErr := os.WriteFile(s.mcpTokenPath(), []byte(token), 0o600); writeErr != nil {
			return "", fmt.Errorf("store mcp token: %w", err)
		}
	}
	return token, nil
}

// DeleteMCPToken removes the token entirely, from both possible locations.
func (s *Store) DeleteMCPToken() error {
	err := s.secrets.Delete(mcpTokenSecretName)
	if rmErr := os.Remove(s.mcpTokenPath()); rmErr != nil && !os.IsNotExist(rmErr) && err == nil {
		err = rmErr
	}
	return err
}
