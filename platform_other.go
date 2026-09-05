//go:build !windows

package main

import "syscall"

// hiddenWindow is a no-op off Windows.
func hiddenWindow() *syscall.SysProcAttr { return nil }

// applyLaunchAtLogin is only implemented on Windows.
func (a *App) applyLaunchAtLogin(bool) {}
