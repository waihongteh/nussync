package study

// In-app chat with the Claude Code CLI. Multi-turn works by remembering the
// `session_id` from the first run's result line and passing it back as
// `--resume <id>` on later turns (verified against CLI 2.1.251: `-r, --resume
// [value]` is accepted together with `-p`). If a resume ever fails, the caller
// falls back to replaying the transcript in the prompt — see ReplayPrompt.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ChatOptions builds the CLI options for one chat turn. resume is the Claude
// session id from a previous turn ("" for the first turn); dir/dirs make the
// context file reachable by the Read tool.
func ChatOptions(model, resume, dir string, dirs []string, needRead bool) Options {
	o := Options{
		Model:    NormalizeModel(model),
		Dir:      dir,
		MaxTurns: 20,
		Resume:   strings.TrimSpace(resume),
	}
	if needRead {
		o.AddDirs = dirs
	} else {
		o.NoTools = true
		o.MaxTurns = 2
	}
	return o
}

// ChatPrompt frames one chat turn. On the first turn (first == true) the
// context file is introduced; later turns are just the user's message, because
// --resume carries the conversation.
func ChatPrompt(contextPath, contextName, message string, first bool) string {
	msg := strings.TrimSpace(message)
	if !first {
		return msg
	}
	var sb strings.Builder
	if strings.TrimSpace(contextPath) != "" {
		fmt.Fprintf(&sb, "Context file: %s. Read it with the Read tool before answering. "+
			"Cite page numbers.\n", contextPath)
		if strings.TrimSpace(contextName) != "" {
			fmt.Fprintf(&sb, "Its title is %q.\n", contextName)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("You are a study assistant inside NUSSync. Answer in GitHub-flavoured " +
		"Markdown, concisely, and say plainly when the answer is not in the material.\n\n")
	sb.WriteString(msg)
	return sb.String()
}

// ReplayPrompt rebuilds the conversation in a single prompt. Used only when
// --resume fails (a session the CLI no longer has on disk).
func ReplayPrompt(contextPath, contextName string, history []ChatTurn, message string) string {
	var sb strings.Builder
	sb.WriteString(ChatPrompt(contextPath, contextName, "", true))
	sb.WriteString("Conversation so far:\n")
	for _, t := range history {
		role := "User"
		if t.Role == "assistant" {
			role = "Assistant"
		}
		fmt.Fprintf(&sb, "\n%s: %s\n", role, strings.TrimSpace(t.Text))
	}
	sb.WriteString("\nUser: " + strings.TrimSpace(message) + "\n\nAssistant:")
	return sb.String()
}

// ChatTurn is one stored message, used to rebuild a lost session.
type ChatTurn struct {
	Role string // user | assistant
	Text string
}

// ChatTitle derives a session title from its first user message.
func ChatTitle(message string) string {
	t := strings.TrimSpace(strings.ReplaceAll(message, "\n", " "))
	t = strings.Join(strings.Fields(t), " ")
	if t == "" {
		return "New chat"
	}
	if len(t) > 60 {
		t = trimUTF8(t, 60) + "…"
	}
	return t
}

func trimUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && s[len(s)-1]&0xC0 == 0x80 {
		s = s[:len(s)-1]
	}
	return strings.TrimSpace(s)
}

// RunChat streams one turn. onText receives each assistant text block as it
// arrives (the CLI's stream-json emits whole assistant messages, not token
// deltas, so a block is the smallest unit available); onProgress receives tool
// activity lines. The final Result carries the session id to resume next time.
func RunChat(ctx context.Context, prompt string, o Options,
	onText func(string), onProgress func(string)) (Result, error) {

	bin, err := Bin()
	if err != nil {
		return Result{}, err
	}

	cmd := exec.Command(bin, Args(o)...)
	cmd.Dir = o.Dir
	cmd.Env = childEnv()
	cmd.Stdin = strings.NewReader(prompt)
	cmd.SysProcAttr = noWindow()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start claude: %w", err)
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			killTree(cmd.Process.Pid)
		case <-done:
		}
	}()

	var res Result
	sawResult := false
	rd := bufio.NewReaderSize(stdout, 1<<20)
	for {
		line, rerr := readLongLine(rd)
		if line != "" {
			if r, ok := handleChatLine(line, onText, onProgress); ok {
				res = r
				sawResult = true
			}
		}
		if rerr != nil {
			break
		}
	}
	waitErr := cmd.Wait()

	if ctx.Err() != nil {
		return res, ctx.Err()
	}
	if !sawResult {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = "claude produced no result line"
		}
		if waitErr != nil {
			return res, fmt.Errorf("claude failed: %s (%v)", firstLine(msg), waitErr)
		}
		return res, errors.New("claude failed: " + firstLine(msg))
	}
	return res, nil
}

// handleChatLine is handleLine plus a text callback for assistant blocks.
func handleChatLine(line string, onText, onProgress func(string)) (Result, bool) {
	var head struct {
		Type string `json:"type"`
	}
	if json.Unmarshal([]byte(line), &head) != nil {
		return Result{}, false
	}
	switch head.Type {
	case "result":
		var rl resultLine
		if json.Unmarshal([]byte(line), &rl) != nil {
			return Result{}, false
		}
		return Result{
			Text:       rl.Result,
			SessionID:  rl.SessionID,
			IsError:    rl.IsError,
			CostUSD:    rl.TotalCost,
			DurationMS: rl.DurationMS,
			NumTurns:   rl.NumTurns,
			InputToks:  rl.Usage.InputTokens,
			OutputToks: rl.Usage.OutputTokens,
		}, true
	case "assistant":
		var sl streamLine
		if json.Unmarshal([]byte(line), &sl) != nil {
			return Result{}, false
		}
		for _, c := range sl.Message.Content {
			switch c.Type {
			case "text":
				if onText != nil && strings.TrimSpace(c.Text) != "" {
					onText(c.Text)
				}
			case "tool_use":
				if onProgress != nil && c.Name != "" {
					onProgress("Reading the document (" + c.Name + ")…")
				}
			}
		}
	case "system":
		if onProgress != nil {
			var sl streamLine
			if json.Unmarshal([]byte(line), &sl) == nil && sl.Subtype == "init" {
				onProgress("Claude Code session started…")
			}
		}
	}
	return Result{}, false
}

// PaperSummaryPrompt asks for the fixed section set of a paper key-points
// summary (docs/CONTRACT_PAPERS.md).
func PaperSummaryPrompt(s Source) string {
	var sb strings.Builder
	sb.WriteString("You are helping a final-year project student whose topic is LLM " +
		"unlearning and knowledge editing read a research paper.\n\n")
	sb.WriteString(sourceBlock([]Source{s}))
	sb.WriteString(`
Write the key points in GitHub-flavoured Markdown with exactly these level-2 sections, in this order:

## Contribution
What the paper claims to add, in two to four sentences.

## Method
How it works, concretely: the objective, the algorithm, the setting. Name the models and datasets.

## Key results
The numbers. Quote the actual figures and the baselines they beat, with the table or page they come from.

## Limitations
What the paper does not show, the assumptions it makes, and where the evaluation is thin.

## Relevance to LLM unlearning & knowledge editing
How this connects to that literature, and what an FYP could take from it.

## One-line takeaway
A single sentence.

## Related work worth reading
Three to six papers this one builds on or argues with, each one line: authors, year, title, why.

Rules: cite page numbers as (p. N) for every specific claim, especially the numbers. Do not
invent results. Output only the Markdown, with no preamble.
`)
	return sb.String()
}
