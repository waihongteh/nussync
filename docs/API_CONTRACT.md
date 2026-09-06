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
    Synced bool /*downloaded locally*/
    IsNew bool /*changed after the feed was last marked seen — badge it;
      see docs/CONTRACT_FEATURES.md*/
    Children []FileNode /*for dirs*/
}
type SearchHit struct { File FileNode; CourseCode string; Snippet string; Score float64 }
// Snippet marks matched terms with literal <b>…</b> (SQLite FTS5 snippet()).
// The rest is extracted document text: escape it, then re-enable only those
// markers (frontend util.snippetHTML).
type Deadline struct {
    ID int; CourseID int; CourseCode string; Title string
    Type string /*"assignment"|"quiz"|"discussion"*/
    DueAt string /*RFC3339*/; Submitted bool; URL string; PointsPossible float64
    // Detail fields: zero-valued from GetDeadlines, filled by
    // GetDeadlineDetail(id). See docs/CONTRACT_FEATURES.md.
    Description string /*raw Canvas HTML — the FRONTEND must sanitize it*/
    SubmissionTypes []string; Attachments []FileNode
    Score float64; Graded bool; Stats *ScoreStats /*null unless disclosed*/
}
type ScoreStats struct { Mean, Min, Max, Median float64; Count int }
type Announcement struct {
    ID int; CourseCode string; Title string; PostedAt string
    HTML string; Text string /*plain*/; URL string; Read bool
}
type Grade struct {
    CourseCode string; Title string; Score float64; Possible float64
    GradedAt string; URL string
    Mean float64 /*class mean, 0 when never cached by GetDeadlineDetail*/
}
type Settings struct {
    CanvasURL string; CanvasToken string; SyncDir string
    TelegramToken string; TelegramChatID string
    ReminderLadder []string /*Go durations e.g. ["72h","48h","24h","3h","1h"]*/
    MaxFileMB int /*skip larger; 0 = no limit*/; SkipExts []string /*[".mp4"]*/
    SyncIntervalMin int; NotifyAnnouncements bool; NotifyGrades bool
    NotifyDesktop bool /*Windows toast after a sync brings new files; default true*/
    LaunchAtLogin bool; Theme string /*"system"|"light"|"dark"*/
    Hotkey string /*global show/hide, default "ctrl+shift+n"; "" disables*/
    // Papers (docs/CONTRACT_PAPERS.md). The paper tracker runs on a SECOND
    // Telegram bot with its own token, chat id and pairing; the token is
    // imported from .env key PAPER_TRACKER_TELEGRAM_TOKEN when empty.
    PaperTelegramToken string; PaperTelegramChatID string
    PaperKeywords []string   /*digest filter; default: machine unlearning, LLM
      unlearning, knowledge editing, model editing, knowledge unlearning,
      memorization*/
    PaperCategories []string /*arXiv categories, default ["cs.CL","cs.LG","cs.AI"]*/
    PaperDigestHour int      /*0-23 local time, default 9*/
    NotifyPapers bool        /*default true*/
    PaperTopVenues []string  /*tier-2 venues for paper ranking; default the
      26 names in docs/CONTRACT_PAPERS.md, empty falls back to that list*/
    PaperPreferPublished bool /*rank published venues above preprints, default true*/
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
GetFileInfo(fileID int) (FileNode, error)            // one file's metadata (viewer header)
GetFileText(fileID int, maxChars int) (string, error) // extracted plain text for the
                                                    // in-app text preview (pptx/docx/xlsx and
                                                    // any indexed format). Reads the FTS index,
                                                    // falling back to a live extraction when the
                                                    // file is not indexed yet; errors when the
                                                    // format yields no text. maxChars <= 0 = no cap.
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

Feature bindings added later (what's-new feed, assignment detail, window
control) live in `docs/CONTRACT_FEATURES.md`. Study/AI lives in
`docs/CONTRACT_STUDY.md`; the Papers tracker, paper summaries and the in-app
Claude chat live in `docs/CONTRACT_PAPERS.md`.

## Asset server routes (assets.go)

Registered as `assetserver.Options.Handler`, so these sit on the same origin as
the frontend (`wails://` in the app, `http://localhost:34115` under `wails dev`)
and the WebView renders them with no external process. Everything not under
`/local/` falls through to the embedded frontend.

```
GET /local/{fileID}      stream the synced file with that id (path from the store)
GET /local/path?p={abs}  stream an absolute path — 403 unless it resolves inside
                         SyncDir (symlinks resolved, case-insensitive on Windows)
```

Both answer with the mapped `Content-Type` (pdf; png/jpg/gif/svg/webp/bmp;
everything textual — txt/md/csv/json/log/source code, **including .html/.htm** —
as `text/plain; charset=utf-8` so course content never runs as markup in the
app's origin), `Content-Length`, `Accept-Ranges: bytes` with full Range support
(the Chromium PDF viewer seeks), `Cache-Control: no-store`, `X-Content-Type-
Options: nosniff` and an inline `Content-Disposition`. 404 when the id is
unknown or the file is not on disk, 400 for a malformed id, 405 for non-GET/HEAD.

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
