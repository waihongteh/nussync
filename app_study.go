package main

// Study/AI backend. All generation shells out to the locally installed Claude
// Code CLI (the user has no API key), one job at a time. See
// docs/CONTRACT_STUDY.md.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nussync/internal/store"
	"nussync/internal/study"
)

// ------------------------------------------------------------------ runtime

type studyTask struct {
	id string
	fn func(ctx context.Context, a *App, id string) (float64, error)
}

type studyRuntime struct {
	initMu sync.Mutex
	ready  bool

	mu      sync.Mutex
	jobs    map[string]StudyJob
	cancels map[string]context.CancelFunc
	queue   chan studyTask

	statusMu sync.Mutex
	status   StudyStatus
	statusAt time.Time
}

var (
	studyRT     studyRuntime
	studyJobSeq atomic.Int64
)

// studyInit prepares the Study feature: it creates the study_* tables, clears
// jobs orphaned by a crash and starts the single-slot worker.
//
// The orchestrator should call this from App.startup, but every public method
// calls it lazily too, so the feature works even when startup does not. A
// failure is not memoised: the store may simply not be open yet.
func (a *App) studyInit() error {
	studyRT.initMu.Lock()
	defer studyRT.initMu.Unlock()
	if studyRT.ready {
		return nil
	}
	if a.st == nil {
		return errors.New("not initialised")
	}
	if err := a.st.MigrateStudy(); err != nil {
		return err
	}
	_ = a.st.ResetRunningStudyJobs()

	studyRT.mu.Lock()
	studyRT.jobs = map[string]StudyJob{}
	studyRT.cancels = map[string]context.CancelFunc{}
	studyRT.queue = make(chan studyTask, 64)
	q := studyRT.queue
	studyRT.mu.Unlock()

	go a.studyWorker(q)
	studyRT.ready = true
	return nil
}

// studyWorker runs queued jobs strictly one at a time: the subscription has
// rate limits and two concurrent `claude` processes just make both slower.
func (a *App) studyWorker(q chan studyTask) {
	for t := range q {
		a.runStudyTask(t)
	}
}

func (a *App) runStudyTask(t studyTask) {
	job, ok := a.studyJob(t.id)
	if !ok || job.Status == "cancelled" {
		return
	}

	ctx, cancel := context.WithCancel(a.baseCtx())
	studyRT.mu.Lock()
	studyRT.cancels[t.id] = cancel
	studyRT.mu.Unlock()
	defer func() {
		cancel()
		studyRT.mu.Lock()
		delete(studyRT.cancels, t.id)
		studyRT.mu.Unlock()
	}()

	a.updateStudyJob(t.id, func(j *StudyJob) {
		j.Status = "running"
		j.StartedAt = nowRFC3339()
		j.Progress = "Starting Claude Code…"
	})

	cost, err := t.fn(ctx, a, t.id)

	a.updateStudyJob(t.id, func(j *StudyJob) {
		j.CostUSD = cost
		j.FinishedAt = nowRFC3339()
		switch {
		case ctx.Err() != nil:
			j.Status = "cancelled"
			j.Progress = "Cancelled"
			j.Error = ""
		case err != nil:
			j.Status = "error"
			j.Error = err.Error()
			j.Progress = ""
		default:
			j.Status = "done"
			j.Progress = "Done"
			j.Error = ""
		}
	})

	final, _ := a.studyJob(t.id)
	switch final.Status {
	case "done":
		a.toast("success", "Study: "+final.Kind+" ready")
	case "error":
		a.toast("error", "Study: "+final.Kind+" failed — "+final.Error)
	}
}

// ------------------------------------------------------------- job bookkeeping

func nowRFC3339() string { return time.Now().Format(time.RFC3339) }

func (a *App) newStudyJob(kind string, fileIDs []int, model string) StudyJob {
	id := fmt.Sprintf("sj_%d_%d", time.Now().UnixNano(), studyJobSeq.Add(1))
	j := StudyJob{
		ID:      id,
		Kind:    kind,
		FileIDs: fileIDs,
		Status:  "queued",
		Model:   study.NormalizeModel(model),
	}
	studyRT.mu.Lock()
	studyRT.jobs[id] = j
	studyRT.mu.Unlock()
	a.persistStudyJob(j)
	a.emit("study:job", j)
	return j
}

func (a *App) studyJob(id string) (StudyJob, bool) {
	studyRT.mu.Lock()
	j, ok := studyRT.jobs[id]
	studyRT.mu.Unlock()
	return j, ok
}

