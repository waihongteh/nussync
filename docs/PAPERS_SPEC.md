# Papers feature spec (queued for build after usage reset, ~02:38 2026-09-06)

User: FYP on LLM unlearning / knowledge editing. Needs paper search, download,
reading-progress tracking, Telegram paper-of-the-day. Google Scholar has NO API:
never scrape it; only an "Open in Scholar" button.

## Sources
- arXiv API `http://export.arxiv.org/api/query` (Atom; >=3s between requests).
- Semantic Scholar Graph API `https://api.semanticscholar.org/graph/v1` (no key;
  fields title,abstract,year,venue,authors,citationCount,openAccessPdf,tldr,
  externalIds,url,publicationDate; paper/search, paper/{id}, /citations,
  /references; recommendations POST .../recommendations/v1/papers positivePaperIds).
  Retry on 429. Cache 24h in `papers_cache` table.

## Backend (Go, new files: internal/papers/**, internal/store/papers.go,
types_papers.go, app_papers.go with papersInit() via sync.Once, cli_papers.go,
docs/CONTRACT_PAPERS.md; minimal edits to config/notify)
Types: Paper{ID "arxiv:..."|"s2:...", ArxivID, S2ID, DOI, Title, Authors[],
Year, Venue, Abstract, TLDR, CitationCount, URL, PDFURL, PublishedAt, Source};
LibraryPaper{Paper; Status toread|reading|done; Page; Pages; Stars 0-5; Tags[];
Notes; KeyIdea; LocalPath; AddedAt; UpdatedAt; ReadAt; FileID};
PaperSearchResult{Papers, Total}; CitationLink{Paper; InLibrary; Status};
PaperDigest{Date; Papers[]; Reason[]}.
Settings: PaperKeywords (default machine unlearning, LLM unlearning, knowledge
editing, model editing, knowledge unlearning, memorization), PaperCategories
(cs.CL, cs.LG, cs.AI), PaperDigestHour 9, NotifyPapers true.
Methods: SearchPapers(q, source all|arxiv|s2, limit); GetPaper(id);
AddPaperToLibrary(p); RemovePaperFromLibrary(id); GetLibrary(status);
UpdateLibraryPaper(lp) (ReadAt on done); DownloadPaperPDF(id) ->
<SyncDir>/Papers/<year> - <first author> - <title>.pdf, register row in `files`
(synthetic "Papers" course) + FTS index so Study/search work, set FileID;
GetCitations/GetReferences(id, limit); GetRecommendations(limit) (S2 recs from
library, fallback arXiv keyword search); GetPaperDigest(date) cached per day
(arXiv new in categories matching keywords top 5 + 3 recs not in library);
SendPaperDigestNow(); ExportBibTeX(ids); OpenScholar(query).
Event: papers:updated.
Telegram: daily digest at PaperDigestHour (title bold, first 3 authors,
year/venue, TLDR/200 chars, link; <4000 chars); commands /paper, /save <n>,
/reading; update /help.
Tests: arXiv Atom parse fixture, S2 JSON parse, keyword match, BibTeX, filename.
CLI `--papers-test "<query>"` real run: search, digest, download one PDF.

## Frontend (after backend): Papers view — search box (arxiv/s2/all), result
cards (add / download / open), Library with status columns or filter, progress
(page/pages slider), stars, tags, notes, key idea, "Study this paper" jump to
Study view with FileID, citations/references drawer with in-library marks,
Recommendations strip, Digest preview + "Send now", BibTeX export copy/save,
Settings section for keywords/categories/hour/toggle. Nav item "Papers".

## Telegram: SEPARATE BOT (added 2026-09-05 23:50)
- `.env` has `PAPER_TRACKER_TELEGRAM_TOKEN` — a second bot dedicated to papers.
  Import into Settings as `PaperTelegramToken`; separate `PaperTelegramChatID`
  and its own pairing (`PairPaperTelegram()`), own long-poll loop in
  internal/notify (reuse telegram.Client). Digest + /paper /save /reading
  /help go through THIS bot only; the NUSSync bot keeps course commands.
- Settings UI: Papers section shows the paper bot status + Pair + Send test.
- Ask user for the bot's @username when awake (unknown yet); not needed for API.
