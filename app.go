package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	gosync "sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"nussync/internal/canvas"
	"nussync/internal/config"
	"nussync/internal/notify"
	"nussync/internal/store"
	"nussync/internal/sync"
	"nussync/internal/telegram"
)

// App is the Wails-bound backend. See docs/API_CONTRACT.md.
type App struct {
	ctx context.Context

	mu       gosync.RWMutex
	cfg      config.Settings
	status   SyncStatus
	canvas   *canvas.Client
	telegram *telegram.Client

	st        *store.Store
	sched     *notify.Scheduler
	syncMu    gosync.Mutex
	syncing   bool
	cancelFn  context.CancelFunc
	lastEmit  time.Time
	timerStop chan struct{}

	headless bool // CLI mode: no Wails runtime available
}

// NewApp creates the App.
func NewApp() *App {
	return &App{status: SyncStatus{Phase: "idle"}}
}

// ---------------------------------------------------------------- lifecycle

// Init loads config and opens the store. Safe to call in headless CLI mode.
func (a *App) Init(headless bool) error {
	a.headless = headless

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if config.ImportEnvInto(&cfg) {
		_ = config.Save(cfg)
	}

	dbPath, err := config.DBPath()
	if err != nil {
		return err
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	a.mu.Lock()
	a.cfg = cfg
	a.st = st
	a.canvas = canvas.New(cfg.CanvasURL, cfg.CanvasToken)
	a.telegram = telegram.New(cfg.TelegramToken)
	a.mu.Unlock()

	last, _ := st.GetKV("last_sync")
	a.mu.Lock()
	a.status.LastRun = last
	a.mu.Unlock()

	return nil
}

// startup is the Wails OnStartup hook.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if a.st == nil {
		if err := a.Init(false); err != nil {
			log.Printf("nussync: startup: %v", err)
			return
		}
	}

	cfg := a.settings()
	a.sched = notify.NewScheduler(a.st, a.telegram, cfg)
	a.sched.Start(ctx)

	a.applyLaunchAtLogin(cfg.LaunchAtLogin)
	a.restartSyncTimer(cfg.SyncIntervalMin)

	// Kick off an initial sync shortly after launch.
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
		if cfg.CanvasToken != "" {
			_ = a.SyncNow()
		}
	}()
}

// shutdown is the Wails OnShutdown hook.
func (a *App) shutdown(context.Context) {
	if a.sched != nil {
		a.sched.Stop()
	}
	a.stopSyncTimer()
	if a.st != nil {
		_ = a.st.Close()
	}
}

func (a *App) settings() config.Settings {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

// emit publishes a Wails event, unless running headless.
func (a *App) emit(name string, data ...any) {
	if a.headless || a.ctx == nil {
		return
	}
	wruntime.EventsEmit(a.ctx, name, data...)
}

func (a *App) toast(level, msg string) {
	a.emit("toast", Toast{Level: level, Message: msg})
}

// ------------------------------------------------------------------ courses

// GetCourses lists all known courses.
func (a *App) GetCourses() ([]Course, error) {
	if a.st == nil {
		return nil, errors.New("not initialised")
	}
	rows, err := a.st.Courses()
	if err != nil {
		return nil, err
	}
	out := make([]Course, 0, len(rows))
	for _, r := range rows {
		out = append(out, toCourse(r))
	}
	return out, nil
}

// SetCourseEnabled includes or excludes a course from syncing.
func (a *App) SetCourseEnabled(id int, enabled bool) error {
	if a.st == nil {
		return errors.New("not initialised")
	}
	return a.st.SetCourseEnabled(id, enabled)
}

// -------------------------------------------------------------------- files

// GetTree returns the nested file tree of one course. courseID 0 returns every
// course's tree, each wrapped in a top-level folder named after the course
// code — the command palette calls GetTree(0) once to build its file index.
func (a *App) GetTree(courseID int) ([]FileNode, error) {
	if a.st == nil {
		return nil, errors.New("not initialised")
	}
	courses, err := a.st.Courses()
	if err != nil {
		return nil, err
	}
	syncDir := a.settings().SyncDir

	if courseID != 0 {
		files, err := a.st.FilesByCourse(courseID)
		if err != nil {
			return nil, err
		}
		code := ""
		for _, c := range courses {
			if c.ID == courseID {
				code = c.Code
				break
			}
		}
		return BuildTree(courseID, filepath.Join(syncDir, sync.CourseFolder(code)), files), nil
	}

	out := make([]FileNode, 0, len(courses))
	for _, c := range courses {
		files, err := a.st.FilesByCourse(c.ID)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			continue
		}
		root := filepath.Join(syncDir, sync.CourseFolder(c.Code))
		children := BuildTree(c.ID, root, files)
		var size int64
		for _, ch := range children {
			size += ch.Size
		}
		out = append(out, FileNode{
			CourseID: c.ID, Name: c.Code, Path: root, RelPath: "",
			IsDir: true, Size: size, Source: "files",
			Synced: true, Children: children,
		})
	}
	return out, nil
}

