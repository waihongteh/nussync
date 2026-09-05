package study

import (
	"strings"
	"testing"
)

// argIndex returns the position of arg in args, or -1.
func argIndex(args []string, arg string) int {
	for i, a := range args {
		if a == arg {
			return i
		}
	}
	return -1
}

func TestChatOptionsArgs(t *testing.T) {
	for _, tc := range []struct {
		name       string
		model      string
		resume     string
		dirs       []string
		needRead   bool
		wantModel  string
		wantResume string
		wantRead   bool
	}{
		{"first turn with a file", "opus", "", []string{`C:\lib\Papers`}, true,
			"opus", "", true},
		{"later turn resumes", "", "sess-123", []string{`C:\lib\Papers`}, true,
			"sonnet", "sess-123", true},
		{"no context, no tools", "haiku", "", nil, false, "haiku", "", false},
		{"resume is trimmed", "sonnet", "  sess-9  ", nil, false, "sonnet", "sess-9", false},
	} {
		o := ChatOptions(tc.model, tc.resume, `C:\lib\Papers`, tc.dirs, tc.needRead)
		if o.Model != tc.wantModel {
			t.Errorf("%s: model = %q, want %q", tc.name, o.Model, tc.wantModel)
		}
		if o.Resume != tc.wantResume {
			t.Errorf("%s: resume = %q, want %q", tc.name, o.Resume, tc.wantResume)
		}
		args := Args(o)

		// -p and stream-json are mandatory on every run.
		if argIndex(args, "-p") != 0 {
			t.Errorf("%s: -p must be the first argument: %v", tc.name, args)
		}
		if argIndex(args, "--safe-mode") < 0 || argIndex(args, "--verbose") < 0 {
			t.Errorf("%s: missing --safe-mode/--verbose: %v", tc.name, args)
		}
		if i := argIndex(args, "--model"); i < 0 || args[i+1] != tc.wantModel {
			t.Errorf("%s: --model not %q: %v", tc.name, tc.wantModel, args)
		}

		ri := argIndex(args, "--resume")
		if tc.wantResume == "" {
			if ri >= 0 {
				t.Errorf("%s: --resume must be absent on the first turn: %v", tc.name, args)
			}
		} else if ri < 0 || args[ri+1] != tc.wantResume {
			t.Errorf("%s: --resume %q missing: %v", tc.name, tc.wantResume, args)
		}

		hasRead := argIndex(args, "--allowedTools") >= 0
		if hasRead != tc.wantRead {
			t.Errorf("%s: Read tool = %v, want %v: %v", tc.name, hasRead, tc.wantRead, args)
		}
		if tc.wantRead {
			if i := argIndex(args, "--add-dir"); i < 0 || args[i+1] != tc.dirs[0] {
				t.Errorf("%s: --add-dir %q missing: %v", tc.name, tc.dirs[0], args)
			}
		}
	}
}

func TestChatPrompt(t *testing.T) {
	first := ChatPrompt(`C:\lib\Papers\2024 - Lovelace - X.pdf`, "Unlearning at Scale",
		"What is the forget set?", true)
	for _, want := range []string{
		`Context file: C:\lib\Papers\2024 - Lovelace - X.pdf.`,
		"Read it with the Read tool before answering.",
		"Cite page numbers.",
		"What is the forget set?",
	} {
		if !strings.Contains(first, want) {
			t.Errorf("first-turn prompt missing %q\n---\n%s", want, first)
		}
	}

	// Later turns carry only the message: --resume supplies the history.
	later := ChatPrompt(`C:\lib\Papers\x.pdf`, "X", "  And the retain set?  ", false)
	if later != "And the retain set?" {
		t.Errorf("later-turn prompt = %q, want the bare message", later)
	}

	// A chat with no context still works.
	none := ChatPrompt("", "", "Hello", true)
	if strings.Contains(none, "Context file") {
		t.Errorf("context-free prompt should not mention a file:\n%s", none)
	}
	if !strings.Contains(none, "Hello") {
		t.Errorf("context-free prompt lost the message:\n%s", none)
	}
}

func TestReplayPrompt(t *testing.T) {
	got := ReplayPrompt(`C:\p.pdf`, "P", []ChatTurn{
		{Role: "user", Text: "First question"},
		{Role: "assistant", Text: "First answer"},
	}, "Second question")
	for _, want := range []string{
		"Context file: C:\\p.pdf.",
		"User: First question",
		"Assistant: First answer",
		"User: Second question",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("replay prompt missing %q\n---\n%s", want, got)
		}
	}
}

func TestChatTitle(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"What is the forget set?", "What is the forget set?"},
		{"  multi\nline   message ", "multi line message"},
		{"", "New chat"},
		{strings.Repeat("a", 80), strings.Repeat("a", 60) + "…"},
	} {
		if got := ChatTitle(tc.in); got != tc.want {
			t.Errorf("ChatTitle(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPaperSummaryPromptSections(t *testing.T) {
	p := PaperSummaryPrompt(Source{Name: "X", Path: `C:\p.pdf`, Readable: true, Pages: 12})
	for _, want := range []string{
		"## Contribution", "## Method", "## Key results", "## Limitations",
		"## Relevance to LLM unlearning & knowledge editing",
		"## One-line takeaway", "## Related work worth reading",
		`C:\p.pdf`,
	} {
		if !strings.Contains(p, want) {
			t.Errorf("summary prompt missing %q", want)
		}
	}
}
