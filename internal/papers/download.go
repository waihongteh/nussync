package papers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MaxPDFBytes caps a downloaded paper (papers are small; this only guards
// against a mis-typed URL streaming a video).
const MaxPDFBytes = 200 << 20

// DownloadPDF streams a paper PDF to dir/<Filename(p)> and returns the absolute
// path and the byte count. It writes to a .part file and renames, so an
// interrupted download never leaves a half file in place.
func DownloadPDF(ctx context.Context, hc *http.Client, p Paper, dir string) (string, int64, error) {
	if strings.TrimSpace(p.PDFURL) == "" {
		return "", 0, fmt.Errorf("no PDF link for %s", p.ID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, err
	}
	dst := filepath.Join(dir, Filename(p))

	if p.Source == "arxiv" || strings.Contains(p.PDFURL, "arxiv.org") {
		// arXiv PDFs come off the same host as the API and share its limiter.
		if err := arxivWait(ctx); err != nil {
			return "", 0, err
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.PDFURL, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", UserAgent)
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Minute}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("download %s: HTTP %d", p.PDFURL, resp.StatusCode)
	}

	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", 0, err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, MaxPDFBytes))
	cerr := f.Close()
	if err != nil {
		os.Remove(tmp)
		return "", 0, err
	}
	if cerr != nil {
		os.Remove(tmp)
		return "", 0, cerr
	}
	// Windows rename fails over an existing file.
	_ = os.Remove(dst)
	if err := os.Rename(tmp, dst); err != nil {
		return "", 0, err
	}
	return dst, n, nil
}
