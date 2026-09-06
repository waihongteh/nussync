package main

// Papers/chat contract types. JSON field names are the Go field names exactly
// (no json tags) — do not rename. See docs/CONTRACT_PAPERS.md.

// Paper is one work from arXiv or Semantic Scholar.
type Paper struct {
	ID            string // "arxiv:<id without version>" | "s2:<paperId>"
	ArxivID       string
	S2ID          string
	DOI           string
	Title         string
	Authors       []string
	Year          int
	Venue         string
	Abstract      string
	TLDR          string
	CitationCount int
	URL           string
	PDFURL        string
	PublishedAt   string
	Source        string // "arxiv" | "s2"

	// Venue ranking (internal/papers/venue.go). Derived on every read, never
	// stored: only Venue itself survives in papers_library.
	VenueTier  int    // 2 top venue, 1 published, 0 preprint/unknown
	VenueShort string // badge label, e.g. "NeurIPS 2025" or "preprint"
	Published  bool   // VenueTier >= 1
}

// LibraryPaper is a saved paper plus its reading state. Paper is embedded, so
// its fields appear flattened in JSON/TS exactly as on Paper.
type LibraryPaper struct {
	Paper
	Status    string // "toread" | "reading" | "done"
	Page      int
	Pages     int
	Stars     int // 0-5
	Tags      []string
	Notes     string
	KeyIdea   string
	LocalPath string
	AddedAt   string
	UpdatedAt string
	ReadAt    string
	FileID    int // synthetic files row, 0 until the PDF is downloaded
}

// PaperSearchResult is one page of search results.
type PaperSearchResult struct {
	Papers []Paper
	Total  int
	// Note explains a degraded result, e.g. Semantic Scholar being rate
	// limited so venues could only come from arXiv comments. "" when fine.
	Note string
}

// CitationLink is a citing/cited paper with its library state.
type CitationLink struct {
	Paper
	InLibrary bool
	Status    string // library status, "" when not saved
}

// PaperDigest is one day's paper-of-the-day selection. Reason[i] explains
// Papers[i].
type PaperDigest struct {
	Date   string // YYYY-MM-DD
	Papers []Paper
	Reason []string
}

// PaperSummary is a cached Claude summary of one paper.
type PaperSummary struct {
	PaperID   string
	Markdown  string
	CreatedAt string
	Model     string
}

// ChatSession is one in-app Claude conversation, optionally bound to a synced
// file (FileID) or a library paper (PaperID).
type ChatSession struct {
	ID              string
	FileID          int
	PaperID         string
	Title           string
	ClaudeSessionID string
	Model           string
	CreatedAt       string
	UpdatedAt       string
}

// ChatMessage is one turn of a chat session.
type ChatMessage struct {
	ID        int
	SessionID string
	Role      string // "user" | "assistant"
	Text      string
	CreatedAt string
}

// ChatDelta is the `chat:delta` event payload.
type ChatDelta struct {
	SessionID string
	Text      string
}

// ChatDone is the `chat:done` event payload. Error is "" on success.
type ChatDone struct {
	SessionID string
	JobID     string
	Error     string
}
