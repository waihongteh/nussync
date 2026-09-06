package papers

import (
	"testing"
	"time"
)

func TestDetectVenue(t *testing.T) {
	prefs := DefaultVenuePrefs()
	for _, tc := range []struct {
		name    string
		venue   string
		comment string
		doi     string
		year    int
		tier    int
		short   string
	}{
		// ---- arXiv comments
		{"accepted at", "", "Accepted at NeurIPS 2025", "", 2025, 2, "NeurIPS 2025"},
		{"accepted to spelled out", "", "Accepted to the 2026 International Conference on Learning Representations", "", 2026, 2, "ICLR 2026"},
		{"to appear in", "", "To appear in TMLR", "", 2025, 2, "TMLR 2025"},
		{"published in", "", "Published in Proceedings of ACL 2024, 12 pages", "", 2024, 2, "ACL 2024"},
		{"camera ready", "", "EMNLP 2025 camera-ready version", "", 2025, 2, "EMNLP 2025"},
		{"accepted as oral", "", "Accepted as an oral at ICML 2025", "", 2025, 2, "ICML 2025"},
		{"workshop is tier 1", "", "Accepted at the NeurIPS 2025 Workshop on Machine Unlearning", "", 2025, 1, "NeurIPS 2025 Workshop"},
		{"page count only", "", "10 pages, 4 figures", "", 2025, 0, "preprint"},
		{"code link is not a venue", "", "Code at https://www.example.com/repo", "", 2025, 0, "preprint"},
		{"unknown venue in comment", "", "Accepted at the Nordic Symposium on Forgetting", "", 2025, 1, "Nordic Symposium on Forgetting"},

		// ---- Semantic Scholar / journal_ref venue names
		{"s2 short name", "NeurIPS", "", "", 2024, 2, "NeurIPS 2024"},
		{"s2 long name", "Advances in Neural Information Processing Systems", "", "", 2023, 2, "NeurIPS 2023"},
		{"s2 nips alias", "NIPS", "", "", 2019, 2, "NeurIPS 2019"},
		{"s2 usenix", "USENIX Security Symposium", "", "", 2024, 2, "USENIX Security 2024"},
		{"s2 journal", "Journal of Machine Learning Research", "", "", 2022, 2, "JMLR 2022"},
		{"s2 workshop", "ICML 2024 Workshop on Data Attribution", "", "", 2024, 1, "ICML 2024 Workshop"},
		{"other journal is tier 1", "Nature Machine Intelligence", "", "", 2025, 1, "Nature Machine Intelligence"},
		{"arxiv venue is a preprint", "arXiv cs.LG", "", "", 2025, 0, "preprint"},
		{"corr is a preprint", "CoRR abs/2401.00001", "", "", 2025, 0, "preprint"},
		{"empty venue", "", "", "", 2025, 0, "preprint"},

		// ---- DOI as weak evidence
		{"publisher doi", "", "", "10.18653/v1/2024.acl-long.5", 2024, 1, "published"},
		{"arxiv doi is not evidence", "", "", "10.48550/arXiv.2401.00001", 2024, 0, "preprint"},

		// ---- venue beats a page-count comment
		{"venue wins over noise", "ICLR", "9 pages", "", 2026, 2, "ICLR 2026"},
	} {
		got := DetectVenue(tc.venue, tc.comment, tc.doi, tc.year, prefs)
		if got.Tier != tc.tier || got.Short != tc.short {
			t.Errorf("%s: got tier %d %q, want tier %d %q",
				tc.name, got.Tier, got.Short, tc.tier, tc.short)
		}
		if got.Published != (tc.tier >= 1) {
			t.Errorf("%s: Published = %v for tier %d", tc.name, got.Published, got.Tier)
		}
	}
}

func TestDetectVenueRespectsTopList(t *testing.T) {
	prefs := VenuePrefs{TopVenues: []string{"ICLR"}, PreferPublished: true}
	if got := DetectVenue("NeurIPS", "", "", 2025, prefs); got.Tier != 1 {
		t.Errorf("NeurIPS off the top list = tier %d, want 1", got.Tier)
	}
	if got := DetectVenue("ICLR", "", "", 2025, prefs); got.Tier != 2 {
		t.Errorf("ICLR on the top list = tier %d, want 2", got.Tier)
	}
}

