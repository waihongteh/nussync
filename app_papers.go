package main

// Papers backend: arXiv + Semantic Scholar search, a local library with
// reading state, PDF download into the synced library, the daily Telegram
// digest through the SECOND (paper) bot, BibTeX export and the on-demand
// Claude paper summary. See docs/CONTRACT_PAPERS.md.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	gosync "sync"
	"sync/atomic"
	"time"

	"nussync/internal/config"
	"nussync/internal/index"
	"nussync/internal/notify"
	"nussync/internal/papers"
	"nussync/internal/store"
	"nussync/internal/study"
	"nussync/internal/telegram"
)

// ------------------------------------------------------------------ runtime

// papersRT holds the process-wide paper clients and the paper bot.
var papersRT struct {
	once gosync.Once
	err  error

	arxiv *papers.Arxiv
	s2    *papers.S2
	http  *http.Client

	mu  gosync.Mutex
	bot *notify.PaperBot
	// lastDigest is what /save <n> indexes into.
	lastDigest papers.Digest
}

// papersInit prepares the Papers feature: tables, HTTP clients and — in the
// GUI — the second Telegram bot with its digest ticker. Called from
// App.startup right after studyInit, and lazily by every method below.
func (a *App) papersInit() error {
	if a.st == nil {
		return errors.New("not initialised")
	}
	setVenuePrefs(a.settings())
	papersRT.once.Do(func() {
		if err := a.st.MigratePapers(); err != nil {
			papersRT.err = err
			return
		}
		if err := a.st.EnsurePapersCourse(); err != nil {
			papersRT.err = err
			return
		}
		papersRT.arxiv = papers.NewArxiv()
		papersRT.s2 = papers.NewS2(papersCache{a.st})
		papersRT.http = &http.Client{Timeout: 5 * time.Minute}

		if !a.headless {
			a.startPaperBot()
		}
	})
	return papersRT.err
}

// papersCache adapts the store to the papers.Cache interface.
type papersCache struct{ st *store.Store }

func (c papersCache) Get(key string, maxAge time.Duration) (string, bool) {
	return c.st.PapersCacheGet(key, maxAge)
}
func (c papersCache) Put(key, body string) { c.st.PapersCachePut(key, body) }

func (a *App) paperTelegram() *telegram.Client {
	return telegram.New(a.settings().PaperTelegramToken)
}

func (a *App) startPaperBot() {
	cfg := a.settings()
	bot := notify.NewPaperBot(a.st, a.paperTelegram(), cfg)
	bot.Help = papers.PaperHelpMessage
	bot.Digest = func(ctx context.Context) (string, error) {
		d, err := a.buildDigest(ctx, todayLocal())
		if err != nil {
			return "", err
		}
		return papers.DigestMessage(d), nil
	}
	bot.Save = func(ctx context.Context, n int) (string, error) { return a.savePaperN(n) }
	bot.Reading = func(ctx context.Context) (string, error) { return a.readingMessage() }
	bot.SendDigest = func(ctx context.Context) error { return a.SendPaperDigestNow() }
	bot.DigestHour = func() int { return a.settings().PaperDigestHour }
	bot.Enabled = func() bool { return a.settings().NotifyPapers }
	bot.OnPair = func(chatID string) { _, _ = a.savePairedPaperChat(chatID) }
	bot.Start(a.baseCtx())

	papersRT.mu.Lock()
	papersRT.bot = bot
	papersRT.mu.Unlock()
}

func paperBot() *notify.PaperBot {
	papersRT.mu.Lock()
	defer papersRT.mu.Unlock()
	return papersRT.bot
}

// venuePrefs is the snapshot of the ranking settings used by toPaper and
// toLibraryPaper, which have no App receiver. It is refreshed by papersInit
// and by papersApplySettings; the zero value falls back to the defaults.
var venuePrefsVal atomic.Pointer[papers.VenuePrefs]

func setVenuePrefs(cfg config.Settings) {
	p := papers.VenuePrefs{
		TopVenues:       cfg.PaperTopVenues,
		PreferPublished: cfg.PaperPreferPublished,
	}.Normalised()
	venuePrefsVal.Store(&p)
}

// venuePrefs returns the current ranking settings.
func venuePrefs() papers.VenuePrefs {
	if p := venuePrefsVal.Load(); p != nil {
		return *p
	}
	return papers.DefaultVenuePrefs()
}

// papersChanged emits the refresh event the Papers view listens on.
func (a *App) papersChanged() { a.emit("papers:updated") }

