// Package sync downloads Canvas course content into the local library.
package sync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"nussync/internal/canvas"
	"nussync/internal/config"
	"nussync/internal/index"
	"nussync/internal/store"
)

// Source values for a discovered file.
const (
	SourceFiles   = "files"
	SourceModules = "modules"
)

// Progress is reported to the caller during a run.
type Progress struct {
	Phase           string // "listing" | "downloading" | "indexing" | "idle" | "error"
	Course          string
	Done            int
	Total           int
	CurrentFile     string
	BytesDownloaded int64
	Err             string
}

// Result summarises a completed run.
type Result struct {
	FilesDownloaded int
	FilesSkipped    int
	BytesDownloaded int64
	CoursesSynced   int
	Errors          []string

	// NewAnnouncementIDs are announcements first seen during this run. The UI
	// toasts on these; the Telegram scheduler has its own `notified` flag, so
	// an unpaired bot must not make every sync re-announce the whole backlog.
	NewAnnouncementIDs []int
}

// Engine performs one sync at a time.
type Engine struct {
	Client     *canvas.Client
	Store      *store.Store
	Settings   config.Settings
	OnProgress func(Progress)

	// Workers is the download concurrency (default 4).
	Workers int

	// newAnns collects announcement IDs inserted by this run. Only written
	// from the sequential metadata loop.
	newAnns []int
}

// New builds an engine.
func New(c *canvas.Client, st *store.Store, s config.Settings, onProgress func(Progress)) *Engine {
	return &Engine{Client: c, Store: st, Settings: s, OnProgress: onProgress, Workers: 4}
}

func (e *Engine) report(p Progress) {
	if e.OnProgress != nil {
		e.OnProgress(p)
	}
}

// candidate is a file discovered from either source, before de-duplication.
type candidate struct {
	File       canvas.File
	Source     string
	FolderPath string
	Module     string
}

// Run performs a full sync: refresh courses, then per enabled course list and
// download files, then refresh deadlines/announcements/grades, then index.
func (e *Engine) Run(ctx context.Context) (Result, error) {
	var res Result

	e.report(Progress{Phase: "listing", Course: "Courses"})
	courses, err := e.Client.ActiveCourses(ctx)
	if err != nil {
		return res, fmt.Errorf("list courses: %w", err)
	}

	known := map[int]bool{}
	existing, _ := e.Store.Courses()
	for _, c := range existing {
		known[c.ID] = true
	}
	for _, c := range courses {
		code := CourseCode(c)
		term := ""
		if c.Term != nil {
			term = c.Term.Name
		}
		row := store.Course{ID: c.ID, Code: code, Name: c.Name, Term: term, Enabled: true}
		if known[c.ID] {
			row.Enabled = true // preserved by UpsertCourse's DO UPDATE
		}
		if err := e.Store.UpsertCourse(row); err != nil {
			return res, err
		}
	}

	// Course codes were just refreshed; move any library still on the legacy
	// "every cross-listed code joined by _" folder naming before listing files,
	// so nothing is re-downloaded.
	if _, err := MigrateCourseFolders(e.Store, e.Settings.SyncDir); err != nil {
		res.Errors = append(res.Errors, "migrate folders: "+err.Error())
	}

	enabled, err := e.Store.EnabledCourses()
	if err != nil {
		return res, err
	}
	active := map[int]bool{}
	for _, c := range courses {
		active[c.ID] = true
	}

	for _, c := range enabled {
		if !active[c.ID] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if err := e.syncCourse(ctx, c, &res); err != nil {
			if errors.Is(err, context.Canceled) {
				return res, err
			}
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", c.Code, err))
			continue
		}
		res.CoursesSynced++
		_ = e.Store.TouchCourseSynced(c.ID, time.Now())
	}

	// Metadata refresh.
	e.newAnns = nil
	for _, c := range enabled {
		if !active[c.ID] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return res, err
		}
		e.report(Progress{Phase: "listing", Course: c.Code, CurrentFile: "deadlines & grades"})
		if err := e.refreshCourseMeta(ctx, c); err != nil && !errors.Is(err, context.Canceled) {
			res.Errors = append(res.Errors, fmt.Sprintf("%s meta: %v", c.Code, err))
		}
	}

	res.NewAnnouncementIDs = e.newAnns

	e.report(Progress{Phase: "indexing", BytesDownloaded: res.BytesDownloaded})
	if err := e.indexNew(ctx); err != nil && !errors.Is(err, context.Canceled) {
		res.Errors = append(res.Errors, fmt.Sprintf("index: %v", err))
	}

	_ = e.Store.SetKV("last_sync", time.Now().Format(time.RFC3339))
	return res, ctx.Err()
}