func TestAnnotateVenueUpgradesVenueString(t *testing.T) {
	p := Paper{Venue: "arXiv cs.LG", Comment: "Accepted at ACL 2025", Year: 2025}
	AnnotateVenue(&p, DefaultVenuePrefs())
	if p.Venue != "ACL 2025" || p.VenueShort != "ACL 2025" || !p.Published || p.VenueTier != 2 {
		t.Fatalf("annotated = %+v", p)
	}
	// A real venue name is never overwritten.
	q := Paper{Venue: "Nature Machine Intelligence", Year: 2025}
	AnnotateVenue(&q, DefaultVenuePrefs())
	if q.Venue != "Nature Machine Intelligence" || q.VenueTier != 1 {
		t.Fatalf("annotated = %+v", q)
	}
}

func TestScoreOrdering(t *testing.T) {
	prefs := DefaultVenuePrefs()
	old := time.Now().AddDate(-3, 0, 0).Format("2006-01-02")
	fresh := time.Now().AddDate(0, 0, -3).Format("2006-01-02")

	top := Paper{ID: "top", Venue: "NeurIPS", Year: 2024, CitationCount: 12, PublishedAt: old}
	journal := Paper{ID: "journal", Venue: "Nature Machine Intelligence", Year: 2024, CitationCount: 12, PublishedAt: old}
	preprint := Paper{ID: "preprint", Venue: "arXiv cs.LG", Year: 2026, CitationCount: 12, PublishedAt: fresh}
	cited := Paper{ID: "cited", Venue: "arXiv cs.LG", Year: 2020, CitationCount: 400, PublishedAt: old}

	ps := []Paper{preprint, cited, journal, top}
	SortByScore(ps, prefs)
	want := []string{"top", "journal", "cited", "preprint"}
	for i, id := range want {
		if ps[i].ID != id {
			t.Fatalf("order = %s, want %v", ids(ps), want)
		}
	}

	// The tier weight is a thumb on the scale, not a veto: a preprint with
	// thousands of citations still outranks a lightly cited journal paper.
	famous := Paper{ID: "famous", Venue: "arXiv cs.LG", Year: 2017, CitationCount: 90000, PublishedAt: old}
	ps3 := []Paper{journal, famous}
	SortByScore(ps3, prefs)
	if ps3[0].ID != "famous" {
		t.Errorf("90k-citation preprint ranked below a 12-citation journal paper (%s)", ids(ps3))
	}

	// Without PreferPublished the tier weight is gone and citations decide.
	ps2 := []Paper{preprint, journal, top, cited}
	SortByScore(ps2, VenuePrefs{TopVenues: DefaultTopVenues()})
	if ps2[0].ID != "cited" {
		t.Errorf("PreferPublished off: first = %s, want cited (%s)", ps2[0].ID, ids(ps2))
	}
}

func TestScoreRecencyAndCitations(t *testing.T) {
	prefs := DefaultVenuePrefs()
	fresh := Paper{PublishedAt: time.Now().AddDate(0, 0, -1).Format("2006-01-02")}
	stale := Paper{PublishedAt: time.Now().AddDate(-5, 0, 0).Format("2006-01-02")}
	if Score(fresh, prefs) <= Score(stale, prefs) {
		t.Error("a paper from yesterday must outrank one from five years ago")
	}
	if got := Score(fresh, prefs); got > RecencyBonus+0.01 {
		t.Errorf("recency bonus %v exceeds the %v cap", got, RecencyBonus)
	}
	a := Paper{CitationCount: 100}
	b := Paper{CitationCount: 10}
	if Score(a, prefs) <= Score(b, prefs) {
		t.Error("more citations must score higher")
	}
}

func TestFilterPublished(t *testing.T) {
	ps := AnnotateVenues([]Paper{
		{ID: "a", Venue: "ICLR", Year: 2025},
		{ID: "b", Venue: "arXiv cs.LG"},
		{ID: "c", Venue: "Nature"},
	}, DefaultVenuePrefs())
	got := FilterPublished(ps)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "c" {
		t.Errorf("FilterPublished = %s", ids(got))
	}
}

func ids(ps []Paper) string {
	out := ""
	for _, p := range ps {
		out += p.ID + " "
	}
	return out
}
