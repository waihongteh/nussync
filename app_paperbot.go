package main

// App side of the paper bot's richer Telegram commands (/search, /save,
// /download, /library, /done, /start-reading).
//
// This lives in its own file rather than in app_papers.go so the bot's command
// surface can grow without touching the Papers backend. It reaches the backend
// only through existing App methods; notify.PaperExt is the interface the bot
// calls back through.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"nussync/internal/notify"
)

// paperExt implements notify.PaperExt on top of the App's Papers bindings.
type paperExt struct{ a *App }

// installPaperBotExt attaches the extended command set to the running paper
// bot. Called from App.startup after papersInit has created the bot.
func (a *App) installPaperBotExt() {
	if b := paperBot(); b != nil {
		b.SetExt(paperExt{a})
	}
}

// paperItem flattens a contract Paper into the bot's transport view.
func paperItem(p Paper) notify.PaperItem {
	return notify.PaperItem{
		ID: p.ID, Title: p.Title, Authors: p.Authors, Year: p.Year,
		Venue: p.Venue, Citations: p.CitationCount, URL: p.URL,
	}
}

func (x paperExt) SearchPapers(ctx context.Context, query string, limit int) ([]notify.PaperItem, error) {
	res, err := x.a.SearchPapers(query, "all", limit)
	if err != nil {
		return nil, err
	}
	out := make([]notify.PaperItem, 0, len(res.Papers))
	for _, p := range res.Papers {
		out = append(out, paperItem(p))
	}
	return out, nil
}

func (x paperExt) DigestItems(ctx context.Context) ([]notify.PaperItem, error) {
	d, err := x.a.buildDigest(ctx, todayLocal())
	if err != nil {
		return nil, err
	}
	out := make([]notify.PaperItem, 0, len(d.Papers))
	for _, p := range d.Papers {
		out = append(out, paperItem(toPaper(p)))
	}
	return out, nil
}

func (x paperExt) SavePaper(ctx context.Context, id string) (string, error) {
	if err := x.a.papersInit(); err != nil {
		return "", err
	}
	lp, err := x.a.ensureLibrary(id)
	if err != nil {
		return "", err
	}
	return lp.Title, nil
}

func (x paperExt) DownloadPaperPDF(ctx context.Context, id string) (string, error) {
	lp, err := x.a.DownloadPaperPDF(id)
	if err != nil {
		return "", err
	}
	if lp.LocalPath == "" {
		return "", errors.New("the PDF could not be saved")
	}
	return filepath.Base(lp.LocalPath), nil
}

func (x paperExt) LibraryList(ctx context.Context, status string, limit int) ([]notify.PaperItem, error) {
	rows, err := x.a.GetLibrary(status)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]notify.PaperItem, 0, len(rows))
	for _, r := range rows {
		it := paperItem(r.Paper)
		it.Status, it.Page, it.Pages = r.Status, r.Page, r.Pages
		out = append(out, it)
	}
	return out, nil
}

func (x paperExt) SetPaperStatus(ctx context.Context, id, status string) (string, error) {
	if err := x.a.papersInit(); err != nil {
		return "", err
	}
	row, ok, err := x.a.st.LibraryPaperByID(id)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("%s is not in your library — /save it first", strings.TrimSpace(id))
	}
	lp := toLibraryPaper(row)
	lp.Status = status
	saved, err := x.a.UpdateLibraryPaper(lp)
	if err != nil {
		return "", err
	}
	return saved.Title, nil
}
