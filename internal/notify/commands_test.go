package notify

import (
	"strings"
	"testing"
	"time"

	"nussync/internal/store"
)

// now is a fixed clock: Wed 10 Sep 2026, 09:00 local.
func testNow() time.Time {
	return time.Date(2026, 9, 10, 9, 0, 0, 0, time.Local)
}

func at(offset time.Duration) string {
	return testNow().Add(offset).Format(time.RFC3339)
}

func TestParseCommand(t *testing.T) {
	cases := []struct {
		in        string
		cmd, args string
		ok        bool
	}{
		{"/due", "/due", "", true},
		{"  /DUE  ", "/due", "", true},
		{"/due@nuscanvassync_bot", "/due", "", true},
		{"/files week 3 notes", "/files", "week 3 notes", true},
		{"/files@nuscanvassync_bot mdp", "/files", "mdp", true},
		{"hello", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		cmd, args, ok := ParseCommand(c.in)
		if ok != c.ok || cmd != c.cmd || args != c.args {
			t.Errorf("ParseCommand(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.in, cmd, args, ok, c.cmd, c.args, c.ok)
		}
	}
}

func TestDueMessageGrouping(t *testing.T) {
	now := testNow()
	ds := []store.Deadline{
		{CourseCode: "CS4246", Title: "Assignment 1", DueAt: at(6 * time.Hour)},  // Today
		{CourseCode: "MA3236", Title: "Tutorial 3", DueAt: at(26 * time.Hour)},   // Tomorrow
		{CourseCode: "NST2030", Title: "Essay", DueAt: at(4 * 24 * time.Hour)},   // This week
		{CourseCode: "TR3201N", Title: "Report", DueAt: at(20 * 24 * time.Hour)}, // Later
		{CourseCode: "CS4246", Title: "Submitted thing", DueAt: at(time.Hour), Submitted: true},
	}
	got := DueMessage(ds, now, 10)

	for _, want := range []string{"Today", "Tomorrow", "This week", "Later",
		"CS4246", "Assignment 1", "MA3236", "NST2030", "TR3201N"} {
		if !strings.Contains(got, want) {
			t.Errorf("DueMessage missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Submitted thing") {
		t.Errorf("DueMessage included a submitted deadline:\n%s", got)
	}
	// Buckets must appear in urgency order.
	if i, j := strings.Index(got, "Today"), strings.Index(got, "Later"); i > j {
		t.Errorf("buckets out of order:\n%s", got)
	}
}

func TestDueMessageLimitAndOrder(t *testing.T) {
	var ds []store.Deadline
	for i := 1; i <= 15; i++ {
		ds = append(ds, store.Deadline{
			CourseCode: "CS1010",
			Title:      "Task " + string(rune('A'+i-1)),
			DueAt:      at(time.Duration(16-i) * 24 * time.Hour),
		})
	}
	got := DueMessage(ds, testNow(), 10)
	if n := strings.Count(got, "• "); n != 10 {
		t.Fatalf("want 10 entries, got %d:\n%s", n, got)
	}
	// Soonest first: Task O (1 day out) must precede Task F (10 days out).
	if strings.Index(got, "Task O") > strings.Index(got, "Task F") {
		t.Errorf("entries not sorted soonest-first:\n%s", got)
	}
}

func TestDueMessageEmpty(t *testing.T) {
	if got := DueMessage(nil, testNow(), 10); !strings.Contains(got, "Nothing unsubmitted") {
		t.Errorf("unexpected empty message: %q", got)
	}
}

func TestDueMessageEscapesHTML(t *testing.T) {
	ds := []store.Deadline{{CourseCode: "CS<1>", Title: "A & B", DueAt: at(time.Hour)}}
	got := DueMessage(ds, testNow(), 10)
	if strings.Contains(got, "CS<1>") || !strings.Contains(got, "A &amp; B") {
		t.Errorf("title not escaped for Telegram HTML:\n%s", got)
	}
}

func TestNewFilesMessageGroupsByCourse(t *testing.T) {
	files := []store.FeedFile{
		{File: store.File{Name: "Lecture 1.pdf"}, CourseCode: "CS4246", New: true},
		{File: store.File{Name: "Lecture 2.pdf"}, CourseCode: "CS4246"},
		{File: store.File{Name: "Sheet.xlsx"}, CourseCode: "MA3236", New: true},
	}
	got := NewFilesMessage(files, 30)
	if !strings.Contains(got, "<b>CS4246</b>") || !strings.Contains(got, "<b>MA3236</b>") {
		t.Errorf("missing course headings:\n%s", got)
	}
	if strings.Count(got, "<b>CS4246</b>") != 1 {
		t.Errorf("CS4246 heading repeated:\n%s", got)
	}
	if !strings.Contains(got, "(new)") || !strings.Contains(got, "(updated)") {
		t.Errorf("missing new/updated tags:\n%s", got)
	}
	if !strings.Contains(got, "3 files") {
		t.Errorf("missing count:\n%s", got)
	}
}

func TestNewFilesMessageCapsAndEmpty(t *testing.T) {
	var files []store.FeedFile
	for i := 0; i < 50; i++ {
		files = append(files, store.FeedFile{File: store.File{Name: "f.pdf"}, CourseCode: "CS4246"})
	}
	if n := strings.Count(NewFilesMessage(files, 30), "• "); n != 30 {
		t.Errorf("want 30 capped entries, got %d", n)
	}
	if got := NewFilesMessage(nil, 30); !strings.Contains(got, "No new or updated files") {
		t.Errorf("unexpected empty message: %q", got)
	}
}

func TestFilesMessage(t *testing.T) {
	hits := []store.Hit{
		{File: store.File{Name: "Lecture-mdp.pdf"}, CourseCode: "CS4246"},
		{File: store.File{Name: "notes & refs.pdf"}, CourseCode: "MA3236"},
	}
	got := FilesMessage("mdp", hits, 8)
	if !strings.Contains(got, "<b>CS4246</b> · Lecture-mdp.pdf") {
		t.Errorf("missing course · name line:\n%s", got)
	}
	if !strings.Contains(got, "notes &amp; refs.pdf") {
		t.Errorf("file name not escaped:\n%s", got)
	}
	if got := FilesMessage("nope", nil, 8); !strings.Contains(got, "No files match") {
		t.Errorf("unexpected empty message: %q", got)
	}
	if got := FilesMessage("  ", nil, 8); !strings.Contains(got, "Usage:") {
		t.Errorf("blank query should show usage: %q", got)
	}
}

func TestGradesMessage(t *testing.T) {
	gs := []store.Grade{
		{CourseCode: "CS4246", Title: "Assignment 1", Score: 25.666666, Possible: 26, Mean: 21.5},
		{CourseCode: "MA3236", Title: "Quiz", Score: 8, Possible: 10},
	}
	got := GradesMessage(gs, 10)
	if !strings.Contains(got, "25.67/26") {
		t.Errorf("score not trimmed:\n%s", got)
	}
	if !strings.Contains(got, "class mean 21.5") {
		t.Errorf("missing class mean:\n%s", got)
	}
	if strings.Contains(got, "class mean 0") {
		t.Errorf("mean 0 must be omitted:\n%s", got)
	}
	if got := GradesMessage(nil, 10); !strings.Contains(got, "No grades yet") {
		t.Errorf("unexpected empty message: %q", got)
	}
}

func TestSplitMessage(t *testing.T) {
	// Short bodies pass through untouched.
	if got := SplitMessage("hello", MaxMessage); len(got) != 1 || got[0] != "hello" {
		t.Fatalf("short body split: %#v", got)
	}
	// A long body splits into chunks that all fit and lose no content.
	line := strings.Repeat("x", 200)
	var b strings.Builder
	for i := 0; i < 100; i++ {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	parts := SplitMessage(strings.TrimRight(b.String(), "\n"), 1000)
	if len(parts) < 2 {
		t.Fatalf("want multiple chunks, got %d", len(parts))
	}
	for i, p := range parts {
		if len(p) > 1000 {
			t.Errorf("chunk %d is %d bytes, over the limit", i, len(p))
		}
	}
	if strings.Count(strings.Join(parts, "\n"), line) != 100 {
		t.Errorf("content lost while splitting")
	}
	// A single over-long line is hard-cut rather than dropped.
	one := strings.Repeat("y", 2500)
	parts = SplitMessage(one, 1000)
	if strings.Join(parts, "") != one {
		t.Errorf("over-long line not preserved across chunks")
	}
}

func TestHelpMessageListsCommands(t *testing.T) {
	got := HelpMessage()
	for _, c := range []string{"/due", "/new", "/files", "/sync", "/grades", "/help"} {
		if !strings.Contains(got, c) {
			t.Errorf("help missing %s:\n%s", c, got)
		}
	}
}
