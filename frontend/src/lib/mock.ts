/**
 * Mock backend used when the app runs in a plain browser (`npm run dev`)
 * rather than inside the Wails shell. Mirrors AppAPI exactly and emits the
 * same events the Go backend does, so every view can be developed and
 * visually verified without the desktop binary.
 */

import type {
  Announcement,
  AppAPI,
  AskResult,
  Course,
  Deadline,
  FeedItem,
  FileNode,
  Flashcard,
  Grade,
  Overview,
  Question,
  Quiz,
  QuizAttempt,
  ScoreStats,
  SearchHit,
  Settings,
  Stats,
  StudyJob,
  StudyStatus,
  SyncStatus,
  TelegramStatus,
} from './types';

// ---------------------------------------------------------------- emitter

type Handler = (...args: any[]) => void;

const listeners = new Map<string, Set<Handler>>();

/** Local event bus standing in for the Wails runtime. */
export const localEmitter = {
  on(event: string, cb: Handler): () => void {
    let set = listeners.get(event);
    if (!set) {
      set = new Set();
      listeners.set(event, set);
    }
    set.add(cb);
    return () => set!.delete(cb);
  },
  emit(event: string, ...args: any[]): void {
    const set = listeners.get(event);
    if (!set) return;
    for (const cb of Array.from(set)) {
      try {
        cb(...args);
      } catch (err) {
        console.error(`[mock] listener for "${event}" threw`, err);
      }
    }
  },
};

// ---------------------------------------------------------------- helpers

const now = Date.now();
const HOUR = 3_600_000;
const DAY = 24 * HOUR;

const iso = (msFromNow: number) => new Date(now + msFromNow).toISOString();

const delay = <T>(value: T, ms = 90): Promise<T> =>
  new Promise((resolve) => setTimeout(() => resolve(value), ms));

let fileIdSeq = 1000;

// ---------------------------------------------------------------- courses

const COURSES: Course[] = [
  {
    ID: 1,
    Code: 'CS4246',
    Name: 'AI Planning and Decision Making',
    Term: 'AY25/26 Sem 1',
    FileCount: 34,
    LastSynced: iso(-42 * 60_000),
    Enabled: true,
    Color: '#5B5BD6',
  },
  {
    ID: 2,
    Code: 'MA3236',
    Name: 'Non-Linear Programming',
    Term: 'AY25/26 Sem 1',
    FileCount: 21,
    LastSynced: iso(-42 * 60_000),
    Enabled: true,
    Color: '#0E9F6E',
  },
  {
    ID: 3,
    Code: 'MA3270',
    Name: 'Mathematical Methods in Physics',
    Term: 'AY25/26 Sem 1',
    FileCount: 18,
    LastSynced: iso(-42 * 60_000),
    Enabled: true,
    Color: '#D97706',
  },
  {
    ID: 4,
    Code: 'NST2030',
    Name: 'Science of Music',
    Term: 'AY25/26 Sem 1',
    FileCount: 12,
    LastSynced: iso(-3 * HOUR),
    Enabled: true,
    Color: '#DB2777',
  },
  {
    ID: 5,
    Code: 'CP4101',
    Name: 'B.Comp. Dissertation',
    Term: 'AY25/26 Sem 1',
    FileCount: 9,
    LastSynced: iso(-2 * DAY),
    Enabled: true,
    Color: '#0891B2',
  },
  {
    ID: 6,
    Code: 'NUSC1101',
    Name: 'Ideas and Approaches',
    Term: 'AY25/26 Sem 1',
    FileCount: 7,
    LastSynced: '',
    Enabled: false,
    Color: '#7C3AED',
  },
];

const courseByID = (id: number) => COURSES.find((c) => c.ID === id);

// ---------------------------------------------------------------- file tree

interface FileSpec {
  name: string;
  size?: number;
  ageDays?: number;
  module?: string;
  source?: 'files' | 'modules';
  synced?: boolean;
  children?: FileSpec[];
}

function buildNode(spec: FileSpec, courseID: number, parentRel: string, root: string): FileNode {
  const isDir = Array.isArray(spec.children);
  const relPath = parentRel ? `${parentRel}/${spec.name}` : spec.name;
  const node: FileNode = {
    ID: isDir ? 0 : ++fileIdSeq,
    CourseID: courseID,
    Name: spec.name,
    Path: `${root}\\${relPath.replace(/\//g, '\\')}`,
    RelPath: relPath,
    IsDir: isDir,
    Size: isDir ? 0 : (spec.size ?? 480_000),
    ModifiedAt: iso(-(spec.ageDays ?? 6) * DAY),
    Source: spec.source ?? (spec.module ? 'modules' : 'files'),
    Module: spec.module ?? '',
    Synced: spec.synced !== false,
    IsNew: false,
    Children: null,
  };
  if (isDir) {
    node.Children = spec.children!.map((c) => buildNode(c, courseID, relPath, root));
    // a folder's mtime is its newest child
    const newest = node.Children.reduce((acc, c) => Math.max(acc, Date.parse(c.ModifiedAt)), 0);
    if (newest) node.ModifiedAt = new Date(newest).toISOString();
    node.Size = node.Children.reduce((acc, c) => acc + c.Size, 0);
  }
  return node;
}

const TREE_SPECS: Record<number, FileSpec[]> = {
  1: [
    {
      name: 'Lectures',
      children: [
        { name: 'L01 Intro to Sequential Decision Making.pdf', size: 2_410_000, ageDays: 41, module: 'Week 1' },
        { name: 'L02 Markov Decision Processes.pdf', size: 3_120_000, ageDays: 34, module: 'Week 2' },
        { name: 'L03 Value and Policy Iteration.pptx', size: 5_940_000, ageDays: 27, module: 'Week 3' },
        { name: 'L04 Monte Carlo and TD Learning.pdf', size: 2_880_000, ageDays: 20, module: 'Week 4' },
        { name: 'L05 Deep Q-Networks.pptx', size: 7_260_000, ageDays: 13, module: 'Week 5' },
        { name: 'L06 Policy Gradients.pdf', size: 3_450_000, ageDays: 6, module: 'Week 6', synced: false },
      ],
    },
    {
      name: 'Tutorials',
      children: [
        { name: 'Tutorial 1.pdf', size: 310_000, ageDays: 38 },
        { name: 'Tutorial 1 Solutions.pdf', size: 420_000, ageDays: 31 },
        { name: 'Tutorial 2.pdf', size: 298_000, ageDays: 24 },
        { name: 'Tutorial 2 Solutions.pdf', size: 401_000, ageDays: 17 },
        { name: 'Tutorial 3.pdf', size: 336_000, ageDays: 4 },
      ],
    },
    {
      name: 'Assignments',
      children: [
        {
          name: 'PA1',
          children: [
            { name: 'PA1 Handout.pdf', size: 620_000, ageDays: 30 },
            { name: 'pa1_starter.zip', size: 1_840_000, ageDays: 30 },
            { name: 'rubric.docx', size: 74_000, ageDays: 29 },
          ],
        },
        {
          name: 'PA2',
          children: [
            { name: 'PA2 Handout.pdf', size: 690_000, ageDays: 3 },
            { name: 'pa2_starter.zip', size: 2_210_000, ageDays: 3, synced: false },
          ],
        },
      ],
    },
    { name: 'Syllabus.pdf', size: 180_000, ageDays: 48 },
    { name: 'Reading List.docx', size: 46_000, ageDays: 47 },
  ],
  2: [
    {
      name: 'Lecture Notes',
      children: [
        { name: 'Ch1 Convex Sets.pdf', size: 1_240_000, ageDays: 44, module: 'Convexity' },
        { name: 'Ch2 Convex Functions.pdf', size: 1_610_000, ageDays: 36, module: 'Convexity' },
        { name: 'Ch3 Unconstrained Optimisation.pdf', size: 1_950_000, ageDays: 22, module: 'Algorithms' },
        { name: 'Ch4 KKT Conditions.pdf', size: 2_080_000, ageDays: 9, module: 'Algorithms' },
      ],
    },
    {
      name: 'Homework',
      children: [
        { name: 'HW1.pdf', size: 240_000, ageDays: 40 },
        { name: 'HW2.pdf', size: 252_000, ageDays: 26 },
        { name: 'HW3.pdf', size: 268_000, ageDays: 5 },
        { name: 'HW1 Solutions.pdf', size: 390_000, ageDays: 33 },
      ],
    },
    { name: 'Formula Sheet.pdf', size: 120_000, ageDays: 12 },
    { name: 'Past Papers.zip', size: 14_400_000, ageDays: 18, synced: false },
  ],
  3: [
    {
      name: 'Slides',
      children: [
        { name: 'W1 Complex Analysis Recap.pptx', size: 4_120_000, ageDays: 43 },
        { name: 'W2 Fourier Series.pptx', size: 5_010_000, ageDays: 35 },
        { name: 'W3 Fourier Transforms.pptx', size: 4_880_000, ageDays: 21 },
        { name: 'W4 Green Functions.pptx', size: 6_100_000, ageDays: 8, synced: false },
      ],
    },
    {
      name: 'Problem Sets',
      children: [
        { name: 'PS1.pdf', size: 210_000, ageDays: 39 },
        { name: 'PS2.pdf', size: 226_000, ageDays: 25 },
        { name: 'PS3.pdf', size: 231_000, ageDays: 7 },
      ],
    },
    { name: 'Course Outline.docx', size: 52_000, ageDays: 46 },
  ],
  4: [
    {
      name: 'Weekly Materials',
      children: [
        { name: 'Acoustics and Waves.pdf', size: 1_720_000, ageDays: 37, module: 'Unit 1' },
        { name: 'Timbre and Harmonics.pptx', size: 8_940_000, ageDays: 23, module: 'Unit 2' },
        { name: 'Tuning Systems.pdf', size: 990_000, ageDays: 11, module: 'Unit 3' },
      ],
    },
    {
      name: 'Listening',
      children: [
        { name: 'Overtone Demo.mp4', size: 96_400_000, ageDays: 23, synced: false },
        { name: 'Listening Guide.docx', size: 88_000, ageDays: 22 },
      ],
    },
    { name: 'Group Project Brief.pdf', size: 310_000, ageDays: 14 },
  ],
  5: [
    {
      name: 'Admin',
      children: [
        { name: 'FYP Guidelines AY2526.pdf', size: 420_000, ageDays: 52 },
        { name: 'Milestone Schedule.xlsx', size: 64_000, ageDays: 51 },
      ],
    },
    {
      name: 'Drafts',
      children: [
        { name: 'Interim Report v3.docx', size: 1_320_000, ageDays: 2 },
        { name: 'Literature Review.docx', size: 780_000, ageDays: 16 },
        { name: 'figures.zip', size: 22_800_000, ageDays: 2, synced: false },
      ],
    },
    { name: 'Supervisor Notes.docx', size: 41_000, ageDays: 1 },
  ],
  6: [
    {
      name: 'Readings',
      children: [
        { name: 'Week 1 - What is an Idea.pdf', size: 640_000, ageDays: 45, synced: false },
        { name: 'Week 2 - Frameworks.pdf', size: 710_000, ageDays: 38, synced: false },
      ],
    },
    { name: 'Seminar Schedule.pdf', size: 96_000, ageDays: 49, synced: false },
  ],
};

