# NUSSync — decision log

Last updated: 2026-09-06

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

## Frontend: sidebar pet (2026-09-05)
- **Pet is 100% frontend.** `lib/pet.ts` derives a mood (happy/worried/panic/
  busy/sleepy/proud) from the existing `deadlines` + `syncStatus` stores plus an
  idle timer; `components/Pet.svelte` is inline SVG animated with CSS only (no
  assets, no library). Deliberately no Go changes — progression lives in
  localStorage (`nussync.pet` = xp/level, `nussync.pet.prefs` = enabled/name),
  so the Settings contract stays exactly as `docs/API_CONTRACT.md` defines it.
- XP: +1 per new file (Stats.Files diff), +25 per deadline seen flipping to
  submitted, +5 for the first open of each day. The "submitted" set and the file
  count are *seeded without awarding* on first observation, otherwise a fresh
  install would instantly pay out for pre-existing state.
- Accessories unlock at Lv 3/5/8 (bow/glasses/crown); size caps at +16% (Lv 5).

## Arcade (2026-09-05, frontend-only)
- `lib/games/` = shared engine (`engine.ts`: fixed-timestep rAF loop that stops
  on unmount and auto-pauses on `visibilitychange`/blur, window-level `Keys`
  attached only while a game runs, DPR canvas fit, mulberry32 PRNG, CSS-token
  palette reader) + `sprites.ts` (pet/file/check drawn on canvas) + `scores.ts`
  (localStorage `nussync.arcade.run` / `.merge`) + the two games. View is
  `views/Arcade.svelte`, route `arcade`, sidebar item "Play", palette commands
  "Play: Nibble Run"/"Play: Lecture Merge", easter egg = 5 pet clicks in 2s
  (`pokePet()` in pet.ts). No Go changes, no assets, ~15 KB minified.
- Nibble Run hurdles are the *real* unsubmitted deadlines (height by urgency,
  overdue = wide + red); green gates must be ducked. Daily seed = local
  YYYY-MM-DD so the order is stable all day; free play is random.
- XP goes through the new `addXP(n, reason)` in pet.ts: +1 per collected file
  capped at 30/run, +50 once a day for reaching Final in Lecture Merge.
- Key events are normalised via `keyCode(e)` (`e.code`, falling back to
  `e.key`) — synthetic/automated events arrive with an empty `code`.
- Note: rAF is suspended while the dev Browser pane is hidden, so games look
  frozen there; drive them from a real window when eyeballing feel.

## Theme (2026-09-05)
- `initTheme()` is now idempotent and is called from `main.ts` **before**
  `mount()` so `data-theme` is on `<html>` pre-paint (no white flash in dark).
- `loadSettings` no longer overwrites the theme when the machine already has an
  explicit `nussync.theme` in localStorage (`hasLocalTheme`, captured before the
  store subscription writes the key back). Previously the backend default
  (`system`) stomped whatever the user had just picked, on every load.
- The Settings theme `<select>` binds to the `theme` store, not `draft.Theme`,
  so it always shows what the window is actually rendering.

## In-app file viewer (2026-09-06)
- Clicking a file previews it **inside the WebView** — no external process. Bytes
  come from `assets.go`, registered as `assetserver.Options.Middleware` (NOT
  `Handler`). This is the load-bearing detail: `Handler` only runs when the asset
  lookup MISSES, and under `wails dev` the lookup proxies to Vite, whose SPA
  fallback answers every unknown path with `index.html` 200 — so a `Handler`
  never sees `/local/...` in dev. Verified: with `Handler` every probe returned
  411 bytes of `text/html`; with `Middleware` it returns the real file in both
  dev and the built exe.
- Routes: `GET /local/{fileID}` (path looked up via `store.FileByID`) and
  `GET /local/path?p=<abs>`, the latter 403 unless the path resolves inside
  SyncDir (`filepath.Rel`, symlinks resolved, lower-cased on Windows — a naive
  prefix test would let `…/NUSSyncOther` through). `http.ServeContent` gives
  Content-Length + Range; we set Content-Type, `Accept-Ranges`,
  `Cache-Control: no-store`, `nosniff`.
- **File ids can be negative.** Downloaded papers live in the synthetic Papers
  course and are numbered down from -1000, so the id route rejects only `0`.
  Cost one debugging round; do not "tighten" it back to `id <= 0`.
- Everything textual — including `.html`/`.htm` — is served as
  `text/plain; charset=utf-8` and rendered by us, so course content never runs
  as markup in the app's own origin. PDFs go into a **plain `<iframe>` with no
  `sandbox` attribute**: sandboxing disables the Chromium PDF plugin and yields
  a blank pane.
- New bindings `GetFileInfo(fileID)` and `GetFileText(fileID, maxChars)` (FTS
  text via `store.StudyFileText`, falling back to a live `index.Extract`) back
  the docx/pptx/xlsx "Text preview" mode.
- `components/Viewer.svelte` is the single preview component, reused by Files
  (third pane, drag-resizable, width in localStorage `nussync.viewer.w`, Ctrl+P
  toggles, Esc closes, ←/→ move the selection), Papers (library-card "Preview"),
  Study (overview tab) and What's-new (right-click -> Preview overlay). It sets
  `currentContext` to whatever it is showing, so the chat panel follows the
  preview. Single click previews; double click still opens externally.
- Verified 2026-09-06 against the running app: `/local/<id>` 200
  `application/pdf` + 206 on Range, negative-id paper PDF 200, png/csv 200,
  unknown id 404, malformed id 400, `C:\Windows\win.ini` 403; and in the
  WebView the iframe's `contentDocument.contentType` is `application/pdf` with
  `readyState: complete`, i.e. the built-in viewer really rendered it.

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
- After the feature batch: `go build ./...`, `go vet ./...`, `go test ./...`
  clean; `--check` still lists 5 deadlines (NST2030 Essay Fri 11 Sep) and
  `--sync` ran all 14 courses in 48s, 0 downloads, 0 errors, library unchanged
  at 123 files / 482.4 MB — so the two additive columns cost no re-downloads.
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

## Pages source (2026-09-05, third sync source)
- Some courses publish every lecture note as a **link inside a module Page**,
  never as a File item, with an empty Files tab (CS4246/CS5446, course 97040,
  is the motivating case). `Source = "pages"` now covers those.
- Crawled bodies: module items of type `Page` (`page_url`), standalone wiki
  pages from `/courses/:id/pages` not referenced by any module, assignment
  `description`, announcement `message`. Quizzes/discussion bodies were
  deliberately left out — scope.
- Extractor `sync.FileIDsInHTML` (`internal/sync/links.go`) is a single
  `/files/([0-9]+)` regex over the body. That one pattern covers `?wrap=1`
  hrefs, `/download?download_frd=1`, `data-api-endpoint`, `<img src=.../preview>`
  and bare `/files/N`; the digit run is greedy so nothing matches truncated.
  Bodies are pre-normalised for JSON `\/` escapes and `&amp;`. Table-tested in
  `internal/sync/links_test.go`.
- A link may point at **another course's** file. Those are kept, resolved via
  `/api/v1/files/:id`, and 401/403/404 is swallowed per id. `/courses/:id/pages`
  itself 403/404s when the Pages tab is disabled — non-fatal.
- Priority when one file id appears in several sources: **files-tab > modules >
  pages**, and a row already stored under a different course with an
  authoritative source is never demoted to "pages" (`Engine.claimed` also stops
  two courses fighting over the same id within a run).
