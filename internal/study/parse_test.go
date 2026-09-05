package study

import (
	"strings"
	"testing"
)

func TestExtractJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		bad  bool
	}{
		{name: "bare", in: `{"a":1}`, want: `{"a":1}`},
		{name: "fenced json", in: "```json\n{\"a\":1}\n```", want: `{"a":1}`},
		{name: "fenced bare", in: "```\n{\"a\":1}\n```", want: `{"a":1}`},
		{name: "preamble", in: "Here is the quiz:\n{\"a\":1}\n", want: `{"a":1}`},
		{name: "postamble", in: "{\"a\":1}\nLet me know if you want more.", want: `{"a":1}`},
		{name: "nested", in: `noise {"a":{"b":[1,2]},"c":3} tail`, want: `{"a":{"b":[1,2]},"c":3}`},
		{
			name: "brace inside string",
			in:   `{"prompt":"what does {s,a} mean?","page":3}`,
			want: `{"prompt":"what does {s,a} mean?","page":3}`,
		},
		{
			name: "escaped quote then brace",
			in:   `{"a":"he said \"hi\" }","b":1}`,
			want: `{"a":"he said \"hi\" }","b":1}`,
		},
		{name: "no object", in: "I cannot do that.", bad: true},
		{name: "unterminated", in: `{"a":1`, bad: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ExtractJSON(c.in)
			if c.bad {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestParseQuizLenient(t *testing.T) {
	reply := "Sure! Here you go:\n```json\n" + `{
	  "title": "MDPs",
	  "questions": [
	    {"type":"MCQ","prompt":"What is gamma?","options":["Discount factor","Reward","Policy","State"],
	     "answer":"A) Discount factor","explanation":"It discounts future reward.","page":12},
	    {"type":"","prompt":"Define a policy.","options":[],"answer":"A map from states to actions.",
	     "explanation":"Definition.","page":4},
	    {"type":"mcq","prompt":"Which is the Bellman operator?","options":["a","b","c","d"],
	     "answer":"c","explanation":"","page":7},
	    {"type":"short","prompt":"   ","answer":"dropped"}
	  ]
	}` + "\n```\nHope that helps!"

	q, err := ParseQuiz(reply)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if q.Title != "MDPs" {
		t.Fatalf("title = %q", q.Title)
	}
	if len(q.Questions) != 3 {
		t.Fatalf("questions = %d, want 3 (blank prompt dropped)", len(q.Questions))
	}
	if q.Questions[0].Type != "mcq" || q.Questions[0].Answer != "A" {
		t.Fatalf("q0 = %+v", q.Questions[0])
	}
	if q.Questions[1].Type != "short" {
		t.Fatalf("q1 type = %q, want short (inferred from empty options)", q.Questions[1].Type)
	}
	if q.Questions[2].Answer != "C" {
		t.Fatalf("q2 answer = %q, want C", q.Questions[2].Answer)
	}
}

func TestParseQuizRejectsProse(t *testing.T) {
	if _, err := ParseQuiz("I could not read the document."); err == nil {
		t.Fatal("expected an error for a non-JSON reply")
	}
	if _, err := ParseQuiz(`{"title":"x","questions":[]}`); err == nil {
		t.Fatal("expected an error for an empty question list")
	}
}

func TestNormalizeLetter(t *testing.T) {
	opts := []string{"Discount factor", "Reward", "Policy", "State"}
	cases := map[string]string{
		"B":               "B",
		"b":               "B",
		"b)":              "B",
		"C. Policy":       "C",
		"Policy":          "C",
		"  State  ":       "D",
		"discount factor": "A",
		"nonsense":        "NONSENSE",
	}
	for in, want := range cases {
		if got := NormalizeLetter(in, opts); got != want {
			t.Errorf("NormalizeLetter(%q) = %q, want %q", in, got, want)
		}
	}
	prefixed := []string{"A) Alpha", "B) Beta"}
	if got := NormalizeLetter("Beta", prefixed); got != "B" {
		t.Errorf("prefixed option match = %q, want B", got)
	}
}

func TestParseCards(t *testing.T) {
	cards, err := ParseCards("```json\n" + `{"cards":[{"front":"Gamma?","back":"Discount factor"},
		{"front":"","back":"dropped"},{"front":"Policy?","back":"State to action"}]}` + "\n```")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cards) != 2 {
		t.Fatalf("cards = %d, want 2", len(cards))
	}
	if cards[1].Front != "Policy?" {
		t.Fatalf("cards[1] = %+v", cards[1])
	}
	if _, err := ParseCards("no json here"); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseAskFallsBackToProse(t *testing.T) {
	a := ParseAsk(`{"answer":"The **discount factor**.","citations":["p. 12"]}`)
	if a.Answer != "The **discount factor**." || len(a.Citations) != 1 {
		t.Fatalf("structured parse = %+v", a)
	}
	prose := "The discount factor, see p. 12."
	if got := ParseAsk(prose); got.Answer != prose || len(got.Citations) != 0 {
		t.Fatalf("prose fallback = %+v", got)
	}
}

func TestArgsShape(t *testing.T) {
	args := Args(Options{Model: "opus", AddDirs: []string{`C:\a`}, MaxTurns: 7})
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-p", "--output-format stream-json", "--verbose", "--safe-mode",
		"--model opus", "--max-turns 7", "--permission-mode dontAsk",
		"--tools Read", "--allowedTools Read", `--add-dir C:\a`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q\ngot: %s", want, joined)
		}
	}
	if strings.Contains(strings.Join(Args(Options{NoTools: true}), " "), "--allowedTools") {
		t.Error("NoTools must not allow any tool")
	}
	if got := NormalizeModel("Sonnet"); got != "sonnet" {
		t.Errorf("NormalizeModel = %q", got)
	}
	if got := NormalizeModel("gpt"); got != DefaultModel {
		t.Errorf("unknown model = %q, want default", got)
	}
	if got := NormalizeModel("claude-sonnet-5"); got != "claude-sonnet-5" {
		t.Errorf("full model name = %q", got)
	}
}