const SYNC_ROOT = 'C:\\Users\\tehwa\\NUSSync';

const TREES: Record<number, FileNode[]> = Object.fromEntries(
  Object.entries(TREE_SPECS).map(([id, specs]) => {
    const courseID = Number(id);
    const code = courseByID(courseID)?.Code ?? 'MISC';
    const root = `${SYNC_ROOT}\\${code}`;
    return [courseID, specs.map((s) => buildNode(s, courseID, '', root))];
  }),
);

function flatten(nodes: FileNode[], out: FileNode[] = []): FileNode[] {
  for (const n of nodes) {
    if (n.IsDir) flatten(n.Children ?? [], out);
    else out.push(n);
  }
  return out;
}

const ALL_FILES: FileNode[] = COURSES.flatMap((c) => flatten(TREES[c.ID] ?? []));

const SNIPPETS = [
  '…the Bellman optimality equation gives V*(s) = max_a Σ P(s\'|s,a)[R + γV*(s\')]…',
  '…under Slater\'s condition strong duality holds, so the KKT conditions are both necessary and sufficient…',
  '…convergence is guaranteed provided the step size satisfies the Robbins–Monro conditions…',
  '…the discrete Fourier transform decomposes the signal into orthogonal basis functions…',
  '…submissions after the stated deadline incur a 10% penalty per day, capped at three days…',
];

// ---------------------------------------------------------------- deadlines

type DeadlineSeed = Omit<Deadline, 'Description' | 'SubmissionTypes' | 'Attachments' | 'Score' | 'Graded' | 'Stats'>;

const DEADLINE_SEEDS: DeadlineSeed[] = [
  {
    ID: 501,
    CourseID: 2,
    CourseCode: 'MA3236',
    Title: 'Homework 2 — Duality and KKT',
    Type: 'assignment',
    DueAt: iso(-2 * DAY - 4 * HOUR),
    Submitted: false,
    URL: 'https://canvas.nus.edu.sg/courses/2/assignments/501',
    PointsPossible: 20,
  },
  {
    ID: 502,
    CourseID: 1,
    CourseCode: 'CS4246',
    Title: 'Quiz 3 — Temporal Difference Learning',
    Type: 'quiz',
    DueAt: iso(9 * HOUR),
    Submitted: false,
    URL: 'https://canvas.nus.edu.sg/courses/1/quizzes/502',
    PointsPossible: 10,
  },
  {
    ID: 503,
    CourseID: 5,
    CourseCode: 'CP4101',
    Title: 'Interim Report Submission',
    Type: 'assignment',
    DueAt: iso(20 * HOUR),
    Submitted: true,
    URL: 'https://canvas.nus.edu.sg/courses/5/assignments/503',
    PointsPossible: 100,
  },
  {
    ID: 504,
    CourseID: 3,
    CourseCode: 'MA3270',
    Title: 'Problem Set 3',
    Type: 'assignment',
    DueAt: iso(1 * DAY + 14 * HOUR),
    Submitted: false,
    URL: 'https://canvas.nus.edu.sg/courses/3/assignments/504',
    PointsPossible: 25,
  },
  {
    ID: 505,
    CourseID: 1,
    CourseCode: 'CS4246',
    Title: 'Programming Assignment 2 — Deep Q-Networks',
    Type: 'assignment',
    DueAt: iso(2 * DAY + 6 * HOUR),
    Submitted: false,
    URL: 'https://canvas.nus.edu.sg/courses/1/assignments/505',
    PointsPossible: 40,
  },
  {
    ID: 506,
    CourseID: 4,
    CourseCode: 'NST2030',
    Title: 'Listening Journal — Week 8',
    Type: 'discussion',
    DueAt: iso(4 * DAY + 3 * HOUR),
    Submitted: false,
    URL: 'https://canvas.nus.edu.sg/courses/4/discussion_topics/506',
    PointsPossible: 5,
  },
  {
    ID: 507,
    CourseID: 2,
    CourseCode: 'MA3236',
    Title: 'Homework 3 — Interior Point Methods',
    Type: 'assignment',
    DueAt: iso(9 * DAY),
    Submitted: false,
    URL: 'https://canvas.nus.edu.sg/courses/2/assignments/507',
    PointsPossible: 20,
  },
  {
    ID: 508,
    CourseID: 4,
    CourseCode: 'NST2030',
    Title: 'Group Project Presentation',
    Type: 'assignment',
    DueAt: iso(17 * DAY + 5 * HOUR),
    Submitted: false,
    URL: 'https://canvas.nus.edu.sg/courses/4/assignments/508',
    PointsPossible: 30,
  },
];

/** GetDeadlines returns the plain rows; detail fields stay zero-valued. */
const DEADLINES: Deadline[] = DEADLINE_SEEDS.map((d) => ({
  ...d,
  Description: '',
  SubmissionTypes: null,
  Attachments: null,
  Score: 0,
  Graded: false,
  Stats: null,
}));

// ------------------------------------------------------------ announcements

