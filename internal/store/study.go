package store

import (
	"database/sql"
	"strconv"
	"strings"
	"time"
)

// Study tables. Owned by the Study/AI feature; every table is prefixed
// `study_`. MigrateStudy is called lazily on first use of the feature rather
// than from Store.migrate, so a user who never opens the Study panel never
// pays for it (and so this file stays independent of store.go).
const studySchema = `
CREATE TABLE IF NOT EXISTS study_jobs (
  id          TEXT PRIMARY KEY,
  kind        TEXT NOT NULL DEFAULT '',
  file_ids    TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL DEFAULT 'queued',
  progress    TEXT NOT NULL DEFAULT '',
  error       TEXT NOT NULL DEFAULT '',
  started_at  TEXT NOT NULL DEFAULT '',
  finished_at TEXT NOT NULL DEFAULT '',
  model       TEXT NOT NULL DEFAULT '',
  cost_usd    REAL NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_study_jobs_created ON study_jobs(created_at DESC);

-- One cached overview per (file, model). Regenerating overwrites.
CREATE TABLE IF NOT EXISTS study_overviews (
  file_id    INTEGER NOT NULL,
  model      TEXT NOT NULL DEFAULT '',
  markdown   TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (file_id, model)
);

CREATE TABLE IF NOT EXISTS study_quizzes (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  file_ids   TEXT NOT NULL DEFAULT '',
  title      TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  model      TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS study_questions (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  quiz_id     INTEGER NOT NULL,
  ord         INTEGER NOT NULL DEFAULT 0,
  type        TEXT NOT NULL DEFAULT 'mcq',
  prompt      TEXT NOT NULL DEFAULT '',
  options     TEXT NOT NULL DEFAULT '',
  answer      TEXT NOT NULL DEFAULT '',
  explanation TEXT NOT NULL DEFAULT '',
  page        INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_study_questions_quiz ON study_questions(quiz_id, ord);

CREATE TABLE IF NOT EXISTS study_attempts (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  quiz_id  INTEGER NOT NULL,
  answers  TEXT NOT NULL DEFAULT '',
  score    INTEGER NOT NULL DEFAULT 0,
  total    INTEGER NOT NULL DEFAULT 0,
  taken_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_study_attempts_quiz ON study_attempts(quiz_id, taken_at DESC);

CREATE TABLE IF NOT EXISTS study_asks (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  file_ids   TEXT NOT NULL DEFAULT '',
  question   TEXT NOT NULL DEFAULT '',
  answer     TEXT NOT NULL DEFAULT '',
  citations  TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  model      TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS study_cards (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  file_id    INTEGER NOT NULL,
  front      TEXT NOT NULL DEFAULT '',
  back       TEXT NOT NULL DEFAULT '',
  due        TEXT NOT NULL DEFAULT '',
  interval   INTEGER NOT NULL DEFAULT 0,
  ease       REAL NOT NULL DEFAULT 2.5,
  reps       INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_study_cards_due ON study_cards(due);
`

// MigrateStudy creates the study_* tables. Idempotent.
func (s *Store) MigrateStudy() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(studySchema)
	return err
}

// ------------------------------------------------------------------- helpers

// JoinIDs renders an int slice as the comma-separated form used in the
// file_ids columns.
func JoinIDs(ids []int) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.Itoa(id))
	}
	return strings.Join(parts, ",")
}

// SplitIDs parses the comma-separated form back into ints.
func SplitIDs(s string) []int {
	var out []int
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if n, err := strconv.Atoi(p); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// ---------------------------------------------------------------------- jobs

// StudyJob is one queued or finished generation run.
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
	CreatedAt  string
}

