package main

// Study/AI contract types. JSON field names are the Go field names exactly
// (no json tags) — do not rename. See docs/CONTRACT_STUDY.md.

// StudyStatus reports whether the Claude Code CLI is usable.
type StudyStatus struct {
	CLIFound bool
	Version  string
	LoggedIn bool
	Error    string
	Models   []string
}

// StudyJob is one generation run. Kind is overview|quiz|ask|flashcards;
// Status is queued|running|done|error|cancelled.
type StudyJob struct {
	ID         string
	Kind       string
	FileIDs    []int
	Status     string
	Progress   string
	Error      string
	StartedAt  string
	FinishedAt string
	Model      string
	CostUSD    float64
}

// Overview is a cached study overview of one document.
type Overview struct {
	FileID    int
	Markdown  string
	CreatedAt string
	Model     string
}

// Question is one quiz question. Type is mcq|short; for mcq Answer is the
// option letter A-D, for short it is the model answer.
type Question struct {
	ID          int
	Type        string
	Prompt      string
	Options     []string
	Answer      string
	Explanation string
	Page        int
}

// Quiz is a generated self-test.
type Quiz struct {
	ID        int
	FileIDs   []int
	Title     string
	CreatedAt string
	Model     string
	Questions []Question
}

// QuizAttempt is one recorded run through a quiz. Answers maps question ID to
// the user's answer: an option letter for mcq, "correct"/"wrong" self-marks for
// short questions.
type QuizAttempt struct {
	QuizID  int
	Answers map[int]string
	Score   int
	Total   int
	TakenAt string
}

// Flashcard is one spaced-repetition card.
type Flashcard struct {
	ID       int
	FileID   int
	Front    string
	Back     string
	Due      string
	Interval int
	Ease     float64
}

// AskResult is one answered question about the source material.
type AskResult struct {
	Question  string
	Answer    string // markdown
	Citations []string
	CreatedAt string
}

// StudyProgress is the payload of the `study:progress` event.
type StudyProgress struct {
	JobID string
	Text  string
}
