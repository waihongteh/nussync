# NUSSync — feature bindings contract

Companion to `docs/API_CONTRACT.md`, same rules: Go struct `App` in package
`main`, bound via Wails v2, called from the frontend as
`window.go.main.App.<Method>(...)`. No json tags anywhere, so TS field names are
the Go PascalCase names verbatim. Everything here is implemented and live; the
UI is the only missing half.

---

## 1. What's-new feed

Every file row now carries two stamps: `first_seen_at` (written once, on the
first successful download) and `last_changed_at` (rewritten every time the file
is re-downloaded because Canvas reported it changed). A kv row `feed_seen_at`
records when the user last opened the feed.

Rows that predate this feature have empty stamps and never appear in the feed or
the unseen count — the library is not retroactively "new".

### Types
```go
type FeedItem struct {
    ID         int    // canvas file id
    CourseID   int
    CourseCode string // full code, e.g. "CS4246/CS5446"
    Name       string
    Path       string // absolute local path — pass to OpenFile / RevealFile
    RelPath    string
    Size       int64
    ChangedAt  string // RFC3339
    Kind       string // "new" | "updated"
    Module     string // same meaning as FileNode.Module
}
```

### Methods
```
GetWhatsNew(sinceDays int) ([]FeedItem, error)  // newest first; sinceDays <= 0 => 7
MarkFeedSeen() error                            // stamps now; emits feed:updated with 0
GetUnseenCount() (int, error)                   // files changed after feed_seen_at
```

`FileNode.IsNew` (see API_CONTRACT.md) is true for any file changed after
`feed_seen_at`, so the Files view can badge rows without a second call.

### Event
- `feed:updated` — payload `int`, the unseen count. Emitted after every
  successful sync and after `MarkFeedSeen()` (with 0).

### Windows toast
After a sync that downloaded files never seen before, the backend pushes a
native toast: `"NUSSync — 3 new files in CS4246, MA3236"` (courses busiest
first, capped at 4 with a `+N more` suffix). Clicking it calls `ShowWindow()`
in-process. Gated on the new `Settings.NotifyDesktop` (default **true**). No
frontend work is required beyond exposing the toggle in Settings.

---

## 2. Telegram two-way commands

Entirely backend-side; listed here so the Settings screen can describe it.

A single long-poll loop (`internal/notify.Bot`, `getUpdates` with offset and a
50s server timeout) runs for the lifetime of the app. **Telegram delivers each
update to exactly one `getUpdates` caller**, so this loop is the only consumer:
`PairTelegram()` now waits on the loop (`Bot.AwaitPair`) instead of polling in
parallel. In headless CLI mode no loop runs, so `--pair` / `--notify-test` fall
back to the old direct long poll — both still work unchanged.

Messages from any chat other than the paired one are ignored silently. While
unpaired, the first `/start` claims the bot and is persisted.

| Command | Reply |
|---|---|
| `/due` | next 10 unsubmitted deadlines, grouped Overdue / Today / Tomorrow / This week / Later |
| `/new` | files changed in the last 24h, grouped by course, max 30 |
| `/files <query>` | FTS search, top 8, `course · name` |
| `/sync` | replies "🔄 Syncing…", then the summary when the run finishes |
| `/grades` | last 10 graded submissions (with the class mean when cached) |
| `/help`, `/start` | the command list |
| anything else | the command list |

All replies are Telegram HTML and are split into <4000-byte chunks on line
boundaries.

---

## 3. Assignment detail + class stats

### Types
```go
type ScoreStats struct { Mean, Min, Max, Median float64; Count int }
```
`Count` is 0 on Canvas builds that do not report it — treat 0 as "unknown", not
"nobody submitted".

`Deadline` gains (zero-valued from `GetDeadlines`, filled by
`GetDeadlineDetail`):

| Field | Notes |
|---|---|
| `Description string` | **Raw Canvas HTML, deliberately unsanitized server-side.** The frontend MUST sanitize before rendering. |
| `SubmissionTypes []string` | e.g. `["online_upload"]`, `["online_quiz"]` |
| `Attachments []FileNode` | files linked from `Description` that we have already synced, matched by Canvas file id via the existing `sync.FileIDsInHTML` extractor; files we never synced are omitted |
| `Score float64` | the user's score, 0 when ungraded |
| `Graded bool` | |
| `Stats *ScoreStats` | **null** unless the assignment is graded *and* Canvas discloses `score_statistics` |

`Grade` gains `Mean float64` — the cached class mean, 0 when that assignment's
detail has never been fetched. It is filled opportunistically, so the Grades
view should render the mean only when non-zero.

### Method
```
GetDeadlineDetail(id int) (Deadline, error)   // id = Deadline.ID (Canvas assignment id)
```
Lazy: hits `GET /api/v1/courses/:id/assignments/:id?include[]=score_statistics&include[]=submission`
and caches the result in SQLite for **6 hours**. When Canvas is unreachable and
nothing is cached, it returns the plain deadline row with empty detail fields
rather than an error — render the panel with what it has.

---

## 4. Tray: next 3 deadlines

Backend-only. The tray menu shows up to three disabled rows above
Open / Sync / Quit, formatted `CS4246 · Assignment 1 · in 2d`, and the tray
tooltip is `Next: <that same line>`. Refreshed after every sync and every 15
minutes. Clicking a row shows the window.

---

## 5. Global hotkey

`Settings.Hotkey` (default `"ctrl+shift+n"`, empty string disables) toggles the
window from anywhere. Implemented with Win32 `RegisterHotKey` plus a
`GetMessage` loop on a `runtime.LockOSThread`-pinned goroutine
(`hotkey_windows.go`; other platforms get a no-op stub). The old combination is
unregistered whenever `SaveSettings` changes the value.

Accepted syntax, case- and space-insensitive: modifiers `ctrl`/`control`,
`alt`/`option`, `shift`, `win`/`super`/`meta`/`cmd`, joined by `+`, then exactly
one key — a letter, a digit, `f1`–`f12`, or one of `space enter return tab esc
escape backspace delete insert home end pageup pagedown left up right down`.
Registration can still fail if another app already owns the combination; that is
logged, not surfaced, so the Settings UI should treat the field as best-effort.

### Window methods (also bound)
```
ShowWindow()      // also called by the tray and by a toast click
HideWindow()
ToggleWindow()    // what the hotkey fires
```

---

## Settings additions

```go
NotifyDesktop bool   // default true
Hotkey        string // default "ctrl+shift+n"; "" disables
```
Both round-trip through `GetSettings`/`SaveSettings` like every other field.
