package store

// Course is a persisted course row.
type Course struct {
	ID         int
	Code       string
	Name       string
	Term       string
	Enabled    bool
	LastSynced string
	FileCount  int
}

// File is a persisted file row (one Canvas file id).
type File struct {
	ID          int
	CourseID    int
	Name        string
	RelPath     string // course-relative, forward slashes
	AbsPath     string
	Size        int64
	ModifiedAt  string // RFC3339
	UpdatedAt   string // RFC3339
	Source      string // "files" | "modules"
	Module      string
	Synced      bool
	ContentHash string
	Indexed     bool
	URL         string
}

// Deadline is a persisted assignment/quiz deadline.
type Deadline struct {
	ID             int
	CourseID       int
	CourseCode     string
	Title          string
	Type           string
	DueAt          string
	Submitted      bool
	URL            string
	PointsPossible float64
}

// Announcement is a persisted course announcement.
type Announcement struct {
	ID         int
	CourseID   int
	CourseCode string
	Title      string
	PostedAt   string
	HTML       string
	Text       string
	URL        string
	Read       bool
	Notified   bool
}

// Grade is a persisted graded submission.
type Grade struct {
	AssignmentID int
	CourseID     int
	CourseCode   string
	Title        string
	Score        float64
	Possible     float64
	GradedAt     string
	URL          string
	Notified     bool
}

// Hit is one search result row.
type Hit struct {
	File       File
	CourseCode string
	Snippet    string
	Score      float64
}

// Stats summarises the local library.
type Stats struct {
	Files     int
	Bytes     int64
	Courses   int
	Deadlines int
	LastSync  string
}
