//go:build windows

package study

import (
	"os/exec"
	"strconv"
	"syscall"
)

const createNoWindow = 0x08000000

// noWindow keeps the child console from flashing on screen.
func noWindow() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// killTree terminates the process and every descendant. `claude` is a .cmd shim
// that spawns node, so killing only the direct child leaves the real work
// running (and holding a subscription slot).
func killTree(pid int) {
	if pid <= 0 {
		return
	}
	cmd := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	_ = cmd.Run()
}
