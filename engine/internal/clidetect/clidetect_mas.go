//go:build mas

package clidetect

import "context"

// Detect always reports "not found" in the App Store build: locating the CLI
// requires spawning a login shell, which the sandbox forbids.
func Detect(context.Context) string { return "" }
