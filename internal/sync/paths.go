package sync

import (
	"path/filepath"
	"strings"
)

// reservedNames are Windows device names that cannot be used as a path segment.
var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// SanitizeSegment makes one path component safe on Windows: it replaces the
// illegal characters <>:"/\|?* and control chars with "_", trims trailing dots
// and spaces, escapes reserved device names, and caps the length at 120 bytes
// (preserving the extension). Empty input yields "_".
func SanitizeSegment(s string) string {
	s = strings.TrimSpace(s)
	var sb strings.Builder
	sb.Grow(len(s))
	for _, r := range s {
		switch {
		case r < 0x20 || r == 0x7f:
			sb.WriteByte('_')
		case strings.ContainsRune(`<>:"/\|?*`, r):
			sb.WriteByte('_')
		default:
			sb.WriteRune(r)
		}
	}
	out := strings.TrimRight(sb.String(), " .")
	out = strings.TrimSpace(out)
	if out == "" {
		return "_"
	}

	base := out
	if i := strings.LastIndex(out, "."); i > 0 {
		base = out[:i]
	}
	if reservedNames[strings.ToUpper(base)] {
		out = "_" + out
	}

	return truncateName(out, 120)
}

// truncateName shortens a file name to max bytes, keeping its extension.
func truncateName(s string, max int) string {
	if len(s) <= max {
		return s
	}
	ext := filepath.Ext(s)
	if len(ext) > 16 {
		ext = ""
	}
	keep := max - len(ext)
	if keep < 1 {
		return trimRunes(s, max)
	}
	return trimRunes(s[:len(s)-len(ext)], keep) + ext
}

func trimRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && s[len(s)-1]&0xC0 == 0x80 {
		s = s[:len(s)-1]
	}
	return strings.TrimRight(s, " .")
}

// SanitizeRel sanitizes each segment of a "/"-separated relative path and
// returns it joined with "/". "." and ".." segments are dropped.
func SanitizeRel(rel string) string {
	rel = strings.ReplaceAll(rel, "\\", "/")
	parts := strings.Split(rel, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "." || p == ".." {
			continue
		}
		out = append(out, SanitizeSegment(p))
	}
	return strings.Join(out, "/")
}

// RelPathFor computes the course-relative destination path (forward slashes)
// for a file. Files-tab files keep their folder path; module-only files land
// under "Modules/<module title>/".
func RelPathFor(source, folderPath, module, name string) string {
	name = SanitizeSegment(name)
	if source == SourceModules {
		mod := SanitizeSegment(module)
		if mod == "_" || module == "" {
			return "Modules/" + name
		}
		return "Modules/" + mod + "/" + name
	}
	fp := SanitizeRel(folderPath)
	if fp == "" {
		return name
	}
	return fp + "/" + name
}

// AbsPathFor joins the sync root, course code and relative path into a native
// absolute path.
func AbsPathFor(syncDir, courseCode, rel string) string {
	segs := append([]string{syncDir, SanitizeSegment(courseCode)},
		strings.Split(SanitizeRel(rel), "/")...)
	return filepath.Join(segs...)
}