// GetRecentFiles lists the most recently modified files across all courses.
func (a *App) GetRecentFiles(limit int) ([]FileNode, error) {
	if a.st == nil {
		return nil, errors.New("not initialised")
	}
	files, err := a.st.RecentFiles(limit)
	if err != nil {
		return nil, err
	}
	out := make([]FileNode, 0, len(files))
	for _, f := range files {
		out = append(out, toFileNode(f))
	}
	return out, nil
}

// Search runs a full-text search over file names and extracted text.
// courseID 0 searches every course.
func (a *App) Search(query string, courseID int) ([]SearchHit, error) {
	if a.st == nil {
		return nil, errors.New("not initialised")
	}
	hits, err := a.st.Search(query, courseID, 200)
	if err != nil {
		return nil, err
	}
	out := make([]SearchHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, SearchHit{
			File:       toFileNode(h.File),
			CourseCode: h.CourseCode,
			Snippet:    h.Snippet,
			Score:      h.Score,
		})
	}
	return out, nil
}

// OpenFile opens a local file with the OS default application.
func (a *App) OpenFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("empty path")
	}
	return openWithShell(path)
}

// RevealFile shows a file in Explorer with it selected.
func (a *App) RevealFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("empty path")
	}
	return revealInExplorer(path)
}

// OpenURL opens a URL in the user's browser.
func (a *App) OpenURL(url string) error {
	if strings.TrimSpace(url) == "" {
		return errors.New("empty url")
	}
	if !a.headless && a.ctx != nil {
		wruntime.BrowserOpenURL(a.ctx, url)
		return nil
	}
	return openWithShell(url)
}

// --------------------------------------------------------------------- sync

// SyncNow starts a sync in the background and returns immediately.
func (a *App) SyncNow() error {
	a.syncMu.Lock()
	if a.syncing {
		a.syncMu.Unlock()
		return errors.New("sync already running")
	}
	if a.st == nil {
		a.syncMu.Unlock()
		return errors.New("not initialised")
	}
	cfg := a.settings()
	if strings.TrimSpace(cfg.CanvasToken) == "" {
		a.syncMu.Unlock()
		return errors.New("no Canvas token configured")
	}

	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	a.syncing = true
	a.cancelFn = cancel
	a.syncMu.Unlock()

	a.mu.Lock()
	a.status = SyncStatus{Running: true, Phase: "listing", LastRun: a.status.LastRun}
	a.mu.Unlock()

	go a.runSync(ctx, cfg)
	return nil
}

func (a *App) runSync(ctx context.Context, cfg config.Settings) {
	defer func() {
		a.syncMu.Lock()
		a.syncing = false
		a.cancelFn = nil
		a.syncMu.Unlock()
	}()

	eng := sync.New(a.canvasClient(), a.st, cfg, a.onProgress)
	res, err := eng.Run(ctx)

	a.mu.Lock()
	a.status.Running = false
	a.status.CurrentFile = ""
	a.status.BytesDownloaded = res.BytesDownloaded
	a.status.LastRun = time.Now().Format(time.RFC3339)
	switch {
	case errors.Is(err, context.Canceled):
		a.status.Phase = "idle"
		a.status.LastError = "cancelled"
	case err != nil:
		a.status.Phase = "error"
		a.status.LastError = err.Error()
	default:
		a.status.Phase = "idle"
		a.status.LastError = ""
		if len(res.Errors) > 0 {
			a.status.LastError = res.Errors[0]
		}
	}
	final := a.status
	a.mu.Unlock()

	a.emit("sync:status", final)
	a.emit("sync:done", final)
	a.emit("deadlines:updated")

	if err == nil {
		// Only announcements first seen in this run. The `notified` flag is the
		// Telegram scheduler's, and stays set-free while the bot is unpaired —
		// keying off it would re-toast the whole backlog on every sync.
		if len(res.NewAnnouncementIDs) > 0 {
			fresh := map[int]bool{}
			for _, id := range res.NewAnnouncementIDs {
				fresh[id] = true
			}
			if anns, e := a.st.Announcements(len(fresh) * 4); e == nil {
				out := make([]Announcement, 0, len(fresh))
				for _, an := range anns {
					if fresh[an.ID] {
						out = append(out, toAnnouncement(an))
					}
				}
				if len(out) > 0 {
					a.emit("announcements:new", out)
				}
			}
		}
		a.toast("success", fmt.Sprintf("Synced %d files (%s)",
			res.FilesDownloaded, humanBytes(res.BytesDownloaded)))
	} else if !errors.Is(err, context.Canceled) {
		a.toast("error", "Sync failed: "+err.Error())
	}

	if a.sched != nil {
		a.sched.Kick()
	}
}

