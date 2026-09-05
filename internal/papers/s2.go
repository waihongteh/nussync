package papers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// S2Endpoint is the Semantic Scholar Graph API root (no API key: the public
// tier is heavily rate limited, so every failure here must be non-fatal).
const S2Endpoint = "https://api.semanticscholar.org/graph/v1"

// S2Fields is the field list requested for every paper object.
const S2Fields = "title,abstract,year,venue,authors,citationCount,openAccessPdf,tldr,externalIds,url,publicationDate"

// S2MaxRetries is how many times a 429 is retried before giving up.
const S2MaxRetries = 3

// Cache stores GET response bodies. Implemented by the store.
type Cache interface {
	// Get returns a cached body no older than maxAge.
	Get(key string, maxAge time.Duration) (string, bool)
	// Put stores a body under key with the current timestamp.
	Put(key, body string)
}

// CacheTTL is how long Semantic Scholar GETs are reused.
const CacheTTL = 24 * time.Hour

// S2 queries the Semantic Scholar Graph API.
type S2 struct {
	HTTP  *http.Client
	Cache Cache
	// Sleep is the backoff sleeper (overridable in tests).
	Sleep func(context.Context, time.Duration) error
}

// NewS2 builds a client.
func NewS2(c Cache) *S2 {
	return &S2{HTTP: &http.Client{Timeout: 45 * time.Second}, Cache: c}
}

