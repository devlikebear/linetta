//go:build windows

package settings

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These tests talk to the real Credential Manager. They use a per-run target
// prefix and delete what they create, so they neither collide with a developer's
// actual Linetta credentials nor leave entries behind.
func newTestCredentialStore(t *testing.T) credentialSecretStore {
	t.Helper()
	prefix := fmt.Sprintf("devlikebear.linetta.test.%d:", time.Now().UnixNano())
	return credentialSecretStore{prefix: prefix}
}

func TestCredentialSecretStore_roundTrip(t *testing.T) {
	s := newTestCredentialStore(t)
	const name = "provider.test.api_key"
	t.Cleanup(func() {
		if err := s.Delete(name); err != nil {
			t.Errorf("cleanup Delete: %v", err)
		}
	})

	if ok, err := s.Exists(name); err != nil || ok {
		t.Fatalf("Exists before write = (%v, %v), want (false, nil)", ok, err)
	}
	if got, ok, err := s.Get(name); err != nil || ok || got != "" {
		t.Fatalf("Get before write = (%q, %v, %v), want (\"\", false, nil)", got, ok, err)
	}

	if err := s.Set(name, "sk-first"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, ok, err := s.Get(name)
	if err != nil || !ok || got != "sk-first" {
		t.Fatalf("Get after write = (%q, %v, %v), want (\"sk-first\", true, nil)", got, ok, err)
	}
	if ok, err := s.Exists(name); err != nil || !ok {
		t.Fatalf("Exists after write = (%v, %v), want (true, nil)", ok, err)
	}

	// A second Set must replace rather than duplicate or fail.
	if err := s.Set(name, "sk-second"); err != nil {
		t.Fatalf("Set overwrite: %v", err)
	}
	if got, _, _ := s.Get(name); got != "sk-second" {
		t.Fatalf("Get after overwrite = %q, want sk-second", got)
	}

	if err := s.Delete(name); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if ok, err := s.Exists(name); err != nil || ok {
		t.Fatalf("Exists after delete = (%v, %v), want (false, nil)", ok, err)
	}
}

func TestCredentialSecretStore_deleteMissingIsNotAnError(t *testing.T) {
	s := newTestCredentialStore(t)
	if err := s.Delete("never.written"); err != nil {
		t.Fatalf("Delete of absent entry: %v", err)
	}
}

func TestCredentialSecretStore_setEmptyDeletes(t *testing.T) {
	s := newTestCredentialStore(t)
	const name = "web_search.api_key"
	t.Cleanup(func() { _ = s.Delete(name) })

	if err := s.Set(name, "BSA-token"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Set(name, ""); err != nil {
		t.Fatalf("Set empty: %v", err)
	}
	if ok, err := s.Exists(name); err != nil || ok {
		t.Fatalf("Exists after empty Set = (%v, %v), want (false, nil)", ok, err)
	}
}

func TestCredentialSecretStore_preservesExactBytes(t *testing.T) {
	s := newTestCredentialStore(t)
	cases := map[string]string{
		"ascii":   "sk-proj-0123456789abcdef",
		"unicode": "키-값-🔑-テスト",
		"padded":  "  leading and trailing  ",
		"newline": "line1\nline2",
		"long":    strings.Repeat("k", credMaxBlobSize),
	}
	for label, value := range cases {
		name := "roundtrip." + label
		if err := s.Set(name, value); err != nil {
			t.Fatalf("%s: Set: %v", label, err)
		}
		got, ok, err := s.Get(name)
		if err != nil || !ok {
			t.Fatalf("%s: Get = (%v, %v)", label, ok, err)
		}
		if got != value {
			t.Errorf("%s: Get = %q, want %q", label, got, value)
		}
		if err := s.Delete(name); err != nil {
			t.Errorf("%s: Delete: %v", label, err)
		}
	}
}

func TestCredentialSecretStore_rejectsOversizedValue(t *testing.T) {
	s := newTestCredentialStore(t)
	err := s.Set("too.big", strings.Repeat("k", credMaxBlobSize+1))
	if err == nil {
		t.Fatal("Set of an oversized value should error")
	}
	if !strings.Contains(err.Error(), "over the") {
		t.Errorf("error should explain the size limit, got: %v", err)
	}
}

// The store is reached through the SecretStore interface everywhere else, so
// make sure the concrete type actually satisfies it.
func TestCredentialSecretStore_implementsSecretStore(t *testing.T) {
	var _ SecretStore = credentialSecretStore{}
	if _, ok := defaultSecretStore().(credentialSecretStore); !ok {
		t.Fatalf("defaultSecretStore() = %T, want credentialSecretStore on Windows", defaultSecretStore())
	}
}

// End to end through the same Store calls the UI drives: saving a web_search
// key used to fail outright on Windows because there was no secret backend.
func TestStore_keysPersistThroughCredentialManager(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	secrets := newTestCredentialStore(t)
	t.Cleanup(func() {
		_ = secrets.Delete(webSearchAPIKeySecretName)
		_ = secrets.Delete(providerAPIKeySecretName(ProviderOpenRouter))
	})

	s, err := NewWithSecretStore(secrets)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	key := "BSA-windows-round-trip"
	if _, err := s.Set(ctx, Patch{WebSearchAPIKey: &key}); err != nil {
		t.Fatalf("Set web_search key: %v", err)
	}
	if got := s.WebSearchAPIKey(); got != key {
		t.Fatalf("WebSearchAPIKey() = %q, want %q", got, key)
	}

	// settings.get must report presence without leaking the value, and the
	// value must never reach settings.json on disk.
	view, err := s.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !view.WebSearchAPIKeySet {
		t.Error("web_search_api_key_set should be true after saving a key")
	}
	if view.WebSearchAPIKey != "" {
		t.Errorf("settings.get leaked the key: %q", view.WebSearchAPIKey)
	}
	onDisk, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	if strings.Contains(string(onDisk), key) {
		t.Error("the key was written to settings.json")
	}

	// Provider keys take the same path.
	provider := ProviderOpenRouter
	if _, err := s.Set(ctx, Patch{
		Provider:  &provider,
		Providers: map[string]ProviderConfig{provider: {APIKey: "sk-or-windows"}},
	}); err != nil {
		t.Fatalf("Set provider key: %v", err)
	}
	stored, ok, err := secrets.Get(providerAPIKeySecretName(provider))
	if err != nil || !ok || stored != "sk-or-windows" {
		t.Fatalf("provider key = (%q, %v, %v), want (\"sk-or-windows\", true, nil)", stored, ok, err)
	}

	// Clearing removes the credential rather than leaving a stale entry.
	empty := ""
	if _, err := s.Set(ctx, Patch{WebSearchAPIKey: &empty}); err != nil {
		t.Fatalf("clear web_search key: %v", err)
	}
	if ok, err := secrets.Exists(webSearchAPIKeySecretName); err != nil || ok {
		t.Fatalf("web_search credential after clear = (%v, %v), want (false, nil)", ok, err)
	}
}
