//go:build !mas

// Package clidetect locates the Claude Code CLI executable when it is not on
// the process PATH. macOS GUI apps inherit a minimal PATH (not the user's login
// shell PATH), so a plain `claude` lookup fails even when it is installed via
// Homebrew or npm. Detection therefore consults the login shell and known
// install locations as well.
package clidetect

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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
// common Homebrew/npm/native install locations.
func DefaultDetector() Detector {
	home, _ := os.UserHomeDir()
	known := []string{
		"/opt/homebrew/bin/claude", // Homebrew (Apple Silicon)
		"/usr/local/bin/claude",    // Homebrew (Intel) / npm global
	}
	if home != "" {
		known = append(known,
			filepath.Join(home, ".local", "bin", "claude"),      // pip/uv/manual
			filepath.Join(home, ".claude", "local", "claude"),   // Claude Code native install
			filepath.Join(home, ".npm-global", "bin", "claude"), // npm prefix override
		)
	}
	return Detector{
		LookPath:    exec.LookPath,
		ShellLookup: shellLookup,
		KnownPaths:  known,
		IsExec:      isExecutable,
	}
}

// Detect runs the default detector.
func Detect(ctx context.Context) string { return DefaultDetector().Detect(ctx) }

// shellLookup runs the user's login shell so brew/npm PATH entries from their
// profile are visible, then asks where `claude` resolves. Bounded by a timeout
// so a misconfigured profile cannot hang the engine.
func shellLookup(ctx context.Context) string {
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		shell = "/bin/zsh"
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, shell, "-l", "-c", "command -v claude").Output()
	if err != nil {
		return ""
	}
	// `command -v` may print multiple lines for shell functions; take the first.
	line := strings.TrimSpace(string(out))
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	return line
}

func isExecutable(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}
