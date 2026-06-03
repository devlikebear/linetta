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
	res, err := SetSettings(store)(context.Background(), json.RawMessage(`{"provider":"openai-codex"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got settings.Config
	_ = json.Unmarshal(res, &got)
	if got.Provider != "openai-codex" {
		t.Errorf("provider not applied: %+v", got)
	}
}

func TestSetSettingsHandler_invalidProvider(t *testing.T) {
	store := newSettingsFixture(t)
	_, err := SetSettings(store)(context.Background(), json.RawMessage(`{"provider":"nope"}`))
	if err == nil {
		t.Error("expected validation error")
	}
}