- Layout: `Modules/<module>/<page>/...` for module pages, `Pages/<page>/...`,
  `Assignments/<name>/...`, `Announcements/<title>/...` (`sync.RelPathForPage`).
  `FileNode.Module` carries the same label ("<Module> / <Page>", "Pages / <P>",
  "Assignments / <name>", "Announcements / <title>"); contract updated.
- Caching: new `pages(course_id, url, title, updated_at, file_ids)` table. The
  list endpoint returns `updated_at` without the body, so an unchanged page
  skips its body fetch entirely. New `files.origin` column (additive
  `ALTER TABLE` via `store.addColumn`, safe on existing DBs) records
  `page:<slug>` / `assignment:<id>` / `announcement:<id>`.
- `Assignments`/`Announcements` are memoised per run (`Engine.assnCache` /
  `annCache`, cleared by `resetCaches`) because both the crawl and the metadata
  refresh need them.
- First run: **23 new files, 15.3 MB**. CS4246/CS5446 15 (Course Overview,
  Lecture-Intro, Lecture-classic v4.0 = "Classical (Symbolic) Planning",
  Lecture-complex, Lecture-rational-utility, Lecture-mdp, Lecture-sdm, 3
  tutorials, Assignment-1 + programming package, project guidelines), TPC 4
  (FAQ screenshots), NUSC1101 2, NOC 1, TR3201N 1 — the last two from
  announcement images. Library 100 -> 123 files. Second run downloaded 0 and
  ran ~17s faster thanks to the page cache.

## Feature batch (2026-09-05 night, backend)
Contract for the new bindings: **`docs/API_CONTRACT.md` stays the base contract;
`docs/CONTRACT_FEATURES.md` is the source of truth for everything below.**

- **What's-new feed.** Additive `files.first_seen_at` / `files.last_changed_at`
  (RFC3339, `store.addColumn`). `first_seen_at` is **write-once in SQL** (an
  `ON CONFLICT ... CASE WHEN files.first_seen_at=''` clause) and
  `last_changed_at` only moves when the caller supplies one — so the engine's
  "unchanged, just re-upsert" path passes `""` and preserves both stamps
  without needing a second query. Rows predating the feature keep empty stamps
  and are deliberately never "new": verified by a `--sync` that downloaded 0
  files and left all 123 rows alone. `feed_seen_at` kv drives `GetUnseenCount`
  and `FileNode.IsNew`. Event `feed:updated` carries the count.
- **Windows toast** via `git.sr.ht/~jackmordaunt/go-toast/v2` (already an
  indirect Wails dep; builds on every OS because its non-Windows backend is a
  no-op, so `notify_toast.go` needs no build tag). `toast.SetActivationCallback`
  gives a real in-process click handler → `App.ShowWindow`, so no protocol
  handler was needed. The icon must be an absolute path on disk (a toast renders
  from a temp dir), so the embedded PNG is written once to
  `%APPDATA%/NUSSync/appicon.png`. Setting `NotifyDesktop`, default true.
- **Telegram two-way commands** in `internal/notify/bot.go` (loop) +
  `commands.go` (pure formatters, table-tested). **Telegram hands each update to
  exactly one `getUpdates` caller** — two pollers silently steal each other's
  messages — so `notify.Bot` is the single consumer and `App.PairTelegram` now
  waits on `Bot.AwaitPair` instead of long-polling itself. Headless CLI has no
  bot, so `--pair` / `--notify-test` still use the direct
  `AwaitStart`/`FindExistingChat` path; those were refactored onto a new
  exported `telegram.Client.GetUpdates`. Command handling runs on its own
  goroutine because `/sync` blocks for minutes.
- **Assignment detail.** `GetDeadlineDetail(id)` fetches
  `assignments/:id?include[]=score_statistics&include[]=submission` and caches it
  in a new `assignment_cache` table for 6h; `grades` LEFT JOINs that table for
  `Grade.Mean`. `Stats` is only set for graded assignments — Canvas returns
  `score_statistics` without a `count` on this build, so `ScoreStats.Count` is
  usually 0 and means "unknown". Attachments reuse `sync.FileIDsInHTML` over the
  description; unsynced ids are dropped. **Description is returned raw** — the
  frontend sanitizes.
- **Tray** shows up to 3 disabled `CODE · Title · in 2d` rows. systray cannot
  reorder a menu, so the three slots plus a separator are created up front and
  `Hide()`n; `App.trayRefresh` is the hook the sync loop and a 15-min ticker call.
- **Global hotkey** `hotkey.go` (portable parser + tests) / `hotkey_windows.go`
  (`RegisterHotKey` with `MOD_NOREPEAT` on a `runtime.LockOSThread` goroutine
  plus a `GetMessageW` loop). Passing hwnd=0 binds the hotkey to the *calling
  thread*, which is why the thread must be pinned; teardown posts `WM_QUIT` to
  the recorded thread id. Setting `Hotkey`, default `"ctrl+shift+n"`, `""`
  disables. Wails v2 has no "is window visible", so `App.windowVisible` tracks
  it.

## Study/AI backend (2026-09-05)
Contract: `docs/CONTRACT_STUDY.md`. Code: `internal/study/**`, `app_study.go`,
`types_study.go`, `cli_study.go`, `internal/store/study.go` (tables `study_*`,
created lazily by `MigrateStudy`, **not** from `Store.migrate`).

- **Claude Code CLI flags that work** (v2.1.251):
  `claude -p --output-format stream-json --verbose --safe-mode
  --strict-mcp-config --model <opus|sonnet|haiku> --max-turns N
  --permission-mode dontAsk --tools Read --allowedTools Read --add-dir <dir>`.
  `stream-json` **requires** `--verbose` or the CLI refuses to start.
  `--permission-mode dontAsk` auto-denies instead of prompting.
- **`--safe-mode` is mandatory, not optional.** Without it the child inherits
  the user's SessionStart hooks — this machine has one that injects a
  "CAVEMAN MODE ACTIVE" output style, which would rewrite every overview and
  break every JSON reply. Safe mode also drops CLAUDE.md, skills, plugins and
  custom agents, so runs are deterministic and start faster. `--bare` is *not*
  a substitute: it forces `ANTHROPIC_API_KEY` auth, which this user has not got.
- **The prompt goes on stdin, never in argv.** `claude` on PATH is the npm
  `claude.cmd` shim; Go runs `.cmd` through `cmd.exe`, which mangles a
  multi-line quoted argument and the process dies with exit 1 and *no output at
  all*. `claude -p` with the prompt piped in works. We also prefer the real
  `%APPDATA%/npm/node_modules/@anthropic-ai/claude-code/bin/claude.exe` over
  the shim. Stdin is closed after the prompt so the child never blocks.
- **Nested-session env vars break auth.** Launching NUSSync from inside a
  Claude Code session leaks `CLAUDECODE`, `CLAUDE_CODE_SDK_HAS_*_AUTH_REFRESH`,
  `CLAUDE_CODE_MESSAGING_*` etc.; the child then defers OAuth refresh to a host
  that is not listening. `study.childEnv()` strips them.
- **`claude auth status` prints JSON** (`{"loggedIn":…,"authMethod":…}`), is
  free and instant — used as the primary login check before falling back to a
  billable one-turn probe. Result cached 10 min.
- **Exit code is 0 even on failure.** Always read `is_error` in the
  `type:"result"` line; `total_cost_usd`, `duration_ms`, `num_turns` and
  `usage` are on that same line.
- Stream lines seen: `system/init`, `system/hook_*` (absent under safe mode),
  `assistant` (text + tool_use), `user` (tool results), `result`. Tool-result
  lines can be megabytes, so stdout is read with an unbounded line reader, not
  `bufio.Scanner`. Progress is scraped from `assistant` lines into
  `study:progress`.
- Cancel = `taskkill /T /F /PID`: killing only the direct child orphans node.
- **Not verified end to end against the live model.** On 2026-09-05 the
  on-disk credentials (`~/.claude/.credentials.json`) had `expiresAt: 0` and
  every child run returned `"Failed to authenticate: OAuth session expired and
  could not be refreshed"` — the Claude Code *host app* holds the live tokens
  in memory and the CLI cannot refresh on its own. Fix on the user's side:
  run `claude auth login` (or `claude setup-token` for a long-lived token) in
  a normal terminal, then re-run `--study-test`. Everything else was verified:
  process spawn, stream parsing, `is_error` surfacing, page count (17 for the
  CS4246 Course Overview PDF), job queue, events, and — through a stub CLI
  pointed at by the `NUSSYNC_CLAUDE_BIN` override — fenced-JSON extraction,
  SQLite round-trip and exact MCQ grading (3/3).
- SM-2 lite: grade 0-3. Hard multiplies by 0.6, good by 1.0, easy by 1.3 on top
  of the ease factor; ramp 1 day then 6 days; ease floor 1.3; interval capped
  at 365. Hard **must** shrink relative to good — an earlier 1.2 modifier made
  "hard" schedule further out than "good".
- `NUSSYNC_CLAUDE_BIN` overrides binary discovery (testing only).

## Frontend: feature + study batch (2026-09-06)

Built by a frontend subagent under `frontend/` only; `App.svelte`, `Sidebar`,
`CommandPalette`, `pet.ts`, `Pet.svelte`, `games/**` and `Arcade.svelte` were
owned by another session at the time, so **the nav items, routes, palette
commands and the Arcade card for Quiz Rush still need wiring** by whoever owns
those files. Everything else is done.

New views (self-contained, `src/lib/views/`):
- `WhatsNew.svelte` — feed grouped day then course, new/updated chip, click
  opens, right-click reveals, "Mark all seen", 3/7/14/30-day window.
- `Study.svelte` — on-demand only. Left picker (PDFs first, page count fetched
  lazily on select, multi-select), right tabs Overview/Quiz/Ask/Flashcards,
  each showing cached results first. Jobs tracked via `study:job` /
  `study:progress` with a progress line, elapsed seconds and Cancel.
- `QuizRush.svelte` — arcade run over `GetQuizzes(0)`, 3 lives, timer 12s->5s
  as the streak grows, x1..x5 multiplier, short-answer boss every 10th, redo
  pile at the end, best per course in `nussync.arcade.rush`.
- `_Preview.svelte` — dev-only harness. `main.ts` mounts it INSTEAD of App only
  when `location.search` contains `preview=`; keep that guard. Use
  `npm run dev` then `/?preview=1&view=study`.

Edited: `Files.svelte` (New chip on `FileNode.IsNew`), `Home.svelte` ("new
since last visit" line), `Deadlines.svelte` (right-side detail panel:
sanitized description, submission types, score, class range bar with your
score marked, attachments, Open in Canvas), `Grades.svelte` (class mean +
"vs class" delta column), `Settings.svelte` (NotifyDesktop toggle, Hotkey
field, Telegram command help card, Study status card + model select).

Decisions:
- `unseenCount` lives in `stores.ts`, refreshed on `feed:updated` and after
  every sync — the nav badge reads that store.
- `navigate()` is also reachable from decoupled components by dispatching
  `new CustomEvent('nussync:navigate', {detail:{view:'study'}})` on window;
  `wireEvents()` listens. Quiz Rush's empty state uses it.
- Markdown from the model is rendered by `util.markdownToHTML` — escape-first,
  then re-enable only headings/bold/italics/code/lists/quotes. No library, and
  no path by which model output can inject HTML.
- The study model preference is frontend-only, `localStorage`
  `nussync.study.model`, read by both Settings and Study.
- Quiz Rush calls the pet through `import * as pet` +
  `(pet as any).addXP?.(n, 'rush')` so it never breaks if pet.ts changes.
- `mock.ts` now fakes the whole study backend: a ~5s job with real progress
  lines and cancel, a pre-seeded bank (3 quizzes / 15 questions), one cached
  overview, 6 flashcards, deadline detail with score statistics, and a feed
  whose `feed_seen_at` sits 2.5 days back so `IsNew` is non-trivial.

`npm run check` 0 errors, `npm run build` clean, console clean in the harness.

## Papers backend (2026-09-06)
Contract: `docs/CONTRACT_PAPERS.md`. Code: `internal/papers/**`, `app_papers.go`
(`papersInit()` via `sync.Once`, called from `startup` right after
`studyInit()`), `types_papers.go`, `internal/store/papers.go`
(`MigratePapers`), `internal/notify/paperbot.go`, `internal/study/chat.go`,
`app_chat.go`, `cli_papers.go`.

- **arXiv**: `https://export.arxiv.org/api/query` (the `http://` form answers
  301). Atom parsed with `encoding/xml`; Go matches namespaced elements
  (`opensearch:totalResults`, `arxiv:primary_category`, `arxiv:doi`,
  `arxiv:journal_ref`) by local name, so no namespace tags are needed. One
  process-wide limiter enforces >= 3 s between calls, and PDF downloads from
  arxiv.org go through it too. User-Agent `NUSSync/1.0`. Verified 2026-09-06:
  466 hits for "LLM unlearning" in 715 ms.
- **Semantic Scholar**: no key, and the public tier answered **429 on the very
  first request** during testing — assume it is usually rate limited. 429/5xx
  retries 3x with 1s/2s/4s backoff; failure is non-fatal for
  `SearchPapers(..., "all", ...)` (arXiv-only results come back) and for the
  digest (which then falls back to an arXiv keyword search, with the reason
  line saying so). Every GET body is cached 24 h in `papers_cache`.
- **Course -1 convention.** A downloaded paper needs a `files` row so Study,
  FTS and chat work on it, so `store.EnsurePapersCourse()` creates one synthetic
  course `id=-1, code="Papers", name="Research papers", enabled=0`, and each PDF
  gets a negative `files.id` allocated from -1000 downwards
  (`store.NextPaperFileID`). Canvas ids are always positive, so nothing
  collides. The sync engine only iterates courses Canvas listed, so -1 is never
  synced or folder-migrated; `App.GetCourses` additionally hides it while its
  file count is 0. Files land in `<SyncDir>/Papers/<year> - <surname> -
  <title>.pdf` and are text-extracted into `files_fts` immediately.
- **Chat resume**: `claude -p ... -r/--resume <session id>` IS accepted by CLI
  2.1.251 (`claude -p --help` lists `-r, --resume [value]`), so multi-turn is
  the stored `session_id` from the first result line. `study.RunChat` duplicates
  the ~50 lines of process plumbing from `Run` rather than editing `claude.go`,
  because it needs a per-assistant-block text callback. **The CLI streams whole
  assistant messages, not token deltas**, so one `chat:delta` event = one
  assistant text block. If a resume fails (session gone from disk), the turn is
  retried once from scratch with the transcript replayed in the prompt
  (`study.ReplayPrompt`).
- Summary and chat reuse the Study single-slot job queue (kinds
  `paper_summary`, `chat`), so they never run two `claude` processes at once.
  `StartPaperSummary` downloads the PDF inside the job when LocalPath is empty.
