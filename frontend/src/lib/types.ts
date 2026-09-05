/**
 * TypeScript mirrors of the Go types in docs/API_CONTRACT.md.
 * JSON field names are the Go field names exactly (no json tags) => PascalCase.
 */

export interface Course {
  ID: number;
  Code: string;
  Name: string;
  Term: string;
  FileCount: number;
  /** RFC3339 or "" */
  LastSynced: string;
  Enabled: boolean;
  /** hex, deterministic from ID */
  Color: string;
}

export interface FileNode {
  /** canvas file id, 0 for folder */
  ID: number;
  CourseID: number;
  Name: string;
  /** absolute local path */
  Path: string;
  RelPath: string;
  IsDir: boolean;
  Size: number;
  /** RFC3339 */
  ModifiedAt: string;
  /** "files" | "modules" */
  Source: string;
  /** module title or "" */
  Module: string;
  /** downloaded locally */
  Synced: boolean;
  /** changed after the feed was last marked seen — badge it */
  IsNew: boolean;
  /** for dirs */
  Children: FileNode[] | null;
}

export interface SearchHit {
  File: FileNode;
  CourseCode: string;
  Snippet: string;
  Score: number;
}

export type DeadlineType = 'assignment' | 'quiz' | 'discussion';

export interface Deadline {
  ID: number;
  CourseID: number;
  CourseCode: string;
  Title: string;
  Type: DeadlineType | string;
  /** RFC3339 */
  DueAt: string;
  Submitted: boolean;
  URL: string;
  PointsPossible: number;
  /** raw Canvas HTML — the frontend MUST sanitize it. Filled by GetDeadlineDetail. */
  Description: string;
  /** e.g. ["online_upload"] */
  SubmissionTypes: string[] | null;
  Attachments: FileNode[] | null;
  /** the user's score, 0 when ungraded */
  Score: number;
  Graded: boolean;
  /** null unless graded AND Canvas discloses score statistics */
  Stats: ScoreStats | null;
}

export interface ScoreStats {
  Mean: number;
  Min: number;
  Max: number;
  Median: number;
  /** 0 on Canvas builds that do not report it — "unknown", not "nobody" */
  Count: number;
}

export interface Announcement {
  ID: number;
  CourseCode: string;
  Title: string;
  PostedAt: string;
  HTML: string;
  /** plain */
  Text: string;
  URL: string;
  Read: boolean;
}

export interface Grade {
  CourseCode: string;
  Title: string;
  Score: number;
  Possible: number;
  GradedAt: string;
  URL: string;
  /** class mean, 0 when never cached by GetDeadlineDetail */
  Mean: number;
}

export interface Settings {
  CanvasURL: string;
  CanvasToken: string;
  SyncDir: string;
  TelegramToken: string;
  TelegramChatID: string;
  /** Go durations e.g. ["72h","48h","24h","3h","1h"] */
  ReminderLadder: string[];
  /** skip larger; 0 = no limit */
  MaxFileMB: number;
  /** e.g. [".mp4"] */
  SkipExts: string[];
  SyncIntervalMin: number;
  NotifyAnnouncements: boolean;
  NotifyGrades: boolean;
  /** Windows toast after a sync brings new files; default true */
  NotifyDesktop: boolean;
  LaunchAtLogin: boolean;
  /** global show/hide, default "ctrl+shift+n"; "" disables */
  Hotkey: string;
  /** "system" | "light" | "dark" */
  Theme: 'system' | 'light' | 'dark' | string;
}

export type SyncPhase = 'idle' | 'listing' | 'downloading' | 'indexing' | 'error';

export interface SyncStatus {
  Running: boolean;
  Phase: SyncPhase | string;
  Course: string;
  Done: number;
  Total: number;
  CurrentFile: string;
  LastRun: string;
  LastError: string;
  BytesDownloaded: number;
}

export interface TelegramStatus {
  Configured: boolean;
  ChatID: string;
  BotName: string;
}

export interface Stats {
  Files: number;
  Bytes: number;
  Courses: number;
  Deadlines: number;
  LastSync: string;
}

// -------------------------------------------------------------- what's new

export interface FeedItem {
  /** canvas file id */
  ID: number;
  CourseID: number;
  /** full code, e.g. "CS4246/CS5446" */
  CourseCode: string;
  Name: string;
  /** absolute local path — pass to OpenFile / RevealFile */
  Path: string;
  RelPath: string;
  Size: number;
  /** RFC3339 */
  ChangedAt: string;
  Kind: 'new' | 'updated' | string;
  Module: string;
}

// ------------------------------------------------------------------- study

export interface StudyStatus {
  CLIFound: boolean;
  Version: string;
  LoggedIn: boolean;
  Error: string;
  /** ["opus","sonnet","haiku"] */
  Models: string[] | null;
}

export type StudyKind = 'overview' | 'quiz' | 'ask' | 'flashcards';
export type StudyJobStatus = 'queued' | 'running' | 'done' | 'error' | 'cancelled';

