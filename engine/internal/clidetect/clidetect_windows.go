//go:build !mas && !mobile && windows

package clidetect

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// defaultPathExt mirrors the Windows default when PATHEXT is unset.
const defaultPathExt = ".COM;.EXE;.BAT;.CMD"

// shellLookup has no Windows counterpart. There is no login shell whose profile
// contributes PATH entries the GUI process would otherwise miss, and
// exec.LookPath already resolves PATHEXT extensions, so the PATH probe covers
// everything a shell lookup could find.
func shellLookup(context.Context) string { return "" }

// knownPaths lists the default Claude Code install locations on Windows.
// Unlike the Unix list these must carry an extension, because a bare `claude`
// with no extension is not runnable on Windows.
func knownPaths(home string) []string {
	var known []string
	if home != "" {
		known = append(known,
			filepath.Join(home, ".local", "bin", "claude.exe"),    // native installer
			filepath.Join(home, ".claude", "local", "claude.exe"), // native install (legacy dir)
			filepath.Join(home, ".claude", "local", "claude.cmd"),
		)
	}
	if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
		known = append(known,
			filepath.Join(appData, "npm", "claude.cmd"), // npm global
			filepath.Join(appData, "npm", "claude.exe"),
		)
	}
	if localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); localAppData != "" {
		known = append(known,
			filepath.Join(localAppData, "Programs", "claude", "claude.exe"),
		)
	}
	return known
}

// isExecutable reports whether path is a runnable program.
//
// Windows has no executable permission bit: os.Stat reports mode 0666 for every
// writable regular file, so the Unix `mode&0o111 != 0` test rejects even a valid
// claude.exe. Decide on the file extension against PATHEXT instead.
func isExecutable(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return false
	}
	for _, candidate := range executableExts() {
		if ext == candidate {
			return true
		}
	}
	return false
}

// executableExts returns the lowercased PATHEXT entries, each with a leading dot.
func executableExts() []string {
	pathExt := strings.TrimSpace(os.Getenv("PATHEXT"))
	if pathExt == "" {
		pathExt = defaultPathExt
	}
	var exts []string
	for _, ext := range strings.Split(pathExt, ";") {
		ext = strings.ToLower(strings.TrimSpace(ext))
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		exts = append(exts, ext)
	}
	return exts
}
