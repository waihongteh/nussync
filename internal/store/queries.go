package store

import (
	"database/sql"
	"time"
)

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------- courses

// UpsertCourse inserts or updates a course, preserving the enabled flag.
func (s *Store) UpsertCourse(c Course) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`
		INSERT INTO courses(id, code, name, term, enabled, last_synced)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			code=excluded.code, name=excluded.name, term=excluded.term`,
		c.ID, c.Code, c.Name, c.Term, b2i(c.Enabled), c.LastSynced)
	return err
}

// SetCourseEnabled toggles whether a course participates in sync.
func (s *Store) SetCourseEnabled(id int, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE courses SET enabled=? WHERE id=?`, b2i(enabled), id)
	return err
}

// TouchCourseSynced records the time a course finished syncing.
func (s *Store) TouchCourseSynced(id int, t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE courses SET last_synced=? WHERE id=?`, t.Format(time.RFC3339), id)
	return err
}

// Courses lists all known courses with their local file counts.
func (s *Store) Courses() ([]Course, error) {
	rows, err := s.db.Query(`
		SELECT c.id, c.code, c.name, c.term, c.enabled, c.last_synced,
		       (SELECT COUNT(*) FROM files f WHERE f.course_id = c.id) AS n
		FROM courses c ORDER BY c.code, c.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Course
	for rows.Next() {
		var c Course
		var en int
		if err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.Term, &en, &c.LastSynced, &c.FileCount); err != nil {
			return nil, err
		}
		c.Enabled = en != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// EnabledCourses returns only courses selected for sync.
func (s *Store) EnabledCourses() ([]Course, error) {
	all, err := s.Courses()
	if err != nil {
		return nil, err
	}
	var out []Course
	for _, c := range all {
		if c.Enabled {
			out = append(out, c)
		}
	}
	return out, nil
}

// ------------------------------------------------------------------ files

// UpsertFile inserts or updates a file row (keyed by Canvas file id).
func (s *Store) UpsertFile(f File) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// first_seen_at is write-once; last_changed_at only moves forward when the
	// caller supplies one (an unchanged re-upsert passes "" and keeps the old
	// stamp). Rows written before the feed existed keep their empty stamps.
	_, err := s.db.Exec(`
		INSERT INTO files(id, course_id, name, rel_path, abs_path, size, modified_at,
		                  updated_at, source, module, synced, content_hash, indexed, url, origin,
		                  first_seen_at, last_changed_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			course_id=excluded.course_id, name=excluded.name, rel_path=excluded.rel_path,
			abs_path=excluded.abs_path, size=excluded.size, modified_at=excluded.modified_at,
			updated_at=excluded.updated_at, source=excluded.source, module=excluded.module,
			synced=excluded.synced, content_hash=excluded.content_hash,
			indexed=excluded.indexed, url=excluded.url, origin=excluded.origin,
			first_seen_at=CASE WHEN COALESCE(files.first_seen_at,'')=''
			                   THEN excluded.first_seen_at ELSE files.first_seen_at END,
			last_changed_at=CASE WHEN COALESCE(excluded.last_changed_at,'')=''
			                     THEN files.last_changed_at ELSE excluded.last_changed_at END`,
		f.ID, f.CourseID, f.Name, f.RelPath, f.AbsPath, f.Size, f.ModifiedAt,
		f.UpdatedAt, f.Source, f.Module, b2i(f.Synced), f.ContentHash, b2i(f.Indexed),
		f.URL, f.Origin, f.FirstSeenAt, f.LastChangedAt)
	return err
}

// FileByID loads one file row.
func (s *Store) FileByID(id int) (File, bool, error) {
	rows, err := s.db.Query(fileSelect+` WHERE id=?`, id)
	if err != nil {
		return File{}, false, err
	}
	defer rows.Close()
	fs, err := scanFiles(rows)
	if err != nil || len(fs) == 0 {
		return File{}, false, err
	}
	return fs[0], true, nil
}

const fileSelect = `SELECT id, course_id, name, rel_path, abs_path, size, modified_at,
       updated_at, source, module, synced, content_hash, indexed, url,
       COALESCE(origin,''), COALESCE(first_seen_at,''), COALESCE(last_changed_at,'')
       FROM files`

func scanFiles(rows *sql.Rows) ([]File, error) {
	var out []File
	for rows.Next() {
		var f File
		var synced, indexed int
		if err := rows.Scan(&f.ID, &f.CourseID, &f.Name, &f.RelPath, &f.AbsPath, &f.Size,
			&f.ModifiedAt, &f.UpdatedAt, &f.Source, &f.Module, &synced, &f.ContentHash,
			&indexed, &f.URL, &f.Origin, &f.FirstSeenAt, &f.LastChangedAt); err != nil {
			return nil, err
		}
		f.Synced = synced != 0
		f.Indexed = indexed != 0
		out = append(out, f)
	}
	return out, rows.Err()
}

// FilesByCourse lists a course's files ordered by path.
func (s *Store) FilesByCourse(courseID int) ([]File, error) {
	rows, err := s.db.Query(fileSelect+` WHERE course_id=? ORDER BY rel_path, name`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

// RecentFiles lists the newest files across all courses.
func (s *Store) RecentFiles(limit int) ([]File, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(fileSelect+` ORDER BY modified_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

// UnindexedFiles lists synced files that still need text extraction.
func (s *Store) UnindexedFiles() ([]File, error) {
	rows, err := s.db.Query(fileSelect + ` WHERE synced=1 AND indexed=0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

// MarkIndexed records a file as indexed and refreshes its FTS row.
func (s *Store) MarkIndexed(id int, name, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM files_fts WHERE rowid=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO files_fts(rowid, name, text) VALUES(?,?,?)`, id, name, text); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE files SET indexed=1 WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// MarkUnsynced clears the synced flag (local file vanished).
func (s *Store) MarkUnsynced(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE files SET synced=0, indexed=0 WHERE id=?`, id)
	return err
}

// -------------------------------------------------------------- deadlines

// ReplaceDeadlines swaps in a fresh set of deadlines for one course.
func (s *Store) ReplaceDeadlines(courseID int, ds []Deadline) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM deadlines WHERE course_id=?`, courseID); err != nil {
		return err
	}
	for _, d := range ds {
		if _, err := tx.Exec(`
			INSERT INTO deadlines(id, course_id, course_code, title, type, due_at, submitted, url, points_possible)
			VALUES(?,?,?,?,?,?,?,?,?)`,
			d.ID, d.CourseID, d.CourseCode, d.Title, d.Type, d.DueAt, b2i(d.Submitted), d.URL, d.PointsPossible); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Deadlines lists upcoming deadlines plus overdue-but-unsubmitted ones.
func (s *Store) Deadlines(now time.Time) ([]Deadline, error) {
	rows, err := s.db.Query(`
		SELECT id, course_id, course_code, title, type, due_at, submitted, url, points_possible
		FROM deadlines WHERE due_at != '' ORDER BY due_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Deadline
	for rows.Next() {
		var d Deadline
		var sub int
		if err := rows.Scan(&d.ID, &d.CourseID, &d.CourseCode, &d.Title, &d.Type,
			&d.DueAt, &sub, &d.URL, &d.PointsPossible); err != nil {
			return nil, err
		}
		d.Submitted = sub != 0
		due, err := time.Parse(time.RFC3339, d.DueAt)
		if err != nil {
			continue
		}
		if due.After(now) || !d.Submitted {
			out = append(out, d)
		}
	}
	return out, rows.Err()
}

// PendingDeadlines lists unsubmitted deadlines still in the future.
func (s *Store) PendingDeadlines(now time.Time) ([]Deadline, error) {
	all, err := s.Deadlines(now)
	if err != nil {
		return nil, err
	}
	var out []Deadline
	for _, d := range all {
		if d.Submitted {
			continue
		}
		due, err := time.Parse(time.RFC3339, d.DueAt)
		if err != nil || !due.After(now) {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

// ---------------------------------------------------------- announcements

// UpsertAnnouncement stores an announcement, returning true when it is new.
func (s *Store) UpsertAnnouncement(a Announcement) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var exists int
	err := s.db.QueryRow(`SELECT 1 FROM announcements WHERE id=?`, a.ID).Scan(&exists)
	isNew := err == sql.ErrNoRows
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	_, err = s.db.Exec(`
		INSERT INTO announcements(id, course_id, course_code, title, posted_at, html, text, url, read, notified)
		VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			course_code=excluded.course_code, title=excluded.title, posted_at=excluded.posted_at,
			html=excluded.html, text=excluded.text, url=excluded.url`,
		a.ID, a.CourseID, a.CourseCode, a.Title, a.PostedAt, a.HTML, a.Text, a.URL,
		b2i(a.Read), b2i(a.Notified))
	return isNew, err
}

// Announcements lists announcements newest first.
func (s *Store) Announcements(limit int) ([]Announcement, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT id, course_id, course_code, title, posted_at, html, text, url, read, notified
		FROM announcements ORDER BY posted_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAnnouncements(rows)
}

// UnnotifiedAnnouncements lists announcements not yet pushed to Telegram.
func (s *Store) UnnotifiedAnnouncements() ([]Announcement, error) {
	rows, err := s.db.Query(`
		SELECT id, course_id, course_code, title, posted_at, html, text, url, read, notified
		FROM announcements WHERE notified=0 ORDER BY posted_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAnnouncements(rows)
}

func scanAnnouncements(rows *sql.Rows) ([]Announcement, error) {
	var out []Announcement
	for rows.Next() {
		var a Announcement
		var read, notified int
		if err := rows.Scan(&a.ID, &a.CourseID, &a.CourseCode, &a.Title, &a.PostedAt,
			&a.HTML, &a.Text, &a.URL, &read, &notified); err != nil {
			return nil, err
		}
		a.Read = read != 0
		a.Notified = notified != 0
		out = append(out, a)
	}
	return out, rows.Err()
}

// MarkAnnouncementRead flags an announcement as read in the UI.
func (s *Store) MarkAnnouncementRead(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE announcements SET read=1 WHERE id=?`, id)
	return err
}

// MarkAnnouncementNotified flags an announcement as pushed.
func (s *Store) MarkAnnouncementNotified(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE announcements SET notified=1 WHERE id=?`, id)
	return err
}

// ----------------------------------------------------------------- grades

// UpsertGrade stores a grade, returning true when it is new or newly changed.
func (s *Store) UpsertGrade(g Grade) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var prevGradedAt string
	var prevScore float64
	err := s.db.QueryRow(`SELECT graded_at, score FROM grades WHERE assignment_id=?`, g.AssignmentID).
		Scan(&prevGradedAt, &prevScore)
	changed := err == sql.ErrNoRows || prevGradedAt != g.GradedAt || prevScore != g.Score
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	notified := b2i(g.Notified)
	if changed {
		notified = 0
	}
	_, err = s.db.Exec(`
		INSERT INTO grades(assignment_id, course_id, course_code, title, score, possible, graded_at, url, notified)
		VALUES(?,?,?,?,?,?,?,?,?)
		ON CONFLICT(assignment_id) DO UPDATE SET
			course_code=excluded.course_code, title=excluded.title, score=excluded.score,
			possible=excluded.possible, graded_at=excluded.graded_at, url=excluded.url,
			notified=excluded.notified`,
		g.AssignmentID, g.CourseID, g.CourseCode, g.Title, g.Score, g.Possible,
		g.GradedAt, g.URL, notified)
	return changed, err
}

// Grades lists graded submissions newest first.
func (s *Store) Grades() ([]Grade, error) {
	rows, err := s.db.Query(gradeSelect + ` ORDER BY g.graded_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGrades(rows)
}

// UnnotifiedGrades lists grades not yet pushed to Telegram.
func (s *Store) UnnotifiedGrades() ([]Grade, error) {
	rows, err := s.db.Query(gradeSelect + ` WHERE g.notified=0 ORDER BY g.graded_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGrades(rows)
}

// gradeSelect joins the cached class mean (0 when the assignment detail has
// never been fetched) onto every grade row.
const gradeSelect = `
	SELECT g.assignment_id, g.course_id, g.course_code, g.title, g.score, g.possible,
	       g.graded_at, g.url, g.notified,
	       COALESCE(CASE WHEN ac.has_stats=1 THEN ac.stat_mean END, 0)
	FROM grades g LEFT JOIN assignment_cache ac ON ac.assignment_id = g.assignment_id`

func scanGrades(rows *sql.Rows) ([]Grade, error) {
	var out []Grade
	for rows.Next() {
		var g Grade
		var n int
		if err := rows.Scan(&g.AssignmentID, &g.CourseID, &g.CourseCode, &g.Title,
			&g.Score, &g.Possible, &g.GradedAt, &g.URL, &n, &g.Mean); err != nil {
			return nil, err
		}
		g.Notified = n != 0
		out = append(out, g)
	}
	return out, rows.Err()
}

// MarkGradeNotified flags a grade as pushed.
func (s *Store) MarkGradeNotified(assignmentID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE grades SET notified=1 WHERE assignment_id=?`, assignmentID)
	return err
}

// -------------------------------------------------------------- reminders

// ReminderSent reports whether a ladder rung has already fired.
func (s *Store) ReminderSent(assignmentID int, rung string) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM reminders_sent WHERE assignment_id=? AND rung=?`,
		assignmentID, rung).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// MarkReminderSent records a fired (or suppressed) ladder rung.
func (s *Store) MarkReminderSent(assignmentID int, rung string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`
		INSERT INTO reminders_sent(assignment_id, rung, sent_at) VALUES(?,?,?)
		ON CONFLICT(assignment_id, rung) DO NOTHING`,
		assignmentID, rung, at.Format(time.RFC3339))
	return err
}

// ------------------------------------------------------------------ stats

// Stats summarises the local library.
func (s *Store) Stats() (Stats, error) {
	var st Stats
	var bytes sql.NullInt64
	if err := s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(size),0) FROM files WHERE synced=1`).
		Scan(&st.Files, &bytes); err != nil {
		return st, err
	}
	st.Bytes = bytes.Int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM courses WHERE enabled=1`).Scan(&st.Courses); err != nil {
		return st, err
	}
	ds, err := s.Deadlines(time.Now())
	if err != nil {
		return st, err
	}
	st.Deadlines = len(ds)
	st.LastSync, _ = s.GetKV("last_sync")
	return st, nil
}

// RewriteCoursePaths rewrites the abs_path prefix of every file in a course
// after its on-disk folder was renamed. Both prefixes are native paths.
func (s *Store) RewriteCoursePaths(courseID int, oldPrefix, newPrefix string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(
		`UPDATE files SET abs_path = ? || substr(abs_path, ?) WHERE course_id=? AND substr(abs_path,1,?) = ?`,
		newPrefix, len(oldPrefix)+1, courseID, len(oldPrefix), oldPrefix)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
