package study

import (
	"encoding/json"
	"errors"
	"strings"
)

// QuizJSON is the shape asked of the model for a quiz.
type QuizJSON struct {
	Title     string         `json:"title"`
	Questions []QuestionJSON `json:"questions"`
}

// QuestionJSON is one generated question.
type QuestionJSON struct {
	Type        string   `json:"type"`
	Prompt      string   `json:"prompt"`
	Options     []string `json:"options"`
	Answer      string   `json:"answer"`
	Explanation string   `json:"explanation"`
	Page        int      `json:"page"`
}

// CardsJSON is the shape asked of the model for flashcards.
type CardsJSON struct {
	Cards []CardJSON `json:"cards"`
}

// CardJSON is one generated flashcard.
type CardJSON struct {
	Front string `json:"front"`
	Back  string `json:"back"`
}

// AskJSON is the shape asked of the model for a cited answer.
type AskJSON struct {
	Answer    string   `json:"answer"`
	Citations []string `json:"citations"`
}

// ExtractJSON pulls the first complete JSON object out of a model reply.
//
// Models wrap JSON in ```json fences, prepend "Here is the quiz:", or append a
// closing remark, all of which break a plain json.Unmarshal. This strips fences
// and then scans for a balanced {...} run, honouring string literals and
// escapes so braces inside strings do not confuse the brace counter.
func ExtractJSON(s string) (string, error) {
	s = stripFences(strings.TrimSpace(s))
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return "", errors.New("no JSON object in reply")
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1], nil
			}
		}
	}
	return "", errors.New("unterminated JSON object in reply")
}

// stripFences removes ```json … ``` (or bare ```) wrappers.
func stripFences(s string) string {
	for {
		i := strings.Index(s, "```")
		if i < 0 {
			return s
		}
		rest := s[i+3:]
		// Drop an optional language tag on the same line.
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			tag := strings.TrimSpace(rest[:nl])
			if tag == "" || isWord(tag) {
				rest = rest[nl+1:]
			}
		}
		end := strings.Index(rest, "```")
		if end < 0 {
			return strings.TrimSpace(s[:i] + rest)
		}
		return strings.TrimSpace(rest[:end])
	}
}

func isWord(s string) bool {
	if s == "" || len(s) > 16 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

// ParseQuiz decodes a quiz reply leniently and normalises the questions.
func ParseQuiz(reply string) (QuizJSON, error) {
	raw, err := ExtractJSON(reply)
	if err != nil {
		return QuizJSON{}, err
	}
	var q QuizJSON
	if err := json.Unmarshal([]byte(raw), &q); err != nil {
		return QuizJSON{}, err
	}
	out := q.Questions[:0]
	for _, qq := range q.Questions {
		qq.Type = strings.ToLower(strings.TrimSpace(qq.Type))
		if qq.Type != "mcq" && qq.Type != "short" {
			if len(qq.Options) > 0 {
				qq.Type = "mcq"
			} else {
				qq.Type = "short"
			}
		}
		if qq.Type == "mcq" {
			qq.Answer = NormalizeLetter(qq.Answer, qq.Options)
		}
		if strings.TrimSpace(qq.Prompt) == "" {
			continue
		}
		out = append(out, qq)
	}
	q.Questions = out
	if len(q.Questions) == 0 {
		return q, errors.New("quiz had no usable questions")
	}
	return q, nil
}

// NormalizeLetter turns an MCQ answer into a bare option letter A-D. Models
// answer variously with "B", "b)", "B. Utility", or the full option text.
func NormalizeLetter(ans string, options []string) string {
	t := strings.TrimSpace(ans)
	if t == "" {
		return ""
	}
	upper := strings.ToUpper(t)
	if len(upper) == 1 && upper[0] >= 'A' && upper[0] <= 'Z' {
		return upper
	}
	if len(upper) >= 2 && upper[0] >= 'A' && upper[0] <= 'Z' {
		switch upper[1] {
		case ')', '.', ':', ' ', '-':
			return upper[:1]
		}
	}
	// Match the option text.
	for i, o := range options {
		if strings.EqualFold(strings.TrimSpace(o), t) {
			return string(rune('A' + i))
		}
	}
	// Options themselves may be prefixed ("A) Utility"); compare tails.
	for i, o := range options {
		o = strings.TrimSpace(o)
		if len(o) > 2 && (o[1] == ')' || o[1] == '.') {
			o = strings.TrimSpace(o[2:])
		}
		if strings.EqualFold(o, t) {
			return string(rune('A' + i))
		}
	}
	return upper
}

// ParseCards decodes a flashcard reply leniently.
func ParseCards(reply string) ([]CardJSON, error) {
	raw, err := ExtractJSON(reply)
	if err != nil {
		return nil, err
	}
	var c CardsJSON
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return nil, err
	}
	out := c.Cards[:0]
	for _, cc := range c.Cards {
		if strings.TrimSpace(cc.Front) == "" || strings.TrimSpace(cc.Back) == "" {
			continue
		}
		out = append(out, cc)
	}
	if len(out) == 0 {
		return nil, errors.New("no usable flashcards")
	}
	return out, nil
}

// ParseAsk decodes an ask reply. The answer is markdown, so a failure to find
// JSON is not fatal: the whole reply becomes the answer with no citations.
func ParseAsk(reply string) AskJSON {
	raw, err := ExtractJSON(reply)
	if err == nil {
		var a AskJSON
		if json.Unmarshal([]byte(raw), &a) == nil && strings.TrimSpace(a.Answer) != "" {
			return a
		}
	}
	return AskJSON{Answer: strings.TrimSpace(reply)}
}
