package papers

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ArxivEndpoint is the Atom query API. https:// is used directly: the http://
// form answers 301 and a redirect would lose the User-Agent on some proxies.
const ArxivEndpoint = "https://export.arxiv.org/api/query"

// arXiv asks for at least 3 seconds between requests. One process-wide limiter
// covers every caller (search, digest, by-id).
var (
	arxivMu   sync.Mutex
	arxivLast time.Time
)

// ArxivMinInterval is the client-side rate limit.
const ArxivMinInterval = 3 * time.Second

// arxivWait blocks until the limiter allows another call, or ctx ends.
func arxivWait(ctx context.Context) error {
	arxivMu.Lock()
	defer arxivMu.Unlock()
	wait := ArxivMinInterval - time.Since(arxivLast)
	if wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	arxivLast = time.Now()
	return nil
}

// Arxiv queries the arXiv Atom API.
type Arxiv struct {
	HTTP *http.Client
}

// NewArxiv builds a client with a sane timeout.
func NewArxiv() *Arxiv {
	return &Arxiv{HTTP: &http.Client{Timeout: 45 * time.Second}}
}

// Sort orders for the API.
const (
	SortRelevance = "relevance"
	SortSubmitted = "submittedDate"
)

// Search runs a free-text query. Every whitespace-separated term is required in
// any field (`all:`); a quoted phrase is kept as one term.
func (c *Arxiv) Search(ctx context.Context, query string, limit int, sortBy string) ([]Paper, int, error) {
	return c.SearchQuery(ctx, BuildSearchQuery(query), limit, sortBy)
}