// Search runs a paper search.
func (c *S2) Search(ctx context.Context, query string, limit int) ([]Paper, int, error) {
	if strings.TrimSpace(query) == "" {
		return nil, 0, fmt.Errorf("s2: empty query")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	v := url.Values{}
	v.Set("query", query)
	v.Set("limit", strconv.Itoa(limit))
	v.Set("fields", S2Fields)
	body, err := c.get(ctx, S2Endpoint+"/paper/search?"+v.Encode())
	if err != nil {
		return nil, 0, err
	}
	return ParseS2Search(body)
}

// Paper fetches one paper. id may be a raw S2 paper id or "arXiv:2401.00001".
func (c *S2) Paper(ctx context.Context, id string) (Paper, error) {
	v := url.Values{}
	v.Set("fields", S2Fields)
	body, err := c.get(ctx, S2Endpoint+"/paper/"+url.PathEscape(id)+"?"+v.Encode())
	if err != nil {
		return Paper{}, err
	}
	var raw s2Paper
	if err := json.Unmarshal(body, &raw); err != nil {
		return Paper{}, fmt.Errorf("s2: parse paper: %w", err)
	}
	return raw.toPaper(), nil
}

// Citations lists papers citing id.
func (c *S2) Citations(ctx context.Context, id string, limit int) ([]Paper, error) {
	return c.edge(ctx, id, "citations", limit)
}

// References lists papers cited by id.
func (c *S2) References(ctx context.Context, id string, limit int) ([]Paper, error) {
	return c.edge(ctx, id, "references", limit)
}

func (c *S2) edge(ctx context.Context, id, kind string, limit int) ([]Paper, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	v := url.Values{}
	v.Set("limit", strconv.Itoa(limit))
	v.Set("fields", S2Fields)
	body, err := c.get(ctx, S2Endpoint+"/paper/"+url.PathEscape(id)+"/"+kind+"?"+v.Encode())
	if err != nil {
		return nil, err
	}
	return ParseS2Edges(body)
}

// Recommendations asks for papers similar to the given library papers.
func (c *S2) Recommendations(ctx context.Context, positiveIDs []string, limit int) ([]Paper, error) {
	if len(positiveIDs) == 0 {
		return nil, fmt.Errorf("s2: no seed papers")
	}
	if limit <= 0 {
		limit = 10
	}
	payload, _ := json.Marshal(map[string]any{"positivePaperIds": positiveIDs})
	u := "https://api.semanticscholar.org/recommendations/v1/papers?" +
		url.Values{"limit": {strconv.Itoa(limit)}, "fields": {S2Fields}}.Encode()

	body, err := c.do(ctx, http.MethodPost, u, payload)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		RecommendedPapers []s2Paper `json:"recommendedPapers"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, fmt.Errorf("s2: parse recommendations: %w", err)
	}
	out := make([]Paper, 0, len(wrap.RecommendedPapers))
	for _, r := range wrap.RecommendedPapers {
		out = append(out, r.toPaper())
	}
	return out, nil
}

// get performs a cached GET (24h) with 429 backoff.
func (c *S2) get(ctx context.Context, u string) ([]byte, error) {
	if c.Cache != nil {
		if b, ok := c.Cache.Get(u, CacheTTL); ok {
			return []byte(b), nil
		}
	}
	body, err := c.do(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if c.Cache != nil {
		c.Cache.Put(u, string(body))
	}
	return body, nil
}

// do issues one request, retrying up to S2MaxRetries times on HTTP 429 or 5xx
// with exponential backoff (1s, 2s, 4s).
func (c *S2) do(ctx context.Context, method, u string, payload []byte) ([]byte, error) {
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	sleep := c.Sleep
	if sleep == nil {
		sleep = sleepCtx
	}

	var lastErr error
	for attempt := 0; attempt <= S2MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			if err := sleep(ctx, backoff); err != nil {
				return nil, err
			}
		}
		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, rdr)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", UserAgent)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			continue
		}
		body, rerr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		resp.Body.Close()
		if rerr != nil {
			lastErr = rerr
			continue
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			return body, nil
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			lastErr = fmt.Errorf("s2: HTTP %d (rate limited)", resp.StatusCode)
			continue
		default:
			return nil, fmt.Errorf("s2: HTTP %d: %s", resp.StatusCode,
				firstLine(strings.TrimSpace(string(body))))
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("s2: request failed")
	}
	return nil, lastErr
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// ------------------------------------------------------------------- JSON

type s2Paper struct {
	PaperID  string `json:"paperId"`
	Title    string `json:"title"`
	Abstract string `json:"abstract"`
	Year     int    `json:"year"`
	Venue    string `json:"venue"`
	Authors  []struct {
		Name string `json:"name"`
	} `json:"authors"`
	CitationCount int `json:"citationCount"`
	OpenAccessPDF *struct {
		URL string `json:"url"`
	} `json:"openAccessPdf"`
	TLDR *struct {
		Text string `json:"text"`
	} `json:"tldr"`
	ExternalIDs struct {
		ArXiv string `json:"ArXiv"`
		DOI   string `json:"DOI"`
	} `json:"externalIds"`
	URL             string `json:"url"`
	PublicationDate string `json:"publicationDate"`
}

func (r s2Paper) toPaper() Paper {
	p := Paper{
		ID:            S2PaperID(r.PaperID),
		S2ID:          r.PaperID,
		ArxivID:       StripVersion(r.ExternalIDs.ArXiv),
		DOI:           r.ExternalIDs.DOI,
		Title:         collapseSpace(r.Title),
		Abstract:      strings.TrimSpace(r.Abstract),
		Year:          r.Year,
		Venue:         strings.TrimSpace(r.Venue),
		CitationCount: r.CitationCount,
		URL:           r.URL,
		PublishedAt:   r.PublicationDate,
		Source:        "s2",
	}
	for _, a := range r.Authors {
		if n := collapseSpace(a.Name); n != "" {
			p.Authors = append(p.Authors, n)
		}
	}
	if r.TLDR != nil {
		p.TLDR = strings.TrimSpace(r.TLDR.Text)
	}
	if r.OpenAccessPDF != nil {
		p.PDFURL = r.OpenAccessPDF.URL
	}
	if p.PDFURL == "" && p.ArxivID != "" {
		p.PDFURL = "https://arxiv.org/pdf/" + p.ArxivID
	}
	if p.URL == "" && p.S2ID != "" {
		p.URL = "https://www.semanticscholar.org/paper/" + p.S2ID
	}
	return p
}

// ParseS2Search parses a /paper/search response.
func ParseS2Search(body []byte) ([]Paper, int, error) {
	var wrap struct {
		Total int       `json:"total"`
		Data  []s2Paper `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, 0, fmt.Errorf("s2: parse search: %w", err)
	}
	out := make([]Paper, 0, len(wrap.Data))
	for _, r := range wrap.Data {
		if r.PaperID == "" {
			continue
		}
		out = append(out, r.toPaper())
	}
	return out, wrap.Total, nil
}

// ParseS2Edges parses a /citations or /references response, whose rows wrap the
// real paper in "citingPaper" / "citedPaper".
func ParseS2Edges(body []byte) ([]Paper, error) {
	var wrap struct {
		Data []struct {
			CitingPaper *s2Paper `json:"citingPaper"`
			CitedPaper  *s2Paper `json:"citedPaper"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, fmt.Errorf("s2: parse edges: %w", err)
	}
	out := make([]Paper, 0, len(wrap.Data))
	for _, row := range wrap.Data {
		r := row.CitingPaper
		if r == nil {
			r = row.CitedPaper
		}
		if r == nil || r.PaperID == "" {
			continue
		}
		out = append(out, r.toPaper())
	}
	return out, nil
}
