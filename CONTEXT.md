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

## Verified API facts (2026-09-05)
- Token works; user id 127122. 14 active courses, 51 total.
- Rate limit bucket 700; request cost 0.35–1.05. No pressure.
- `/users/self/todo` and `/planner/items` both work for due dates.

## Status
- [x] API probe, .gitignore
- [ ] Go backend: client, sync, sqlite, config
- [ ] Telegram + reminder daemon
- [ ] Wails UI (minimal, modern)
- [ ] Build exe

## Dead ends
(none yet)
