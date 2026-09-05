package store

import (
	"database/sql"
	"strings"
	"time"
)

// KVFeedSeen is the kv key holding when the user last opened the what's-new
// feed (RFC3339). Empty means "never".
const KVFeedSeen = "feed_seen_at"

// FeedSeenAt returns the last time the feed was marked seen ("" if never).
func (s *Store) FeedSeenAt() string {
	v, _ := s.GetKV(KVFeedSeen)
	return v
}

// MarkFeedSeen stamps the feed as viewed at t.
func (s *Store) MarkFeedSeen(t time.Time) error {
	return s.SetKV(KVFeedSeen, t.UTC().Format(time.RFC3339))
}

const feedSelect = `
	SELECT f.id, f.course_id, f.name, f.rel_path, f.abs_path, f.size, f.modified_at,
	       f.updated_at, f.source, f.module, f.synced, f.content_hash, f.indexed, f.url,
	       COALESCE(f.origin,''), COALESCE(f.first_seen_at,''), COALESCE(f.last_changed_at,''),
	       COALESCE(c.code,'')
	FROM files f LEFT JOIN courses c ON c.id = f.course_id
	WHERE COALESCE(f.last_changed_at,'') >= ?`

func scanFeed(rows *sql.Rows) ([]FeedFile, error) {
	var out []FeedFile
	for rows.Next() {
		var (
			ff              FeedFile
			synced, indexed int
			f               = &ff.File
		)
		if err := rows.Scan(&f.ID, &f.CourseID, &f.Name, &f.RelPath, &f.AbsPath, &f.Size,
			&f.ModifiedAt, &f.UpdatedAt, &f.Source, &f.Module, &synced, &f.ContentHash,
			&indexed, &f.URL, &f.Origin, &f.FirstSeenAt, &f.LastChangedAt,
			&ff.CourseCode); err != nil {
			return nil, err
		}
		f.Synced = synced != 0
		f.Indexed = indexed != 0
		ff.ChangedAt = f.LastChangedAt
		ff.New = f.FirstSeenAt != "" && f.FirstSeenAt == f.LastChangedAt
		out = append(out, ff)
	}
	return out, rows.Err()
}

// ChangedFiles lists files whose last_changed_at is at or after since, newest
// first. limit <= 0 means no limit. Timestamps are compared as RFC3339 UTC
// strings, which sort lexicographically.
func (s *Store) ChangedFiles(since time.Time, limit int) ([]FeedFile, error) {
	q := feedSelect + ` ORDER BY f.last_changed_at DESC, f.id DESC`
	args := []any{stamp(since)}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeed(rows)
}

// UnseenCount counts files changed strictly after the feed was last marked
// seen. When the feed has never been seen, everything with a stamp counts.
func (s *Store) UnseenCount() (int, error) {
	seen := s.FeedSeenAt()
	var n int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM files
		WHERE COALESCE(last_changed_at,'') != '' AND last_changed_at > ?`, seen).Scan(&n)
	return n, err
}

// stamp renders t as the RFC3339 UTC string used in the file columns.
func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// ------------------------------------------------ assignment detail cache

// AssignmentDetailGet loads the cached detail for an assignment. ok is false
// when absent; callers decide whether FetchedAt is too old.
func (s *Store) AssignmentDetailGet(assignmentID int) (AssignmentDetail, bool, error) {
	var (
		d               = AssignmentDetail{AssignmentID: assignmentID}
		types           string
		graded, hasStat int
		st              ScoreStats
	)
	err := s.db.QueryRow(`
		SELECT course_id, description, submission_types, score, graded,
		       has_stats, stat_mean, stat_min, stat_max, stat_median, stat_count, fetched_at
		FROM assignment_cache WHERE assignment_id=?`, assignmentID).
		Scan(&d.CourseID, &d.Description, &types, &d.Score, &graded,
			&hasStat, &st.Mean, &st.Min, &st.Max, &st.Median, &st.Count, &d.FetchedAt)
	if err == sql.ErrNoRows {
		return AssignmentDetail{}, false, nil
	}
	if err != nil {
		return AssignmentDetail{}, false, err
	}
	d.Graded = graded != 0
	if types != "" {
		d.SubmissionTypes = strings.Split(types, ",")
	}
	if hasStat != 0 {
		cp := st
		d.Stats = &cp
	}
	return d, true, nil
}

// AssignmentDetailPut stores (or refreshes) one cached assignment detail.
func (s *Store) AssignmentDetailPut(d AssignmentDetail) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var (
		st      ScoreStats
		hasStat int
	)
	if d.Stats != nil {
		st = *d.Stats
		hasStat = 1
	}
	if d.FetchedAt == "" {
		d.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.Exec(`
		INSERT INTO assignment_cache(assignment_id, course_id, description, submission_types,
		                             score, graded, has_stats, stat_mean, stat_min, stat_max,
		                             stat_median, stat_count, fetched_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(assignment_id) DO UPDATE SET
			course_id=excluded.course_id, description=excluded.description,
			submission_types=excluded.submission_types, score=excluded.score,
			graded=excluded.graded, has_stats=excluded.has_stats,
			stat_mean=excluded.stat_mean, stat_min=excluded.stat_min,
			stat_max=excluded.stat_max, stat_median=excluded.stat_median,
			stat_count=excluded.stat_count, fetched_at=excluded.fetched_at`,
		d.AssignmentID, d.CourseID, d.Description, strings.Join(d.SubmissionTypes, ","),
		d.Score, b2i(d.Graded), hasStat, st.Mean, st.Min, st.Max, st.Median, st.Count,
		d.FetchedAt)
	return err
}

// DeadlineByID loads a single deadline row.
func (s *Store) DeadlineByID(id int) (Deadline, bool, error) {
	var d Deadline
	var sub int
	err := s.db.QueryRow(`
		SELECT id, course_id, course_code, title, type, due_at, submitted, url, points_possible
		FROM deadlines WHERE id=?`, id).
		Scan(&d.ID, &d.CourseID, &d.CourseCode, &d.Title, &d.Type, &d.DueAt, &sub,
			&d.URL, &d.PointsPossible)
	if err == sql.ErrNoRows {
		return Deadline{}, false, nil
	}
	if err != nil {
		return Deadline{}, false, err
	}
	d.Submitted = sub != 0
	return d, true, nil
}

// FilesByIDs loads file rows for the given Canvas file ids, preserving the
// order of ids and silently skipping ones we have never synced.
func (s *Store) FilesByIDs(ids []int) ([]File, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.Query(fileSelect+` WHERE id IN (`+ph+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found, err := scanFiles(rows)
	if err != nil {
		return nil, err
	}
	byID := make(map[int]File, len(found))
	for _, f := range found {
		byID[f.ID] = f
	}
	out := make([]File, 0, len(ids))
	for _, id := range ids {
		if f, ok := byID[id]; ok {
			out = append(out, f)
		}
	}
	return out, nil
}
