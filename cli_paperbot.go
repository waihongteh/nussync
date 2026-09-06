package main

// Headless harness for the paper Telegram bot's command handler:
//
//	nussync --papers-bot-test "/search LLM unlearning"
//
// It builds a PaperBot with exactly the hooks App.startPaperBot installs, runs
// one command through the real handler and prints the reply to stdout instead
// of sending it to Telegram. Nothing is sent; the chat id is only used as the
// key for the remembered "last results" list, so a /search here really does
// leave a numbering that a later /save picks up.

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"nussync/internal/notify"
	"nussync/internal/papers"
)

// runPaperBotCLI is the entry point for --papers-bot-test. Hook it into
// cli.go's switch with:
//
//	case "--papers-bot-test": return runPaperBotCLI(os.Args[2:])
func runPaperBotCLI(args []string) int {
	text := strings.TrimSpace(strings.Join(args, " "))
	if text == "" {
		text = "/help"
	}

	app := NewApp()
	if err := app.Init(true); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer app.shutdown(context.Background())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	app.ctx = ctx

	if err := app.papersInit(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	// headless Init means startPaperBot never ran, so build the bot here with
	// the same hooks the GUI installs.
	cfg := app.settings()
	bot := notify.NewPaperBot(app.st, app.paperTelegram(), cfg)
	bot.Help = papers.PaperHelpMessage
	bot.Digest = func(ctx context.Context) (string, error) {
		d, err := app.buildDigest(ctx, todayLocal())
		if err != nil {
			return "", err
		}
		return papers.DigestMessage(d), nil
	}
	bot.Save = func(ctx context.Context, n int) (string, error) { return app.savePaperN(n) }
	bot.Reading = func(ctx context.Context) (string, error) { return app.readingMessage() }
	bot.SetExt(paperExt{app})

	chatID := strings.TrimSpace(cfg.PaperTelegramChatID)
	if chatID == "" {
		chatID = "cli"
		fmt.Println("(paper bot not paired — using a scratch chat id for the list memory)")
	}

	cmd, cmdArgs, ok := notify.ParseCommand(text)
	if !ok {
		fmt.Fprintf(os.Stderr, "error: %q is not a command\n", text)
		return 2
	}
	fmt.Printf("> %s %s\n\n", cmd, cmdArgs)

	reply := bot.Handle(ctx, chatID, cmd, cmdArgs)
	for i, chunk := range notify.SplitMessage(reply, notify.MaxMessage) {
		if i > 0 {
			fmt.Println("\n--- message", i+1, "---")
		}
		fmt.Println(chunk)
	}
	fmt.Printf("\n(%d bytes, %d message(s), limit %d)\n",
		len(reply), len(notify.SplitMessage(reply, notify.MaxMessage)), notify.MaxMessage)
	return 0
}
