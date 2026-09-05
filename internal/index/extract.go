// Package index extracts plain text from downloaded files for FTS indexing.
package index

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// MaxChars caps extracted text per file.
const MaxChars = 200_000

// plainExts are read verbatim as UTF-8 text.
var plainExts = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".csv": true, ".tsv": true,
	".json": true, ".yaml": true, ".yml": true, ".xml": true, ".html": true,
	".htm": true, ".tex": true, ".bib": true, ".log": true, ".rst": true,
	".go": true, ".py": true, ".java": true, ".c": true, ".h": true, ".cpp": true,
	".hpp": true, ".cs": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true,
	".rs": true, ".rb": true, ".php": true, ".sh": true, ".ps1": true, ".sql": true,
	".r": true, ".m": true, ".jl": true, ".scala": true, ".kt": true, ".swift": true,
	".ipynb": true, ".svelte": true, ".vue": true, ".css": true, ".scss": true,
}

// Supported reports whether Extract can pull text out of this file type.
func Supported(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".pdf", ".docx", ".pptx", ".xlsx":
		return true
	}
	return plainExts[ext]
}

// Extract returns plain text for path, or "" for unsupported/unreadable files.
// It never panics: malformed documents yield whatever was recovered.
func Extract(path string) (text string) {
	defer func() {
		if r := recover(); r != nil {
			text = ""
		}
	}()

	ext := strings.ToLower(filepath.Ext(path))
	var out string
	switch ext {
	case ".pdf":
		out = extractPDF(path)
	case ".docx":
		out = extractZipXML(path, []string{"word/document.xml"}, "word/footnotes.xml")
	case ".pptx":
		out = extractZipXML(path, []string{"ppt/slides/"}, "ppt/notesSlides/")
	case ".xlsx":
		out = extractZipXML(path, []string{"xl/sharedStrings.xml"}, "")
	default:
		if plainExts[ext] {
			out = extractPlain(path)
		}
	}
	return clamp(normalizeSpace(out))
}

func extractPlain(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 4*MaxChars))
	if err != nil {
		return ""
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return "" // binary masquerading as text
	}
	return string(b)
}

// extractZipXML unzips an OOXML container and strips tags from every entry
// whose name matches one of the prefixes (extra is an optional extra prefix).
func extractZipXML(path string, prefixes []string, extra string) string {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return ""
	}
	defer zr.Close()

	if extra != "" {
		prefixes = append(prefixes, extra)
	}
	var sb strings.Builder
	for _, f := range zr.File {
		if sb.Len() > MaxChars {
			break
		}
		if !strings.HasSuffix(strings.ToLower(f.Name), ".xml") {
			continue
		}
		matched := false
		for _, p := range prefixes {
			if f.Name == p || strings.HasPrefix(f.Name, p) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(rc, 8*1024*1024))
		rc.Close()
		if err != nil {
			continue
		}
		sb.WriteString(StripXML(b))
		sb.WriteByte(' ')
	}
	return sb.String()
}

// StripXML returns the character data of an XML document, with a space between
// elements so adjacent runs do not glue together.
func StripXML(b []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity

	var sb strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.CharData:
			sb.Write(t)
		case xml.EndElement:
			// Paragraph/row/cell/break boundaries become whitespace.
			switch t.Name.Local {
			case "p", "t", "tr", "tc", "br", "si", "r":
				sb.WriteByte(' ')
			}
		}
		if sb.Len() > 4*MaxChars {
			break
		}
	}
	return sb.String()
}

// normalizeSpace collapses runs of whitespace and drops control characters.
func normalizeSpace(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	space := true // leading trim
	for _, r := range s {
		if r == '�' {
			continue
		}
		if unicode.IsSpace(r) {
			if !space {
				sb.WriteByte(' ')
				space = true
			}
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		sb.WriteRune(r)
		space = false
	}
	return strings.TrimSpace(sb.String())
}

func clamp(s string) string {
	if len(s) <= MaxChars {
		return s
	}
	s = s[:MaxChars]
	// avoid cutting a UTF-8 rune in half
	for len(s) > 0 && !isRuneStart(s[len(s)-1]) {
		s = s[:len(s)-1]
	}
	return s
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }
