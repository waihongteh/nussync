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

## Dead ends
- Sanitizing the whole cross-listed course code into one folder name.
- Passing a multi-line `claude` prompt as an argv element on Windows (see above).
- `claude --bare` for the Study backend: it disables OAuth and demands an API key.
- Anthropic Go SDK for Study: needs an API key the user does not have.
- FTS5 contentless tables (`content=''`) — `snippet()` cannot work on them.
- `regexp.MustCompile` with a backreference (`</\1>`) — panics at init in RE2.
- Guarding `FileIDsInHTML` with `strings.Contains(body, "/files/")` *before*
  unescaping `\/` — JSON-escaped bodies then extract nothing. Normalise first.
- Running the Telegram pairing long poll alongside the command loop. Telegram
  gives each update to one `getUpdates` caller only; both consumers then lose
  messages at random. One loop, and pairing waits on it.
- Back-filling `first_seen_at` for the existing library so the feed has content
  on day one — every synced file would show as "new" forever after.

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
- Desktop shortcut created at %USERPROFILE%\Desktop\NUSSync.lnk -> build/bin/nussync.exe.
  Taskbar pin cannot be automated on Win11; user pins manually.

## Repo
- GitHub: https://github.com/waihongteh/nussync (origin, branch main). Windows-only target; Mac/Linux port abandoned 2026-09-05 by user choice.
