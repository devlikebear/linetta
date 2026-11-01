//go:build !mobile && windows

package gitsync

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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

// The Windows build of ssh reads a host key answer or a passphrase from the
// console, and this console has no window to answer in.
func TestGitCommand_keepsSSHFromPromptingTheHiddenConsole(t *testing.T) {
	t.Setenv("SSH_ASKPASS", "")
	env := gitCommand(context.Background(), t.TempDir(), "push").Env
	if !slices.Contains(env, "SSH_ASKPASS_REQUIRE=force") {
		t.Error("SSH_ASKPASS_REQUIRE=force is missing")
	}
	if !slices.Contains(env, "SSH_ASKPASS="+noAskpass) {
		t.Errorf("SSH_ASKPASS=%s is missing", noAskpass)
	}
}

func TestGitCommand_keepsTheWritersAskpass(t *testing.T) {
	t.Setenv("SSH_ASKPASS", `C:\tools\askpass.exe`)
	env := gitCommand(context.Background(), t.TempDir(), "push").Env
	if slices.Contains(env, "SSH_ASKPASS="+noAskpass) {
		t.Error("the writer's SSH_ASKPASS was replaced")
	}
	if !slices.Contains(env, `SSH_ASKPASS=C:\tools\askpass.exe`) {
		t.Error("the writer's SSH_ASKPASS is gone")
	}
}

// fakeGit puts a batch file in front of the real git. It stands in for Git's
// cmd\git.exe wrapper: the process exec starts is not the one doing the work.
func fakeGit(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "git.cmd"), []byte(script), 0o644); err != nil {
		t.Fatalf("write git.cmd: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// A child that outlives the kill keeps the output pipes open, and then the call
// comes back only when gitWaitDelay gives up on them.
func TestRunGitProd_timeoutEndsTheWholeProcessTree(t *testing.T) {
	fakeGit(t, "@ping -n 60 127.0.0.1\r\n")
	const timeout = 500 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	_, err := runGitProd(ctx, t.TempDir(), "status")
	elapsed := time.Since(start)
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if elapsed >= timeout+gitWaitDelay {
		t.Fatalf("runGitProd took %s, so a child survived the timeout", elapsed)
	}
}

// Without it, a child the tree kill missed would hold runGitProd for as long as
// that child lives.
func TestGitCommand_boundsTheWaitForOutput(t *testing.T) {
	if got := gitCommand(context.Background(), t.TempDir(), "push").WaitDelay; got != gitWaitDelay {
		t.Fatalf("WaitDelay = %s, want %s", got, gitWaitDelay)
	}
}

// Git is done and said so. Something it left behind holding the pipes is no
// reason to report a failure.
func TestRunGitProd_succeedsWhenAChildOutlivesGit(t *testing.T) {
	fakeGit(t, "@start /b ping -n 8 127.0.0.1\r\n@exit /b 0\r\n")
	// Not a TempDir: the child still has its working directory open at cleanup.
	if _, err := runGitProd(context.Background(), ".", "status"); err != nil {
		t.Fatalf("runGitProd: %v", err)
	}
}