// syncCourse lists a single course from both sources and downloads what changed.
func (e *Engine) syncCourse(ctx context.Context, c store.Course, res *Result) error {
	e.report(Progress{Phase: "listing", Course: c.Code})

	cands := map[int]candidate{}

	// --- Files tab (folder tree) ---
	folders, err := e.Client.Folders(ctx, c.ID)
	if err != nil {
		if !canvas.IsPermission(err) && !canvas.IsNotFound(err) {
			if errors.Is(err, context.Canceled) {
				return err
			}
			res.Errors = append(res.Errors, fmt.Sprintf("%s folders: %v", c.Code, err))
		}
	} else {
		canvas.SortFolders(folders)
		paths := canvas.FolderPath(folders)
		for _, f := range folders {
			if err := ctx.Err(); err != nil {
				return err
			}
			if f.ForSubmissions {
				continue
			}
			files, err := e.Client.FolderFiles(ctx, f.ID)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return err
				}
				if !canvas.IsPermission(err) && !canvas.IsNotFound(err) {
					res.Errors = append(res.Errors, fmt.Sprintf("%s folder %d: %v", c.Code, f.ID, err))
				}
				continue
			}
			for _, file := range files {
				cands[file.ID] = candidate{File: file, Source: SourceFiles, FolderPath: paths[f.ID]}
			}
		}
	}

	// --- Modules ---
	modules, err := e.Client.Modules(ctx, c.ID)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		if !canvas.IsPermission(err) && !canvas.IsNotFound(err) {
			res.Errors = append(res.Errors, fmt.Sprintf("%s modules: %v", c.Code, err))
		}
	}
	for _, m := range modules {
		for _, it := range m.Items {
			if err := ctx.Err(); err != nil {
				return err
			}
			if it.Type != "File" || it.ContentID == 0 {
				continue
			}
			if _, dup := cands[it.ContentID]; dup {
				// Present in both: keep the Files-tab path, note the module.
				cur := cands[it.ContentID]
				if cur.Module == "" {
					cur.Module = m.Name
					cands[it.ContentID] = cur
				}
				continue
			}
			f, err := e.Client.FileByID(ctx, it.ContentID)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return err
				}
				if !canvas.IsPermission(err) && !canvas.IsNotFound(err) {
					res.Errors = append(res.Errors, fmt.Sprintf("%s file %d: %v", c.Code, it.ContentID, err))
				}
				continue
			}
			if f.DisplayName == "" && f.Filename == "" {
				f.DisplayName = it.Title
			}
			cands[f.ID] = candidate{File: f, Source: SourceModules, Module: m.Name}
		}
	}

	// --- Decide what to download ---
	type job struct {
		cand candidate
		row  store.File
	}
	var jobs []job
	ids := make([]int, 0, len(cands))
	for id := range cands {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	for _, id := range ids {
		cd := cands[id]
		name := cd.File.Name()
		if name == "" {
			continue
		}
		rel := RelPathFor(cd.Source, cd.FolderPath, cd.Module, name)
		abs := AbsPathFor(e.Settings.SyncDir, c.Code, rel)

		row := store.File{
			ID:         cd.File.ID,
			CourseID:   c.ID,
			Name:       name,
			RelPath:    rel,
			AbsPath:    abs,
			Size:       cd.File.Size,
			ModifiedAt: rfc(cd.File.Mtime()),
			UpdatedAt:  rfc(derefTime(cd.File.UpdatedAt)),
			Source:     cd.Source,
			Module:     cd.Module,
			URL:        cd.File.URL,
		}

		if e.shouldSkip(name, cd.File.Size) {
			res.FilesSkipped++
			row.Synced = false
			// Record metadata anyway so the UI can show the file exists remotely.
			if prev, ok, _ := e.Store.FileByID(row.ID); ok {
				row.ContentHash = prev.ContentHash
				row.Indexed = prev.Indexed
				row.Synced = prev.Synced && fileExists(prev.AbsPath)
			}
			_ = e.Store.UpsertFile(row)
			continue
		}

		prev, ok, err := e.Store.FileByID(row.ID)
		if err != nil {
			return err
		}
		need := true
		if ok && prev.Synced && fileExists(prev.AbsPath) &&
			prev.Size == row.Size && prev.UpdatedAt == row.UpdatedAt &&
			prev.ModifiedAt == row.ModifiedAt && prev.AbsPath == row.AbsPath {
			need = false
			row.ContentHash = prev.ContentHash
			row.Indexed = prev.Indexed
			row.Synced = true
			_ = e.Store.UpsertFile(row)
		}
		if need {
			if ok {
				row.ContentHash = ""
				row.Indexed = false
			}
			jobs = append(jobs, job{cand: cd, row: row})
		}
	}

	total := len(jobs)
	if total == 0 {
		e.report(Progress{Phase: "downloading", Course: c.Code, Done: 0, Total: 0,
			BytesDownloaded: res.BytesDownloaded})
		return nil
	}

	// --- Download with a worker pool ---
	workers := e.Workers
	if workers <= 0 {
		workers = 4
	}
	if workers > total {
		workers = total
	}

	var (
		mu    sync.Mutex
		done  int
		wg    sync.WaitGroup
		jobCh = make(chan job)
	)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobCh {
				if ctx.Err() != nil {
					return
				}
				n, hash, err := e.Client.Download(ctx, j.cand.File.URL, j.row.AbsPath)
				mu.Lock()
				done++
				if err == nil {
					j.row.Synced = true
					j.row.ContentHash = hash
					if j.row.Size == 0 {
						j.row.Size = n
					}
					res.FilesDownloaded++
					res.BytesDownloaded += n
					_ = e.Store.UpsertFile(j.row)
				} else if !errors.Is(err, context.Canceled) {
					res.Errors = append(res.Errors, fmt.Sprintf("%s %s: %v", c.Code, j.row.Name, err))
					j.row.Synced = false
					_ = e.Store.UpsertFile(j.row)
				}
				snapshot := Progress{
					Phase: "downloading", Course: c.Code, Done: done, Total: total,
					CurrentFile: j.row.Name, BytesDownloaded: res.BytesDownloaded,
				}
				mu.Unlock()
				e.report(snapshot)
			}
		}()
	}

