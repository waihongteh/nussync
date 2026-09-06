package main

// Contract types shared with the frontend. JSON field names are the Go field
// names exactly (no json tags) — do not rename. See docs/API_CONTRACT.md.

// Course is an enrolled Canvas course.
type Course struct {
	ID         int
	Code       string
	Name       string
	Term       string
	FileCount  int
	LastSynced string // RFC3339 or ""
	Enabled    bool
	Color      string // hex, deterministic from ID
}

// FileNode is a file or directory in the local library tree.
type FileNode struct {
	ID         int // canvas file id, 0 for folder
	CourseID   int
	Name       string
	Path       string // absolute local path
	RelPath    string
	IsDir      bool
	Size       int64
	ModifiedAt string // RFC3339
	Source     string // "files" | "modules" | "pages"
	Module     string // module title or ""
	Synced     bool   // downloaded locally
	IsNew      bool   // changed after the feed was last marked seen
	Children   []FileNode
}

// FeedItem is one entry of the what's-new feed.
type FeedItem struct {
	ID         int // canvas file id
	CourseID   int
	CourseCode string
	Name       string
	Path       string // absolute local path
	RelPath    string
	Size       int64
	ChangedAt  string // RFC3339
	Kind       string // "new" | "updated"
	Module     string
}

// ScoreStats is the class-wide score summary of a graded assignment.
type ScoreStats struct {
	Mean   float64
	Min    float64
	Max    float64
	Median float64
	Count  int
}

// SearchHit is one full-text search result.
type SearchHit struct {
	File       FileNode
	CourseCode string
	Snippet    string
	Score      float64
}

// Deadline is an upcoming or overdue assignment/quiz/discussion.
type Deadline struct {
	ID             int
	CourseID       int
	CourseCode     string
	Title          string
	Type           string // "assignment" | "quiz" | "discussion"
	DueAt          string // RFC3339
	Submitted      bool
	URL            string
	PointsPossible float64

	// Only filled by GetDeadlineDetail; zero-valued from GetDeadlines.
	Description     string      // raw Canvas HTML — the frontend sanitizes it
	SubmissionTypes []string    // e.g. ["online_upload"]
	Attachments     []FileNode  // files linked in Description that we synced
	Score           float64     // the user's score, 0 when ungraded
	Graded          bool        //
	Stats           *ScoreStats // nil when Canvas discloses no statistics
}

// Announcement is a course announcement.
type Announcement struct {
	ID         int
	CourseCode string
	Title      string
	PostedAt   string
	HTML       string
	Text       string // plain
	URL        string
	Read       bool
}

// Grade is a graded submission.
type Grade struct {
	CourseCode string
	Title      string
	Score      float64
	Possible   float64
	GradedAt   string
	URL        string
	Mean       float64 // class mean if cached, else 0
}

// Settings is the user-editable configuration.
type Settings struct {
	CanvasURL      string
	CanvasToken    string
	SyncDir        string
	TelegramToken  string
	TelegramChatID string

	ReminderLadder []string // Go durations e.g. ["72h","48h","24h","3h","1h"]
	MaxFileMB      int      // skip larger; 0 = no limit
	SkipExts       []string // [".mp4"]

	SyncIntervalMin     int
	NotifyAnnouncements bool
	NotifyGrades        bool
	NotifyDesktop       bool // Windows toast after a sync brings new files
	LaunchAtLogin       bool
	Theme               string // "system" | "light" | "dark"
	Hotkey              string // global show/hide, e.g. "ctrl+shift+n"; "" disables

	// Papers (docs/CONTRACT_PAPERS.md). The paper tracker runs on a SECOND
	// Telegram bot with its own token, chat id and pairing.
	PaperTelegramToken  string
	PaperTelegramChatID string
	PaperKeywords       []string // digest keyword filter
	PaperCategories     []string // arXiv categories, e.g. ["cs.CL","cs.LG"]
	PaperDigestHour     int      // 0-23 local time, default 9
	NotifyPapers        bool
	// PaperTopVenues are the venues that count as tier 2 when ranking search
	// results, the digest and recommendations; PaperPreferPublished turns
	// that venue weight on (default true).
	PaperTopVenues       []string
	PaperPreferPublished bool
}

// SyncStatus is the live state of the sync engine.
type SyncStatus struct {
	Running         bool
	Phase           string // "idle" | "listing" | "downloading" | "indexing" | "error"
	Course          string
	Done            int
	Total           int
	CurrentFile     string
	LastRun         string
	LastError       string
	BytesDownloaded int64
}

// TelegramStatus describes the bot pairing state.
type TelegramStatus struct {
	Configured bool
	ChatID     string
	BotName    string
}

// Stats summarises the local library.
type Stats struct {
	Files     int
	Bytes     int64
	Courses   int
	Deadlines int
	LastSync  string
}

// Toast is the payload of the `toast` event.
type Toast struct {
	Level   string // "info" | "success" | "error"
	Message string
}

// Rect is one highlighted box, normalised 0..1 against the PDF page it sits on
// (origin top-left), so it survives any zoom level.
type Rect struct {
	X float64
	Y float64
	W float64
	H float64
}

// Highlight is one saved PDF highlight. A selection that spans pages is stored
// as one Highlight per page.
type Highlight struct {
	ID        int
	FileID    int
	Page      int // 1-based
	Rects     []Rect
	Text      string
	Color     string // "yellow" | "green" | "blue" | "pink"
	Note      string
	CreatedAt string
	UpdatedAt string
}
