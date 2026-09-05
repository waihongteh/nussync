package canvas

import "time"

// User is the /users/self payload subset we need.
type User struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
	LoginID   string `json:"login_id"`
}

// Term is a Canvas enrollment term.
type Term struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Course is an active course enrollment.
type Course struct {
	ID                     int    `json:"id"`
	Name                   string `json:"name"`
	CourseCode             string `json:"course_code"`
	Term                   *Term  `json:"term"`
	AccessRestrictedByDate bool   `json:"access_restricted_by_date"`
}

// Folder is a node in the Files tab tree.
type Folder struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	FullName       string `json:"full_name"`
	ParentFolderID *int   `json:"parent_folder_id"`
	FilesCount     int    `json:"files_count"`
	Hidden         bool   `json:"hidden"`
	ForSubmissions bool   `json:"for_submissions"`
}

// File is a Canvas file object.
type File struct {
	ID            int        `json:"id"`
	FolderID      int        `json:"folder_id"`
	DisplayName   string     `json:"display_name"`
	Filename      string     `json:"filename"`
	ContentType   string     `json:"content-type"`
	URL           string     `json:"url"`
	Size          int64      `json:"size"`
	UpdatedAt     *time.Time `json:"updated_at"`
	ModifiedAt    *time.Time `json:"modified_at"`
	Locked        bool       `json:"locked"`
	Hidden        bool       `json:"hidden"`
	LockedForUser bool       `json:"locked_for_user"`
}

// Mtime returns the best available modification timestamp.
func (f File) Mtime() time.Time {
	if f.ModifiedAt != nil && !f.ModifiedAt.IsZero() {
		return *f.ModifiedAt
	}
	if f.UpdatedAt != nil {
		return *f.UpdatedAt
	}
	return time.Time{}
}

// Name returns the friendliest available file name.
func (f File) Name() string {
	if f.DisplayName != "" {
		return f.DisplayName
	}
	return f.Filename
}

// ModuleItem is one entry inside a module.
type ModuleItem struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	ContentID int    `json:"content_id"`
	HTMLURL   string `json:"html_url"`
	URL       string `json:"url"`
}

// Module is a course module with (optionally inlined) items.
type Module struct {
	ID    int          `json:"id"`
	Name  string       `json:"name"`
	Items []ModuleItem `json:"items"`
}

// Submission is the submission sub-object on an assignment.
type Submission struct {
	ID            int        `json:"id"`
	AssignmentID  int        `json:"assignment_id"`
	Score         *float64   `json:"score"`
	Grade         *string    `json:"grade"`
	GradedAt      *time.Time `json:"graded_at"`
	SubmittedAt   *time.Time `json:"submitted_at"`
	WorkflowState string     `json:"workflow_state"`
	Attempt       int        `json:"attempt"`
	Missing       bool       `json:"missing"`
}

// Assignment covers assignments and quizzes (submission_types online_quiz).
type Assignment struct {
	ID                      int         `json:"id"`
	CourseID                int         `json:"course_id"`
	Name                    string      `json:"name"`
	DueAt                   *time.Time  `json:"due_at"`
	LockAt                  *time.Time  `json:"lock_at"`
	HTMLURL                 string      `json:"html_url"`
	PointsPossible          float64     `json:"points_possible"`
	SubmissionTypes         []string    `json:"submission_types"`
	HasSubmittedSubmissions bool        `json:"has_submitted_submissions"`
	Submission              *Submission `json:"submission"`
	Published               bool        `json:"published"`
	OmitFromFinalGrade      bool        `json:"omit_from_final_grade"`
}

// Kind maps an assignment to the contract's Deadline.Type.
func (a Assignment) Kind() string {
	for _, t := range a.SubmissionTypes {
		switch t {
		case "online_quiz":
			return "quiz"
		case "discussion_topic":
			return "discussion"
		}
	}
	return "assignment"
}

// Submitted reports whether the current user has turned this in.
func (a Assignment) Submitted() bool {
	if a.Submission != nil {
		if a.Submission.SubmittedAt != nil {
			return true
		}
		switch a.Submission.WorkflowState {
		case "submitted", "graded", "pending_review", "complete":
			return true
		}
		return false
	}
	return false
}

// DiscussionTopic is used for announcements (only_announcements=true).
type DiscussionTopic struct {
	ID        int        `json:"id"`
	Title     string     `json:"title"`
	Message   string     `json:"message"`
	PostedAt  *time.Time `json:"posted_at"`
	CreatedAt *time.Time `json:"created_at"`
	HTMLURL   string     `json:"html_url"`
	URL       string     `json:"url"`
	Author    struct {
		DisplayName string `json:"display_name"`
	} `json:"author"`
}

// Posted returns the best posting timestamp.
func (d DiscussionTopic) Posted() time.Time {
	if d.PostedAt != nil {
		return *d.PostedAt
	}
	if d.CreatedAt != nil {
		return *d.CreatedAt
	}
	return time.Time{}
}
