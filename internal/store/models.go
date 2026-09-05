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
	Source      string // "files" | "modules" | "pages"
	Module      string
	Origin      string // where a "pages" file was linked from (page URL etc.)
	Synced      bool
	ContentHash string
	Indexed     bool
	URL         string

	// FirstSeenAt is stamped once, when the file is first downloaded.
	// LastChangedAt is re-stamped on every re-download caused by a change.
	// Both RFC3339, "" for rows predating the what's-new feed.
	FirstSeenAt   string
	LastChangedAt string
}

// FeedFile is one row of the what's-new feed: a file joined to its course code.
type FeedFile struct {
	File       File
	CourseCode string
	ChangedAt  string // RFC3339, last_changed_at
	New        bool   // first_seen_at == last_changed_at
}

// ScoreStats is Canvas score_statistics for one assignment.
type ScoreStats struct {
	Mean   float64
	Min    float64
	Max    float64
	Median float64
	Count  int
}

// AssignmentDetail is the cached lazy detail of one assignment.
type AssignmentDetail struct {
	AssignmentID    int
	CourseID        int
	Description     string
	SubmissionTypes []string
	Score           float64
	Graded          bool
	Stats           *ScoreStats
	FetchedAt       string // RFC3339
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
	// Mean is the class mean from assignment_cache, 0 when not cached.
	Mean float64
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