const ANNOUNCEMENTS: Announcement[] = [
  {
    ID: 901,
    CourseCode: 'CS4246',
    Title: 'PA2 released — deadline and starter code',
    PostedAt: iso(-5 * HOUR),
    HTML:
      '<p>Dear all,</p><p><strong>Programming Assignment 2</strong> is now available under Files &rarr; Assignments &rarr; PA2. ' +
      'You will implement a DQN agent for the <em>LunarLander-v2</em> environment.</p>' +
      '<ul><li>Deadline: <strong>Friday 23:59</strong></li><li>Weight: 15% of final grade</li>' +
      '<li>Starter code: <a href="https://canvas.nus.edu.sg/courses/1/files/pa2_starter.zip">pa2_starter.zip</a></li></ul>' +
      '<p>Please start early — training runs take about 40 minutes on CPU.</p><p>— Prof Lee</p>',
    Text: 'Programming Assignment 2 is now available under Files > Assignments > PA2...',
    URL: 'https://canvas.nus.edu.sg/courses/1/discussion_topics/901',
    Read: false,
  },
  {
    ID: 902,
    CourseCode: 'MA3236',
    Title: 'Consultation slots for next week',
    PostedAt: iso(-1 * DAY - 2 * HOUR),
    HTML:
      '<p>I will hold extra consultation on <strong>Tuesday 2–4pm</strong> and <strong>Thursday 10am–12pm</strong> ' +
      'in S17-06-12. Please book a 15-minute slot on the shared sheet.</p>' +
      '<p>Bring your HW2 attempts — I would rather discuss your work than re-derive the notes.</p>',
    Text: 'I will hold extra consultation on Tuesday 2-4pm and Thursday 10am-12pm in S17-06-12...',
    URL: 'https://canvas.nus.edu.sg/courses/2/discussion_topics/902',
    Read: false,
  },
  {
    ID: 903,
    CourseCode: 'MA3270',
    Title: 'Correction to Problem Set 3, Question 4',
    PostedAt: iso(-2 * DAY - 7 * HOUR),
    HTML:
      '<p>There is a typo in Q4(b): the boundary condition should read <em>u(0, t) = 0</em>, not <em>u(0, t) = 1</em>. ' +
      'An updated PDF has been uploaded.</p><p>Apologies for the confusion.</p>',
    Text: 'There is a typo in Q4(b): the boundary condition should read u(0, t) = 0...',
    URL: 'https://canvas.nus.edu.sg/courses/3/discussion_topics/903',
    Read: true,
  },
  {
    ID: 904,
    CourseCode: 'CP4101',
    Title: 'Interim report template updated',
    PostedAt: iso(-4 * DAY),
    HTML:
      '<p>The interim report template has been revised to include a <strong>risk register</strong> section. ' +
      'If you have already started writing, you only need to add the new section — no reformatting required.</p>' +
      '<p>Page limit remains <strong>12 pages excluding references</strong>.</p>',
    Text: 'The interim report template has been revised to include a risk register section...',
    URL: 'https://canvas.nus.edu.sg/courses/5/discussion_topics/904',
    Read: true,
  },
  {
    ID: 905,
    CourseCode: 'NST2030',
    Title: 'Guest lecture: acoustics of the Esplanade concert hall',
    PostedAt: iso(-6 * DAY - 3 * HOUR),
    HTML:
      '<p>We are joined next week by an acoustician who worked on the Esplanade. The session replaces the usual ' +
      'seminar and will run in LT27.</p><p>Attendance is not graded but is strongly encouraged.</p>',
    Text: 'We are joined next week by an acoustician who worked on the Esplanade...',
    URL: 'https://canvas.nus.edu.sg/courses/4/discussion_topics/905',
    Read: true,
  },
];

// ---------------------------------------------------------------- grades

type GradeSeed = Omit<Grade, 'Mean'>;

const GRADE_SEEDS: GradeSeed[] = [
  { CourseCode: 'CS4246', Title: 'Quiz 1 — MDP Basics', Score: 9, Possible: 10, GradedAt: iso(-28 * DAY), URL: 'https://canvas.nus.edu.sg/courses/1/grades' },
  { CourseCode: 'CS4246', Title: 'Quiz 2 — Dynamic Programming', Score: 7.5, Possible: 10, GradedAt: iso(-14 * DAY), URL: 'https://canvas.nus.edu.sg/courses/1/grades' },
  { CourseCode: 'CS4246', Title: 'Programming Assignment 1', Score: 36, Possible: 40, GradedAt: iso(-9 * DAY), URL: 'https://canvas.nus.edu.sg/courses/1/grades' },
  { CourseCode: 'MA3236', Title: 'Homework 1', Score: 17, Possible: 20, GradedAt: iso(-21 * DAY), URL: 'https://canvas.nus.edu.sg/courses/2/grades' },
  { CourseCode: 'MA3236', Title: 'Midterm Test', Score: 41, Possible: 60, GradedAt: iso(-6 * DAY), URL: 'https://canvas.nus.edu.sg/courses/2/grades' },
  { CourseCode: 'MA3270', Title: 'Problem Set 1', Score: 22, Possible: 25, GradedAt: iso(-24 * DAY), URL: 'https://canvas.nus.edu.sg/courses/3/grades' },
  { CourseCode: 'MA3270', Title: 'Problem Set 2', Score: 19, Possible: 25, GradedAt: iso(-10 * DAY), URL: 'https://canvas.nus.edu.sg/courses/3/grades' },
  { CourseCode: 'NST2030', Title: 'Listening Journal — Week 4', Score: 5, Possible: 5, GradedAt: iso(-18 * DAY), URL: 'https://canvas.nus.edu.sg/courses/4/grades' },
  { CourseCode: 'NST2030', Title: 'Reflection Essay', Score: 13, Possible: 20, GradedAt: iso(-5 * DAY), URL: 'https://canvas.nus.edu.sg/courses/4/grades' },
  { CourseCode: 'CP4101', Title: 'Project Proposal', Score: 18, Possible: 20, GradedAt: iso(-31 * DAY), URL: 'https://canvas.nus.edu.sg/courses/5/grades' },
];

/** Means are filled opportunistically by GetDeadlineDetail — some are still 0. */
const GRADE_MEANS: Record<string, number> = {
  'Quiz 1 — MDP Basics': 7.8,
  'Quiz 2 — Dynamic Programming': 7.9,
  'Programming Assignment 1': 31.4,
  'Homework 1': 14.2,
  'Midterm Test': 38.6,
  'Problem Set 2': 20.1,
  'Reflection Essay': 14.8,
};

const GRADES: Grade[] = GRADE_SEEDS.map((g) => ({ ...g, Mean: GRADE_MEANS[g.Title] ?? 0 }));

// ---------------------------------------------------------------- settings

let settings: Settings = {
  CanvasURL: 'https://canvas.nus.edu.sg',
  CanvasToken: '7~mockmockmockmockmockmockmock',
  SyncDir: SYNC_ROOT,
  TelegramToken: '',
  TelegramChatID: '',
  ReminderLadder: ['72h', '48h', '24h', '3h', '1h'],
  MaxFileMB: 100,
  SkipExts: ['.mp4', '.mov'],
  SyncIntervalMin: 60,
  NotifyAnnouncements: true,
  NotifyGrades: true,
  NotifyDesktop: true,
  LaunchAtLogin: false,
  Hotkey: 'ctrl+shift+n',
  Theme: 'system',
};

let telegram: TelegramStatus = { Configured: false, ChatID: '', BotName: '@nuscanvassync_bot' };

// ---------------------------------------------------------------- sync sim

let status: SyncStatus = {
  Running: false,
  Phase: 'idle',
  Course: '',
  Done: 0,
  Total: 0,
  CurrentFile: '',
  LastRun: iso(-42 * 60_000),
  LastError: '',
  BytesDownloaded: 0,
};

let syncTimers: ReturnType<typeof setTimeout>[] = [];

function clearSyncTimers() {
  syncTimers.forEach(clearTimeout);
  syncTimers = [];
}

function at(ms: number, fn: () => void) {
  syncTimers.push(setTimeout(fn, ms));
}

function push(patch: Partial<SyncStatus>) {
  status = { ...status, ...patch };
  localEmitter.emit('sync:status', status);
}

