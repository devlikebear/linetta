//go:build !mobile && !windows

package gitsync

import "os/exec"

// unattended has no Unix counterpart: a child process only gets a terminal
// window if something asks for one.
func unattended(*exec.Cmd) {}
