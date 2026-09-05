package store

import (
	"database/sql"
	"strings"
	"time"
)

// Papers tables (and the chat tables, which the Papers/Study chat panel share).
// Created lazily by MigratePapers on first use of the feature, exactly like
// MigrateStudy — a user who never opens the Papers view never pays for them.
const papersSchema = `
CREATE TABLE IF NOT EXISTS papers_library (
  id             TEXT PRIMARY KEY,          -- "arxiv:2401.00001" | "s2:<id>"
  arxiv_id       TEXT NOT NULL DEFAULT '',
  s2_id          TEXT NOT NULL DEFAULT '',
  doi            TEXT NOT NULL DEFAULT '',
  title          TEXT NOT NULL DEFAULT '',
  authors        TEXT NOT NULL DEFAULT '',  -- unit-separator joined
  year           INTEGER NOT NULL DEFAULT 0,
  venue          TEXT NOT NULL DEFAULT '',
  abstract       TEXT NOT NULL DEFAULT '',
  tldr           TEXT NOT NULL DEFAULT '',
  citation_count INTEGER NOT NULL DEFAULT 0,
  url            TEXT NOT NULL DEFAULT '',
  pdf_url        TEXT NOT NULL DEFAULT '',
  published_at   TEXT NOT NULL DEFAULT '',
  source         TEXT NOT NULL DEFAULT '',
  status         TEXT NOT NULL DEFAULT 'toread',
  page           INTEGER NOT NULL DEFAULT 0,
  pages          INTEGER NOT NULL DEFAULT 0,
  stars          INTEGER NOT NULL DEFAULT 0,
  tags           TEXT NOT NULL DEFAULT '',
  notes          TEXT NOT NULL DEFAULT '',
  key_idea       TEXT NOT NULL DEFAULT '',
  local_path     TEXT NOT NULL DEFAULT '',
  added_at       TEXT NOT NULL DEFAULT '',
  updated_at     TEXT NOT NULL DEFAULT '',
  read_at        TEXT NOT NULL DEFAULT '',
  file_id        INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_papers_status ON papers_library(status, updated_at DESC);

-- 24h cache of Semantic Scholar GET bodies, keyed by full URL.
CREATE TABLE IF NOT EXISTS papers_cache (
  key        TEXT PRIMARY KEY,
  body       TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL DEFAULT ''
);

-- One built digest per day (JSON blob of the contract type).
CREATE TABLE IF NOT EXISTS papers_digests (
  date       TEXT PRIMARY KEY,
  body       TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS paper_summaries (
  paper_id   TEXT NOT NULL,
  model      TEXT NOT NULL DEFAULT '',
  markdown   TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (paper_id, model)
);

-- In-app Claude chat. A session is bound to a file, a paper, or neither.
CREATE TABLE IF NOT EXISTS study_chats (
  id                TEXT PRIMARY KEY,
  file_id           INTEGER NOT NULL DEFAULT 0,
  paper_id          TEXT NOT NULL DEFAULT '',
  title             TEXT NOT NULL DEFAULT '',
  claude_session_id TEXT NOT NULL DEFAULT '',
  model             TEXT NOT NULL DEFAULT '',
  created_at        TEXT NOT NULL DEFAULT '',
  updated_at        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_study_chats_ctx ON study_chats(file_id, paper_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS study_chat_messages (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  role       TEXT NOT NULL DEFAULT 'user',
  text       TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_study_chat_messages ON study_chat_messages(session_id, id);
`

// MigratePapers creates the papers_*, paper_summaries and chat tables.
// Idempotent.
func (s *Store) MigratePapers() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(papersSchema)
	return err
}

const listSep = "\x1f" // unit separator: never appears in a title or a name

func joinList(xs []string) string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return strings.Join(out, listSep)
}

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, listSep)
}

// ------------------------------------------------------------------ library

// LibraryPaper is one saved paper with its reading state.
type LibraryPaper struct {
	ID            string
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
	Source        string

	Status    string // toread | reading | done
	Page      int
	Pages     int
	Stars     int
	Tags      []string
	Notes     string
	KeyIdea   string
	LocalPath string
	AddedAt   string
	UpdatedAt string
	ReadAt    string
	FileID    int
}

