//go:build !mobile && windows

package gitsync

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// A test binary has a console for git to inherit, so it cannot see the window
// itself. What it can hold down is the flag that keeps one from opening when the
// app, which has no console, runs git.
func TestGitCommand_opensNoConsoleWindow(t *testing.T) {
	cmd := gitCommand(context.Background(), t.TempDir(), "status")
	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil, so a GUI process opens a window for git")
	}
	flags := cmd.SysProcAttr.CreationFlags
	if flags&windows.CREATE_NO_WINDOW == 0 {
		t.Errorf("CreationFlags %#x lack CREATE_NO_WINDOW", flags)
	}
	// Windows ignores CREATE_NO_WINDOW when DETACHED_PROCESS is also set.
	if flags&windows.DETACHED_PROCESS != 0 {
		t.Errorf("CreationFlags %#x include DETACHED_PROCESS", flags)
	}
}

// Git still has to hand its output back through the windowless console.
func TestRunGitProd_capturesOutputWithoutAConsoleWindow(t *testing.T) {
	out, err := runGitProd(context.Background(), t.TempDir(), "--version")
	if err != nil {
		t.Fatalf("runGitProd: %v", err)
	}
	if !strings.HasPrefix(out, "git version") {
		t.Fatalf("output = %q, want git's version line", out)
	}
}
