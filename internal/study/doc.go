package study

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
)

// PageCount returns the number of pages in a PDF, or 0 when the file is not a
// PDF or cannot be parsed. The upstream reader panics on malformed files, so
// the whole call is guarded.
func PageCount(path string) (n int) {
	defer func() {
		if r := recover(); r != nil {
			n = 0
		}
	}()
	if !strings.EqualFold(filepath.Ext(path), ".pdf") {
		return 0
	}
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return 0
	}
	r, err := pdf.NewReader(f, fi.Size())
	if err != nil {
		return 0
	}
	n = r.NumPage()
	if n < 0 {
		n = 0
	}
	return n
}

// readableExts are the formats Claude Code's Read tool opens directly. PDFs are
// rendered page by page; images are described. Office formats are not supported
// by Read, so those fall back to the text NUSSync already extracted for FTS.
var readableExts = map[string]bool{
	".pdf": true, ".txt": true, ".md": true, ".markdown": true, ".csv": true,
	".tsv": true, ".json": true, ".yaml": true, ".yml": true, ".xml": true,
	".html": true, ".htm": true, ".tex": true, ".bib": true, ".log": true,
	".rst": true, ".go": true, ".py": true, ".java": true, ".c": true,
	".h": true, ".cpp": true, ".hpp": true, ".cs": true, ".js": true,
	".ts": true, ".tsx": true, ".jsx": true, ".rs": true, ".rb": true,
	".php": true, ".sh": true, ".ps1": true, ".sql": true, ".r": true,
	".m": true, ".jl": true, ".scala": true, ".kt": true, ".swift": true,
	".ipynb": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".webp": true,
}

// ClaudeCanRead reports whether the Read tool can open this file directly.
func ClaudeCanRead(path string) bool {
	return readableExts[strings.ToLower(filepath.Ext(path))]
}

// MaxFallbackChars caps how much extracted text is inlined into a prompt for a
// format Claude Code cannot open (pptx/docx/xlsx).
const MaxFallbackChars = 120_000

// ClampText trims inlined fallback text to MaxFallbackChars on a rune boundary.
func ClampText(s string) string {
	if len(s) <= MaxFallbackChars {
		return s
	}
	s = s[:MaxFallbackChars]
	for len(s) > 0 && s[len(s)-1]&0xC0 == 0x80 {
		s = s[:len(s)-1]
	}
	return s + "\n…[truncated]"
}
