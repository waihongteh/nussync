package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	"nussync/internal/config"
	"nussync/internal/notify"
	"nussync/internal/sync"
	"nussync/internal/telegram"
)

// runCLI executes a headless command and returns a process exit code.
func runCLI(mode string) int {
	app := NewApp()
	if err := app.Init(true); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer app.shutdown(context.Background())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	app.ctx = ctx

	switch mode {
	case "--sync":
		return cliSync(ctx, app)
	case "--check":
		return cliCheck(ctx, app)
	case "--pair":
		return cliPair(app)
	case "--notify-test":
		return cliNotifyTest(app)
	case "--study-test":
		return runStudyCLI(os.Args[2:])
	}
	return 2
}

func cliSync(ctx context.Context, app *App) int {
	cfg := app.settings()
	if strings.TrimSpace(cfg.CanvasToken) == "" {
		fmt.Fprintln(os.Stderr, "error: no Canvas token (set CANVAS_TOKEN in .env)")
		return 1
	}
	who, err := app.TestCanvas()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: canvas auth failed:", err)
		return 1
	}
	fmt.Printf("Canvas: %s\nSync dir: %s\n\n", who, cfg.SyncDir)

	start := time.Now()
	var lastPhase, lastCourse string
	eng := sync.New(app.canvasClient(), app.st, cfg, func(p sync.Progress) {
		if p.Phase != lastPhase || p.Course != lastCourse {
			lastPhase, lastCourse = p.Phase, p.Course
			if p.Course != "" {
				fmt.Printf("[%s] %s\n", p.Phase, p.Course)
			} else {
				fmt.Printf("[%s]\n", p.Phase)
			}
		}
		if p.Phase == "downloading" && p.Total > 0 && p.Done%10 == 0 {
			fmt.Printf("    %d/%d  %s\n", p.Done, p.Total, p.CurrentFile)
		}
	})
	res, err := eng.Run(ctx)

	fmt.Printf("\nDownloaded %d files (%s) across %d courses in %s; %d skipped by filters.\n",
		res.FilesDownloaded, humanBytes(res.BytesDownloaded), res.CoursesSynced,
		time.Since(start).Round(time.Second), res.FilesSkipped)

	if st, e := app.st.Stats(); e == nil {
		fmt.Printf("Library now: %d files, %s.\n", st.Files, humanBytes(st.Bytes))
	}
	if len(res.Errors) > 0 {
		fmt.Printf("\n%d non-fatal errors:\n", len(res.Errors))
		limit := len(res.Errors)
		if limit > 20 {
			limit = 20
		}
		for _, e := range res.Errors[:limit] {
			fmt.Println("  -", e)
		}
		if limit < len(res.Errors) {
			fmt.Printf("  ... and %d more\n", len(res.Errors)-limit)
		}
	}
	if err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func cliCheck(ctx context.Context, app *App) int {
	cfg := app.settings()
	// Refresh metadata so --check works on a fresh database.
	if strings.TrimSpace(cfg.CanvasToken) != "" {
		if err := refreshMetaOnly(ctx, app, cfg); err != nil && ctx.Err() == nil {
			fmt.Fprintln(os.Stderr, "warning:", err)
		}
	}

	ds, err := app.GetDeadlines()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i].DueAt < ds[j].DueAt })

	now := time.Now()
	fmt.Printf("%d deadlines\n\n", len(ds))
	for _, d := range ds {
		due, err := time.Parse(time.RFC3339, d.DueAt)
		if err != nil {
			continue
		}
		state := "unsubmitted"
		if d.Submitted {
			state = "submitted"
		}
		when := notify.FormatDue(due)
		rel := notify.Humanize(due.Sub(now))
		if due.Before(now) {
			rel = rel + " ago"
		} else {
			rel = "in " + rel
		}
		fmt.Printf("%-10s %-8s %-46s %s (%s) [%s]\n",
			d.CourseCode, d.Type, truncate(d.Title, 46), when, rel, state)
	}
	return 0
}

// refreshMetaOnly pulls deadlines/announcements/grades without downloading files.
func refreshMetaOnly(ctx context.Context, app *App, cfg config.Settings) error {
	eng := sync.New(app.canvasClient(), app.st, cfg, nil)
	return eng.RefreshMetadata(ctx)
}

func cliPair(app *App) int {
	cfg := app.settings()
	if strings.TrimSpace(cfg.TelegramToken) == "" {
		fmt.Fprintln(os.Stderr, "error: no TELEGRAM_TOKEN configured")
		return 1
	}
	fmt.Printf("Send /start to @%s within 60s...\n", telegram.BotUsername)
	id, err := app.PairTelegram()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Println("Paired. Chat id saved.")
	_ = id
	return 0
}

func cliNotifyTest(app *App) int {
	cfg := app.settings()
	if strings.TrimSpace(cfg.TelegramToken) == "" {
		fmt.Fprintln(os.Stderr, "error: no TELEGRAM_TOKEN configured")
		return 1
	}
	if strings.TrimSpace(cfg.TelegramChatID) == "" {
		// Try to discover a chat id from an existing /start in the queue.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		id, err := app.telegramClient().FindExistingChat(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		if id == "" {
			fmt.Printf("Not paired: no /start found. Send /start to @%s, then run --pair.\n",
				telegram.BotUsername)
			return 1
		}
		if _, err := app.savePairedChat(id); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		fmt.Println("Discovered and saved Telegram chat id.")
	}
	if err := app.SendTestTelegram(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Println("Test message sent.")
	return 0
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
