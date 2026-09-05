//go:build windows

package main

import (
	"log"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	procRegisterHotKey     = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey   = user32.NewProc("UnregisterHotKey")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procPostThreadMessageW = user32.NewProc("PostThreadMessageW")

	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	procGetCurrentThreadID = kernel32.NewProc("GetCurrentThreadId")
)

const (
	wmQuit   = 0x0012
	wmHotkey = 0x0312
	// hotkeyID is arbitrary but must be < 0xC000 for a thread hotkey.
	hotkeyID = 1
)

// winMsg mirrors the Win32 MSG struct for GetMessageW.
type winMsg struct {
	HWND    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

// applyHotkey (re)registers the global show/hide shortcut. spec is a Settings
// Hotkey string; "" unregisters and leaves no hotkey.
//
// RegisterHotKey binds the hotkey to the CALLING THREAD when hwnd is 0, and
// WM_HOTKEY is then posted to that thread's message queue — which only works if
// the thread never migrates. Hence runtime.LockOSThread plus a dedicated
// GetMessage loop; Stop posts WM_QUIT to that thread id to unwind it.
func (a *App) applyHotkey(spec string) {
	a.stopHotkey()

	hk, ok, err := ParseHotkey(spec)
	if err != nil {
		log.Printf("nussync: %v", err)
		return
	}
	if !ok {
		return
	}

	tidCh := make(chan uint32, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		tid, _, _ := procGetCurrentThreadID.Call()
		r, _, e := procRegisterHotKey.Call(0, hotkeyID,
			uintptr(hk.Mods|modNoRepeat), uintptr(hk.VK))
		if r == 0 {
			log.Printf("nussync: hotkey %q not registered (already taken?): %v", spec, e)
			tidCh <- 0
			return
		}
		defer procUnregisterHotKey.Call(0, hotkeyID)
		tidCh <- uint32(tid)

		var m winMsg
		for {
			// GetMessageW returns 0 on WM_QUIT and -1 on error; both end the loop.
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			if m.Message == wmHotkey && m.WParam == hotkeyID {
				a.ToggleWindow()
			}
		}
	}()

	if tid := <-tidCh; tid != 0 {
		a.mu.Lock()
		a.hotkeyThread = tid
		a.mu.Unlock()
	}
}

// stopHotkey unwinds the hotkey thread, unregistering the shortcut.
func (a *App) stopHotkey() {
	a.mu.Lock()
	tid := a.hotkeyThread
	a.hotkeyThread = 0
	a.mu.Unlock()
	if tid != 0 {
		procPostThreadMessageW.Call(uintptr(tid), wmQuit, 0, 0)
	}
}