/** A ~4s fake sync that walks through listing -> downloading -> indexing. */
function runFakeSync() {
  clearSyncTimers();
  const enabled = COURSES.filter((c) => c.Enabled);
  const total = 46;
  status = {
    Running: true,
    Phase: 'listing',
    Course: enabled[0]?.Code ?? '',
    Done: 0,
    Total: total,
    CurrentFile: '',
    LastRun: status.LastRun,
    LastError: '',
    BytesDownloaded: 0,
  };
  localEmitter.emit('sync:status', status);

  const pool = ALL_FILES.filter((f) => f.CourseID !== 6);
  const steps = 34;
  const downloadStart = 600;
  const downloadEnd = 3300;

  at(downloadStart, () => push({ Phase: 'downloading' }));

  for (let i = 1; i <= steps; i++) {
    const t = downloadStart + ((downloadEnd - downloadStart) * i) / steps;
    at(t, () => {
      const f = pool[(i * 3) % pool.length];
      push({
        Phase: 'downloading',
        Course: courseByID(f.CourseID)?.Code ?? '',
        Done: Math.round((total * i) / steps),
        CurrentFile: f.Name,
        BytesDownloaded: status.BytesDownloaded + f.Size,
      });
    });
  }

  at(downloadEnd + 120, () =>
    push({ Phase: 'indexing', Done: total, CurrentFile: 'Building search index…', Course: '' }),
  );

  at(4000, () => {
    // mark the previously-unsynced files as downloaded
    for (const f of ALL_FILES) if (f.CourseID !== 6) f.Synced = true;
    const stamp = new Date().toISOString();
    for (const c of COURSES) if (c.Enabled) c.LastSynced = stamp;
    status = {
      Running: false,
      Phase: 'idle',
      Course: '',
      Done: total,
      Total: total,
      CurrentFile: '',
      LastRun: stamp,
      LastError: '',
      BytesDownloaded: status.BytesDownloaded,
    };
    localEmitter.emit('sync:status', status);
    localEmitter.emit('sync:done', status);
    localEmitter.emit('deadlines:updated');
    localEmitter.emit('toast', { Level: 'success', Message: `Sync complete — ${total} files up to date` });
  });
}

// ---------------------------------------------------------------- feed

/** When the user last opened the What's-new feed. */
let feedSeenAt = now - 2.5 * DAY;

/** Files re-downloaded because Canvas reported a change (vs. brand new). */
const UPDATED_NAMES = new Set([
  'Tutorial 2 Solutions.pdf',
  'PS3.pdf',
  'Formula Sheet.pdf',
  'Supervisor Notes.docx',
]);

/** Rows that predate the feature never appear in the feed. */
const FEED_HORIZON = 30 * DAY;

function feedEligible(f: FileNode): boolean {
  return !f.IsDir && f.Synced && now - Date.parse(f.ModifiedAt) <= FEED_HORIZON;
}

/** Re-stamp FileNode.IsNew across every tree after the seen marker moves. */
function refreshIsNew(): void {
  const walk = (nodes: FileNode[]) => {
    for (const n of nodes) {
      if (n.IsDir) walk(n.Children ?? []);
      else n.IsNew = feedEligible(n) && Date.parse(n.ModifiedAt) > feedSeenAt;
    }
  };
  for (const list of Object.values(TREES)) walk(list);
}

refreshIsNew();

function unseenCount(): number {
  return ALL_FILES.filter((f) => f.IsNew).length;
}

function toFeedItem(f: FileNode): FeedItem {
  return {
    ID: f.ID,
    CourseID: f.CourseID,
    CourseCode: courseByID(f.CourseID)?.Code ?? '',
    Name: f.Name,
    Path: f.Path,
    RelPath: f.RelPath,
    Size: f.Size,
    ChangedAt: f.ModifiedAt,
    Kind: UPDATED_NAMES.has(f.Name) ? 'updated' : 'new',
    Module: f.Module,
  };
}

// ------------------------------------------------------- deadline detail

interface DetailSeed {
  Description: string;
  SubmissionTypes: string[];
  attachmentNames?: string[];
  Score?: number;
  Graded?: boolean;
  Stats?: ScoreStats;
}

const DEADLINE_DETAILS: Record<number, DetailSeed> = {
  501: {
    Description:
      '<p>Work through the duality material from <strong>Chapters 3 and 4</strong>. Submit a single PDF.</p>' +
      '<ol><li>Derive the Lagrangian dual of the given QP.</li>' +
      '<li>State the KKT conditions and verify Slater&rsquo;s condition holds.</li>' +
      '<li>Show that strong duality gives a zero duality gap here.</li></ol>' +
      '<p>Late submissions incur <em>10% per day</em>, capped at three days.</p>',
    SubmissionTypes: ['online_upload'],
    attachmentNames: ['HW2.pdf', 'Ch4 KKT Conditions.pdf'],
    Score: 14,
    Graded: true,
    Stats: { Mean: 13.1, Min: 4, Max: 20, Median: 13.5, Count: 118 },
  },
  502: {
    Description:
      '<p>A short online quiz on <strong>temporal difference learning</strong>. You get one attempt and 25 minutes.</p>' +
      '<ul><li>Covers TD(0), SARSA and Q-learning.</li><li>Open notes, no collaboration.</li></ul>',
    SubmissionTypes: ['online_quiz'],
    attachmentNames: ['L04 Monte Carlo and TD Learning.pdf'],
  },
  503: {
    Description:
      '<p>Submit the interim report using the revised template (the one with the <strong>risk register</strong> section).</p>' +
      '<p>Page limit is <strong>12 pages excluding references</strong>.</p>',
    SubmissionTypes: ['online_upload'],
    attachmentNames: ['Interim Report v3.docx', 'FYP Guidelines AY2526.pdf'],
    Score: 86,
    Graded: true,
    Stats: { Mean: 78.4, Min: 52, Max: 96, Median: 79, Count: 41 },
  },
  505: {
    Description:
      '<p>Implement a <strong>DQN</strong> agent for <em>LunarLander-v2</em>. Starter code is in ' +
      '<code>pa2_starter.zip</code>.</p><ul><li>Replay buffer and target network are both required.</li>' +
      '<li>Report mean return over 100 evaluation episodes.</li>' +
      '<li>Training takes roughly 40 minutes on CPU — start early.</li></ul>',
    SubmissionTypes: ['online_upload', 'online_text_entry'],
    attachmentNames: ['PA2 Handout.pdf', 'pa2_starter.zip'],
  },
  504: {
    Description:
      '<p>Questions 1&ndash;5 of Problem Set 3. Note the corrected boundary condition in Q4(b): <em>u(0, t) = 0</em>.</p>',
    SubmissionTypes: ['online_upload'],
    attachmentNames: ['PS3.pdf'],
  },
};

function fileByName(name: string): FileNode | undefined {
  return ALL_FILES.find((f) => f.Name === name);
}

// ------------------------------------------------------------------ study

const STUDY_STATUS: StudyStatus = {
  CLIFound: true,
  Version: '2.0.31 (Claude Code)',
  LoggedIn: true,
  Error: '',
  Models: ['opus', 'sonnet', 'haiku'],
};

let jobSeq = 0;
let quizSeq = 100;
let questionSeq = 1000;
let cardSeq = 5000;

const JOBS: StudyJob[] = [];
const OVERVIEWS = new Map<number, Overview>();
const QUIZZES: Quiz[] = [];
const ATTEMPTS: QuizAttempt[] = [];
const ASKS: Array<AskResult & { fileIDs: number[] }> = [];
const CARDS: Flashcard[] = [];

const jobTimers = new Map<string, Array<ReturnType<typeof setTimeout>>>();

function emitJob(j: StudyJob) {
  localEmitter.emit('study:job', { ...j });
}

const PROGRESS_LINES: Record<string, string[]> = {
  overview: ['Reading the file…', 'Extracting section structure…', 'Summarising key ideas…', 'Writing the overview…'],
  quiz: ['Reading the file…', 'Picking examinable concepts…', 'Drafting questions…', 'Checking answers and page refs…'],
  ask: ['Reading the file…', 'Locating the relevant passages…', 'Composing an answer…'],
  flashcards: ['Reading the file…', 'Selecting atomic facts…', 'Writing card fronts and backs…'],
};

/**
 * Fake ~5s job: queued -> running with live progress lines -> done. `finish`
 * writes the result into the cache and returns the toast message.
 */
