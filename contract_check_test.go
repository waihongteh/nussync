package main

import "testing"

// TestContractSignatures fails to compile if any method in
// docs/API_CONTRACT.md is missing or has a different signature.
func TestContractSignatures(t *testing.T) {
	a := NewApp()
	var (
		_ func() ([]Course, error)               = a.GetCourses
		_ func(int, bool) error                  = a.SetCourseEnabled
		_ func(int) ([]FileNode, error)          = a.GetTree
		_ func(int) ([]FileNode, error)          = a.GetRecentFiles
		_ func(string, int) ([]SearchHit, error) = a.Search
		_ func(string) error                     = a.OpenFile
		_ func(string) error                     = a.RevealFile
		_ func(string) error                     = a.OpenURL
		_ func() error                           = a.SyncNow
		_ func() error                           = a.CancelSync
		_ func() SyncStatus                      = a.GetSyncStatus
		_ func() ([]Deadline, error)             = a.GetDeadlines
		_ func(int) ([]Announcement, error)      = a.GetAnnouncements
		_ func(int) error                        = a.MarkAnnouncementRead
		_ func() ([]Grade, error)                = a.GetGrades
		_ func() Settings                        = a.GetSettings
		_ func(Settings) error                   = a.SaveSettings
		_ func() (string, error)                 = a.TestCanvas
		_ func() TelegramStatus                  = a.GetTelegramStatus
		_ func() (string, error)                 = a.PairTelegram
		_ func() error                           = a.SendTestTelegram
		_ func() (string, error)                 = a.ChooseSyncDir
		_ func() (Stats, error)                  = a.GetStats

		// docs/CONTRACT_FEATURES.md
		_ func(int) ([]FeedItem, error) = a.GetWhatsNew
		_ func() error                  = a.MarkFeedSeen
		_ func() (int, error)           = a.GetUnseenCount
		_ func(int) (Deadline, error)   = a.GetDeadlineDetail
		_ func()                        = a.ShowWindow
		_ func()                        = a.HideWindow
		_ func()                        = a.ToggleWindow
	)
}
