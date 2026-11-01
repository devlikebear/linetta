//go:build !mobile && windows

package gitsync

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// hideConsole keeps git off the writer's screen. The app is a GUI process with
// no console to inherit, so Windows would otherwise open a terminal window for
// every git call of a sync. CREATE_NO_WINDOW gives git a console of its own that
// has no window, and whatever git starts shares it. DETACHED_PROCESS would not
// do: git would have no console, and each console program it starts would open
// a window of its own.
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}
