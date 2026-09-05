package main

import (
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/energye/systray"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// trayDeadlines is how many upcoming deadlines the menu shows.
const trayDeadlines = 3

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

	// Deadline slots come first so they sit above the actions. They are created
	// up front (systray cannot reorder a menu later) and hidden until filled.
	slots := make([]*systray.MenuItem, trayDeadlines)
	for i := range slots {
		slots[i] = systray.AddMenuItem("", "Upcoming deadline")
		slots[i].Disable()
		slots[i].Hide()
		slots[i].Click(func() { a.ShowWindow() })
	}
	sep := systray.AddMenuItem("", "")
	sep.Disable()
	sep.Hide()

	open := systray.AddMenuItem("Open NUSSync", "Show the main window")
	syncNow := systray.AddMenuItem("Sync now", "Run a sync immediately")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "Exit NUSSync")

	open.Click(func() { a.ShowWindow() })
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
	systray.SetOnClick(func(menu systray.IMenu) { a.ShowWindow() })

	refresh := func() { refreshTrayDeadlines(a, slots, sep) }
	a.mu.Lock()
	a.trayRefresh = refresh
	a.mu.Unlock()
	refresh()

	// Deadlines drift into view with time, not only with syncs.
	go func() {
		t := time.NewTicker(15 * time.Minute)
		defer t.Stop()
		for range t.C {
			refresh()
		}
	}()
}

// refreshTrayDeadlines redraws the disabled deadline rows and the tooltip.
func refreshTrayDeadlines(a *App, slots []*systray.MenuItem, sep *systray.MenuItem) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("nussync: tray refresh: %v", r)
		}
	}()
	if a.st == nil {
		return
	}
	now := time.Now()
	ds, err := a.st.PendingDeadlines(now)
	if err != nil {
		return
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i].DueAt < ds[j].DueAt })
	if len(ds) > len(slots) {
		ds = ds[:len(slots)]
	}

	for i, item := range slots {
		if i >= len(ds) {
			item.Hide()
			continue
		}
		due, err := time.Parse(time.RFC3339, ds[i].DueAt)
		if err != nil {
			item.Hide()
			continue
		}
		item.SetTitle(fmt.Sprintf("%s · %s · %s",
			ds[i].CourseCode, truncate(ds[i].Title, 40), shortUntil(due.Sub(now))))
		item.SetTooltip(ds[i].CourseCode + " due " + due.Local().Format("Mon 2 Jan, 15:04"))
		item.Show()
	}

	if len(ds) == 0 {
		sep.Hide()
		systray.SetTooltip("NUSSync - no deadlines pending")
		return
	}
	sep.Show()
	if due, err := time.Parse(time.RFC3339, ds[0].DueAt); err == nil {
		systray.SetTooltip(fmt.Sprintf("Next: %s · %s · %s",
			ds[0].CourseCode, truncate(ds[0].Title, 40), shortUntil(due.Sub(now))))
	}
}

// shortUntil renders a remaining duration compactly ("in 2d", "in 5h", "now").
func shortUntil(d time.Duration) string {
	switch {
	case d <= 0:
		return "overdue"
	case d >= 48*time.Hour:
		return fmt.Sprintf("in %dd", int(d/(24*time.Hour)))
	case d >= 24*time.Hour:
		return "in 1d"
	case d >= time.Hour:
		return fmt.Sprintf("in %dh", int(d/time.Hour))
	case d >= time.Minute:
		return fmt.Sprintf("in %dm", int(d/time.Minute))
	default:
		return "now"
	}
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
