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

// CourseFolder is the on-disk directory name for a course code. Cross-listed
// codes such as "TR3202S/TR3202T/ETP3201S/ETP3201T/ETP3206L/ETP3201I" would
// otherwise become an unusable 50-character folder, so only the first segment
// is used. The full code is still what the DB and UI show.
func CourseFolder(courseCode string) string {
	code := strings.ReplaceAll(courseCode, "\\", "/")
	if i := strings.Index(code, "/"); i > 0 {
		code = code[:i]
	}
	return SanitizeSegment(code)
}

// LegacyCourseFolder is the pre-2026-09 folder name, in which "/" was replaced
// by "_" and every code was kept. Used only to migrate existing libraries.
func LegacyCourseFolder(courseCode string) string {
	return SanitizeSegment(strings.ReplaceAll(
		strings.ReplaceAll(courseCode, "\\", "/"), "/", "_"))
}

// AbsPathFor joins the sync root, course folder and relative path into a native
// absolute path.
func AbsPathFor(syncDir, courseCode, rel string) string {
	segs := append([]string{syncDir, CourseFolder(courseCode)},
		strings.Split(SanitizeRel(rel), "/")...)
	return filepath.Join(segs...)
}

// RelPathForPage computes the course-relative destination for a file that was
// only discovered as a link inside an HTML body. dir is the already-composed
// logical directory ("Modules/<module>/<page>", "Pages/<page>",
// "Assignments/<name>", "Announcements/<title>"); every segment is sanitized.
func RelPathForPage(dir, name string) string {
	d := SanitizeRel(dir)
	n := SanitizeSegment(name)
	if d == "" {
		return n
	}
	return d + "/" + n
}