func todayLocal() string { return time.Now().Local().Format("2006-01-02") }

// ------------------------------------------------------------- conversions

func toPaper(p papers.Paper) Paper {
	papers.AnnotateVenue(&p, venuePrefs())
	return Paper{
		ID: p.ID, ArxivID: p.ArxivID, S2ID: p.S2ID, DOI: p.DOI, Title: p.Title,
		Authors: p.Authors, Year: p.Year, Venue: p.Venue, Abstract: p.Abstract,
		TLDR: p.TLDR, CitationCount: p.CitationCount, URL: p.URL,
		PDFURL: p.PDFURL, PublishedAt: p.PublishedAt, Source: p.Source,
		VenueTier: p.VenueTier, VenueShort: p.VenueShort, Published: p.Published,
	}
}

func fromPaper(p Paper) papers.Paper {
	return papers.Paper{
		ID: p.ID, ArxivID: p.ArxivID, S2ID: p.S2ID, DOI: p.DOI, Title: p.Title,
		Authors: p.Authors, Year: p.Year, Venue: p.Venue, Abstract: p.Abstract,
		TLDR: p.TLDR, CitationCount: p.CitationCount, URL: p.URL,
		PDFURL: p.PDFURL, PublishedAt: p.PublishedAt, Source: p.Source,
		VenueTier: p.VenueTier, VenueShort: p.VenueShort, Published: p.Published,
	}
}

func toPapers(ps []papers.Paper) []Paper {
	out := make([]Paper, 0, len(ps))
	for _, p := range ps {
		out = append(out, toPaper(p))
	}
	return out
}

func toLibraryPaper(r store.LibraryPaper) LibraryPaper {
	// The venue tier is derived, not stored, so a library row is re-annotated
	// on the way out — the settings may have changed since it was saved.
	return LibraryPaper{
		Paper:  toPaper(libraryPaperToPapers(r)),
		Status: r.Status, Page: r.Page, Pages: r.Pages, Stars: r.Stars,
		Tags: r.Tags, Notes: r.Notes, KeyIdea: r.KeyIdea, LocalPath: r.LocalPath,
		AddedAt: r.AddedAt, UpdatedAt: r.UpdatedAt, ReadAt: r.ReadAt, FileID: r.FileID,
	}
}

func fromLibraryPaper(lp LibraryPaper) store.LibraryPaper {
	return store.LibraryPaper{
		ID: lp.ID, ArxivID: lp.ArxivID, S2ID: lp.S2ID, DOI: lp.DOI,
		Title: lp.Title, Authors: lp.Authors, Year: lp.Year, Venue: lp.Venue,
		Abstract: lp.Abstract, TLDR: lp.TLDR, CitationCount: lp.CitationCount,
		URL: lp.URL, PDFURL: lp.PDFURL, PublishedAt: lp.PublishedAt,
		Source: lp.Source, Status: lp.Status, Page: lp.Page, Pages: lp.Pages,
		Stars: lp.Stars, Tags: lp.Tags, Notes: lp.Notes, KeyIdea: lp.KeyIdea,
		LocalPath: lp.LocalPath, AddedAt: lp.AddedAt, UpdatedAt: lp.UpdatedAt,
		ReadAt: lp.ReadAt, FileID: lp.FileID,
	}
}

func libraryPaperToPapers(r store.LibraryPaper) papers.Paper {
	return papers.Paper{
		ID: r.ID, ArxivID: r.ArxivID, S2ID: r.S2ID, DOI: r.DOI, Title: r.Title,
		Authors: r.Authors, Year: r.Year, Venue: r.Venue, Abstract: r.Abstract,
		TLDR: r.TLDR, CitationCount: r.CitationCount, URL: r.URL,
		PDFURL: r.PDFURL, PublishedAt: r.PublishedAt, Source: r.Source,
	}
}

// -------------------------------------------------------------------- search

// SearchPapers queries arXiv, Semantic Scholar or both, ranked best-first.
func (a *App) SearchPapers(query string, source string, limit int) (PaperSearchResult, error) {
	return a.SearchPapersFiltered(query, source, limit, false)
}