const paperSelect = `SELECT id, arxiv_id, s2_id, doi, title, authors, year, venue,
	abstract, tldr, citation_count, url, pdf_url, published_at, source,
	status, page, pages, stars, tags, notes, key_idea, local_path,
	added_at, updated_at, read_at, file_id FROM papers_library`

func scanPapers(rows *sql.Rows) ([]LibraryPaper, error) {
	var out []LibraryPaper
	for rows.Next() {
		var p LibraryPaper
		var authors, tags string
		if err := rows.Scan(&p.ID, &p.ArxivID, &p.S2ID, &p.DOI, &p.Title, &authors,
			&p.Year, &p.Venue, &p.Abstract, &p.TLDR, &p.CitationCount, &p.URL,
			&p.PDFURL, &p.PublishedAt, &p.Source, &p.Status, &p.Page, &p.Pages,
			&p.Stars, &tags, &p.Notes, &p.KeyIdea, &p.LocalPath, &p.AddedAt,
			&p.UpdatedAt, &p.ReadAt, &p.FileID); err != nil {
			return nil, err
		}
		p.Authors = splitList(authors)
		p.Tags = splitList(tags)
		out = append(out, p)
	}
	return out, rows.Err()
}

// PutLibraryPaper inserts or updates a saved paper. Reading state (status,
// page, stars, tags, notes, key idea, local path, file id) is written as given.
func (s *Store) PutLibraryPaper(p LibraryPaper) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.AddedAt == "" {
		p.AddedAt = time.Now().UTC().Format(time.RFC3339)
	}
	p.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if p.Status == "" {
		p.Status = "toread"
	}
	_, err := s.db.Exec(`
		INSERT INTO papers_library(id, arxiv_id, s2_id, doi, title, authors, year,
			venue, abstract, tldr, citation_count, url, pdf_url, published_at, source,
			status, page, pages, stars, tags, notes, key_idea, local_path,
			added_at, updated_at, read_at, file_id)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			arxiv_id=excluded.arxiv_id, s2_id=excluded.s2_id, doi=excluded.doi,
			title=excluded.title, authors=excluded.authors, year=excluded.year,
			venue=excluded.venue, abstract=excluded.abstract, tldr=excluded.tldr,
			citation_count=excluded.citation_count, url=excluded.url,
			pdf_url=excluded.pdf_url, published_at=excluded.published_at,
			source=excluded.source, status=excluded.status, page=excluded.page,
			pages=excluded.pages, stars=excluded.stars, tags=excluded.tags,
			notes=excluded.notes, key_idea=excluded.key_idea,
			local_path=excluded.local_path, updated_at=excluded.updated_at,
			read_at=excluded.read_at, file_id=excluded.file_id`,
		p.ID, p.ArxivID, p.S2ID, p.DOI, p.Title, joinList(p.Authors), p.Year,
		p.Venue, p.Abstract, p.TLDR, p.CitationCount, p.URL, p.PDFURL,
		p.PublishedAt, p.Source, p.Status, p.Page, p.Pages, p.Stars,
		joinList(p.Tags), p.Notes, p.KeyIdea, p.LocalPath, p.AddedAt,
		p.UpdatedAt, p.ReadAt, p.FileID)
	return err
}

// LibraryPaperByID loads one saved paper.
func (s *Store) LibraryPaperByID(id string) (LibraryPaper, bool, error) {
	rows, err := s.db.Query(paperSelect+` WHERE id=?`, id)
	if err != nil {
		return LibraryPaper{}, false, err
	}
	defer rows.Close()
	ps, err := scanPapers(rows)
	if err != nil || len(ps) == 0 {
		return LibraryPaper{}, false, err
	}
	return ps[0], true, nil
}

// Library lists saved papers, newest update first. status "" = every status.
func (s *Store) Library(status string) ([]LibraryPaper, error) {
	q := paperSelect + ` ORDER BY updated_at DESC, added_at DESC`
	var (
		rows *sql.Rows
		err  error
	)
	if strings.TrimSpace(status) == "" {
		rows, err = s.db.Query(q)
	} else {
		rows, err = s.db.Query(paperSelect+
			` WHERE status=? ORDER BY updated_at DESC, added_at DESC`, status)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPapers(rows)
}

// DeleteLibraryPaper removes a saved paper (the PDF on disk is left alone).
func (s *Store) DeleteLibraryPaper(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM papers_library WHERE id=?`, id)
	return err
}

// LibraryIDs returns the set of saved paper ids.
func (s *Store) LibraryIDs() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT id, status FROM papers_library`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, st string
		if err := rows.Scan(&id, &st); err != nil {
			return nil, err
		}
		out[id] = st
	}
	return out, rows.Err()
}

