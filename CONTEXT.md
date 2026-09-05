# NUSSync — decision log

Last updated: 2026-09-05

## Direction
Desktop app for NUS Canvas (canvas.nus.edu.sg): auto-sync all course files
(Files tab + Modules), fast local browser/search UI, Telegram reminders for
unsubmitted assignments/quizzes, plus announcements/grades notifications.

## Decisions
- **Stack: Go + Wails v2**, frontend vanilla/Svelte, SQLite via
  `modernc.org/sqlite` (pure Go, no cgo). Chosen because: single small exe,
  fast startup, no gcc/MSVC needed on Windows. Machine had no Go/Rust/gcc;
  Go installed via winget 2026-09-05. Electron rejected (fat, slow start).
  Tauri rejected (needs Rust + MSVC toolchain install).
- Auth: Canvas personal access token in `.env` (`CANVAS_URL`, `CANVAS_TOKEN`).
  Never print token. `.env` gitignored.
- Sync sources: BOTH `/courses/:id/modules?include[]=items` (File items ->
  `/files/:id`) AND `/courses/:id/folders` tree. Verified 2026-09-05: two
  courses (76056, 40631) return 403 on `/files` but 200 on `/folders`; two
  others have zero modules. Per-course 403 must be non-fatal.
- Reminder ladder: 3d/2d/1d/3h/1h before due; record sent rungs per
  assignment in SQLite to avoid spam.
- **Frontend never imports `frontend/wailsjs/`.** Everything goes through
  `frontend/src/lib/api.ts`, which calls `window.go.main.App.<Method>` when the
  Wails shell is present and otherwise falls back to `frontend/src/lib/mock.ts`.
  Reason: `wailsjs/` is regenerated on build and only has a `Greet` stub, and
  the mock lets the whole UI be built/reviewed with `npm run dev` in a plain
  browser. Events use `window.runtime.EventsOn`, else the mock's local emitter.
- Frontend state is plain `svelte/store` writables in `lib/stores.ts` (not
  runes) so the file can stay `.ts` and be imported from non-component modules;
  components themselves use Svelte 5 runes.
- **Scaffold fix:** the Wails svelte-ts template pinned `vite@^8` with
  `@sveltejs/vite-plugin-svelte@^6`, whose peer range is `vite ^6||^7` — a hard
  ERESOLVE on `npm install`. Bumped the plugin to `^7.3.0` (peers `vite ^8`).
  Do not downgrade vite instead; wails.json was left untouched. Also replaced
  the template's `new App()` bootstrap with Svelte 5's `mount()` and deleted the
  bundled Nunito font (system stack only, no external font fetch).

## Backend decisions (2026-09-05)
- **Contract types live in package `main`** (`types.go`), not a shared package,
  so Wails binding generation emits them under the `main` namespace exactly as
  `docs/API_CONTRACT.md` promises. `internal/store` has its own row structs and
  `convert.go` maps between them. Deliberate boilerplate — do not "simplify" it
  with type aliases to an internal package; that would move the generated TS
  models out of the `main` namespace.
- **FTS5 table is an ordinary table, not contentless.** `content=''` was tried
  first and rejected: contentless FTS5 cannot return column values, so
  `snippet()` fails. Now `files_fts(name, text)` with `rowid = files.id`;
  reindex is DELETE + INSERT in one tx (`store.MarkIndexed`).
- Search falls back to `LIKE` on name/rel_path when the FTS MATCH errors or
  returns nothing, so not-yet-indexed files are still findable.
- **Download never leaks the token.** A Canvas file `url` 302s to a signed S3
  URL; `canvas.Download` strips `Authorization` on any cross-host redirect,
  streams to a `.part` temp file and renames (removing dst first — Windows
  rename fails over an existing file).
- Per-course 401/403/404 are swallowed everywhere (folders, folder files,
  modules, assignments, announcements) — confirmed necessary on this account.
- `db.SetMaxOpenConns(1)` plus a write mutex in `store`: modernc SQLite in WAL
  mode still contends badly with the 4 concurrent download workers.
- Reminder-ladder rung selection is a pure function (`notify.Ladder.Active`),
  table-tested with a fake clock: rung `d` fires when
  `next_smaller < remaining <= d`. On first run, `Superseded` rungs are recorded
  as sent *without* sending (kv key `notify_bootstrapped`) so a fresh install
  does not blast a backlog.
- Go RE2 has no backreferences — the HTML `<style>/<script>` stripper spells
  out each element instead of using `\1`.
- Course code = Canvas `course_code` with any `[section]` suffix trimmed. The
  `/` between cross-listed codes is **kept** (`CS4246/CS5446`) in the DB and UI;
  the on-disk folder is `sync.CourseFolder` = the first segment only, because
  the full form produced 50-char directories such as
  `TR3202S_TR3202T_ETP3201S_ETP3201T_ETP3206L_ETP3201I`. `sync.MigrateCourseFolders`
  renames legacy folders and rewrites `files.abs_path` at the start of every
  sync run (idempotent, best-effort) — verified 2026-09-05 to migrate two
  courses with **zero** re-downloads.
- CLI escape hatches for testing without the GUI: `--sync`, `--check`,
  `--pair`, `--notify-test`.

## Verified API facts (2026-09-05)
- Token works; user id 127122. 14 active courses, 51 total.
- Rate limit bucket 700; request cost 0.35–1.05. No pressure.
- `/users/self/todo` and `/planner/items` both work for due dates.

