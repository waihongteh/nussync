package notify

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"nussync/internal/store"
	"nussync/internal/telegram"
)

// MaxMessage is the largest body we hand to sendMessage. Telegram's hard limit
// is 4096 UTF-8 bytes; we stay under it and split on line boundaries.
const MaxMessage = 4000

// HelpMessage lists the two-way commands.
func HelpMessage() string {
	return strings.Join([]string{
		"<b>NUSSync</b> commands",
		"",
		"/due — next 10 deadlines you have not submitted",
		"/new — files that changed in the last 24 hours",
		"/files &lt;query&gt; — search your synced files",
		"/sync — run a sync now",
		"/grades — your 10 most recent grades",
		"/help — this message",
	}, "\n")
}

// dueBucket labels a deadline relative to now, in local time.
func dueBucket(due, now time.Time) string {
	due, now = due.Local(), now.Local()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch {
	case due.Before(today):
		return "Overdue"
	case due.Before(today.AddDate(0, 0, 1)):
		return "Today"
	case due.Before(today.AddDate(0, 0, 2)):
		return "Tomorrow"
	case due.Before(today.AddDate(0, 0, 7)):
		return "This week"
	default:
		return "Later"
	}
}

var bucketOrder = []string{"Overdue", "Today", "Tomorrow", "This week", "Later"}

// DueMessage renders the /due reply: up to limit unsubmitted deadlines,
// soonest first, grouped by how urgent they are.
func DueMessage(ds []store.Deadline, now time.Time, limit int) string {
	if limit <= 0 {
		limit = 10
	}
	type item struct {
		d   store.Deadline
		due time.Time
	}
	var items []item
	for _, d := range ds {
		if d.Submitted {
			continue
		}
		due, err := time.Parse(time.RFC3339, d.DueAt)
		if err != nil {
			continue
		}
		items = append(items, item{d, due})
	}
	if len(items) == 0 {
		return "✅ Nothing unsubmitted is due. Enjoy it."
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].due.Before(items[j].due) })
	if len(items) > limit {
		items = items[:limit]
	}

	grouped := map[string][]string{}
	for _, it := range items {
		rel := "in " + Humanize(it.due.Sub(now))
		if it.due.Before(now) {
			rel = Humanize(now.Sub(it.due)) + " ago"
		}
		grouped[dueBucket(it.due, now)] = append(grouped[dueBucket(it.due, now)],
			fmt.Sprintf("• <b>%s</b> %s — %s (%s)",
				telegram.EscapeHTML(it.d.CourseCode),
				telegram.EscapeHTML(it.d.Title),
				FormatDue(it.due), rel))
	}

	out := []string{"⏰ <b>Upcoming deadlines</b>"}
	for _, b := range bucketOrder {
		lines := grouped[b]
		if len(lines) == 0 {
			continue
		}
		out = append(out, "", "<b>"+b+"</b>")
		out = append(out, lines...)
	}
	return strings.Join(out, "\n")
}

// NewFilesMessage renders the /new reply: files changed since the cutoff,
// grouped by course, capped at limit entries.
func NewFilesMessage(files []store.FeedFile, limit int) string {
	if limit <= 0 {
		limit = 30
	}
	if len(files) == 0 {
		return "📁 No new or updated files in the last 24 hours."
	}
	if len(files) > limit {
		files = files[:limit]
	}

	var order []string
	byCourse := map[string][]string{}
	for _, f := range files {
		code := f.CourseCode
		if code == "" {
			code = "Other"
		}
		if _, seen := byCourse[code]; !seen {
			order = append(order, code)
		}
		tag := "updated"
		if f.New {
			tag = "new"
		}
		byCourse[code] = append(byCourse[code],
			fmt.Sprintf("• %s <i>(%s)</i>", telegram.EscapeHTML(f.File.Name), tag))
	}

	out := []string{fmt.Sprintf("📁 <b>%s in the last 24 hours</b>", plural(len(files), "file"))}
	for _, code := range order {
		out = append(out, "", "<b>"+telegram.EscapeHTML(code)+"</b>")
		out = append(out, byCourse[code]...)
	}
	return strings.Join(out, "\n")
}

// FilesMessage renders the /files reply: the top hits as "course · name".
func FilesMessage(query string, hits []store.Hit, limit int) string {
	if limit <= 0 {
		limit = 8
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return "Usage: <code>/files &lt;query&gt;</code>"
	}
	if len(hits) == 0 {
		return "🔍 No files match " + telegram.EscapeHTML(q) + "."
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := []string{"🔍 <b>" + telegram.EscapeHTML(q) + "</b>"}
	for _, h := range hits {
		code := h.CourseCode
		if code == "" {
			code = "?"
		}
		out = append(out, fmt.Sprintf("• <b>%s</b> · %s",
			telegram.EscapeHTML(code), telegram.EscapeHTML(h.File.Name)))
	}
	return strings.Join(out, "\n")
}

// GradesMessage renders the /grades reply: the most recent graded submissions.
// Grade.Mean is only shown when the assignment detail cache has one.
func GradesMessage(gs []store.Grade, limit int) string {
	if limit <= 0 {
		limit = 10
	}
	if len(gs) == 0 {
		return "📊 No grades yet."
	}
	if len(gs) > limit {
		gs = gs[:limit]
	}
	out := []string{"📊 <b>Recent grades</b>"}
	for _, g := range gs {
		line := fmt.Sprintf("• <b>%s</b> %s — %s/%s",
			telegram.EscapeHTML(g.CourseCode), telegram.EscapeHTML(g.Title),
			trimFloat(g.Score), trimFloat(g.Possible))
		if g.Mean > 0 {
			line += fmt.Sprintf(" <i>(class mean %s)</i>", trimFloat(g.Mean))
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// SplitMessage breaks a body into chunks of at most max UTF-8 bytes, cutting on
// newlines where possible so HTML tags are never split mid-element. A single
// line longer than max is hard-cut on a rune boundary.
func SplitMessage(s string, max int) []string {
	if max <= 0 {
		max = MaxMessage
	}
	if len(s) <= max {
		return []string{s}
	}
	var out []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, line := range strings.Split(s, "\n") {
		for len(line) > max {
			flush()
			out = append(out, trimRunes(line[:max], max))
			line = line[len(trimRunes(line[:max], max)):]
		}
		// +1 for the newline we are about to add.
		if cur.Len() > 0 && cur.Len()+1+len(line) > max {
			flush()
		}
		if cur.Len() > 0 {
			cur.WriteByte('\n')
		}
		cur.WriteString(line)
	}
	flush()
	return out
}

// ParseCommand splits "/files week 3@bot" into ("/files", "week 3").
// Returns ok=false when the text is not a command.
func ParseCommand(text string) (cmd, args string, ok bool) {
	t := strings.TrimSpace(text)
	if !strings.HasPrefix(t, "/") {
		return "", "", false
	}
	cmd, args, _ = strings.Cut(t, " ")
	// Group chats address commands as "/due@nuscanvassync_bot".
	if at := strings.IndexByte(cmd, '@'); at > 0 {
		cmd = cmd[:at]
	}
	return strings.ToLower(cmd), strings.TrimSpace(args), true
}