// -------------------------------------------------------------------- cache

// PapersCacheGet returns a cached body younger than maxAge.
func (s *Store) PapersCacheGet(key string, maxAge time.Duration) (string, bool) {
	var body, at string
	err := s.db.QueryRow(`SELECT body, fetched_at FROM papers_cache WHERE key=?`, key).
		Scan(&body, &at)
	if err != nil {
		return "", false
	}
	t, err := time.Parse(time.RFC3339, at)
	if err != nil || time.Since(t) > maxAge {
		return "", false
	}
	return body, true
}

// PapersCachePut stores a response body.
func (s *Store) PapersCachePut(key, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.db.Exec(`
		INSERT INTO papers_cache(key, body, fetched_at) VALUES(?,?,?)
		ON CONFLICT(key) DO UPDATE SET body=excluded.body, fetched_at=excluded.fetched_at`,
		key, body, time.Now().UTC().Format(time.RFC3339))
}

// ------------------------------------------------------------------ digests

// PaperDigestGet loads the stored JSON blob for a date.
func (s *Store) PaperDigestGet(date string) (string, bool, error) {
	var body string
	err := s.db.QueryRow(`SELECT body FROM papers_digests WHERE date=?`, date).Scan(&body)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return body, err == nil, err
}

// PaperDigestPut stores the JSON blob for a date.
func (s *Store) PaperDigestPut(date, body string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`
		INSERT INTO papers_digests(date, body, created_at) VALUES(?,?,?)
		ON CONFLICT(date) DO UPDATE SET body=excluded.body, created_at=excluded.created_at`,
		date, body, time.Now().UTC().Format(time.RFC3339))
	return err
}

// ---------------------------------------------------------------- summaries

// PaperSummary is a cached Claude summary of one paper.
type PaperSummary struct {
	PaperID   string
	Model     string
	Markdown  string
	CreatedAt string
}

// PutPaperSummary stores (replacing) the summary for a paper+model.
func (s *Store) PutPaperSummary(x PaperSummary) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`
		INSERT INTO paper_summaries(paper_id, model, markdown, created_at)
		VALUES(?,?,?,?)
		ON CONFLICT(paper_id, model) DO UPDATE SET
			markdown=excluded.markdown, created_at=excluded.created_at`,
		x.PaperID, x.Model, x.Markdown, x.CreatedAt)
	return err
}

// PaperSummaryFor returns the newest summary of a paper across models.
func (s *Store) PaperSummaryFor(paperID string) (PaperSummary, bool, error) {
	var x PaperSummary
	err := s.db.QueryRow(`SELECT paper_id, model, markdown, created_at
		FROM paper_summaries WHERE paper_id=? ORDER BY created_at DESC LIMIT 1`, paperID).
		Scan(&x.PaperID, &x.Model, &x.Markdown, &x.CreatedAt)
	if err == sql.ErrNoRows {
		return PaperSummary{}, false, nil
	}
	return x, err == nil, err
}

// --------------------------------------------------------------------- chat

// ChatSession is one Claude conversation, optionally bound to a file or paper.
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

// ChatMessage is one turn in a chat session.
type ChatMessage struct {
	ID        int
	SessionID string
	Role      string // user | assistant
	Text      string
	CreatedAt string
}

// PutChatSession inserts or updates a session row.
func (s *Store) PutChatSession(c ChatSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`
		INSERT INTO study_chats(id, file_id, paper_id, title, claude_session_id,
			model, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			file_id=excluded.file_id, paper_id=excluded.paper_id,
			title=excluded.title, claude_session_id=excluded.claude_session_id,
			model=excluded.model, updated_at=excluded.updated_at`,
		c.ID, c.FileID, c.PaperID, c.Title, c.ClaudeSessionID, c.Model,
		c.CreatedAt, c.UpdatedAt)
	return err
}

const chatSelect = `SELECT id, file_id, paper_id, title, claude_session_id, model,
	created_at, updated_at FROM study_chats`

