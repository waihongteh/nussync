package notify

// Formatting and state for the paper bot's richer command set: /search,
// /save, /download, /library, /done and /start-reading.
//
// notify stays a transport package: it never talks to arXiv, Semantic Scholar
// or the library itself. The app implements PaperExt and hands the bot plain
// PaperItem values, which is also why every formatter below is a pure function
// that can be table-tested with fake data.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"nussync/internal/telegram"
)

// PaperItem is the transport-level view of one paper in a bot listing. It is
// deliberately a flat copy rather than papers.Paper so that notify does not
// depend on the papers package.
type PaperItem struct {
	ID        string
	Title     string
	Authors   []string
	Year      int
	Venue     string
	Citations int
	URL       string

	// Library-only fields; zero for search and digest results.
	Status string
	Page   int
	Pages  int
}

// List kinds. A chat has at most one "last list" (search results or the
// digest, whichever was sent last) plus one "last library listing".
const (
	ListSearch  = "search"
	ListDigest  = "digest"
	ListLibrary = "library"
)

// PaperList is a numbered listing a chat has seen, remembered so that
// /save 3, /download 3 and /done 3 know what "3" means. It is kept in memory
// and mirrored into the kv table so numbering survives a restart.
type PaperList struct {
	Kind  string   `json:"kind"`
	Query string   `json:"query,omitempty"`
	IDs   []string `json:"ids"`
	// Titles are stored alongside so a reply can name the paper without a
	// second lookup.
	Titles []string `json:"titles"`
}

// Item returns the id and title of the 1-based entry n.
func (l PaperList) Item(n int) (id, title string, ok bool) {
	if n < 1 || n > len(l.IDs) {
		return "", "", false
	}
	title = ""
	if n-1 < len(l.Titles) {
		title = l.Titles[n-1]
	}
	return l.IDs[n-1], title, true
}

// NewPaperList builds a list from the items just rendered.
func NewPaperList(kind, query string, items []PaperItem) PaperList {
	l := PaperList{Kind: kind, Query: query}
	for _, it := range items {
		l.IDs = append(l.IDs, it.ID)
		l.Titles = append(l.Titles, it.Title)
	}
	return l
}

func (l PaperList) encode() string {
	b, err := json.Marshal(l)
	if err != nil {
		return ""
	}
	return string(b)
}

func decodePaperList(s string) (PaperList, bool) {
	var l PaperList
	if strings.TrimSpace(s) == "" {
		return l, false
	}
	if json.Unmarshal([]byte(s), &l) != nil || len(l.IDs) == 0 {
		return PaperList{}, false
	}
	return l, true
}

// PaperExt is the app-side half of the richer commands. Every method may block
// on the network; the bot always calls them from the per-update goroutine.
type PaperExt interface {
	// SearchPapers runs the same all-sources search the desktop app uses.
	SearchPapers(ctx context.Context, query string, limit int) ([]PaperItem, error)
	// DigestItems returns today's digest entries in the order they were
	// numbered, so /save and /download work after /paper.
	DigestItems(ctx context.Context) ([]PaperItem, error)
	// SavePaper adds a paper to the library as "toread" and returns its title.
	SavePaper(ctx context.Context, id string) (string, error)
	// DownloadPaperPDF fetches the PDF and returns the local file name.
	DownloadPaperPDF(ctx context.Context, id string) (string, error)
	// LibraryList lists saved papers; status "" means every status.
	LibraryList(ctx context.Context, status string, limit int) ([]PaperItem, error)
	// SetPaperStatus moves a paper to "toread" | "reading" | "done".
	SetPaperStatus(ctx context.Context, id, status string) (string, error)
}

// SearchLimit is how many search hits the bot shows.
const SearchLimit = 5

// LibraryLimit is how many library rows the bot shows.
const LibraryLimit = 15

// PaperHelpMessage lists the paper bot's commands. It replaces the shorter
// digest-only help once the app has installed a PaperExt.
func PaperHelpMessage() string {
	return strings.Join([]string{
		"<b>NUSSync papers</b>",
		"",
		"/paper — today's digest (new arXiv papers + recommendations)",
		"/search &lt;query&gt; — search arXiv + Semantic Scholar, top 5",
		"/save &lt;n&gt; — add entry n of the last list to your library",
		"/download &lt;n&gt; — fetch the PDF for entry n",
		"/library [toread|reading|done] — your saved papers",
		"/reading — papers you are part-way through",
		"/start-reading &lt;n&gt; — mark entry n of the last /library as reading",
		"/done &lt;n&gt; — mark entry n of the last /library as done",
		"/help — this message",
	}, "\n")
}

// authorLine renders the first two authors, with "et al." when there are more.
func authorLine(authors []string) string {
	var kept []string
	for _, a := range authors {
		if a = strings.TrimSpace(a); a != "" {
			kept = append(kept, a)
		}
	}
	if len(kept) == 0 {
		return ""
	}
	suffix := ""
	if len(kept) > 2 {
		kept, suffix = kept[:2], " et al."
	}
	return strings.Join(kept, ", ") + suffix
}

