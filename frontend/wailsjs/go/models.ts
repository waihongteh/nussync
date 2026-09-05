export namespace main {
	
	export class Announcement {
	    ID: number;
	    CourseCode: string;
	    Title: string;
	    PostedAt: string;
	    HTML: string;
	    Text: string;
	    URL: string;
	    Read: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Announcement(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.CourseCode = source["CourseCode"];
	        this.Title = source["Title"];
	        this.PostedAt = source["PostedAt"];
	        this.HTML = source["HTML"];
	        this.Text = source["Text"];
	        this.URL = source["URL"];
	        this.Read = source["Read"];
	    }
	}
	export class AskResult {
	    Question: string;
	    Answer: string;
	    Citations: string[];
	    CreatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new AskResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Question = source["Question"];
	        this.Answer = source["Answer"];
	        this.Citations = source["Citations"];
	        this.CreatedAt = source["CreatedAt"];
	    }
	}
	export class Course {
	    ID: number;
	    Code: string;
	    Name: string;
	    Term: string;
	    FileCount: number;
	    LastSynced: string;
	    Enabled: boolean;
	    Color: string;
	
	    static createFrom(source: any = {}) {
	        return new Course(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Code = source["Code"];
	        this.Name = source["Name"];
	        this.Term = source["Term"];
	        this.FileCount = source["FileCount"];
	        this.LastSynced = source["LastSynced"];
	        this.Enabled = source["Enabled"];
	        this.Color = source["Color"];
	    }
	}
	export class ScoreStats {
	    Mean: number;
	    Min: number;
	    Max: number;
	    Median: number;
	    Count: number;
	
	    static createFrom(source: any = {}) {
	        return new ScoreStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Mean = source["Mean"];
	        this.Min = source["Min"];
	        this.Max = source["Max"];
	        this.Median = source["Median"];
	        this.Count = source["Count"];
	    }
	}
	export class FileNode {
	    ID: number;
	    CourseID: number;
	    Name: string;
	    Path: string;
	    RelPath: string;
	    IsDir: boolean;
	    Size: number;
	    ModifiedAt: string;
	    Source: string;
	    Module: string;
	    Synced: boolean;
	    IsNew: boolean;
	    Children: FileNode[];
	
	    static createFrom(source: any = {}) {
	        return new FileNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.CourseID = source["CourseID"];
	        this.Name = source["Name"];
	        this.Path = source["Path"];
	        this.RelPath = source["RelPath"];
	        this.IsDir = source["IsDir"];
	        this.Size = source["Size"];
	        this.ModifiedAt = source["ModifiedAt"];
	        this.Source = source["Source"];
	        this.Module = source["Module"];
	        this.Synced = source["Synced"];
	        this.IsNew = source["IsNew"];
	        this.Children = this.convertValues(source["Children"], FileNode);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Deadline {
	    ID: number;
	    CourseID: number;
	    CourseCode: string;
	    Title: string;
	    Type: string;
	    DueAt: string;
	    Submitted: boolean;
	    URL: string;
	    PointsPossible: number;
	    Description: string;
	    SubmissionTypes: string[];
	    Attachments: FileNode[];
	    Score: number;
	    Graded: boolean;
	    Stats?: ScoreStats;
	
	    static createFrom(source: any = {}) {
	        return new Deadline(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.CourseID = source["CourseID"];
	        this.CourseCode = source["CourseCode"];
	        this.Title = source["Title"];
	        this.Type = source["Type"];
	        this.DueAt = source["DueAt"];
	        this.Submitted = source["Submitted"];
	        this.URL = source["URL"];
	        this.PointsPossible = source["PointsPossible"];
	        this.Description = source["Description"];
	        this.SubmissionTypes = source["SubmissionTypes"];
	        this.Attachments = this.convertValues(source["Attachments"], FileNode);
	        this.Score = source["Score"];
	        this.Graded = source["Graded"];
	        this.Stats = this.convertValues(source["Stats"], ScoreStats);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FeedItem {
	    ID: number;
	    CourseID: number;
	    CourseCode: string;
	    Name: string;
	    Path: string;
	    RelPath: string;
	    Size: number;
	    ChangedAt: string;
	    Kind: string;
	    Module: string;
	
	    static createFrom(source: any = {}) {
	        return new FeedItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.CourseID = source["CourseID"];
	        this.CourseCode = source["CourseCode"];
	        this.Name = source["Name"];
	        this.Path = source["Path"];
	        this.RelPath = source["RelPath"];
	        this.Size = source["Size"];
	        this.ChangedAt = source["ChangedAt"];
	        this.Kind = source["Kind"];
	        this.Module = source["Module"];
	    }
	}
	
	export class Flashcard {
	    ID: number;
	    FileID: number;
	    Front: string;
	    Back: string;
	    Due: string;
	    Interval: number;
	    Ease: number;
	
	    static createFrom(source: any = {}) {
	        return new Flashcard(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.FileID = source["FileID"];
	        this.Front = source["Front"];
	        this.Back = source["Back"];
	        this.Due = source["Due"];
	        this.Interval = source["Interval"];
	        this.Ease = source["Ease"];
	    }
	}
	export class Grade {
	    CourseCode: string;
	    Title: string;
	    Score: number;
	    Possible: number;
	    GradedAt: string;
	    URL: string;
	    Mean: number;
	
	    static createFrom(source: any = {}) {
	        return new Grade(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.CourseCode = source["CourseCode"];
	        this.Title = source["Title"];
	        this.Score = source["Score"];
	        this.Possible = source["Possible"];
	        this.GradedAt = source["GradedAt"];
	        this.URL = source["URL"];
	        this.Mean = source["Mean"];
	    }
	}
	export class Overview {
	    FileID: number;
	    Markdown: string;
	    CreatedAt: string;
	    Model: string;
	
	    static createFrom(source: any = {}) {
	        return new Overview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.FileID = source["FileID"];
	        this.Markdown = source["Markdown"];
	        this.CreatedAt = source["CreatedAt"];
	        this.Model = source["Model"];
	    }
	}
	export class Question {
	    ID: number;
	    Type: string;
	    Prompt: string;
	    Options: string[];
	    Answer: string;
	    Explanation: string;
	    Page: number;
	
	    static createFrom(source: any = {}) {
	        return new Question(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Type = source["Type"];
	        this.Prompt = source["Prompt"];
	        this.Options = source["Options"];
	        this.Answer = source["Answer"];
	        this.Explanation = source["Explanation"];
	        this.Page = source["Page"];
	    }
	}
	export class Quiz {
	    ID: number;
	    FileIDs: number[];
	    Title: string;
	    CreatedAt: string;
	    Model: string;
	    Questions: Question[];
	
	    static createFrom(source: any = {}) {
	        return new Quiz(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.FileIDs = source["FileIDs"];
	        this.Title = source["Title"];
	        this.CreatedAt = source["CreatedAt"];
	        this.Model = source["Model"];
	        this.Questions = this.convertValues(source["Questions"], Question);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class QuizAttempt {
	    QuizID: number;
	    Answers: Record<number, string>;
	    Score: number;
	    Total: number;
	    TakenAt: string;
	
	    static createFrom(source: any = {}) {
	        return new QuizAttempt(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.QuizID = source["QuizID"];
	        this.Answers = source["Answers"];
	        this.Score = source["Score"];
	        this.Total = source["Total"];
	        this.TakenAt = source["TakenAt"];
	    }
	}
	
	export class SearchHit {
	    File: FileNode;
	    CourseCode: string;
	    Snippet: string;
	    Score: number;
	
	    static createFrom(source: any = {}) {
	        return new SearchHit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.File = this.convertValues(source["File"], FileNode);
	        this.CourseCode = source["CourseCode"];
	        this.Snippet = source["Snippet"];
	        this.Score = source["Score"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Settings {
	    CanvasURL: string;
	    CanvasToken: string;
	    SyncDir: string;
	    TelegramToken: string;
	    TelegramChatID: string;
	    ReminderLadder: string[];
	    MaxFileMB: number;
	    SkipExts: string[];
	    SyncIntervalMin: number;
	    NotifyAnnouncements: boolean;
	    NotifyGrades: boolean;
	    NotifyDesktop: boolean;
	    LaunchAtLogin: boolean;
	    Theme: string;
	    Hotkey: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.CanvasURL = source["CanvasURL"];
	        this.CanvasToken = source["CanvasToken"];
	        this.SyncDir = source["SyncDir"];
	        this.TelegramToken = source["TelegramToken"];
	        this.TelegramChatID = source["TelegramChatID"];
	        this.ReminderLadder = source["ReminderLadder"];
	        this.MaxFileMB = source["MaxFileMB"];
	        this.SkipExts = source["SkipExts"];
	        this.SyncIntervalMin = source["SyncIntervalMin"];
	        this.NotifyAnnouncements = source["NotifyAnnouncements"];
	        this.NotifyGrades = source["NotifyGrades"];
	        this.NotifyDesktop = source["NotifyDesktop"];
	        this.LaunchAtLogin = source["LaunchAtLogin"];
	        this.Theme = source["Theme"];
	        this.Hotkey = source["Hotkey"];
	    }
	}
	export class Stats {
	    Files: number;
	    Bytes: number;
	    Courses: number;
	    Deadlines: number;
	    LastSync: string;
	
	    static createFrom(source: any = {}) {
	        return new Stats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Files = source["Files"];
	        this.Bytes = source["Bytes"];
	        this.Courses = source["Courses"];
	        this.Deadlines = source["Deadlines"];
	        this.LastSync = source["LastSync"];
	    }
	}
	export class StudyJob {
	    ID: string;
	    Kind: string;
	    FileIDs: number[];
	    Status: string;
	    Progress: string;
	    Error: string;
	    StartedAt: string;
	    FinishedAt: string;
	    Model: string;
	    CostUSD: number;
	
	    static createFrom(source: any = {}) {
	        return new StudyJob(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Kind = source["Kind"];
	        this.FileIDs = source["FileIDs"];
	        this.Status = source["Status"];
	        this.Progress = source["Progress"];
	        this.Error = source["Error"];
	        this.StartedAt = source["StartedAt"];
	        this.FinishedAt = source["FinishedAt"];
	        this.Model = source["Model"];
	        this.CostUSD = source["CostUSD"];
	    }
	}
	export class StudyStatus {
	    CLIFound: boolean;
	    Version: string;
	    LoggedIn: boolean;
	    Error: string;
	    Models: string[];
	
	    static createFrom(source: any = {}) {
	        return new StudyStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.CLIFound = source["CLIFound"];
	        this.Version = source["Version"];
	        this.LoggedIn = source["LoggedIn"];
	        this.Error = source["Error"];
	        this.Models = source["Models"];
	    }
	}
	export class SyncStatus {
	    Running: boolean;
	    Phase: string;
	    Course: string;
	    Done: number;
	    Total: number;
	    CurrentFile: string;
	    LastRun: string;
	    LastError: string;
	    BytesDownloaded: number;
	
	    static createFrom(source: any = {}) {
	        return new SyncStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Running = source["Running"];
	        this.Phase = source["Phase"];
	        this.Course = source["Course"];
	        this.Done = source["Done"];
	        this.Total = source["Total"];
	        this.CurrentFile = source["CurrentFile"];
	        this.LastRun = source["LastRun"];
	        this.LastError = source["LastError"];
	        this.BytesDownloaded = source["BytesDownloaded"];
	    }
	}
	export class TelegramStatus {
	    Configured: boolean;
	    ChatID: string;
	    BotName: string;
	
	    static createFrom(source: any = {}) {
	        return new TelegramStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Configured = source["Configured"];
	        this.ChatID = source["ChatID"];
	        this.BotName = source["BotName"];
	    }
	}

}

