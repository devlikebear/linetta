package settings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/atomicfile"
)

// vaultKeyFileName holds the vault recovery key on platforms with no secure
// secret backend, at 0600, for the reason publishTokenFileName gives: any
// process running as this user can already read the manuscripts this key
// protects on the server. Only those platforms: unlike the publish token, a
// locked Keychain must not send this key to a plaintext file.
const vaultKeyFileName = "vault-key"

// ErrVaultKeyExists means this device already holds a different recovery key.
// Everything sealed under the stored key is unreadable without it, so replacing
// it is a decision for the caller (DeleteVaultKey first), never a side effect of
// setting a new one.
var ErrVaultKeyExists = errors.New("settings: a different vault key is already stored")

func (s *Store) vaultKeyPath() string {
	return filepath.Join(s.dir, vaultKeyFileName)
}

// VaultKey returns the stored recovery key. Unlike PublishToken it reports
// failure: a Keychain that is locked or denied is not "no key", and a caller
// that took it for one would set up a new key over the old.
func (s *Store) VaultKey() (string, bool, error) {
	secret, ok, err := s.secrets.Get(vaultKeySecretName)
	if err == nil && ok {
		return secret, true, nil
	}
	raw, readErr := os.ReadFile(s.vaultKeyPath())
	if readErr == nil {
		return strings.TrimSpace(string(raw)), true, nil
	}
	if !os.IsNotExist(readErr) {
		return "", false, readErr
	}
	return "", false, err
}

// VaultKeyExists reports whether a recovery key is stored, without reading its
// value (on macOS a read can raise a Keychain prompt).
func (s *Store) VaultKeyExists() (bool, error) {
	ok, err := s.secrets.Exists(vaultKeySecretName)
	if err == nil && ok {
		return true, nil
	}
	if _, statErr := os.Stat(s.vaultKeyPath()); statErr == nil {
		return true, nil
	} else if !os.IsNotExist(statErr) {
		return false, statErr
	}
	return false, err
}

// SetVaultKey stores the recovery key, and refuses to replace a different one.
// See ErrVaultKeyExists.
func (s *Store) SetVaultKey(key string) error {
	if key == "" {
		return errors.New("settings: refusing to store an empty vault key")
	}
	current, ok, err := s.VaultKey()
	if err != nil {
		return err
	}
	if ok {
		if current != key {
			return ErrVaultKeyExists
		}
		return nil
	}
	if err := s.secrets.Set(vaultKeySecretName, key); !errors.Is(err, ErrSecretStoreUnsupported) {
		return err
	}
	return atomicfile.Write(s.vaultKeyPath(), []byte(key), 0o600)
}

// DeleteVaultKey removes the recovery key from both possible places. Nothing
// calls it on disconnect: losing the site connection must not lose the only key
// to what is on the server.
func (s *Store) DeleteVaultKey() error {
	err := s.secrets.Delete(vaultKeySecretName)
	if rmErr := os.Remove(s.vaultKeyPath()); rmErr != nil && !os.IsNotExist(rmErr) && err == nil {
		err = rmErr
	}
	return err
}
