// Package study drives the locally installed Claude Code CLI (`claude`) as a
// child process to generate study material from synced course files.
//
// The user has no Anthropic API key: everything runs through their Claude Code
// subscription, so the CLI is invoked in non-interactive print mode and Claude
// reads the source document itself with the Read tool.
package study

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Models offered in the UI. Claude Code accepts these aliases for --model.
var Models = []string{"opus", "sonnet", "haiku"}

// DefaultModel is used when the caller passes "".
const DefaultModel = "sonnet"

// NormalizeModel maps an arbitrary string onto a supported alias.
func NormalizeModel(m string) string {
	m = strings.ToLower(strings.TrimSpace(m))
	for _, k := range Models {
		if m == k {
			return m
		}
	}
	if m != "" {
		// Full model names ("claude-sonnet-5") are passed through untouched.
		if strings.HasPrefix(m, "claude-") {
			return m
		}
	}
	return DefaultModel
}

// ---------------------------------------------------------------- discovery

var (
	binOnce sync.Once
	binPath string
	binErr  error
)

// Bin locates the claude executable, caching the result.
func Bin() (string, error) {
	binOnce.Do(func() {
		binPath, binErr = findClaude()
	})
	return binPath, binErr
}

func findClaude() (string, error) {
	if p := strings.TrimSpace(os.Getenv("NUSSYNC_CLAUDE_BIN")); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	// Prefer the real executable over the npm shim. `claude` on PATH is
	// claude.cmd, and Go runs a .cmd through cmd.exe, which mangles arguments
	// (the exe next to it takes them verbatim).
	var cands []string
	if appdata := os.Getenv("APPDATA"); appdata != "" {
		cands = append(cands,
			filepath.Join(appdata, "npm", "node_modules", "@anthropic-ai",
				"claude-code", "bin", "claude.exe"),
			filepath.Join(appdata, "npm", "claude.exe"),
		)
	}
	for _, c := range cands {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c, nil
		}
	}
	cands = cands[:0]
	if p, err := exec.LookPath("claude"); err == nil {
		return p, nil
	}
	if appdata := os.Getenv("APPDATA"); appdata != "" {
		cands = append(cands,
			filepath.Join(appdata, "npm", "claude.cmd"),
			filepath.Join(appdata, "npm", "claude"),
		)
	}
	if home, err := os.UserHomeDir(); err == nil {
		cands = append(cands,
			filepath.Join(home, ".claude", "local", "claude.exe"),
			filepath.Join(home, ".claude", "local", "claude"),
			filepath.Join(home, ".local", "bin", "claude.exe"),
			filepath.Join(home, ".local", "bin", "claude"),
		)
	}
	for _, c := range cands {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c, nil
		}
	}
	return "", errors.New("claude CLI not found on PATH")
}

// Version runs `claude --version` and returns the bare version number.
func Version(ctx context.Context) (string, error) {
	bin, err := Bin()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := output(ctx, bin, "--version")
	if err != nil {
		return "", err
	}
	// "2.1.251 (Claude Code)"
	f := strings.Fields(strings.TrimSpace(out))
	if len(f) == 0 {
		return "", errors.New("empty --version output")
	}
	return f[0], nil
}

// authStatus is the JSON printed by `claude auth status`.
type authStatus struct {
	LoggedIn   bool   `json:"loggedIn"`
	AuthMethod string `json:"authMethod"`
}

// LoggedIn reports whether the CLI has usable credentials. `claude auth status`
// is free and instant, so it is tried first; only when it says no do we spend a
// real (tiny) turn, because the status command can be wrong inside a nested
// Claude Code session.
func LoggedIn(ctx context.Context, model string) (bool, string) {
	bin, err := Bin()
	if err != nil {
		return false, err.Error()
	}
	sctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	out, serr := output(sctx, bin, "auth", "status")
	cancel()
	if serr == nil {
		var as authStatus
		if json.Unmarshal([]byte(strings.TrimSpace(out)), &as) == nil && as.LoggedIn {
			return true, ""
		}
	}

	pctx, pcancel := context.WithTimeout(ctx, 45*time.Second)
	defer pcancel()
	res, err := Run(pctx, "Reply with exactly: OK", Options{
		Model:    NormalizeModel(model),
		MaxTurns: 1,
		NoTools:  true,
	}, nil)
	if err != nil {
		return false, err.Error()
	}
	if res.IsError {
		return false, res.Text
	}
	return true, ""
}

// ------------------------------------------------------------------ running

// Options configures one `claude -p` invocation.
type Options struct {
	Model    string   // alias or full model name
	Dir      string   // working directory (defaults to the process cwd)
	AddDirs  []string // extra --add-dir entries the Read tool may reach
	MaxTurns int      // 0 = 30
	Resume   string   // session id to --resume (JSON retry)
	NoTools  bool     // disable every tool (probe / text-only prompts)
}

// Result is the parsed `type:"result"` line of a run.
type Result struct {
	Text       string
	SessionID  string
	IsError    bool
	CostUSD    float64
	DurationMS int
	NumTurns   int
	InputToks  int
	OutputToks int
}

