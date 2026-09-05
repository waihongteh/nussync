package sync

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeSegment(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "Lecture 1.pdf", "Lecture 1.pdf"},
		{"slash", "Week 1/2 notes.pdf", "Week 1_2 notes.pdf"},
		{"backslash", `a\b.txt`, "a_b.txt"},
		{"colon", "Topic: Intro.pdf", "Topic_ Intro.pdf"},
		{"quotes", `He said "hi".txt`, "He said _hi_.txt"},
		{"pipe-question-star", "a|b?c*d", "a_b_c_d"},
		{"angle brackets", "<tag>.md", "_tag_.md"},
		{"trailing dot", "notes...", "notes"},
		{"trailing space", "notes  ", "notes"},
		{"trailing dot and space", "notes. . ", "notes"},
		{"leading space trimmed", "  notes.pdf", "notes.pdf"},
		{"control chars", "a\tb\nc", "a_b_c"},
		{"empty", "", "_"},
		{"only dots", "...", "_"},
		{"reserved CON", "CON", "_CON"},
		{"reserved with ext", "nul.txt", "_nul.txt"},
		{"reserved lowercase com1", "com1", "_com1"},
		{"not reserved", "CONTROL.txt", "CONTROL.txt"},
		{"unicode kept", "分析 résumé.pdf", "分析 résumé.pdf"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeSegment(tc.in); got != tc.want {
				t.Errorf("SanitizeSegment(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeSegmentTruncates(t *testing.T) {
	long := strings.Repeat("a", 300) + ".pdf"
	got := SanitizeSegment(long)
	if len(got) > 120 {
		t.Fatalf("length %d exceeds 120", len(got))
	}
	if !strings.HasSuffix(got, ".pdf") {
		t.Fatalf("extension lost: %q", got)
	}
}

func TestSanitizeRel(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"Lectures", "Lectures"},
		{"Lectures/Week 1", "Lectures/Week 1"},
		{"/Lectures//Week 1/", "Lectures/Week 1"},
		{"Lectures/../etc", "Lectures/etc"},
		{"./Lectures", "Lectures"},
		{`Lectures\Week 1`, "Lectures/Week 1"},
		{"Topic: A/B", "Topic_ A/B"},
		{"trailing.  /x", "trailing/x"},
	}
	for _, tc := range tests {
		if got := SanitizeRel(tc.in); got != tc.want {
			t.Errorf("SanitizeRel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRelPathFor(t *testing.T) {
	tests := []struct {
		name                          string
		source, folder, module, fname string
		want                          string
	}{
		{"files root", SourceFiles, "", "", "syllabus.pdf", "syllabus.pdf"},
		{"files nested", SourceFiles, "Lectures/Week 1", "", "L1.pdf", "Lectures/Week 1/L1.pdf"},
		{"files ignores module", SourceFiles, "Lectures", "Week 1", "L1.pdf", "Lectures/L1.pdf"},
		{"module only", SourceModules, "", "Week 1: Intro", "L1.pdf", "Modules/Week 1_ Intro/L1.pdf"},
		{"module empty title", SourceModules, "", "", "L1.pdf", "Modules/L1.pdf"},
		{"module dirty name", SourceModules, "", "A/B", "x?.pdf", "Modules/A_B/x_.pdf"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RelPathFor(tc.source, tc.folder, tc.module, tc.fname)
			if got != tc.want {
				t.Errorf("RelPathFor(%q,%q,%q,%q) = %q, want %q",
					tc.source, tc.folder, tc.module, tc.fname, got, tc.want)
			}
		})
	}
}

func TestAbsPathFor(t *testing.T) {
	got := AbsPathFor(filepath.Join("C:", "Users", "x", "NUSSync"), "CS4246", "Lectures/Week 1/L1.pdf")
	want := filepath.Join("C:", "Users", "x", "NUSSync", "CS4246", "Lectures", "Week 1", "L1.pdf")
	if got != want {
		t.Errorf("AbsPathFor = %q, want %q", got, want)
	}
}

func TestCourseFolder(t *testing.T) {
	cases := []struct{ code, folder, legacy string }{
		{"MA3236", "MA3236", "MA3236"},
		{"CS4246/CS5446", "CS4246", "CS4246_CS5446"},
		{"TR3202S/TR3202T/ETP3201S/ETP3201T/ETP3206L/ETP3201I", "TR3202S",
			"TR3202S_TR3202T_ETP3201S_ETP3201T_ETP3206L_ETP3201I"},
		{"NOC_AY2425ST_AY2526S1", "NOC_AY2425ST_AY2526S1", "NOC_AY2425ST_AY2526S1"},
	}
	for _, c := range cases {
		if got := CourseFolder(c.code); got != c.folder {
			t.Errorf("CourseFolder(%q) = %q, want %q", c.code, got, c.folder)
		}
		if got := LegacyCourseFolder(c.code); got != c.legacy {
			t.Errorf("LegacyCourseFolder(%q) = %q, want %q", c.code, got, c.legacy)
		}
	}
}
