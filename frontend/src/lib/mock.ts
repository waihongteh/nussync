/**
 * Mock backend used when the app runs in a plain browser (`npm run dev`)
 * rather than inside the Wails shell. Mirrors AppAPI exactly and emits the
 * same events the Go backend does, so every view can be developed and
 * visually verified without the desktop binary.
 */

import type {
  Announcement,
  AppAPI,
  Course,
  Deadline,
  FileNode,
  Grade,
  SearchHit,
  Settings,
  Stats,
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

const DEADLINES: Deadline[] = [
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

const GRADES: Grade[] = [
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
  LaunchAtLogin: false,
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
};