## Integration pass (2026-09-05, real bindings)
Drove the whole UI in a browser at `http://localhost:34115` against a live
`wails dev`. Worked untouched: Home stats/due-soon/recent/announcements, Files
tree + per-course filter, FTS content search, Deadlines (NST2030 Essay, Fri 11
Sep), Announcements expand + MarkAnnouncementRead, Grades, Settings
(GetSettings populated, TestCanvas -> "TEH WAI HONG", SaveSettings round-trips
byte-identical incl. the token), Sync now (live `sync:status`, `sync:done`,
toast), OpenFile/RevealFile. `ReminderLadder`/`SkipExts`/`Color` all non-null;
`Children: null` on leaf nodes is fine because the frontend always does
`?? []`. Wails `main` namespace matches `window.go.main.App`.

Bugs found and fixed:
- `GetTree(0)` returned `[]` (`FilesByCourse(0)` matches nothing), so the Ctrl+K
  palette had an empty file index. Now merges every course, each under a
  top-level dir node. Contract updated.
- FTS `snippet()` `<b>` markers rendered as literal text. Fixed in the frontend
  (`util.snippetHTML`: escape everything, then re-enable only `<b>` as `<mark>`)
  rather than changing the marker in Go — the snippet body is untrusted PDF text.
- Grades showed raw Canvas floats (`25.66666666666668 / 26`) -> `util.fmtScore`.
- Telegram bot rendered as `@@nuscanvassync_bot`: Go already returns the leading
  `@`. Frontend no longer prepends one; `BotName` includes `@` per contract.
- `announcements:new` was emitted from `UnnotifiedAnnouncements`, whose flag is
  the *Telegram* scheduler's. With the bot unpaired nothing is ever marked, so
  every sync toasted "26 new announcements". The engine now records
  `Result.NewAnnouncementIDs` (announcements actually inserted this run) and
  `app.go` emits only those.

Not a bug (do not chase): `ipc.js:41 Cannot read properties of null (reading
'nodes')` in the console is Wails' own dev-server connection-overlay mounting
against a `#wails-spinner` anchor our index.html doesn't have. Dev-mode only;
absent from `wails build` output. Same for `Unknown message from front end:
runtime:ready`, which only happens when a *browser* tab (not the native window)
attaches to :34115.

Untested: `ChooseSyncDir` (native modal would block the automation) and
`PairTelegram` (needs a human to /start the bot).

## App icon
`build/make_icon.py` (Pillow) renders the indigo #5B5BD6 rounded square with a
sync arc + "N" to `build/appicon.png` (256x256) and writes
`build/windows/icon.ico` directly (7 sizes). The .ico is **embedded** in
`main.go` (`appIconICO`) so `tray.go` shows the same artwork regardless of the
working directory — it used to read `build/windows/icon.ico` off disk relative
to cwd, which only worked when run from the repo.

## Status
- [x] API probe, .gitignore
- [x] Go backend: config, canvas client, sqlite store + FTS, sync engine,
      text extraction, `app.go` implementing the full contract, systray, CLI.
- [x] Telegram client + reminder/announcement/grade scheduler.
- [x] Wails UI — all 6 views + command palette + toasts done 2026-09-05.
      `npm run build` and `npm run check` both clean; verified against the mock
      backend in a browser (light + dark). Untested against real Go bindings.
- [x] Build exe — `wails build` clean, `build/bin/nussync.exe` (16.9 MB),
      launches and stays up. `go test ./...` and `npm run check` (123 files,
      0 errors) both clean.

## Verified backend runs (2026-09-05)
- `go build ./...`, `go vet ./...`, `go test ./...` all clean.
- `go run . --check`: 5 deadlines, incl. NST2030 Essay due Fri 11 Sep 23:59.
- `go run . --sync`: 100 files / 467 MB across 14 courses in 1m33s, **zero**
  non-fatal errors, 0 skipped by filters (no oversized videos this term).
  A second run downloaded 0 files, so change detection works.
  100/100 files indexed, 94 with extracted text; FTS search returns sensible
  bm25-ranked hits with snippets.
- Telegram bot token valid (`getMe` -> `@nuscanvassync_bot`) but **not paired**:
  nobody has sent `/start`, so `getUpdates` is empty and `--notify-test` exits 1
  by design. Send `/start` to the bot then run `nussync --pair`.

## Dead ends
- Sanitizing the whole cross-listed course code into one folder name.
- FTS5 contentless tables (`content=''`) — `snippet()` cannot work on them.
- `regexp.MustCompile` with a backreference (`</\1>`) — panics at init in RE2.

## Operational
- Toolchain: Go 1.27 (`C:\Program Files\Go\bin`), wails CLI in `~/go/bin`,
  Node 22. Prepend both to PATH in Git Bash. `wails doctor` green.
- Telegram bot: @nuscanvassync_bot, token in `.env` as TELEGRAM_TOKEN.
- Work split: orchestrator (main session) + Opus subagents per component;
  contract in `docs/API_CONTRACT.md` is the source of truth for bindings.

## Queued (user requests 2026-09-05 night)
- Files linked from module Pages (e.g. CS4246 has 0 File items, 5 Page items;
  sample: /courses/97040/pages/week-1-overview). Fetch page body via
  /api/v1/courses/:id/pages/:slug, regex `/files/(\d+)`, download via
  /api/v1/files/:id. Also crawl /courses/:id/pages and assignment/announcement
  bodies for attachments.
- Canvas pet in sidebar: mood from deadlines/sync, click quips, level from
  synced files + on-time submissions, nameable, can be disabled.
- Dark theme: already implemented (Settings > App > Theme); verify in real app.
