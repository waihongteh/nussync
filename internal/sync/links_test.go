package sync

import (
	"reflect"
	"testing"
)

func TestFileIDsInHTML(t *testing.T) {
	cases := []struct {
		name string
		html string
		want []int
	}{
		{"empty", "", nil},
		{"no links", `<p>Read chapter 3 before the lecture.</p>`, nil},
		{
			"canvas anchor with wrap and data-api-endpoint",
			`<p><a class="instructure_file_link inline_disabled" ` +
				`href="https://canvas.nus.edu.sg/courses/97040/files/1234567?wrap=1" ` +
				`target="_blank" rel="noopener" ` +
				`data-api-endpoint="https://canvas.nus.edu.sg/api/v1/courses/97040/files/1234567" ` +
				`data-api-returntype="File">Course Overview</a></p>`,
			[]int{1234567},
		},
		{
			"download link",
			`<a href="/courses/97040/files/2222/download?download_frd=1">Introduction</a>`,
			[]int{2222},
		},
		{
			"embedded image preview",
			`<img src="https://canvas.nus.edu.sg/courses/97040/files/333/preview" alt="plan">`,
			[]int{333},
		},
		{
			"bare file url and iframe",
			`<iframe src="/files/44/download"></iframe><a href="/files/55">x</a>`,
			[]int{44, 55},
		},
		{
			"dedup across forms, first-appearance order",
			`<a href="/courses/97040/files/900?wrap=1" ` +
				`data-api-endpoint="/api/v1/courses/97040/files/900">A</a>` +
				`<a href="/courses/97040/files/800/download?download_frd=1">B</a>` +
				`<a href="/files/900/preview">A again</a>`,
			[]int{900, 800},
		},
		{
			"cross-course link is kept",
			`<a href="https://canvas.nus.edu.sg/courses/11111/files/77?wrap=1">other course</a>`,
			[]int{77},
		},
		{
			"escaped slashes and encoded ampersand",
			`{"body":"<a href=\"\/courses\/97040\/files\/61?wrap=1&amp;x=2\">Planning<\/a>"}`,
			[]int{61},
		},
		{
			"folder listing url has no numeric id",
			`<a href="/courses/97040/files/folder/Lectures">Lectures</a>`,
			nil,
		},
		{
			"realistic week page",
			`<h2>Week 1</h2><ul>
<li><a class="instructure_file_link" href="https://canvas.nus.edu.sg/courses/97040/files/4980121?wrap=1" data-api-endpoint="https://canvas.nus.edu.sg/api/v1/courses/97040/files/4980121">Course Overview</a></li>
<li><a class="instructure_file_link" href="https://canvas.nus.edu.sg/courses/97040/files/4980122/download?download_frd=1">Introduction</a></li>
<li><a href="/courses/97040/files/4980123?wrap=1">Classical (Symbolic) Planning</a></li>
</ul>`,
			[]int{4980121, 4980122, 4980123},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FileIDsInHTML(tc.html)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("FileIDsInHTML() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRelPathForPage(t *testing.T) {
	cases := []struct{ dir, name, want string }{
		{"Modules/Week 1: Intro/Overview", "notes.pdf", "Modules/Week 1_ Intro/Overview/notes.pdf"},
		{"Pages/Syllabus", "a/b.pdf", "Pages/Syllabus/a_b.pdf"},
		{"", "x.pdf", "x.pdf"},
		{"Announcements/../etc", "x.pdf", "Announcements/etc/x.pdf"},
	}
	for _, tc := range cases {
		if got := RelPathForPage(tc.dir, tc.name); got != tc.want {
			t.Errorf("RelPathForPage(%q,%q) = %q, want %q", tc.dir, tc.name, got, tc.want)
		}
	}
}
