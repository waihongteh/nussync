package papers

import (
	"strings"
	"testing"
)

func TestBibKey(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    Paper
		want string
	}{
		{"normal", Paper{Authors: []string{"Ada Lovelace"}, Year: 2024,
			Title: "Machine Unlearning at Scale"}, "lovelace2024machine"},
		{"stop word skipped", Paper{Authors: []string{"Alan Turing"}, Year: 2023,
			Title: "On the Foundations of Editing"}, "turing2023foundations"},
		{"no author", Paper{Year: 2022, Title: "Anonymous Work"}, "anon2022anonymous"},
		{"no year", Paper{Authors: []string{"Grace Hopper"}, Title: "Compilers"},
			"hoppercompilers"},
		{"accents and hyphens", Paper{Authors: []string{"Jean-Luc Béziau"}, Year: 2021,
			Title: "Épistémologie of Logic"}, "bziau2021pistmologie"},
	} {
		if got := BibKey(tc.p); got != tc.want {
			t.Errorf("%s: BibKey = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestBibEntryArxivIsMisc(t *testing.T) {
	p := Paper{
		ID: "arxiv:2401.00001", ArxivID: "2401.00001", Source: "arxiv",
		Title: "Machine Unlearning at Scale", Authors: []string{"Ada Lovelace", "Alan Turing"},
		Year: 2024, Venue: "arXiv cs.LG", URL: "https://arxiv.org/abs/2401.00001",
	}
	e := BibEntry(p)
	for _, want := range []string{
		"@misc{lovelace2024machine,",
		"title = {Machine Unlearning at Scale},",
		"author = {Ada Lovelace and Alan Turing},",
		"year = {2024},",
		"eprint = {2401.00001},",
		"archivePrefix = {arXiv},",
		"primaryClass = {cs.LG},",
		"url = {https://arxiv.org/abs/2401.00001},",
	} {
		if !strings.Contains(e, want) {
			t.Errorf("entry missing %q\n---\n%s", want, e)
		}
	}
	if strings.Contains(e, "@article") {
		t.Error("an arXiv preprint must be @misc")
	}
}

func TestBibEntryDOIIsArticle(t *testing.T) {
	p := Paper{
		ID: "s2:abc", S2ID: "abc", Source: "s2", DOI: "10.1000/abc",
		Title: "Knowledge Editing & Memory", Authors: []string{"Grace Hopper"},
		Year: 2023, Venue: "NeurIPS", URL: "https://example.org/p",
	}
	e := BibEntry(p)
	for _, want := range []string{
		"@article{hopper2023knowledge,",
		"journal = {NeurIPS},",
		"doi = {10.1000/abc},",
		`title = {Knowledge Editing \& Memory},`,
	} {
		if !strings.Contains(e, want) {
			t.Errorf("entry missing %q\n---\n%s", want, e)
		}
	}
}

func TestBibTeXDisambiguatesKeys(t *testing.T) {
	p := Paper{Authors: []string{"Ada Lovelace"}, Year: 2024,
		Title: "Machine Unlearning", ArxivID: "2401.1", Source: "arxiv"}
	out := BibTeX([]Paper{p, p})
	if !strings.Contains(out, "@misc{lovelace2024machine,") {
		t.Errorf("missing base key:\n%s", out)
	}
	if !strings.Contains(out, "@misc{lovelace2024machinea,") {
		t.Errorf("duplicate key was not disambiguated:\n%s", out)
	}
}
