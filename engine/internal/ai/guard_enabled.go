//go:build !mas

package ai

// guardProvider permits every provider in non-App-Store builds.
func guardProvider(string) error { return nil }

// UnavailableProviders lists providers that cannot work in this build. The
// non-App-Store build supports all providers.
func UnavailableProviders() []string { return nil }
