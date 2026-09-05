package papers

import (
	"fmt"
	"strings"
	"unicode"
)

// BibKey builds a citation key: lowercase first-author surname + year + the
// first meaningful word of the title, e.g. "zhang2024machine".
func BibKey(p Paper) string {
	last := asciiWord(FirstAuthorLast(p.Authors))
	if last == "" {
		last = "anon"
	}
	year := ""
	if p.Year > 0 {
		year = fmt.Sprint(p.Year)
	}
	first := ""
	for _, w := range strings.Fields(p.Title) {
		w = asciiWord(w)
		if w == "" || bibStopWords[w] {
			continue
		}
		first = w
		break
	}
	return strings.ToLower(last + year + first)
}

var bibStopWords = map[string]bool{
	"a": true, "an": true, "the": true, "on": true, "of": true, "in": true,
	"for": true, "and": true, "to": true, "is": true, "are": true, "with": true,
	"towards": true, "toward": true,
}

// asciiWord keeps only letters and digits, lowercased.
func asciiWord(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// BibEntry renders one BibTeX entry. arXiv preprints become @misc with
// eprint/archivePrefix/primaryClass; anything with a DOI becomes @article.
func BibEntry(p Paper) string {
	key := BibKey(p)
	authors := strings.Join(p.Authors, " and ")
	title := bibEscape(p.Title)

	var sb strings.Builder
	if p.DOI != "" && p.Source != "arxiv" {
		fmt.Fprintf(&sb, "@article{%s,\n", key)
		writeField(&sb, "title", title)
		writeField(&sb, "author", bibEscape(authors))
		if p.Venue != "" {
			writeField(&sb, "journal", bibEscape(p.Venue))
		}
		if p.Year > 0 {
			writeField(&sb, "year", fmt.Sprint(p.Year))
		}
		writeField(&sb, "doi", p.DOI)
		if p.URL != "" {
			writeField(&sb, "url", p.URL)
		}
		sb.WriteString("}\n")
		return sb.String()
	}

	fmt.Fprintf(&sb, "@misc{%s,\n", key)
	writeField(&sb, "title", title)
	writeField(&sb, "author", bibEscape(authors))
	if p.Year > 0 {
		writeField(&sb, "year", fmt.Sprint(p.Year))
	}
	if p.ArxivID != "" {
		writeField(&sb, "eprint", p.ArxivID)
		writeField(&sb, "archivePrefix", "arXiv")
		if pc := primaryClass(p.Venue); pc != "" {
			writeField(&sb, "primaryClass", pc)
		}
	}
	if p.DOI != "" {
		writeField(&sb, "doi", p.DOI)
	}
	if p.URL != "" {
		writeField(&sb, "url", p.URL)
	}
	sb.WriteString("}\n")
	return sb.String()
}

// primaryClass pulls "cs.CL" out of a venue string like "arXiv cs.CL".
func primaryClass(venue string) string {
	for _, f := range strings.Fields(venue) {
		if strings.Count(f, ".") == 1 && len(f) >= 4 && len(f) <= 12 &&
			!strings.ContainsAny(f, ",;:()") {
			left, right, _ := strings.Cut(f, ".")
			if left != "" && right != "" && isAlpha(left) && isAlphaNum(right) {
				return f
			}
		}
	}
	return ""
}

func isAlpha(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return s != ""
}

func isAlphaNum(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return s != ""
}

func writeField(sb *strings.Builder, name, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fmt.Fprintf(sb, "  %s = {%s},\n", name, value)
}

// bibEscape neutralises the characters BibTeX treats specially.
func bibEscape(s string) string {
	r := strings.NewReplacer(
		"{", "\\{", "}", "\\}", "$", "\\$", "%", "\\%",
		"&", "\\&", "#", "\\#", "_", "\\_",
	)
	return r.Replace(collapseSpace(s))
}

// BibTeX renders a whole bibliography.
func BibTeX(ps []Paper) string {
	var sb strings.Builder
	seen := map[string]int{}
	for _, p := range ps {
		entry := BibEntry(p)
		key := BibKey(p)
		if n := seen[key]; n > 0 {
			// Disambiguate duplicate keys with a, b, c…
			suffix := string(rune('a' + n - 1))
			entry = strings.Replace(entry, "{"+key+",", "{"+key+suffix+",", 1)
		}
		seen[key]++
		sb.WriteString(entry)
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}
