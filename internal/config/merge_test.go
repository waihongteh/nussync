package config

import (
	"reflect"
	"strings"
	"testing"
)

// stored is a realistic on-disk config with all three secrets set.
func stored() Settings {
	s := Defaults()
	s.CanvasToken = "canvas-secret"
	s.TelegramToken = "telegram-secret"
	s.PaperTelegramToken = "paper-secret"
	s.TelegramChatID = "878378450"
	s.PaperTelegramChatID = "123456"
	return s
}

// The 2026-09-06 incident: the Settings view sent a struct whose CanvasToken
// was empty and the app wrote it straight to disk.
func TestMergeKeepsSecretsWhenIncomingIsEmpty(t *testing.T) {
	cur := stored()
	in := cur
	in.CanvasToken = ""
	in.TelegramToken = "   " // whitespace counts as empty
	in.PaperTelegramToken = ""
	in.TelegramChatID = ""
	in.PaperTelegramChatID = ""

	got, kept := Merge(cur, in)

	if got.CanvasToken != cur.CanvasToken {
		t.Errorf("CanvasToken = %q, want the stored one preserved", got.CanvasToken)
	}
	if got.TelegramToken != cur.TelegramToken {
		t.Errorf("TelegramToken was blanked")
	}
	if got.PaperTelegramToken != cur.PaperTelegramToken {
		t.Errorf("PaperTelegramToken was blanked")
	}
	if got.TelegramChatID != cur.TelegramChatID || got.PaperTelegramChatID != cur.PaperTelegramChatID {
		t.Errorf("a stale draft un-paired a bot")
	}
	for _, want := range []string{"CanvasToken", "TelegramToken", "PaperTelegramToken",
		"TelegramChatID", "PaperTelegramChatID"} {
		if !contains(kept, want) {
			t.Errorf("kept = %v, want it to report %s", kept, want)
		}
	}
}

func TestMergeAppliesRealChanges(t *testing.T) {
	cur := stored()
	in := cur
	in.CanvasToken = "rotated-token"
	in.SyncIntervalMin = 180
	in.NotifyGrades = false
	in.SkipExts = []string{".mp4"}

	got, kept := Merge(cur, in)
	if len(kept) != 0 {
		t.Errorf("kept = %v, want nothing preserved", kept)
	}
	if got.CanvasToken != "rotated-token" {
		t.Errorf("a new token must win: %q", got.CanvasToken)
	}
	if got.SyncIntervalMin != 180 || got.NotifyGrades {
		t.Errorf("non-secret edits were dropped: %+v", got)
	}
	if !reflect.DeepEqual(got.SkipExts, []string{".mp4"}) {
		t.Errorf("SkipExts = %v", got.SkipExts)
	}
}

// nil means "the caller does not know about this field"; an empty non-nil
// slice means "the user cleared it".
func TestMergeNilSliceKeepsStoredButEmptySliceClears(t *testing.T) {
	cur := stored()
	in := cur
	in.SkipExts = nil
	in.PaperTopVenues = nil
	in.ReminderLadder = []string{}

	got, _ := Merge(cur, in)
	if !reflect.DeepEqual(got.SkipExts, cur.SkipExts) {
		t.Errorf("nil SkipExts should keep the stored list, got %v", got.SkipExts)
	}
	if !reflect.DeepEqual(got.PaperTopVenues, cur.PaperTopVenues) {
		t.Errorf("nil PaperTopVenues should keep the stored list")
	}
	if got.ReminderLadder == nil || len(got.ReminderLadder) != 0 {
		t.Errorf("an explicitly emptied list must survive the merge: %v", got.ReminderLadder)
	}
}

// A draft seeded before GetSettings resolved is all-zero. Nothing in it may
// destroy the stored configuration.
func TestMergeZeroStructKeepsEverythingLoadBearing(t *testing.T) {
	cur := stored()
	got, kept := Merge(cur, Settings{})

	if got.CanvasToken != cur.CanvasToken || got.TelegramToken != cur.TelegramToken ||
		got.PaperTelegramToken != cur.PaperTelegramToken {
		t.Fatalf("an empty struct wiped the secrets")
	}
	if got.CanvasURL != cur.CanvasURL || got.SyncDir != cur.SyncDir || got.Theme != cur.Theme {
		t.Errorf("an empty struct wiped CanvasURL/SyncDir/Theme")
	}
	if len(kept) != 5 {
		t.Errorf("kept = %v, want all five secrets reported", kept)
	}
}

// Clearing a token is still possible, just not by accident.
func TestClearSecretIsTheOnlyWayToBlankOne(t *testing.T) {
	cur := stored()
	for _, f := range secretFields {
		if strings.TrimSpace(*f.get(&cur)) == "" {
			t.Fatalf("%s should be set in the fixture", f.name)
		}
	}
	if len(secretFields) != 5 {
		t.Errorf("secretFields = %d, want 5", len(secretFields))
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
