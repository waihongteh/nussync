package store

import (
	"strings"
	"unicode"
)

// BuildFTSQuery turns a user query into an FTS5 MATCH expression: each token is
// quoted, and the final token gets a prefix `*` so search-as-you-type works.
// Returns "" when the query has no usable tokens.
func BuildFTSQuery(q string) string {
	toks := tokenize(q)
	if len(toks) == 0 {
		return ""
	}
	parts := make([]string, 0, len(toks))
	for i, t := range toks {
		quoted := `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
		if i == len(toks)-1 {
			quoted += "*"
		}
		parts = append(parts, quoted)
	}
	return strings.Join(parts, " AND ")
}

// tokenize splits on anything that is not a letter or digit.
func tokenize(q string) []string {
	fields := strings.FieldsFunc(q, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" {
			out = append(out, strings.ToLower(f))
		}
	}
	return out
}

// Search runs an FTS5 query over file names and extracted text, falling back to
// a LIKE match on the file name when the FTS expression is rejected or empty.
// courseID 0 searches every course.
func (s *Store) Search(query string, courseID, limit int) ([]Hit, error) {
	if limit <= 0 {
		limit = 100
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}

	match := BuildFTSQuery(query)
	if match != "" {
		hits, err := s.searchFTS(match, courseID, limit)
		if err == nil {
			if len(hits) > 0 {
				return hits, nil
			}
		}
		// fall through to LIKE on error or empty result
	}
	return s.searchLike(query, courseID, limit)
}

func (s *Store) searchFTS(match string, courseID, limit int) ([]Hit, error) {
	sqlText := `
		SELECT f.id, f.course_id, f.name, f.rel_path, f.abs_path, f.size, f.modified_at,
		       f.updated_at, f.source, f.module, f.synced, f.content_hash, f.indexed, f.url,
		       COALESCE(f.first_seen_at,''), COALESCE(f.last_changed_at,''),
		       COALESCE(c.code,''),
		       snippet(files_fts, 1, '<b>', '</b>', '…', 12),
		       bm25(files_fts)
		FROM files_fts
		JOIN files f ON f.id = files_fts.rowid
		LEFT JOIN courses c ON c.id = f.course_id
		WHERE files_fts MATCH ?`
	args := []any{match}
	if courseID != 0 {
		sqlText += ` AND f.course_id = ?`
		args = append(args, courseID)
	}
	sqlText += ` ORDER BY bm25(files_fts) LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Hit
	for rows.Next() {
		var h Hit
		var synced, indexed int
		var snip string
		var score float64
		if err := rows.Scan(&h.File.ID, &h.File.CourseID, &h.File.Name, &h.File.RelPath,
			&h.File.AbsPath, &h.File.Size, &h.File.ModifiedAt, &h.File.UpdatedAt,
			&h.File.Source, &h.File.Module, &synced, &h.File.ContentHash, &indexed,
			&h.File.URL, &h.File.FirstSeenAt, &h.File.LastChangedAt,
			&h.CourseCode, &snip, &score); err != nil {
			return nil, err
		}
		h.File.Synced = synced != 0
		h.File.Indexed = indexed != 0
		h.Snippet = snip
		// bm25 is negative-better; flip so larger Score = more relevant.
		h.Score = -score
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) searchLike(query string, courseID, limit int) ([]Hit, error) {
	pattern := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	sqlText := `
		SELECT f.id, f.course_id, f.name, f.rel_path, f.abs_path, f.size, f.modified_at,
		       f.updated_at, f.source, f.module, f.synced, f.content_hash, f.indexed, f.url,
		       COALESCE(f.first_seen_at,''), COALESCE(f.last_changed_at,''),
		       COALESCE(c.code,'')
		FROM files f LEFT JOIN courses c ON c.id = f.course_id
		WHERE (LOWER(f.name) LIKE ? OR LOWER(f.rel_path) LIKE ?)`
	args := []any{pattern, pattern}
	if courseID != 0 {
		sqlText += ` AND f.course_id = ?`
		args = append(args, courseID)
	}
	sqlText += ` ORDER BY f.modified_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Hit
	for rows.Next() {
		var h Hit
		var synced, indexed int
		if err := rows.Scan(&h.File.ID, &h.File.CourseID, &h.File.Name, &h.File.RelPath,
			&h.File.AbsPath, &h.File.Size, &h.File.ModifiedAt, &h.File.UpdatedAt,
			&h.File.Source, &h.File.Module, &synced, &h.File.ContentHash, &indexed,
			&h.File.URL, &h.File.FirstSeenAt, &h.File.LastChangedAt,
			&h.CourseCode); err != nil {
			return nil, err
		}
		h.File.Synced = synced != 0
		h.File.Indexed = indexed != 0
		h.Score = 0
		out = append(out, h)
	}
	return out, rows.Err()
}
