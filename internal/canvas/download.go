package canvas

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Download streams a Canvas file URL to dst, writing to a temp file first and
// renaming on success. It follows the signed-URL redirect but strips the
// Authorization header when the redirect leaves the Canvas host.
//
// Returns the number of bytes written and the SHA-256 of the content.
func (c *Client) Download(ctx context.Context, fileURL, dst string) (int64, string, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, "", err
	}

	origin := hostOf(c.BaseURL)
	hc := &http.Client{
		Timeout: 30 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("canvas: too many redirects")
			}
			// Never leak the bearer token to a different (S3/CDN) host.
			if !strings.EqualFold(req.URL.Host, origin) {
				req.Header.Del("Authorization")
			}
			return nil
		},
	}

	var resp *http.Response
	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
		if err != nil {
			return 0, "", err
		}
		if strings.EqualFold(hostOf(fileURL), origin) {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}
		r, err := hc.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return 0, "", ctx.Err()
			}
			lastErr = err
			if !sleepBackoff(ctx, attempt) {
				return 0, "", ctx.Err()
			}
			continue
		}
		if r.StatusCode == http.StatusTooManyRequests || r.StatusCode >= 500 {
			r.Body.Close()
			lastErr = &APIError{Status: r.StatusCode, Path: fileURL}
			if attempt == c.MaxRetries {
				return 0, "", lastErr
			}
			if !sleepBackoff(ctx, attempt) {
				return 0, "", ctx.Err()
			}
			continue
		}
		if r.StatusCode >= 400 {
			body, _ := io.ReadAll(io.LimitReader(r.Body, 512))
			r.Body.Close()
			return 0, "", &APIError{Status: r.StatusCode, Path: fileURL, Body: strings.TrimSpace(string(body))}
		}
		resp = r
		break
	}
	if resp == nil {
		if lastErr == nil {
			lastErr = fmt.Errorf("canvas: download failed")
		}
		return 0, "", lastErr
	}
	defer resp.Body.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".nussync-*.part")
	if err != nil {
		return 0, "", err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName)
	}()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), resp.Body)
	if err != nil {
		return 0, "", err
	}
	if err := tmp.Close(); err != nil {
		return 0, "", err
	}

	_ = os.Remove(dst) // Windows rename fails if dst exists
	if err := os.Rename(tmpName, dst); err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

func hostOf(raw string) string {
	raw = strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	if i := strings.IndexAny(raw, "/?#"); i >= 0 {
		raw = raw[:i]
	}
	return raw
}
