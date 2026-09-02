package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

func newSettingsFixture(t *testing.T) *settings.Store {
	t.Helper()
	t.Setenv("LINETTA_HOME", t.TempDir())
	s, err := settings.NewWithSecretStore(settings.NewMemorySecretStore())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestGetSettingsHandler_returnsDefaults(t *testing.T) {
	store := newSettingsFixture(t)
	res, err := GetSettings(store)(context.Background(), nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got settings.Config
	if err := json.Unmarshal(res, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Provider != "openai-codex" {
		t.Errorf("provider = %q", got.Provider)
	}
	if got.BackupDir == "" {
		t.Error("backup_dir not surfaced")
	}
}

func TestSetSettingsHandler_partial(t *testing.T) {
	store := newSettingsFixture(t)
	res, err := SetSettings(store)(context.Background(), json.RawMessage(`{"theme":"dark"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got settings.Config
	_ = json.Unmarshal(res, &got)
	if got.Theme != "dark" {
		t.Errorf("theme not applied: %+v", got)
	}
}

func TestSetSettingsHandler_invalidValueIsRejected(t *testing.T) {
	store := newSettingsFixture(t)
	_, err := SetSettings(store)(context.Background(), json.RawMessage(`{"theme":"nope"}`))
	if err == nil {
		t.Error("expected validation error")
	}
}

// The provider fields are back on the patch surface (#90/#91). An unknown id
// is rejected outright rather than silently dropped.
func TestSetSettingsHandler_rejectsUnknownProvider(t *testing.T) {
	store := newSettingsFixture(t)
	_, err := SetSettings(store)(context.Background(),
		json.RawMessage(`{"provider":"nope","theme":"dark"}`))
	if err == nil {
		t.Fatal("expected an error for an unknown provider")
	}
}

func TestSetSettingsHandler_appliesProviderPatch(t *testing.T) {
	store := newSettingsFixture(t)
	res, err := SetSettings(store)(context.Background(), json.RawMessage(
		`{"provider":"anthropic","providers":{"anthropic":{"model":"claude-sonnet-4-5","consented_at":1700000000000}}}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got settings.Config
	_ = json.Unmarshal(res, &got)
	if got.Provider != "anthropic" {
		t.Errorf("provider = %q", got.Provider)
	}
	if got.Providers["anthropic"].Model != "claude-sonnet-4-5" || got.Providers["anthropic"].ConsentedAt != 1700000000000 {
		t.Errorf("provider patch not applied: %+v", got.Providers["anthropic"])
	}
}
