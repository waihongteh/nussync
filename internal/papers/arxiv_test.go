package papers

import "testing"

// A trimmed but structurally faithful arXiv Atom response.
const atomFixture = `<?xml version='1.0' encoding='UTF-8'?>
<feed xmlns:opensearch="http://a9.com/-/spec/opensearch/1.1/" xmlns:arxiv="http://arxiv.org/schemas/atom" xmlns="http://www.w3.org/2005/Atom">
  <title>arXiv Query: search_query=all:"LLM unlearning"</title>
  <opensearch:totalResults>155</opensearch:totalResults>
  <opensearch:startIndex>0</opensearch:startIndex>
  <entry>
    <id>http://arxiv.org/abs/2605.30919v2</id>
    <title>De-attribute to Forget for
      LLM Unlearning</title>
    <updated>2026-07-04T15:08:50Z</updated>
    <link href="https://arxiv.org/abs/2605.30919v2" rel="alternate" type="text/html"/>
    <link href="http://arxiv.org/pdf/2605.30919v2" rel="related" type="application/pdf" title="pdf"/>
    <summary>  We frame LLM unlearning as zeroing out data attribution.
</summary>
    <category term="cs.LG" scheme="http://arxiv.org/schemas/atom"/>
    <category term="cs.AI" scheme="http://arxiv.org/schemas/atom"/>
    <published>2026-05-29T07:03:20Z</published>
    <arxiv:primary_category term="cs.LG"/>
    <author><name>Xinyang Lu</name></author>
    <author><name>Jiabao Pan</name></author>
  </entry>
  <entry>
    <id>http://arxiv.org/abs/2401.00001v1</id>
    <title>Knowledge Editing at Scale</title>
    <summary>An editing method.</summary>
    <published>2024-01-02T00:00:00Z</published>
    <arxiv:doi>10.1000/xyz</arxiv:doi>
    <arxiv:journal_ref>ACL 2024</arxiv:journal_ref>
    <arxiv:primary_category term="cs.CL"/>
    <author><name>Ada Lovelace</name></author>
  </entry>
</feed>`

func TestParseAtom(t *testing.T) {
	ps, total, err := ParseAtom([]byte(atomFixture))
	if err != nil {
		t.Fatalf("ParseAtom: %v", err)
	}
	if total != 155 {
		t.Errorf("total = %d, want 155", total)
	}
	if len(ps) != 2 {
		t.Fatalf("got %d entries, want 2", len(ps))
	}

	tests := []struct {
		field string
		got   any
		want  any
	}{
		{"ID", ps[0].ID, "arxiv:2605.30919"},
		{"ArxivID", ps[0].ArxivID, "2605.30919"},
		{"Title", ps[0].Title, "De-attribute to Forget for LLM Unlearning"},
		{"Abstract", ps[0].Abstract, "We frame LLM unlearning as zeroing out data attribution."},
		{"Year", ps[0].Year, 2026},
		{"Authors", len(ps[0].Authors), 2},
		{"FirstAuthor", ps[0].Authors[0], "Xinyang Lu"},
		{"PDFURL", ps[0].PDFURL, "https://arxiv.org/pdf/2605.30919v2"},
		{"URL", ps[0].URL, "https://arxiv.org/abs/2605.30919"},
		{"Venue-from-category", ps[0].Venue, "arXiv cs.LG"},
		{"Source", ps[0].Source, "arxiv"},
		{"DOI", ps[1].DOI, "10.1000/xyz"},
		{"JournalRef", ps[1].Venue, "ACL 2024"},
		{"Year2", ps[1].Year, 2024},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.field, tc.got, tc.want)
		}
	}
}

func TestStripVersion(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"2401.00001v3", "2401.00001"},
		{"2401.00001", "2401.00001"},
		{"cs/0501001v1", "cs/0501001"},
		{"2401.00001v12", "2401.00001"},
		{"", ""},
	} {
		if got := StripVersion(tc.in); got != tc.want {
			t.Errorf("StripVersion(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuildSearchQuery(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"LLM unlearning", `all:LLM AND all:unlearning`},
		{`"knowledge editing" scale`, `all:"knowledge editing" AND all:scale`},
		{"  ", ""},
		{"unlearning", "all:unlearning"},
	} {
		if got := BuildSearchQuery(tc.in); got != tc.want {
			t.Errorf("BuildSearchQuery(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCategoryAndKeywordQuery(t *testing.T) {
	if got, want := CategoryQuery([]string{"cs.CL", "cs.LG"}),
		"(cat:cs.CL OR cat:cs.LG)"; got != want {
		t.Errorf("CategoryQuery = %q, want %q", got, want)
	}
	if got, want := KeywordQuery([]string{"LLM unlearning", ""}),
		`(all:"LLM unlearning")`; got != want {
		t.Errorf("KeywordQuery = %q, want %q", got, want)
	}
	if got := CategoryQuery(nil); got != "" {
		t.Errorf("CategoryQuery(nil) = %q, want empty", got)
	}
}

func TestFilename(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   Paper
		want string
	}{
		{"normal", Paper{Year: 2024, Authors: []string{"Ada Lovelace"},
			Title: "Knowledge Editing at Scale"},
			"2024 - Lovelace - Knowledge Editing at Scale.pdf"},
		{"illegal chars", Paper{Year: 2023, Authors: []string{"Kim Jae-Won"},
			Title: `Who/What: "Unlearning?"`},
			"2023 - Jae-Won - Who_What_ _Unlearning__.pdf"},
		{"no year, no author", Paper{ID: "s2:abc", Title: "Untitled work"},
			"n.d. - Unknown - Untitled work.pdf"},
		{"multiline title", Paper{Year: 2025, Authors: []string{"Zhang Wei"},
			Title: "A\n  long   title"},
			"2025 - Wei - A long title.pdf"},
	} {
		if got := Filename(tc.in); got != tc.want {
			t.Errorf("%s: Filename = %q, want %q", tc.name, got, tc.want)
		}
	}
}
