package papers

import (
	"strings"
	"testing"
)

func TestMatchKeywords(t *testing.T) {
	kw := []string{"machine unlearning", "LLM unlearning", "knowledge editing", "memorization"}
	for _, tc := range []struct {
		name string
		p    Paper
		ok   bool
		hits []string
	}{
		{"title match", Paper{Title: "Machine Unlearning at Scale"}, true,
			[]string{"machine unlearning"}},
		{"abstract match", Paper{Title: "Forgetting", Abstract: "We study knowledge editing."},
			true, []string{"knowledge editing"}},
		{"tldr match", Paper{Title: "X", TLDR: "About memorization."}, true,
			[]string{"memorization"}},
		{"two hits keeps caller order", Paper{Title: "LLM Unlearning",
			Abstract: "via knowledge editing"}, true,
			[]string{"LLM unlearning", "knowledge editing"}},
		{"no match", Paper{Title: "Diffusion models for images"}, false, nil},
	} {
		ok, hits := MatchKeywords(tc.p, kw)
		if ok != tc.ok {
			t.Errorf("%s: ok = %v, want %v", tc.name, ok, tc.ok)
		}
		if strings.Join(hits, "|") != strings.Join(tc.hits, "|") {
			t.Errorf("%s: hits = %v, want %v", tc.name, hits, tc.hits)
		}
	}
}

func TestBuildDigest(t *testing.T) {
	kw := []string{"unlearning"}
	fresh := []Paper{
		{ID: "arxiv:1", Title: "Unlearning A"},
		{ID: "arxiv:2", Title: "Irrelevant B"},
		{ID: "arxiv:3", Title: "Unlearning C"},
		{ID: "arxiv:4", Title: "Unlearning D (already saved)"},
	}
	recs := []Paper{
		{ID: "s2:r1", Title: "Rec one"},
		{ID: "arxiv:1", Title: "Unlearning A"}, // already in the arXiv half
		{ID: "s2:r2", Title: "Rec two"},
	}
	lib := map[string]bool{"arxiv:4": true}

	d := BuildDigest("2026-09-06", fresh, kw, recs, "",
		func(id string) bool { return lib[id] }, 2, 2)

	if d.Date != "2026-09-06" {
		t.Errorf("date = %q", d.Date)
	}
	want := []string{"arxiv:1", "arxiv:3", "s2:r1", "s2:r2"}
	if len(d.Papers) != len(want) {
		t.Fatalf("got %d papers, want %d: %+v", len(d.Papers), len(want), d.Papers)
	}
	for i, id := range want {
		if d.Papers[i].ID != id {
			t.Errorf("papers[%d] = %q, want %q", i, d.Papers[i].ID, id)
		}
	}
	if len(d.Reason) != len(d.Papers) {
		t.Fatalf("Reason has %d entries, Papers %d", len(d.Reason), len(d.Papers))
	}
	if !strings.Contains(d.Reason[0], "unlearning") {
		t.Errorf("reason[0] = %q, want the matched keyword", d.Reason[0])
	}
	if !strings.Contains(d.Reason[2], "recommended") {
		t.Errorf("reason[2] = %q, want the recommendation wording", d.Reason[2])
	}
}

func TestDigestMessage(t *testing.T) {
	d := Digest{Date: "2026-09-06", Papers: []Paper{{
		ID: "arxiv:1", Title: "A & B <unlearning>",
		Authors: []string{"A One", "B Two", "C Three", "D Four"},
		Year:    2026, Venue: "arXiv cs.LG", CitationCount: 7,
		TLDR: strings.Repeat("x", 400), URL: "https://arxiv.org/abs/1",
	}}, Reason: []string{"new on arXiv"}}

	msg := DigestMessage(d)
	for _, want := range []string{"<b>1. A &amp; B &lt;unlearning&gt;</b>",
		"A One, B Two, C Three et al.", "2026 · arXiv cs.LG · 7 citations",
		"https://arxiv.org/abs/1", "/save"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q\n---\n%s", want, msg)
		}
	}
	if strings.Contains(msg, strings.Repeat("x", 250)) {
		t.Error("TLDR was not clamped to 200 characters")
	}
	if len(msg) > MaxDigestMessage {
		t.Errorf("message is %d bytes, over the %d cap", len(msg), MaxDigestMessage)
	}
}

func TestDigestMessageStaysUnderCap(t *testing.T) {
	var ps []Paper
	var reasons []string
	for i := 0; i < 40; i++ {
		ps = append(ps, Paper{ID: "arxiv:x", Title: strings.Repeat("Title ", 12),
			Authors: []string{"Someone Long-Name"}, Year: 2026,
			Abstract: strings.Repeat("abstract ", 40), URL: "https://arxiv.org/abs/x"})
		reasons = append(reasons, "new on arXiv — matches unlearning")
	}
	msg := DigestMessage(Digest{Date: "2026-09-06", Papers: ps, Reason: reasons})
	if len(msg) > MaxDigestMessage {
		t.Fatalf("message is %d bytes, over the %d cap", len(msg), MaxDigestMessage)
	}
	if !strings.Contains(msg, "and") {
		t.Error("expected an overflow note")
	}
}

func TestDigestMessageEmpty(t *testing.T) {
	if got := DigestMessage(Digest{Date: "2026-09-06"}); !strings.Contains(got, "nothing new") {
		t.Errorf("empty digest = %q", got)
	}
}
