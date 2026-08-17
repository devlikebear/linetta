//go:build !darwin && !windows

package settings

// Linux, Android and other targets without a native backend fall through to an
// unsupported secret store. macOS uses the Keychain (secrets_darwin.go) and
// Windows the Credential Manager (secrets_windows.go); Linux still needs a
// libsecret/Secret Service backend, and Android an Android Keystore one.
func defaultSecretStore() SecretStore {
	return unsupportedSecretStore{}
}