type resultLine struct {
	Type       string  `json:"type"`
	Subtype    string  `json:"subtype"`
	Result     string  `json:"result"`
	SessionID  string  `json:"session_id"`
	IsError    bool    `json:"is_error"`
	TotalCost  float64 `json:"total_cost_usd"`
	DurationMS int     `json:"duration_ms"`
	NumTurns   int     `json:"num_turns"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type streamLine struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Message struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
			Name string `json:"name"`
		} `json:"content"`
	} `json:"message"`
}

// Args builds the command line (exported for tests and for the CLI banner).
//
// The prompt itself is NOT an argument: it is written to the child's stdin.
// Prompts are multi-line and full of quotes, and when `claude` resolves to the
// npm .cmd shim Go hands the argument to cmd.exe, which silently mangles it and
// the process dies with no output at all.
func Args(o Options) []string {
	maxTurns := o.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 30
	}
	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--verbose",
		// --safe-mode disables the user's hooks, CLAUDE.md, skills, plugins and
		// custom agents. Without it a SessionStart hook (e.g. an output-style
		// hook) rewrites the answer and JSON replies come back mangled.
		"--safe-mode",
		"--strict-mcp-config",
		"--model", NormalizeModel(o.Model),
		"--max-turns", fmt.Sprint(maxTurns),
		"--permission-mode", "dontAsk",
	}
	if o.NoTools {
		args = append(args, "--tools", "")
	} else {
		args = append(args, "--tools", "Read", "--allowedTools", "Read")
	}
	for _, d := range o.AddDirs {
		if strings.TrimSpace(d) != "" {
			args = append(args, "--add-dir", d)
		}
	}
	if o.Resume != "" {
		args = append(args, "--resume", o.Resume)
	}
	return args
}

// scrubbed environment keys. When NUSSync itself is launched from inside a
// Claude Code session these leak in and make the child defer OAuth refresh to a
// host that is not listening, which fails with "OAuth session expired".
var dropEnv = []string{
	"CLAUDECODE", "CLAUDE_CODE_SDK_HAS_HOST_AUTH_REFRESH",
	"CLAUDE_CODE_SDK_HAS_OAUTH_REFRESH", "CLAUDE_CODE_ENTRYPOINT",
	"CLAUDE_CODE_SESSION_ID", "CLAUDE_CODE_HOST_SESSION_ID",
	"CLAUDE_CODE_CHILD_SESSION", "CLAUDE_CODE_MESSAGING_SOCKET",
	"CLAUDE_CODE_MESSAGING_TOKEN", "CLAUDE_CODE_OAUTH_SCOPES",
	"CLAUDE_AGENT_SDK_VERSION", "CLAUDE_CODE_EXECPATH", "CLAUDE_PID",
	"AI_AGENT",
}

func childEnv() []string {
	drop := make(map[string]bool, len(dropEnv))
	for _, k := range dropEnv {
		drop[k] = true
	}
	src := os.Environ()
	out := make([]string, 0, len(src))
	for _, kv := range src {
		k, _, _ := strings.Cut(kv, "=")
		if drop[strings.ToUpper(k)] {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// Run executes one prompt and returns the final assistant text. onProgress, if
// set, receives short human-readable progress lines as they stream in.
func Run(ctx context.Context, prompt string, o Options, onProgress func(string)) (Result, error) {
	bin, err := Bin()
	if err != nil {
		return Result{}, err
	}

	cmd := exec.Command(bin, Args(o)...)
	cmd.Dir = o.Dir
	cmd.Env = childEnv()
	// The prompt arrives on stdin, which is then closed: the child sees EOF and
	// never blocks waiting for more input.
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

	// Cancellation kills the whole tree: the launcher is a .cmd shim on Windows,
	// so killing only the direct child orphans the real node process.
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
		line, err := readLongLine(rd)
		if line != "" {
			if r, ok := handleLine(line, onProgress); ok {
				res = r
				sawResult = true
			}
		}
		if err != nil {
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

func handleLine(line string, onProgress func(string)) (Result, bool) {
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
		if onProgress == nil {
			return Result{}, false
		}
		var sl streamLine
		if json.Unmarshal([]byte(line), &sl) != nil {
			return Result{}, false
		}
		for _, c := range sl.Message.Content {
			switch c.Type {
			case "tool_use":
				if c.Name != "" {
					onProgress("Reading document (" + c.Name + ")…")
				}
			case "text":
				if t := firstLine(strings.TrimSpace(c.Text)); t != "" {
					onProgress(truncate(t, 110))
				}
			}
		}
	case "system":
		if onProgress != nil && head.Type == "system" {
			var sl streamLine
			if json.Unmarshal([]byte(line), &sl) == nil && sl.Subtype == "init" {
				onProgress("Claude Code session started…")
			}
		}
	}
	return Result{}, false
}

// readLongLine reads one \n-terminated line of any length.
func readLongLine(rd *bufio.Reader) (string, error) {
	var sb strings.Builder
	for {
		chunk, err := rd.ReadString('\n')
		sb.WriteString(chunk)
		if err == nil {
			return strings.TrimRight(sb.String(), "\r\n"), nil
		}
		if err == io.EOF {
			return strings.TrimRight(sb.String(), "\r\n"), io.EOF
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		return strings.TrimRight(sb.String(), "\r\n"), err
	}
}

func output(ctx context.Context, bin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = childEnv()
	cmd.Stdin = nil
	cmd.SysProcAttr = noWindow()
	b, err := cmd.Output()
	return string(b), err
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