- **Paper Telegram bot is a SECOND bot**: `@paper_trackerrr_bot` (token in
  `.env` as `PAPER_TRACKER_TELEGRAM_TOKEN`, imported into
  `Settings.PaperTelegramToken` on first run / whenever empty). It has its own
  poll loop (`notify.PaperBot`), its own chat id, its own pairing
  (`PairPaperTelegram`) and its own once-a-minute digest ticker at
  `PaperDigestHour` (kv `papers_last_digest`, stamped *before* sending so a
  failure does not retry every minute). Commands `/paper`, `/save <n>`,
  `/reading`, `/help`, `/start`; all paper logic reaches notify through hook
  funcs so the package stays a transport. **Not paired** as of 2026-09-06 —
  nobody has sent /start to it.
- Google Scholar: `OpenScholar` builds a search URL and hands it to the shell.
  Never fetched, never parsed.

### Verified runs (2026-09-06)
- `go run . --papers-test "LLM unlearning"`: arXiv 5/5 with authors, years and
  PDF links; S2 429 (non-fatal); digest of 8 papers, Telegram body 3636 bytes
  (< 4000); downloaded 2506.13181 into `<SyncDir>\Papers\` as
  `2025 - Spohn - Align-then-Unlearn_ ....pdf`, files row `course -1 (Papers)`,
  462.5 KB, synced+indexed, 8 pages; BibTeX `@misc{spohn2025alignthenunlearn}`
  with eprint/archivePrefix/primaryClass.
- `--claude-test`: both the summary job and the chat job finish with
  `error - Failed to authenticate: OAuth session expired and could not be
  refreshed`, i.e. the known CLI auth problem surfaces cleanly in `Job.Error`
  and the user message is still stored. Blocked on `claude auth login`.
- `--digest-send`: refused, "the paper bot @paper_trackerrr_bot is NOT paired".
- `go build ./... && go vet ./... && go test ./... && gofmt -l .` all clean.

## Study: readable PDF handling (2026-09-06)
Code: `internal/study/readable.go` (+ `readable_test.go`), wired into
`app_study.go` (`studySources`), `app_chat.go` (`runChatTurn`),
`app_papers.go` (`StartPaperSummary`) and the `--study-test` banner.

- **The "password-protected" error is a lie.** Claude Code's Read tool refuses
  most NUS lecture PDFs with *"PDF is password-protected. Please provide an
  unprotected version."* The files are **not** encrypted: the CS4246 Course
  Overview deck has no `/Encrypt` anywhere in it and `api.DecryptFile` answers
  "this file is not encrypted". Measured against CLI 2.1.251 on 2026-09-06,
  two separate things make Read fire that message:
  1. **Size.** A PDF over roughly 128 KiB is always refused. Accepted at
     104.5 KB, refused at 141.7 KB, and it holds however the file was produced
     — a pdfcpu rewrite of the same pages fails identically once it is big
     enough. `ReadableLimit` is set to 120 KiB, the conservative side.
  2. **Structure.** Some *small* files are refused too (a 38 KB PDF 1.3 essay
     prompt, an 84 KB table). Rewriting exactly the same pages through pdfcpu
     makes both readable, so the parser is choking on assembly, not content.
     Not xref streams — flattening those changed nothing.
- Consequence: **do not gate the fix on encryption detection.** Every PDF is
  normalised through pdfcpu into `%LOCALAPPDATA%/NUSSync/study-cache/`
  (`<fileid>-<sha of path|size|mtime>.pdf`, so an edited file re-caches). That
  one step repairs the structural class *and* decrypts a genuinely
  owner-password-protected file for free (`api.DecryptFile` with empty
  passwords, tried first when the trailer holds `/Encrypt`).
- When the normalised copy is still over `ReadableLimit`, `study.Resolve`
  writes the FTS-extracted text to `<cache>/<fileid>.txt` and points Claude at
  that, with `Source.Note` telling it this is a text-only extract and figures
  are unavailable. The 662 KB CS4246 deck takes this path.
- `pdfcpu` v0.15.0, pure Go. `api.DisableConfigDir()` once (a `sync.Once`) so
  concurrent jobs never race on its config directory; `ValidationRelaxed`; the
  call is wrapped in `recover` because pdfcpu panics on some malformed files.
  Writes go to a temp file and are renamed.
- `Resolve` is total: non-PDFs pass through, unopenable formats (pptx/docx)
  keep `Readable=false` and the existing inline-text path, and when there is no
  text either it hands back the original so Claude reports the real failure
  rather than the job silently losing its only source.
- **Known quality gap, not fixed.** For the big deck the model itself reported
  the extract is partly garbled (grading weightage and registration slides), so
  those sections of the overview are vague. The better fix is to split an
  over-limit PDF into page-range chunks under `ReadableLimit` and hand Claude
  several real PDFs — but single pages of that deck run 61–184 KB, so the
  heaviest pages would still need the text fallback. Deferred.
- **Config reload for the bots.** The CLI (`--pair`, `--notify-test`) writes
  `config.json` directly, typically while the GUI is running, and nothing in
  the GUI re-read it — a chat id paired from a terminal stayed invisible until
  restart. `App.watchConfig` (app.go) now stats `config.json` every 60 s and,
  when the four Telegram fields differ from memory, pushes them into
  `sched.SetSettings` / `bot.SetConfig` / `papersApplySettings`. **The poll
  loops do not need restarting**: `Bot.loop` and `PaperBot.loop` both call
  `snapshot()` on every iteration, so a swapped client is picked up on the next
  pass. Only the Telegram fields are copied back, so a disk write cannot
  clobber the rest of the in-memory settings.

### Verified run (2026-09-06)
`go run . --study-test "<CS4246 Course Overview 2627-1.pdf>" sonnet 5`, with
`claude auth login` finally done (the OAuth blocker in the 2026-09-05 notes is
gone — `auth status` reports `loggedIn:true`, `claude.ai`, max):
- Overview: **done in 1m18.089s, $0.0744**, all six sections present.
- Quiz: **done in 1m30.926s, $0.0871**, 5 questions (4 mcq + 1 short) with
  pages and explanations; perfect attempt graded **5/5**.
- `go build ./... && go vet ./... && go test ./... && gofmt -l .` all clean.


## Quest: progression + boss ladder (2026-09-06, frontend-only)

`lib/quest.ts` (persisted at `localStorage['nussync.quest']`) layers a Duolingo-ish
daily goal on top of the existing pet XP. Nothing in Go changed; the ladder runs
entirely on `GetQuizzes(0)` + `GetTree` + the `deadlines` store.

- **One XP currency.** `pet.ts` stays the only place XP is awarded. It gained
  `onXP(fn)` and `registerQuipSource(fn)`; quest.ts registers against both so the
  import stays one-way (quest -> pet) and cycle-free. Never make pet.ts import
  quest.ts.
- **Streak roll-over** settles only *fully elapsed* days (`rollover()`); today is
  added live by the `streak` derived store, so the number cannot double-count.
  A missed day burns a freeze (1 per 7-day streak, max 2) before breaking.
- **Stats** are derived counters, not stored numbers: `1 + floor(sqrt(n))` over
  correct answers / flashcards / (overviews+previews) / best rush streak. Gear
  unlocks at stat thresholds and is drawn by `PetSprite` itself (it reads
  `gear`/`skinAccent` from quest.ts), so every call site gets it for free.
- **Boss HP** = `max(3, ATK * questions * 0.7)`; boss ATK = `1 + tier`. An early
  `max(8, ...)` floor made tier 1 unwinnable at ATK 1 — don't reintroduce it.
  Boss-fight hits call `recordBossHit()` (counter only, no XP) because the fight's
  XP is paid once at win/lose; awarding per question would double-pay.
- **Papers done +20** is scanned from `GetLibrary('done')` on the `papers:updated`
  event rather than hooked into Papers.svelte (owned by another workstream).
- Boss art is procedural SVG from the tier number (`components/BossSprite.svelte`),
  no assets. Deadline bosses use `(deadline.ID % 10) + 1` as their seed.
- Route `quest`, nav item after Play, palette "Go to Quest", Arcade card.

## Dead ends
- Sanitizing the whole cross-listed course code into one folder name.
- Passing a multi-line `claude` prompt as an argv element on Windows (see above).
- `claude --bare` for the Study backend: it disables OAuth and demands an API key.
- Anthropic Go SDK for Study: needs an API key the user does not have.
- Treating the Read tool's "PDF is password-protected" as meaning encrypted.
  It is a generic failure message; see "Study: readable PDF handling".
- Flattening cross-reference streams / object streams to PDF 1.4 layout to
  make a big PDF readable. Makes no difference; only size does.
- FTS5 contentless tables (`content=''`) — `snippet()` cannot work on them.
- `regexp.MustCompile` with a backreference (`</\1>`) — panics at init in RE2.
- Guarding `FileIDsInHTML` with `strings.Contains(body, "/files/")` *before*
  unescaping `\/` — JSON-escaped bodies then extract nothing. Normalise first.
- Running the Telegram pairing long poll alongside the command loop. Telegram
  gives each update to one `getUpdates` caller only; both consumers then lose
  messages at random. One loop, and pairing waits on it.
- Back-filling `first_seen_at` for the existing library so the feed has content
  on day one — every synced file would show as "new" forever after.
- `http://export.arxiv.org/api/query` — 301s; use https directly.
- Relying on Semantic Scholar for anything load-bearing: the keyless tier 429s
  constantly. It is enrichment only.
