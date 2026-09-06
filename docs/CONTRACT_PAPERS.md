# NUSSync — Papers, paper summary and chat contract

Backend = Go methods on `App` in `app_papers.go` / `app_chat.go` (package main),
bound via Wails v2. Frontend calls `window.go.main.App.<Method>(...)` (Promise).
Types below are Go; JSON field names are the Go field names exactly (no json
tags), so frontend TS uses the same PascalCase. Same conventions as
`docs/API_CONTRACT.md`. This file is the source of truth for everything below;
`docs/API_CONTRACT.md` remains the base contract (and carries the new Settings
fields).

Sources: **arXiv** (Atom API) and **Semantic Scholar** (Graph API, no key).
Google Scholar has **no API and is never scraped** — `OpenScholar` only opens a
search URL in the browser.

Summary and chat are **on demand** and shell out to the locally installed
**Claude Code CLI** exactly like Study (`docs/CONTRACT_STUDY.md`); there is no
Anthropic API key.

## Types
```go
type Paper struct {
    ID string /*"arxiv:<id without version>" | "s2:<paperId>"*/
    ArxivID string /*versionless*/; S2ID string; DOI string
    Title string; Authors []string; Year int; Venue string
    Abstract string; TLDR string /*Semantic Scholar only*/
    CitationCount int; URL string; PDFURL string
    PublishedAt string /*RFC3339 (arXiv) or YYYY-MM-DD (S2)*/
    Source string /*"arxiv" | "s2"*/
    VenueTier int /*2 top venue, 1 published, 0 preprint/unknown — DERIVED*/
    VenueShort string /*badge label: "NeurIPS 2025" | "published" | "preprint"*/
    Published bool /*VenueTier >= 1*/
}
type LibraryPaper struct {
    Paper                       // EMBEDDED: its fields are flattened in JSON/TS
    Status string /*"toread"|"reading"|"done"*/
    Page int; Pages int; Stars int /*0-5*/; Tags []string
    Notes string; KeyIdea string
    LocalPath string /*absolute path of the downloaded PDF, "" until downloaded*/
    AddedAt string; UpdatedAt string; ReadAt string /*set when Status becomes "done"*/
    FileID int /*synthetic files row, 0 until downloaded — pass to Study/chat*/
}
type PaperSearchResult struct {
    Papers []Paper; Total int
    Note string /*"" normally; set when the result is degraded (S2 rate limited)*/
}
type CitationLink struct { Paper; InLibrary bool; Status string /*"" when not saved*/ }
type PaperDigest struct { Date string /*YYYY-MM-DD*/; Papers []Paper; Reason []string /*Reason[i] explains Papers[i]*/ }
type PaperSummary struct { PaperID string; Markdown string; CreatedAt string; Model string }

type ChatSession struct {
    ID string; FileID int; PaperID string; Title string
    ClaudeSessionID string /*CLI session id used for --resume*/
    Model string; CreatedAt string; UpdatedAt string
}
type ChatMessage struct {
    ID int; SessionID string; Role string /*"user"|"assistant"*/
    Text string; CreatedAt string
}
type ChatDelta struct { SessionID string; Text string }        // `chat:delta`
type ChatDone  struct { SessionID string; JobID string; Error string } // `chat:done`
```

New `Settings` fields (full type in `docs/API_CONTRACT.md`):
```go
PaperTelegramToken string; PaperTelegramChatID string  // SECOND bot
PaperKeywords []string    // default: machine unlearning, LLM unlearning,
                          // knowledge editing, model editing,
                          // knowledge unlearning, memorization
PaperCategories []string  // default ["cs.CL","cs.LG","cs.AI"]
PaperDigestHour int       // 0-23 local, default 9
NotifyPapers bool         // default true
PaperTopVenues []string   // tier-2 venues; default NeurIPS, ICML, ICLR, ACL,
                          // EMNLP, NAACL, EACL, COLING, AAAI, IJCAI, COLM,
                          // TACL, JMLR, TMLR, CVPR, ICCV, ECCV, KDD, WWW,
                          // SIGIR, USENIX Security, IEEE S&P, CCS, NDSS,
                          // ICSE, FSE. Empty falls back to that list.
PaperPreferPublished bool // default true; turns the tier weight in Score on
```

