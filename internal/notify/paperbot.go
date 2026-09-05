package notify

// Second Telegram bot, dedicated to the paper tracker. The .env token
// PAPER_TRACKER_TELEGRAM_TOKEN belongs to a different bot than the NUSSync
// course bot, so it needs its own getUpdates loop, its own pairing and its own
// chat id — Telegram hands each update to exactly one poller per bot token, so
// the two loops never contend with each other.
//
// Everything paper-specific (search, digest building, the library) lives in
// app_papers.go and reaches this file through the hook functions below; notify
// stays a transport.

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"nussync/internal/config"
	"nussync/internal/store"
	"nussync/internal/telegram"
)

// kvPaperDigest records the date of the last digest sent, so a restart inside
// the digest hour does not send it twice.
const kvPaperDigest = "papers_last_digest"

// PaperBot polls the paper bot and answers /paper, /save, /reading, /help.
type PaperBot struct {
	Store *store.Store
	Now   func() time.Time

	// Hooks supplied by the app. Each returns a ready-to-send HTML body.
	Digest  func(ctx context.Context) (string, error)
	Save    func(ctx context.Context, n int) (string, error)
	Reading func(ctx context.Context) (string, error)
	Help    func() string
	// SendDigest pushes the daily digest at DigestHour. Nil disables it.
	SendDigest func(ctx context.Context) error
	// DigestHour and Enabled read the live settings.
	DigestHour func() int
	Enabled    func() bool
	// OnPair persists a newly discovered chat id.
	OnPair func(chatID string)

	mu      sync.Mutex
	client  *telegram.Client
	chatID  string
	running bool
	waiters []chan string

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewPaperBot builds the paper bot (not started).
func NewPaperBot(st *store.Store, client *telegram.Client, cfg config.Settings) *PaperBot {
	return &PaperBot{
		Store: st, Now: time.Now, client: client,
		chatID: strings.TrimSpace(cfg.PaperTelegramChatID),
	}
}

// SetConfig swaps in new settings and/or a new client at runtime.
func (b *PaperBot) SetConfig(cfg config.Settings, client *telegram.Client) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.chatID = strings.TrimSpace(cfg.PaperTelegramChatID)
	if client != nil {
		b.client = client
	}
}

// Running reports whether the poll loop is alive.
func (b *PaperBot) Running() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.running
}

// ChatID is the paired chat, "" when unpaired.
func (b *PaperBot) ChatID() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.chatID
}

func (b *PaperBot) snapshot() (*telegram.Client, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.client, b.chatID
}

// Start launches the poll loop and the digest ticker.
func (b *PaperBot) Start(ctx context.Context) {
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

	b.wg.Add(2)
	go func() {
		defer b.wg.Done()
		defer func() {
			b.mu.Lock()
			b.running = false
			b.mu.Unlock()
		}()
		b.loop(ctx)
	}()
	go func() {
		defer b.wg.Done()
		b.digestLoop(ctx)
	}()
}

// Stop halts both goroutines.
func (b *PaperBot) Stop() {
	b.mu.Lock()
	cancel := b.cancel
	b.cancel = nil
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	b.wg.Wait()
}

// AwaitPair blocks until an unpaired chat sends /start, returning its chat id.
func (b *PaperBot) AwaitPair(ctx context.Context, timeout time.Duration) string {
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

func (b *PaperBot) pair(chatID string) {
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

func (b *PaperBot) loop(ctx context.Context) {
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

func (b *PaperBot) dispatch(ctx context.Context, u telegram.Update) {
	if u.ChatID == "" {
		return
	}
	_, paired := b.snapshot()
	cmd, args, ok := ParseCommand(u.Text)

	if paired == "" {
		if ok && cmd == "/start" {
			b.pair(u.ChatID)
			go b.reply(ctx, u.ChatID, "✅ Paired with the NUSSync paper tracker.\n\n"+b.help())
		}
		return
	}
	if u.ChatID != paired {
		return
	}
	if !ok {
		go b.reply(ctx, u.ChatID, b.help())
		return
	}
	go b.handle(ctx, u.ChatID, cmd, args)
}

func (b *PaperBot) help() string {
	if b.Help != nil {
		return b.Help()
	}
	return "/paper /save &lt;n&gt; /reading /help"
}

func (b *PaperBot) handle(ctx context.Context, chatID, cmd, args string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("nussync: paper command %s: %v", cmd, r)
		}
	}()

	switch cmd {
	case "/paper":
		if b.Digest == nil {
			b.reply(ctx, chatID, "Papers are not available right now.")
			return
		}
		b.reply(ctx, chatID, "🔎 Building today's digest…")
		msg, err := b.Digest(ctx)
		if err != nil {
			b.reply(ctx, chatID, "❌ "+telegram.EscapeHTML(err.Error()))
			return
		}
		b.reply(ctx, chatID, msg)

	case "/save":
		n, err := strconv.Atoi(strings.TrimSpace(args))
		if err != nil || n <= 0 {
			b.reply(ctx, chatID, "Usage: <code>/save &lt;n&gt;</code> — the number from the last digest.")
			return
		}
		if b.Save == nil {
			b.reply(ctx, chatID, "Papers are not available right now.")
			return
		}
		msg, err := b.Save(ctx, n)
		if err != nil {
			b.reply(ctx, chatID, "❌ "+telegram.EscapeHTML(err.Error()))
			return
		}
		b.reply(ctx, chatID, msg)

	case "/reading":
		if b.Reading == nil {
			b.reply(ctx, chatID, "Papers are not available right now.")
			return
		}
		msg, err := b.Reading(ctx)
		if err != nil {
			b.reply(ctx, chatID, "❌ "+telegram.EscapeHTML(err.Error()))
			return
		}
		b.reply(ctx, chatID, msg)

	default: // /help, /start, anything else
		b.reply(ctx, chatID, b.help())
	}
}

func (b *PaperBot) reply(ctx context.Context, chatID, body string) {
	client, _ := b.snapshot()
	if client == nil {
		return
	}
	for _, chunk := range SplitMessage(body, MaxMessage) {
		if err := client.SendMessage(ctx, chatID, chunk); err != nil {
			if ctx.Err() == nil {
				log.Printf("nussync: paper reply: %v", err)
			}
			return
		}
	}
}

// digestLoop checks once a minute whether the daily digest is due.
func (b *PaperBot) digestLoop(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			b.maybeDigest(ctx)
		}
	}
}

func (b *PaperBot) maybeDigest(ctx context.Context) {
	if b.SendDigest == nil || b.Store == nil {
		return
	}
	if b.Enabled != nil && !b.Enabled() {
		return
	}
	if _, chat := b.snapshot(); strings.TrimSpace(chat) == "" {
		return
	}
	hour := 9
	if b.DigestHour != nil {
		hour = b.DigestHour()
	}
	now := time.Now
	if b.Now != nil {
		now = b.Now
	}
	local := now().Local()
	if local.Hour() != hour {
		return
	}
	today := local.Format("2006-01-02")
	if last, _ := b.Store.GetKV(kvPaperDigest); last == today {
		return
	}
	// Stamp before sending: a failure must not retry every minute for an hour.
	_ = b.Store.SetKV(kvPaperDigest, today)
	if err := b.SendDigest(ctx); err != nil && ctx.Err() == nil {
		log.Printf("nussync: paper digest: %v", err)
	}
}
