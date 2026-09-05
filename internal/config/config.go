// Package config loads and persists NUSSync user settings.
package config

import (
	"bufio"
	"encoding/json"
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
	LaunchAtLogin       bool
	Theme               string
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
		Theme:               "system",
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
}

// ImportEnvInto fills any still-empty credentials from the process environment
// and from a .env file, without overwriting configured values.
func ImportEnvInto(s *Settings) bool {
	before := *s
	env := readDotEnv(".env")
	for _, k := range []string{"CANVAS_URL", "CANVAS_TOKEN", "TELEGRAM_TOKEN", "TELEGRAM_CHAT_ID"} {
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
