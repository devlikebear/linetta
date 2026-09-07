package settings

import "errors"

// ErrSecretStoreUnsupported is what Set returns on a build with no OS secret
// backend at all — Linux, Android, and a cgo-less darwin bridge.
//
// It exists so a caller can tell the two failures apart: "this platform has
// nowhere to put a secret, and never will until someone writes a backend" is
// a permanent fact about the build, while a Keychain that is locked or denied
// is a transient fault on a platform that does have a store. The legacy
// plaintext migration in load() reports them differently for that reason —
// see legacyPlaintextKeys in settings.go (#113).
var ErrSecretStoreUnsupported = errors.New("settings: secure secret storage is only available on macOS and Windows")

type unsupportedSecretStore struct{}

func (unsupportedSecretStore) Get(string) (string, bool, error) {
	return "", false, nil
}

func (unsupportedSecretStore) Exists(string) (bool, error) {
	return false, nil
}

func (unsupportedSecretStore) Set(string, string) error {
	return ErrSecretStoreUnsupported
}

func (unsupportedSecretStore) Delete(string) error {
	return nil
}
