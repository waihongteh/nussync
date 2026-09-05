//go:build !windows

package study

import (
	"os"
	"syscall"
)

// noWindow is a no-op off Windows.
func noWindow() *syscall.SysProcAttr { return nil }

// killTree kills the process (no shim layer to worry about off Windows).
func killTree(pid int) {
	if pid <= 0 {
		return
	}
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}
