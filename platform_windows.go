//go:build windows

package main

import (
	"log"
	"os"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const runValueName = "NUSSync"

// hiddenWindow keeps the helper cmd.exe console from flashing.
func hiddenWindow() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

// applyLaunchAtLogin adds or removes the HKCU Run registry entry.
func (a *App) applyLaunchAtLogin(enabled bool) {
	if err := setLaunchAtLogin(enabled); err != nil {
		log.Printf("nussync: launch-at-login: %v", err)
	}
}

func setLaunchAtLogin(enabled bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	if !enabled {
		err := k.DeleteValue(runValueName)
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return k.SetStringValue(runValueName, `"`+exe+`"`)
}