function startJob(kind: string, fileIDs: number[], model: string, finish: () => string): string {
  const id = `job-${++jobSeq}`;
  const job: StudyJob = {
    ID: id,
    Kind: kind,
    FileIDs: [...fileIDs],
    Status: 'queued',
    Progress: 'Queued',
    Error: '',
    StartedAt: new Date().toISOString(),
    FinishedAt: '',
    Model: model || 'sonnet',
    CostUSD: 0,
  };
  JOBS.unshift(job);
  const timers: Array<ReturnType<typeof setTimeout>> = [];
  jobTimers.set(id, timers);
  const at = (ms: number, fn: () => void) => timers.push(setTimeout(fn, ms));

  emitJob(job);

  const lines = PROGRESS_LINES[kind] ?? PROGRESS_LINES.overview;
  at(180, () => {
    job.Status = 'running';
    job.Progress = lines[0];
    emitJob(job);
    localEmitter.emit('study:progress', { JobID: id, Text: lines[0] });
  });

  lines.slice(1).forEach((line, i) => {
    at(900 + i * 1150, () => {
      job.Progress = line;
      emitJob(job);
      localEmitter.emit('study:progress', { JobID: id, Text: line });
    });
  });

  at(5000, () => {
    let message: string;
    try {
      message = finish();
    } catch (err) {
      job.Status = 'error';
      job.Error = err instanceof Error ? err.message : String(err);
      job.FinishedAt = new Date().toISOString();
      emitJob(job);
      localEmitter.emit('toast', { Level: 'error', Message: `Study job failed: ${job.Error}` });
      jobTimers.delete(id);
      return;
    }
    job.Status = 'done';
    job.Progress = 'Done';
    job.FinishedAt = new Date().toISOString();
    job.CostUSD = Math.round((0.02 + Math.random() * 0.09) * 1000) / 1000;
    emitJob(job);
    localEmitter.emit('toast', { Level: 'success', Message: message });
    jobTimers.delete(id);
  });

  return id;
}

// ------------------------------------------------------- generated content

const OVERVIEW_BODIES: Record<number, string> = {
  1: [
    '# {name}',
    '',
    '## What this covers',
    'This deck develops **sequential decision making** under uncertainty. The through-line',
    'is that a policy is only as good as the value function you can estimate for it.',
    '',
    '## Key ideas',
    '- A *Markov decision process* is the tuple `(S, A, P, R, gamma)`.',
    "- The **Bellman optimality equation** ties a state's value to its successors.",
    '- Policy iteration alternates *evaluation* and *improvement*; value iteration folds the two together.',
    '- Discounting with gamma < 1 keeps infinite-horizon returns bounded.',
    '',
    '## Worth memorising',
    "1. `V*(s) = max_a sum P(s'|s,a) [ R(s,a,s') + gamma V*(s') ]`",
    '2. Contraction mapping means value iteration converges geometrically.',
    '3. Greedy improvement over an exact `V^pi` never makes the policy worse.',
    '',
    '## Where students slip',
    'Confusing the *state* value `V` with the *action* value `Q`, and forgetting that policy',
    'improvement needs the **full** expectation, not a single sampled successor.',
  ].join('\n'),
  2: [
    '# {name}',
    '',
    '## What this covers',
    'Constrained optimisation, from **convexity** through to the KKT conditions.',
    '',
    '## Key ideas',
    '- A set is convex when every chord between two of its points stays inside it.',
    '- The *Lagrangian* is `L(x, lam, nu) = f(x) + sum lam_i g_i(x) + sum nu_j h_j(x)`.',
    '- Weak duality always holds; **strong duality** needs a constraint qualification.',
    "- Under **Slater's condition** the KKT conditions are necessary *and* sufficient.",
    '',
    '## Worth memorising',
    '1. Stationarity, primal feasibility, dual feasibility, complementary slackness.',
    '2. `lam_i >= 0` for inequality multipliers, unrestricted sign for equalities.',
    '3. A zero duality gap certifies optimality without re-solving the primal.',
    '',
    '## Where students slip',
    'Writing complementary slackness as `lam_i = 0` *and* `g_i(x) = 0` rather than the',
    'product being zero.',
  ].join('\n'),
  3: [
    '# {name}',
    '',
    '## What this covers',
    'Transform methods for linear PDEs and the boundary-value problems they solve.',
    '',
    '## Key ideas',
    '- **Fourier series** decompose a periodic signal onto an orthogonal basis.',
    '- The transform pair swaps differentiation for multiplication by `i*omega`.',
    "- *Green's functions* express a solution as a convolution with the impulse response.",
    '- Boundary conditions pick out which eigenfunctions survive.',
    '',
    '## Worth memorising',
    '1. Parseval: energy is preserved between the time and frequency domains.',
    '2. Convolution in one domain is multiplication in the other.',
    '3. A Sturm-Liouville operator has real eigenvalues and orthogonal eigenfunctions.',
    '',
    '## Where students slip',
    'Dropping the `1/2pi` normalisation, and applying the boundary condition *after*',
    'transforming rather than before.',
  ].join('\n'),
};

const GENERIC_OVERVIEW = [
  '# {name}',
  '',
  '## What this covers',
  'A working summary of the material in this file, at the level the assessment expects.',
  '',
  '## Key ideas',
  '- The main argument is developed in stages; each section builds on the last.',
  '- Definitions come first, then the results that depend on them.',
  '- Worked examples show the method rather than just the answer.',
  '',
  '## Worth memorising',
  '1. The definitions introduced early — everything later leans on them.',
  '2. The two or three results that are quoted repeatedly.',
  '3. The conditions under which each result actually applies.',
  '',
  '## Where students slip',
  'Learning the statement of a result without its hypotheses, then applying it where the',
  'hypotheses fail.',
].join('\n');

function makeOverview(fileID: number, model: string): Overview {
  const f = ALL_FILES.find((x) => x.ID === fileID);
  const body = (OVERVIEW_BODIES[f?.CourseID ?? 0] ?? GENERIC_OVERVIEW).replace(
    /\{name\}/g,
    f?.Name.replace(/\.[a-z0-9]+$/i, '') ?? 'This file',
  );
  return { FileID: fileID, Markdown: body, CreatedAt: new Date().toISOString(), Model: model || 'sonnet' };
}

type QSeed = Omit<Question, 'ID'>;

