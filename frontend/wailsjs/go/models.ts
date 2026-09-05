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
	export class Grade {
	    CourseCode: string;
	    Title: string;
	    Score: number;
	    Possible: number;
	    GradedAt: string;
	    URL: string;
	
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
	    LaunchAtLogin: boolean;
	    Theme: string;
	
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
	        this.LaunchAtLogin = source["LaunchAtLogin"];
	        this.Theme = source["Theme"];
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