- Scraping Google Scholar (no API, against its terms) — button only.

## Incident: config.json lost its Canvas token (2026-09-06)

**Symptom.** `%APPDATA%/NUSSync/config.json` was found with `CanvasToken:""`
(Telegram tokens blank too) and only the *original 13* Settings keys — no
`Hotkey`, no `Paper*`. The app then 401'd on every Canvas call.

**Root cause.** A stale build was still running. The file is 2-space
`json.MarshalIndent` in struct-declaration order, so a Go build wrote it — but
a build whose `config.Settings` was older than the one on disk. Fingerprint,
verified by string-grepping the binaries: the current `config.json` is missing
exactly `PaperTopVenues` and `PaperPreferPublished`, and those two symbols are
present in `build/bin/nussync.exe` but **absent from `build/bin/nussync-dev.exe`**
(the `wails dev` build of an earlier revision). The incident file was the same
failure one schema generation further back. Two instances were running at once,
so the older one's `SaveSettings` — which at the time did a bare
`config.Save(fromSettings(s))`, no merge, no guard — serialised *its* struct
over the newer file, dropping the fields it did not know about and persisting
the empty `CanvasToken` its Settings draft happened to hold (the draft is
seeded before `GetSettings` resolves, so a Save that early sends blanks).
Nothing was corrupt; a whole-struct overwrite from a stale process is enough.

**Fixes (all in tree, uncommitted at time of writing).**
- `main.go`: single-instance lock (`sg.nus.nussync.single-instance`) — the
  second launch raises the existing window instead of running a rival writer.
- `config.Merge` + `config.Read` + `config.ClearSecret`: an empty incoming
  secret means "the caller does not have it", never "delete it"; nil slice =
  unchanged, empty slice = cleared; `Read` parses without ever writing (unlike
  `Load`, which creates the file). `ClearSecret` is the only way to blank a
  token. Paired chat ids are treated as secrets too, so a stale draft cannot
  un-pair a bot.
- `App.SaveSettings` merges onto the freshest on-disk copy and logs which
  secrets it had to preserve (names only, never values).
- `watchConfig` uses `config.Read`, ignores a parse failure or a missing
  `CanvasURL` (half-written file), never writes, and only ever *adopts*
  non-empty values.
