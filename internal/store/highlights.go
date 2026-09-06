package store

import (
	"database/sql"
	"errors"
	"time"
)

// PDF highlight storage. Owned by the viewer feature; MigrateHighlights is
// called lazily on first use (same pattern as MigrateStudy) so a user who never
// highlights anything never pays for the table.
//
// `rects` is a JSON array of {X,Y,W,H} normalised 0..1 against the page box —
// the store keeps it opaque, package main marshals it (see app_highlights.go).
const highlightSchema = `
CREATE TABLE IF NOT EXISTS highlights (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  file_id    INTEGER NOT NULL,
  page       INTEGER NOT NULL DEFAULT 1,
  rects      TEXT NOT NULL DEFAULT '[]',
  text       TEXT NOT NULL DEFAULT '',
  color      TEXT NOT NULL DEFAULT 'yellow',
  note       TEXT NOT NULL DEFAULT '',
  kind       TEXT NOT NULL DEFAULT 'highlight',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_highlights_file ON highlights(file_id, page);
`

// MigrateHighlights creates the highlights table. Idempotent.
func (s *Store) MigrateHighlights() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(highlightSchema); err != nil {
		return err
	}
	// Added 2026-09: text-box annotations share the table. CREATE TABLE IF NOT
	// EXISTS does nothing for a database that already has the old shape, so the
	// column is added separately; existing rows default to 'highlight'.
	return s.addColumn("highlights", "kind", "TEXT NOT NULL DEFAULT 'highlight'")
}

// Highlight is one stored highlight. Rects is the raw JSON blob.
type Highlight struct {
	ID     int
	FileID int
	Page   int
	Rects  string
	Text   string
	Color  string
	Note   string
	// Kind is "highlight" (a mark over text) or "note" (a free-floating text box).
	Kind      string
	CreatedAt string
	UpdatedAt string
}

const highlightSelect = `SELECT id, file_id, page, rects, text, color, note, kind,
	created_at, updated_at FROM highlights`

func scanHighlights(rows *sql.Rows) ([]Highlight, error) {
	out := []Highlight{}
	for rows.Next() {
		var h Highlight
		if err := rows.Scan(&h.ID, &h.FileID, &h.Page, &h.Rects, &h.Text,
			&h.Color, &h.Note, &h.Kind, &h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// HighlightsForFile lists a file's highlights in reading order.
func (s *Store) HighlightsForFile(fileID int) ([]Highlight, error) {
	rows, err := s.db.Query(highlightSelect+` WHERE file_id=? ORDER BY page, id`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHighlights(rows)
}

// HighlightByID loads one highlight.
func (s *Store) HighlightByID(id int) (Highlight, bool, error) {
	rows, err := s.db.Query(highlightSelect+` WHERE id=?`, id)
	if err != nil {
		return Highlight{}, false, err
	}
	defer rows.Close()
	hs, err := scanHighlights(rows)
	if err != nil || len(hs) == 0 {
		return Highlight{}, false, err
	}
	return hs[0], true, nil
}

// PutHighlight inserts (ID == 0) or updates by id, returning the stored row.
func (s *Store) PutHighlight(h Highlight) (Highlight, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	h.UpdatedAt = now
	if h.ID == 0 {
		if h.CreatedAt == "" {
			h.CreatedAt = now
		}
		res, err := s.db.Exec(`
			INSERT INTO highlights(file_id, page, rects, text, color, note, kind, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?,?,?)`,
			h.FileID, h.Page, h.Rects, h.Text, h.Color, h.Note, h.Kind, h.CreatedAt, h.UpdatedAt)
		if err != nil {
			return Highlight{}, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return Highlight{}, err
		}
		h.ID = int(id)
		return h, nil
	}

	res, err := s.db.Exec(`
		UPDATE highlights SET file_id=?, page=?, rects=?, text=?, color=?, note=?, kind=?, updated_at=?
		WHERE id=?`,
		h.FileID, h.Page, h.Rects, h.Text, h.Color, h.Note, h.Kind, h.UpdatedAt, h.ID)
	if err != nil {
		return Highlight{}, err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return Highlight{}, errors.New("no such highlight")
	}
	// created_at is never rewritten by an update; read the stored one back.
	var created string
	if err := s.db.QueryRow(`SELECT created_at FROM highlights WHERE id=?`, h.ID).Scan(&created); err == nil {
		h.CreatedAt = created
	}
	return h, nil
}

// DeleteHighlight removes one highlight. Deleting a missing id is not an error.
func (s *Store) DeleteHighlight(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM highlights WHERE id=?`, id)
	return err
}
