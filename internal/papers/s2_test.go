package papers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const s2SearchFixture = `{
  "total": 421,
  "offset": 0,
  "data": [
    {
      "paperId": "abc123",
      "title": "Machine Unlearning for LLMs",
      "abstract": "We study forgetting.",
      "year": 2024,
      "venue": "NeurIPS",
      "publicationDate": "2024-06-01",
      "citationCount": 42,
      "authors": [{"name": "Ada Lovelace"}, {"name": "Alan Turing"}],
      "openAccessPdf": {"url": "https://example.org/paper.pdf"},
      "tldr": {"text": "Forgetting works."},
      "externalIds": {"ArXiv": "2401.00001", "DOI": "10.1000/abc"},
      "url": "https://www.semanticscholar.org/paper/abc123"
    },
    {
      "paperId": "def456",
      "title": "No extras here",
      "year": 0,
      "authors": [],
      "externalIds": {},
      "tldr": null,
      "openAccessPdf": null
    }
  ]
}`

const s2CitationsFixture = `{
  "data": [
    {"citingPaper": {"paperId": "cite1", "title": "Cites it", "year": 2025,
      "authors": [{"name": "Grace Hopper"}], "externalIds": {}}},
    {"citingPaper": null},
    {"citedPaper": {"paperId": "ref1", "title": "Cited by it", "year": 2020,
      "authors": [], "externalIds": {"ArXiv": "1909.01234v2"}}}
  ]
}`

func TestParseS2Search(t *testing.T) {
	ps, total, err := ParseS2Search([]byte(s2SearchFixture))
	if err != nil {
		t.Fatalf("ParseS2Search: %v", err)
	}
	if total != 421 {
		t.Errorf("total = %d, want 421", total)
	}
	if len(ps) != 2 {
		t.Fatalf("got %d papers, want 2", len(ps))
	}
	for _, tc := range []struct {
		field string
		got   any
		want  any
	}{
		{"ID", ps[0].ID, "s2:abc123"},
		{"S2ID", ps[0].S2ID, "abc123"},
		{"ArxivID", ps[0].ArxivID, "2401.00001"},
		{"DOI", ps[0].DOI, "10.1000/abc"},
		{"TLDR", ps[0].TLDR, "Forgetting works."},
		{"Venue", ps[0].Venue, "NeurIPS"},
		{"Citations", ps[0].CitationCount, 42},
		{"PDFURL", ps[0].PDFURL, "https://example.org/paper.pdf"},
		{"Authors", len(ps[0].Authors), 2},
		{"Source", ps[0].Source, "s2"},
		{"PublishedAt", ps[0].PublishedAt, "2024-06-01"},
		{"empty TLDR", ps[1].TLDR, ""},
		{"fallback URL", ps[1].URL, "https://www.semanticscholar.org/paper/def456"},
		{"no PDF", ps[1].PDFURL, ""},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.field, tc.got, tc.want)
		}
	}
}

func TestParseS2Edges(t *testing.T) {
	ps, err := ParseS2Edges([]byte(s2CitationsFixture))
	if err != nil {
		t.Fatalf("ParseS2Edges: %v", err)
	}
	if len(ps) != 2 {
		t.Fatalf("got %d papers, want 2 (the null row is dropped)", len(ps))
	}
	if ps[0].ID != "s2:cite1" || ps[1].ID != "s2:ref1" {
		t.Errorf("ids = %q, %q", ps[0].ID, ps[1].ID)
	}
	if ps[1].ArxivID != "1909.01234" {
		t.Errorf("arXiv id = %q, want the versionless form", ps[1].ArxivID)
	}
	if ps[1].PDFURL != "https://arxiv.org/pdf/1909.01234" {
		t.Errorf("PDFURL = %q, want the arXiv fallback", ps[1].PDFURL)
	}
}

func TestMergeDedupesByArxivID(t *testing.T) {
	ax := []Paper{{ID: "arxiv:2401.00001", ArxivID: "2401.00001",
		Title: "Machine Unlearning for LLMs", Source: "arxiv"}}
	s2 := []Paper{
		{ID: "s2:abc123", S2ID: "abc123", ArxivID: "2401.00001", DOI: "10.1000/abc",
			TLDR: "Forgetting works.", Venue: "NeurIPS", CitationCount: 42, Year: 2024},
		{ID: "s2:zzz", S2ID: "zzz", Title: "Unrelated"},
	}
	got := Merge(ax, s2)
	if len(got) != 2 {
		t.Fatalf("merged to %d papers, want 2", len(got))
	}
	if got[0].ID != "arxiv:2401.00001" {
		t.Errorf("arXiv entry should win: %q", got[0].ID)
	}
	for _, tc := range []struct {
		field string
		got   any
		want  any
	}{
		{"S2ID", got[0].S2ID, "abc123"},
		{"DOI", got[0].DOI, "10.1000/abc"},
		{"TLDR", got[0].TLDR, "Forgetting works."},
		{"Venue", got[0].Venue, "NeurIPS"},
		{"Citations", got[0].CitationCount, 42},
		{"Year", got[0].Year, 2024},
		{"second", got[1].ID, "s2:zzz"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.field, tc.got, tc.want)
		}
	}
}

// TestS2RetriesOn429 checks the backoff path: two 429s then a 200.
func TestS2RetriesOn429(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"message":"Too Many Requests"}`))
			return
		}
		w.Write([]byte(s2SearchFixture))
	}))
	defer srv.Close()

	c := &S2{HTTP: srv.Client(), Sleep: func(context.Context, time.Duration) error { return nil }}
	body, err := c.do(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if calls != 3 {
		t.Errorf("made %d calls, want 3 (two retries)", calls)
	}
	if _, total, err := ParseS2Search(body); err != nil || total != 421 {
		t.Errorf("parse after retry: total=%d err=%v", total, err)
	}
}

// TestS2GivesUpAfterMaxRetries makes the failure non-fatal for the caller.
func TestS2GivesUpAfterMaxRetries(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := &S2{HTTP: srv.Client(), Sleep: func(context.Context, time.Duration) error { return nil }}
	if _, err := c.do(context.Background(), http.MethodGet, srv.URL, nil); err == nil {
		t.Fatal("want an error after exhausting retries")
	}
	if calls != S2MaxRetries+1 {
		t.Errorf("made %d calls, want %d", calls, S2MaxRetries+1)
	}
}