// SearchPapersFiltered is SearchPapers with the "Published only" filter: when
// publishedOnly is set, preprints with no detectable venue are dropped.
//
// Semantic Scholar's public tier rate-limits aggressively, so an S2 failure is
// NOT fatal: the arXiv results are returned on their own, with venues read from
// the arXiv comments alone and a Note saying so.
func (a *App) SearchPapersFiltered(query string, source string, limit int, publishedOnly bool) (PaperSearchResult, error) {
	if err := a.papersInit(); err != nil {
		return PaperSearchResult{}, err
	}
	if strings.TrimSpace(query) == "" {
		return PaperSearchResult{}, errors.New("empty query")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		source = "all"
	}

	ctx, cancel := context.WithTimeout(a.baseCtx(), 90*time.Second)
	defer cancel()

	var (
		ax, s2        []papers.Paper
		total         int
		firstError    error
		s2Unavailable bool
	)
	if source == "all" || source == "arxiv" {
		ps, n, err := papersRT.arxiv.Search(ctx, query, limit, papers.SortRelevance)
		if err != nil {
			firstError = err
		} else {
			ax, total = ps, n
		}
	}
	if source == "all" || source == "s2" {
		ps, n, err := papersRT.s2.Search(ctx, query, limit)
		if err != nil {
			if source == "s2" {
				return PaperSearchResult{}, err
			}
			// Non-fatal: arXiv-only results, venues from the comments.
			s2Unavailable = true
		} else {
			s2 = ps
			if n > total {
				total = n
			}
		}
	}
	if len(ax) == 0 && len(s2) == 0 && firstError != nil {
		return PaperSearchResult{}, firstError
	}
	prefs := venuePrefs()
	merged := papers.SortByScore(papers.Merge(ax, s2), prefs)
	if publishedOnly {
		merged = papers.FilterPublished(merged)
		total = len(merged)
	}
	if total < len(merged) {
		total = len(merged)
	}
	res := PaperSearchResult{Papers: toPapers(merged), Total: total}
	if s2Unavailable && len(ax) > 0 {
		res.Note = "Semantic Scholar is rate-limited — venues were read from " +
			"the arXiv comments only, so some published papers may show as preprints."
	}
	return res, nil
}

// GetPaper resolves one paper by id ("arxiv:…" or "s2:…"), preferring the
// saved copy and falling back to the source API.
func (a *App) GetPaper(id string) (Paper, error) {
	if err := a.papersInit(); err != nil {
		return Paper{}, err
	}
	if lp, ok, err := a.st.LibraryPaperByID(id); err == nil && ok {
		return toLibraryPaper(lp).Paper, nil
	}
	p, err := a.fetchPaper(id)
	if err != nil {
		return Paper{}, err
	}
	return toPaper(p), nil
}

// fetchPaper resolves an id against the source API.
func (a *App) fetchPaper(id string) (papers.Paper, error) {
	ctx, cancel := context.WithTimeout(a.baseCtx(), 60*time.Second)
	defer cancel()

	kind, rest := papers.SplitID(id)
	switch kind {
	case "arxiv":
		p, err := papersRT.arxiv.ByID(ctx, rest)
		if err != nil {
			return papers.Paper{}, err
		}
		// Best-effort enrichment (TLDR, citation count); never fatal.
		if s2p, e := papersRT.s2.Paper(ctx, "arXiv:"+rest); e == nil {
			p = papers.Merge([]papers.Paper{p}, []papers.Paper{s2p})[0]
		}
		return p, nil
	case "s2":
		return papersRT.s2.Paper(ctx, rest)
	default:
		return papers.Paper{}, fmt.Errorf("unknown paper id %q", id)
	}
}

// ------------------------------------------------------------------ library

// AddPaperToLibrary saves a paper with status "toread".
func (a *App) AddPaperToLibrary(p Paper) (LibraryPaper, error) {
	if err := a.papersInit(); err != nil {
		return LibraryPaper{}, err
	}
	if strings.TrimSpace(p.ID) == "" {
		return LibraryPaper{}, errors.New("paper has no id")
	}
	if existing, ok, err := a.st.LibraryPaperByID(p.ID); err == nil && ok {
		return toLibraryPaper(existing), nil
	}
	row := fromLibraryPaper(LibraryPaper{Paper: p, Status: "toread"})
	row.AddedAt = nowRFC3339()
	if err := a.st.PutLibraryPaper(row); err != nil {
		return LibraryPaper{}, err
	}
	saved, _, err := a.st.LibraryPaperByID(p.ID)
	if err != nil {
		return LibraryPaper{}, err
	}
	a.papersChanged()
	return toLibraryPaper(saved), nil
}

