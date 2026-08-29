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

// The provider fields are no longer part of the patch surface (#47). A stale
// client sending one must not be able to write it back.
func TestSetSettingsHandler_ignoresRetiredProviderFields(t *testing.T) {
	store := newSettingsFixture(t)
	res, err := SetSettings(store)(context.Background(),
		json.RawMessage(`{"provider":"nope","theme":"dark"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got settings.Config
	_ = json.Unmarshal(res, &got)
	if got.Provider == "nope" {
		t.Error("a retired field was written through the patch surface")
	}
	if got.Theme != "dark" {
		t.Errorf("the rest of the patch was not applied: %+v", got)
	}
}