// onProgress mirrors engine progress into SyncStatus, throttled to ~4/s.
func (a *App) onProgress(p sync.Progress) {
	a.mu.Lock()
	a.status.Running = true
	a.status.Phase = p.Phase
	a.status.Course = p.Course
	a.status.Done = p.Done
	a.status.Total = p.Total
	a.status.CurrentFile = p.CurrentFile
	if p.BytesDownloaded > 0 {
		a.status.BytesDownloaded = p.BytesDownloaded
	}
	if p.Err != "" {
		a.status.LastError = p.Err
	}
	snapshot := a.status
	now := time.Now()
	throttled := now.Sub(a.lastEmit) < 250*time.Millisecond
	if !throttled {
		a.lastEmit = now
	}
	a.mu.Unlock()

	if !throttled {
		a.emit("sync:status", snapshot)
	}
}

// CancelSync aborts a running sync.
func (a *App) CancelSync() error {
	a.syncMu.Lock()
	cancel := a.cancelFn
	a.syncMu.Unlock()
	if cancel == nil {
		return errors.New("no sync running")
	}
	cancel()
	return nil
}

// GetSyncStatus returns the current sync state.
func (a *App) GetSyncStatus() SyncStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.status
}

// ------------------------------------------------- deadlines / news / grades

// GetDeadlines lists upcoming deadlines plus overdue-unsubmitted ones.
func (a *App) GetDeadlines() ([]Deadline, error) {
	if a.st == nil {
		return nil, errors.New("not initialised")
	}
	rows, err := a.st.Deadlines(time.Now())
	if err != nil {
		return nil, err
	}
	out := make([]Deadline, 0, len(rows))
	for _, d := range rows {
		out = append(out, toDeadline(d))
	}
	return out, nil
}

// GetAnnouncements lists recent announcements.
func (a *App) GetAnnouncements(limit int) ([]Announcement, error) {
	if a.st == nil {
		return nil, errors.New("not initialised")
	}
	rows, err := a.st.Announcements(limit)
	if err != nil {
		return nil, err
	}
	out := make([]Announcement, 0, len(rows))
	for _, an := range rows {
		out = append(out, toAnnouncement(an))
	}
	return out, nil
}

// MarkAnnouncementRead flags an announcement as read.
func (a *App) MarkAnnouncementRead(id int) error {
	if a.st == nil {
		return errors.New("not initialised")
	}
	return a.st.MarkAnnouncementRead(id)
}

// GetGrades lists graded submissions, newest first.
func (a *App) GetGrades() ([]Grade, error) {
	if a.st == nil {
		return nil, errors.New("not initialised")
	}
	rows, err := a.st.Grades()
	if err != nil {
		return nil, err
	}
	out := make([]Grade, 0, len(rows))
	for _, g := range rows {
		out = append(out, toGrade(g))
	}
	return out, nil
}

// ----------------------------------------------------------------- settings

// GetSettings returns the current configuration.
func (a *App) GetSettings() Settings {
	return toSettings(a.settings())
}

// SaveSettings persists settings and applies them immediately.
func (a *App) SaveSettings(s Settings) error {
	cfg := fromSettings(s)
	if err := config.Save(cfg); err != nil {
		return err
	}
	// Re-read so defaults/normalisation are reflected back.
	saved, err := config.Load()
	if err != nil {
		saved = cfg
	}

	a.mu.Lock()
	a.cfg = saved
	a.canvas = canvas.New(saved.CanvasURL, saved.CanvasToken)
	a.telegram = telegram.New(saved.TelegramToken)
	tg := a.telegram
	a.mu.Unlock()

	if a.sched != nil {
		a.sched.SetSettings(saved, tg)
	}
	a.applyLaunchAtLogin(saved.LaunchAtLogin)
	a.restartSyncTimer(saved.SyncIntervalMin)
	return nil
}

// TestCanvas verifies the token and returns the user's display name.
func (a *App) TestCanvas() (string, error) {
	ctx, cancel := context.WithTimeout(a.baseCtx(), 30*time.Second)
	defer cancel()
	u, err := a.canvasClient().Self(ctx)
	if err != nil {
		return "", err
	}
	if u.Name == "" {
		return u.ShortName, nil
	}
	return u.Name, nil
}

func (a *App) canvasClient() *canvas.Client {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.canvas
}

func (a *App) telegramClient() *telegram.Client {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.telegram
}

func (a *App) baseCtx() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

// ----------------------------------------------------------------- telegram

