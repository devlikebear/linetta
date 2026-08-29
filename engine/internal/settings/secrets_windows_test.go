//go:build windows

package settings

import (
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

// The settings patch can no longer write provider or web-search keys — the
// companion that used them is gone (#47). What still matters is that a key a
// writer saved before the removal is moved out of settings.json and into the
// OS credential store on load, so it is not left sitting in a plaintext file
// that nothing reads any more.
func TestStore_migratesADiskKeyIntoTheCredentialManager(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	secrets := newTestCredentialStore(t)
	t.Cleanup(func() { _ = secrets.Delete(webSearchAPIKeySecretName) })

	const key = "BSA-left-over-from-before"
	seed := `{"web_search_api_key":"` + key + `"}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(seed), 0o600); err != nil {
		t.Fatalf("seed settings.json: %v", err)
	}

	if _, err := NewWithSecretStore(secrets); err != nil {
		t.Fatalf("New: %v", err)
	}

	stored, ok, err := secrets.Get(webSearchAPIKeySecretName)
	if err != nil || !ok || stored != key {
		t.Fatalf("migrated key = (%q, %v, %v), want (%q, true, nil)", stored, ok, err, key)
	}
	onDisk, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	if strings.Contains(string(onDisk), key) {
		t.Error("the key was left in settings.json")
	}
}
