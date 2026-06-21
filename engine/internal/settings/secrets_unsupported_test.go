package settings

import "testing"

func TestUnsupportedSecretStoreErrors(t *testing.T) {
	var s SecretStore = unsupportedSecretStore{}
	if err := s.Set("k", "v"); err == nil {
		t.Fatal("Set should error on unsupported store")
	}
	if _, ok, _ := s.Get("k"); ok {
		t.Fatal("Get should report not-found on unsupported store")
	}
}
