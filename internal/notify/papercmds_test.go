package notify

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeExt is a PaperExt backed by fixed data, so the command handler can be
// driven end to end without arXiv, Semantic Scholar or SQLite.
type fakeExt struct {
	search  []PaperItem
	digest  []PaperItem
	library []PaperItem

	searchErr error
	saved     []string
	dl        []string
	status    map[string]string
	lastQuery string
	lastLimit int
}

func newFakeExt() *fakeExt {
	return &fakeExt{
		search: []PaperItem{
			{ID: "arxiv:2506.13181", Title: "Align-then-Unlearn",
				Authors: []string{"Spohn", "Meier", "Brox", "Fischer"},
				Year:    2025, Citations: 3, URL: "https://arxiv.org/abs/2506.13181"},
			{ID: "s2:abc", Title: "Machine Unlearning at Scale",
				Authors: []string{"Bourtoule"}, Year: 2021, Venue: "IEEE S&P",
				Citations: 1, URL: "https://example.org/2"},
			{ID: "arxiv:3", Title: "Third <paper>"},
			{ID: "arxiv:4", Title: "Fourth"},
			{ID: "arxiv:5", Title: "Fifth"},
			{ID: "arxiv:6", Title: "Sixth — should be trimmed"},
		},
		digest: []PaperItem{
			{ID: "arxiv:d1", Title: "Digest One"},
			{ID: "arxiv:d2", Title: "Digest Two"},
		},
		library: []PaperItem{
			{ID: "arxiv:l1", Title: "Lib One", Status: "reading", Page: 4, Pages: 12},
			{ID: "arxiv:l2", Title: "Lib Two", Status: "toread"},
		},
		status: map[string]string{},
	}
}

func (f *fakeExt) SearchPapers(_ context.Context, q string, limit int) ([]PaperItem, error) {
	f.lastQuery, f.lastLimit = q, limit
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.search, nil
}
func (f *fakeExt) DigestItems(context.Context) ([]PaperItem, error) { return f.digest, nil }
func (f *fakeExt) SavePaper(_ context.Context, id string) (string, error) {
	f.saved = append(f.saved, id)
	return "Saved " + id, nil
}
func (f *fakeExt) DownloadPaperPDF(_ context.Context, id string) (string, error) {
	f.dl = append(f.dl, id)
	return "2025 - Spohn - Align.pdf", nil
}
func (f *fakeExt) LibraryList(_ context.Context, status string, _ int) ([]PaperItem, error) {
	if status == "" {
		return f.library, nil
	}
	var out []PaperItem
	for _, it := range f.library {
		if it.Status == status {
			out = append(out, it)
		}
	}
	return out, nil
}
func (f *fakeExt) SetPaperStatus(_ context.Context, id, status string) (string, error) {
	f.status[id] = status
	return "Lib " + id, nil
}

// newTestBot builds a bot with no store: list memory then lives only in the
// in-memory map, which is what the kv table mirrors in production.
func newTestBot(ext PaperExt) *PaperBot {
	b := &PaperBot{}
	b.SetExt(ext)
	b.Reading = func(context.Context) (string, error) { return "📚 <b>Reading now</b>", nil }
	b.Digest = func(context.Context) (string, error) { return "📄 <b>Paper digest</b>", nil }
	return b
}