func (a *App) updateStudyJob(id string, mut func(*StudyJob)) {
	studyRT.mu.Lock()
	j, ok := studyRT.jobs[id]
	if ok {
		mut(&j)
		studyRT.jobs[id] = j
	}
	studyRT.mu.Unlock()
	if !ok {
		return
	}
	a.persistStudyJob(j)
	a.emit("study:job", j)
}

// setStudyProgress updates the live progress line. Progress churns fast, so it
// is not written to SQLite — only the in-memory job and the two events.
func (a *App) setStudyProgress(id, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	studyRT.mu.Lock()
	j, ok := studyRT.jobs[id]
	if ok {
		j.Progress = text
		studyRT.jobs[id] = j
	}
	studyRT.mu.Unlock()
	if !ok {
		return
	}
	a.emit("study:progress", StudyProgress{JobID: id, Text: text})
	a.emit("study:job", j)
}

func (a *App) persistStudyJob(j StudyJob) {
	if a.st == nil {
		return
	}
	_ = a.st.PutStudyJob(store.StudyJob{
		ID: j.ID, Kind: j.Kind, FileIDs: j.FileIDs, Status: j.Status,
		Progress: j.Progress, Error: j.Error, StartedAt: j.StartedAt,
		FinishedAt: j.FinishedAt, Model: j.Model, CostUSD: j.CostUSD,
	})
}

func (a *App) enqueueStudy(j StudyJob, fn func(context.Context, *App, string) (float64, error)) error {
	studyRT.mu.Lock()
	q := studyRT.queue
	studyRT.mu.Unlock()
	if q == nil {
		return errors.New("study subsystem not initialised")
	}
	select {
	case q <- studyTask{id: j.ID, fn: fn}:
		return nil
	default:
		a.updateStudyJob(j.ID, func(x *StudyJob) {
			x.Status = "error"
			x.Error = "study queue full"
		})
		return errors.New("study queue full")
	}
}

// ------------------------------------------------------------------- status

// GetStudyStatus reports whether the Claude Code CLI is installed and logged
// in. The login probe costs a real (tiny) turn, so the result is cached for
// ten minutes.
func (a *App) GetStudyStatus() StudyStatus {
	studyRT.statusMu.Lock()
	if time.Since(studyRT.statusAt) < 10*time.Minute && studyRT.status.Version != "" {
		s := studyRT.status
		studyRT.statusMu.Unlock()
		return s
	}
	studyRT.statusMu.Unlock()

	st := StudyStatus{Models: append([]string(nil), study.Models...)}
	if _, err := study.Bin(); err != nil {
		st.Error = "Claude Code CLI not found. Install it with `npm i -g @anthropic-ai/claude-code`."
		return st
	}
	st.CLIFound = true

	ctx, cancel := context.WithTimeout(a.baseCtx(), 90*time.Second)
	defer cancel()

	v, err := study.Version(ctx)
	if err != nil {
		st.Error = "claude --version failed: " + err.Error()
		return st
	}
	st.Version = v

	ok, msg := study.LoggedIn(ctx, study.DefaultModel)
	st.LoggedIn = ok
	if !ok {
		st.Error = "Claude Code is not signed in. Run `claude auth login` (or " +
			"`claude setup-token`) in a terminal, then try again. " + msg
	}

	studyRT.statusMu.Lock()
	studyRT.status = st
	studyRT.statusAt = time.Now()
	studyRT.statusMu.Unlock()
	return st
}

// --------------------------------------------------------------- job queries

// GetStudyJob returns one job.
func (a *App) GetStudyJob(id string) (StudyJob, error) {
	if err := a.studyInit(); err != nil {
		return StudyJob{}, err
	}
	if j, ok := a.studyJob(id); ok {
		return j, nil
	}
	row, ok, err := a.st.StudyJobByID(id)
	if err != nil {
		return StudyJob{}, err
	}
	if !ok {
		return StudyJob{}, errors.New("no such job: " + id)
	}
	return toStudyJob(row), nil
}

// GetStudyJobs lists the 20 most recent jobs, newest first.
func (a *App) GetStudyJobs() ([]StudyJob, error) {
	if err := a.studyInit(); err != nil {
		return nil, err
	}
	rows, err := a.st.StudyJobs(20)
	if err != nil {
		return nil, err
	}
	out := make([]StudyJob, 0, len(rows))
	for _, r := range rows {
		j := toStudyJob(r)
		// The live copy has fresher progress than the persisted row.
		if live, ok := a.studyJob(j.ID); ok {
			j = live
		}
		out = append(out, j)
	}
	return out, nil
}