- Settings.svelte: Save stays disabled until `GetSettings` resolves (`loaded`),
  "Unsaved changes" is a real `deepEqual` (a stringify compare lied whenever
  Svelte's state proxy reordered keys), and the payload is the current stored
  settings with the draft laid over them. `Settings.svelte` is the only caller
  of `api.saveSettings` — verified by grep, no quick action saves settings.
- `canvas.APIError.Error()` is now one short line
  (`canvas: GET /users/self -> 401 unauthenticated (user authorisation
  required)`): first `errors[].message` only, body dropped (still on `e.Body`).
- Inline errors wrap instead of truncating: `.err-text` (pre-wrap,
  word-break, `max-height: 8.7em; overflow-y: auto`) in Settings (Canvas test
  result, both Study CLI branches) and Study.svelte (status banner, job bar —
  the job bar used to `truncate` an error, hiding the useful half).

**Do not retry.** Do not "clean up" `config.json` by hand-writing a partial
object, and do not leave a `wails dev` instance running while testing a
`wails build` exe: the config file is whole-struct overwritten by whoever
saves last, and only the merge guard now makes that survivable.

## Operational
- Toolchain: Go 1.27 (`C:\Program Files\Go\bin`), wails CLI in `~/go/bin`,
  Node 22. Prepend both to PATH in Git Bash. `wails doctor` green.
- Telegram bot: @nuscanvassync_bot, token in `.env` as TELEGRAM_TOKEN.
- Work split: orchestrator (main session) + Opus subagents per component;
  contract in `docs/API_CONTRACT.md` is the source of truth for bindings.

## Queued (user requests 2026-09-05 night)
- [x] Files linked from module Pages — done, see "Pages source" above.
- Canvas pet in sidebar: mood from deadlines/sync, click quips, level from
  synced files + on-time submissions, nameable, can be disabled.
- Dark theme: already implemented (Settings > App > Theme); verify in real app.
- [x] Extra features approved 2026-09-05 ("useful, not redundant"): What's-new
  feed + New badges + Windows toast; Telegram two-way commands (/due /new
  /files /sync); assignment detail panel + class score stats; tray next-3
  deadlines; global hotkey Ctrl+Shift+N. **Backend done** — see "Feature batch"
  above and `docs/CONTRACT_FEATURES.md`; the UI for the feed, the New badges,
  the assignment detail panel and the two new Settings toggles is still to
  build. Rejected: timetable (NUSMods), submit-from-app, favourites,
  Panopto/Zoom recordings.
- Study/AI panel (approved 2026-09-05, ON-DEMAND ONLY, never auto): Overview,
  Quiz (MCQ+short, self-test mode), Ask (Q&A with page cites), Flashcards.
  NO API key (user has none). Backend shells out to the installed Claude
  Code CLI (`claude -p ... --output-format json --allowedTools Read`,
  v2.1.251 at %APPDATA%/npm/claude) which uses the user's subscription.
  Claude Code reads the PDF itself. Model dropdown opus/sonnet. Show page
  count before run; stream/cancel; cache outputs in SQLite. Anthropic Go SDK
  rejected: needs API key.
- Arcade (2026-09-05): Nibble Run (runner, hurdles = real deadlines) and
  Lecture Merge (2048) built by frontend worker. Queued: Quiz Rush — arcade
  game over the Study quiz bank (lives, shrinking timer, streak multiplier,
  boss short-answer rounds, wrong -> flashcard pile, per-course best, XP).
  **Built 2026-09-06** as `views/QuizRush.svelte`; needs an Arcade card to
  reach it. Same for the What's-new and Study views — see the frontend batch
  section above for the wiring that is still outstanding.
- Desktop shortcut created at %USERPROFILE%\Desktop\NUSSync.lnk -> build/bin/nussync.exe.
  Taskbar pin cannot be automated on Win11; user pins manually.

## Papers + chat frontend (2026-09-06)
- Built against `docs/PAPERS_SPEC.md` and `docs/CONTRACT_PAPERS.md` while the Go
  side was written concurrently. `CONTRACT_PAPERS.md` is the source of truth and
  two things in it differ from the earlier spec sketch — the frontend follows the
  contract: **ChatSession.ID / ChatMessage.SessionID / ChatDelta.SessionID /
  ChatDone.SessionID are strings** (not ints), and **CitationLink embeds Paper**,
  so its JSON is a flattened Paper plus `InLibrary` / `Status` (no `.Paper` key).
- New files: `src/lib/views/Papers.svelte` (route `papers`, nav after Study),
  `src/lib/components/ChatPanel.svelte` (fixed right panel, 390px, Ctrl+J /
  top-bar button to toggle, Esc to close).
- New stores in `src/lib/stores.ts`: `currentContext` {fileID, paperID, name}
  (set by Files selection + context menu, Study's primary selection, Papers'
  Chat button), `chatOpen` + `openChat()`, and `studyPreselect` + `studyFile()`
  — Papers' "Study" button parks a FileID there and Study consumes it once on
  mount. Deliberately a one-shot store, not a route param: the router is a plain
  writable with no params.
- Paper summaries ride the shared Study job queue: `study:job` events with
  Kind `paper_summary`. Papers.svelte tracks the one in-flight job locally
  (jobs carry no PaperID) and reloads the cached summary on `done`.
- Mock gotcha found the hard way: seeded chat message ids overlapped the mock's
  `chatMsgSeq`, producing duplicate `{#each}` keys that crashed ChatPanel and
  made Ctrl+J silently stop working. Sequences now start above the seeds, and
  optimistic rows use a dedicated negative counter.
- Verified in `npm run dev` against the mock (8 search results, 5-paper library,
  citations/references, recommendations, digest, a ~4s summary job, streaming
  chat): search, add/download/open, status/stars/tags/notes autosave, drawer,
  BibTeX modal, Ctrl+J, Esc, Enter vs Shift+Enter, and context switching from
  Files. `npm run check` 0 errors, `npm run build` clean. Not yet run against
  the real Go backend.

## Papers: venue-priority ranking (2026-09-06)

Goal: peer-reviewed work outranks preprints in search, the digest and
recommendations.

- **`internal/papers/venue.go`** is the whole feature: `DetectVenue`,
  `AnnotateVenue(s)`, `Score`, `SortByScore`, `FilterPublished`. Tiers: 2 = a
  venue on `PaperTopVenues`, 1 = any other detected venue (and every workshop
  paper, including workshops at top venues), 0 = preprint/unknown.
- **`VenueTier`/`VenueShort`/`Published` are DERIVED, never stored.** No
  `papers_library` migration: `toPaper`/`toLibraryPaper` re-annotate on every
  read, so changing the settings re-tiers the whole library instantly. The one
  thing that must persist is the venue *name*, so when the venue was found only
  in the arXiv comment, `AnnotateVenue` writes the short name back into `Venue`
  ("arXiv cs.LG" -> "ACL 2025") before the row is saved. Do not "optimise" this
  into stored columns without also handling settings changes.
- `papers.Paper` gained an unexported-in-spirit `Comment` field (the arXiv
  `<arxiv:comment>`); it is evidence for detection only and is deliberately NOT
  on the contract `main.Paper`.
- Detection evidence, strongest first: S2 `venue` / `publicationVenue.name` /
  `journal.name` (the last two were added to `S2Fields`, search + by-id only —
  the edge/recommendation endpoints reject unknown fields with a 400, which is
  why `S2EdgeFields` was left alone), arXiv `journal_ref`, arXiv comment
  ("Accepted at/to", "To appear in", "Published in", "camera-ready"), then a
  non-arXiv DOI as weak evidence (tier 1, label "published"). arXiv's own
  10.48550 DOI proves nothing and is ignored.
- Short aliases that occur in prose (`acl`, `ccs`, `www`, `fse`) are marked
  *risky*: they only count when the candidate string is short or a year sits
  within 12 characters. URLs are stripped from comments first, which is what
  stops "https://www.github.com/…" from reading as WWW.
- `Score` = tier (2 -> +100, 1 -> +40, only when `PaperPreferPublished`) +
  `log(cites+1)*8` + recency (<= 12 months, +15 decaying). `SortByScore` adds up
  to +5 for the source's own position so query relevance survives as a
  tie-break. Deliberately not a veto: a 90k-citation preprint still beats a
  12-citation journal paper (test asserts it).
- **The digest keeps at least 2 preprints** (`MinFreshPreprints`). "New on
  arXiv today" is almost entirely preprints, so pure score ordering emptied the
  fresh half — `pickDigestFresh` swaps the weakest published picks back out.
- `SearchPapers` is now a wrapper over **`SearchPapersFiltered(query, source,
  limit, publishedOnly)`** so the old binding signature is untouched.
  `PaperSearchResult` gained `Note`, set when S2 429s (then venues can only come
  from arXiv comments) and shown above the results.
- `config.DefaultTopVenues()` duplicates `papers.DefaultTopVenues()`: config
  cannot import papers (papers -> sync -> config is an import cycle).
  `TestTopVenueDefaultsMatch` in package main guards the copy.
- Frontend: gold badge for tier 2, plain for tier 1, dashed/muted "preprint" for
  tier 0, on result cards, library cards, the recs/digest rail and the
  citations drawer. Sort selects — results Best/Citations/Year, library those
  plus Added; "Best" must NOT re-sort results (the backend order is the
  ranking). "Published only" chip calls `searchPapersFiltered`. Settings ▸
  Papers has the top-venue chip editor and the "Prefer published" toggle.

### Verified run (2026-09-06)
`go run . --papers-test "machine unlearning"` with S2 429ing (the usual case):
the Note surfaced, and TMLR/ICML/CVPR papers detected purely from arXiv
comments took the top slots over same-day preprints; 13 of 20 results survived
"Published only".

## Repo
- GitHub: https://github.com/waihongteh/nussync (origin, branch main). Windows-only target; Mac/Linux port abandoned 2026-09-05 by user choice.

## Status: SHIPPED (2026-09-06 03:25)

Full integration pass against a live `wails dev` + real Canvas/arXiv backend,
then a clean `wails build`. Everything listed in the previous snapshot is now
verified against the real bindings, not the mock.

`go build/vet/test ./...` clean, `gofmt -l .` empty, `npm run check`
139 files / 0 errors, `npm run build` clean, `wails build` ->
`build/bin/nussync.exe` (17.7 MB); the exe launches, stays up, logs nothing but
the WebView2 line, and survives two Ctrl+Shift+N presses.

### Verified live this pass
- **What's new**: 125-item feed grouped day/course, "Mark all seen" -> nav badge
  0 and every `New` chip in Files cleared. `feed:updated` moves the badge after
  a sync (107 -> 125) without a reload.
- **Deadlines detail**: `GetDeadlineDetail` renders sanitized description HTML,
  submission types, points ("10 pts" on CS4246 Assignment 1), and resolves
  attachments through `FileIDsInHTML` (NST2030 Essay -> "Essay - Prompt.pdf").
- **Study**: status card = CLI found / not signed in, with the auth instructions
  and Re-check; every run button disabled in that state (so no hang is
  possible). Lazy page count works (Course Overview -> "17p").
- **Papers**: arXiv+S2 search ("machine unlearning" -> 40 of 2.4M), Add,
  Download (-> `files` row -1002, Pages 7, LocalPath set), status/page/stars/
  notes autosave and survive a reload, Citations/References drawer, BibTeX
  modal (3 entries), Recommendations, today's digest. The "Papers" pseudo-course
  shows in the sidebar/Files with its file count — intended — and is now
  filtered out of Settings > Courses to sync.
- **Chat**: Ctrl+J (real key) opens the panel; Files selection supplies context;
  session list; `SendChat` ends with exactly one `chat:done` carrying
  `Error: "Failed to authenticate: ..."` in ~5 s, no hang.
- **Arcade**: three cards; Quiz Rush empty state -> "Open Study" navigates.
- **Settings**: `SaveSettings` round-trips byte-identical incl. all three
  tokens; Hotkey and PaperDigestHour edits persist; paper bot card shows
  @paper_trackerrr_bot.
- Sidebar badges, all 10 palette navigation commands, the 3 arcade commands and
  the file index are present. Console is clean apart from the known dev-only
  `ipc.js ... reading 'nodes'` noise.

### Bugs found and fixed this pass
- `internal/papers/s2.go:21` — the citations/references and recommendations
  endpoints reject `tldr` outright (`HTTP 400 Unrecognized or unsupported
  fields: [tldr]`), so **every** Citations/References call failed and
  recommendations always silently fell back to arXiv. Added `S2EdgeFields`
  (S2Fields minus tldr) for `edge()` and `Recommendations()`; only
  `/paper/search` and `/paper/{id}` serve tldr.
- `frontend/src/lib/views/Papers.svelte:270` — `patch()` merged onto the `p` the
  each-block handed it, so two edits inside the 700 ms debounce window made the
  second drop the first (status + stars were lost when set right before a note).
  It now merges onto the current row from `library`.
- `frontend/src/lib/views/Settings.svelte:419` — "Courses to sync" listed the
  synthetic `Papers` course (id -1) once it owned files; its toggle did nothing.
  Now `$courses.filter((c) => c.ID > 0)`.

### Still untested / outstanding
- **Class score statistics and Grade.Mean**: no graded submissions exist on this
  account this term (`grades` table is empty), so the stats bar and the
  "vs class" column have only ever run against the mock.
- **Study end to end** and **paper summaries**: still blocked on
  `claude auth login`. Everything up to the auth failure is verified.
- Neither Telegram bot is paired (@nuscanvassync_bot, @paper_trackerrr_bot);
  Pair / Send test / digest delivery unexercised.
- `ChooseSyncDir` (native modal blocks automation).
- Global hotkey: registers with no error and the exe survives the keypresses,
  but the show/hide toggle itself was not observed.
- The arcade games' feel was not eyeballed (rAF is suspended while the dev
  browser pane is hidden).
