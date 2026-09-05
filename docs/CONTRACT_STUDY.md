# NUSSync — Study/AI contract

Backend = Go methods on `App` in `app_study.go` (package main), bound via
Wails v2. Frontend calls `window.go.main.App.<Method>(...)` (Promise). Types
below are Go; JSON field names are the Go field names exactly (no json tags),
so frontend TS uses the same PascalCase. Same conventions as
`docs/API_CONTRACT.md`.

Everything is **on demand** — nothing here ever runs automatically. All
generation shells out to the locally installed **Claude Code CLI** (`claude`),
which uses the user's Claude subscription; there is no Anthropic API key.

## Types
```go
type StudyStatus struct {
    CLIFound bool; Version string; LoggedIn bool; Error string
    Models []string /*["opus","sonnet","haiku"]*/
}
type StudyJob struct {
    ID string; Kind string /*overview|quiz|ask|flashcards*/
    FileIDs []int
    Status string /*queued|running|done|error|cancelled*/
    Progress string; Error string
    StartedAt string /*RFC3339*/; FinishedAt string /*RFC3339*/
    Model string; CostUSD float64
}
type Overview struct { FileID int; Markdown string; CreatedAt string; Model string }
type Question struct {
    ID int; Type string /*mcq|short*/; Prompt string; Options []string
    Answer string /*mcq: option letter A-D; short: the model answer*/
    Explanation string; Page int
}
type Quiz struct {
    ID int; FileIDs []int; Title string; CreatedAt string; Model string
    Questions []Question
}
type QuizAttempt struct {
    QuizID int; Answers map[int]string /*question ID -> answer*/
    Score int; Total int; TakenAt string
}
type Flashcard struct {
    ID int; FileID int; Front string; Back string
    Due string /*RFC3339*/; Interval int /*days*/; Ease float64
}
type AskResult struct {
    Question string; Answer string /*markdown*/; Citations []string; CreatedAt string
}
type StudyProgress struct { JobID string; Text string } // `study:progress` payload
```

## Methods on App
```
GetStudyStatus() StudyStatus
    // Cached for 10 minutes. CLIFound from locating the binary, Version from
    // `claude --version`, LoggedIn from `claude auth status` falling back to a
    // one-turn probe. Error carries the remediation hint when LoggedIn is false.

StartOverview(fileID int, model string) (jobID string, err error)
GetOverview(fileID int) (Overview, error)      // zero value when none cached

StartQuiz(fileIDs []int, model string, n int) (jobID string, err error)  // n<=0 -> 10, capped at 50
GetQuizzes(fileID int) ([]Quiz, error)         // fileID 0 = every quiz, newest first
GetQuiz(id int) (Quiz, error)
SubmitQuizAttempt(a QuizAttempt) (QuizAttempt, error)
    // Grades and stores. MCQ is graded exactly against the stored option
    // letter (the caller's answer is normalised, so "b", "B)" and the option
    // text all work). Short answers are self-marked: the UI passes "correct"
    // or "wrong". Score/Total/TakenAt are filled in on the returned value.
GetQuizAttempts(quizID int) ([]QuizAttempt, error)   // newest first

StartAsk(fileIDs []int, question string, model string) (jobID string, err error)
GetAsks(fileID int) ([]AskResult, error)       // fileID 0 = all, newest first

StartFlashcards(fileID int, model string, n int) (jobID string, err error)  // n<=0 -> 20, capped at 100
GetDueFlashcards(limit int) ([]Flashcard, error)     // due <= now, soonest first
ReviewFlashcard(id int, grade int) error       // grade 0-3 = again/hard/good/easy, SM-2 lite

GetStudyJob(id string) (StudyJob, error)
GetStudyJobs() ([]StudyJob, error)             // 20 most recent, newest first
CancelStudyJob(id string) error                // kills the claude process tree

GetFilePageCount(fileID int) (int, error)      // PDF page count; 0 when not a PDF
```

All `Start*` methods return immediately with a job ID; the work is queued.
`model` accepts `"opus"`, `"sonnet"`, `"haiku"` (or a full model name); `""`
means `sonnet`.

## Events (EventsEmit from Go)
- `study:job` — payload `StudyJob`, on every status or progress change.
- `study:progress` — payload `StudyProgress` `{JobID, Text}`, one line of live
  progress scraped from the CLI's streaming output.
- `toast` — the shared toast event, on job completion or failure.

## Behaviour
- **One job at a time.** A single worker goroutine drains a 64-slot queue;
  concurrent `claude` processes only trip subscription rate limits.
- **Cancel** kills the whole process tree (`taskkill /T /F /PID`) — the
  launcher spawns a child, so killing only the direct process orphans the work.
- **Cache**: one overview per (file id, model); regenerating overwrites.
  Quizzes, asks and flashcards accumulate.
- **Startup wiring**: `App.startup` should call `a.studyInit()`. It is also
  called lazily by every method above, so the feature works without it.
- **Fallback for pptx/docx/xlsx**: Claude Code's `Read` tool cannot open Office
  formats, so those prompts inline the text NUSSync already extracted for FTS
  (`files_fts`), clamped to 120k characters. PDFs, text, code and images are
  read by Claude directly from the absolute path.
- **JSON replies are parsed leniently**: fences are stripped and the first
  balanced `{...}` is extracted. On failure the job retries once in the same
  session (`--resume`), then once from scratch, before erroring.

## Storage
SQLite tables in the existing database, all prefixed `study_`, created lazily
by `store.MigrateStudy()`: `study_jobs`, `study_overviews`, `study_quizzes`,
`study_questions`, `study_attempts`, `study_asks`, `study_cards`.

## CLI
`nussync --study-test <file> [model] [n]` runs an overview plus an n-question
quiz through the real CLI and prints results, timings and cost.
