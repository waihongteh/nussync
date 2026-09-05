package store

import (
	"path/filepath"
	"testing"
)

func TestBuildFTSQuery(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
		{"punctuation only", "--- ***", ""},
		{"single token", "markov", `"markov"*`},
		{"two tokens", "markov decision", `"markov" AND "decision"*`},
		{"case folded", "Markov DECISION", `"markov" AND "decision"*`},
		{"punctuation split", "week-1 notes.pdf", `"week" AND "1" AND "notes" AND "pdf"*`},
		{"quotes neutralised", `say "hi"`, `"say" AND "hi"*`},
		{"fts operators are literal", "a OR b", `"a" AND "or" AND "b"*`},
		{"unicode", "résumé", `"résumé"*`},
		{"digits", "cs4246 lecture 3", `"cs4246" AND "lecture" AND "3"*`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := BuildFTSQuery(tc.in); got != tc.want {
				t.Errorf("BuildFTSQuery(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSearchFTSAndFallback(t *testing.T) {
	s := newTestStore(t)

	if err := s.UpsertCourse(Course{ID: 1, Code: "CS4246", Name: "AI Planning", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	files := []File{
		{ID: 10, CourseID: 1, Name: "Lecture 1.pdf", RelPath: "Lectures/Lecture 1.pdf",
			AbsPath: `C:\x\Lecture 1.pdf`, Synced: true, ModifiedAt: "2026-09-01T00:00:00Z"},
		{ID: 11, CourseID: 1, Name: "Tutorial 2.pdf", RelPath: "Tutorials/Tutorial 2.pdf",
			AbsPath: `C:\x\Tutorial 2.pdf`, Synced: true, ModifiedAt: "2026-09-02T00:00:00Z"},
	}
	for _, f := range files {
		if err := s.UpsertFile(f); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MarkIndexed(10, "Lecture 1.pdf Lectures", "Markov decision processes and value iteration"); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkIndexed(11, "Tutorial 2.pdf Tutorials", "Bayesian networks exercise"); err != nil {
		t.Fatal(err)
	}

	t.Run("content match", func(t *testing.T) {
		hits, err := s.Search("markov", 0, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 1 || hits[0].File.ID != 10 {
			t.Fatalf("got %+v", hits)
		}
		if hits[0].CourseCode != "CS4246" {
			t.Errorf("CourseCode = %q", hits[0].CourseCode)
		}
		if hits[0].Snippet == "" {
			t.Error("expected a snippet")
		}
	})

	t.Run("prefix match on last token", func(t *testing.T) {
		hits, err := s.Search("bayes", 0, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 1 || hits[0].File.ID != 11 {
			t.Fatalf("got %+v", hits)
		}
	})

	t.Run("name match", func(t *testing.T) {
		hits, err := s.Search("tutorial", 0, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 1 || hits[0].File.ID != 11 {
			t.Fatalf("got %+v", hits)
		}
	})

	t.Run("course filter", func(t *testing.T) {
		hits, err := s.Search("markov", 999, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 0 {
			t.Fatalf("expected no hits for other course, got %+v", hits)
		}
	})

	t.Run("operator-looking query does not error", func(t *testing.T) {
		if _, err := s.Search(`markov AND ( NEAR "x"`, 0, 10); err != nil {
			t.Fatalf("query with FTS syntax must not error: %v", err)
		}
	})

	t.Run("like fallback finds unindexed name", func(t *testing.T) {
		if err := s.UpsertFile(File{ID: 12, CourseID: 1, Name: "Syllabus.docx",
			RelPath: "Syllabus.docx", AbsPath: `C:\x\Syllabus.docx`, Synced: true}); err != nil {
			t.Fatal(err)
		}
		hits, err := s.Search("syllabus", 0, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 1 || hits[0].File.ID != 12 {
			t.Fatalf("got %+v", hits)
		}
	})
}

func TestReindexReplacesRow(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertFile(File{ID: 1, CourseID: 1, Name: "a.pdf", Synced: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkIndexed(1, "a.pdf", "alpha"); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkIndexed(1, "a.pdf", "beta"); err != nil {
		t.Fatal(err)
	}
	hits, err := s.Search("beta", 0, 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("hits=%v err=%v", hits, err)
	}
	// The old text must be gone from the index (LIKE fallback won't match it).
	hits, err = s.Search("alpha", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("stale FTS row survived: %+v", hits)
	}
}
