package main

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"nussync/internal/store"
)

// The markdown export has to keep the two kinds apart: a highlight is a block
// quote plus its note, a text box is a single "Note (p. N)" line.
func TestExportHighlightsKinds(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	a := NewApp()
	a.st = st
	a.headless = true
	highlightsInit = sync.Once{}
	highlightsErr = nil

	rects := []Rect{{X: 0.1, Y: 0.2, W: 0.3, H: 0.04}}
	if _, err := a.SaveHighlight(Highlight{FileID: 5, Page: 1, Rects: rects,
		Text: "entropy   is  expected surprise", Color: "green", Note: "learn this"}); err != nil {
		t.Fatalf("save highlight: %v", err)
	}
	if _, err := a.SaveHighlight(Highlight{FileID: 5, Page: 2, Rects: rects,
		Text: "ask about the discount factor", Color: "yellow", Kind: "note"}); err != nil {
		t.Fatalf("save note: %v", err)
	}
	// An empty box still exports, so the reader knows it is there.
	if _, err := a.SaveHighlight(Highlight{FileID: 5, Page: 2, Rects: rects, Kind: "note"}); err != nil {
		t.Fatalf("save empty note: %v", err)
	}

	md, err := a.ExportHighlights(5)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	for _, want := range []string{
		"## Page 1",
		"> entropy is expected surprise",
		"learn this",
		"## Page 2",
		"\U0001F4DD Note (p. 2): ask about the discount factor",
		"\U0001F4DD Note (p. 2): (empty)",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("export missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "> ask about the discount factor") {
		t.Errorf("a text box must not be quoted like a highlight:\n%s", md)
	}
}

// An unknown kind (or one from an older frontend) reads back as a highlight.
func TestSaveHighlightNormalisesKind(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	a := NewApp()
	a.st = st
	a.headless = true
	highlightsInit = sync.Once{}
	highlightsErr = nil

	rects := []Rect{{X: 0, Y: 0, W: 0.2, H: 0.1}}
	for _, in := range []string{"", "scribble"} {
		got, err := a.SaveHighlight(Highlight{FileID: 1, Page: 1, Rects: rects, Kind: in})
		if err != nil {
			t.Fatalf("save %q: %v", in, err)
		}
		if got.Kind != "highlight" {
			t.Errorf("kind %q became %q, want highlight", in, got.Kind)
		}
	}
	got, err := a.SaveHighlight(Highlight{FileID: 1, Page: 1, Rects: rects, Kind: "note"})
	if err != nil || got.Kind != "note" {
		t.Errorf("note kind not kept: %+v %v", got, err)
	}
}
