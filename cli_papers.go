package main

// Headless test harness for the Papers backend:
//
//	nussync --papers-test "<query>" [--digest-send]
//
// It runs the real arXiv and Semantic Scholar APIs through the same code paths
// the GUI uses: search each source, build today's digest, download one small
// arXiv PDF into <SyncDir>/Papers/ and confirm the files row + page count.
// With --digest-send it also pushes the digest through the paper bot (which
// requires the bot to be paired).

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"nussync/internal/papers"
	"nussync/internal/store"
	"nussync/internal/study"
)

// runPapersCLI is the entry point for --papers-test. Hook it into cli.go's
// switch with:  case "--papers-test": return runPapersCLI(os.Args[2:])
func runPapersCLI(args []string) int {
	query := ""
	send := false
	claude := false
	for _, a := range args {
		switch {
		case a == "--digest-send":
			send = true
		case a == "--claude-test":
			claude = true
		case strings.HasPrefix(a, "--"):
			// ignore unknown flags
		case query == "":
			query = a
		}
	}
	if query == "" {
		query = "LLM unlearning"
	}

	app := NewApp()
	if err := app.Init(true); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer app.shutdown(context.Background())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	app.ctx = ctx

	if err := app.papersInit(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	cfg := app.settings()
	fmt.Printf("Query: %q\nSync dir: %s\nKeywords: %s\nCategories: %s\nDigest hour: %d\n\n",
		query, cfg.SyncDir, strings.Join(cfg.PaperKeywords, ", "),
		strings.Join(cfg.PaperCategories, ", "), cfg.PaperDigestHour)

	// ---------------------------------------------------------------- arXiv
	start := time.Now()
	ax, axTotal, err := papersRT.arxiv.Search(ctx, query, 5, papers.SortRelevance)
	if err != nil {
		fmt.Fprintln(os.Stderr, "arxiv: error:", err)
	} else {
		fmt.Printf("arXiv: %d hits (showing %d) in %s\n", axTotal, len(ax),
			time.Since(start).Round(time.Millisecond))
		printPapers(ax)
	}

	// ------------------------------------------------------- Semantic Scholar
	start = time.Now()
	s2, s2Total, err := papersRT.s2.Search(ctx, query, 5)
	if err != nil {
		fmt.Printf("\nSemantic Scholar: unavailable (%v) — non-fatal, arXiv results stand\n", err)
	} else {
		fmt.Printf("\nSemantic Scholar: %d hits (showing %d) in %s\n", s2Total, len(s2),
			time.Since(start).Round(time.Millisecond))
		printPapers(s2)
	}

	merged := papers.Merge(ax, s2)
	fmt.Printf("\nMerged: %d unique papers (deduped by arXiv id)\n", len(merged))

	// ------------------------------------------------------ venue ranking
	prefs := venuePrefs()
	fmt.Printf("\nRanking: prefer published = %v, %d top venues\n",
		prefs.PreferPublished, len(prefs.TopVenues))
	res, rerr := app.SearchPapersFiltered(query, "all", 20, false)
	if rerr != nil {
		fmt.Fprintln(os.Stderr, "ranked search: error:", rerr)
	} else {
		if res.Note != "" {
			fmt.Println("note:", res.Note)
		}
		fmt.Println("Top 5 by Score:")
		for i, p := range res.Papers {
			if i >= 5 {
				break
			}
			fmt.Printf("  %d. [tier %d · %s] %s\n     %d cites · %s · %s\n",
				i+1, p.VenueTier, p.VenueShort, truncate(p.Title, 66),
				p.CitationCount, p.Source, p.URL)
		}
		if pub, perr := app.SearchPapersFiltered(query, "all", 20, true); perr == nil {
			fmt.Printf("Published only: %d of %d results survive the filter\n",
				len(pub.Papers), len(res.Papers))
		}
	}

	// --------------------------------------------------------------- digest
	fmt.Println("\nBuilding today's digest…")
	d, err := app.GetPaperDigest("")
	if err != nil {
		fmt.Fprintln(os.Stderr, "digest: error:", err)
	} else {
		fmt.Printf("Digest %s: %d papers\n", d.Date, len(d.Papers))
		for i, p := range d.Papers {
			reason := ""
			if i < len(d.Reason) {
				reason = d.Reason[i]
			}
			fmt.Printf("  %d. %s (%s)\n", i+1, truncate(p.Title, 70), reason)
		}
		msg := papers.DigestMessage(papers.Digest{Date: d.Date,
			Papers: digestPapers(d), Reason: d.Reason})
		fmt.Printf("Telegram body: %d bytes (limit %d)\n", len(msg), papers.MaxDigestMessage)
	}

	// ------------------------------------------------------------- download
	var target Paper
	for _, p := range merged {
		if p.Source == "arxiv" && p.PDFURL != "" {
			target = toPaper(p)
			break
		}
	}
	if target.ID == "" {
		fmt.Println("\nNo arXiv PDF to download.")
	} else {
		fmt.Printf("\nDownloading %s\n  %s\n", target.ID, target.Title)
		if _, err := app.AddPaperToLibrary(target); err != nil {
			fmt.Fprintln(os.Stderr, "library: error:", err)
		}
		lp, err := app.DownloadPaperPDF(target.ID)
		if err != nil {
			fmt.Fprintln(os.Stderr, "download: error:", err)
		} else {
			fmt.Printf("  -> %s\n  file id %d, %d pages\n", lp.LocalPath, lp.FileID, lp.Pages)
			if f, ok, ferr := app.st.FileByID(lp.FileID); ferr == nil && ok {
				fmt.Printf("  files row: course %d (%s), rel %q, %s, synced=%v, indexed=%v\n",
					f.CourseID, courseCodeOf(app, f.CourseID), f.RelPath,
					humanBytes(f.Size), f.Synced, f.Indexed)
			} else {
				fmt.Println("  files row: MISSING")
			}
			fmt.Printf("  study.PageCount: %d\n", study.PageCount(lp.LocalPath))
		}
	}

	// ------------------------------------------------------------- bibtex
	if len(merged) > 0 {
		bib, err := app.ExportBibTeX([]string{merged[0].ID})
		if err == nil {
			fmt.Println("\nBibTeX for the first result:")
			fmt.Println(bib)
		}
	}

	// ------------------------------------------------- summary + chat (Claude)
	if claude && target.ID != "" {
		runClaudeChecks(ctx, app, target.ID)
	}

	// --------------------------------------------------------- digest send
	if send {
		st := app.GetPaperTelegramStatus()
		if !st.Configured {
			fmt.Printf("\n--digest-send: the paper bot %s is NOT paired (no chat id). "+
				"Send /start to it, then click Pair in Settings ▸ Papers.\n", st.BotName)
			return 1
		}
		if err := app.SendPaperDigestNow(); err != nil {
			fmt.Fprintln(os.Stderr, "digest send: error:", err)
			return 1
		}
		fmt.Println("\nDigest sent to the paper bot chat.")
	}
	return 0
}

// runClaudeChecks exercises StartPaperSummary and SendChat end to end. Both go
// through the Claude Code CLI, so on a machine where `claude auth login` has
// not been run they are expected to finish with the auth error in Job.Error —
// which is exactly what this check verifies surfaces cleanly.
func runClaudeChecks(ctx context.Context, app *App, paperID string) {
	fmt.Println("\nClaude: paper summary…")
	jobID, err := app.StartPaperSummary(paperID, "sonnet")
	if err != nil {
		fmt.Println("  StartPaperSummary:", err)
	} else {
		printJobOutcome(ctx, app, jobID)
	}

	fmt.Println("\nClaude: chat…")
	s, err := app.StartChat(0, paperID, "sonnet")
	if err != nil {
		fmt.Println("  StartChat:", err)
		return
	}
	fmt.Printf("  session %s (%s)\n", s.ID, s.Title)
	jobID, err = app.SendChat(s.ID, "In one sentence: what does this paper unlearn?")
	if err != nil {
		fmt.Println("  SendChat:", err)
		return
	}
	printJobOutcome(ctx, app, jobID)
	msgs, _ := app.GetChatMessages(s.ID)
	fmt.Printf("  %d stored messages\n", len(msgs))
}

// printJobOutcome polls a study job to completion and prints its result.
func printJobOutcome(ctx context.Context, app *App, jobID string) {
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		j, err := app.GetStudyJob(jobID)
		if err != nil {
			fmt.Println("  job:", err)
			return
		}
		switch j.Status {
		case "done":
			fmt.Printf("  job %s: done in %s (cost $%.4f)\n", j.Kind, j.FinishedAt, j.CostUSD)
			return
		case "error", "cancelled":
			fmt.Printf("  job %s: %s — %s\n", j.Kind, j.Status, j.Error)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
	fmt.Println("  job: timed out")
}

func digestPapers(d PaperDigest) []papers.Paper {
	out := make([]papers.Paper, 0, len(d.Papers))
	for _, p := range d.Papers {
		out = append(out, fromPaper(p))
	}
	return out
}

func courseCodeOf(app *App, id int) string {
	if id == store.PapersCourseID {
		return "Papers"
	}
	cs, err := app.st.Courses()
	if err != nil {
		return ""
	}
	for _, c := range cs {
		if c.ID == id {
			return c.Code
		}
	}
	return ""
}

func printPapers(ps []papers.Paper) {
	for i, p := range ps {
		authors := strings.Join(p.Authors, ", ")
		if len(p.Authors) > 3 {
			authors = strings.Join(p.Authors[:3], ", ") + " et al."
		}
		fmt.Printf("  %d. [%s] %s\n     %s\n     %d %s · %d citations · %s\n",
			i+1, p.ID, truncate(p.Title, 70), truncate(authors, 70),
			p.Year, p.Venue, p.CitationCount, p.URL)
		if p.Comment != "" {
			fmt.Printf("     comment: %s\n", truncate(p.Comment, 90))
		}
		if p.TLDR != "" {
			fmt.Printf("     TLDR: %s\n", truncate(p.TLDR, 100))
		}
	}
}