// RemovePaperFromLibrary forgets a paper. The downloaded PDF is left on disk.
func (a *App) RemovePaperFromLibrary(id string) error {
	if err := a.papersInit(); err != nil {
		return err
	}
	if err := a.st.DeleteLibraryPaper(id); err != nil {
		return err
	}
	a.papersChanged()
	return nil
}

// GetLibrary lists saved papers; status "" or "all" returns every status.
func (a *App) GetLibrary(status string) ([]LibraryPaper, error) {
	if err := a.papersInit(); err != nil {
		return nil, err
	}
	if strings.EqualFold(strings.TrimSpace(status), "all") {
		status = ""
	}
	rows, err := a.st.Library(status)
	if err != nil {
		return nil, err
	}
	out := make([]LibraryPaper, 0, len(rows))
	for _, r := range rows {
		out = append(out, toLibraryPaper(r))
	}
	return out, nil
}

// UpdateLibraryPaper writes back reading state. Moving to "done" stamps ReadAt.
func (a *App) UpdateLibraryPaper(lp LibraryPaper) (LibraryPaper, error) {
	if err := a.papersInit(); err != nil {
		return LibraryPaper{}, err
	}
	cur, ok, err := a.st.LibraryPaperByID(lp.ID)
	if err != nil {
		return LibraryPaper{}, err
	}
	if !ok {
		return LibraryPaper{}, fmt.Errorf("paper %s is not in the library", lp.ID)
	}
	row := fromLibraryPaper(lp)
	row.AddedAt = cur.AddedAt
	if row.Status == "" {
		row.Status = cur.Status
	}
	if row.Stars < 0 {
		row.Stars = 0
	}
	if row.Stars > 5 {
		row.Stars = 5
	}
	if row.Status == "done" && row.ReadAt == "" {
		row.ReadAt = nowRFC3339()
	}
	if row.Status != "done" {
		row.ReadAt = ""
	}
	if err := a.st.PutLibraryPaper(row); err != nil {
		return LibraryPaper{}, err
	}
	saved, _, err := a.st.LibraryPaperByID(lp.ID)
	if err != nil {
		return LibraryPaper{}, err
	}
	a.papersChanged()
	return toLibraryPaper(saved), nil
}

// DownloadPaperPDF fetches the PDF into <SyncDir>/Papers/ and registers it as a
// `files` row under the synthetic "Papers" course (id -1) so the Study panel,
// full-text search and the chat panel all work on it.
func (a *App) DownloadPaperPDF(id string) (LibraryPaper, error) {
	if err := a.papersInit(); err != nil {
		return LibraryPaper{}, err
	}
	lp, err := a.ensureLibrary(id)
	if err != nil {
		return LibraryPaper{}, err
	}
	ctx, cancel := context.WithTimeout(a.baseCtx(), 10*time.Minute)
	defer cancel()
	out, err := a.downloadPaper(ctx, lp)
	if err != nil {
		return LibraryPaper{}, err
	}
	a.papersChanged()
	return toLibraryPaper(out), nil
}

// ensureLibrary returns the saved row for id, saving it first if needed.
func (a *App) ensureLibrary(id string) (store.LibraryPaper, error) {
	if lp, ok, err := a.st.LibraryPaperByID(id); err != nil {
		return store.LibraryPaper{}, err
	} else if ok {
		return lp, nil
	}
	p, err := a.fetchPaper(id)
	if err != nil {
		return store.LibraryPaper{}, err
	}
	if _, err := a.AddPaperToLibrary(toPaper(p)); err != nil {
		return store.LibraryPaper{}, err
	}
	lp, _, err := a.st.LibraryPaperByID(id)
	return lp, err
}

