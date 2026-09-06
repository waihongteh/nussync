package main

// PDF highlights. The in-app PDF.js viewer (frontend PdfViewer.svelte) draws
// them; this file is only storage plus the markdown export. See
// docs/API_CONTRACT.md.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"nussync/internal/store"
)

// highlightColors is the palette the viewer offers; anything else is coerced
// to yellow so a stray value cannot render as an invisible swatch.
var highlightColors = map[string]bool{"yellow": true, "green": true, "blue": true, "pink": true}

var highlightsInit sync.Once
var highlightsErr error

// highlightsReady creates the table on first use. Not memoised on failure —
// the store may simply not be open yet — except for the happy path.
func (a *App) highlightsReady() error {
	if a.st == nil {
		return errors.New("not initialised")
	}
	highlightsInit.Do(func() { highlightsErr = a.st.MigrateHighlights() })
	if highlightsErr != nil {
		highlightsInit = sync.Once{}
		return highlightsErr
	}
	return nil
}

// toHighlight maps a store row to the contract type.
func toHighlight(r store.Highlight) Highlight {
	rects := []Rect{}
	if strings.TrimSpace(r.Rects) != "" {
		_ = json.Unmarshal([]byte(r.Rects), &rects)
	}
	if rects == nil {
		rects = []Rect{}
	}
	return Highlight{
		ID:        r.ID,
		FileID:    r.FileID,
		Page:      r.Page,
		Rects:     rects,
		Text:      r.Text,
		Color:     r.Color,
		Note:      r.Note,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

// GetHighlights returns every highlight on a file, ordered by page.
func (a *App) GetHighlights(fileID int) ([]Highlight, error) {
	if err := a.highlightsReady(); err != nil {
		return nil, err
	}
	rows, err := a.st.HighlightsForFile(fileID)
	if err != nil {
		return nil, err
	}
	out := make([]Highlight, 0, len(rows))
	for _, r := range rows {
		out = append(out, toHighlight(r))
	}
	return out, nil
}

// SaveHighlight inserts (ID 0) or updates an existing highlight and returns the
// stored row. Emits `highlights:updated`.
func (a *App) SaveHighlight(h Highlight) (Highlight, error) {
	if err := a.highlightsReady(); err != nil {
		return Highlight{}, err
	}
	if h.FileID == 0 {
		return Highlight{}, errors.New("highlight needs a file id")
	}
	if h.Page < 1 {
		h.Page = 1
	}
	if !highlightColors[h.Color] {
		h.Color = "yellow"
	}
	if len(h.Rects) == 0 {
		return Highlight{}, errors.New("highlight has no rectangles")
	}
	blob, err := json.Marshal(h.Rects)
	if err != nil {
		return Highlight{}, err
	}
	saved, err := a.st.PutHighlight(store.Highlight{
		ID: h.ID, FileID: h.FileID, Page: h.Page, Rects: string(blob),
		Text: h.Text, Color: h.Color, Note: h.Note, CreatedAt: h.CreatedAt,
	})
	if err != nil {
		return Highlight{}, err
	}
	a.emit("highlights:updated", map[string]any{"FileID": h.FileID})
	return toHighlight(saved), nil
}

// DeleteHighlight removes one highlight. Emits `highlights:updated`.
func (a *App) DeleteHighlight(id int) error {
	if err := a.highlightsReady(); err != nil {
		return err
	}
	fileID := 0
	if row, ok, err := a.st.HighlightByID(id); err == nil && ok {
		fileID = row.FileID
	}
	if err := a.st.DeleteHighlight(id); err != nil {
		return err
	}
	a.emit("highlights:updated", map[string]any{"FileID": fileID})
	return nil
}

// ExportHighlights renders a file's highlights as markdown, grouped by page:
// a block quote of the highlighted text plus the note underneath.
func (a *App) ExportHighlights(fileID int) (string, error) {
	hs, err := a.GetHighlights(fileID)
	if err != nil {
		return "", err
	}
	title := fmt.Sprintf("file %d", fileID)
	if a.st != nil {
		if f, ok, err := a.st.FileByID(fileID); err == nil && ok {
			title = f.Name
		}
	}

	sort.SliceStable(hs, func(i, j int) bool { return hs[i].Page < hs[j].Page })

	var b strings.Builder
	fmt.Fprintf(&b, "# Highlights — %s\n\n", title)
	if len(hs) == 0 {
		b.WriteString("_No highlights yet._\n")
		return b.String(), nil
	}
	page := -1
	for _, h := range hs {
		if h.Page != page {
			page = h.Page
			fmt.Fprintf(&b, "## Page %d\n\n", page)
		}
		for _, line := range strings.Split(strings.TrimSpace(collapseWS(h.Text)), "\n") {
			fmt.Fprintf(&b, "> %s\n", line)
		}
		b.WriteString("\n")
		if note := strings.TrimSpace(h.Note); note != "" {
			fmt.Fprintf(&b, "%s\n\n", note)
		}
	}
	return b.String(), nil
}

// collapseWS squashes the runs of whitespace a PDF text layer produces so a
// quoted line does not come out with a dozen spaces mid-sentence.
func collapseWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// SaveTextFile offers a native Save-As dialog and writes content there.
// Returns the chosen path, or "" when the user cancelled.
func (a *App) SaveTextFile(name, content string) (string, error) {
	if a.headless || a.ctx == nil {
		return "", errors.New("no GUI available")
	}
	path, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
		Title:                "Save as",
		DefaultFilename:      filepath.Base(strings.TrimSpace(name)),
		DefaultDirectory:     a.settings().SyncDir,
		CanCreateDirectories: true,
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