// metaLine renders "2025 · NeurIPS · 12 citations". A paper with no venue is
// labelled "preprint" so the line never collapses to a bare year.
func metaLine(it PaperItem) string {
	var parts []string
	if it.Year > 0 {
		parts = append(parts, fmt.Sprint(it.Year))
	}
	if v := strings.TrimSpace(it.Venue); v != "" {
		parts = append(parts, v)
	} else {
		parts = append(parts, "preprint")
	}
	parts = append(parts, plural(it.Citations, "citation"))
	return strings.Join(parts, " · ")
}

// SearchResultsMessage renders /search: numbered hits with authors, venue,
// citations and a link.
func SearchResultsMessage(query string, items []PaperItem) string {
	q := strings.TrimSpace(query)
	if q == "" {
		return "Usage: <code>/search &lt;query&gt;</code>"
	}
	if len(items) == 0 {
		return "🔍 Nothing found for <b>" + telegram.EscapeHTML(q) + "</b>."
	}
	var sb strings.Builder
	sb.WriteString("🔍 <b>" + telegram.EscapeHTML(q) + "</b>\n")
	for i, it := range items {
		fmt.Fprintf(&sb, "\n<b>%d. %s</b>\n", i+1, telegram.EscapeHTML(it.Title))
		if a := authorLine(it.Authors); a != "" {
			sb.WriteString(telegram.EscapeHTML(a) + "\n")
		}
		sb.WriteString("<i>" + telegram.EscapeHTML(metaLine(it)) + "</i>\n")
		if it.URL != "" {
			sb.WriteString(telegram.EscapeHTML(it.URL) + "\n")
		}
	}
	sb.WriteString("\nReply <code>/save &lt;n&gt;</code> to add one, " +
		"<code>/download &lt;n&gt;</code> for the PDF.")
	return sb.String()
}

// statusLabel turns a stored status into something readable.
func statusLabel(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "toread", "":
		return "to read"
	case "reading":
		return "reading"
	case "done":
		return "done"
	default:
		return strings.ToLower(strings.TrimSpace(s))
	}
}

// NormalizeStatus maps a user-typed filter onto a stored status. ok is false
// when the word is not a status at all.
func NormalizeStatus(s string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "all":
		return "", true
	case "toread", "to-read", "to_read", "unread", "new":
		return "toread", true
	case "reading", "inprogress", "in-progress":
		return "reading", true
	case "done", "read", "finished":
		return "done", true
	}
	return "", false
}

// LibraryMessage renders /library: up to LibraryLimit rows with their status
// and reading position.
func LibraryMessage(status string, items []PaperItem) string {
	head := "📚 <b>Library</b>"
	if status != "" {
		head = "📚 <b>Library — " + statusLabel(status) + "</b>"
	}
	if len(items) == 0 {
		if status != "" {
			return head + "\n\nNothing with that status yet."
		}
		return head + "\n\nYour library is empty. Try <code>/search &lt;query&gt;</code>."
	}
	var sb strings.Builder
	sb.WriteString(head + "\n")
	for i, it := range items {
		fmt.Fprintf(&sb, "\n<b>%d. %s</b>\n", i+1, telegram.EscapeHTML(it.Title))
		line := statusLabel(it.Status)
		if it.Pages > 0 {
			line += fmt.Sprintf(" · page %d/%d", it.Page, it.Pages)
		}
		sb.WriteString("<i>" + telegram.EscapeHTML(line) + "</i>\n")
	}
	sb.WriteString("\n<code>/start-reading &lt;n&gt;</code> · " +
		"<code>/done &lt;n&gt;</code> · <code>/download &lt;n&gt;</code>")
	return sb.String()
}

// SavedMessage confirms /save.
func SavedMessage(n int, title string) string {
	return fmt.Sprintf("✅ Saved <b>%s</b> as <i>to read</i>.\n"+
		"<code>/download %d</code> to fetch the PDF.",
		telegram.EscapeHTML(title), n)
}

// DownloadedMessage confirms /download.
func DownloadedMessage(title, filename string) string {
	if strings.TrimSpace(filename) == "" {
		return "⬇️ Downloaded <b>" + telegram.EscapeHTML(title) + "</b>."
	}
	return "⬇️ <b>" + telegram.EscapeHTML(title) + "</b>\nSaved as <code>" +
		telegram.EscapeHTML(filename) + "</code>."
}

// StatusChangedMessage confirms /done and /start-reading.
func StatusChangedMessage(title, status string) string {
	icon := "📖"
	if statusLabel(status) == "done" {
		icon = "✅"
	}
	return fmt.Sprintf("%s <b>%s</b> is now <i>%s</i>.",
		icon, telegram.EscapeHTML(title), statusLabel(status))
}

// NoListMessage explains that there is nothing numbered to index into.
func NoListMessage(cmd string) string {
	return "There is no recent list to number. Run <code>/search &lt;query&gt;</code> " +
		"or <code>/paper</code> first, then <code>" +
		telegram.EscapeHTML(cmd) + " &lt;n&gt;</code>."
}

// NoLibraryListMessage is the /done and /start-reading equivalent.
func NoLibraryListMessage(cmd string) string {
	return "Run <code>/library</code> first, then <code>" +
		telegram.EscapeHTML(cmd) + " &lt;n&gt;</code>."
}

// OutOfRangeMessage reports an n that no entry has.
func OutOfRangeMessage(n, have int) string {
	unit := "entries"
	if have == 1 {
		unit = "entry"
	}
	return fmt.Sprintf("There is no entry %d — the last list had %d %s.", n, have, unit)
}