// GetTelegramStatus reports the bot pairing state.
func (a *App) GetTelegramStatus() TelegramStatus {
	cfg := a.settings()
	st := TelegramStatus{
		ChatID:  cfg.TelegramChatID,
		BotName: "@" + telegram.BotUsername,
	}
	st.Configured = strings.TrimSpace(cfg.TelegramToken) != "" &&
		strings.TrimSpace(cfg.TelegramChatID) != ""
	if strings.TrimSpace(cfg.TelegramToken) != "" {
		ctx, cancel := context.WithTimeout(a.baseCtx(), 10*time.Second)
		defer cancel()
		if b, err := a.telegramClient().GetMe(ctx); err == nil && b.Username != "" {
			st.BotName = "@" + b.Username
		}
	}
	return st
}

// PairTelegram waits up to 60s for the user to send /start, then saves the chat.
func (a *App) PairTelegram() (string, error) {
	cfg := a.settings()
	if strings.TrimSpace(cfg.TelegramToken) == "" {
		return "", errors.New("no Telegram bot token configured")
	}
	tg := a.telegramClient()

	ctx, cancel := context.WithTimeout(a.baseCtx(), 65*time.Second)
	defer cancel()

	chatID, err := tg.FindExistingChat(ctx)
	if err == nil && chatID != "" {
		return a.savePairedChat(chatID)
	}
	chatID, err = tg.AwaitStart(ctx, 60*time.Second)
	if err != nil {
		return "", err
	}
	return a.savePairedChat(chatID)
}

func (a *App) savePairedChat(chatID string) (string, error) {
	a.mu.Lock()
	a.cfg.TelegramChatID = chatID
	cfg := a.cfg
	tg := a.telegram
	a.mu.Unlock()

	if err := config.Save(cfg); err != nil {
		return "", err
	}
	if a.sched != nil {
		a.sched.SetSettings(cfg, tg)
	}
	return chatID, nil
}

// SendTestTelegram sends a confirmation message to the paired chat.
func (a *App) SendTestTelegram() error {
	cfg := a.settings()
	if strings.TrimSpace(cfg.TelegramChatID) == "" {
		return errors.New("Telegram not paired: send /start to @" + telegram.BotUsername + " then click Pair")
	}
	ctx, cancel := context.WithTimeout(a.baseCtx(), 30*time.Second)
	defer cancel()
	return a.telegramClient().SendMessage(ctx, cfg.TelegramChatID,
		"✅ <b>NUSSync</b> is connected. You'll get deadline reminders here.")
}

// -------------------------------------------------------------------- misc

// ChooseSyncDir opens a native folder picker; returns "" if cancelled.
func (a *App) ChooseSyncDir() (string, error) {
	if a.headless || a.ctx == nil {
		return "", errors.New("no GUI available")
	}
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:                "Choose the NUSSync folder",
		DefaultDirectory:     a.settings().SyncDir,
		CanCreateDirectories: true,
	})
}

// GetStats summarises the local library.
func (a *App) GetStats() (Stats, error) {
	if a.st == nil {
		return Stats{}, errors.New("not initialised")
	}
	s, err := a.st.Stats()
	if err != nil {
		return Stats{}, err
	}
	return Stats{
		Files:     s.Files,
		Bytes:     s.Bytes,
		Courses:   s.Courses,
		Deadlines: s.Deadlines,
		LastSync:  s.LastSync,
	}, nil
}

// --------------------------------------------------------- periodic syncing

func (a *App) restartSyncTimer(minutes int) {
	a.stopSyncTimer()
	if minutes <= 0 {
		return
	}
	stop := make(chan struct{})
	a.mu.Lock()
	a.timerStop = stop
	a.mu.Unlock()

	go func() {
		t := time.NewTicker(time.Duration(minutes) * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-a.baseCtx().Done():
				return
			case <-t.C:
				if err := a.SyncNow(); err != nil {
					log.Printf("nussync: scheduled sync: %v", err)
				}
			}
		}
	}()
}

func (a *App) stopSyncTimer() {
	a.mu.Lock()
	stop := a.timerStop
	a.timerStop = nil
	a.mu.Unlock()
	if stop != nil {
		close(stop)
	}
}

// openWithShell launches a path or URL with the Windows shell.
func openWithShell(target string) error {
	cmd := exec.Command("cmd", "/c", "start", "", target)
	cmd.SysProcAttr = hiddenWindow()
	return cmd.Start()
}

// revealInExplorer opens Explorer with the file selected.
func revealInExplorer(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	// explorer.exe returns exit code 1 even on success; ignore Wait errors.
	cmd := exec.Command("explorer", "/select,"+abs)
	_ = cmd.Start()
	go func() { _ = cmd.Wait() }()
	return nil
}