// PutStudyJob inserts or replaces a job row.
func (s *Store) PutStudyJob(j StudyJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j.CreatedAt == "" {
		j.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.Exec(`
		INSERT INTO study_jobs(id, kind, file_ids, status, progress, error,
		                       started_at, finished_at, model, cost_usd, created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			kind=excluded.kind, file_ids=excluded.file_ids, status=excluded.status,
			progress=excluded.progress, error=excluded.error,
			started_at=excluded.started_at, finished_at=excluded.finished_at,
			model=excluded.model, cost_usd=excluded.cost_usd`,
		j.ID, j.Kind, JoinIDs(j.FileIDs), j.Status, j.Progress, j.Error,
		j.StartedAt, j.FinishedAt, j.Model, j.CostUSD, j.CreatedAt)
	return err
}

const studyJobSelect = `SELECT id, kind, file_ids, status, progress, error,
	started_at, finished_at, model, cost_usd, created_at FROM study_jobs`

func scanStudyJobs(rows *sql.Rows) ([]StudyJob, error) {
	var out []StudyJob
	for rows.Next() {
		var j StudyJob
		var ids string
		if err := rows.Scan(&j.ID, &j.Kind, &ids, &j.Status, &j.Progress, &j.Error,
			&j.StartedAt, &j.FinishedAt, &j.Model, &j.CostUSD, &j.CreatedAt); err != nil {
			return nil, err
		}
		j.FileIDs = SplitIDs(ids)
		out = append(out, j)
	}
	return out, rows.Err()
}

// StudyJobByID loads one job.
func (s *Store) StudyJobByID(id string) (StudyJob, bool, error) {
	rows, err := s.db.Query(studyJobSelect+` WHERE id=?`, id)
	if err != nil {
		return StudyJob{}, false, err
	}
	defer rows.Close()
	js, err := scanStudyJobs(rows)
	if err != nil || len(js) == 0 {
		return StudyJob{}, false, err
	}
	return js[0], true, nil
}

// StudyJobs lists the most recent jobs, newest first.
func (s *Store) StudyJobs(limit int) ([]StudyJob, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(studyJobSelect+` ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStudyJobs(rows)
}

// ResetRunningStudyJobs marks jobs left running by a crash as errored.
func (s *Store) ResetRunningStudyJobs() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`
		UPDATE study_jobs SET status='error', error='interrupted by app restart',
		       finished_at=? WHERE status IN ('queued','running')`,
		time.Now().UTC().Format(time.RFC3339))
	return err
}

// ----------------------------------------------------------------- overviews

// StudyOverview is a cached document overview.
type StudyOverview struct {
	FileID    int
	Model     string
	Markdown  string
	CreatedAt string
}

// PutStudyOverview stores (replacing) the overview for a file+model.
func (s *Store) PutStudyOverview(o StudyOverview) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`
		INSERT INTO study_overviews(file_id, model, markdown, created_at)
		VALUES(?,?,?,?)
		ON CONFLICT(file_id, model) DO UPDATE SET
			markdown=excluded.markdown, created_at=excluded.created_at`,
		o.FileID, o.Model, o.Markdown, o.CreatedAt)
	return err
}

// StudyOverviewFor returns the newest overview of a file across models.
func (s *Store) StudyOverviewFor(fileID int) (StudyOverview, bool, error) {
	var o StudyOverview
	err := s.db.QueryRow(`
		SELECT file_id, model, markdown, created_at FROM study_overviews
		WHERE file_id=? ORDER BY created_at DESC LIMIT 1`, fileID).
		Scan(&o.FileID, &o.Model, &o.Markdown, &o.CreatedAt)
	if err == sql.ErrNoRows {
		return StudyOverview{}, false, nil
	}
	return o, err == nil, err
}

// ------------------------------------------------------------------- quizzes

// StudyQuestion is one stored quiz question.
type StudyQuestion struct {
	ID          int
	Type        string
	Prompt      string
	Options     []string
	Answer      string
	Explanation string
	Page        int
}

// StudyQuiz is a stored quiz with its questions.
type StudyQuiz struct {
	ID        int
	FileIDs   []int
	Title     string
	CreatedAt string
	Model     string
	Questions []StudyQuestion
}

const optSep = "\x1f" // unit separator: never appears in generated option text

// InsertStudyQuiz writes a quiz and its questions, returning the new quiz id.
func (s *Store) InsertStudyQuiz(q StudyQuiz) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO study_quizzes(file_ids, title, created_at, model)
		VALUES(?,?,?,?)`, JoinIDs(q.FileIDs), q.Title, q.CreatedAt, q.Model)
	if err != nil {
		return 0, err
	}
	id64, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for i, qq := range q.Questions {
		if _, err := tx.Exec(`
			INSERT INTO study_questions(quiz_id, ord, type, prompt, options, answer, explanation, page)
			VALUES(?,?,?,?,?,?,?,?)`,
			id64, i, qq.Type, qq.Prompt, strings.Join(qq.Options, optSep),
			qq.Answer, qq.Explanation, qq.Page); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(id64), nil
}

// StudyQuizByID loads one quiz with its questions.
func (s *Store) StudyQuizByID(id int) (StudyQuiz, bool, error) {
	var q StudyQuiz
	var ids string
	err := s.db.QueryRow(`SELECT id, file_ids, title, created_at, model FROM study_quizzes WHERE id=?`, id).
		Scan(&q.ID, &ids, &q.Title, &q.CreatedAt, &q.Model)
	if err == sql.ErrNoRows {
		return StudyQuiz{}, false, nil
	}
	if err != nil {
		return StudyQuiz{}, false, err
	}
	q.FileIDs = SplitIDs(ids)
	qs, err := s.studyQuestions(id)
	if err != nil {
		return StudyQuiz{}, false, err
	}
	q.Questions = qs
	return q, true, nil
}

func (s *Store) studyQuestions(quizID int) ([]StudyQuestion, error) {
	rows, err := s.db.Query(`
		SELECT id, type, prompt, options, answer, explanation, page
		FROM study_questions WHERE quiz_id=? ORDER BY ord, id`, quizID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StudyQuestion
	for rows.Next() {
		var q StudyQuestion
		var opts string
		if err := rows.Scan(&q.ID, &q.Type, &q.Prompt, &opts, &q.Answer, &q.Explanation, &q.Page); err != nil {
			return nil, err
		}
		if opts != "" {
			q.Options = strings.Split(opts, optSep)
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// StudyQuizzes lists quizzes that include fileID (0 = every quiz), newest first.
func (s *Store) StudyQuizzes(fileID int) ([]StudyQuiz, error) {
	rows, err := s.db.Query(`SELECT id, file_ids, title, created_at, model
		FROM study_quizzes ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type row struct {
		q   StudyQuiz
		ids string
	}
	var raw []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.q.ID, &r.ids, &r.q.Title, &r.q.CreatedAt, &r.q.Model); err != nil {
			return nil, err
		}
		raw = append(raw, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []StudyQuiz
	for _, r := range raw {
		r.q.FileIDs = SplitIDs(r.ids)
		if fileID != 0 && !containsInt(r.q.FileIDs, fileID) {
			continue
		}
		qs, err := s.studyQuestions(r.q.ID)
		if err != nil {
			return nil, err
		}
		r.q.Questions = qs
		out = append(out, r.q)
	}
	return out, nil
}

func containsInt(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// ------------------------------------------------------------------ attempts

// StudyAttempt is one recorded quiz attempt. Answers is opaque JSON.
type StudyAttempt struct {
	QuizID  int
	Answers string
	Score   int
	Total   int
	TakenAt string
}

// InsertStudyAttempt records an attempt.
func (s *Store) InsertStudyAttempt(a StudyAttempt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO study_attempts(quiz_id, answers, score, total, taken_at)
		VALUES(?,?,?,?,?)`, a.QuizID, a.Answers, a.Score, a.Total, a.TakenAt)
	return err
}

// StudyAttempts lists a quiz's attempts, newest first.
func (s *Store) StudyAttempts(quizID int) ([]StudyAttempt, error) {
	rows, err := s.db.Query(`SELECT quiz_id, answers, score, total, taken_at
		FROM study_attempts WHERE quiz_id=? ORDER BY taken_at DESC, id DESC`, quizID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StudyAttempt
	for rows.Next() {
		var a StudyAttempt
		if err := rows.Scan(&a.QuizID, &a.Answers, &a.Score, &a.Total, &a.TakenAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------- asks

// StudyAsk is one stored question-and-answer.
type StudyAsk struct {
	FileIDs   []int
	Question  string
	Answer    string
	Citations []string
	CreatedAt string
	Model     string
}

const citeSep = "\x1f"

// InsertStudyAsk records a Q&A result.
func (s *Store) InsertStudyAsk(a StudyAsk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO study_asks(file_ids, question, answer, citations, created_at, model)
		VALUES(?,?,?,?,?,?)`, JoinIDs(a.FileIDs), a.Question, a.Answer,
		strings.Join(a.Citations, citeSep), a.CreatedAt, a.Model)
	return err
}

// StudyAsks lists Q&As touching fileID (0 = all), newest first.
func (s *Store) StudyAsks(fileID int) ([]StudyAsk, error) {
	rows, err := s.db.Query(`SELECT file_ids, question, answer, citations, created_at, model
		FROM study_asks ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StudyAsk
	for rows.Next() {
		var a StudyAsk
		var ids, cites string
		if err := rows.Scan(&ids, &a.Question, &a.Answer, &cites, &a.CreatedAt, &a.Model); err != nil {
			return nil, err
		}
		a.FileIDs = SplitIDs(ids)
		if fileID != 0 && !containsInt(a.FileIDs, fileID) {
			continue
		}
		if cites != "" {
			a.Citations = strings.Split(cites, citeSep)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// --------------------------------------------------------------- flashcards

// StudyCard is one flashcard with its scheduling state.
type StudyCard struct {
	ID       int
	FileID   int
	Front    string
	Back     string
	Due      string
	Interval int
	Ease     float64
	Reps     int
}

// InsertStudyCards adds a batch of new cards.
func (s *Store) InsertStudyCards(cards []StudyCard, createdAt string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range cards {
		if _, err := tx.Exec(`
			INSERT INTO study_cards(file_id, front, back, due, interval, ease, reps, created_at)
			VALUES(?,?,?,?,?,?,?,?)`,
			c.FileID, c.Front, c.Back, c.Due, c.Interval, c.Ease, c.Reps, createdAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DueStudyCards lists cards due at or before now, soonest first.
func (s *Store) DueStudyCards(now time.Time, limit int) ([]StudyCard, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT id, file_id, front, back, due, interval, ease, reps FROM study_cards
		WHERE due <= ? ORDER BY due ASC, id ASC LIMIT ?`,
		now.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StudyCard
	for rows.Next() {
		var c StudyCard
		if err := rows.Scan(&c.ID, &c.FileID, &c.Front, &c.Back, &c.Due,
			&c.Interval, &c.Ease, &c.Reps); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// StudyCardByID loads one card.
func (s *Store) StudyCardByID(id int) (StudyCard, bool, error) {
	var c StudyCard
	err := s.db.QueryRow(`SELECT id, file_id, front, back, due, interval, ease, reps
		FROM study_cards WHERE id=?`, id).
		Scan(&c.ID, &c.FileID, &c.Front, &c.Back, &c.Due, &c.Interval, &c.Ease, &c.Reps)
	if err == sql.ErrNoRows {
		return StudyCard{}, false, nil
	}
	return c, err == nil, err
}

// UpdateStudyCardSchedule writes back a card's post-review scheduling state.
func (s *Store) UpdateStudyCardSchedule(id int, due string, interval int, ease float64, reps int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE study_cards SET due=?, interval=?, ease=?, reps=? WHERE id=?`,
		due, interval, ease, reps, id)
	return err
}

// StudyFileText returns the FTS-indexed extracted text of a file, for formats
// Claude Code's Read tool cannot open (pptx/docx/xlsx).
func (s *Store) StudyFileText(fileID int) (string, error) {
	var text string
	err := s.db.QueryRow(`SELECT text FROM files_fts WHERE rowid=?`, fileID).Scan(&text)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return text, err
}
