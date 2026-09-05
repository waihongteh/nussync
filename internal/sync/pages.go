package sync

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"nussync/internal/canvas"
	"nussync/internal/store"
)

// htmlSource is one rich-text body to be scanned for Canvas file links.
type htmlSource struct {
	Label string // FileNode.Module value, e.g. "Week 1 / Overview"
	Dir   string // course-relative destination directory
	IDs   []int  // file ids extracted from the body
	Orig  string // human-readable origin, stored on the file row
}

// collectFromHTML finds files that are only linked from rich text — module
// Pages, standalone wiki pages, assignment descriptions and announcement
// bodies — and adds them to cands as Source "pages". Ids already discovered by
// the Files tab or Modules keep their existing candidate (priority
// files > modules > pages).
func (e *Engine) collectFromHTML(ctx context.Context, c store.Course,
	modules []canvas.Module, cands map[int]candidate, res *Result) error {

	e.report(Progress{Phase: "listing", Course: c.Code, CurrentFile: "pages"})

	// Canvas updated_at per page slug, so unchanged pages skip the body fetch.
	stamps := map[string]string{}
	titles := map[string]string{}
	var listed []canvas.Page
	pages, err := e.Client.Pages(ctx, c.ID)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		// Pages tab disabled for the course: not an error.
		if !canvas.IsPermission(err) && !canvas.IsNotFound(err) {
			res.Errors = append(res.Errors, fmt.Sprintf("%s pages: %v", c.Code, err))
		}
	} else {
		listed = pages
		for _, p := range pages {
			stamps[p.URL] = p.Stamp()
			titles[p.URL] = p.Title
		}
	}

	var sources []htmlSource
	inModule := map[string]bool{}

	// --- Pages reachable from a module (nicest folder layout) ---
	for _, m := range modules {
		for _, it := range m.Items {
			if err := ctx.Err(); err != nil {
				return err
			}
			slug := it.PageURL
			if it.Type != "Page" || slug == "" || inModule[slug] {
				continue
			}
			inModule[slug] = true
			title := firstNonEmpty(it.Title, titles[slug], slug)
			ids, err := e.pageFileIDs(ctx, c.ID, slug, title, stamps[slug])
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return err
				}
				if !canvas.IsPermission(err) && !canvas.IsNotFound(err) {
					res.Errors = append(res.Errors, fmt.Sprintf("%s page %s: %v", c.Code, slug, err))
				}
				continue
			}
			sources = append(sources, htmlSource{
				Label: m.Name + " / " + title,
				Dir:   "Modules/" + m.Name + "/" + title,
				IDs:   ids,
				Orig:  fmt.Sprintf("page:%s", slug),
			})
		}
	}

	// --- Pages not referenced by any module ---
	for _, p := range listed {
		if err := ctx.Err(); err != nil {
			return err
		}
		if inModule[p.URL] {
			continue
		}
		title := firstNonEmpty(p.Title, p.URL)
		ids, err := e.pageFileIDs(ctx, c.ID, p.URL, title, p.Stamp())
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			if !canvas.IsPermission(err) && !canvas.IsNotFound(err) {
				res.Errors = append(res.Errors, fmt.Sprintf("%s page %s: %v", c.Code, p.URL, err))
			}
			continue
		}
		sources = append(sources, htmlSource{
			Label: "Pages / " + title,
			Dir:   "Pages/" + title,
			IDs:   ids,
			Orig:  fmt.Sprintf("page:%s", p.URL),
		})
	}

	// --- Assignment descriptions (already fetched for deadlines) ---
	assignments, err := e.assignments(ctx, c.ID)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
	} else {
		for _, a := range assignments {
			ids := FileIDsInHTML(a.Description)
			if len(ids) == 0 {
				continue
			}
			sources = append(sources, htmlSource{
				Label: "Assignments / " + a.Name,
				Dir:   "Assignments/" + a.Name,
				IDs:   ids,
				Orig:  fmt.Sprintf("assignment:%d", a.ID),
			})
		}
	}

	// --- Announcement bodies ---
	anns, err := e.announcements(ctx, c.ID)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
	} else {
		for _, a := range anns {
			ids := FileIDsInHTML(a.Message)
			if len(ids) == 0 {
				continue
			}
			sources = append(sources, htmlSource{
				Label: "Announcements / " + a.Title,
				Dir:   "Announcements/" + a.Title,
				IDs:   ids,
				Orig:  fmt.Sprintf("announcement:%d", a.ID),
			})
		}
	}

	// --- Resolve each id we do not already know ---
	for _, src := range sources {
		for _, id := range src.IDs {
			if err := ctx.Err(); err != nil {
				return err
			}
			if _, dup := cands[id]; dup {
				continue // Files tab or Modules already claimed it
			}
			if e.claimed[id] {
				continue // another course's rich text linked it first this run
			}
			// An authoritative row from a previous run (possibly on another
			// course) also wins: never demote a files/modules file to "pages".
			if prev, ok, _ := e.Store.FileByID(id); ok &&
				prev.Source != SourcePages && prev.CourseID != c.ID {
				continue
			}
			f, err := e.Client.FileByID(ctx, id)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return err
				}
				// Linked file in a course we cannot read, or deleted: skip.
				if !canvas.IsPermission(err) && !canvas.IsNotFound(err) {
					res.Errors = append(res.Errors, fmt.Sprintf("%s linked file %d: %v", c.Code, id, err))
				}
				continue
			}
			if f.ID == 0 || f.Name() == "" {
				continue
			}
			e.claimed[id] = true
			cands[f.ID] = candidate{
				File:   f,
				Source: SourcePages,
				Module: src.Label,
				Dir:    src.Dir,
				Origin: src.Orig,
			}
		}
	}
	return nil
}

