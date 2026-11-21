//go:build !mas && !mobile

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

func TestDetect_picksTheNewestOfSeveralInstalls(t *testing.T) {
	versions := map[string]string{
		"/home/me/.local/bin/claude": "2.1.280 (Claude Code)",
		"/opt/homebrew/bin/claude":   "2.1.330 (Claude Code)",
	}
	d := Detector{
		LookPath:    func(string) (string, error) { return "", errNotFound },
		ShellLookup: func(context.Context) string { return "/home/me/.local/bin/claude" },
		KnownPaths:  []string{"/opt/homebrew/bin/claude", "/home/me/.local/bin/claude"},
		IsExec:      func(string) bool { return true },
		Version:     func(_ context.Context, p string) string { return versions[p] },
	}
	if got := d.Detect(context.Background()); got != "/opt/homebrew/bin/claude" {
		t.Fatalf("got %q, want the 2.1.330 install", got)
	}
}

func TestDetect_keepsTheEarlierOnATieOrAnUnreadableVersion(t *testing.T) {
	versions := map[string]string{
		"/a/claude": "2.1.300 (Claude Code)",
		"/b/claude": "2.1.300 (Claude Code)",
		"/c/claude": "garbage",
	}
	d := Detector{
		LookPath:   func(string) (string, error) { return "/a/claude", nil },
		KnownPaths: []string{"/b/claude", "/c/claude"},
		IsExec:     func(string) bool { return true },
		Version:    func(_ context.Context, p string) string { return versions[p] },
	}
	if got := d.Detect(context.Background()); got != "/a/claude" {
		t.Fatalf("got %q, want /a/claude", got)
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"2.1.330", "2.1.280", true},
		{"2.1.280", "2.1.330", false},
		{"2.10.0", "2.9.9", true},
		{"2.1", "2.1.0", false},
		{"2.1.0", "", true},
		{"", "2.1.0", false},
	}
	for _, c := range cases {
		if got := newer(parseVersion(c.a), parseVersion(c.b)); got != c.want {
			t.Errorf("newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