feed:
	for _, j := range jobs {
		select {
		case <-ctx.Done():
			break feed
		case jobCh <- j:
		}
	}
	close(jobCh)
	wg.Wait()

	return ctx.Err()
}

// shouldSkip applies the MaxFileMB and SkipExts filters.
func (e *Engine) shouldSkip(name string, size int64) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, s := range e.Settings.SkipExts {
		if s != "" && strings.EqualFold(s, ext) {
			return true
		}
	}
	if e.Settings.MaxFileMB > 0 && size > int64(e.Settings.MaxFileMB)*1024*1024 {
		return true
	}
	return false
}

// indexNew extracts text from newly downloaded files.
func (e *Engine) indexNew(ctx context.Context) error {
	files, err := e.Store.UnindexedFiles()
	if err != nil {
		return err
	}
	for i, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !fileExists(f.AbsPath) {
			_ = e.Store.MarkUnsynced(f.ID)
			continue
		}
		e.report(Progress{Phase: "indexing", Done: i + 1, Total: len(files), CurrentFile: f.Name})
		text := ""
		if index.Supported(f.AbsPath) {
			text = index.Extract(f.AbsPath)
		}
		if err := e.Store.MarkIndexed(f.ID, f.Name+" "+strings.ReplaceAll(f.RelPath, "/", " "), text); err != nil {
			return err
		}
	}
	return nil
}

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func rfc(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// CourseCode derives a short course code, preferring Canvas's course_code but
// trimming the NUS section suffix (e.g. "CS4246 [2610]" -> "CS4246").
//
// Cross-listed modules keep every code, separated by "/", exactly as Canvas
// reports them ("CS4246/CS5446"). That is the display/DB form; the on-disk
// folder name comes from CourseFolder, which keeps only the first segment.
func CourseCode(c canvas.Course) string {
	code := strings.TrimSpace(c.CourseCode)
	if code == "" {
		code = strings.TrimSpace(c.Name)
	}
	if code == "" {
		return fmt.Sprintf("course-%d", c.ID)
	}
	// Drop a trailing bracketed section/term marker.
	if i := strings.IndexAny(code, "["); i > 0 {
		code = strings.TrimSpace(code[:i])
	}
	if f := strings.Fields(code); len(f) > 0 {
		// Canvas NUS codes look like "CS4246" or "NST2030 Sustainability".
		if len(f[0]) >= 4 && hasDigit(f[0]) {
			code = f[0]
		}
	}
	// Sanitize per segment so "/" survives as the cross-listing separator.
	parts := strings.Split(code, "/")
	for i, p := range parts {
		parts[i] = SanitizeSegment(p)
	}
	return strings.Join(parts, "/")
}

func hasDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}
