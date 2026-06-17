//go:build !mas

package ai

// guardProvider permits every provider in non-App-Store builds.
func guardProvider(string) error { return nil }
