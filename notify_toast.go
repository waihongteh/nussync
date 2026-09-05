package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	gosync "sync"

	toast "git.sr.ht/~jackmordaunt/go-toast/v2"

	"nussync/internal/config"
)

// toastAppID groups our notifications in the Windows Action Centre.
const toastAppID = "NUSSync"

var toastOnce gosync.Once

// initToast registers the app with the Windows Runtime once per process and
// wires the activation callback so clicking a toast reopens the window.
//
// go-toast builds on every platform (its non-Windows implementation is a
// no-op), so this needs no build tag.
func (a *App) initToast() {
	toastOnce.Do(func() {
		exe, _ := os.Executable()
		if err := toast.SetAppData(toast.AppData{
			AppID:         toastAppID,
			ActivationExe: exe,
			IconPath:      toastIconPath(),
		}); err != nil {
			log.Printf("nussync: toast app data: %v", err)
		}
		// Only fires while the process is alive and the COM path (not the
		// PowerShell fallback) is in use — hence "at minimum the toast shows".
		toast.SetActivationCallback(func(args string, _ []toast.UserData) {
			a.ShowWindow()
		})
	})
}

// toastIconPath materialises the embedded app icon next to config.json, since
// a toast is rendered from a temp directory and needs an absolute path.
func toastIconPath() string {
	dir, err := config.Dir()
	if err != nil || len(appIconPNG) == 0 {
		return ""
	}
	p := filepath.Join(dir, "appicon.png")
	if st, err := os.Stat(p); err == nil && st.Size() == int64(len(appIconPNG)) {
		return p
	}
	if err := os.WriteFile(p, appIconPNG, 0o644); err != nil {
		return ""
	}
	return p
}

// notifyNewFiles pops a Windows toast summarising the files a sync brought in,
// e.g. "3 new files in CS4246, MA3236". Silent when the setting is off, when
// nothing is new, or when the OS refuses the notification.
func (a *App) notifyNewFiles(total int, byCourse map[string]int) {
	if total <= 0 || !a.settings().NotifyDesktop {
		return
	}
	a.initToast()
	n := toast.Notification{
		AppID: toastAppID,
		Title: "NUSSync",
		Body:  newFilesSummary(total, byCourse),
		Icon:  toastIconPath(),
	}
	if err := n.Push(); err != nil {
		log.Printf("nussync: toast: %v", err)
	}
}

// newFilesSummary renders the toast body. Courses are listed busiest first and
// capped so the line stays readable.
func newFilesSummary(total int, byCourse map[string]int) string {
	noun := "new files"
	if total == 1 {
		noun = "new file"
	}
	if len(byCourse) == 0 {
		return fmt.Sprintf("%d %s synced", total, noun)
	}
	codes := make([]string, 0, len(byCourse))
	for c := range byCourse {
		codes = append(codes, c)
	}
	sort.Slice(codes, func(i, j int) bool {
		if byCourse[codes[i]] != byCourse[codes[j]] {
			return byCourse[codes[i]] > byCourse[codes[j]]
		}
		return codes[i] < codes[j]
	})
	const maxCodes = 4
	suffix := ""
	if len(codes) > maxCodes {
		suffix = fmt.Sprintf(" +%d more", len(codes)-maxCodes)
		codes = codes[:maxCodes]
	}
	return fmt.Sprintf("%d %s in %s%s", total, noun, strings.Join(codes, ", "), suffix)
}