const QUESTION_BANK: Record<number, QSeed[]> = {
  1: [
    {
      Type: 'mcq',
      Prompt: 'In an MDP, what does the discount factor gamma control?',
      Options: [
        'How much future rewards are worth relative to immediate ones',
        'The probability of transitioning to a terminal state',
        'The learning rate of the value update',
        'The exploration/exploitation trade-off',
      ],
      Answer: 'A',
      Explanation:
        'Gamma weights rewards by how far in the future they arrive; gamma < 1 also keeps the infinite-horizon return finite.',
      Page: 7,
    },
    {
      Type: 'mcq',
      Prompt: 'Value iteration converges because the Bellman optimality operator is…',
      Options: ['Linear', 'A contraction mapping in the sup-norm', 'Idempotent', 'Monotone but not bounded'],
      Answer: 'B',
      Explanation:
        'It contracts with modulus gamma, so the Banach fixed-point theorem gives a unique V* and geometric convergence.',
      Page: 21,
    },
    {
      Type: 'mcq',
      Prompt: 'Which update rule is off-policy?',
      Options: ['SARSA', 'TD(0) policy evaluation', 'Q-learning', 'Monte Carlo first-visit'],
      Answer: 'C',
      Explanation:
        'Q-learning bootstraps from max_a Q(s2,a) regardless of the action actually taken, so it learns the greedy policy while behaving otherwise.',
      Page: 14,
    },
    {
      Type: 'mcq',
      Prompt: 'What problem does a target network in DQN address?',
      Options: [
        'Exploding gradients from large rewards',
        'Correlated samples within a minibatch',
        'A moving regression target destabilising training',
        'Overestimation caused by function approximation',
      ],
      Answer: 'C',
      Explanation: 'Freezing the bootstrap target for several thousand steps stops the network chasing its own predictions.',
      Page: 32,
    },
    {
      Type: 'short',
      Prompt: 'State the Bellman optimality equation for V*.',
      Options: null,
      Answer: "V*(s) = max_a sum_{s'} P(s'|s,a) [ R(s,a,s') + gamma V*(s') ]",
      Explanation: 'The value of a state under the optimal policy is the best expected one-step reward plus discounted successor value.',
      Page: 9,
    },
    {
      Type: 'short',
      Prompt: 'Why does experience replay help a DQN agent?',
      Options: null,
      Answer:
        'It breaks the temporal correlation between consecutive samples and reuses each transition many times, which makes the gradient estimates closer to i.i.d. and far more sample-efficient.',
      Explanation: 'Without replay the network sees a highly correlated stream and tends to forget earlier parts of the state space.',
      Page: 34,
    },
  ],
  2: [
    {
      Type: 'mcq',
      Prompt: 'Which condition guarantees strong duality for a convex program?',
      Options: [
        'Linear independence of the constraint gradients',
        "Slater's condition",
        'Compactness of the feasible set',
        'Differentiability of the objective',
      ],
      Answer: 'B',
      Explanation: 'A strictly feasible point for the inequality constraints is enough for a zero duality gap in a convex problem.',
      Page: 12,
    },
    {
      Type: 'mcq',
      Prompt: 'Complementary slackness says that at an optimum…',
      Options: [
        'lam_i = 0 for every i',
        'g_i(x) = 0 for every i',
        'lam_i * g_i(x) = 0 for every i',
        'lam_i + g_i(x) = 0 for every i',
      ],
      Answer: 'C',
      Explanation: 'Either the constraint is active or its multiplier vanishes — the product, not each factor, is zero.',
      Page: 18,
    },
    {
      Type: 'mcq',
      Prompt: 'A function is convex if and only if its epigraph is…',
      Options: ['Closed', 'Bounded', 'A convex set', 'A cone'],
      Answer: 'C',
      Explanation: 'Convexity of the epigraph is the geometric restatement of the analytic inequality.',
      Page: 4,
    },
    {
      Type: 'mcq',
      Prompt: 'For a twice-differentiable f on an open convex domain, convexity is equivalent to…',
      Options: ['grad f(x) = 0 somewhere', 'Hessian positive semidefinite everywhere', 'Hessian positive definite everywhere', 'f bounded below'],
      Answer: 'B',
      Explanation:
        'A positive semidefinite Hessian on the whole domain characterises convexity; strict definiteness gives strict convexity but is not necessary.',
      Page: 6,
    },
    {
      Type: 'short',
      Prompt: 'List the four KKT conditions.',
      Options: null,
      Answer: 'Stationarity of the Lagrangian, primal feasibility, dual feasibility (multipliers >= 0), and complementary slackness.',
      Explanation: 'Under a constraint qualification these are necessary; for a convex problem they are also sufficient.',
      Page: 19,
    },
  ],
  3: [
    {
      Type: 'mcq',
      Prompt: 'Under the Fourier transform, differentiation in time becomes…',
      Options: ['Division by i*omega', 'Multiplication by i*omega', 'Convolution with a step', 'Multiplication by omega squared'],
      Answer: 'B',
      Explanation: 'That is exactly why transforms turn linear constant-coefficient ODEs into algebra.',
      Page: 11,
    },
    {
      Type: 'mcq',
      Prompt: "A Green's function is the response of the operator to…",
      Options: ['A sinusoid', 'A unit impulse', 'A step input', 'Homogeneous boundary data'],
      Answer: 'B',
      Explanation: 'Linearity then gives the general solution as a convolution of the source with the impulse response.',
      Page: 24,
    },
    {
      Type: 'mcq',
      Prompt: 'Eigenfunctions of a regular Sturm-Liouville problem are…',
      Options: ['Always polynomials', 'Orthogonal with respect to the weight function', 'Never unique', 'Complex-valued in general'],
      Answer: 'B',
      Explanation: 'Self-adjointness gives real eigenvalues and a weighted-orthogonal eigenbasis.',
      Page: 16,
    },
    {
      Type: 'short',
      Prompt: "State Parseval's theorem in words.",
      Options: null,
      Answer:
        'The total energy of a signal computed in the time domain equals the total energy of its transform in the frequency domain, up to the chosen normalisation constant.',
      Explanation: 'It follows from the transform being a unitary map on L-squared.',
      Page: 13,
    },
  ],
};

const GENERIC_QUESTIONS: QSeed[] = [
  {
    Type: 'mcq',
    Prompt: 'What is the stated purpose of this document?',
    Options: [
      'To set out the material and how it will be assessed',
      'To replace the lectures entirely',
      'To collect past exam papers',
      'To record attendance',
    ],
    Answer: 'A',
    Explanation: 'The opening section frames the scope and the assessment that follows from it.',
    Page: 1,
  },
  {
    Type: 'mcq',
    Prompt: 'Which section introduces the definitions the rest of the file depends on?',
    Options: ['The appendix', 'The first substantive section', 'The reference list', 'The summary'],
    Answer: 'B',
    Explanation: 'Definitions are established before any result that uses them.',
    Page: 2,
  },
  {
    Type: 'short',
    Prompt: 'Summarise the main argument of this file in two sentences.',
    Options: null,
    Answer:
      'The file introduces its core definitions, then develops the results that follow from them, closing with worked examples that show the method in use.',
    Explanation: 'A good summary names the definitions, the results and the worked method.',
    Page: 1,
  },
];

function makeQuiz(fileIDs: number[], model: string, n: number): Quiz {
  const count = n > 0 ? Math.min(n, 50) : 5;
  const files = fileIDs.map((id) => ALL_FILES.find((f) => f.ID === id)).filter(Boolean) as FileNode[];
  const pool: QSeed[] = [];
  const seen = new Set<number>();
  for (const f of files) {
    if (seen.has(f.CourseID)) continue;
    seen.add(f.CourseID);
    pool.push(...(QUESTION_BANK[f.CourseID] ?? []));
  }
  pool.push(...GENERIC_QUESTIONS);

  const questions: Question[] = [];
  for (let i = 0; i < count; i++) {
    questions.push({ ...pool[i % pool.length], ID: ++questionSeq });
  }
  const title =
    files.length === 1
      ? files[0].Name.replace(/\.[a-z0-9]+$/i, '')
      : `${files.length} files · ${courseByID(files[0]?.CourseID ?? 0)?.Code ?? 'Mixed'}`;
  return {
    ID: ++quizSeq,
    FileIDs: [...fileIDs],
    Title: `${title} — ${count} questions`,
    CreatedAt: new Date().toISOString(),
    Model: model || 'sonnet',
    Questions: questions,
  };
}

const CARD_BANK: Record<number, Array<[string, string]>> = {
  1: [
    ['Markov property', 'The next state depends only on the current state and action, not on the history that led there.'],
    ['Policy pi', 'A mapping from states to a distribution over actions.'],
    ['Return G_t', 'The discounted sum of future rewards from time t onwards.'],
    ['On-policy vs off-policy', 'On-policy learns the value of the behaviour policy; off-policy learns about a different target policy.'],
    ['Bootstrapping', 'Updating an estimate using another estimate rather than a full sampled return.'],
    ['Epsilon-greedy', 'Take the greedy action with probability 1 - epsilon, otherwise act uniformly at random.'],
  ],
  2: [
    ['Convex set', 'Every convex combination of two points in the set is also in the set.'],
    ['Lagrangian', 'The objective augmented with weighted constraints: f(x) + sum lam_i g_i(x) + sum nu_j h_j(x).'],
    ['Weak duality', 'The dual optimum is always at most the primal optimum, for any problem.'],
    ['Duality gap', 'Primal optimum minus dual optimum; zero under strong duality.'],
    ["Slater's condition", 'A strictly feasible point exists for the inequality constraints of a convex problem.'],
    ['Complementary slackness', 'lam_i * g_i(x*) = 0 for every inequality constraint at an optimum.'],
  ],
  3: [
    ['Fourier series', 'Expansion of a periodic function onto an orthogonal basis of sines and cosines.'],
    ['Convolution theorem', 'Convolution in one domain equals pointwise multiplication in the other.'],
    ["Green's function", 'The response of a linear operator to a unit impulse, with the given boundary conditions.'],
    ['Parseval', 'Energy is conserved between a signal and its transform.'],
    ['Sturm-Liouville', 'A self-adjoint second-order operator with real eigenvalues and orthogonal eigenfunctions.'],
  ],
};

const GENERIC_CARDS: Array<[string, string]> = [
  ['Core definition', 'The term this file introduces first, and the property that makes it useful.'],
  ['Main result', 'The statement that the rest of the file relies on, together with its hypotheses.'],
  ['Common mistake', 'Applying the main result outside the conditions under which it holds.'],
];

function makeCards(fileID: number, n: number): Flashcard[] {
  const f = ALL_FILES.find((x) => x.ID === fileID);
  const pool = CARD_BANK[f?.CourseID ?? 0] ?? GENERIC_CARDS;
  const count = n > 0 ? Math.min(n, 100) : Math.min(pool.length, 20);
  const out: Flashcard[] = [];
  for (let i = 0; i < count; i++) {
    const pair = pool[i % pool.length];
    out.push({
      ID: ++cardSeq,
      FileID: fileID,
      Front: pair[0],
      Back: pair[1],
      Due: new Date(now - 60_000).toISOString(),
      Interval: 0,
      Ease: 2.5,
    });
  }
  return out;
}

