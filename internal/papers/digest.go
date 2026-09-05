package papers

import (
	"fmt"
	"strings"

	"nussync/internal/telegram"
)

// MaxDigestMessage keeps the Telegram body under the Bot API's 4096-byte cap.
const MaxDigestMessage = 4000

// Digest is one day's paper-of-the-day selection. Reason[i] explains Papers[i].
type Digest struct {
	Date   string
	Papers []Paper
	Reason []string
}

// Reasons a paper made it into a digest.
const (
	ReasonRecommended = "recommended from your library"
	ReasonKeywords    = "recent on arXiv, matches your keywords"
)

// MatchKeywords reports whether the paper's title/abstract/TLDR mentions any of
// the keywords, and returns the ones that matched (in the caller's order).
func MatchKeywords(p Paper, keywords []string) (bool, []string) {
	hay := strings.ToLower(p.Title + "\n" + p.Abstract + "\n" + p.TLDR)
	var hits []string
	for _, k := range keywords {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if strings.Contains(hay, strings.ToLower(k)) {
			hits = append(hits, k)
		}
	}
	return len(hits) > 0, hits
}

// BuildDigest picks up to topN keyword-matching new arXiv papers plus up to
// recN recommendations that are not already in the library.
//
// inLibrary reports whether a paper id is already saved; it also filters the
// arXiv side so the digest never re-suggests something the user has.
func BuildDigest(date string, arxivNew []Paper, keywords []string, recs []Paper,
	recReason string, inLibrary func(id string) bool, topN, recN int) Digest {

	if strings.TrimSpace(recReason) == "" {
		recReason = ReasonRecommended
	}

	if inLibrary == nil {
		inLibrary = func(string) bool { return false }
	}
	d := Digest{Date: date}
	seen := map[string]bool{}

	for _, p := range arxivNew {
		if len(d.Papers) >= topN {
			break
		}
		if seen[p.ID] || inLibrary(p.ID) {
			continue
		}
		ok, hits := MatchKeywords(p, keywords)
		if !ok {
			continue
		}
		seen[p.ID] = true
		d.Papers = append(d.Papers, p)
		d.Reason = append(d.Reason, "new on arXiv — matches "+strings.Join(hits, ", "))
	}

	added := 0
	for _, p := range recs {
		if added >= recN {
			break
		}
		if seen[p.ID] || inLibrary(p.ID) {
			continue
		}
		seen[p.ID] = true
		d.Papers = append(d.Papers, p)
		d.Reason = append(d.Reason, recReason)
		added++
	}
	return d
}

// DigestMessage renders the Telegram HTML body, numbered so /save <n> works.
// The result never exceeds MaxDigestMessage bytes.
func DigestMessage(d Digest) string {
	if len(d.Papers) == 0 {
		return "📄 <b>Papers</b> — nothing new matched your keywords today."
	}
	head := "📄 <b>Paper digest</b> — " + telegram.EscapeHTML(d.Date) + "\n"
	var sb strings.Builder
	sb.WriteString(head)
	for i, p := range d.Papers {
		reason := ""
		if i < len(d.Reason) {
			reason = d.Reason[i]
		}
		block := digestEntry(i+1, p, reason)
		if sb.Len()+len(block) > MaxDigestMessage-60 {
			fmt.Fprintf(&sb, "\n… and %d more.", len(d.Papers)-i)
			break
		}
		sb.WriteString(block)
	}
	sb.WriteString("\nReply <code>/save &lt;n&gt;</code> to add one to your library.")
	return sb.String()
}

func digestEntry(n int, p Paper, reason string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n<b>%d. %s</b>\n", n, telegram.EscapeHTML(p.Title))

	authors := p.Authors
	suffix := ""
	if len(authors) > 3 {
		authors = authors[:3]
		suffix = " et al."
	}
	if len(authors) > 0 {
		sb.WriteString(telegram.EscapeHTML(strings.Join(authors, ", ")) + suffix + "\n")
	}

	meta := ""
	if p.Year > 0 {
		meta = fmt.Sprint(p.Year)
	}
	if p.Venue != "" {
		if meta != "" {
			meta += " · "
		}
		meta += p.Venue
	}
	if p.CitationCount > 0 {
		if meta != "" {
			meta += " · "
		}
		meta += fmt.Sprintf("%d citations", p.CitationCount)
	}
	if meta != "" {
		sb.WriteString("<i>" + telegram.EscapeHTML(meta) + "</i>\n")
	}

	blurb := p.TLDR
	if blurb == "" {
		blurb = p.Abstract
	}
	if blurb = strings.TrimSpace(collapseSpace(blurb)); blurb != "" {
		if len(blurb) > 200 {
			blurb = trimBytes(blurb, 200) + "…"
		}
		sb.WriteString(telegram.EscapeHTML(blurb) + "\n")
	}
	if reason != "" {
		sb.WriteString("<i>" + telegram.EscapeHTML(reason) + "</i>\n")
	}
	if p.URL != "" {
		sb.WriteString(telegram.EscapeHTML(p.URL) + "\n")
	}
	return sb.String()
}

// ReadingMessage renders /reading: the papers currently in progress.
func ReadingMessage(titles []string, pages [][2]int) string {
	if len(titles) == 0 {
		return "📚 Nothing marked as reading. Use the Papers view to start one."
	}
	var sb strings.Builder
	sb.WriteString("📚 <b>Reading now</b>\n")
	for i, t := range titles {
		fmt.Fprintf(&sb, "\n• <b>%s</b>", telegram.EscapeHTML(t))
		if i < len(pages) && pages[i][1] > 0 {
			fmt.Fprintf(&sb, "\n  page %d / %d", pages[i][0], pages[i][1])
		}
	}
	return sb.String()
}

// PaperHelpMessage lists the paper bot's commands.
func PaperHelpMessage() string {
	return strings.Join([]string{
		"<b>NUSSync papers</b>",
		"",
		"/paper — today's digest (new arXiv papers + recommendations)",
		"/save &lt;n&gt; — add entry n of the last digest to your library",
		"/reading — papers you are part-way through",
		"/help — this message",
	}, "\n")
}
