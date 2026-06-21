//go:build !darwin

package settings

// Android and other non-Apple targets intentionally use an unsupported secret
// store until a native Android Keystore backend is added.
func defaultSecretStore() SecretStore {
	return unsupportedSecretStore{}
}
