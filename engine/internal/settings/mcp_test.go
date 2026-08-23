package settings

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func configJSON(t *testing.T, c Config) string {
	t.Helper()
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return string(raw)
}

func newMCPStore(t *testing.T) *Store {
	t.Helper()
	t.Setenv("LINETTA_HOME", t.TempDir())
	s, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("NewWithSecretStore: %v", err)
	}
	return s
}

// MCP must be inert until the writer turns it on.
func TestMCPDefaultsAreOff(t *testing.T) {
	s := newMCPStore(t)
	if got := s.MCPMode(); got != MCPModeOff {
		t.Errorf("default mode = %q, want %q", got, MCPModeOff)
	}
	if got := s.MCPPort(); got != DefaultMCPPort {
		t.Errorf("default port = %d, want %d", got, DefaultMCPPort)
	}
	if s.HasMCPConsent() {
		t.Error("consent must not be granted by default")
	}
	if s.MCPToken() != "" {
		t.Error("no token should exist before MCP is enabled")
	}
}

func TestMCPModeRoundTripsAndRejectsUnknown(t *testing.T) {
	s := newMCPStore(t)
	ctx := context.Background()
	for _, mode := range ValidMCPModes() {
		if _, err := s.Set(ctx, Patch{MCPMode: &mode}); err != nil {
			t.Fatalf("Set(%q): %v", mode, err)
		}
		if got := s.MCPMode(); got != mode {
			t.Errorf("mode = %q, want %q", got, mode)
		}
	}
	bogus := "wide_open"
	if _, err := s.Set(ctx, Patch{MCPMode: &bogus}); err == nil {
		t.Fatal("an unknown mode must be rejected, not silently accepted")
	}
}

// A corrupt or hand-edited value must degrade to off, never to an open server.
func TestUnknownModeOnDiskFallsBackToOff(t *testing.T) {
	c := normalizeMCPPreferences(Config{MCPMode: "full-access-please", MCPPort: DefaultMCPPort})
	if c.MCPMode != MCPModeOff {
		t.Fatalf("mode = %q, want %q", c.MCPMode, MCPModeOff)
	}
}

func TestMCPPortValidation(t *testing.T) {
	s := newMCPStore(t)
	ctx := context.Background()
	ok := 8123
	if _, err := s.Set(ctx, Patch{MCPPort: &ok}); err != nil {
		t.Fatalf("Set(port): %v", err)
	}
	if got := s.MCPPort(); got != ok {
		t.Errorf("port = %d, want %d", got, ok)
	}
	for _, bad := range []int{0, 80, 70000} {
		if _, err := s.Set(ctx, Patch{MCPPort: &bad}); err == nil {
			t.Errorf("port %d should be rejected", bad)
		}
	}
	if c := normalizeMCPPreferences(Config{MCPMode: MCPModeOff, MCPPort: 42}); c.MCPPort != DefaultMCPPort {
		t.Errorf("out-of-range disk value = %d, want default %d", c.MCPPort, DefaultMCPPort)
	}
}

// The token follows the api_key convention: stored in the secret store, never
// returned by settings.get, exposed only as a presence flag.
func TestMCPTokenIsRedactedAndPresenceOnly(t *testing.T) {
	s := newMCPStore(t)
	token, err := s.EnsureMCPToken()
	if err != nil {
		t.Fatalf("EnsureMCPToken: %v", err)
	}
	if len(token) < 32 {
		t.Fatalf("token looks too short: %q", token)
	}
	if again, _ := s.EnsureMCPToken(); again != token {
		t.Error("EnsureMCPToken must reuse the existing token")
	}

	got, err := s.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.MCPTokenSet {
		t.Error("mcp_token_set should be true once a token exists")
	}
	blob := configJSON(t, got)
	if strings.Contains(blob, token) {
		t.Fatal("settings.get leaked the raw MCP token")
	}

	rotated, err := s.RegenerateMCPToken()
	if err != nil {
		t.Fatalf("RegenerateMCPToken: %v", err)
	}
	if rotated == token {
		t.Error("regenerating must issue a different token")
	}
	if err := s.DeleteMCPToken(); err != nil {
		t.Fatalf("DeleteMCPToken: %v", err)
	}
	if s.MCPToken() != "" {
		t.Error("token should be gone after delete")
	}
}

// The disk file must never carry the presence flag (it is derived state).
func TestMCPTokenFlagNotPersisted(t *testing.T) {
	c := sanitizeConfigForDisk(Config{MCPTokenSet: true})
	if c.MCPTokenSet {
		t.Fatal("mcp_token_set must be cleared before writing settings.json")
	}
}

func TestMCPConsentGate(t *testing.T) {
	s := newMCPStore(t)
	ctx := context.Background()
	if s.HasMCPConsent() {
		t.Fatal("consent must start ungranted")
	}
	version := MCPConsentVersion
	at := int64(1_700_000_000_000)
	if _, err := s.Set(ctx, Patch{MCPConsentVersion: &version, MCPConsentedAt: &at}); err != nil {
		t.Fatalf("Set(consent): %v", err)
	}
	if !s.HasMCPConsent() {
		t.Error("consent should be granted after accepting the current revision")
	}
}

func TestMCPProjectRestriction(t *testing.T) {
	s := newMCPStore(t)
	if s.MCPProjectID() != "" {
		t.Fatal("no restriction by default")
	}
	id := "proj-1"
	if _, err := s.Set(context.Background(), Patch{MCPProjectID: &id}); err != nil {
		t.Fatalf("Set(project): %v", err)
	}
	if got := s.MCPProjectID(); got != id {
		t.Errorf("project = %q, want %q", got, id)
	}
}