// downloadPaper does the work behind DownloadPaperPDF; it is also called by
// StartPaperSummary and the chat panel when a paper has no local file yet.
func (a *App) downloadPaper(ctx context.Context, lp store.LibraryPaper) (store.LibraryPaper, error) {
	if lp.LocalPath != "" {
		if _, err := os.Stat(lp.LocalPath); err == nil && lp.FileID != 0 {
			return lp, nil
		}
	}
	dir := filepath.Join(a.settings().SyncDir, "Papers")
	p := libraryPaperToPapers(lp)
	if p.PDFURL == "" {
		// A saved row may predate the PDF link; refresh from the source.
		if fresh, err := a.fetchPaper(lp.ID); err == nil {
			p.PDFURL = fresh.PDFURL
		}
	}
	path, size, err := papers.DownloadPDF(ctx, papersRT.http, p, dir)
	if err != nil {
		return lp, err
	}

	fileID, ok, err := a.st.PaperFileIDByPath(path)
	if err != nil {
		return lp, err
	}
	if !ok {
		if lp.FileID != 0 {
			fileID = lp.FileID
		} else if fileID, err = a.st.NextPaperFileID(); err != nil {
			return lp, err
		}
	}
	now := nowRFC3339()
	rel := "Papers/" + filepath.Base(path)
	if err := a.st.UpsertFile(store.File{
		ID: fileID, CourseID: store.PapersCourseID, Name: filepath.Base(path),
		RelPath: rel, AbsPath: path, Size: size, ModifiedAt: now, UpdatedAt: now,
		Source: "papers", Module: "Papers", Origin: "paper:" + lp.ID,
		Synced: true, URL: p.URL, FirstSeenAt: now, LastChangedAt: now,
	}); err != nil {
		return lp, err
	}
	// Index the text so FTS search and the Study/chat fallbacks work.
	if index.Supported(path) {
		if text := index.Extract(path); strings.TrimSpace(text) != "" {
			_ = a.st.MarkIndexed(fileID, filepath.Base(path), text)
		}
	}

	lp.LocalPath = path
	lp.FileID = fileID
	if n := study.PageCount(path); n > 0 {
		lp.Pages = n
	}
	if err := a.st.PutLibraryPaper(lp); err != nil {
		return lp, err
	}
	out, _, err := a.st.LibraryPaperByID(lp.ID)
	return out, err
}

// ------------------------------------------------------- citations / recs

// GetCitations lists papers citing id, marked with their library state.
func (a *App) GetCitations(id string, limit int) ([]CitationLink, error) {
	return a.citationEdges(id, limit, true)
}

// GetReferences lists papers cited by id, marked with their library state.
func (a *App) GetReferences(id string, limit int) ([]CitationLink, error) {
	return a.citationEdges(id, limit, false)
}

func (a *App) citationEdges(id string, limit int, citing bool) ([]CitationLink, error) {
	if err := a.papersInit(); err != nil {
		return nil, err
	}
	s2id, err := a.s2IDFor(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.baseCtx(), 90*time.Second)
	defer cancel()

	var ps []papers.Paper
	if citing {
		ps, err = papersRT.s2.Citations(ctx, s2id, limit)
	} else {
		ps, err = papersRT.s2.References(ctx, s2id, limit)
	}
	if err != nil {
		return nil, err
	}
	lib, _ := a.st.LibraryIDs()
	out := make([]CitationLink, 0, len(ps))
	for _, p := range ps {
		l := CitationLink{Paper: toPaper(p)}
		if st, ok := lib[p.ID]; ok {
			l.InLibrary, l.Status = true, st
		} else if p.ArxivID != "" {
			if st, ok := lib[papers.ArxivPaperID(p.ArxivID)]; ok {
				l.InLibrary, l.Status = true, st
			}
		}
		out = append(out, l)
	}
	return out, nil
}

// s2IDFor maps a NUSSync paper id onto something the S2 API accepts.
func (a *App) s2IDFor(id string) (string, error) {
	kind, rest := papers.SplitID(id)
	switch kind {
	case "s2":
		return rest, nil
	case "arxiv":
		return "arXiv:" + rest, nil
	}
	if lp, ok, _ := a.st.LibraryPaperByID(id); ok {
		if lp.S2ID != "" {
			return lp.S2ID, nil
		}
		if lp.ArxivID != "" {
			return "arXiv:" + lp.ArxivID, nil
		}
	}
	return "", fmt.Errorf("unknown paper id %q", id)
}

// GetRecommendations suggests papers from the library's most recent additions.
// Falls back to an arXiv keyword search when Semantic Scholar is unavailable
// or the library is empty.
func (a *App) GetRecommendations(limit int) ([]Paper, error) {
	if err := a.papersInit(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 10
	}
	ctx, cancel := context.WithTimeout(a.baseCtx(), 90*time.Second)
	defer cancel()
	ps, _, err := a.recommendations(ctx, limit)
	if err != nil {
		return nil, err
	}
	return toPapers(papers.SortByScore(ps, venuePrefs())), nil
}