export interface StudyJob {
  ID: string;
  Kind: StudyKind | string;
  FileIDs: number[] | null;
  Status: StudyJobStatus | string;
  Progress: string;
  Error: string;
  /** RFC3339 */
  StartedAt: string;
  /** RFC3339 */
  FinishedAt: string;
  Model: string;
  CostUSD: number;
}

export interface Overview {
  FileID: number;
  Markdown: string;
  CreatedAt: string;
  Model: string;
}

export interface Question {
  ID: number;
  Type: 'mcq' | 'short' | string;
  Prompt: string;
  Options: string[] | null;
  /** mcq: option letter A-D; short: the model answer */
  Answer: string;
  Explanation: string;
  Page: number;
}

export interface Quiz {
  ID: number;
  FileIDs: number[] | null;
  Title: string;
  CreatedAt: string;
  Model: string;
  Questions: Question[] | null;
}

export interface QuizAttempt {
  QuizID: number;
  /** question ID -> answer */
  Answers: Record<number, string>;
  Score: number;
  Total: number;
  TakenAt: string;
}

export interface Flashcard {
  ID: number;
  FileID: number;
  Front: string;
  Back: string;
  /** RFC3339 */
  Due: string;
  /** days */
  Interval: number;
  Ease: number;
}

export interface AskResult {
  Question: string;
  /** markdown */
  Answer: string;
  Citations: string[] | null;
  CreatedAt: string;
}

/** `study:progress` payload */
export interface StudyProgress {
  JobID: string;
  Text: string;
}

export interface ToastPayload {
  /** info | success | error */
  Level: 'info' | 'success' | 'error' | string;
  Message: string;
}

/** Event names emitted by the Go backend. */
export type AppEvent =
  | 'sync:status'
  | 'sync:done'
  | 'deadlines:updated'
  | 'announcements:new'
  | 'toast'
  | 'feed:updated'
  | 'study:job'
  | 'study:progress';

/** Typed shape of the bound Go App methods. */
export interface AppAPI {
  GetCourses(): Promise<Course[]>;
  SetCourseEnabled(id: number, enabled: boolean): Promise<void>;
  GetTree(courseID: number): Promise<FileNode[]>;
  GetRecentFiles(limit: number): Promise<FileNode[]>;
  Search(query: string, courseID: number): Promise<SearchHit[]>;
  OpenFile(path: string): Promise<void>;
  RevealFile(path: string): Promise<void>;
  OpenURL(url: string): Promise<void>;
  SyncNow(): Promise<void>;
  CancelSync(): Promise<void>;
  GetSyncStatus(): Promise<SyncStatus>;
  GetDeadlines(): Promise<Deadline[]>;
  GetAnnouncements(limit: number): Promise<Announcement[]>;
  MarkAnnouncementRead(id: number): Promise<void>;
  GetGrades(): Promise<Grade[]>;
  GetSettings(): Promise<Settings>;
  SaveSettings(s: Settings): Promise<void>;
  TestCanvas(): Promise<string>;
  GetTelegramStatus(): Promise<TelegramStatus>;
  PairTelegram(): Promise<string>;
  SendTestTelegram(): Promise<void>;
  ChooseSyncDir(): Promise<string>;
  GetStats(): Promise<Stats>;

  // ---------------------------------------------------------- what's new
  GetWhatsNew(sinceDays: number): Promise<FeedItem[]>;
  MarkFeedSeen(): Promise<void>;
  GetUnseenCount(): Promise<number>;

  // ------------------------------------------------------ deadline detail
  GetDeadlineDetail(id: number): Promise<Deadline>;

  // ------------------------------------------------------ window control
  ShowWindow(): Promise<void>;
  HideWindow(): Promise<void>;
  ToggleWindow(): Promise<void>;

  // ------------------------------------------------------------- study
  GetStudyStatus(): Promise<StudyStatus>;
  StartOverview(fileID: number, model: string): Promise<string>;
  GetOverview(fileID: number): Promise<Overview>;
  StartQuiz(fileIDs: number[], model: string, n: number): Promise<string>;
  GetQuizzes(fileID: number): Promise<Quiz[]>;
  GetQuiz(id: number): Promise<Quiz>;
  SubmitQuizAttempt(a: QuizAttempt): Promise<QuizAttempt>;
  GetQuizAttempts(quizID: number): Promise<QuizAttempt[]>;
  StartAsk(fileIDs: number[], question: string, model: string): Promise<string>;
  GetAsks(fileID: number): Promise<AskResult[]>;
  StartFlashcards(fileID: number, model: string, n: number): Promise<string>;
  GetDueFlashcards(limit: number): Promise<Flashcard[]>;
  ReviewFlashcard(id: number, grade: number): Promise<void>;
  GetStudyJob(id: string): Promise<StudyJob>;
  GetStudyJobs(): Promise<StudyJob[]>;
  CancelStudyJob(id: string): Promise<void>;
  GetFilePageCount(fileID: number): Promise<number>;
}

// ------------------------------------------------------------------ UI-only

/** A single entry in the right-click context menu. */
export interface MenuItem {
  label: string;
  icon?: string;
  disabled?: boolean;
  danger?: boolean;
  run: () => void;
}
