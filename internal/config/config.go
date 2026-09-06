// Package config loads and persists NUSSync user settings.
package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Settings mirrors the API contract Settings type exactly.
type Settings struct {
	CanvasURL      string
	CanvasToken    string
	SyncDir        string
	TelegramToken  string
	TelegramChatID string

	ReminderLadder []string
	MaxFileMB      int
	SkipExts       []string

	SyncIntervalMin     int
	NotifyAnnouncements bool
	NotifyGrades        bool
	NotifyDesktop       bool
	LaunchAtLogin       bool
	Theme               string
	// Hotkey is a global show/hide shortcut, e.g. "ctrl+shift+n". Empty
	// disables the hotkey entirely.
	Hotkey string

	// Papers feature (docs/CONTRACT_PAPERS.md). The paper tracker uses a
	// SECOND Telegram bot, so it has its own token and chat id; the NUSSync
	// bot keeps the course commands.
	PaperTelegramToken  string
	PaperTelegramChatID string
	PaperKeywords       []string
	PaperCategories     []string
	PaperDigestHour     int
	NotifyPapers        bool
	// Venue ranking: PaperTopVenues are the tier-2 venues, editable by the
	// user; PaperPreferPublished turns the tier weight in papers.Score on.
	PaperTopVenues       []string
	PaperPreferPublished bool
}

// Defaults returns the baseline settings used on first run.
func Defaults() Settings {
	home, _ := os.UserHomeDir()
	return Settings{
		CanvasURL:           "https://canvas.nus.edu.sg",
		SyncDir:             filepath.Join(home, "NUSSync"),
		ReminderLadder:      []string{"72h", "48h", "24h", "3h", "1h"},
		MaxFileMB:           500,
		SkipExts:            []string{".mp4", ".mov", ".mkv"},
		SyncIntervalMin:     30,
		NotifyAnnouncements: true,
		NotifyGrades:        true,
		NotifyDesktop:       true,
		Theme:               "system",
		Hotkey:              "ctrl+shift+n",
		PaperKeywords: []string{
			"machine unlearning", "LLM unlearning", "knowledge editing",
			"model editing", "knowledge unlearning", "memorization",
		},
		PaperCategories:      []string{"cs.CL", "cs.LG", "cs.AI"},
		PaperDigestHour:      9,
		NotifyPapers:         true,
		PaperTopVenues:       DefaultTopVenues(),
		PaperPreferPublished: true,
	}
}

// DefaultTopVenues is the shipped tier-2 venue list. It must stay identical to
// papers.DefaultTopVenues() — config cannot import papers (papers -> sync ->
// config is an import cycle), so TestTopVenueDefaultsMatch in package main
// guards the copy.
func DefaultTopVenues() []string {
	return []string{
		"NeurIPS", "ICML", "ICLR", "ACL", "EMNLP", "NAACL", "EACL", "COLING",
		"AAAI", "IJCAI", "COLM", "TACL", "JMLR", "TMLR", "CVPR", "ICCV",
		"ECCV", "KDD", "WWW", "SIGIR", "USENIX Security", "IEEE S&P", "CCS",
		"NDSS", "ICSE", "FSE",
	}
}

var mu sync.Mutex

// Dir is %APPDATA%/NUSSync (or the platform equivalent).
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "NUSSync")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// Path is the config.json location.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// DBPath is the SQLite database location.
func DBPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "nussync.db"), nil
}

// Load reads config.json, filling in defaults and importing .env on first run.
func Load() (Settings, error) {
	mu.Lock()
	defer mu.Unlock()

	s := Defaults()
	p, err := Path()
	if err != nil {
		return s, err
	}
	b, err := os.ReadFile(p)
	if err == nil {
		if err := json.Unmarshal(b, &s); err != nil {
			return Defaults(), err
		}
		normalize(&s)
		return s, nil
	}
	if !os.IsNotExist(err) {
		return s, err
	}

	// First run: import from .env in the working directory if present.
	applyEnv(&s, readDotEnv(".env"))
	normalize(&s)
	_ = save(s)
	return s, nil
}

