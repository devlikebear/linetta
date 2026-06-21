//go:build mas || mobile

package clidetect

import "context"

// Detect always reports "not found" in restricted builds: locating the CLI
// requires spawning a login shell, which MAS and mobile runtimes forbid.
func Detect(context.Context) string { return "" }
