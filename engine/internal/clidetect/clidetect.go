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
	"path/filepath"
	"strconv"
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
	// Version reports what `<path> --version` prints, or "" when it cannot
	// tell. Nil keeps the first candidate found. A machine can hold more
	// than one install — an npm one under ~/.local and a Homebrew one, say —
	// and the writer runs `claude update` on whichever their terminal finds,
	// so the first on PATH can be the one left behind. A model a newer CLI
	// serves is then refused by the older one until Linetta stops using it.
	Version func(ctx context.Context, path string) string
}

// Detect returns the claude to run, or "" if none. Every source is asked, in
// order — PATH, the login shell, the known install locations — and when more
// than one distinct install turns up, the newest version wins; a tie, or a
// version that cannot be read, keeps the earlier one.
func (d Detector) Detect(ctx context.Context) string {
	candidates := d.candidates(ctx)
	if len(candidates) == 0 {
		return ""
	}
	if d.Version == nil || len(candidates) == 1 {
		return candidates[0]
	}
	best, bestVersion := candidates[0], parseVersion(d.Version(ctx, candidates[0]))
	for _, p := range candidates[1:] {
		if v := parseVersion(d.Version(ctx, p)); newer(v, bestVersion) {
			best, bestVersion = p, v
		}
	}
	return best
}

// candidates lists every executable claude the sources name, in the order
// they are asked, once per install: two names for the same file (a symlink
// in PATH and its target among the known paths) are one candidate.
func (d Detector) candidates(ctx context.Context) []string {
	var named []string
	if d.LookPath != nil {
		if p, err := d.LookPath("claude"); err == nil {
			named = append(named, p)
		}
	}
	if d.ShellLookup != nil {
		if p := d.ShellLookup(ctx); p != "" {
			named = append(named, p)
		}
	}
	named = append(named, d.KnownPaths...)

	var out []string
	seen := map[string]bool{}
	for _, p := range named {
		if !d.IsExec(p) {
			continue
		}
		key := p
		if real, err := filepath.EvalSymlinks(p); err == nil {
			key = real
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}

// parseVersion reads the leading "2.1.330" of "2.1.330 (Claude Code)". Nil
// when there is none.
func parseVersion(s string) []int {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil
	}
	var out []int
	for _, part := range strings.Split(strings.TrimPrefix(fields[0], "v"), ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	return out
}

// newer reports whether a is a later version than b. A readable version is
// later than an unreadable one.
func newer(a, b []int) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// cliVersion runs `<path> --version`, bounded so a wedged install cannot hold
// detection up.
func cliVersion(ctx context.Context, path string) string {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, path, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
		Version:     cliVersion,
	}
}

// Detect runs the default detector.
func Detect(ctx context.Context) string { return DefaultDetector().Detect(ctx) }
