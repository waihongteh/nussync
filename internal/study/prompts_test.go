package study

import (
	"strings"
	"testing"
)

func TestOverviewPromptHasAllSections(t *testing.T) {
	p := OverviewPrompt(Source{Name: "Lecture-mdp.pdf", Path: `C:\x\Lecture-mdp.pdf`,
		Pages: 30, Readable: true})
	for _, want := range []string{
		"## Summary", "## Key concepts", "## Definitions & formulas",
		"## Worked example", "## Likely exam questions", "## Gaps to check",
		`Read this file with the Read tool: C:\x\Lecture-mdp.pdf`,
		"30 pages",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("overview prompt missing %q", want)
		}
	}
	if strings.Contains(p, "read it in chunks") {
		t.Error("30 pages should not trigger chunked reading")
	}
}

func TestLongPDFAsksForChunkedReading(t *testing.T) {
	p := OverviewPrompt(Source{Name: "big.pdf", Path: `C:\x\big.pdf`,
		Pages: ChunkThreshold + 1, Readable: true})
	if !strings.Contains(p, "read it in chunks") {
		t.Errorf("a %d-page PDF must ask for chunked reading", ChunkThreshold+1)
	}
}

func TestUnreadableSourceInlinesExtractedText(t *testing.T) {
	s := Source{Name: "slides.pptx", Path: `C:\x\slides.pptx`, Readable: false,
		Text: "Markov decision processes"}
	p := QuizPrompt([]Source{s}, 10)
	if !strings.Contains(p, "Markov decision processes") {
		t.Error("fallback text must be inlined for formats Read cannot open")
	}
	if strings.Contains(p, "Read this file with the Read tool") {
		t.Error("must not tell Claude to Read an unsupported format")
	}
	if NeedsRead([]Source{s}) {
		t.Error("NeedsRead must be false when nothing is readable")
	}
	if !NeedsRead([]Source{{Path: `C:\x\a.pdf`, Readable: true}}) {
		t.Error("NeedsRead must be true for a readable path")
	}
}

func TestQuizPromptSplitsMCQAndShort(t *testing.T) {
	p := QuizPrompt([]Source{{Name: "a.pdf", Path: `C:\a.pdf`, Readable: true}}, 10)
	if !strings.Contains(p, "Write 10 questions: 7 of type \"mcq\"") {
		t.Errorf("70/30 split wrong:\n%s", p)
	}
	if !strings.Contains(p, "3 of type \"short\"") {
		t.Error("expected 3 short questions for n=10")
	}
	if !strings.Contains(p, "no markdown fences") {
		t.Error("structured prompts must forbid fences")
	}
	// n=5 -> round(3.5) = 4 mcq, 1 short
	p5 := QuizPrompt([]Source{{Name: "a.pdf"}}, 5)
	if !strings.Contains(p5, "Write 5 questions: 4 of type \"mcq\" (exactly 4 options) and 1 of type \"short\"") {
		t.Errorf("n=5 split wrong:\n%s", p5)
	}
}

func TestFlashcardAndAskPrompts(t *testing.T) {
	f := FlashcardsPrompt(Source{Name: "a.pdf", Path: `C:\a.pdf`, Readable: true}, 0)
	if !strings.Contains(f, "Write 20 flashcards") {
		t.Error("flashcard default should be 20")
	}
	if !strings.Contains(f, `{"cards": [{"front": string, "back": string}]}`) {
		t.Error("flashcard schema missing")
	}
	a := AskPrompt([]Source{{Name: "a.pdf", Path: `C:\a.pdf`, Readable: true}}, "  What is gamma? ")
	if !strings.Contains(a, "What is gamma?") {
		t.Error("question not embedded")
	}
	if !strings.Contains(a, "Cite the page") {
		t.Error("ask prompt must demand page citations")
	}
}

func TestClampText(t *testing.T) {
	if got := ClampText("short"); got != "short" {
		t.Fatalf("got %q", got)
	}
	long := strings.Repeat("a", MaxFallbackChars+100)
	got := ClampText(long)
	if !strings.HasSuffix(got, "[truncated]") || len(got) > MaxFallbackChars+20 {
		t.Fatalf("clamp failed, len=%d", len(got))
	}
}

func TestClaudeCanRead(t *testing.T) {
	for _, p := range []string{`C:\a.pdf`, `C:\a.PDF`, `C:\a.md`, `C:\a.png`} {
		if !ClaudeCanRead(p) {
			t.Errorf("%s should be readable", p)
		}
	}
	for _, p := range []string{`C:\a.pptx`, `C:\a.docx`, `C:\a.xlsx`, `C:\a.zip`} {
		if ClaudeCanRead(p) {
			t.Errorf("%s should not be readable by the Read tool", p)
		}
	}
}