function makeAsk(fileIDs: number[], question: string): AskResult & { fileIDs: number[] } {
  const files = fileIDs.map((id) => ALL_FILES.find((f) => f.ID === id)).filter(Boolean) as FileNode[];
  const cites = files.slice(0, 3).map((f, i) => `${f.Name} · p.${8 + i * 7}`);
  return {
    fileIDs: [...fileIDs],
    Question: question,
    Answer: [
      'Short answer: **yes, with one caveat**.',
      '',
      'The material defines the object first and only then states the result, so the hypotheses',
      'matter as much as the conclusion. In the notes this is developed in three steps:',
      '',
      '- the definition is given in its most general form;',
      '- a *special case* is worked through in full;',
      '- the general statement is then proved by reducing to that case.',
      '',
      'The caveat is that the reduction needs the regularity assumption stated earlier — drop it',
      'and the second step no longer applies.',
    ].join('\n'),
    Citations: cites.length ? cites : ['No files selected'],
    CreatedAt: new Date().toISOString(),
  };
}

/** Normalise an MCQ answer the way the backend does: "b", "B)" and the option text all work. */
function normaliseMCQ(given: string, q: Question): string {
  const g = (given ?? '').trim();
  if (!g) return '';
  const letter = /^([A-Za-z])[).\s]*$/.exec(g);
  if (letter) return letter[1].toUpperCase();
  const idx = (q.Options ?? []).findIndex((o) => o.trim().toLowerCase() === g.toLowerCase());
  if (idx >= 0) return String.fromCharCode(65 + idx);
  return g.toUpperCase();
}

// --- pre-seeded cache, so the Study tabs and Quiz Rush are not empty on load

(function seedStudy() {
  const seeds: Array<[string, number]> = [
    ['L02 Markov Decision Processes.pdf', 6],
    ['Ch4 KKT Conditions.pdf', 5],
    ['PS3.pdf', 4],
  ];
  seeds.forEach(([name, n], i) => {
    const f = fileByName(name);
    if (!f) return;
    const q = makeQuiz([f.ID], 'sonnet', n);
    q.CreatedAt = new Date(now - (i + 1) * 2 * HOUR).toISOString();
    QUIZZES.unshift(q);
  });
  const l02 = fileByName('L02 Markov Decision Processes.pdf');
  if (l02) {
    OVERVIEWS.set(l02.ID, { ...makeOverview(l02.ID, 'sonnet'), CreatedAt: new Date(now - 3 * HOUR).toISOString() });
    CARDS.push(...makeCards(l02.ID, 6));
    const oldest = QUIZZES[QUIZZES.length - 1];
    if (oldest) {
      ATTEMPTS.push({
        QuizID: oldest.ID,
        Answers: {},
        Score: 4,
        Total: (oldest.Questions ?? []).length,
        TakenAt: new Date(now - 26 * HOUR).toISOString(),
      });
    }
  }
})();

// ---------------------------------------------------------------- the mock

