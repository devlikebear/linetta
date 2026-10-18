package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPublishTokenLivesInTheSecretStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LINETTA_HOME", home)
	secrets := NewMemorySecretStore()
	s, err := NewWithSecretStore(secrets)
	if err != nil {
		t.Fatal(err)
	}
	if s.PublishToken() != "" || s.PublishTokenExists() {
		t.Fatal("a fresh store has no publish token")
	}
	if err := s.SetPublishToken("lwk_abc"); err != nil {
		t.Fatal(err)
	}
	if got, ok, _ := secrets.Get(publishTokenSecretName); !ok || got != "lwk_abc" {
		t.Fatalf("secret store holds %q, %v", got, ok)
	}
	if _, err := os.Stat(filepath.Join(home, publishTokenFileName)); !os.IsNotExist(err) {
		t.Fatal("no fallback file when the secret store works")
	}
	if s.PublishToken() != "lwk_abc" || !s.PublishTokenExists() {
		t.Fatal("token not read back")
	}
	if err := s.DeletePublishToken(); err != nil {
		t.Fatal(err)
	}
	if s.PublishTokenExists() {
		t.Fatal("token survived delete")
	}
}

// Linux has no secure backend; the token goes to a 0600 file, like mcp-token.
func TestPublishTokenFallsBackToAFileWithoutASecureStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LINETTA_HOME", home)
	s, err := NewWithSecretStore(unsupportedSecretStore{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPublishToken("lwk_abc"); err != nil {
		t.Fatalf("SetPublishToken without a secure store: %v", err)
	}
	path := filepath.Join(home, publishTokenFileName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("token file mode = %o, want 600", info.Mode().Perm())
	}
	if s.PublishToken() != "lwk_abc" || !s.PublishTokenExists() {
		t.Fatal("fallback token not read back")
	}
	if err := s.DeletePublishToken(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("fallback file survived delete")
	}
}

func TestPublishAccountPersistsWithoutTheToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LINETTA_HOME", home)
	s, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatal(err)
	}
	acc := PublishAccount{SiteURL: "https://works.example", Handle: "devlikebear", PenName: "마곰", ConnectedAt: 1}
	if err := s.SetPublishAccount(&acc); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPublishToken("lwk_secret"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(home, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "lwk_secret") {
		t.Fatal("the token must never reach settings.json")
	}
	var disk Config
	if err := json.Unmarshal(raw, &disk); err != nil || disk.PublishAccount == nil || disk.PublishAccount.PenName != "마곰" {
		t.Fatalf("account not persisted: %s", raw)
	}

	reloaded, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := reloaded.PublishAccount(); !ok || got != acc {
		t.Fatalf("reloaded account = %+v, %v", got, ok)
	}
	// An unrelated settings.set keeps it.
	lang := "en"
	if _, err := reloaded.Set(t.Context(), Patch{Language: &lang}); err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.PublishAccount(); !ok {
		t.Fatal("settings.set dropped the publish account")
	}
	if err := reloaded.SetPublishAccount(nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.PublishAccount(); ok {
		t.Fatal("account not cleared")
	}
}