// CancelStudyJob stops a queued or running job, killing the claude process
// tree when one is live.
func (a *App) CancelStudyJob(id string) error {
	if err := a.studyInit(); err != nil {
		return err
	}
	studyRT.mu.Lock()
	cancel := studyRT.cancels[id]
	j, ok := studyRT.jobs[id]
	studyRT.mu.Unlock()
	if !ok {
		return errors.New("no such job: " + id)
	}
	if cancel != nil {
		cancel()
		return nil
	}
	if j.Status == "queued" {
		a.updateStudyJob(id, func(x *StudyJob) {
			x.Status = "cancelled"
			x.FinishedAt = nowRFC3339()
		})
		return nil
	}
	return errors.New("job is not running")
}

func toStudyJob(r store.StudyJob) StudyJob {
	return StudyJob{
		ID: r.ID, Kind: r.Kind, FileIDs: r.FileIDs, Status: r.Status,
		Progress: r.Progress, Error: r.Error, StartedAt: r.StartedAt,
		FinishedAt: r.FinishedAt, Model: r.Model, CostUSD: r.CostUSD,
	}
}

// ------------------------------------------------------------------ sources

// studySources resolves file ids into prompt sources, plus the directories the
// Read tool must be allowed to reach.
func (a *App) studySources(fileIDs []int) ([]study.Source, string, []string, error) {
	if len(fileIDs) == 0 {
		return nil, "", nil, errors.New("no files selected")
	}
	var (
		srcs []study.Source
		dirs []string
		seen = map[string]bool{}
	)
	for _, id := range fileIDs {
		f, ok, err := a.st.FileByID(id)
		if err != nil {
			return nil, "", nil, err
		}
		if !ok {
			return nil, "", nil, fmt.Errorf("file %d not in the library", id)
		}
		s := study.Source{Name: f.Name, Path: f.AbsPath}
		if st, err := os.Stat(f.AbsPath); err != nil || st.IsDir() {
			s.Path = ""
		}
		if s.Path != "" {
			// Many lecture PDFs cannot be opened by the Read tool as they sit
			// on disk; Resolve hands back a normalised copy (or a text extract)
			// from the study cache instead. Page count still comes from the
			// original, which is the document the student sees.
			s.Pages = study.PageCount(s.Path)
			p := study.Resolve(s.Path, id, func() string {
				txt, _ := a.st.StudyFileText(id)
				return txt
			})
			s.Path, s.Readable, s.Note = p.Path, p.Readable, p.Note
			d := filepath.Dir(s.Path)
			if !seen[d] {
				seen[d] = true
				dirs = append(dirs, d)
			}
		}
		if !s.Readable {
			txt, _ := a.st.StudyFileText(id)
			s.Text = study.ClampText(txt)
			if strings.TrimSpace(s.Text) == "" && s.Path == "" {
				return nil, "", nil, fmt.Errorf("%s is not synced and has no extracted text", f.Name)
			}
		}
		srcs = append(srcs, s)
	}
	dir := ""
	if len(dirs) > 0 {
		dir = dirs[0]
	} else {
		dir = a.settings().SyncDir
	}
	return srcs, dir, dirs, nil
}

func (a *App) studyOpts(model, dir string, dirs []string, needRead bool) study.Options {
	o := study.Options{Model: study.NormalizeModel(model), Dir: dir, MaxTurns: 40}
	if !needRead {
		o.NoTools = true
		o.MaxTurns = 2
	} else {
		o.AddDirs = dirs
	}
	return o
}

// runStudyJSON runs a prompt whose reply must parse, retrying once in the same
// session when the model wraps or narrates its JSON. Returns total cost.
func (a *App) runStudyJSON(ctx context.Context, jobID, prompt string, o study.Options,
	parse func(string) error) (float64, error) {

	prog := func(t string) { a.setStudyProgress(jobID, t) }

	res, err := study.Run(ctx, prompt, o, prog)
	cost := res.CostUSD
	if err != nil {
		return cost, err
	}
	if res.IsError {
		return cost, errors.New(strings.TrimSpace(res.Text))
	}
	if perr := parse(res.Text); perr == nil {
		return cost, nil
	} else {
		a.setStudyProgress(jobID, "Reply was not valid JSON — asking again…")
		_ = perr
	}

	retry := o
	retry.Resume = res.SessionID
	res2, err2 := study.Run(ctx, study.RetryPrompt, retry, prog)
	cost += res2.CostUSD
	if err2 != nil || res2.IsError {
		// --resume can fail (session not persisted); fall back to a fresh run.
		fresh := o
		res3, err3 := study.Run(ctx, prompt+"\n\n"+study.RetryPrompt, fresh, prog)
		cost += res3.CostUSD
		if err3 != nil {
			return cost, err3
		}
		if res3.IsError {
			return cost, errors.New(strings.TrimSpace(res3.Text))
		}
		if perr := parse(res3.Text); perr != nil {
			return cost, fmt.Errorf("model did not return valid JSON: %w", perr)
		}
		return cost, nil
	}
	if perr := parse(res2.Text); perr != nil {
		return cost, fmt.Errorf("model did not return valid JSON: %w", perr)
	}
	return cost, nil
}

