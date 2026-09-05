package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/energye/systray"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// startTray adds a system tray icon in a background goroutine. energye/systray
// is designed to coexist with a Wails v2 main loop.
func startTray(a *App) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("nussync: tray unavailable: %v", r)
			}
		}()
		systray.Run(func() { onTrayReady(a) }, func() {})
	}()
}

func onTrayReady(a *App) {
	systray.SetIcon(trayIcon())
	systray.SetTitle("NUSSync")
	systray.SetTooltip("NUSSync - Canvas sync")

	open := systray.AddMenuItem("Open NUSSync", "Show the main window")
	syncNow := systray.AddMenuItem("Sync now", "Run a sync immediately")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "Exit NUSSync")

	open.Click(func() {
		if a.ctx != nil {
			wruntime.WindowShow(a.ctx)
		}
	})
	syncNow.Click(func() {
		if err := a.SyncNow(); err != nil {
			log.Printf("nussync: tray sync: %v", err)
		}
	})
	quit.Click(func() {
		if a.ctx != nil {
			wruntime.Quit(a.ctx)
			return
		}
		systray.Quit()
	})

	// Left-clicking the tray icon reopens the window.
	systray.SetOnClick(func(menu systray.IMenu) {
		if a.ctx != nil {
			wruntime.WindowShow(a.ctx)
		}
	})
}

// trayIcon prefers the .ico next to the executable / in build/windows, and
// falls back to the embedded PNG.
func trayIcon() []byte {
	candidates := []string{filepath.Join("build", "windows", "icon.ico")}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "icon.ico"))
	}
	for _, p := range candidates {
		if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
			return b
		}
	}
	return appIconPNG
}