- **Local DB was rebuilt at some point before this pass** (deadlines /
  announcements / grades / kv were empty, 107 file rows, FTS holding 2 docs).
  A full sync inside the app repopulated it: 126 files / ~178 MB, 5 deadlines,
  26 announcements, 0 grades. Side effect: every row got a fresh
  `first_seen_at`, so the whole library showed as "new" once.

## Status snapshot 2026-09-06 00:20 (superseded by "Status: SHIPPED" above)
- Built + committed + pushed: backend (sync incl. Pages, FTS, Telegram bot +
  commands, reminders, feed, detail/stats, tray, hotkey, toast, Study via
  Claude Code CLI), frontend (all views incl. What's new, Study, Arcade with
  Nibble Run / Lecture Merge / Quiz Rush, pet, settings). Exe builds.
- NOT yet integration-tested against real backend: What's new, deadline
  detail, Study, Quiz Rush, settings additions (only mock-verified).
- Study blocked until user runs `claude auth login`.
- Papers **backend** built 2026-09-06 (see "Papers backend" above): search,
  library, download+index, digest, BibTeX, paper summary, in-app chat, second
  Telegram bot. Frontend Papers view + chat panel is the frontend agent's half.
  Outstanding: pair the paper bot (@paper_trackerrr_bot), run
  `claude auth login` for summary/chat, then an integration pass and a build.
- Queued 2026-09-06 morning: MarkAllAnnouncementsRead binding + button;
  Deadlines badge = unsubmitted count only (not all upcoming).
- Queued: sidebar courses drag-to-reorder (persist order), right-click Hide
  (sidebar only; sync unaffected), collapsed "Hidden (n)" row to unhide,
  "Hide non-academic" quick action.
- Pet placement modes (2026-09-06): `petMode` dock/drag/wander in
  `nussync.pet.prefs`, position fractions in `nussync.pet.pos`; SVG extracted to
  `components/PetSprite.svelte`, non-dock modes render in `components/PetOverlay.svelte`
  (fixed layer, `pointer-events:none`, z-index 30 = below chat 45 / palette 150 / toasts 200).
- Done 2026-09-06: Deadlines sidebar badge now counts `!Submitted` locally in
  `Sidebar.svelte` (was `upcomingCount`, which dropped overdue-unsubmitted).
- Done 2026-09-06: `MarkAllAnnouncementsRead` (app.go + store) with an
  Announcements header button and a palette command, both via the shared
  `lib/announceActions.ts`; documented in docs/CONTRACT_FEATURES.md §6.
- Done 2026-09-06: sidebar course order/hiding in `lib/courseOrder.ts`
  (localStorage `nussync.courses.order` / `.hidden`, no backend surface) —
  pointer-event drag (150ms hold or hover grip, ROW pitch 28px), right-click
  ContextMenu (hide / move to top / open in Canvas), collapsed "Hidden (n)"
  row, Alt+↑/↓ on a focused row, and a Settings → Sync "Hide non-academic
  courses" quick action. Hidden is presentation only: those courses still sync
  and still appear in Files/Deadlines/Settings.

## Paper bot: search + library commands (2026-09-06)
Code: `internal/notify/papercmds.go` (+ `papercmds_test.go`), the extended
handler in `internal/notify/paperbot.go`, `app_paperbot.go`, `cli_paperbot.go`.

- Commands added to the **paper** bot only (deliberately not the course bot):
  `/search <query>` (top 5, all sources), `/download <n>`, `/library
  [toread|reading|done]` (max 15), `/done <n>`, `/start-reading <n>` (alias
  `/read <n>`). `/save <n>` now indexes into whichever numbered list the chat
  saw last — search results *or* the digest.
- **Numbering is remembered per chat**, in memory and mirrored into the kv
  table (`paperbot_list_<chat>` for search/digest, `paperbot_lib_<chat>` for
  the library listing), so it survives a restart. Two keys, not one: a later
  `/search` must not silently repoint `/done 2` at a search hit. Verified
  across separate processes.
- `PaperBot.Handle(ctx, chatID, cmd, args)` is the single command entry point
  and **returns** the reply text; the poll loop wraps it in `handle`, which
  sends the "Searching…"-style progress line first. That split is what lets the
  CLI print a reply instead of sending one, and what makes the handler
  table-testable with a fake `PaperExt`.
- **`notify` stays a transport package.** The app implements the new
  `notify.PaperExt` interface (search/digest/save/download/library/status) and
  hands the bot flat `notify.PaperItem` values, so notify never imports
  `internal/papers`. Formatting for the new commands therefore lives in
  `notify` (`SearchResultsMessage`, `LibraryMessage`, …), not in
  `papers/digest.go`.
- Wiring lives in **`app_paperbot.go`**, not `app_papers.go`: `App.startup`
  calls `installPaperBotExt()` right after `papersInit()`, which does
  `paperBot().SetExt(paperExt{a})`. `SetExt` is mutex-guarded because the poll
  loop is already running. With no ext installed the bot keeps exactly its old
  digest-only behaviour (and its old help), which is what the tests pin.