func TestParsePaperCommands(t *testing.T) {
	cases := []struct {
		in        string
		cmd, args string
		ok        bool
	}{
		{"/search LLM unlearning", "/search", "LLM unlearning", true},
		{"/SEARCH@paper_trackerrr_bot  machine unlearning ", "/search", "machine unlearning", true},
		{"/download 3", "/download", "3", true},
		{"/library toread", "/library", "toread", true},
		{"/start-reading 2", "/start-reading", "2", true},
		{"/read 2", "/read", "2", true},
		{"not a command", "", "", false},
	}
	for _, c := range cases {
		cmd, args, ok := ParseCommand(c.in)
		if ok != c.ok || cmd != c.cmd || args != c.args {
			t.Errorf("ParseCommand(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.in, cmd, args, ok, c.cmd, c.args, c.ok)
		}
	}
}

func TestNormalizeStatus(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"", "", true}, {"all", "", true}, {"toread", "toread", true},
		{"To-Read", "toread", true}, {"reading", "reading", true},
		{"DONE", "done", true}, {"banana", "", false},
	}
	for _, c := range cases {
		got, ok := NormalizeStatus(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("NormalizeStatus(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestSearchResultsMessage(t *testing.T) {
	f := newFakeExt()
	got := SearchResultsMessage("LLM unlearning", f.search[:2])

	for _, want := range []string{
		"<b>1. Align-then-Unlearn</b>",
		"Spohn, Meier et al.", // first two authors only
		"2025 · preprint · 3 citations",
		"https://arxiv.org/abs/2506.13181",
		"<b>2. Machine Unlearning at Scale</b>",
		"2021 · IEEE S&amp;P · 1 citation", // singular, and HTML-escaped
		"/save",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("SearchResultsMessage missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Brox") {
		t.Errorf("author list not trimmed to two:\n%s", got)
	}
	if len(got) >= MaxMessage {
		t.Errorf("message is %d bytes, over the %d limit", len(got), MaxMessage)
	}
}

func TestSearchResultsMessageEmpty(t *testing.T) {
	if got := SearchResultsMessage("  ", nil); !strings.Contains(got, "Usage") {
		t.Errorf("blank query should show usage, got %q", got)
	}
	if got := SearchResultsMessage("nope", nil); !strings.Contains(got, "Nothing found") {
		t.Errorf("no hits should say so, got %q", got)
	}
}

func TestHandleSearchRemembersResults(t *testing.T) {
	f := newFakeExt()
	b := newTestBot(f)
	ctx := context.Background()

	got := b.Handle(ctx, "chat1", "/search", "LLM unlearning")
	if f.lastQuery != "LLM unlearning" || f.lastLimit != SearchLimit {
		t.Fatalf("search called with (%q,%d), want (%q,%d)",
			f.lastQuery, f.lastLimit, "LLM unlearning", SearchLimit)
	}
	if strings.Contains(got, "Sixth") {
		t.Errorf("more than %d results shown:\n%s", SearchLimit, got)
	}
	if !strings.Contains(got, "<b>5. Fifth</b>") {
		t.Errorf("expected 5 numbered results:\n%s", got)
	}
	if strings.Contains(got, "<paper>") {
		t.Errorf("title was not HTML-escaped:\n%s", got)
	}

	// /save 2 must hit the second search hit, and hint at /download.
	save := b.Handle(ctx, "chat1", "/save", "2")
	if len(f.saved) != 1 || f.saved[0] != "s2:abc" {
		t.Fatalf("saved %v, want [s2:abc]", f.saved)
	}
	if !strings.Contains(save, "/download 2") {
		t.Errorf("save reply missing the download hint:\n%s", save)
	}

	// /download 1 uses the same numbering and reports the local file name.
	dl := b.Handle(ctx, "chat1", "/download", "1")
	if len(f.dl) != 1 || f.dl[0] != "arxiv:2506.13181" {
		t.Fatalf("downloaded %v, want [arxiv:2506.13181]", f.dl)
	}
	if !strings.Contains(dl, "2025 - Spohn - Align.pdf") {
		t.Errorf("download reply missing the file name:\n%s", dl)
	}

	// A different chat has its own (empty) numbering.
	if other := b.Handle(ctx, "chat2", "/save", "1"); !strings.Contains(other, "/search") {
		t.Errorf("chat2 should have no list, got:\n%s", other)
	}
}

func TestHandleDigestThenSave(t *testing.T) {
	f := newFakeExt()
	b := newTestBot(f)
	ctx := context.Background()

	if got := b.Handle(ctx, "c", "/paper", ""); !strings.Contains(got, "Paper digest") {
		t.Fatalf("/paper reply: %s", got)
	}
	if got := b.Handle(ctx, "c", "/save", "2"); !strings.Contains(got, "arxiv:d2") {
		t.Errorf("/save after digest should use the digest numbering, got:\n%s", got)
	}
	// A search then replaces the numbering.
	b.Handle(ctx, "c", "/search", "q")
	b.Handle(ctx, "c", "/save", "1")
	if f.saved[len(f.saved)-1] != "arxiv:2506.13181" {
		t.Errorf("last save was %q, want the first search hit", f.saved[len(f.saved)-1])
	}
}

func TestHandleLibraryAndStatus(t *testing.T) {
	f := newFakeExt()
	b := newTestBot(f)
	ctx := context.Background()

	got := b.Handle(ctx, "c", "/library", "")
	for _, want := range []string{"<b>1. Lib One</b>", "reading · page 4/12",
		"<b>2. Lib Two</b>", "to read", "/start-reading"} {
		if !strings.Contains(got, want) {
			t.Errorf("LibraryMessage missing %q:\n%s", want, got)
		}
	}

	if got := b.Handle(ctx, "c", "/done", "2"); !strings.Contains(got, "done") {
		t.Errorf("/done reply: %s", got)
	}
	if f.status["arxiv:l2"] != "done" {
		t.Errorf("status map = %v, want arxiv:l2 -> done", f.status)
	}
	if got := b.Handle(ctx, "c", "/read", "1"); !strings.Contains(got, "reading") {
		t.Errorf("/read reply: %s", got)
	}
	if f.status["arxiv:l1"] != "reading" {
		t.Errorf("status map = %v, want arxiv:l1 -> reading", f.status)
	}

	// A later /search must not disturb the library numbering.
	b.Handle(ctx, "c", "/search", "q")
	b.Handle(ctx, "c", "/start-reading", "2")
	if f.status["arxiv:l2"] != "reading" {
		t.Errorf("library numbering was clobbered by /search: %v", f.status)
	}

	// Filtered listing.
	if got := b.Handle(ctx, "c", "/library", "reading"); !strings.Contains(got, "Lib One") ||
		strings.Contains(got, "Lib Two") {
		t.Errorf("/library reading filtered wrongly:\n%s", got)
	}
	if got := b.Handle(ctx, "c", "/library", "banana"); !strings.Contains(got, "Usage") {
		t.Errorf("bad status should show usage:\n%s", got)
	}
}

func TestHandleErrorsAndUnknown(t *testing.T) {
	f := newFakeExt()
	f.searchErr = errors.New("arxiv is down & sad")
	b := newTestBot(f)
	ctx := context.Background()

	got := b.Handle(ctx, "c", "/search", "x")
	if !strings.Contains(got, "arxiv is down &amp; sad") {
		t.Errorf("error not surfaced/escaped:\n%s", got)
	}
	if got := b.Handle(ctx, "c", "/search", ""); !strings.Contains(got, "Usage") {
		t.Errorf("empty query:\n%s", got)
	}
	if got := b.Handle(ctx, "c", "/download", "abc"); !strings.Contains(got, "Usage") {
		t.Errorf("non-numeric n:\n%s", got)
	}
	if got := b.Handle(ctx, "c", "/done", "1"); !strings.Contains(got, "/library") {
		t.Errorf("/done with no library listing:\n%s", got)
	}

	// Unknown commands and /help both return the extended help.
	help := b.Handle(ctx, "c", "/help", "")
	if !strings.Contains(help, "/search") || !strings.Contains(help, "/download") ||
		!strings.Contains(help, "/library") {
		t.Errorf("help does not list the new commands:\n%s", help)
	}
	if got := b.Handle(ctx, "c", "/wat", ""); got != help {
		t.Errorf("unknown command should return help, got:\n%s", got)
	}
	if len(help) >= MaxMessage {
		t.Errorf("help is %d bytes, over the %d limit", len(help), MaxMessage)
	}
}

func TestHandleOutOfRange(t *testing.T) {
	b := newTestBot(newFakeExt())
	ctx := context.Background()
	b.Handle(ctx, "c", "/search", "q")
	got := b.Handle(ctx, "c", "/save", "9")
	if !strings.Contains(got, "no entry 9") || !strings.Contains(got, "5 entries") {
		t.Errorf("out-of-range reply:\n%s", got)
	}
}

func TestPaperListRoundTrip(t *testing.T) {
	l := NewPaperList(ListSearch, "q", []PaperItem{
		{ID: "a", Title: "A"}, {ID: "b", Title: "B"},
	})
	back, ok := decodePaperList(l.encode())
	if !ok {
		t.Fatal("decode failed")
	}
	id, title, ok := back.Item(2)
	if !ok || id != "b" || title != "B" {
		t.Errorf("Item(2) = (%q,%q,%v)", id, title, ok)
	}
	if _, _, ok := back.Item(0); ok {
		t.Error("Item(0) should not resolve")
	}
	if _, _, ok := back.Item(3); ok {
		t.Error("Item(3) should not resolve")
	}
	if _, ok := decodePaperList(""); ok {
		t.Error("empty kv value should not decode")
	}
	if _, ok := decodePaperList("{{"); ok {
		t.Error("garbage should not decode")
	}
}

func TestExtlessBotKeepsDigestCommands(t *testing.T) {
	b := &PaperBot{}
	b.Help = func() string { return "old help" }
	b.Save = func(_ context.Context, n int) (string, error) { return "saved digest entry", nil }
	ctx := context.Background()
	if got := b.Handle(ctx, "c", "/help", ""); got != "old help" {
		t.Errorf("without an ext the digest-era help should stand, got %q", got)
	}
	if got := b.Handle(ctx, "c", "/save", "1"); got != "saved digest entry" {
		t.Errorf("digest /save fallback: %q", got)
	}
	if got := b.Handle(ctx, "c", "/search", "x"); !strings.Contains(got, "not available") {
		t.Errorf("/search without an ext: %q", got)
	}
}