// SearchQuery runs a raw arXiv `search_query` expression.
func (c *Arxiv) SearchQuery(ctx context.Context, searchQuery string, limit int, sortBy string) ([]Paper, int, error) {
	if strings.TrimSpace(searchQuery) == "" {
		return nil, 0, fmt.Errorf("arxiv: empty query")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if sortBy == "" {
		sortBy = SortRelevance
	}
	v := url.Values{}
	v.Set("search_query", searchQuery)
	v.Set("start", "0")
	v.Set("max_results", strconv.Itoa(limit))
	v.Set("sortBy", sortBy)
	v.Set("sortOrder", "descending")

	body, err := c.get(ctx, ArxivEndpoint+"?"+v.Encode())
	if err != nil {
		return nil, 0, err
	}
	return ParseAtom(body)
}

// ByID fetches one paper by its arXiv id (with or without a version).
func (c *Arxiv) ByID(ctx context.Context, arxivID string) (Paper, error) {
	v := url.Values{}
	v.Set("id_list", strings.TrimSpace(arxivID))
	v.Set("max_results", "1")
	body, err := c.get(ctx, ArxivEndpoint+"?"+v.Encode())
	if err != nil {
		return Paper{}, err
	}
	ps, _, err := ParseAtom(body)
	if err != nil {
		return Paper{}, err
	}
	if len(ps) == 0 {
		return Paper{}, fmt.Errorf("arxiv: %s not found", arxivID)
	}
	return ps[0], nil
}

func (c *Arxiv) get(ctx context.Context, u string) ([]byte, error) {
	if err := arxivWait(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("arxiv: HTTP %d", resp.StatusCode)
	}
	return body, nil
}

// BuildSearchQuery turns a user query into an arXiv `search_query` expression:
// every term is ANDed under the `all:` field, and "quoted phrases" stay whole.
func BuildSearchQuery(q string) string {
	terms := splitTerms(q)
	if len(terms) == 0 {
		return ""
	}
	parts := make([]string, 0, len(terms))
	for _, t := range terms {
		if strings.ContainsAny(t, " \t") {
			parts = append(parts, `all:"`+t+`"`)
		} else {
			parts = append(parts, "all:"+t)
		}
	}
	return strings.Join(parts, " AND ")
}

// CategoryQuery builds `(cat:cs.CL OR cat:cs.LG)` for the digest.
func CategoryQuery(cats []string) string {
	var parts []string
	for _, c := range cats {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		parts = append(parts, "cat:"+c)
	}
	if len(parts) == 0 {
		return ""
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

// KeywordQuery builds `(all:"llm unlearning" OR all:"model editing")`.
func KeywordQuery(keywords []string) string {
	var parts []string
	for _, k := range keywords {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		parts = append(parts, `all:"`+k+`"`)
	}
	if len(parts) == 0 {
		return ""
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

// splitTerms splits on whitespace, keeping "quoted phrases" together.
func splitTerms(q string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			out = append(out, s)
		}
		cur.Reset()
	}
	for _, r := range q {
		switch {
		case r == '"':
			if inQuote {
				flush()
			}
			inQuote = !inQuote
		case (r == ' ' || r == '\t' || r == '\n') && !inQuote:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// ------------------------------------------------------------------- Atom

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Total   int         `xml:"totalResults"`
	Entries []atomEntry `xml:"entry"`
}

type atomLink struct {
	Href  string `xml:"href,attr"`
	Rel   string `xml:"rel,attr"`
	Type  string `xml:"type,attr"`
	Title string `xml:"title,attr"`
}

type atomEntry struct {
	ID        string `xml:"id"`
	Title     string `xml:"title"`
	Summary   string `xml:"summary"`
	Published string `xml:"published"`
	Updated   string `xml:"updated"`
	Authors   []struct {
		Name string `xml:"name"`
	} `xml:"author"`
	Links      []atomLink `xml:"link"`
	Categories []struct {
		Term string `xml:"term,attr"`
	} `xml:"category"`
	PrimaryCategory struct {
		Term string `xml:"term,attr"`
	} `xml:"primary_category"`
	DOI        string `xml:"doi"`
	JournalRef string `xml:"journal_ref"`
	Comment    string `xml:"comment"`
}

// ParseAtom parses an arXiv Atom feed into papers plus the total hit count.
func ParseAtom(body []byte) ([]Paper, int, error) {
	var f atomFeed
	if err := xml.Unmarshal(body, &f); err != nil {
		return nil, 0, fmt.Errorf("arxiv: parse feed: %w", err)
	}
	out := make([]Paper, 0, len(f.Entries))
	for _, e := range f.Entries {
		p := entryToPaper(e)
		if p.ArxivID == "" {
			continue
		}
		out = append(out, p)
	}
	return out, f.Total, nil
}

func entryToPaper(e atomEntry) Paper {
	raw := arxivIDFromURL(e.ID)
	base := StripVersion(raw)
	p := Paper{
		ID:          ArxivPaperID(base),
		ArxivID:     base,
		DOI:         strings.TrimSpace(e.DOI),
		Title:       collapseSpace(e.Title),
		Abstract:    strings.TrimSpace(collapseSpace(e.Summary)),
		Venue:       strings.TrimSpace(collapseSpace(e.JournalRef)),
		URL:         "https://arxiv.org/abs/" + base,
		PDFURL:      "https://arxiv.org/pdf/" + base,
		PublishedAt: strings.TrimSpace(e.Published),
		Source:      "arxiv",
		Comment:     strings.TrimSpace(collapseSpace(e.Comment)),
	}
	for _, a := range e.Authors {
		if n := collapseSpace(a.Name); n != "" {
			p.Authors = append(p.Authors, n)
		}
	}
	for _, l := range e.Links {
		if l.Title == "pdf" && l.Href != "" {
			p.PDFURL = strings.Replace(l.Href, "http://", "https://", 1)
		}
	}
	if p.Venue == "" && e.PrimaryCategory.Term != "" {
		p.Venue = "arXiv " + e.PrimaryCategory.Term
	}
	if t, err := time.Parse(time.RFC3339, p.PublishedAt); err == nil {
		p.Year = t.Year()
	}
	return p
}

// arxivIDFromURL pulls "2401.00001v2" out of "http://arxiv.org/abs/2401.00001v2".
func arxivIDFromURL(u string) string {
	u = strings.TrimSpace(u)
	if i := strings.Index(u, "/abs/"); i >= 0 {
		return u[i+len("/abs/"):]
	}
	if i := strings.LastIndex(u, "/"); i >= 0 {
		return u[i+1:]
	}
	return u
}

// Categories returns an entry's arXiv categories (used by the digest filter).
func Categories(body []byte) [][]string {
	var f atomFeed
	if xml.Unmarshal(body, &f) != nil {
		return nil
	}
	out := make([][]string, 0, len(f.Entries))
	for _, e := range f.Entries {
		var cats []string
		for _, c := range e.Categories {
			cats = append(cats, c.Term)
		}
		out = append(out, cats)
	}
	return out
}
