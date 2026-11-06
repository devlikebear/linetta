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

func TestCredentialTarget(t *testing.T) {
	cases := []struct{ service, name, want string }{
		{linettaSecretService, "openai", "devlikebear.linetta:openai"},
		{"devlikebear.linetta", "", "devlikebear.linetta:"},
		{"devlikebear.clio", "openai", "devlikebear.clio:openai"},
	}
	for _, c := range cases {
		if got := credentialTarget(c.service, c.name); got != c.want {
			t.Errorf("credentialTarget(%q,%q) = %q, want %q", c.service, c.name, got, c.want)
		}
	}
}
