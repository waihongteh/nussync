# NUSSync — Go <-> Frontend API contract

Backend = Go struct `App` in `app.go` (package main), bound via Wails v2.
Frontend calls `window.go.main.App.<Method>(...)` (Promise). Events via
Wails runtime `EventsOn`. Types below are Go; JSON field names are the Go
field names exactly (no json tags), so frontend TS uses the same PascalCase.

## Types
```go
type Course struct {
    ID int; Code string; Name string; Term string
    FileCount int; LastSynced string /*RFC3339 or ""*/; Enabled bool
    Color string /*hex, deterministic from ID*/
}
type FileNode struct {
    ID int /*canvas file id, 0 for folder*/; CourseID int
    Name string; Path string /*absolute local path*/; RelPath string
    IsDir bool; Size int64; ModifiedAt string /*RFC3339*/
    Source string /*"files"|"modules"|"pages"*/; Module string /*module title, or
      for "pages" the linking context: "<Module> / <Page>", "Pages / <Page>",
      "Assignments / <name>", "Announcements / <title>"; "" when unknown*/
    Synced bool /*downloaded locally*/; Children []FileNode /*for dirs*/
}
type SearchHit struct { File FileNode; CourseCode string; Snippet string; Score float64 }
// Snippet marks matched terms with literal <b>…</b> (SQLite FTS5 snippet()).
// The rest is extracted document text: escape it, then re-enable only those
// markers (frontend util.snippetHTML).
type Deadline struct {
    ID int; CourseID int; CourseCode string; Title string
    Type string /*"assignment"|"quiz"|"discussion"*/
    DueAt string /*RFC3339*/; Submitted bool; URL string; PointsPossible float64
}
type Announcement struct {
    ID int; CourseCode string; Title string; PostedAt string
    HTML string; Text string /*plain*/; URL string; Read bool
}
type Grade struct { CourseCode string; Title string; Score float64; Possible float64; GradedAt string; URL string }
type Settings struct {
    CanvasURL string; CanvasToken string; SyncDir string
    TelegramToken string; TelegramChatID string
    ReminderLadder []string /*Go durations e.g. ["72h","48h","24h","3h","1h"]*/
    MaxFileMB int /*skip larger; 0 = no limit*/; SkipExts []string /*[".mp4"]*/
    SyncIntervalMin int; NotifyAnnouncements bool; NotifyGrades bool
    LaunchAtLogin bool; Theme string /*"system"|"light"|"dark"*/
}
type SyncStatus struct {
    Running bool; Phase string /*"idle"|"listing"|"downloading"|"indexing"|"error"*/
    Course string; Done int; Total int; CurrentFile string
    LastRun string; LastError string; BytesDownloaded int64
}
type TelegramStatus struct { Configured bool; ChatID string; BotName string /*includes the leading @*/ }
type Stats struct { Files int; Bytes int64; Courses int; Deadlines int; LastSync string }
```

## Methods on App
```
GetCourses() ([]Course, error)
SetCourseEnabled(id int, enabled bool) error
GetTree(courseID int) ([]FileNode, error)          // full nested tree; courseID 0
                                                    // = every course, each wrapped in a
                                                    // top-level dir node named Course.Code
GetRecentFiles(limit int) ([]FileNode, error)       // newest ModifiedAt first
Search(query string, courseID int) ([]SearchHit, error)  // courseID 0 = all; FTS over name + extracted text
OpenFile(path string) error                          // default app
RevealFile(path string) error                        // explorer /select
OpenURL(url string) error
SyncNow() error                                      // async, returns immediately
CancelSync() error
GetSyncStatus() SyncStatus
GetDeadlines() ([]Deadline, error)                   // upcoming + overdue-unsubmitted, sorted by DueAt
GetAnnouncements(limit int) ([]Announcement, error)
MarkAnnouncementRead(id int) error
GetGrades() ([]Grade, error)
GetSettings() Settings
SaveSettings(s Settings) error                       // applies immediately (reschedules timers)
TestCanvas() (string, error)                         // returns display name
GetTelegramStatus() TelegramStatus
PairTelegram() (string, error)                       // waits <=60s for user to /start bot; saves+returns chat id
SendTestTelegram() error
ChooseSyncDir() (string, error)                      // native folder dialog; "" if cancelled
GetStats() (Stats, error)
```

## Events (EventsEmit from Go)
- `sync:status` payload SyncStatus (throttled ~4/s)
- `sync:done` payload SyncStatus
- `deadlines:updated` no payload
- `announcements:new` payload []Announcement (only announcements first seen in
  that sync — not everything the Telegram scheduler has yet to send)
- `toast` payload {Level string /*info|success|error*/; Message string}

## Storage
- Config: `%APPDATA%/NUSSync/config.json`. On first run, if `.env` in cwd has
  CANVAS_URL/CANVAS_TOKEN/TELEGRAM_TOKEN, import them. Bot username: @nuscanvassync_bot.
- DB: `%APPDATA%/NUSSync/nussync.db` (SQLite, modernc.org/sqlite, FTS5).
- Default SyncDir: `%USERPROFILE%/NUSSync/<CourseFolder>/...` where CourseFolder
  is the first "/"-separated segment of `Course.Code` (cross-listed modules such
  as `TR3202S/TR3202T/ETP3201S/...` land in `TR3202S/`). The full code stays in
  the DB and the UI. `sync.MigrateCourseFolders` renames libraries still using
  the old `_`-joined naming and rewrites stored paths, so nothing re-downloads.
