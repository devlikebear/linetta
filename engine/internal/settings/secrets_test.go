package settings

import "testing"

// A test binary must never get the OS credential store by default: it is not
// scoped by LINETTA_HOME, so it would be the developer's real keychain. See
// defaultSecretStore.
func TestDefaultSecretStore_isNeverTheOSStoreUnderTest(t *testing.T) {
	if _, ok := defaultSecretStore().(unsupportedSecretStore); !ok {
		t.Fatalf("defaultSecretStore() = %T under test, want unsupportedSecretStore", defaultSecretStore())
	}
}
