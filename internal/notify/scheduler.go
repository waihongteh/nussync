package notify

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"nussync/internal/config"
	"nussync/internal/store"
	"nussync/internal/telegram"
)

// Sender delivers a message; the scheduler is agnostic to the transport.
type Sender interface {
	SendMessage(ctx context.Context, chatID, text string) error
}

// Clock lets tests drive time deterministically.
type Clock func() time.Time

// Scheduler pushes reminders, announcements and grades to Telegram.
type Scheduler struct {
	Store  *store.Store
	Sender Sender
	Now    Clock

	mu       sync.Mutex
	settings config.Settings

	kick   chan struct{}
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewScheduler builds a scheduler (not started).
func NewScheduler(st *store.Store, sender Sender, s config.Settings) *Scheduler {
	return &Scheduler{
		Store:    st,
		Sender:   sender,
		Now:      time.Now,
		settings: s,
		kick:     make(chan struct{}, 1),
	}
}

// SetSettings swaps in new settings (and possibly a new sender) at runtime.
func (s *Scheduler) SetSettings(cfg config.Settings, sender Sender) {
	s.mu.Lock()
	s.settings = cfg
	if sender != nil {
		s.Sender = sender
	}
	s.mu.Unlock()
}

func (s *Scheduler) cfg() config.Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

// Start runs the 5-minute ticker loop until ctx is cancelled or Stop is called.
func (s *Scheduler) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		daily := time.NewTicker(time.Minute)
		defer daily.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.RunOnce(ctx)
			case <-s.kick:
				s.RunOnce(ctx)
			case <-daily.C:
				s.maybeDigest(ctx)
			}
		}
	}()
}

// Stop halts the loop.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

// Kick asks for an immediate pass (non-blocking).
func (s *Scheduler) Kick() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// RunOnce performs a single notification pass.
func (s *Scheduler) RunOnce(ctx context.Context) {
	cfg := s.cfg()
	if s.Sender == nil || strings.TrimSpace(cfg.TelegramChatID) == "" {
		return
	}
	s.checkReminders(ctx, cfg)
	if cfg.NotifyAnnouncements {
		s.checkAnnouncements(ctx, cfg)
	}
	if cfg.NotifyGrades {
		s.checkGrades(ctx, cfg)
	}
}

const kvFirstRun = "notify_bootstrapped"

// checkReminders walks unsubmitted future deadlines and fires due ladder rungs.
func (s *Scheduler) checkReminders(ctx context.Context, cfg config.Settings) {
	ladder := ParseLadder(cfg.ReminderLadder)
	if len(ladder) == 0 {
		return
	}
	now := s.Now()
	deadlines, err := s.Store.PendingDeadlines(now)
	if err != nil {
		return
	}

	bootstrapped, _ := s.Store.GetKV(kvFirstRun)
	firstRun := bootstrapped == ""

	for _, d := range deadlines {
		due, err := time.Parse(time.RFC3339, d.DueAt)
		if err != nil {
			continue
		}
		remaining := due.Sub(now)

		if firstRun {
			// Suppress the backlog: record every elapsed rung except the
			// current one, which is still allowed to send below.
			for _, r := range ladder.Superseded(remaining) {
				_ = s.Store.MarkReminderSent(d.ID, RungKey(r), now)
			}
		}

		rung, ok := ladder.Active(remaining)
		if !ok {
			continue
		}
		sent, err := s.Store.ReminderSent(d.ID, RungKey(rung))
		if err != nil || sent {
			continue
		}
		msg := ReminderText(d, due, remaining)
		if err := s.Sender.SendMessage(ctx, cfg.TelegramChatID, msg); err != nil {
			continue // retry next tick
		}
		_ = s.Store.MarkReminderSent(d.ID, RungKey(rung), now)
	}

	if firstRun {
		_ = s.Store.SetKV(kvFirstRun, now.Format(time.RFC3339))
	}
}