// ----------------------------------------------------------------- overview

// StartOverview queues generation of a study overview for one file.
func (a *App) StartOverview(fileID int, model string) (string, error) {
	if err := a.studyInit(); err != nil {
		return "", err
	}
	srcs, dir, dirs, err := a.studySources([]int{fileID})
	if err != nil {
		return "", err
	}
	j := a.newStudyJob("overview", []int{fileID}, model)
	err = a.enqueueStudy(j, func(ctx context.Context, a *App, id string) (float64, error) {
		o := a.studyOpts(j.Model, dir, dirs, study.NeedsRead(srcs))
		a.setStudyProgress(id, "Reading "+srcs[0].Name+"…")
		res, err := study.Run(ctx, study.OverviewPrompt(srcs[0]), o,
			func(t string) { a.setStudyProgress(id, t) })
		if err != nil {
			return res.CostUSD, err
		}
		if res.IsError {
			return res.CostUSD, errors.New(strings.TrimSpace(res.Text))
		}
		md := strings.TrimSpace(res.Text)
		if md == "" {
			return res.CostUSD, errors.New("Claude returned an empty overview")
		}
		return res.CostUSD, a.st.PutStudyOverview(store.StudyOverview{
			FileID: fileID, Model: j.Model, Markdown: md, CreatedAt: nowRFC3339(),
		})
	})
	if err != nil {
		return j.ID, err
	}
	return j.ID, nil
}

// GetOverview returns the cached overview for a file, or a zero value.
func (a *App) GetOverview(fileID int) (Overview, error) {
	if err := a.studyInit(); err != nil {
		return Overview{}, err
	}
	o, ok, err := a.st.StudyOverviewFor(fileID)
	if err != nil || !ok {
		return Overview{}, err
	}
	return Overview{FileID: o.FileID, Markdown: o.Markdown,
		CreatedAt: o.CreatedAt, Model: o.Model}, nil
}

// --------------------------------------------------------------------- quiz

// StartQuiz queues generation of an n-question quiz over the given files.
func (a *App) StartQuiz(fileIDs []int, model string, n int) (string, error) {
	if err := a.studyInit(); err != nil {
		return "", err
	}
	if n <= 0 {
		n = 10
	}
	if n > 50 {
		n = 50
	}
	srcs, dir, dirs, err := a.studySources(fileIDs)
	if err != nil {
		return "", err
	}
	j := a.newStudyJob("quiz", fileIDs, model)
	err = a.enqueueStudy(j, func(ctx context.Context, a *App, id string) (float64, error) {
		o := a.studyOpts(j.Model, dir, dirs, study.NeedsRead(srcs))
		var parsed study.QuizJSON
		cost, err := a.runStudyJSON(ctx, id, study.QuizPrompt(srcs, n), o, func(reply string) error {
			q, e := study.ParseQuiz(reply)
			if e != nil {
				return e
			}
			parsed = q
			return nil
		})
		if err != nil {
			return cost, err
		}
		title := strings.TrimSpace(parsed.Title)
		if title == "" {
			title = srcs[0].Name
		}
		qs := make([]store.StudyQuestion, 0, len(parsed.Questions))
		for _, q := range parsed.Questions {
			qs = append(qs, store.StudyQuestion{
				Type: q.Type, Prompt: q.Prompt, Options: q.Options,
				Answer: q.Answer, Explanation: q.Explanation, Page: q.Page,
			})
		}
		_, err = a.st.InsertStudyQuiz(store.StudyQuiz{
			FileIDs: fileIDs, Title: title, CreatedAt: nowRFC3339(),
			Model: j.Model, Questions: qs,
		})
		return cost, err
	})
	if err != nil {
		return j.ID, err
	}
	return j.ID, nil
}

