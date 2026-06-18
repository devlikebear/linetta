//go:build !mas

package clidetect

import (
	"context"
	"testing"
)

func TestDetect_prefersPATHFirst(t *testing.T) {
	d := Detector{
		LookPath:    func(string) (string, error) { return "/from/path/claude", nil },
		ShellLookup: func(context.Context) string { return "/from/shell/claude" },
		KnownPaths:  []string{"/opt/homebrew/bin/claude"},
		IsExec:      func(string) bool { return true },
	}
	if got := d.Detect(context.Background()); got != "/from/path/claude" {
		t.Fatalf("got %q, want /from/path/claude", got)
	}
}

func TestDetect_fallsBackToShellWhenPATHMisses(t *testing.T) {
	d := Detector{
		LookPath:    func(string) (string, error) { return "", errNotFound },
		ShellLookup: func(context.Context) string { return "/from/shell/claude" },
		KnownPaths:  []string{"/opt/homebrew/bin/claude"},
		IsExec:      func(p string) bool { return p == "/from/shell/claude" },
	}
	if got := d.Detect(context.Background()); got != "/from/shell/claude" {
		t.Fatalf("got %q, want /from/shell/claude", got)
	}
}

func TestDetect_fallsBackToKnownPaths(t *testing.T) {
	d := Detector{
		LookPath:    func(string) (string, error) { return "", errNotFound },
		ShellLookup: func(context.Context) string { return "" },
		KnownPaths:  []string{"/nope/claude", "/opt/homebrew/bin/claude"},
		IsExec:      func(p string) bool { return p == "/opt/homebrew/bin/claude" },
	}
	if got := d.Detect(context.Background()); got != "/opt/homebrew/bin/claude" {
		t.Fatalf("got %q, want /opt/homebrew/bin/claude", got)
	}
}

func TestDetect_returnsEmptyWhenNothingExecutable(t *testing.T) {
	d := Detector{
		LookPath:    func(string) (string, error) { return "/x/claude", nil },
		ShellLookup: func(context.Context) string { return "/y/claude" },
		KnownPaths:  []string{"/z/claude"},
		IsExec:      func(string) bool { return false }, // nothing is executable
	}
	if got := d.Detect(context.Background()); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

var errNotFound = &lookErr{}

type lookErr struct{}

func (*lookErr) Error() string { return "not found" }