// recommendations returns suggestions and the reason line that describes where
// they came from (Semantic Scholar recommendations, or the arXiv fallback).
func (a *App) recommendations(ctx context.Context, limit int) ([]papers.Paper, string, error) {
	seeds := a.recommendationSeeds(5)
	if len(seeds) > 0 {
		if ps, err := papersRT.s2.Recommendations(ctx, seeds, limit); err == nil && len(ps) > 0 {
			return ps, papers.ReasonRecommended, nil
		}
	}
	// Fallback: newest arXiv papers matching the configured keywords.
	cfg := a.settings()
	q := papers.KeywordQuery(cfg.PaperKeywords)
	if q == "" {
		return nil, papers.ReasonKeywords, nil
	}
	ps, _, err := papersRT.arxiv.SearchQuery(ctx, q, limit, papers.SortSubmitted)
	return ps, papers.ReasonKeywords, err
}

// recommendationSeeds picks up to n S2 ids from the library, newest first.
func (a *App) recommendationSeeds(n int) []string {
	rows, err := a.st.Library("")
	if err != nil {
		return nil
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].AddedAt > rows[j].AddedAt })
	var out []string
	for _, r := range rows {
		switch {
		case r.S2ID != "":
			out = append(out, r.S2ID)
		case r.ArxivID != "":
			out = append(out, "arXiv:"+r.ArxivID)
		default:
			continue
		}
		if len(out) >= n {
			break
		}
	}
	return out
}

// -------------------------------------------------------------------- digest

// GetPaperDigest returns (building and caching if needed) the digest for a day.
// date is YYYY-MM-DD; "" means today.
func (a *App) GetPaperDigest(date string) (PaperDigest, error) {
	if err := a.papersInit(); err != nil {
		return PaperDigest{}, err
	}
	if strings.TrimSpace(date) == "" {
		date = todayLocal()
	}
	ctx, cancel := context.WithTimeout(a.baseCtx(), 3*time.Minute)
	defer cancel()
	d, err := a.buildDigest(ctx, date)
	if err != nil {
		return PaperDigest{}, err
	}
	return PaperDigest{Date: d.Date, Papers: toPapers(d.Papers), Reason: d.Reason}, nil
}

// buildDigest returns the cached digest for a date, building it on a miss.
func (a *App) buildDigest(ctx context.Context, date string) (papers.Digest, error) {
	if body, ok, err := a.st.PaperDigestGet(date); err == nil && ok {
		var d papers.Digest
		if json.Unmarshal([]byte(body), &d) == nil && len(d.Papers) > 0 {
			a.rememberDigest(d)
			return d, nil
		}
	}
	cfg := a.settings()

	// New submissions in the configured categories, newest first.
	q := papers.CategoryQuery(cfg.PaperCategories)
	kw := papers.KeywordQuery(cfg.PaperKeywords)
	switch {
	case q != "" && kw != "":
		q = q + " AND " + kw
	case q == "":
		q = kw
	}
	var fresh []papers.Paper
	if q != "" {
		ps, _, err := papersRT.arxiv.SearchQuery(ctx, q, 40, papers.SortSubmitted)
		if err != nil {
			return papers.Digest{}, err
		}
		fresh = ps
	}
	recs, recReason, _ := a.recommendations(ctx, 6)

	lib, _ := a.st.LibraryIDs()
	inLibrary := func(id string) bool { _, ok := lib[id]; return ok }

	d := papers.BuildDigest(date, fresh, cfg.PaperKeywords, recs, recReason,
		inLibrary, 5, 3, venuePrefs())
	if blob, err := json.Marshal(d); err == nil {
		_ = a.st.PaperDigestPut(date, string(blob))
	}
	a.rememberDigest(d)
	return d, nil
}

func (a *App) rememberDigest(d papers.Digest) {
	papersRT.mu.Lock()
	papersRT.lastDigest = d
	papersRT.mu.Unlock()
}

// SendPaperDigestNow builds today's digest and pushes it through the paper bot.
func (a *App) SendPaperDigestNow() error {
	if err := a.papersInit(); err != nil {
		return err
	}
	cfg := a.settings()
	chat := strings.TrimSpace(cfg.PaperTelegramChatID)
	if chat == "" {
		return errors.New("paper Telegram bot is not paired — open Settings ▸ Papers and pair it")
	}
	ctx, cancel := context.WithTimeout(a.baseCtx(), 3*time.Minute)
	defer cancel()
	d, err := a.buildDigest(ctx, todayLocal())
	if err != nil {
		return err
	}
	return a.paperTelegram().SendMessage(ctx, chat, papers.DigestMessage(d))
}

