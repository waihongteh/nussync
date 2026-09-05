package main

import (
	"log"

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

// trayIcon returns the same artwork as the app/exe icon. The .ico is embedded
// (Windows systray needs ICO bytes), so the tray works from any working
// directory; the PNG is only a fallback for non-Windows builds.
func trayIcon() []byte {
	if len(appIconICO) > 0 {
		return appIconICO
	}
	return appIconPNG
}
