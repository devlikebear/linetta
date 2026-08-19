//go:build !mas && !mobile

// Package clidetect locates the Claude Code CLI executable when it is not on
// the process PATH. macOS GUI apps inherit a minimal PATH (not the user's login
// shell PATH), so a plain `claude` lookup fails even when it is installed via
// Homebrew or npm. Detection therefore consults the login shell and known
// install locations as well.
//
// The three probes that depend on the operating system — the login-shell
// lookup, the known install locations, and the executable test — live in
// clidetect_unix.go and clidetect_windows.go.
package clidetect

import (
	"context"
	"os"
	"os/exec"
)

// Detector resolves the Claude Code CLI path from several sources, in order.
// Fields are injectable so the selection logic can be tested without a shell.
type Detector struct {
	// LookPath resolves an executable on the current PATH (exec.LookPath).
	LookPath func(string) (string, error)
	// ShellLookup asks the user's login shell for `command -v claude`.
	ShellLookup func(context.Context) string
	// KnownPaths are absolute candidate locations, probed in order.
	KnownPaths []string
	// IsExec reports whether a path is an existing, executable file.
	IsExec func(string) bool
}

// Detect returns the first resolvable executable claude path, or "" if none.
func (d Detector) Detect(ctx context.Context) string {
	if d.LookPath != nil {
		if p, err := d.LookPath("claude"); err == nil && d.IsExec(p) {
			return p
		}
	}
	if d.ShellLookup != nil {
		if p := d.ShellLookup(ctx); p != "" && d.IsExec(p) {
			return p
		}
	}
	for _, p := range d.KnownPaths {
		if d.IsExec(p) {
			return p
		}
	}
	return ""
}

// DefaultDetector wires the real PATH lookup, login-shell lookup, and the
// platform's common install locations.
func DefaultDetector() Detector {
	home, _ := os.UserHomeDir()
	return Detector{
		LookPath:    exec.LookPath,
		ShellLookup: shellLookup,
		KnownPaths:  knownPaths(home),
		IsExec:      isExecutable,
	}
}

// Detect runs the default detector.
func Detect(ctx context.Context) string { return DefaultDetector().Detect(ctx) }
