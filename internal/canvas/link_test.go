package canvas

import "testing"

func TestNextLink(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no next", `<https://c.nus.edu.sg/api/v1/courses?page=1>; rel="current"`, ""},
		{
			"canvas typical",
			`<https://c.nus.edu.sg/api/v1/courses?page=1&per_page=10>; rel="current",` +
				`<https://c.nus.edu.sg/api/v1/courses?page=2&per_page=10>; rel="next",` +
				`<https://c.nus.edu.sg/api/v1/courses?page=1&per_page=10>; rel="first",` +
				`<https://c.nus.edu.sg/api/v1/courses?page=5&per_page=10>; rel="last"`,
			"https://c.nus.edu.sg/api/v1/courses?page=2&per_page=10",
		},
		{
			"next first",
			`<https://x/api?page=2>; rel="next", <https://x/api?page=1>; rel="current"`,
			"https://x/api?page=2",
		},
		{
			"unquoted rel",
			`<https://x/api?page=3>; rel=next`,
			"https://x/api?page=3",
		},
		{
			"extra whitespace",
			`  <https://x/api?page=3>  ;  rel = "next"  `,
			"https://x/api?page=3",
		},
		{
			"comma inside url brackets",
			`<https://x/api?ids[]=1,2,3&page=2>; rel="next"`,
			"https://x/api?ids[]=1,2,3&page=2",
		},
		{
			"bookmark style cursor",
			`<https://x/api/v1/courses?page=bookmark:WyIxIl0&per_page=100>; rel="next"`,
			"https://x/api/v1/courses?page=bookmark:WyIxIl0&per_page=100",
		},
		{"malformed missing brackets", `https://x/api?page=2; rel="next"`, ""},
		{"only semicolonless", `<https://x/api?page=2>`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NextLink(tc.in); got != tc.want {
				t.Errorf("NextLink(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFolderPath(t *testing.T) {
	root := 1
	sub := 2
	folders := []Folder{
		{ID: 1, Name: "course files", FullName: "course files"},
		{ID: 2, Name: "Lectures", FullName: "course files/Lectures", ParentFolderID: &root},
		{ID: 3, Name: "Week 1", FullName: "course files/Lectures/Week 1", ParentFolderID: &sub},
	}
	got := FolderPath(folders)
	want := map[int]string{1: "", 2: "Lectures", 3: "Lectures/Week 1"}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("FolderPath[%d] = %q, want %q", id, got[id], w)
		}
	}
}

func TestFolderPathFallsBackToParentLinks(t *testing.T) {
	root := 1
	folders := []Folder{
		{ID: 1, Name: "course files"},
		{ID: 2, Name: "Slides", ParentFolderID: &root},
	}
	got := FolderPath(folders)
	if got[2] != "Slides" {
		t.Errorf("FolderPath[2] = %q, want %q", got[2], "Slides")
	}
}

func TestHostOf(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://canvas.nus.edu.sg", "canvas.nus.edu.sg"},
		{"https://canvas.nus.edu.sg/api/v1/files/1", "canvas.nus.edu.sg"},
		{"http://localhost:3000/x?y=1", "localhost:3000"},
		{"https://s3.amazonaws.com/bucket/key?sig=abc", "s3.amazonaws.com"},
	}
	for _, tc := range tests {
		if got := hostOf(tc.in); got != tc.want {
			t.Errorf("hostOf(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestAssignmentKindAndSubmitted(t *testing.T) {
	q := Assignment{SubmissionTypes: []string{"online_quiz"}}
	if q.Kind() != "quiz" {
		t.Errorf("Kind = %q, want quiz", q.Kind())
	}
	d := Assignment{SubmissionTypes: []string{"discussion_topic"}}
	if d.Kind() != "discussion" {
		t.Errorf("Kind = %q, want discussion", d.Kind())
	}
	a := Assignment{SubmissionTypes: []string{"online_upload"}}
	if a.Kind() != "assignment" {
		t.Errorf("Kind = %q, want assignment", a.Kind())
	}

	if a.Submitted() {
		t.Error("no submission object should not count as submitted")
	}
	a.Submission = &Submission{WorkflowState: "unsubmitted"}
	if a.Submitted() {
		t.Error("unsubmitted workflow_state should not count as submitted")
	}
	a.Submission = &Submission{WorkflowState: "graded"}
	if !a.Submitted() {
		t.Error("graded should count as submitted")
	}
}