// pageFileIDs returns the file ids linked from one wiki page, fetching the body
// only when Canvas reports an updated_at we have not already scanned.
func (e *Engine) pageFileIDs(ctx context.Context, courseID int, slug, title, stamp string) ([]int, error) {
	if stamp != "" {
		if pc, ok, err := e.Store.PageCacheGet(courseID, slug); err == nil && ok && pc.UpdatedAt == stamp {
			return pc.FileIDs, nil
		}
	}
	p, err := e.Client.PageBody(ctx, courseID, slug)
	if err != nil {
		return nil, err
	}
	ids := FileIDsInHTML(p.Body)
	st := p.Stamp()
	if st == "" {
		st = stamp
	}
	if st != "" {
		_ = e.Store.PageCachePut(store.PageCache{
			CourseID:  courseID,
			URL:       slug,
			Title:     firstNonEmpty(p.Title, title),
			UpdatedAt: st,
			FileIDs:   ids,
		})
	}
	return ids, nil
}

// assignments memoises Assignments per course for one run: the file crawl and
// the deadline refresh both need them.
func (e *Engine) assignments(ctx context.Context, courseID int) ([]canvas.Assignment, error) {
	if e.assnCache == nil {
		e.assnCache = map[int][]canvas.Assignment{}
		e.assnErr = map[int]error{}
	}
	if v, ok := e.assnCache[courseID]; ok {
		return v, e.assnErr[courseID]
	}
	v, err := e.Client.Assignments(ctx, courseID)
	if errors.Is(err, context.Canceled) {
		return nil, err // do not poison the cache with a cancellation
	}
	e.assnCache[courseID] = v
	e.assnErr[courseID] = err
	return v, err
}

// announcements memoises Announcements per course for one run.
func (e *Engine) announcements(ctx context.Context, courseID int) ([]canvas.DiscussionTopic, error) {
	if e.annCache == nil {
		e.annCache = map[int][]canvas.DiscussionTopic{}
		e.annErr = map[int]error{}
	}
	if v, ok := e.annCache[courseID]; ok {
		return v, e.annErr[courseID]
	}
	v, err := e.Client.Announcements(ctx, courseID)
	if errors.Is(err, context.Canceled) {
		return nil, err
	}
	e.annCache[courseID] = v
	e.annErr[courseID] = err
	return v, err
}

// resetCaches clears the per-run memo caches.
func (e *Engine) resetCaches() {
	e.assnCache = nil
	e.assnErr = nil
	e.annCache = nil
	e.annErr = nil
	e.claimed = map[int]bool{}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
