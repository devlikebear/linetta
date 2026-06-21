package settings

import "errors"

type unsupportedSecretStore struct{}

func (unsupportedSecretStore) Get(string) (string, bool, error) {
	return "", false, nil
}

func (unsupportedSecretStore) Exists(string) (bool, error) {
	return false, nil
}

func (unsupportedSecretStore) Set(string, string) error {
	return errors.New("settings: secure secret storage is only available on macOS")
}

func (unsupportedSecretStore) Delete(string) error {
	return nil
}
