// Package store is the SQLite persistence layer (pure-Go modernc driver).
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

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
  url          TEXT NOT NULL DEFAULT ''
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

CREATE TABLE IF NOT EXISTS kv (
  k TEXT PRIMARY KEY,
  v TEXT NOT NULL DEFAULT ''
);
`

func (s *Store) migrate() error {
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	return nil
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