// savePaperN implements the bot's /save <n> against the last digest.
func (a *App) savePaperN(n int) (string, error) {
	papersRT.mu.Lock()
	d := papersRT.lastDigest
	papersRT.mu.Unlock()
	if len(d.Papers) == 0 {
		ctx, cancel := context.WithTimeout(a.baseCtx(), 3*time.Minute)
		defer cancel()
		var err error
		if d, err = a.buildDigest(ctx, todayLocal()); err != nil {
			return "", err
		}
	}
	if n < 1 || n > len(d.Papers) {
		return "", fmt.Errorf("there is no paper %d in the last digest (%d listed)", n, len(d.Papers))
	}
	lp, err := a.AddPaperToLibrary(toPaper(d.Papers[n-1]))
	if err != nil {
		return "", err
	}
	return "✅ Saved <b>" + telegram.EscapeHTML(lp.Title) + "</b> to your library.", nil
}

// readingMessage implements the bot's /reading.
func (a *App) readingMessage() (string, error) {
	rows, err := a.st.Library("reading")
	if err != nil {
		return "", err
	}
	titles := make([]string, 0, len(rows))
	pages := make([][2]int, 0, len(rows))
	for _, r := range rows {
		titles = append(titles, r.Title)
		pages = append(pages, [2]int{r.Page, r.Pages})
	}
	return papers.ReadingMessage(titles, pages), nil
}

// -------------------------------------------------------------------- export

// ExportBibTeX renders BibTeX for the given paper ids (empty = whole library).
func (a *App) ExportBibTeX(ids []string) (string, error) {
	if err := a.papersInit(); err != nil {
		return "", err
	}
	var ps []papers.Paper
	if len(ids) == 0 {
		rows, err := a.st.Library("")
		if err != nil {
			return "", err
		}
		for _, r := range rows {
			ps = append(ps, libraryPaperToPapers(r))
		}
	} else {
		for _, id := range ids {
			if r, ok, err := a.st.LibraryPaperByID(id); err == nil && ok {
				ps = append(ps, libraryPaperToPapers(r))
				continue
			}
			p, err := a.fetchPaper(id)
			if err != nil {
				return "", err
			}
			ps = append(ps, p)
		}
	}
	if len(ps) == 0 {
		return "", nil
	}
	return papers.BibTeX(ps), nil
}

// OpenScholar opens a Google Scholar search in the browser. Scholar has no API
// and is never scraped — this is the only Scholar integration.
func (a *App) OpenScholar(query string) error {
	if strings.TrimSpace(query) == "" {
		return errors.New("empty query")
	}
	return openWithShell(papers.ScholarURL(query))
}

// ------------------------------------------------------------------ summary

// StartPaperSummary queues a Claude key-points summary of a paper. The PDF is
// downloaded automatically first when the library has no local copy.
func (a *App) StartPaperSummary(paperID string, model string) (string, error) {
	if err := a.papersInit(); err != nil {
		return "", err
	}
	if err := a.studyInit(); err != nil {
		return "", err
	}
	lp, err := a.ensureLibrary(paperID)
	if err != nil {
		return "", err
	}

	j := a.newStudyJob("paper_summary", nil, model)
	err = a.enqueueStudy(j, func(ctx context.Context, a *App, id string) (float64, error) {
		row := lp
		if row.LocalPath == "" || row.FileID == 0 {
			a.setStudyProgress(id, "Downloading the PDF…")
			row, err = a.downloadPaper(ctx, row)
			if err != nil {
				return 0, err
			}
			a.papersChanged()
		}
		// Papers are usually clean arXiv PDFs, but the resolver is applied here
		// too so every Claude path goes through the same readable-copy rules.
		p := study.Resolve(row.LocalPath, row.FileID, func() string {
			txt, _ := a.st.StudyFileText(row.FileID)
			return txt
		})
		src := study.Source{
			Name:     row.Title,
			Path:     p.Path,
			Pages:    row.Pages,
			Readable: p.Readable,
			Note:     p.Note,
		}
		if !src.Readable && row.FileID != 0 {
			txt, _ := a.st.StudyFileText(row.FileID)
			src.Text = study.ClampText(txt)
		}
		dir := filepath.Dir(src.Path)
		o := a.studyOpts(j.Model, dir, []string{dir}, study.NeedsRead([]study.Source{src}))

		a.setStudyProgress(id, "Reading "+row.Title+"…")
		res, err := study.Run(ctx, study.PaperSummaryPrompt(src), o,
			func(t string) { a.setStudyProgress(id, t) })
		if err != nil {
			return res.CostUSD, err
		}
		if res.IsError {
			return res.CostUSD, errors.New(strings.TrimSpace(res.Text))
		}
		md := strings.TrimSpace(res.Text)
		if md == "" {
			return res.CostUSD, errors.New("Claude returned an empty summary")
		}
		if err := a.st.PutPaperSummary(store.PaperSummary{
			PaperID: row.ID, Model: j.Model, Markdown: md, CreatedAt: nowRFC3339(),
		}); err != nil {
			return res.CostUSD, err
		}
		a.papersChanged()
		return res.CostUSD, nil
	})
	if err != nil {
		return j.ID, err
	}
	return j.ID, nil
}