// GetQuizzes lists quizzes covering fileID (0 = every quiz), newest first.
func (a *App) GetQuizzes(fileID int) ([]Quiz, error) {
	if err := a.studyInit(); err != nil {
		return nil, err
	}
	rows, err := a.st.StudyQuizzes(fileID)
	if err != nil {
		return nil, err
	}
	out := make([]Quiz, 0, len(rows))
	for _, r := range rows {
		out = append(out, toQuiz(r))
	}
	return out, nil
}

// GetQuiz loads one quiz.
func (a *App) GetQuiz(id int) (Quiz, error) {
	if err := a.studyInit(); err != nil {
		return Quiz{}, err
	}
	q, ok, err := a.st.StudyQuizByID(id)
	if err != nil {
		return Quiz{}, err
	}
	if !ok {
		return Quiz{}, fmt.Errorf("no quiz %d", id)
	}
	return toQuiz(q), nil
}

func toQuiz(r store.StudyQuiz) Quiz {
	q := Quiz{ID: r.ID, FileIDs: r.FileIDs, Title: r.Title,
		CreatedAt: r.CreatedAt, Model: r.Model}
	q.Questions = make([]Question, 0, len(r.Questions))
	for _, x := range r.Questions {
		q.Questions = append(q.Questions, Question{
			ID: x.ID, Type: x.Type, Prompt: x.Prompt, Options: x.Options,
			Answer: x.Answer, Explanation: x.Explanation, Page: x.Page,
		})
	}
	return q
}

// SubmitQuizAttempt grades and records an attempt. MCQ answers are graded
// exactly against the stored option letter; short answers are self-marked by
// the user, who passes "correct" or "wrong".
func (a *App) SubmitQuizAttempt(att QuizAttempt) (QuizAttempt, error) {
	if err := a.studyInit(); err != nil {
		return att, err
	}
	q, ok, err := a.st.StudyQuizByID(att.QuizID)
	if err != nil {
		return att, err
	}
	if !ok {
		return att, fmt.Errorf("no quiz %d", att.QuizID)
	}

	score := 0
	for _, question := range q.Questions {
		given := strings.TrimSpace(att.Answers[question.ID])
		if given == "" {
			continue
		}
		if question.Type == "mcq" {
			if strings.EqualFold(study.NormalizeLetter(given, question.Options), question.Answer) {
				score++
			}
			continue
		}
		if strings.EqualFold(given, "correct") {
			score++
		}
	}
	att.Score = score
	att.Total = len(q.Questions)
	if att.TakenAt == "" {
		att.TakenAt = nowRFC3339()
	}

	blob, err := json.Marshal(att.Answers)
	if err != nil {
		return att, err
	}
	if err := a.st.InsertStudyAttempt(store.StudyAttempt{
		QuizID: att.QuizID, Answers: string(blob), Score: att.Score,
		Total: att.Total, TakenAt: att.TakenAt,
	}); err != nil {
		return att, err
	}
	return att, nil
}

// GetQuizAttempts lists a quiz's attempts, newest first.
func (a *App) GetQuizAttempts(quizID int) ([]QuizAttempt, error) {
	if err := a.studyInit(); err != nil {
		return nil, err
	}
	rows, err := a.st.StudyAttempts(quizID)
	if err != nil {
		return nil, err
	}
	out := make([]QuizAttempt, 0, len(rows))
	for _, r := range rows {
		att := QuizAttempt{QuizID: r.QuizID, Score: r.Score, Total: r.Total,
			TakenAt: r.TakenAt, Answers: map[int]string{}}
		_ = json.Unmarshal([]byte(r.Answers), &att.Answers)
		out = append(out, att)
	}
	return out, nil
}

// ---------------------------------------------------------------------- ask

// StartAsk queues a cited Q&A over the given files.
func (a *App) StartAsk(fileIDs []int, question string, model string) (string, error) {
	if err := a.studyInit(); err != nil {
		return "", err
	}
	if strings.TrimSpace(question) == "" {
		return "", errors.New("empty question")
	}
	srcs, dir, dirs, err := a.studySources(fileIDs)
	if err != nil {
		return "", err
	}
	j := a.newStudyJob("ask", fileIDs, model)
	err = a.enqueueStudy(j, func(ctx context.Context, a *App, id string) (float64, error) {
		o := a.studyOpts(j.Model, dir, dirs, study.NeedsRead(srcs))
		var parsed study.AskJSON
		cost, err := a.runStudyJSON(ctx, id, study.AskPrompt(srcs, question), o, func(reply string) error {
			parsed = study.ParseAsk(reply)
			if strings.TrimSpace(parsed.Answer) == "" {
				return errors.New("empty answer")
			}
			return nil
		})
		if err != nil {
			return cost, err
		}
		return cost, a.st.InsertStudyAsk(store.StudyAsk{
			FileIDs: fileIDs, Question: strings.TrimSpace(question),
			Answer: parsed.Answer, Citations: parsed.Citations,
			CreatedAt: nowRFC3339(), Model: j.Model,
		})
	})
	if err != nil {
		return j.ID, err
	}
	return j.ID, nil
}