export const mockAPI: AppAPI = {
  GetCourses: () => delay(COURSES.map((c) => ({ ...c }))),

  SetCourseEnabled: async (id, enabled) => {
    const c = courseByID(id);
    if (c) c.Enabled = enabled;
    await delay(null, 40);
  },

  GetTree: async (courseID) => {
    await delay(null, 110);
    if (courseID === 0) {
      return COURSES.flatMap((c) => TREES[c.ID] ?? []);
    }
    return TREES[courseID] ?? [];
  },

  GetRecentFiles: async (limit) => {
    await delay(null, 80);
    return [...ALL_FILES]
      .sort((a, b) => Date.parse(b.ModifiedAt) - Date.parse(a.ModifiedAt))
      .slice(0, limit);
  },

  Search: async (query, courseID) => {
    await delay(null, 180);
    const q = query.trim().toLowerCase();
    if (!q) return [];
    const pool = courseID ? ALL_FILES.filter((f) => f.CourseID === courseID) : ALL_FILES;
    const hits: SearchHit[] = [];
    pool.forEach((f, i) => {
      const nameHit = f.Name.toLowerCase().includes(q);
      const pathHit = f.RelPath.toLowerCase().includes(q);
      // pretend full-text matched for a deterministic slice of the corpus
      const textHit = !nameHit && !pathHit && f.Synced && i % 7 === 0 && q.length >= 3;
      if (!nameHit && !pathHit && !textHit) return;
      hits.push({
        File: f,
        CourseCode: courseByID(f.CourseID)?.Code ?? '',
        Snippet: nameHit || pathHit ? '' : SNIPPETS[i % SNIPPETS.length],
        Score: nameHit ? 1 : pathHit ? 0.6 : 0.3,
      });
    });
    return hits.sort((a, b) => b.Score - a.Score).slice(0, 60);
  },

  OpenFile: async (path) => {
    localEmitter.emit('toast', { Level: 'info', Message: `Opening ${path.split('\\').pop()}` });
    await delay(null, 30);
  },

  RevealFile: async (path) => {
    localEmitter.emit('toast', { Level: 'info', Message: `Revealed in Explorer: ${path}` });
    await delay(null, 30);
  },

  OpenURL: async (url) => {
    localEmitter.emit('toast', { Level: 'info', Message: `Opening ${url}` });
    await delay(null, 30);
  },

  SyncNow: async () => {
    if (status.Running) return;
    runFakeSync();
    await delay(null, 20);
  },

  CancelSync: async () => {
    clearSyncTimers();
    status = { ...status, Running: false, Phase: 'idle', CurrentFile: '', Course: '' };
    localEmitter.emit('sync:status', status);
    localEmitter.emit('toast', { Level: 'info', Message: 'Sync cancelled' });
    await delay(null, 20);
  },

  GetSyncStatus: async () => ({ ...status }),

  GetDeadlines: async () => {
    await delay(null, 90);
    return [...DEADLINES]
      .map((d) => ({ ...d }))
      .sort((a, b) => Date.parse(a.DueAt) - Date.parse(b.DueAt));
  },

  GetAnnouncements: async (limit) => {
    await delay(null, 90);
    return ANNOUNCEMENTS.slice(0, limit).map((a) => ({ ...a }));
  },

  MarkAnnouncementRead: async (id) => {
    const a = ANNOUNCEMENTS.find((x) => x.ID === id);
    if (a) a.Read = true;
    await delay(null, 30);
  },

  GetGrades: async () => delay(GRADES.map((g) => ({ ...g }))),

  GetSettings: async () => ({ ...settings, ReminderLadder: [...settings.ReminderLadder], SkipExts: [...settings.SkipExts] }),

  SaveSettings: async (s) => {
    settings = { ...s, ReminderLadder: [...s.ReminderLadder], SkipExts: [...s.SkipExts] };
    await delay(null, 220);
    localEmitter.emit('toast', { Level: 'success', Message: 'Settings saved' });
  },

  TestCanvas: async () => {
    await delay(null, 700);
    if (!settings.CanvasToken) throw new Error('No Canvas token configured');
    return 'Teh Wai Hong';
  },

  GetTelegramStatus: async () => ({ ...telegram }),

  PairTelegram: async () => {
    await delay(null, 2600);
    telegram = { Configured: true, ChatID: '584219307', BotName: '@nuscanvassync_bot' };
    settings.TelegramChatID = telegram.ChatID;
    return telegram.ChatID;
  },

  SendTestTelegram: async () => {
    await delay(null, 600);
    if (!telegram.Configured) throw new Error('Telegram is not paired yet');
    localEmitter.emit('toast', { Level: 'success', Message: 'Test message sent to Telegram' });
  },

  ChooseSyncDir: async () => {
    await delay(null, 400);
    return 'C:\\Users\\tehwa\\Documents\\NUSSync';
  },

  GetStats: async () => {
    await delay(null, 70);
    return {
      Files: ALL_FILES.length,
      Bytes: ALL_FILES.reduce((acc, f) => acc + f.Size, 0),
      Courses: COURSES.filter((c) => c.Enabled).length,
      Deadlines: DEADLINES.filter((d) => !d.Submitted && Date.parse(d.DueAt) > Date.now()).length,
      LastSync: status.LastRun,
    };
  },
// ------------------------------------------------------------ what's new

  GetWhatsNew: async (sinceDays) => {
    await delay(null, 90);
    const days = sinceDays > 0 ? sinceDays : 7;
    const cutoff = now - days * DAY;
    return ALL_FILES.filter((f) => feedEligible(f) && Date.parse(f.ModifiedAt) >= cutoff)
      .sort((a, b) => Date.parse(b.ModifiedAt) - Date.parse(a.ModifiedAt))
      .map(toFeedItem);
  },

  MarkFeedSeen: async () => {
    feedSeenAt = Date.now();
    refreshIsNew();
    await delay(null, 40);
    localEmitter.emit('feed:updated', 0);
  },

  GetUnseenCount: async () => {
    await delay(null, 30);
    return unseenCount();
  },

  // ------------------------------------------------------- deadline detail

  GetDeadlineDetail: async (id) => {
    await delay(null, 260);
    const base = DEADLINES.find((d) => d.ID === id);
    if (!base) throw new Error(`No deadline with id ${id}`);
    const seed = DEADLINE_DETAILS[id];
    if (!seed) {
      // Canvas unreachable and nothing cached: the plain row, not an error.
      return { ...base };
    }
    const attachments = (seed.attachmentNames ?? [])
      .map((n) => fileByName(n))
      .filter(Boolean) as FileNode[];
    const detail: Deadline = {
      ...base,
      Description: seed.Description,
      SubmissionTypes: [...seed.SubmissionTypes],
      Attachments: attachments.map((f) => ({ ...f })),
      Score: seed.Score ?? 0,
      Graded: seed.Graded ?? false,
      Stats: seed.Stats ? { ...seed.Stats } : null,
    };
    // The backend caches the mean onto the matching Grade row opportunistically.
    if (detail.Stats) {
      const g = GRADES.find((x) => x.Title === base.Title);
      if (g && !g.Mean) g.Mean = detail.Stats.Mean;
    }
    return detail;
  },

  // ------------------------------------------------------- window control

  ShowWindow: async () => {},
  HideWindow: async () => {},
  ToggleWindow: async () => {},

  // ----------------------------------------------------------------- study

  GetStudyStatus: async () => {
    await delay(null, 120);
    return { ...STUDY_STATUS, Models: [...(STUDY_STATUS.Models ?? [])] };
  },

  StartOverview: async (fileID, model) => {
    await delay(null, 40);
    return startJob('overview', [fileID], model, () => {
      OVERVIEWS.set(fileID, makeOverview(fileID, model));
      const f = ALL_FILES.find((x) => x.ID === fileID);
      return `Overview ready — ${f?.Name ?? 'file'}`;
    });
  },

  GetOverview: async (fileID) => {
    await delay(null, 60);
    const o = OVERVIEWS.get(fileID);
    return o ? { ...o } : { FileID: 0, Markdown: '', CreatedAt: '', Model: '' };
  },

  StartQuiz: async (fileIDs, model, n) => {
    await delay(null, 40);
    return startJob('quiz', fileIDs, model, () => {
      const q = makeQuiz(fileIDs, model, n);
      QUIZZES.unshift(q);
      return `Quiz ready — ${(q.Questions ?? []).length} questions`;
    });
  },

  GetQuizzes: async (fileID) => {
    await delay(null, 70);
    const list = fileID ? QUIZZES.filter((q) => (q.FileIDs ?? []).includes(fileID)) : QUIZZES;
    return list.map((q) => ({ ...q, Questions: (q.Questions ?? []).map((x) => ({ ...x })) }));
  },

  GetQuiz: async (id) => {
    await delay(null, 60);
    const q = QUIZZES.find((x) => x.ID === id);
    if (!q) throw new Error(`No quiz with id ${id}`);
    return { ...q, Questions: (q.Questions ?? []).map((x) => ({ ...x })) };
  },

  SubmitQuizAttempt: async (a) => {
    await delay(null, 150);
    const quiz = QUIZZES.find((x) => x.ID === a.QuizID);
    const questions = quiz?.Questions ?? [];
    let score = 0;
    for (const q of questions) {
      const given = a.Answers?.[q.ID] ?? '';
      if (!given) continue;
      if (q.Type === 'short') {
        // Self-marked: the UI passes "correct" or "wrong".
        if (given.trim().toLowerCase() === 'correct') score += 1;
      } else if (normaliseMCQ(given, q) === q.Answer.trim().toUpperCase()) {
        score += 1;
      }
    }
    const graded: QuizAttempt = {
      QuizID: a.QuizID,
      Answers: { ...(a.Answers ?? {}) },
      Score: score,
      Total: questions.length,
      TakenAt: new Date().toISOString(),
    };
    ATTEMPTS.unshift(graded);
    return { ...graded };
  },

  GetQuizAttempts: async (quizID) => {
    await delay(null, 60);
    return ATTEMPTS.filter((x) => x.QuizID === quizID)
      .sort((a, b) => Date.parse(b.TakenAt) - Date.parse(a.TakenAt))
      .map((x) => ({ ...x }));
  },

  StartAsk: async (fileIDs, question, model) => {
    await delay(null, 40);
    return startJob('ask', fileIDs, model, () => {
      ASKS.unshift(makeAsk(fileIDs, question));
      return 'Answer ready';
    });
  },

  GetAsks: async (fileID) => {
    await delay(null, 60);
    const list = fileID ? ASKS.filter((a) => a.fileIDs.includes(fileID)) : ASKS;
    return list.map(({ fileIDs: _ids, ...rest }) => ({ ...rest, Citations: [...(rest.Citations ?? [])] }));
  },

  StartFlashcards: async (fileID, model, n) => {
    await delay(null, 40);
    return startJob('flashcards', [fileID], model, () => {
      const cards = makeCards(fileID, n);
      CARDS.push(...cards);
      return `${cards.length} flashcards ready`;
    });
  },

  GetDueFlashcards: async (limit) => {
    await delay(null, 70);
    const t = Date.now();
    return CARDS.filter((c) => Date.parse(c.Due) <= t)
      .sort((a, b) => Date.parse(a.Due) - Date.parse(b.Due))
      .slice(0, limit > 0 ? limit : 50)
      .map((c) => ({ ...c }));
  },

  ReviewFlashcard: async (id, grade) => {
    await delay(null, 50);
    const c = CARDS.find((x) => x.ID === id);
    if (!c) return;
    // SM-2 lite, same shape as the Go side.
    if (grade <= 0) {
      c.Interval = 0;
      c.Ease = Math.max(1.3, c.Ease - 0.2);
      c.Due = new Date(Date.now() + 60_000).toISOString();
      return;
    }
    const factor = grade === 1 ? 1.2 : grade === 2 ? c.Ease : c.Ease * 1.3;
    c.Interval = c.Interval === 0 ? (grade === 1 ? 1 : grade === 2 ? 2 : 4) : Math.round(c.Interval * factor);
    c.Ease = Math.max(1.3, c.Ease + (grade === 1 ? -0.15 : grade === 3 ? 0.15 : 0));
    c.Due = new Date(Date.now() + c.Interval * DAY).toISOString();
  },

  GetStudyJob: async (id) => {
    await delay(null, 30);
    const j = JOBS.find((x) => x.ID === id);
    if (!j) throw new Error(`No job ${id}`);
    return { ...j };
  },

  GetStudyJobs: async () => {
    await delay(null, 50);
    return JOBS.slice(0, 20).map((j) => ({ ...j }));
  },

  CancelStudyJob: async (id) => {
    const timers = jobTimers.get(id) ?? [];
    timers.forEach(clearTimeout);
    jobTimers.delete(id);
    const j = JOBS.find((x) => x.ID === id);
    if (j && (j.Status === 'queued' || j.Status === 'running')) {
      j.Status = 'cancelled';
      j.Progress = 'Cancelled';
      j.FinishedAt = new Date().toISOString();
      emitJob(j);
      localEmitter.emit('toast', { Level: 'info', Message: 'Study job cancelled' });
    }
    await delay(null, 30);
  },

  GetFilePageCount: async (fileID) => {
    await delay(null, 240);
    const f = ALL_FILES.find((x) => x.ID === fileID);
    if (!f || !/\.pdf$/i.test(f.Name)) return 0;
    return Math.max(4, Math.round(f.Size / 78_000));
  },
};
