package sync

import (
	"context"
	"errors"
	"fmt"
	"html"
	"regexp"
	"strings"

	"nussync/internal/canvas"
	"nussync/internal/store"
)

// RefreshMetadata refreshes courses plus deadlines/announcements/grades for
// every enabled course, without downloading any files.
func (e *Engine) RefreshMetadata(ctx context.Context) error {
	courses, err := e.Client.ActiveCourses(ctx)
	if err != nil {
		return err
	}
	active := map[int]bool{}
	for _, c := range courses {
		term := ""
		if c.Term != nil {
			term = c.Term.Name
		}
		active[c.ID] = true
		if err := e.Store.UpsertCourse(store.Course{
			ID: c.ID, Code: CourseCode(c), Name: c.Name, Term: term, Enabled: true,
		}); err != nil {
			return err
		}
	}
	enabled, err := e.Store.EnabledCourses()
	if err != nil {
		return err
	}
	var errs []string
	for _, c := range enabled {
		if !active[c.ID] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		e.report(Progress{Phase: "listing", Course: c.Code})
		if err := e.refreshCourseMeta(ctx, c); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			errs = append(errs, fmt.Sprintf("%s: %v", c.Code, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d course(s) failed: %s", len(errs), strings.Join(errs, "; "))
	}
	return nil
}

// refreshCourseMeta pulls deadlines, grades and announcements for one course.
func (e *Engine) refreshCourseMeta(ctx context.Context, c store.Course) error {
	var firstErr error

	assignments, err := e.Client.Assignments(ctx, c.ID)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		if !canvas.IsPermission(err) && !canvas.IsNotFound(err) {
			firstErr = err
		}
	} else {
		var ds []store.Deadline
		for _, a := range assignments {
			if a.DueAt != nil && !a.DueAt.IsZero() {
				ds = append(ds, store.Deadline{
					ID:             a.ID,
					CourseID:       c.ID,
					CourseCode:     c.Code,
					Title:          a.Name,
					Type:           a.Kind(),
					DueAt:          rfc(*a.DueAt),
					Submitted:      a.Submitted(),
					URL:            a.HTMLURL,
					PointsPossible: a.PointsPossible,
				})
			}
			if s := a.Submission; s != nil && s.WorkflowState == "graded" && s.Score != nil && s.GradedAt != nil {
				if _, err := e.Store.UpsertGrade(store.Grade{
					AssignmentID: a.ID,
					CourseID:     c.ID,
					CourseCode:   c.Code,
					Title:        a.Name,
					Score:        *s.Score,
					Possible:     a.PointsPossible,
					GradedAt:     rfc(*s.GradedAt),
					URL:          a.HTMLURL,
				}); err != nil {
					return err
				}
			}
		}
		if err := e.Store.ReplaceDeadlines(c.ID, ds); err != nil {
			return err
		}
	}

	anns, err := e.Client.Announcements(ctx, c.ID)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		if firstErr == nil && !canvas.IsPermission(err) && !canvas.IsNotFound(err) {
			firstErr = err
		}
	} else {
		for _, a := range anns {
			url := a.HTMLURL
			if url == "" {
				url = a.URL
			}
			isNew, err := e.Store.UpsertAnnouncement(store.Announcement{
				ID:         a.ID,
				CourseID:   c.ID,
				CourseCode: c.Code,
				Title:      a.Title,
				PostedAt:   rfc(a.Posted()),
				HTML:       a.Message,
				Text:       HTMLToText(a.Message),
				URL:        url,
			})
			if err != nil {
				return err
			}
			if isNew {
				e.newAnns = append(e.newAnns, a.ID)
			}
		}
	}

	return firstErr
}

var (
	// RE2 has no backreferences, so each element is spelled out.
	dropRe  = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>|<script\b[^>]*>.*?</script>|<head\b[^>]*>.*?</head>`)
	tagRe   = regexp.MustCompile(`(?s)<[^>]*>`)
	blockRe = regexp.MustCompile(`(?is)</(p|div|li|tr|h[1-6])>|<br\s*/?>`)
	// includes U+00A0, which &nbsp; unescapes to
	spaceRe = regexp.MustCompile("[ \t ]+")
	nlRe    = regexp.MustCompile(`\n{3,}`)
)

// HTMLToText converts Canvas announcement HTML to readable plain text.
func HTMLToText(s string) string {
	if s == "" {
		return ""
	}
	s = dropRe.ReplaceAllString(s, " ")
	s = blockRe.ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = spaceRe.ReplaceAllString(s, " ")
	s = nlRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