// Save writes settings to disk.
func Save(s Settings) error {
	mu.Lock()
	defer mu.Unlock()
	normalize(&s)
	return save(s)
}

func save(s Settings) error {
	p, err := Path()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func normalize(s *Settings) {
	d := Defaults()
	if strings.TrimSpace(s.CanvasURL) == "" {
		s.CanvasURL = d.CanvasURL
	}
	s.CanvasURL = strings.TrimRight(strings.TrimSpace(s.CanvasURL), "/")
	if strings.TrimSpace(s.SyncDir) == "" {
		s.SyncDir = d.SyncDir
	}
	if len(s.ReminderLadder) == 0 {
		s.ReminderLadder = d.ReminderLadder
	}
	if s.SyncIntervalMin <= 0 {
		s.SyncIntervalMin = d.SyncIntervalMin
	}
	if s.MaxFileMB < 0 {
		s.MaxFileMB = 0
	}
	if s.Theme == "" {
		s.Theme = d.Theme
	}
	// Hotkey is deliberately NOT defaulted here: "" means "disabled", and a
	// config.json missing the key keeps the default because Load() unmarshals
	// on top of Defaults().
	s.Hotkey = strings.ToLower(strings.TrimSpace(s.Hotkey))
	if len(s.PaperKeywords) == 0 {
		s.PaperKeywords = d.PaperKeywords
	}
	if len(s.PaperCategories) == 0 {
		s.PaperCategories = d.PaperCategories
	}
	if s.PaperDigestHour < 0 || s.PaperDigestHour > 23 {
		s.PaperDigestHour = d.PaperDigestHour
	}
	// An empty top-venue list would silently disable tier 2, so it falls back
	// to the shipped list; clearing the ranking is done with the toggle.
	if len(s.PaperTopVenues) == 0 {
		s.PaperTopVenues = d.PaperTopVenues
	}
	for i, e := range s.SkipExts {
		e = strings.ToLower(strings.TrimSpace(e))
		if e != "" && !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		s.SkipExts[i] = e
	}
}

// applyEnv fills empty credential fields from an env map.
func applyEnv(s *Settings, env map[string]string) {
	if v := env["CANVAS_URL"]; v != "" && s.CanvasURL == Defaults().CanvasURL {
		s.CanvasURL = v
	}
	if v := env["CANVAS_TOKEN"]; v != "" && s.CanvasToken == "" {
		s.CanvasToken = v
	}
	if v := env["TELEGRAM_TOKEN"]; v != "" && s.TelegramToken == "" {
		s.TelegramToken = v
	}
	if v := env["TELEGRAM_CHAT_ID"]; v != "" && s.TelegramChatID == "" {
		s.TelegramChatID = v
	}
	// The papers tracker is a separate bot with its own token.
	if v := env["PAPER_TRACKER_TELEGRAM_TOKEN"]; v != "" && s.PaperTelegramToken == "" {
		s.PaperTelegramToken = v
	}
}

// ImportEnvInto fills any still-empty credentials from the process environment
// and from a .env file, without overwriting configured values.
func ImportEnvInto(s *Settings) bool {
	before := *s
	env := readDotEnv(".env")
	for _, k := range []string{"CANVAS_URL", "CANVAS_TOKEN", "TELEGRAM_TOKEN",
		"TELEGRAM_CHAT_ID", "PAPER_TRACKER_TELEGRAM_TOKEN"} {
		if v := os.Getenv(k); v != "" {
			if _, ok := env[k]; !ok {
				env[k] = v
			}
		}
	}
	applyEnv(s, env)
	return before.CanvasToken != s.CanvasToken ||
		before.TelegramToken != s.TelegramToken ||
		before.TelegramChatID != s.TelegramChatID ||
		before.PaperTelegramToken != s.PaperTelegramToken ||
		before.CanvasURL != s.CanvasURL
}

func readDotEnv(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		if k != "" {
			out[k] = v
		}
	}
	return out
}

