package main

import (
	"testing"

	"nussync/internal/config"
)

// A Settings struct from the UI must never be able to blank a stored secret.
// This is the 2026-09-06 regression: config.json ended up with CanvasToken ""
// while the two Telegram tokens survived, and every Canvas call 401'd.
func TestSaveSettingsGuardKeepsStoredSecrets(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	full := config.Defaults()
	full.CanvasToken = "canvas-secret"
	full.TelegramToken = "telegram-secret"
	full.PaperTelegramToken = "paper-secret"
	if err := config.Save(full); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	app := NewApp()

	// The UI sends everything it has, but with an empty Canvas token.
	incoming := toSettings(full)
	incoming.CanvasToken = ""
	incoming.SyncIntervalMin = 60 // a real edit alongside it

	merged, kept := app.mergeIncomingSettings(incoming)
	if merged.CanvasToken != "canvas-secret" {
		t.Fatalf("CanvasToken = %q, want the stored token preserved", merged.CanvasToken)
	}
	if merged.TelegramToken != "telegram-secret" || merged.PaperTelegramToken != "paper-secret" {
		t.Errorf("the other secrets were disturbed: %+v", merged)
	}
	if merged.SyncIntervalMin != 60 {
		t.Errorf("SyncIntervalMin = %d, want the real edit to land", merged.SyncIntervalMin)
	}
	if len(kept) != 1 || kept[0] != "CanvasToken" {
		t.Errorf("kept = %v, want exactly [CanvasToken]", kept)
	}

	// And an all-zero struct (a draft built before GetSettings resolved).
	merged, kept = app.mergeIncomingSettings(Settings{})
	if merged.CanvasToken != "canvas-secret" {
		t.Errorf("an empty draft wiped the Canvas token")
	}
	if len(kept) == 0 {
		t.Errorf("kept = %v, want the preserved secrets reported", kept)
	}

	// A genuinely rotated token still wins.
	incoming = toSettings(full)
	incoming.CanvasToken = "rotated"
	merged, kept = app.mergeIncomingSettings(incoming)
	if merged.CanvasToken != "rotated" || len(kept) != 0 {
		t.Errorf("rotation blocked: token=%q kept=%v", merged.CanvasToken, kept)
	}
}