// ReminderText renders the Telegram HTML body for a deadline reminder.
func ReminderText(d store.Deadline, due time.Time, remaining time.Duration) string {
	kind := "due"
	if d.Type == "quiz" {
		kind = "quiz due"
	}
	body := fmt.Sprintf("⏰ <b>%s</b> — %s %s in %s (%s)",
		telegram.EscapeHTML(d.CourseCode),
		telegram.EscapeHTML(d.Title),
		kind,
		Humanize(remaining),
		FormatDue(due),
	)
	if d.URL != "" {
		body += fmt.Sprintf("\n%s", telegram.EscapeHTML(d.URL))
	}
	return body
}

func (s *Scheduler) checkAnnouncements(ctx context.Context, cfg config.Settings) {
	anns, err := s.Store.UnnotifiedAnnouncements()
	if err != nil {
		return
	}
	for _, a := range anns {
		text := a.Text
		if len(text) > 300 {
			text = trimRunes(text, 300) + "…"
		}
		msg := fmt.Sprintf("📢 <b>%s</b> — %s",
			telegram.EscapeHTML(a.CourseCode), telegram.EscapeHTML(a.Title))
		if text != "" {
			msg += "\n\n" + telegram.EscapeHTML(text)
		}
		if a.URL != "" {
			msg += "\n\n" + telegram.EscapeHTML(a.URL)
		}
		if err := s.Sender.SendMessage(ctx, cfg.TelegramChatID, msg); err != nil {
			return
		}
		_ = s.Store.MarkAnnouncementNotified(a.ID)
	}
}

func (s *Scheduler) checkGrades(ctx context.Context, cfg config.Settings) {
	grades, err := s.Store.UnnotifiedGrades()
	if err != nil {
		return
	}
	for _, g := range grades {
		msg := fmt.Sprintf("📊 <b>%s</b> — %s: %s/%s",
			telegram.EscapeHTML(g.CourseCode), telegram.EscapeHTML(g.Title),
			trimFloat(g.Score), trimFloat(g.Possible))
		if err := s.Sender.SendMessage(ctx, cfg.TelegramChatID, msg); err != nil {
			return
		}
		_ = s.Store.MarkGradeNotified(g.AssignmentID)
	}
}

const kvDigest = "notify_last_digest"

// maybeDigest sends a once-daily 08:00 summary of the coming week.
func (s *Scheduler) maybeDigest(ctx context.Context) {
	cfg := s.cfg()
	if s.Sender == nil || strings.TrimSpace(cfg.TelegramChatID) == "" {
		return
	}
	now := s.Now().Local()
	if now.Hour() != 8 {
		return
	}
	today := now.Format("2006-01-02")
	if last, _ := s.Store.GetKV(kvDigest); last == today {
		return
	}
	deadlines, err := s.Store.PendingDeadlines(s.Now())
	if err != nil {
		return
	}
	var lines []string
	horizon := s.Now().Add(7 * 24 * time.Hour)
	for _, d := range deadlines {
		due, err := time.Parse(time.RFC3339, d.DueAt)
		if err != nil || due.After(horizon) {
			continue
		}
		lines = append(lines, fmt.Sprintf("• <b>%s</b> %s — %s",
			telegram.EscapeHTML(d.CourseCode), telegram.EscapeHTML(d.Title), FormatDue(due)))
	}
	_ = s.Store.SetKV(kvDigest, today)
	if len(lines) == 0 {
		return
	}
	msg := "🗓 <b>Due this week</b>\n\n" + strings.Join(lines, "\n")
	_ = s.Sender.SendMessage(ctx, cfg.TelegramChatID, msg)
}

func trimFloat(f float64) string {
	s := fmt.Sprintf("%.2f", f)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

func trimRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && s[len(s)-1]&0xC0 == 0x80 {
		s = s[:len(s)-1]
	}
	return strings.TrimSpace(s)
}