// Read parses config.json without ever writing to disk. Unlike Load it does
// not create the file, does not import .env and does not normalise anything
// away — it is the read side used by the running app's config watcher, which
// must never be able to author a config.
func Read() (Settings, error) {
	mu.Lock()
	defer mu.Unlock()

	p, err := Path()
	if err != nil {
		return Settings{}, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return Settings{}, err
	}
	s := Defaults()
	if err := json.Unmarshal(b, &s); err != nil {
		return Settings{}, err
	}
	normalize(&s)
	return s, nil
}

// secretFields are the credentials that a caller must never be able to blank
// by accident. They are only ever cleared by an explicit ClearSecret call.
var secretFields = []struct {
	name string
	get  func(*Settings) *string
}{
	{"CanvasToken", func(s *Settings) *string { return &s.CanvasToken }},
	{"TelegramToken", func(s *Settings) *string { return &s.TelegramToken }},
	{"PaperTelegramToken", func(s *Settings) *string { return &s.PaperTelegramToken }},
	// Chat ids are pairing state, not something the settings form edits: the
	// UI only ever sets them through Pair. Treat them as secrets so a stale
	// draft cannot un-pair a bot.
	{"TelegramChatID", func(s *Settings) *string { return &s.TelegramChatID }},
	{"PaperTelegramChatID", func(s *Settings) *string { return &s.PaperTelegramChatID }},
}

// Merge folds an incoming settings struct (typically straight from the UI)
// onto the currently stored one and returns the result plus the names of any
// secrets that were preserved because the caller sent an empty value.
//
// Two classes of field are taken from `stored` rather than `in`:
//
//   - Secrets (tokens and paired chat ids). A blank incoming secret means
//     "the caller did not have it", never "delete it" — a Settings form
//     seeded before GetSettings resolved, or a password input that lost its
//     value, must not be able to wipe the Canvas token off disk. This is the
//     guard for the 2026-09-06 incident where config.json ended up with an
//     empty CanvasToken.
//   - Nil slices. Wails sends `[]` for a list the user emptied and `null` for
//     a field the caller does not know about (an older frontend, a partial
//     struct), so nil means "unchanged" and empty means "cleared".
func Merge(stored, in Settings) (Settings, []string) {
	out := in
	var kept []string
	for _, f := range secretFields {
		cur, next := f.get(&stored), f.get(&out)
		if strings.TrimSpace(*next) == "" && *cur != "" {
			*next = *cur
			kept = append(kept, f.name)
		}
	}
	if strings.TrimSpace(out.CanvasURL) == "" {
		out.CanvasURL = stored.CanvasURL
	}
	if strings.TrimSpace(out.SyncDir) == "" {
		out.SyncDir = stored.SyncDir
	}
	if out.ReminderLadder == nil {
		out.ReminderLadder = stored.ReminderLadder
	}
	if out.SkipExts == nil {
		out.SkipExts = stored.SkipExts
	}
	if out.PaperKeywords == nil {
		out.PaperKeywords = stored.PaperKeywords
	}
	if out.PaperCategories == nil {
		out.PaperCategories = stored.PaperCategories
	}
	if out.PaperTopVenues == nil {
		out.PaperTopVenues = stored.PaperTopVenues
	}
	if strings.TrimSpace(out.Theme) == "" {
		out.Theme = stored.Theme
	}
	return out, kept
}

// ClearSecret blanks one credential on disk. It is the only supported way to
// remove a token, so that Merge can treat every empty incoming secret as
// "unknown" rather than "delete".
func ClearSecret(name string) error {
	cur, err := Read()
	if err != nil {
		return err
	}
	for _, f := range secretFields {
		if f.name == name {
			*f.get(&cur) = ""
			return Save(cur)
		}
	}
	return errors.New("config: unknown secret " + name)
}