## Methods on App — papers
```
SearchPapers(query string, source string, limit int) (PaperSearchResult, error)
    // Wrapper for SearchPapersFiltered(query, source, limit, false).
SearchPapersFiltered(query, source string, limit int, publishedOnly bool) (PaperSearchResult, error)
    // source "all" | "arxiv" | "s2" ("" = all). limit<=0 -> 20, capped at 100.
    // Results are RANKED best-first by papers.Score (see "Venue ranking").
    // publishedOnly drops every VenueTier 0 row and rewrites Total.
    // Semantic Scholar's public tier rate-limits hard: when source is "all" an
    // S2 failure is NON-FATAL and arXiv-only results come back. Duplicates are
    // merged on the versionless arXiv id, the arXiv row winning and inheriting
    // the S2 metadata (TLDR, DOI, venue, citation count).
GetPaper(id string) (Paper, error)            // library copy first, else the API
AddPaperToLibrary(p Paper) (LibraryPaper, error)   // status "toread"; idempotent
RemovePaperFromLibrary(id string) error            // the PDF on disk is kept
GetLibrary(status string) ([]LibraryPaper, error)  // "" or "all" = every status
UpdateLibraryPaper(lp LibraryPaper) (LibraryPaper, error)
    // Writes back reading state. Stars clamped 0-5. Status "done" stamps ReadAt;
    // moving away from "done" clears it. Returns the stored row.
DownloadPaperPDF(id string) (LibraryPaper, error)
    // -> <SyncDir>/Papers/<year> - <first author surname> - <title>.pdf,
    // registers a `files` row under the synthetic "Papers" course (id -1),
    // extracts text into the FTS index, fills LocalPath/FileID/Pages.
    // Saves the paper to the library first when it is not there yet.
GetCitations(id string, limit int) ([]CitationLink, error)   // Semantic Scholar
GetReferences(id string, limit int) ([]CitationLink, error)  // Semantic Scholar
GetRecommendations(limit int) ([]Paper, error)
    // S2 recommendations seeded with the 5 newest library papers; falls back to
    // an arXiv keyword search when S2 is unavailable or the library is empty.
    // Sorted by Score.
GetPaperDigest(date string) (PaperDigest, error)   // "" = today; cached per day
SendPaperDigestNow() error                          // pushes it to the paper bot
ExportBibTeX(ids []string) (string, error)          // empty ids = whole library
OpenScholar(query string) error                     // opens scholar.google.com
```

## Methods on App — paper summary (Claude)
```
StartPaperSummary(paperID string, model string) (jobID string, err error)
    // Queued on the SAME single-slot Study queue, Kind "paper_summary"; progress
    // and completion arrive on `study:job` / `study:progress`. The PDF is
    // downloaded automatically when the library has no local copy.
    // Sections: Contribution / Method / Key results / Limitations /
    // Relevance to LLM unlearning & knowledge editing / One-line takeaway /
    // Related work worth reading.
GetPaperSummary(paperID string) (PaperSummary, error)   // zero value when none
```

## Methods on App — chat
```
StartChat(fileID int, paperID string, model string) (ChatSession, error)
    // fileID 0 and paperID "" is legal: a chat with no context still works.
    // Nothing is sent to Claude until SendChat.
SendChat(sessionID string, message string) (jobID string, err error)
    // Queued (Kind "chat"). The reply streams as `chat:delta` and always ends
    // with exactly one `chat:done` (Error "" on success).
GetChats(fileID int, paperID string) ([]ChatSession, error)  // 0/"" = all
GetChatMessages(sessionID string) ([]ChatMessage, error)     // oldest first
DeleteChat(sessionID string) error
```

## Methods on App — paper Telegram bot (SEPARATE bot)
```
GetPaperTelegramStatus() TelegramStatus   // BotName includes the leading @
PairPaperTelegram() (string, error)       // waits <=60s for /start; saves the chat id
SendPaperTestTelegram() error
```

## Events (EventsEmit from Go)
- `papers:updated` — no payload. Emitted on every library/summary change.
- `chat:delta` — payload `ChatDelta`. The CLI's stream-json emits whole
  assistant messages rather than token deltas, so each event is one assistant
  text block; concatenate them in order.
- `chat:done` — payload `ChatDone`, once per SendChat.
- `study:job` / `study:progress` — the shared job events, also used by
  `paper_summary` and `chat` jobs.
- `toast` — the shared toast event.

## Behaviour and limits
- **arXiv**: `https://export.arxiv.org/api/query`, Atom parsed with
  `encoding/xml`. Free-text queries become `all:<term> AND all:<term>`, quoted
  phrases stay whole. The digest uses
  `(cat:cs.CL OR …) AND (all:"keyword" OR …)` sorted `submittedDate` desc. One
  process-wide limiter enforces **>= 3 s** between calls (PDF downloads from
  arxiv.org share it). User-Agent `NUSSync/1.0`.
