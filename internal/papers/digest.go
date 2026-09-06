package papers

import (
	"fmt"
	"sort"
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

// MinFreshPreprints is how many tier-0 arXiv papers the digest keeps even when
// published work outranks them. "New today" is almost always preprints, so a
// purely score-ordered digest would go stale.
const MinFreshPreprints = 2

// BuildDigest picks up to topN keyword-matching new arXiv papers plus up to
// recN recommendations that are not already in the library. Both halves are
// ordered by Score (venue tier first when prefs.PreferPublished), except that
// the arXiv half always keeps up to MinFreshPreprints preprints.
//
// inLibrary reports whether a paper id is already saved; it also filters the
// arXiv side so the digest never re-suggests something the user has.
func BuildDigest(date string, arxivNew []Paper, keywords []string, recs []Paper,
	recReason string, inLibrary func(id string) bool, topN, recN int,
	prefs VenuePrefs) Digest {

	if strings.TrimSpace(recReason) == "" {
		recReason = ReasonRecommended
	}

	if inLibrary == nil {
		inLibrary = func(string) bool { return false }
	}
	prefs = prefs.Normalised()
	d := Digest{Date: date}
	seen := map[string]bool{}

	// Every keyword match, in the order arXiv gave them (newest first).
	var matched []Paper
	var reasons []string
	for _, p := range arxivNew {
		if seen[p.ID] || inLibrary(p.ID) {
			continue
		}
		ok, hits := MatchKeywords(p, keywords)
		if !ok {
			continue
		}
		seen[p.ID] = true
		AnnotateVenue(&p, prefs)
		matched = append(matched, p)
		reasons = append(reasons, "new on arXiv — matches "+strings.Join(hits, ", "))
	}
	for _, i := range pickDigestFresh(matched, topN, prefs) {
		d.Papers = append(d.Papers, matched[i])
		d.Reason = append(d.Reason, reasons[i])
	}

	ranked := append([]Paper(nil), recs...)
	SortByScore(ranked, prefs)
	added := 0
	for _, p := range ranked {
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

// pickDigestFresh returns the indices of the topN papers to show, best-scoring
// first but with at least MinFreshPreprints preprints kept when the candidate
// list has them.
func pickDigestFresh(ps []Paper, topN int, prefs VenuePrefs) []int {
	if topN <= 0 || len(ps) == 0 {
		return nil
	}
	order := make([]int, len(ps))
	for i := range order {
		order[i] = i
	}
	score := make([]float64, len(ps))
	for i := range ps {
		// The source order is submittedDate desc, so the freshness tie-break
		// is what keeps equal-scoring new papers in arXiv's own order.
		score[i] = Score(ps[i], prefs) + SourceRankBonus*float64(len(ps)-i)/float64(len(ps))
	}
	sort.SliceStable(order, func(a, b int) bool { return score[order[a]] > score[order[b]] })
	if topN > len(order) {
		topN = len(order)
	}
	picked := append([]int(nil), order[:topN]...)

	count := func() int {
		n := 0
		for _, i := range picked {
			if ps[i].VenueTier == 0 {
				n++
			}
		}
		return n
	}
	inPicked := func(i int) bool {
		for _, j := range picked {
			if i == j {
				return true
			}
		}
		return false
	}
	// Swap the weakest published entries for the best-scoring preprints left
	// out, until the floor is met or there is nothing left to swap in.
	for count() < MinFreshPreprints {
		next := -1
		for _, i := range order {
			if ps[i].VenueTier == 0 && !inPicked(i) {
				next = i
				break
			}
		}
		if next < 0 {
			break
		}
		victim := -1
		for k := len(picked) - 1; k >= 0; k-- {
			if ps[picked[k]].VenueTier > 0 {
				victim = k
				break
			}
		}
		if victim < 0 {
			break
		}
		picked[victim] = next
	}
	sort.SliceStable(picked, func(a, b int) bool { return score[picked[a]] > score[picked[b]] })
	return picked
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
