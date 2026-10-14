//go:build !mas && !mobile && !windows

package clidetect

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

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

// knownPaths lists the common Homebrew/npm/native install locations.
func knownPaths(home string) []string {
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
	return known
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
