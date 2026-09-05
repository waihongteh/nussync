package main

// Headless test harness for the Study backend:
//
//	nussync --study-test "<absolute path to a synced file>" [model] [nQuestions]
//
// It runs the real Claude Code CLI through the same code paths the GUI uses
// (job queue, prompts, lenient JSON parsing, SQLite persistence) and prints the
// results plus timings and the cost Claude Code reported.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nussync/internal/study"
)

// runStudyCLI is the entry point for --study-test. Hook it into cli.go's switch
// with:  case "--study-test": return runStudyCLI(os.Args[2:])
func runStudyCLI(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: nussync --study-test <file> [model] [n]")
		return 2
	}
	path, err := filepath.Abs(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	model := study.DefaultModel
	if len(args) > 1 && strings.TrimSpace(args[1]) != "" {
		model = args[1]
	}
	n := 5
	if len(args) > 2 {
		if v, err := strconv.Atoi(args[2]); err == nil && v > 0 {
			n = v
		}
	}

	app := NewApp()
	if err := app.Init(true); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer app.shutdown(context.Background())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	app.ctx = ctx

	if err := app.studyInit(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	st := app.GetStudyStatus()
	fmt.Printf("Claude Code: found=%v version=%s loggedIn=%v models=%v\n",
		st.CLIFound, st.Version, st.LoggedIn, st.Models)
	if st.Error != "" {
		fmt.Printf("  note: %s\n", st.Error)
	}
	if !st.CLIFound {
		return 1
	}

	fileID, err := studyFindFileID(app, path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	pages, _ := app.GetFilePageCount(fileID)
	fmt.Printf("File: %s\n  id=%d pages=%d readable-by-claude=%v\n\n",
		path, fileID, pages, study.ClaudeCanRead(path))

	fmt.Printf("Command line (the prompt is written to stdin, not argv): claude %s\n\n",
		studyRedactedArgs(study.Args(study.Options{
			Model: model, MaxTurns: 40, AddDirs: []string{filepath.Dir(path)},
		})))

	rc := 0

	// -------- overview
	fmt.Println("== Overview ==")
	start := time.Now()
	jobID, err := app.StartOverview(fileID, model)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	job := studyWait(ctx, app, jobID)
	fmt.Printf("status=%s in %s  cost=$%.4f\n", job.Status,
		time.Since(start).Round(time.Millisecond), job.CostUSD)
	if job.Error != "" {
		fmt.Println("error:", job.Error)
		rc = 1
	} else if ov, err := app.GetOverview(fileID); err == nil {
		fmt.Println(studyHead(ov.Markdown, 40))
	}

	// -------- quiz
	fmt.Printf("\n== Quiz (%d questions) ==\n", n)
	start = time.Now()
	jobID, err = app.StartQuiz([]int{fileID}, model, n)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	job = studyWait(ctx, app, jobID)
	fmt.Printf("status=%s in %s  cost=$%.4f\n", job.Status,
		time.Since(start).Round(time.Millisecond), job.CostUSD)
	if job.Error != "" {
		fmt.Println("error:", job.Error)
		return 1
	}
	quizzes, err := app.GetQuizzes(fileID)
	if err != nil || len(quizzes) == 0 {
		fmt.Fprintln(os.Stderr, "error: no quiz stored")
		return 1
	}
	q := quizzes[0]
	fmt.Printf("\n%q — %d questions\n", q.Title, len(q.Questions))
	answers := map[int]string{}
	for i, qq := range q.Questions {
		fmt.Printf("\n%d. [%s, p.%d] %s\n", i+1, qq.Type, qq.Page, qq.Prompt)
		for oi, o := range qq.Options {
			fmt.Printf("     %c) %s\n", 'A'+oi, strings.TrimSpace(o))
		}
		fmt.Printf("   answer: %s\n   why: %s\n", qq.Answer, qq.Explanation)
		answers[qq.ID] = qq.Answer // grade a "perfect" attempt as a sanity check
		if qq.Type == "short" {
			answers[qq.ID] = "correct"
		}
	}

	att, err := app.SubmitQuizAttempt(QuizAttempt{QuizID: q.ID, Answers: answers})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Printf("\nGraded a perfect attempt: %d/%d (expected %d/%d)\n",
		att.Score, att.Total, len(q.Questions), len(q.Questions))
	if att.Score != att.Total {
		rc = 1
	}
	return rc
}

// studyWait polls a job until it leaves the queued/running states.
func studyWait(ctx context.Context, app *App, jobID string) StudyJob {
	last := ""
	for {
		j, err := app.GetStudyJob(jobID)
		if err != nil {
			return StudyJob{ID: jobID, Status: "error", Error: err.Error()}
		}
		if j.Progress != "" && j.Progress != last {
			last = j.Progress
			fmt.Printf("   … %s\n", j.Progress)
		}
		if j.Status != "queued" && j.Status != "running" {
			return j
		}
		select {
		case <-ctx.Done():
			_ = app.CancelStudyJob(jobID)
			return StudyJob{ID: jobID, Status: "cancelled"}
		case <-time.After(400 * time.Millisecond):
		}
	}
}

// studyFindFileID resolves a local path to its Canvas file id.
func studyFindFileID(app *App, path string) (int, error) {
	courses, err := app.st.Courses()
	if err != nil {
		return 0, err
	}
	want := strings.ToLower(filepath.Clean(path))
	for _, c := range courses {
		files, err := app.st.FilesByCourse(c.ID)
		if err != nil {
			return 0, err
		}
		for _, f := range files {
			if strings.ToLower(filepath.Clean(f.AbsPath)) == want {
				return f.ID, nil
			}
		}
	}
	return 0, errors.New("that path is not a synced file in the library: " + path)
}

func studyRedactedArgs(args []string) string {
	var parts []string
	for _, a := range args {
		if strings.ContainsAny(a, " \t") {
			a = strconv.Quote(a)
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

func studyHead(s string, lines int) string {
	ls := strings.Split(s, "\n")
	if len(ls) <= lines {
		return s
	}
	return strings.Join(ls[:lines], "\n") + fmt.Sprintf("\n… (%d more lines)", len(ls)-lines)
}
