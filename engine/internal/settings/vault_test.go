package settings

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const testVaultKey = "ABCD-EFGH-JKMN"

// faultySecretStore is a store that exists but is failing: a Keychain that is
// locked, or an access prompt the writer denied.
type faultySecretStore struct {
	SecretStore
	failReads  bool
	failWrites bool
}

var errKeychainLocked = errors.New("keychain is locked")

func (f faultySecretStore) Get(name string) (string, bool, error) {
	if f.failReads {
		return "", false, errKeychainLocked
	}
	return f.SecretStore.Get(name)
}

func (f faultySecretStore) Exists(name string) (bool, error) {
	if f.failReads {
		return false, errKeychainLocked
	}
	return f.SecretStore.Exists(name)
}

func (f faultySecretStore) Set(name, value string) error {
	if f.failWrites {
		return errKeychainLocked
	}
	return f.SecretStore.Set(name, value)
}

func newVaultStore(t *testing.T, secrets SecretStore) (*Store, string) {
	t.Helper()
	home := t.TempDir()
	s, err := NewForHomeWithSecretStore(home, secrets)
	if err != nil {
		t.Fatal(err)
	}
	return s, home
}

func TestVaultKeyLivesInTheSecretStore(t *testing.T) {
	secrets := NewMemorySecretStore()
	s, home := newVaultStore(t, secrets)

	if key, ok, err := s.VaultKey(); key != "" || ok || err != nil {
		t.Fatalf("fresh store: %q, %v, %v", key, ok, err)
	}
	if ok, err := s.VaultKeyExists(); ok || err != nil {
		t.Fatalf("fresh store exists: %v, %v", ok, err)
	}
	if err := s.SetVaultKey(testVaultKey); err != nil {
		t.Fatal(err)
	}
	if got, ok, _ := secrets.Get(vaultKeySecretName); !ok || got != testVaultKey {
		t.Fatalf("secret store holds %q, %v", got, ok)
	}
	if key, ok, err := s.VaultKey(); key != testVaultKey || !ok || err != nil {
		t.Fatalf("VaultKey: %q, %v, %v", key, ok, err)
	}
	if ok, err := s.VaultKeyExists(); !ok || err != nil {
		t.Fatalf("VaultKeyExists: %v, %v", ok, err)
	}
	if _, err := os.Stat(filepath.Join(home, vaultKeyFileName)); !os.IsNotExist(err) {
		t.Fatal("no fallback file when the secret store works")
	}
}

func TestVaultKeyNeverReachesSettingsJSON(t *testing.T) {
	s, home := newVaultStore(t, NewMemorySecretStore())
	if err := s.SetVaultKey(testVaultKey); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPublishAccount(&PublishAccount{Handle: "writer"}); err != nil {
		t.Fatal(err) // forces settings.json to be rewritten
	}
	raw, err := os.ReadFile(filepath.Join(home, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "writer") {
		t.Fatalf("settings.json was not written: %s", raw)
	}
	if strings.Contains(string(raw), testVaultKey) {
		t.Fatal("settings.json carries the vault key")
	}
}

func TestVaultKeyFallsBackToAPrivateFileOnlyWithoutABackend(t *testing.T) {
	s, home := newVaultStore(t, unsupportedSecretStore{})
	if err := s.SetVaultKey(testVaultKey); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, vaultKeyFileName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("fallback file mode = %v", info.Mode().Perm())
	}
	if key, ok, err := s.VaultKey(); key != testVaultKey || !ok || err != nil {
		t.Fatalf("VaultKey: %q, %v, %v", key, ok, err)
	}
	if ok, err := s.VaultKeyExists(); !ok || err != nil {
		t.Fatalf("VaultKeyExists: %v, %v", ok, err)
	}
	if err := s.DeleteVaultKey(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("DeleteVaultKey left the fallback file")
	}
}

// A store that is failing is not a store with no key. Reporting "no key" here
// would walk the writer into making a new one over the old.
func TestLockedStoreIsNotAnEmptyStore(t *testing.T) {
	base := NewMemorySecretStore()
	if err := base.Set(vaultKeySecretName, testVaultKey); err != nil {
		t.Fatal(err)
	}
	s, home := newVaultStore(t, faultySecretStore{SecretStore: base, failReads: true})

	if _, ok, err := s.VaultKey(); ok || !errors.Is(err, errKeychainLocked) {
		t.Fatalf("VaultKey: %v, %v", ok, err)
	}
	if ok, err := s.VaultKeyExists(); ok || !errors.Is(err, errKeychainLocked) {
		t.Fatalf("VaultKeyExists: %v, %v", ok, err)
	}
	if err := s.SetVaultKey("WXYZ-WXYZ-WXYZ"); !errors.Is(err, errKeychainLocked) {
		t.Fatalf("SetVaultKey: %v", err)
	}
	if got, _, _ := base.Get(vaultKeySecretName); got != testVaultKey {
		t.Fatalf("the stored key became %q", got)
	}
	if _, err := os.Stat(filepath.Join(home, vaultKeyFileName)); !os.IsNotExist(err) {
		t.Fatal("wrote a file past a locked store")
	}
}

func TestFailedKeychainWriteLeavesNoPlaintextFile(t *testing.T) {
	s, home := newVaultStore(t, faultySecretStore{SecretStore: NewMemorySecretStore(), failWrites: true})
	if err := s.SetVaultKey(testVaultKey); !errors.Is(err, errKeychainLocked) {
		t.Fatalf("SetVaultKey: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, vaultKeyFileName)); !os.IsNotExist(err) {
		t.Fatal("a real store that refused the write must not fall back to a file")
	}
}

func TestSetVaultKeyWillNotReplaceAnother(t *testing.T) {
	for name, secrets := range map[string]SecretStore{
		"secret store": NewMemorySecretStore(),
		"file":         unsupportedSecretStore{},
	} {
		t.Run(name, func(t *testing.T) {
			s, _ := newVaultStore(t, secrets)
			if err := s.SetVaultKey(testVaultKey); err != nil {
				t.Fatal(err)
			}
			if err := s.SetVaultKey(testVaultKey); err != nil {
				t.Fatalf("the same key again: %v", err)
			}
			if err := s.SetVaultKey("WXYZ-WXYZ-WXYZ"); !errors.Is(err, ErrVaultKeyExists) {
				t.Fatalf("a different key: %v", err)
			}
			if key, _, _ := s.VaultKey(); key != testVaultKey {
				t.Fatalf("the stored key became %q", key)
			}
			if err := s.DeleteVaultKey(); err != nil {
				t.Fatal(err)
			}
			if err := s.SetVaultKey("WXYZ-WXYZ-WXYZ"); err != nil {
				t.Fatalf("after deleting: %v", err)
			}
		})
	}
}

func TestSetVaultKeyRefusesAnEmptyKey(t *testing.T) {
	s, home := newVaultStore(t, unsupportedSecretStore{})
	if err := s.SetVaultKey(""); err == nil {
		t.Fatal("expected a refusal")
	}
	if _, err := os.Stat(filepath.Join(home, vaultKeyFileName)); !os.IsNotExist(err) {
		t.Fatal("an empty key was written")
	}
}
