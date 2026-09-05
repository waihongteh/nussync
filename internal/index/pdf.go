package index

import (
	"io"
	"os"
	"strings"

	"github.com/ledongthuc/pdf"
)

// extractPDF pulls text out of a PDF. The upstream library panics on some
// malformed files, so every stage is guarded by recover.
func extractPDF(path string) (out string) {
	defer func() {
		if r := recover(); r != nil {
			// keep whatever we managed to accumulate
		}
	}()

	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return ""
	}
	r, err := pdf.NewReader(f, fi.Size())
	if err != nil {
		return ""
	}

	var sb strings.Builder
	// Try the whole-document reader first (fast path).
	if s, ok := wholeDoc(r); ok && strings.TrimSpace(s) != "" {
		return s
	}

	n := r.NumPage()
	for i := 1; i <= n; i++ {
		if sb.Len() > MaxChars {
			break
		}
		sb.WriteString(pageText(r, i))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func wholeDoc(r *pdf.Reader) (s string, ok bool) {
	defer func() {
		if rec := recover(); rec != nil {
			s, ok = "", false
		}
	}()
	rd, err := r.GetPlainText()
	if err != nil {
		return "", false
	}
	b, err := io.ReadAll(io.LimitReader(rd, 4*MaxChars))
	if err != nil {
		return "", false
	}
	return string(b), true
}

func pageText(r *pdf.Reader, i int) (s string) {
	defer func() {
		if rec := recover(); rec != nil {
			s = ""
		}
	}()
	p := r.Page(i)
	if p.V.IsNull() {
		return ""
	}
	txt, err := p.GetPlainText(nil)
	if err != nil {
		return ""
	}
	return txt
}
