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
  LaunchAtLogin: boolean;
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
  | 'toast';

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