- **Semantic Scholar**: `https://api.semanticscholar.org/graph/v1`, no key.
  HTTP 429/5xx is retried up to 3 times with 1s/2s/4s backoff; a final failure
  is surfaced to the caller but is non-fatal for `SearchPapers(…, "all", …)`.
  Every GET body is cached 24 h in `papers_cache(key, body, fetched_at)`.
  **The unauthenticated tier answers 429 most of the time** — treat S2 data
  (TLDR, citations, references, recommendations) as best-effort.
- **Paper ids** are `arxiv:<versionless id>` or `s2:<paperId>`.
- **The "Papers" course**: downloaded PDFs need a `files` row so Study, FTS
  search and chat work on them, so NUSSync creates one synthetic course row
  `id=-1, code="Papers", name="Research papers", enabled=0` and gives each PDF a
  negative `files.id` (from -1000 downwards). Canvas ids are always positive, so
  the two spaces never collide; the sync engine only touches courses Canvas
  listed, and `GetCourses` hides the row while it owns no files.
- **Digest**: top 5 keyword-matching new arXiv papers plus up to 3
  recommendations not already in the library, cached per day, rendered as HTML
  under 4000 bytes. Both halves are ordered by Score, except that the arXiv half
  always keeps at least `papers.MinFreshPreprints` (2) tier-0 papers — "new
  today" is almost always preprints, and a purely score-ordered digest goes
  stale.
- **Venue ranking** (`internal/papers/venue.go`): `VenueTier`/`VenueShort`/
  `Published` are DERIVED on every read (`toPaper`, `toLibraryPaper`), never
  stored — `papers_library` keeps only `venue`. When a venue is found solely in
  the arXiv comment, the detected short name is written back into `Venue` so it
  survives the round-trip. Evidence, strongest first: S2 `venue` /
  `publicationVenue.name` / `journal.name`, arXiv `<arxiv:journal_ref>`, arXiv
  `<arxiv:comment>` ("Accepted at/to X", "To appear in X", "Published in X",
  "X camera-ready"), and — weakest — a non-arXiv DOI (tier 1, label
  "published"). Names are normalised to a canonical short form (NIPS ->
  NeurIPS, "Advances in Neural Information Processing Systems" -> NeurIPS, …).
  A venue containing "workshop" is tier 1, never tier 2.
  `Score(p, prefs)` = tier weight (2 -> +100, 1 -> +40, only when
  `PaperPreferPublished`) + `log(citations+1)*8` + a recency bonus (<= 12
  months, +15 decaying linearly). `SortByScore` adds up to +5 for the source's
  own ordering, so query relevance survives as a tie-break. A heavily cited
  preprint still outranks a lightly cited journal paper: the tier is a thumb on
  the scale, not a veto.
- **S2 rate limiting**: when Semantic Scholar 429s during a search, venues can
  only come from the arXiv comments; the result carries that explanation in
  `PaperSearchResult.Note` and the Papers view shows it above the results.
- **Telegram**: the paper bot is a *second* bot (token from `.env`
  `PAPER_TRACKER_TELEGRAM_TOKEN`, imported into `PaperTelegramToken` on first
  run or whenever it is empty) with its own chat id and pairing. Commands:
  `/paper`, `/save <n>`, `/reading`, `/help`, `/start`. `/save <n>` indexes into
  the most recently built digest. The NUSSync bot keeps the course commands.
- **BibTeX**: arXiv preprints are `@misc` with `eprint`/`archivePrefix`/
  `primaryClass`; anything with a DOI from Semantic Scholar is `@article`. Keys
  are `<surname><year><first meaningful title word>`, disambiguated with a
  trailing letter.
- **Chat multi-turn**: the first turn runs `claude -p` normally and stores the
  `session_id` from the result line; later turns add `--resume <id>` (verified
  working with `-p` on CLI 2.1.251). If a resume fails, the turn is retried once
  from scratch with the transcript replayed inside the prompt.

## Storage
SQLite tables in the existing database, created lazily by
`store.MigratePapers()`: `papers_library`, `papers_cache`, `papers_digests`,
`paper_summaries`, `study_chats`, `study_chat_messages`.

## CLI
`nussync --papers-test "<query>" [--claude-test] [--digest-send]` searches both
sources, builds today's digest, downloads one arXiv PDF and verifies the files
row and page count. `--claude-test` also runs a summary and a chat turn;
`--digest-send` pushes the digest to the paper bot (needs it paired).
