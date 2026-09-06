package store

import "testing"

func TestHighlightRoundTrip(t *testing.T) {
	s := newTestStore(t)
	if err := s.MigrateHighlights(); err != nil {
		t.Fatalf("MigrateHighlights: %v", err)
	}
	// Idempotent.
	if err := s.MigrateHighlights(); err != nil {
		t.Fatalf("MigrateHighlights twice: %v", err)
	}

	if hs, err := s.HighlightsForFile(10); err != nil || len(hs) != 0 {
		t.Fatalf("empty file: %v %v", hs, err)
	}

	in := Highlight{FileID: 10, Page: 3, Rects: `[{"X":0.1,"Y":0.2,"W":0.3,"H":0.04}]`,
		Text: "entropy is expected surprise", Color: "green"}
	saved, err := s.PutHighlight(in)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if saved.ID == 0 || saved.CreatedAt == "" || saved.UpdatedAt == "" {
		t.Fatalf("insert did not fill id/timestamps: %+v", saved)
	}

	// Second page, same file, to prove ordering.
	if _, err := s.PutHighlight(Highlight{FileID: 10, Page: 1, Rects: `[]`, Color: "blue"}); err != nil {
		t.Fatalf("insert 2: %v", err)
	}
	// Another file must not leak in.
	if _, err := s.PutHighlight(Highlight{FileID: 11, Page: 1, Rects: `[]`}); err != nil {
		t.Fatalf("insert 3: %v", err)
	}

	hs, err := s.HighlightsForFile(10)
	if err != nil || len(hs) != 2 {
		t.Fatalf("list: %v %v", hs, err)
	}
	if hs[0].Page != 1 || hs[1].Page != 3 {
		t.Errorf("not ordered by page: %d, %d", hs[0].Page, hs[1].Page)
	}

	// Update keeps created_at and the id.
	saved.Note = "exam"
	saved.Color = "pink"
	up, err := s.PutHighlight(saved)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if up.ID != saved.ID || up.CreatedAt != saved.CreatedAt {
		t.Errorf("update changed identity: %+v vs %+v", up, saved)
	}
	got, ok, err := s.HighlightByID(saved.ID)
	if err != nil || !ok {
		t.Fatalf("by id: %v %v", ok, err)
	}
	if got.Note != "exam" || got.Color != "pink" {
		t.Errorf("update not persisted: %+v", got)
	}

	// Kind round-trips; a row saved without one reads back empty (package main
	// normalises it to "highlight").
	box, err := s.PutHighlight(Highlight{FileID: 10, Page: 2, Rects: `[{"X":0.1,"Y":0.1,"W":0.3,"H":0.1}]`,
		Text: "todo: revise", Color: "yellow", Kind: "note"})
	if err != nil {
		t.Fatalf("insert note: %v", err)
	}
	if got, ok, err := s.HighlightByID(box.ID); err != nil || !ok || got.Kind != "note" {
		t.Errorf("kind not persisted: %+v %v %v", got, ok, err)
	}
	box.Text = "revised"
	if _, err := s.PutHighlight(box); err != nil {
		t.Fatalf("update note: %v", err)
	}
	if got, _, _ := s.HighlightByID(box.ID); got.Kind != "note" || got.Text != "revised" {
		t.Errorf("update dropped kind: %+v", got)
	}
	if err := s.DeleteHighlight(box.ID); err != nil {
		t.Fatalf("delete note: %v", err)
	}

	// Updating a missing row is an error, deleting one is not.
	if _, err := s.PutHighlight(Highlight{ID: 9999, FileID: 10, Rects: `[]`}); err == nil {
		t.Error("update of a missing id should fail")
	}
	if err := s.DeleteHighlight(9999); err != nil {
		t.Errorf("delete of a missing id should be a no-op: %v", err)
	}
	if err := s.DeleteHighlight(saved.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if hs, _ := s.HighlightsForFile(10); len(hs) != 1 {
		t.Errorf("after delete: %d rows", len(hs))
	}
}

// A database created before the `kind` column exists must gain it on migrate,
// with the existing rows reading back as plain highlights.
func TestHighlightsKindMigration(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.db.Exec(`CREATE TABLE highlights (
		id INTEGER PRIMARY KEY AUTOINCREMENT, file_id INTEGER NOT NULL,
		page INTEGER NOT NULL DEFAULT 1, rects TEXT NOT NULL DEFAULT '[]',
		text TEXT NOT NULL DEFAULT '', color TEXT NOT NULL DEFAULT 'yellow',
		note TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("legacy table: %v", err)
	}
	if _, err := s.db.Exec(`INSERT INTO highlights(file_id, page, text) VALUES(7, 1, 'old')`); err != nil {
		t.Fatalf("legacy row: %v", err)
	}
	if err := s.MigrateHighlights(); err != nil {
		t.Fatalf("MigrateHighlights: %v", err)
	}
	hs, err := s.HighlightsForFile(7)
	if err != nil || len(hs) != 1 {
		t.Fatalf("list: %v %v", hs, err)
	}
	if hs[0].Kind != "highlight" || hs[0].Text != "old" {
		t.Errorf("legacy row after migration: %+v", hs[0])
	}
}
