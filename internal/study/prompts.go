package study

import (
	"fmt"
	"strings"
)

// Source is one document a prompt should be built around.
type Source struct {
	Path      string // absolute local path; "" when only text is available
	Name      string
	Pages     int    // 0 when unknown / not a PDF
	Text      string // fallback extracted text when Claude cannot read the file
	Readable  bool   // Claude Code can open this format directly
	Note      string // caveat about Path, e.g. that it is a text-only extract
	CourseTag string
}

// ChunkThreshold is the page count above which Claude is told to read in parts.
const ChunkThreshold = 80

// sourceBlock describes the inputs and how to get at them.
func sourceBlock(srcs []Source) string {
	var sb strings.Builder
	sb.WriteString("Source material:\n")
	for i, s := range srcs {
		fmt.Fprintf(&sb, "%d. %s", i+1, s.Name)
		if s.CourseTag != "" {
			fmt.Fprintf(&sb, " (%s)", s.CourseTag)
		}
		if s.Pages > 0 {
			fmt.Fprintf(&sb, " — %d pages", s.Pages)
		}
		sb.WriteString("\n")
		if s.Readable && s.Path != "" {
			fmt.Fprintf(&sb, "   Read this file with the Read tool: %s\n", s.Path)
			if s.Note != "" {
				fmt.Fprintf(&sb, "   %s\n", s.Note)
			}
			if s.Pages > ChunkThreshold {
				fmt.Fprintf(&sb, "   It is long (%d pages): read it in chunks with the offset/limit "+
					"arguments rather than in one call, and cover the whole document.\n", s.Pages)
			}
		} else if strings.TrimSpace(s.Text) != "" {
			sb.WriteString("   This format cannot be opened directly. Its extracted text follows " +
				"between the markers; page numbers may be unavailable, so use 0 for the page.\n")
			fmt.Fprintf(&sb, "   <<<BEGIN %s>>>\n%s\n   <<<END %s>>>\n", s.Name, s.Text, s.Name)
		} else if s.Path != "" {
			fmt.Fprintf(&sb, "   Try to Read it: %s\n", s.Path)
		}
	}
	return sb.String()
}

// NeedsRead reports whether any source must be opened with the Read tool.
func NeedsRead(srcs []Source) bool {
	for _, s := range srcs {
		if s.Path != "" && s.Readable {
			return true
		}
	}
	return false
}

const jsonRule = "Output ONLY a JSON object matching this schema. No prose before or " +
	"after it, no markdown fences, no explanation of what you did.\n"

// OverviewPrompt asks for a revision-oriented markdown summary of one document.
func OverviewPrompt(s Source) string {
	var sb strings.Builder
	sb.WriteString("You are helping an NUS undergraduate revise a course document.\n\n")
	sb.WriteString(sourceBlock([]Source{s}))
	sb.WriteString(`
Write a study overview in GitHub-flavoured Markdown with exactly these level-2 sections, in this order:

## Summary
Three to six sentences on what the document covers and why it matters in the module.

## Key concepts
Bullets. One concept each, with the page it appears on in the form (p. 12).

## Definitions & formulas
Bullets or a table. Give every formula in LaTeX-free plain notation and say what each symbol means.

## Worked example
One concrete example taken from or modelled on the document, solved step by step.

## Likely exam questions
Four to six questions this material could be examined with, each one line.

## Gaps to check
Things the document assumes, glosses over, or leaves to the reader — what the student should look up.

Rules: cite page numbers as (p. N) wherever you make a specific claim. Do not invent
content that is not in the document. Output only the Markdown, with no preamble.
`)
	return sb.String()
}

// QuizPrompt asks for n questions across the given sources.
func QuizPrompt(srcs []Source, n int) string {
	if n <= 0 {
		n = 10
	}
	mcq := (n*7 + 5) / 10
	if mcq < 1 && n > 0 {
		mcq = 1
	}
	short := n - mcq

	var sb strings.Builder
	sb.WriteString("You are setting a self-test quiz for an NUS undergraduate.\n\n")
	sb.WriteString(sourceBlock(srcs))
	fmt.Fprintf(&sb, `
Write %d questions: %d of type "mcq" (exactly 4 options) and %d of type "short".
Spread them evenly across the whole document — do not take them all from the first pages.
Test understanding, not trivia. For each question give the page it comes from and a
one-to-three sentence explanation of why the answer is right.

For "mcq", "answer" is the letter of the correct option: "A", "B", "C" or "D",
matching the order of the options array. For "short", "answer" is a model answer
of one to three sentences.

%sSchema:
{"title": string,
 "questions": [
   {"type": "mcq"|"short", "prompt": string, "options": [string,string,string,string],
    "answer": string, "explanation": string, "page": number}
 ]}
For "short" questions set "options" to [].
`, n, mcq, short, jsonRule)
	return sb.String()
}

// AskPrompt asks a free-form question about the given sources.
func AskPrompt(srcs []Source, question string) string {
	var sb strings.Builder
	sb.WriteString("You are answering an NUS undergraduate's question about their course material.\n\n")
	sb.WriteString(sourceBlock(srcs))
	sb.WriteString("\nQuestion:\n" + strings.TrimSpace(question) + "\n")
	sb.WriteString(`
Answer from the source material. If the answer is not in it, say so plainly instead of
guessing. Cite the page for every specific claim.

` + jsonRule + `Schema:
{"answer": string, "citations": [string]}
"answer" is GitHub-flavoured Markdown and may contain newlines. "citations" is a list
of short page references such as "p. 14 — Bellman equation".
`)
	return sb.String()
}

// FlashcardsPrompt asks for n atomic question/answer cards.
func FlashcardsPrompt(s Source, n int) string {
	if n <= 0 {
		n = 20
	}
	var sb strings.Builder
	sb.WriteString("You are making spaced-repetition flashcards for an NUS undergraduate.\n\n")
	sb.WriteString(sourceBlock([]Source{s}))
	fmt.Fprintf(&sb, `
Write %d flashcards covering the whole document. Each card must be atomic: one fact,
definition, formula or distinction. "front" is a question or cue of at most 20 words.
"back" is the answer, at most 40 words. No card may need another card to make sense.
Do not duplicate the same fact in two cards.

%sSchema:
{"cards": [{"front": string, "back": string}]}
`, n, jsonRule)
	return sb.String()
}

// RetryPrompt is the follow-up sent when a reply was not valid JSON.
const RetryPrompt = "Your previous output was not valid JSON. Reply again with ONLY " +
	"the JSON object described earlier — no markdown fences, no commentary, nothing " +
	"before the opening brace or after the closing brace."