func scanChats(rows *sql.Rows) ([]ChatSession, error) {
	var out []ChatSession
	for rows.Next() {
		var c ChatSession
		if err := rows.Scan(&c.ID, &c.FileID, &c.PaperID, &c.Title,
			&c.ClaudeSessionID, &c.Model, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ChatSessionByID loads one session.
func (s *Store) ChatSessionByID(id string) (ChatSession, bool, error) {
	rows, err := s.db.Query(chatSelect+` WHERE id=?`, id)
	if err != nil {
		return ChatSession{}, false, err
	}
	defer rows.Close()
	cs, err := scanChats(rows)
	if err != nil || len(cs) == 0 {
		return ChatSession{}, false, err
	}
	return cs[0], true, nil
}

// ChatSessions lists sessions for a context, newest first. fileID 0 and
// paperID "" together mean "every session".
func (s *Store) ChatSessions(fileID int, paperID string) ([]ChatSession, error) {
	q := chatSelect
	var (
		rows *sql.Rows
		err  error
	)
	switch {
	case fileID != 0:
		rows, err = s.db.Query(q+` WHERE file_id=? ORDER BY updated_at DESC, id DESC`, fileID)
	case strings.TrimSpace(paperID) != "":
		rows, err = s.db.Query(q+` WHERE paper_id=? ORDER BY updated_at DESC, id DESC`, paperID)
	default:
		rows, err = s.db.Query(q + ` ORDER BY updated_at DESC, id DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChats(rows)
}

// DeleteChatSession removes a session and its messages.
func (s *Store) DeleteChatSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM study_chat_messages WHERE session_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM study_chats WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// InsertChatMessage appends a message and stamps the session as updated.
func (s *Store) InsertChatMessage(m ChatMessage) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.CreatedAt == "" {
		m.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	res, err := s.db.Exec(`INSERT INTO study_chat_messages(session_id, role, text, created_at)
		VALUES(?,?,?,?)`, m.SessionID, m.Role, m.Text, m.CreatedAt)
	if err != nil {
		return 0, err
	}
	_, _ = s.db.Exec(`UPDATE study_chats SET updated_at=? WHERE id=?`, m.CreatedAt, m.SessionID)
	id, err := res.LastInsertId()
	return int(id), err
}

// ChatMessages lists a session's messages oldest first.
func (s *Store) ChatMessages(sessionID string) ([]ChatMessage, error) {
	rows, err := s.db.Query(`SELECT id, session_id, role, text, created_at
		FROM study_chat_messages WHERE session_id=? ORDER BY id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Text, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SetChatClaudeSession records the CLI session id used for --resume.
func (s *Store) SetChatClaudeSession(id, claudeSessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE study_chats SET claude_session_id=?, updated_at=? WHERE id=?`,
		claudeSessionID, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// ------------------------------------------------- synthetic "Papers" course

// PapersCourseID is the synthetic course row that owns downloaded paper PDFs,
// so they get a `files` row (and therefore FTS, Study and chat) without
// belonging to any Canvas course. Negative ids are never returned by Canvas,
// and the sync engine only ever touches courses Canvas listed.
const PapersCourseID = -1

// EnsurePapersCourse creates the synthetic course row once.
func (s *Store) EnsurePapersCourse() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`
		INSERT INTO courses(id, code, name, term, enabled, last_synced)
		VALUES(?,?,?,?,0,'')
		ON CONFLICT(id) DO NOTHING`,
		PapersCourseID, "Papers", "Research papers", "")
	return err
}

// NextPaperFileID allocates the next synthetic (negative) files.id. Canvas file
// ids are always positive, so the two id spaces never collide.
func (s *Store) NextPaperFileID() (int, error) {
	var min sql.NullInt64
	if err := s.db.QueryRow(`SELECT MIN(id) FROM files`).Scan(&min); err != nil {
		return 0, err
	}
	next := -1000
	if min.Valid && int(min.Int64) <= next {
		next = int(min.Int64) - 1
	}
	return next, nil
}

// PaperFileIDByPath finds an existing synthetic file row for a local path.
func (s *Store) PaperFileIDByPath(absPath string) (int, bool, error) {
	var id int
	err := s.db.QueryRow(`SELECT id FROM files WHERE abs_path=? AND course_id=?`,
		absPath, PapersCourseID).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	return id, err == nil, err
}
