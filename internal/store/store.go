// Package store is the SQLite persistence layer (pure-Go modernc driver).
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store wraps the application database.
type Store struct {
	db *sql.DB
	mu sync.Mutex // serialises writes; SQLite single-writer
}

// Open opens (creating if needed) the database at path and runs migrations.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// DB exposes the underlying handle (tests, ad-hoc queries).
func (s *Store) DB() *sql.DB { return s.db }

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

const schema = `
CREATE TABLE IF NOT EXISTS courses (
  id          INTEGER PRIMARY KEY,
  code        TEXT NOT NULL DEFAULT '',
  name        TEXT NOT NULL DEFAULT '',
  term        TEXT NOT NULL DEFAULT '',
  enabled     INTEGER NOT NULL DEFAULT 1,
  last_synced TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS files (
  id           INTEGER PRIMARY KEY,
  course_id    INTEGER NOT NULL,
  name         TEXT NOT NULL DEFAULT '',
  rel_path     TEXT NOT NULL DEFAULT '',
  abs_path     TEXT NOT NULL DEFAULT '',
  size         INTEGER NOT NULL DEFAULT 0,
  modified_at  TEXT NOT NULL DEFAULT '',
  updated_at   TEXT NOT NULL DEFAULT '',
  source       TEXT NOT NULL DEFAULT 'files',
  module       TEXT NOT NULL DEFAULT '',
  synced       INTEGER NOT NULL DEFAULT 0,
  content_hash TEXT NOT NULL DEFAULT '',
  indexed      INTEGER NOT NULL DEFAULT 0,
  url          TEXT NOT NULL DEFAULT '',
  origin       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_files_course ON files(course_id);
CREATE INDEX IF NOT EXISTS idx_files_modified ON files(modified_at DESC);

-- Ordinary (not contentless) FTS5 table so snippet() can return text.
-- rowid is kept equal to files.id.
CREATE VIRTUAL TABLE IF NOT EXISTS files_fts USING fts5(
  name, text, tokenize='unicode61'
);

CREATE TABLE IF NOT EXISTS deadlines (
  id              INTEGER PRIMARY KEY,
  course_id       INTEGER NOT NULL,
  course_code     TEXT NOT NULL DEFAULT '',
  title           TEXT NOT NULL DEFAULT '',
  type            TEXT NOT NULL DEFAULT 'assignment',
  due_at          TEXT NOT NULL DEFAULT '',
  submitted       INTEGER NOT NULL DEFAULT 0,
  url             TEXT NOT NULL DEFAULT '',
  points_possible REAL NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS announcements (
  id          INTEGER PRIMARY KEY,
  course_id   INTEGER NOT NULL DEFAULT 0,
  course_code TEXT NOT NULL DEFAULT '',
  title       TEXT NOT NULL DEFAULT '',
  posted_at   TEXT NOT NULL DEFAULT '',
  html        TEXT NOT NULL DEFAULT '',
  text        TEXT NOT NULL DEFAULT '',
  url         TEXT NOT NULL DEFAULT '',
  read        INTEGER NOT NULL DEFAULT 0,
  notified    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS grades (
  assignment_id INTEGER PRIMARY KEY,
  course_id     INTEGER NOT NULL DEFAULT 0,
  course_code   TEXT NOT NULL DEFAULT '',
  title         TEXT NOT NULL DEFAULT '',
  score         REAL NOT NULL DEFAULT 0,
  possible      REAL NOT NULL DEFAULT 0,
  graded_at     TEXT NOT NULL DEFAULT '',
  url           TEXT NOT NULL DEFAULT '',
  notified      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS reminders_sent (
  assignment_id INTEGER NOT NULL,
  rung          TEXT NOT NULL,
  sent_at       TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (assignment_id, rung)
);

-- Cached wiki-page metadata so unchanged pages are not re-fetched every sync.
-- file_ids is the comma-separated list extracted from the page body.
CREATE TABLE IF NOT EXISTS pages (
  course_id  INTEGER NOT NULL,
  url        TEXT NOT NULL,
  title      TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  file_ids   TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (course_id, url)
);

CREATE TABLE IF NOT EXISTS kv (
  k TEXT PRIMARY KEY,
  v TEXT NOT NULL DEFAULT ''
);
`

func (s *Store) migrate() error {
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	// Added 2026-09: records where a "pages"-sourced file was linked from.
	// CREATE TABLE IF NOT EXISTS above does nothing for pre-existing databases.
	if err := s.addColumn("files", "origin", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("store: migrate files.origin: %w", err)
	}
	return nil
}

// addColumn adds a column when the table does not already have it.
func (s *Store) addColumn(table, col, decl string) error {
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid, notnull, pk int
			name, typ        string
			dflt             any
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == col {
			return rows.Close()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	_, err = s.db.Exec("ALTER TABLE " + table + " ADD COLUMN " + col + " " + decl)
	return err
}

// PageCache is the cached metadata of one Canvas wiki page.
type PageCache struct {
	CourseID  int
	URL       string
	Title     string
	UpdatedAt string // Canvas updated_at, RFC3339
	FileIDs   []int
}

// PageCacheGet loads the cached row for a page, if any.
func (s *Store) PageCacheGet(courseID int, pageURL string) (PageCache, bool, error) {
	var (
		pc  = PageCache{CourseID: courseID, URL: pageURL}
		ids string
	)
	err := s.db.QueryRow(`SELECT title, updated_at, file_ids FROM pages WHERE course_id=? AND url=?`,
		courseID, pageURL).Scan(&pc.Title, &pc.UpdatedAt, &ids)
	if err == sql.ErrNoRows {
		return PageCache{}, false, nil
	}
	if err != nil {
		return PageCache{}, false, err
	}
	for _, part := range strings.Split(ids, ",") {
		if part == "" {
			continue
		}
		if n, err := strconv.Atoi(part); err == nil {
			pc.FileIDs = append(pc.FileIDs, n)
		}
	}
	return pc, true, nil
}

// PageCachePut stores (or refreshes) the cached row for a page.
func (s *Store) PageCachePut(pc PageCache) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	parts := make([]string, 0, len(pc.FileIDs))
	for _, id := range pc.FileIDs {
		parts = append(parts, strconv.Itoa(id))
	}
	_, err := s.db.Exec(`
		INSERT INTO pages(course_id, url, title, updated_at, file_ids, fetched_at)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(course_id, url) DO UPDATE SET
			title=excluded.title, updated_at=excluded.updated_at,
			file_ids=excluded.file_ids, fetched_at=excluded.fetched_at`,
		pc.CourseID, pc.URL, pc.Title, pc.UpdatedAt, strings.Join(parts, ","),
		time.Now().UTC().Format(time.RFC3339))
	return err
}

// GetKV reads a key, returning "" when absent.
func (s *Store) GetKV(k string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT v FROM kv WHERE k = ?`, k).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// SetKV upserts a key.
func (s *Store) SetKV(k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO kv(k,v) VALUES(?,?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, k, v)
	return err
}
