//go:build !windows

package main

// The global hotkey is a Win32 RegisterHotKey feature; other platforms get a
// no-op so the Settings field is simply inert there.

func (a *App) applyHotkey(string) {}

func (a *App) stopHotkey() {}