- `notify.PaperHelpMessage` (the extended help) shadows
  `papers.PaperHelpMessage` (digest-only) whenever an ext is set — `help()`
  chooses, rather than the app overwriting the `Help` field after `Start`.
- New CLI: `nussync --papers-bot-test "/search LLM unlearning"` runs the real
  handler headlessly and prints the reply plus its byte count. In Git Bash you
  must prefix `MSYS2_ARG_CONV_EXCL='*' MSYS_NO_PATHCONV=1`, otherwise MSYS
  rewrites `/search …` into `C:/Program Files/Git/search …`.

### Verified run (2026-09-06)
`--papers-bot-test "/search LLM unlearning"` on the paired paper bot: 5 hits
with the new venue ranking visible (COLM 2025, ICML 2025 Workshop), first two
authors + "et al.", `year · venue|preprint · N citations`, links, 1077 bytes /
1 message. `/library` listed the 3 saved papers with status and page/pages;
`/help` shows all ten commands; `/download 99` in a *fresh process* answered
"the last list had 5 entries", proving the kv-backed numbering persists.
`go build/vet/test ./...` and `gofmt -l .` clean.
- Dead end / bug 2026-09-06: UpsertGrade reset `notified=0` on every sync
  (unchanged grades re-sent to Telegram on each restart). Fixed with a
  CASE in the upsert; plus first-run backlog suppression keyed
  `notify_backlog_v2` (the older `notify_bootstrapped` key was already set
  on this machine, so a new key was required).
- Queued: chat panel docked (push content, resizable, persisted) instead of overlay; overlay as optional mode.

## Layout: collapsible panes (2026-09-06, frontend-only)
All of it lives in `frontend/src/lib/layout.ts` (plain stores, localStorage
only — never backend settings) plus `resetLayout()`. Keys are namespaced
`nussync.layout.*`.
- **Sidebar has three tiers**, not two: `'auto' | 'expanded' | 'rail'`, default
  `auto` (key `nussync.layout.sidebar`). `auto` = rail below 1100px window
  width. An explicit Ctrl+B choice sticks until the window is *widened past*
  1200px, at which point the mode drops back to `auto`. That revert is a
  **crossing test against the previous width** (kept in `initLayout`), not a
  plain `w > 1200` check — the naive version undoes a deliberate "rail" pick on
  a wide monitor the instant it is made.
- `initLayout()` (called from App's `$effect`) owns the resize listener and
  writes `--sidebar-w` on `<html>` from the resolved mode (rail = 56px, else
  `sidebarW`, 200–280). Everything that positions against the sidebar — the
  drag ghost, the viewer's focus overlay — reads that one variable instead of
  measuring. Sidebar width transition is 160ms; the global
  `prefers-reduced-motion` rule in style.css already zeroes it.
- Rail contents: icon-only nav (badge count collapses to a dot, count moves
  into the tooltip), courses as colour dots (hidden courses omitted, no
  drag-reorder), `Pet compact` = a 28px sprite whose quip bubble floats to the
  right, `SyncPill compact` = one button with an SVG progress ring. The compact
  pill **keeps the `.pill` class**: PetOverlay's wander `dockSpot()` and
  `busyStep()` query `aside.sidebar .pill`, so renaming it would break the
  pet's hop target. `data-nav="deadlines"` likewise stays on rail nav buttons.
- **Tooltips**: `lib/tooltip.ts` exports a `use:tip` action feeding one shared
  store; `components/Tooltip.svelte` (mounted once in App) renders the single
  bubble. 400ms on hover, instant on focus, right-positioned, dismissed by any
  key/scroll/resize. Not `title=` — the OS tooltip is slow and unstyleable.
- **Files tree**: `treeOpen` (`nussync.layout.tree`, default open),
  `treeW` (180–420). Toggle = toolbar `panelLeft` button, Ctrl+Shift+E, palette.
  Auto-hides when the *panes row* (measured with a ResizeObserver, not the
  window) is under 900px and a viewer is open — the user's preference is left
  alone, so it returns on widening. The list breadcrumb is now a clickable
  crumb trail rooted at "All files", because with the tree hidden it is the
  only way up; crumb keys are the same `courseID:relPath` keys as the tree.
- **Viewer focus mode** lives inside `Viewer.svelte` (so Files, Papers, Study
  and What's-new all get it): `viewerFocus` store, *not* persisted, cleared on
  the component's own teardown so the next preview never opens expanded.
  Ctrl+Shift+P, header double-click or the expand button; a slim strip adds the
  breadcrumb, ←/→ (hosts pass `onPrev`/`onNext` when they have a list) and Exit.
  The focused pane is `position: fixed` at `top: var(--topbar-h);
  left: var(--sidebar-w)`. Esc is handled there — **hosts must skip their own
  Esc/arrow handling while `$viewerFocus`** (Files and What's-new do;
  `stopPropagation` is not enough between two `<svelte:window>` listeners).
  Files' Ctrl+P also had to exclude Shift, or it swallowed Ctrl+Shift+P.
- `components/ResizeHandle.svelte` replaces the ad-hoc handle in Files and now
  drives the sidebar, tree and viewer. **Delta-based** (start value + pointer
  delta, `invert` for a pane on the right) rather than "pane width = pointer x −
  pane left", so a clamped pane does not jump when the pointer returns. Arrow
  keys resize when focused (Shift = bigger step), double-click resets to the
  default. It is a focusable `role="separator"` div: a `<button>` may not take
  that role (svelte-check a11y error).
- Settings → App grew a "Layout" sub-section (sidebar mode select + Reset
  layout); the palette gained Toggle sidebar / folder tree / preview focus mode
  / Reset layout, with the shortcut in the hint column.
- **Chat is a docked column by default** (2026-09-06): `chatMode`
  `'docked' | 'overlay'` (`nussync.layout.chatMode`, default docked) + `chatW`
  (320–560, default 380). Docked, App renders `[sidebar][main][handle][chat]`
  and `ChatPanel docked` drops its `position: fixed`/shadow/slide-in; App
  publishes the live column width as `--chat-w` (0 when closed or floating), so
  the viewer's focus mode (`right: var(--chat-w)`) and PetOverlay's `rightEdge()`
  both stay clear of it — the pet re-clamps on a `tick()`, never rAF, because a
  background tab gets no frames. Content narrower than 1100px forces overlay for
  the session (`resolvedChatMode`, preference untouched, "narrow window" chip in
  the header). Files spends a squeeze in a fixed order: tree auto-hides, then
  the list narrows to 260px, then the viewer gives ground down to 420px
  (`effViewerW`; `viewerW` stays the untouched preference). Esc closes a docked
  chat only when the focus is inside it — Files' and Viewer's window handlers
  bail on `closest('aside.chat')`, or typing in the composer would close the
  preview or leave focus mode.
- Verified 2026-09-06: `npm run check` 0 errors/0 warnings, `npm run build` ok;
  in-browser at 1400/1300/1000/880px — Ctrl+B, auto-rail, the 1200px revert,
  tooltips (light + dark), tree toggle + persistence, all three drags and a
  double-click reset, tree auto-hide at 824px panes, focus mode via button and
  palette, Esc exit. No console errors.
  Note for future browser checks: the in-app Browser pane throttles frames, so
  CSS transitions and ResizeObserver callbacks appear frozen until a screenshot
  forces a frame, and `resize_window` does not always fire a `resize` event —
  dispatch one manually when testing width-dependent logic.
