// Package papers implements the research-paper tracker: arXiv + Semantic
// Scholar search, a daily digest, BibTeX export and the naming rules for
// downloaded PDFs.
//
// Google Scholar has no API and is never scraped — the app only ever opens a
// Scholar search URL in the browser.
package papers

import (
	"fmt"
	"net/url"
	"strings"
)

// UserAgent identifies NUSSync to both APIs.
const UserAgent = "NUSSync/1.0"

// Paper is one work from either source. The field set matches the contract
// type in package main (docs/CONTRACT_PAPERS.md) one-for-one.
type Paper struct {
	ID            string // "arxiv:<id without version>" | "s2:<paperId>"
	ArxivID       string // without the version suffix
	S2ID          string
	DOI           string
	Title         string
	Authors       []string
	Year          int
	Venue         string
	Abstract      string
	TLDR          string
	CitationCount int
	URL           string
	PDFURL        string
	PublishedAt   string // RFC3339 or YYYY-MM-DD
	Source        string // "arxiv" | "s2"

	// Venue ranking (venue.go). Derived, never stored: AnnotateVenue fills
	// them from Venue/Comment/DOI every time a paper crosses a boundary.
	VenueTier  int    // 2 top venue, 1 published, 0 preprint/unknown
	VenueShort string // badge label, e.g. "NeurIPS 2025" or "preprint"
	Published  bool   // VenueTier >= 1

	// Comment is the arXiv <arxiv:comment> ("Accepted at ACL 2025"). It is
	// evidence for venue detection only and is not part of the app contract.
	Comment string
}

// ArxivPaperID builds the canonical id for an arXiv paper.
func ArxivPaperID(arxivID string) string { return "arxiv:" + StripVersion(arxivID) }

// S2PaperID builds the canonical id for a Semantic Scholar paper.
func S2PaperID(s2ID string) string { return "s2:" + s2ID }

// SplitID splits "arxiv:2401.00001" into ("arxiv", "2401.00001").
func SplitID(id string) (kind, rest string) {
	k, r, ok := strings.Cut(strings.TrimSpace(id), ":")
	if !ok {
		return "", strings.TrimSpace(id)
	}
	return k, r
}

// StripVersion removes a trailing arXiv version ("2401.00001v3" -> "2401.00001").
func StripVersion(id string) string {
	id = strings.TrimSpace(id)
	for i := len(id) - 1; i >= 0; i-- {
		c := id[i]
		if c >= '0' && c <= '9' {
			continue
		}
		if c == 'v' && i > 0 && i < len(id)-1 {
			return id[:i]
		}
		break
	}
	return id
}

// FirstAuthorLast returns the surname of the first author, or "".
func FirstAuthorLast(authors []string) string {
	if len(authors) == 0 {
		return ""
	}
	f := strings.TrimSpace(authors[0])
	if f == "" {
		return ""
	}
	if i := strings.LastIndex(f, " "); i >= 0 {
		return strings.TrimSpace(f[i+1:])
	}
	return f
}

// Merge combines results from both sources, dropping Semantic Scholar entries
// that duplicate an arXiv entry (matched on the versionless arXiv id) and
// enriching the surviving arXiv entry with the S2 metadata (TLDR, citations,
// venue, DOI).
func Merge(arxiv, s2 []Paper) []Paper {
	byArxiv := map[string]int{}
	out := make([]Paper, 0, len(arxiv)+len(s2))
	for _, p := range arxiv {
		if p.ArxivID != "" {
			byArxiv[StripVersion(p.ArxivID)] = len(out)
		}
		out = append(out, p)
	}
	for _, p := range s2 {
		key := StripVersion(p.ArxivID)
		if key != "" {
			if i, ok := byArxiv[key]; ok {
				enrich(&out[i], p)
				continue
			}
		}
		if key != "" {
			byArxiv[key] = len(out)
		}
		out = append(out, p)
	}
	return out
}

// enrich fills empty fields of an arXiv paper from its S2 twin.
func enrich(dst *Paper, src Paper) {
	if dst.S2ID == "" {
		dst.S2ID = src.S2ID
	}
	if dst.DOI == "" {
		dst.DOI = src.DOI
	}
	if dst.TLDR == "" {
		dst.TLDR = src.TLDR
	}
	// An arXiv row's Venue is "arXiv cs.LG" when the paper carries no
	// journal_ref, which must not hide a real venue coming from S2.
	if IsPreprintVenue(dst.Venue) && !IsPreprintVenue(src.Venue) {
		dst.Venue = src.Venue
	}
	if dst.CitationCount == 0 {
		dst.CitationCount = src.CitationCount
	}
	if dst.Year == 0 {
		dst.Year = src.Year
	}
	if dst.Abstract == "" {
		dst.Abstract = src.Abstract
	}
}

// ScholarURL is the "Open in Google Scholar" link. Scholar has no API and is
// never fetched by NUSSync — this URL is only ever handed to the browser.
func ScholarURL(query string) string {
	return "https://scholar.google.com/scholar?q=" + url.QueryEscape(strings.TrimSpace(query))
}

// Filename builds the on-disk PDF name: "<year> - <first author> - <title>.pdf".
// Every segment is sanitised for Windows and the whole name is capped.
func Filename(p Paper) string {
	year := "n.d."
	if p.Year > 0 {
		year = fmt.Sprint(p.Year)
	}
	author := FirstAuthorLast(p.Authors)
	if author == "" {
		author = "Unknown"
	}
	title := collapseSpace(p.Title)
	if title == "" {
		title = strings.TrimSpace(p.ID)
	}
	if len(title) > 90 {
		title = trimBytes(title, 90)
	}
	return sanitizeSegment(year+" - "+author+" - "+title) + ".pdf"
}

func collapseSpace(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
}

func trimBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && s[len(s)-1]&0xC0 == 0x80 {
		s = s[:len(s)-1]
	}
	return strings.TrimRight(s, " .,-")
}