// GetAsks lists past answers touching fileID (0 = all), newest first.
func (a *App) GetAsks(fileID int) ([]AskResult, error) {
	if err := a.studyInit(); err != nil {
		return nil, err
	}
	rows, err := a.st.StudyAsks(fileID)
	if err != nil {
		return nil, err
	}
	out := make([]AskResult, 0, len(rows))
	for _, r := range rows {
		out = append(out, AskResult{Question: r.Question, Answer: r.Answer,
			Citations: r.Citations, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// --------------------------------------------------------------- flashcards

// StartFlashcards queues generation of n cards for one file.
func (a *App) StartFlashcards(fileID int, model string, n int) (string, error) {
	if err := a.studyInit(); err != nil {
		return "", err
	}
	if n <= 0 {
		n = 20
	}
	if n > 100 {
		n = 100
	}
	srcs, dir, dirs, err := a.studySources([]int{fileID})
	if err != nil {
		return "", err
	}
	j := a.newStudyJob("flashcards", []int{fileID}, model)
	err = a.enqueueStudy(j, func(ctx context.Context, a *App, id string) (float64, error) {
		o := a.studyOpts(j.Model, dir, dirs, study.NeedsRead(srcs))
		var cards []study.CardJSON
		cost, err := a.runStudyJSON(ctx, id, study.FlashcardsPrompt(srcs[0], n), o, func(reply string) error {
			c, e := study.ParseCards(reply)
			if e != nil {
				return e
			}
			cards = c
			return nil
		})
		if err != nil {
			return cost, err
		}
		now := nowRFC3339()
		rows := make([]store.StudyCard, 0, len(cards))
		for _, c := range cards {
			rows = append(rows, store.StudyCard{
				FileID: fileID, Front: strings.TrimSpace(c.Front),
				Back: strings.TrimSpace(c.Back), Due: now,
				Interval: 0, Ease: study.DefaultEase,
			})
		}
		return cost, a.st.InsertStudyCards(rows, now)
	})
	if err != nil {
		return j.ID, err
	}
	return j.ID, nil
}

// GetDueFlashcards lists cards due now, soonest first.
func (a *App) GetDueFlashcards(limit int) ([]Flashcard, error) {
	if err := a.studyInit(); err != nil {
		return nil, err
	}
	rows, err := a.st.DueStudyCards(time.Now(), limit)
	if err != nil {
		return nil, err
	}
	out := make([]Flashcard, 0, len(rows))
	for _, c := range rows {
		out = append(out, Flashcard{ID: c.ID, FileID: c.FileID, Front: c.Front,
			Back: c.Back, Due: c.Due, Interval: c.Interval, Ease: c.Ease})
	}
	return out, nil
}

// ReviewFlashcard applies SM-2 lite scheduling for a 0-3 grade
// (again / hard / good / easy).
func (a *App) ReviewFlashcard(id int, grade int) error {
	if err := a.studyInit(); err != nil {
		return err
	}
	c, ok, err := a.st.StudyCardByID(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no flashcard %d", id)
	}
	if grade < 0 {
		grade = 0
	}
	if grade > 3 {
		grade = 3
	}
	next := study.Review(study.Card{Interval: c.Interval, Ease: c.Ease}, grade, time.Now())
	return a.st.UpdateStudyCardSchedule(id, next.Due.UTC().Format(time.RFC3339),
		next.Interval, next.Ease, c.Reps+1)
}

// -------------------------------------------------------------------- misc

// GetFilePageCount returns a PDF's page count, or 0 for other formats.
func (a *App) GetFilePageCount(fileID int) (int, error) {
	if a.st == nil {
		return 0, errors.New("not initialised")
	}
	f, ok, err := a.st.FileByID(fileID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("file %d not in the library", fileID)
	}
	return study.PageCount(f.AbsPath), nil
}
