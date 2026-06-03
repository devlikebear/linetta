//go:build darwin

package settings

import (
	"errors"
	"os/exec"
	"strings"
)

const keychainService = "devlikebear.linetta"

func defaultSecretStore() SecretStore {
	return keychainSecretStore{service: keychainService}
}

type keychainSecretStore struct {
	service string
}

func (k keychainSecretStore) Get(name string) (string, bool, error) {
	out, err := exec.Command("security", "find-generic-password", "-s", k.service, "-a", name, "-w").Output()
	if err != nil {
		if isKeychainNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return strings.TrimRight(string(out), "\r\n"), true, nil
}

func (k keychainSecretStore) Set(name, value string) error {
	if value == "" {
		return k.Delete(name)
	}
	return exec.Command("security", "add-generic-password", "-U", "-s", k.service, "-a", name, "-w", value).Run()
}

func (k keychainSecretStore) Delete(name string) error {
	err := exec.Command("security", "delete-generic-password", "-s", k.service, "-a", name).Run()
	if err != nil && isKeychainNotFound(err) {
		return nil
	}
	return err
}

func isKeychainNotFound(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	msg := strings.ToLower(string(exitErr.Stderr))
	return strings.Contains(msg, "could not be found") || strings.Contains(msg, "not found")
}
