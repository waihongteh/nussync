// Package canvas is a small REST client for the Canvas LMS API.
package canvas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client talks to a Canvas instance with a personal access token.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client

	// MaxRetries bounds retries on 429/5xx.
	MaxRetries int
}

// New builds a client. baseURL may include or omit a trailing slash.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Token:      token,
		HTTP:       &http.Client{Timeout: 60 * time.Second},
		MaxRetries: 4,
	}
}

// APIError carries the HTTP status of a failed Canvas call.
type APIError struct {
	Status int
	Path   string
	Body   string
}

// Error renders one short line, e.g.
//
//	canvas: GET /users/self -> 401 unauthenticated (user authorization required)
//
// The raw JSON body is deliberately dropped: only the API's own status word
// and the FIRST errors[].message survive, so toasts and the Settings error
// strip stay readable. Callers that need the body still have e.Body.
func (e *APIError) Error() string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "canvas: GET %s -> %d", shortPath(e.Path), e.Status)
	if word, msg := explainBody(e.Body); word != "" || msg != "" {
		if word != "" {
			b.WriteString(" " + word)
		}
		if msg != "" {
			b.WriteString(" (" + msg + ")")
		}
	} else if txt := http.StatusText(e.Status); txt != "" {
		b.WriteString(" " + strings.ToLower(txt))
	}
	return b.String()
}

// shortPath strips the scheme/host and the /api/v1 prefix so the message names
// the endpoint rather than repeating the instance URL on every line.
func shortPath(raw string) string {
	p := raw
	if u, err := url.Parse(raw); err == nil && u.Path != "" {
		p = u.Path
	}
	p = strings.TrimPrefix(p, "/api/v1")
	if p == "" {
		p = raw
	}
	if len(p) > 120 {
		p = p[:117] + "..."
	}
	return p
}

// explainBody pulls the status word and the first errors[].message out of a
// Canvas error body. Canvas answers with several shapes:
//
//	{"status":"unauthenticated","errors":[{"message":"user authorization required"}]}
//	{"errors":[{"message":"..."}]}
//	{"errors":{"base":[{"message":"..."}]}}
//	{"message":"..."}
func explainBody(body string) (word, msg string) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", ""
	}
	var v struct {
		Status  string          `json:"status"`
		Message string          `json:"message"`
		Errors  json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return "", oneLine(body)
	}
	word = strings.TrimSpace(v.Status)
	msg = strings.TrimSpace(v.Message)

	type errItem struct {
		Message string `json:"message"`
	}
	if len(v.Errors) > 0 {
		var list []errItem
		if err := json.Unmarshal(v.Errors, &list); err == nil {
			for _, e := range list {
				if m := strings.TrimSpace(e.Message); m != "" && msg == "" {
					msg = m
					break
				}
			}
		} else {
			var byField map[string][]errItem
			if err := json.Unmarshal(v.Errors, &byField); err == nil {
			outer:
				for _, list := range byField {
					for _, e := range list {
						if m := strings.TrimSpace(e.Message); m != "" {
							msg = m
							break outer
						}
					}
				}
			}
		}
	}
	if word == "" && msg == "" {
		return "", oneLine(body)
	}
	return word, oneLine(msg)
}

// oneLine collapses whitespace and caps the length, so nothing in an error can
// break a single-line toast.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 160 {
		s = s[:157] + "..."
	}
	return s
}

// IsPermission reports whether err is a 401/403 (per-course denial).
func IsPermission(err error) bool {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Status == http.StatusUnauthorized || ae.Status == http.StatusForbidden
	}
	return false
}

// IsNotFound reports whether err is a 404.
func IsNotFound(err error) bool {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Status == http.StatusNotFound
	}
	return false
}

func (c *Client) do(ctx context.Context, rawURL string) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Accept", "application/json")

		resp, err := c.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			if !sleepBackoff(ctx, attempt) {
				return nil, ctx.Err()
			}
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			resp.Body.Close()
			lastErr = &APIError{Status: resp.StatusCode, Path: rawURL, Body: strings.TrimSpace(string(body))}
			if attempt == c.MaxRetries {
				break
			}
			if !sleepBackoff(ctx, attempt) {
				return nil, ctx.Err()
			}
			continue
		}

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			resp.Body.Close()
			return nil, &APIError{Status: resp.StatusCode, Path: rawURL, Body: strings.TrimSpace(string(body))}
		}

		respectRateLimit(ctx, resp)
		return resp, nil
	}
	if lastErr == nil {
		lastErr = errors.New("canvas: request failed")
	}
	return nil, lastErr
}

func sleepBackoff(ctx context.Context, attempt int) bool {
	d := time.Duration(1<<attempt) * time.Second
	if d > 16*time.Second {
		d = 16 * time.Second
	}
	d += time.Duration(rand.Intn(400)) * time.Millisecond
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// respectRateLimit pauses briefly when the Canvas leaky bucket is running low.
func respectRateLimit(ctx context.Context, resp *http.Response) {
	v := resp.Header.Get("X-Rate-Limit-Remaining")
	if v == "" {
		return
	}
	rem, err := strconv.ParseFloat(v, 64)
	if err != nil || rem >= 50 {
		return
	}
	d := 2 * time.Second
	if rem < 20 {
		d = 5 * time.Second
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// getJSON fetches a single (non-paginated) resource.
func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	resp, err := c.do(ctx, c.abs(path))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

// getPaged walks all pages of a list endpoint, appending into out (a *[]T).
// fn is called with the raw JSON body of each page.
func (c *Client) getPaged(ctx context.Context, path string, fn func([]byte) error) error {
	next := c.abs(path)
	seen := map[string]bool{}
	for next != "" {
		if seen[next] {
			break // defensive: malformed Link cycle
		}
		seen[next] = true

		resp, err := c.do(ctx, next)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(resp.Body)
		link := resp.Header.Get("Link")
		resp.Body.Close()
		if err != nil {
			return err
		}
		if err := fn(body); err != nil {
			return err
		}
		next = NextLink(link)
	}
	return nil
}

func (c *Client) abs(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return c.BaseURL + path
}

// NextLink extracts the rel="next" URL from an RFC 5988 Link header.
// Returns "" when absent.
func NextLink(header string) string {
	for _, part := range splitLinkHeader(header) {
		segs := strings.Split(part, ";")
		if len(segs) < 2 {
			continue
		}
		raw := strings.TrimSpace(segs[0])
		if !strings.HasPrefix(raw, "<") || !strings.HasSuffix(raw, ">") {
			continue
		}
		u := raw[1 : len(raw)-1]
		for _, attr := range segs[1:] {
			k, v, ok := strings.Cut(strings.TrimSpace(attr), "=")
			if !ok || strings.TrimSpace(k) != "rel" {
				continue
			}
			v = strings.Trim(strings.TrimSpace(v), `"'`)
			if v == "next" {
				if _, err := url.Parse(u); err == nil {
					return u
				}
			}
		}
	}
	return ""
}

// splitLinkHeader splits on commas that are not inside <...>.
func splitLinkHeader(h string) []string {
	var out []string
	depth := 0
	start := 0
	for i, r := range h {
		switch r {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(h[start:i]))
				start = i + 1
			}
		}
	}
	if start < len(h) {
		out = append(out, strings.TrimSpace(h[start:]))
	}
	return out
}
