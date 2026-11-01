//go:build !mobile && windows

package gitsync

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// noAskpass is a program that does not exist. Handing it to ssh as the askpass
// turns any question ssh wants to ask into an immediate failure.
const noAskpass = "linetta-no-askpass"

const treeKillTimeout = 5 * time.Second

// unattended keeps git off the writer's screen. The app is a GUI process with no
// console to inherit, so Windows would otherwise open a terminal window for
// every git call of a sync. CREATE_NO_WINDOW gives git a console of its own that
// has no window, and whatever git starts shares it. DETACHED_PROCESS would not
// do: git would have no console, and each console program it starts would open
// a window of its own.
//
// Nobody can answer that console, so the Windows build of ssh would wait on it
// forever for a host key or a passphrase. Forcing the askpass makes it fail with
// the reason instead, which reaches the writer as the sync's error. An askpass
// the writer set up is kept.
//
// exec only kills the process it started, and that is often Git's cmd\git.exe
// wrapper, so a timeout has to take the real git and its ssh down too.
func unattended(cmd *exec.Cmd) {
	cmd.SysProcAttr = windowless()
	cmd.Env = append(os.Environ(), "SSH_ASKPASS_REQUIRE=force")
	if os.Getenv("SSH_ASKPASS") == "" {
		cmd.Env = append(cmd.Env, "SSH_ASKPASS="+noAskpass)
	}
	cmd.Cancel = func() error { return killTree(cmd) }
}

func windowless() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}

func killTree(cmd *exec.Cmd) error {
	ctx, cancel := context.WithTimeout(context.Background(), treeKillTimeout)
	defer cancel()
	kill := exec.CommandContext(ctx, "taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	kill.SysProcAttr = windowless()
	if err := kill.Run(); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}
