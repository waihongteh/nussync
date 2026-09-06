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