// GetPaperSummary returns the cached summary, or a zero value.
func (a *App) GetPaperSummary(paperID string) (PaperSummary, error) {
	if err := a.papersInit(); err != nil {
		return PaperSummary{}, err
	}
	x, ok, err := a.st.PaperSummaryFor(paperID)
	if err != nil || !ok {
		return PaperSummary{}, err
	}
	return PaperSummary{PaperID: x.PaperID, Markdown: x.Markdown,
		CreatedAt: x.CreatedAt, Model: x.Model}, nil
}

// ----------------------------------------------------------- paper telegram

// GetPaperTelegramStatus reports the paper bot's pairing state.
func (a *App) GetPaperTelegramStatus() TelegramStatus {
	cfg := a.settings()
	st := TelegramStatus{ChatID: cfg.PaperTelegramChatID}
	if strings.TrimSpace(cfg.PaperTelegramToken) == "" {
		return st
	}
	st.Configured = strings.TrimSpace(cfg.PaperTelegramChatID) != ""
	ctx, cancel := context.WithTimeout(a.baseCtx(), 10*time.Second)
	defer cancel()
	if b, err := a.paperTelegram().GetMe(ctx); err == nil && b.Username != "" {
		st.BotName = "@" + b.Username
	}
	return st
}

// PairPaperTelegram waits up to 60s for a /start on the paper bot.
func (a *App) PairPaperTelegram() (string, error) {
	if err := a.papersInit(); err != nil {
		return "", err
	}
	cfg := a.settings()
	if strings.TrimSpace(cfg.PaperTelegramToken) == "" {
		return "", errors.New("no paper Telegram bot token configured " +
			"(set PAPER_TRACKER_TELEGRAM_TOKEN in .env)")
	}
	ctx, cancel := context.WithTimeout(a.baseCtx(), 65*time.Second)
	defer cancel()

	// One getUpdates consumer per bot token: when the loop runs, wait on it.
	if bot := paperBot(); bot != nil && bot.Running() {
		if id := bot.AwaitPair(ctx, 60*time.Second); id != "" {
			return a.savePairedPaperChat(id)
		}
		return "", errors.New("telegram: no /start received on the paper bot")
	}
	tg := a.paperTelegram()
	if id, err := tg.FindExistingChat(ctx); err == nil && id != "" {
		return a.savePairedPaperChat(id)
	}
	id, err := tg.AwaitStart(ctx, 60*time.Second)
	if err != nil {
		return "", err
	}
	return a.savePairedPaperChat(id)
}

func (a *App) savePairedPaperChat(chatID string) (string, error) {
	a.mu.Lock()
	a.cfg.PaperTelegramChatID = chatID
	cfg := a.cfg
	a.mu.Unlock()

	if err := config.Save(cfg); err != nil {
		return "", err
	}
	if bot := paperBot(); bot != nil {
		bot.SetConfig(cfg, telegram.New(cfg.PaperTelegramToken))
	}
	return chatID, nil
}

// SendPaperTestTelegram sends a confirmation to the paired paper chat.
func (a *App) SendPaperTestTelegram() error {
	cfg := a.settings()
	if strings.TrimSpace(cfg.PaperTelegramChatID) == "" {
		return errors.New("paper bot not paired: send /start to it, then click Pair")
	}
	ctx, cancel := context.WithTimeout(a.baseCtx(), 30*time.Second)
	defer cancel()
	return a.paperTelegram().SendMessage(ctx, cfg.PaperTelegramChatID,
		"📄 <b>NUSSync papers</b> is connected. Daily digests will arrive here.")
}

// papersApplySettings re-points the paper bot after SaveSettings.
func (a *App) papersApplySettings(cfg config.Settings) {
	setVenuePrefs(cfg)
	if bot := paperBot(); bot != nil {
		bot.SetConfig(cfg, telegram.New(cfg.PaperTelegramToken))
	}
}
