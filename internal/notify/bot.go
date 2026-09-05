package notify

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"nussync/internal/config"
	"nussync/internal/store"
	"nussync/internal/telegram"
)

// pollTimeout is the server-side long-poll window. The telegram HTTP client
// times out at 90s, so this must stay comfortably below that.
const pollTimeout = 50

// Bot is the single getUpdates consumer for the bot token. It answers the
// two-way commands (/due, /new, /files, /sync, /grades, /help) from the paired
// chat and silently ignores every other chat.
//
// Telegram delivers each update to exactly one getUpdates call, so nothing else
// in the process may poll while this loop runs — App.PairTelegram waits on
// AwaitPair instead of long-polling itself.
type Bot struct {
	Store *store.Store
	Now   func() time.Time

	// Sync runs a full sync and returns a one-line summary. Nil disables /sync.
	Sync func(context.Context) (string, error)
	// OnPair is called with the chat id of the first /start seen while
	// unpaired, so the app can persist it.
	OnPair func(chatID string)

	mu      sync.Mutex
	client  *telegram.Client
	chatID  string
	running bool
	waiters []chan string

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewBot builds a bot (not started).
func NewBot(st *store.Store, client *telegram.Client, cfg config.Settings) *Bot {
	return &Bot{Store: st, Now: time.Now, client: client, chatID: cfg.TelegramChatID}
}

// SetConfig swaps in new settings and/or a new client at runtime.
func (b *Bot) SetConfig(cfg config.Settings, client *telegram.Client) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.chatID = strings.TrimSpace(cfg.TelegramChatID)
	if client != nil {
		b.client = client
	}
}

// Running reports whether the poll loop is alive. Callers use this to decide
// whether pairing must go through AwaitPair (GUI) or a direct long poll (CLI).
func (b *Bot) Running() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.running
}

func (b *Bot) snapshot() (*telegram.Client, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.client, b.chatID
}

// Start launches the poll loop until ctx is cancelled or Stop is called.
func (b *Bot) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		cancel()
		return
	}
	b.running = true
	b.cancel = cancel
	b.mu.Unlock()

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		defer func() {
			b.mu.Lock()
			b.running = false
			b.mu.Unlock()
		}()
		b.loop(ctx)
	}()
}

// Stop halts the loop and waits for it.
func (b *Bot) Stop() {
	b.mu.Lock()
	cancel := b.cancel
	b.cancel = nil
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	b.wg.Wait()
}

// AwaitPair blocks until the loop sees a /start from an unpaired chat, and
// returns that chat id. Returns "" when the timeout or ctx elapses first.
func (b *Bot) AwaitPair(ctx context.Context, timeout time.Duration) string {
	ch := make(chan string, 1)
	b.mu.Lock()
	if b.chatID != "" {
		id := b.chatID
		b.mu.Unlock()
		return id
	}
	b.waiters = append(b.waiters, ch)
	b.mu.Unlock()

	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case id := <-ch:
		return id
	case <-t.C:
		return ""
	case <-ctx.Done():
		return ""
	}
}

// pair records a newly discovered chat id and wakes AwaitPair callers.
func (b *Bot) pair(chatID string) {
	b.mu.Lock()
	b.chatID = chatID
	waiters := b.waiters
	b.waiters = nil
	b.mu.Unlock()
	for _, w := range waiters {
		select {
		case w <- chatID:
		default:
		}
	}
	if b.OnPair != nil {
		b.OnPair(chatID)
	}
}

func (b *Bot) loop(ctx context.Context) {
	var offset int64
	for {
		if ctx.Err() != nil {
			return
		}
		client, _ := b.snapshot()
		if client == nil || strings.TrimSpace(client.Token) == "" {
			if !sleepCtx(ctx, 30*time.Second) {
				return
			}
			continue
		}

		ups, err := client.GetUpdates(ctx, offset, pollTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// 409 (another poller), network blips: back off and retry.
			if !sleepCtx(ctx, 10*time.Second) {
				return
			}
			continue
		}

		for _, u := range ups {
			if u.ID >= offset {
				offset = u.ID + 1
			}
			b.dispatch(ctx, u)
		}
	}
}

// dispatch routes one update. Handling runs on its own goroutine so a slow
// command (/sync takes minutes) never stalls the poll loop.
func (b *Bot) dispatch(ctx context.Context, u telegram.Update) {
	if u.ChatID == "" {
		return
	}
	_, paired := b.snapshot()
	cmd, args, ok := ParseCommand(u.Text)

	if paired == "" {
		// Unpaired: the first /start claims the bot. Everything else waits.
		if ok && cmd == "/start" {
			b.pair(u.ChatID)
			go b.reply(ctx, u.ChatID, "✅ Paired with NUSSync.\n\n"+HelpMessage())
		}
		return
	}
	if u.ChatID != paired {
		return // silently ignore strangers
	}
	if !ok {
		go b.reply(ctx, u.ChatID, HelpMessage())
		return
	}
	go b.handle(ctx, u.ChatID, cmd, args)
}

func (b *Bot) handle(ctx context.Context, chatID, cmd, args string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("nussync: telegram command %s: %v", cmd, r)
		}
	}()
	now := time.Now
	if b.Now != nil {
		now = b.Now
	}

	switch cmd {
	case "/due":
		ds, err := b.Store.Deadlines(now())
		if err != nil {
			b.reply(ctx, chatID, "Could not read deadlines.")
			return
		}
		b.reply(ctx, chatID, DueMessage(ds, now(), 10))

	case "/new":
		files, err := b.Store.ChangedFiles(now().Add(-24*time.Hour), 30)
		if err != nil {
			b.reply(ctx, chatID, "Could not read the file feed.")
			return
		}
		b.reply(ctx, chatID, NewFilesMessage(files, 30))

	case "/files":
		if strings.TrimSpace(args) == "" {
			b.reply(ctx, chatID, "Usage: <code>/files &lt;query&gt;</code>")
			return
		}
		hits, err := b.Store.Search(args, 0, 8)
		if err != nil {
			b.reply(ctx, chatID, "Search failed.")
			return
		}
		b.reply(ctx, chatID, FilesMessage(args, hits, 8))

	case "/grades":
		gs, err := b.Store.Grades()
		if err != nil {
			b.reply(ctx, chatID, "Could not read grades.")
			return
		}
		b.reply(ctx, chatID, GradesMessage(gs, 10))

	case "/sync":
		if b.Sync == nil {
			b.reply(ctx, chatID, "Sync is not available right now.")
			return
		}
		b.reply(ctx, chatID, "🔄 Syncing…")
		summary, err := b.Sync(ctx)
		if err != nil {
			b.reply(ctx, chatID, "❌ Sync failed: "+telegram.EscapeHTML(err.Error()))
			return
		}
		b.reply(ctx, chatID, "✅ "+telegram.EscapeHTML(summary))

	case "/help", "/start":
		b.reply(ctx, chatID, HelpMessage())

	default:
		b.reply(ctx, chatID, HelpMessage())
	}
}

// reply sends a body, splitting it to stay under Telegram's message limit.
func (b *Bot) reply(ctx context.Context, chatID, body string) {
	client, _ := b.snapshot()
	if client == nil {
		return
	}
	for _, chunk := range SplitMessage(body, MaxMessage) {
		if err := client.SendMessage(ctx, chatID, chunk); err != nil {
			if ctx.Err() == nil {
				log.Printf("nussync: telegram reply: %v", err)
			}
			return
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
