//go:build (!darwin && !windows) || (darwin && !cgo)

package settings

// Linux, Android and other targets without a native backend fall through to an
// unsupported secret store. macOS uses the Keychain (secrets_darwin.go) and
// Windows the Credential Manager (secrets_windows.go); Linux still needs a
// libsecret/Secret Service backend, and Android an Android Keystore one.
//
// macOS lands here too when cgo is off, because the Keychain backend is cgo.
// That is the `linetta-mcp` bridge, which is built CGO_ENABLED=0 for
// cross-compilation and pulls this package in only transitively — it reads its
// token from the discovery file and never touches a secret store. The desktop
// app links the engine as a c-archive, so it always has cgo and the real
// Keychain. Without this, a darwin bridge build fails to compile at all.
func defaultSecretStore() SecretStore {
	return unsupportedSecretStore{}
}
