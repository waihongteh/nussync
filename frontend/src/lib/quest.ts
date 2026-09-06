/**
 * Quest: the progression layer on top of the pet.
 *
 * Everything here is frontend-local and persisted in `localStorage` under
 * `nussync.quest` — the Go side is untouched. XP itself still lives in
 * `pet.ts` (one currency, one level curve); this module listens to every award
 * through `onXP` and layers a daily goal, a streak, derived RPG stats, gear /
 * skin / title unlocks and the boss ladder's progress on top.
 *
 * Plain Svelte stores, same convention as stores.ts, so it stays importable
 * from non-component modules.
 */

import { derived, get, writable } from 'svelte/store';
import { api, on } from './api';
import { addXP, onXP, registerQuipSource } from './pet';
import { courses, deadlines } from './stores';
import type { Deadline, FileNode, Question } from './types';
import { lsGet, lsSet } from './util';

// ------------------------------------------------------------------- types

export type Goal = 20 | 50 | 100;

export const GOALS: Goal[] = [20, 50, 100];

/** Cumulative counters the RPG stats are derived from. */
export interface Counters {
  /** quiz/boss questions answered correctly */
  correct: number;
  /** flashcards reviewed */
  cards: number;
  /** overviews generated */
  overviews: number;
  /** files previewed for long enough to count */
  previews: number;
  /** best Quiz Rush streak ever */
  rushStreak: number;
  /** papers moved to "done" */
  papers: number;
  /** deadline bosses defeated */
  dlBosses: number;
}

export interface QuestSave {
  v: number;
  goal: Goal;
  /** YYYY-MM-DD -> xp earned that day (last 30 days kept) */
  days: Record<string, number>;
  /** completed-day streak; today is added live on read */
  streak: number;
  /** last day the roll-over check ran */
  seen: string;
  /** streak freezes in the bag */
  freezes: number;
  /** streak length at which the last freeze was granted */
  freezeAt: number;
  counters: Counters;
  /** YYYY-MM-DD + count for the daily preview-xp cap */
  previewDay: string;
  previewCount: number;
  /** highest boss tier beaten (0 = none) */
  tier: number;
  /** unlocked loot ids (skins + titles) */
  loot: string[];
  /** equipped skin id, '' = the app accent */
  skin: string;
  /** equipped title id, '' = none */
  title: string;
  /** deadline id -> remaining boss HP */
  dlHP: Record<string, number>;
  /** deadline ids whose boss was beaten in a fight */
  dlBeaten: number[];
  /** deadline ids already awarded the +50 "Submitted" bounty */
  dlDone: number[];
  /** paper ids already counted as done */
  paperIDs: string[];
}

const KEY = 'nussync.quest';

const EMPTY_COUNTERS: Counters = {
  correct: 0,
  cards: 0,
  overviews: 0,
  previews: 0,
  rushStreak: 0,
  papers: 0,
  dlBosses: 0,
};

const DEFAULT: QuestSave = {
  v: 1,
  goal: 50,
  days: {},
  streak: 0,
  seen: '',
  freezes: 0,
  freezeAt: 0,
  counters: { ...EMPTY_COUNTERS },
  previewDay: '',
  previewCount: 0,
  tier: 0,
  loot: [],
  skin: '',
  title: '',
  dlHP: {},
  dlBeaten: [],
  dlDone: [],
  paperIDs: [],
};

// ------------------------------------------------------------------ dates

/** Local calendar day, YYYY-MM-DD (not UTC — a streak is a human thing). */
export function dayKey(d: Date = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

function shiftDay(key: string, delta: number): string {
  const [y, m, d] = key.split('-').map(Number);
  const dt = new Date(y, (m ?? 1) - 1, d ?? 1);
  dt.setDate(dt.getDate() + delta);
  return dayKey(dt);
}

/** The last 7 day keys, oldest first. */
export function weekKeys(now: Date = new Date()): string[] {
  const today = dayKey(now);
  const out: string[] = [];
  for (let i = 6; i >= 0; i--) out.push(shiftDay(today, -i));
  return out;
}

// ------------------------------------------------------------------- store

function sanitize(raw: Partial<QuestSave> | null | undefined): QuestSave {
  const s = { ...DEFAULT, ...(raw ?? {}) };
  const goal = GOALS.includes(s.goal as Goal) ? (s.goal as Goal) : DEFAULT.goal;
  return {
    ...DEFAULT,
    ...s,
    goal,
    days: typeof s.days === 'object' && s.days ? { ...s.days } : {},
    counters: { ...EMPTY_COUNTERS, ...(s.counters ?? {}) },
    loot: Array.isArray(s.loot) ? [...s.loot] : [],
    dlHP: typeof s.dlHP === 'object' && s.dlHP ? { ...s.dlHP } : {},
    dlBeaten: Array.isArray(s.dlBeaten) ? [...s.dlBeaten] : [],
    dlDone: Array.isArray(s.dlDone) ? [...s.dlDone] : [],
    paperIDs: Array.isArray(s.paperIDs) ? [...s.paperIDs] : [],
    tier: Math.max(0, Math.min(10, Math.floor(s.tier ?? 0))),
    freezes: Math.max(0, Math.min(2, Math.floor(s.freezes ?? 0))),
  };
}

export const quest = writable<QuestSave>(sanitize(lsGet<QuestSave>(KEY, DEFAULT)));

quest.subscribe((s) => lsSet(KEY, s));

function edit(fn: (s: QuestSave) => void): void {
  quest.update((s) => {
    const next: QuestSave = {
      ...s,
      days: { ...s.days },
      counters: { ...s.counters },
      loot: [...s.loot],
      dlHP: { ...s.dlHP },
      dlBeaten: [...s.dlBeaten],
      dlDone: [...s.dlDone],
      paperIDs: [...s.paperIDs],
    };
    fn(next);
    return next;
  });
}

// -------------------------------------------------------- streak roll-over

/** Keep the day log bounded — 30 days is plenty for a 7-day recap. */
function prune(days: Record<string, number>): Record<string, number> {
  const keys = Object.keys(days).sort();
  if (keys.length <= 30) return days;
  const out: Record<string, number> = {};
  for (const k of keys.slice(-30)) out[k] = days[k];
  return out;
}

/**
 * Settle every fully elapsed day since the last check: a day that hit the goal
 * extends the streak, a missed day burns a freeze if there is one and breaks
 * the streak otherwise. Today is never settled here — it is added live by the
 * `streak` derived store, so the number cannot double-count.
 */
export function rollover(): void {
  const today = dayKey();
  edit((s) => {
    if (!s.seen) {
      s.seen = today;
      return;
    }
    if (s.seen >= today) return;
    let d = s.seen;
    let guard = 0;
    while (d < today && guard++ < 400) {
      const got = s.days[d] ?? 0;
      if (got >= s.goal) s.streak += 1;
      else if (s.freezes > 0) s.freezes -= 1;
      else s.streak = 0;
      d = shiftDay(d, 1);
    }
    s.seen = today;
    s.days = prune(s.days);
    grantFreezes(s);
  });
}

/** One freeze per whole 7 days of streak, capped at 2 in the bag. */
function grantFreezes(s: QuestSave): void {
  const live = s.streak + ((s.days[dayKey()] ?? 0) >= s.goal ? 1 : 0);
  if (live < 7) return;
  if (Math.floor(live / 7) <= Math.floor(s.freezeAt / 7)) return;
  s.freezeAt = live;
  s.freezes = Math.min(2, s.freezes + 1);
}

// ------------------------------------------------------------- derivations

export const goal = derived(quest, ($q) => $q.goal);
export const todayXP = derived(quest, ($q) => $q.days[dayKey()] ?? 0);
export const goalMet = derived(quest, ($q) => ($q.days[dayKey()] ?? 0) >= $q.goal);
export const goalProgress = derived(quest, ($q) =>
  Math.min(1, ($q.days[dayKey()] ?? 0) / Math.max(1, $q.goal)),
);

/** Streak including today, so the ring and the number agree. */
export const streak = derived(quest, ($q) => $q.streak + (($q.days[dayKey()] ?? 0) >= $q.goal ? 1 : 0));
export const freezes = derived(quest, ($q) => $q.freezes);

/** Last 7 days as `{ key, label, xp }`, oldest first. */
export const week = derived(quest, ($q) =>
  weekKeys().map((k) => ({
    key: k,
    label: new Date(`${k}T12:00:00`).toLocaleDateString(undefined, { weekday: 'narrow' }),
    xp: $q.days[k] ?? 0,
  })),
);

/** Stat curve: 1 + floor(sqrt(n)) — fast early, flat later. */
export function statOf(n: number): number {
  return 1 + Math.floor(Math.sqrt(Math.max(0, n)));
}

export interface PetStats {
  atk: number;
  def: number;
  hp: number;
  spd: number;
}

export const petStats = derived(quest, ($q): PetStats => {
  const c = $q.counters;
  return {
    atk: statOf(c.correct),
    def: statOf(c.cards),
    hp: statOf(c.overviews + c.previews),
    spd: statOf(c.rushStreak),
  };
});

// ---------------------------------------------------------------- cosmetics

export interface Skin {
  id: string;
  name: string;
  accent: string;
}

/** Loot skins. `accent` replaces --accent on the sprite only. */
export const SKINS: Skin[] = [
  { id: 'moss', name: 'Moss', accent: '#3f9e6a' },
  { id: 'ocean', name: 'Ocean', accent: '#3a7fd4' },
  { id: 'ember', name: 'Ember', accent: '#d4663a' },
  { id: 'grape', name: 'Grape', accent: '#9b5bd6' },
  { id: 'gold', name: 'Gold', accent: '#c99a2e' },
];

export interface GearItem {
  id: 'sword' | 'steel' | 'scarf' | 'shield' | 'badge' | 'sneakers';
  name: string;
  stat: keyof PetStats;
  at: number;
}

/** Accessories, unlocked purely by stat thresholds. */
export const GEAR: GearItem[] = [
  { id: 'sword', name: 'Wooden sword', stat: 'atk', at: 5 },
  { id: 'steel', name: 'Steel sword', stat: 'atk', at: 10 },
  { id: 'scarf', name: 'Scarf', stat: 'def', at: 5 },
  { id: 'shield', name: 'Shield', stat: 'def', at: 10 },
  { id: 'badge', name: 'Heart badge', stat: 'hp', at: 8 },
  { id: 'sneakers', name: 'Sneakers', stat: 'spd', at: 6 },
];

export interface GearSet {
  sword: boolean;
  steel: boolean;
  scarf: boolean;
  shield: boolean;
  badge: boolean;
  sneakers: boolean;
}

export const gear = derived(petStats, ($s): GearSet => {
  const has = (g: GearItem) => $s[g.stat] >= g.at;
  return {
    sword: has(GEAR[0]),
    steel: has(GEAR[1]),
    scarf: has(GEAR[2]),
    shield: has(GEAR[3]),
    badge: has(GEAR[4]),
    sneakers: has(GEAR[5]),
  };
});

export interface TitleDef {
  id: string;
  name: string;
  how: string;
}

export const TITLES: TitleDef[] = [
  { id: 'freshman', name: 'Freshman', how: 'Show up once.' },
  { id: 'grinder', name: 'Grinder', how: 'Hold a 7-day streak.' },
  { id: 'unlearner', name: 'Unlearner', how: 'Mark 10 papers done.' },
  { id: 'slayer', name: 'Deadline Slayer', how: 'Defeat 5 deadline bosses.' },
  { id: 'finals', name: 'Finals Survivor', how: 'Beat tier 10.' },
];

/** Titles are milestone-derived, plus anything the ladder handed out as loot. */
export const unlockedTitles = derived([quest, streak], ([$q, $streak]) => {
  const ids = new Set<string>(['freshman']);
  if ($streak >= 7 || $q.loot.includes('title:grinder')) ids.add('grinder');
  if ($q.counters.papers >= 10) ids.add('unlearner');
  if ($q.counters.dlBosses >= 5) ids.add('slayer');
  if ($q.tier >= 10 || $q.loot.includes('title:finals')) ids.add('finals');
  return TITLES.filter((t) => ids.has(t.id));
});

export const unlockedSkins = derived(quest, ($q) => SKINS.filter((s) => $q.loot.includes(`skin:${s.id}`)));

/** Hex accent of the equipped skin, or '' to keep the app accent. */
export const skinAccent = derived(quest, ($q) => SKINS.find((s) => s.id === $q.skin)?.accent ?? '');

export const equippedTitle = derived([quest, unlockedTitles], ([$q, $titles]) => {
  const t = $titles.find((x) => x.id === $q.title);
  return t?.name ?? '';
});

export function equipSkin(id: string): void {
  edit((s) => {
    s.skin = s.skin === id ? '' : id;
  });
}

export function equipTitle(id: string): void {
  edit((s) => {
    s.title = s.title === id ? '' : id;
  });
}

export function setGoal(g: Goal): void {
  edit((s) => {
    s.goal = g;
  });
}

// ------------------------------------------------------------- xp plumbing

/** Log an award against today's total. Called for every xp source. */
function logXP(points: number): void {
  if (points <= 0) return;
  const today = dayKey();
  edit((s) => {
    s.days = prune({ ...s.days, [today]: (s.days[today] ?? 0) + points });
    grantFreezes(s);
  });
}

function bump(key: keyof Counters, by = 1): void {
  edit((s) => {
    s.counters[key] = Math.max(0, s.counters[key] + by);
  });
}

// ------------------------------------------------------------- xp sources

/** A quiz question answered correctly; under 5s is worth a small bonus. */
export function recordCorrect(elapsedMs: number): void {
  bump('correct');
  addXP(elapsedMs < 5_000 ? 5 : 3, 'quiz correct');
}

/**
 * A boss-fight question answered correctly. The xp for a fight is paid out at
 * the end (win or lose), so this only feeds ATK — otherwise a fight would be
 * paid for twice.
 */
export function recordBossHit(): void {
  bump('correct');
}

export function recordQuizDone(): void {
  addXP(15, 'quiz complete');
}

/** Flashcard graded: 2 for Good/Easy (grade >= 2), 1 otherwise. */
export function recordFlashcard(grade: number): void {
  bump('cards');
  addXP(grade >= 2 ? 2 : 1, 'flashcard');
}

export function recordOverview(): void {
  bump('overviews');
  addXP(10, 'overview');
}

/** A file kept open long enough to count as read. Capped at 10 a day. */
export function recordPreview(): void {
  const today = dayKey();
  let counted = false;
  edit((s) => {
    if (s.previewDay !== today) {
      s.previewDay = today;
      s.previewCount = 0;
    }
    if (s.previewCount >= 10) return;
    s.previewCount += 1;
    s.counters.previews += 1;
    counted = true;
  });
  if (counted) addXP(2, 'preview');
}

/** A finished Quiz Rush run: score/10 xp, and SPD tracks the best streak. */
export function recordRush(score: number, bestStreak: number): void {
  edit((s) => {
    s.counters.rushStreak = Math.max(s.counters.rushStreak, bestStreak);
  });
  addXP(Math.max(1, Math.round(score / 10)), 'rush');
}

// -------------------------------------------------------------- boss ladder

export interface TierDef {
  tier: number;
  name: string;
  questions: number;
  /** per-question timer, ms */
  timer: number;
  atk: number;
  /** tiers at or above this mix every course */
  mixed: boolean;
}

export function tierDef(t: number): TierDef {
  const tier = Math.max(1, Math.min(10, Math.floor(t)));
  if (tier === 10) {
    return { tier, name: 'Finals', questions: 20, timer: 6_000, atk: 11, mixed: true };
  }
  return {
    tier,
    name: BOSS_NAMES[tier - 1],
    questions: Math.round(5 + ((tier - 1) * 10) / 9),
    timer: 15_000 - (tier - 1) * 1_000,
    atk: 1 + tier,
    mixed: tier >= 6,
  };
}

export const BOSS_NAMES = [
  'Lecture Slime',
  'Tutorial Gremlin',
  'Reading Week Wraith',
  'Midterm Maw',
  'Lab Report Lurker',
  'Group Project Hydra',
  'Recess Week Revenant',
  'Consultation Cyclops',
  'Revision Behemoth',
  'Finals',
];

/** Highest tier beaten; tier N+1 is the one that is unlocked. */
export const tierBeaten = derived(quest, ($q) => $q.tier);

export function tierUnlocked(t: number, beaten: number): boolean {
  return t <= beaten + 1;
}

/** Record a ladder win: xp, tier progress and the tier's loot drop. */
export function winTier(t: number): string[] {
  const dropped: string[] = [];
  edit((s) => {
    if (t > s.tier) s.tier = Math.min(10, t);
    for (const id of TIER_LOOT[t] ?? []) {
      if (!s.loot.includes(id)) {
        s.loot.push(id);
        dropped.push(id);
      }
    }
  });
  addXP(20 + t * 6, `boss ${t}`);
  return dropped;
}

export function loseTier(t: number): void {
  addXP(Math.max(2, Math.round(t / 2) + 2), `boss ${t} attempt`);
}

/** What each tier drops. Accessories come from stats, so these are cosmetics. */
const TIER_LOOT: Record<number, string[]> = {
  1: ['skin:moss'],
  3: ['skin:ocean'],
  4: ['title:grinder'],
  5: ['skin:ember'],
  7: ['skin:grape'],
  10: ['skin:gold', 'title:finals'],
};

export function lootLabel(id: string): string {
  const [kind, key] = id.split(':');
  if (kind === 'skin') return `${SKINS.find((s) => s.id === key)?.name ?? key} skin`;
  if (kind === 'title') return `“${TITLES.find((t) => t.id === key)?.name ?? key}” title`;
  return id;
}

// ------------------------------------------------------------ deadline boss

export interface DeadlineBoss {
  id: number;
  title: string;
  courseID: number;
  courseCode: string;
  dueAt: string;
  /** days left, min 1 — also the boss's full HP */
  maxHP: number;
  hp: number;
  beaten: boolean;
  submitted: boolean;
  overdue: boolean;
}

const DAY_MS = 86_400_000;

function daysLeft(dueAt: string, now = Date.now()): number {
  const t = Date.parse(dueAt);
  if (!isFinite(t)) return 1;
  return Math.max(1, Math.ceil((t - now) / DAY_MS));
}

/** One boss per deadline. Submitted ones stay in the list, marked Defeated. */
export const deadlineBosses = derived([deadlines, quest], ([$d, $q]): DeadlineBoss[] =>
  $d.map((d: Deadline) => {
    const max = daysLeft(d.DueAt);
    const stored = $q.dlHP[String(d.ID)];
    const beaten = $q.dlBeaten.includes(d.ID) || d.Submitted;
    return {
      id: d.ID,
      title: d.Title,
      courseID: d.CourseID,
      courseCode: d.CourseCode,
      dueAt: d.DueAt,
      maxHP: max,
      hp: beaten ? 0 : typeof stored === 'number' ? Math.max(0, Math.min(max, stored)) : max,
      beaten,
      submitted: d.Submitted,
      overdue: Date.parse(d.DueAt) < Date.now(),
    };
  }),
);

/**
 * Apply one attack round's damage. Returns true when this blow finished the
 * boss off (and only then, once, is loot handed out).
 */
export function damageDeadlineBoss(id: number, damage: number, maxHP: number, beforeDue = true): boolean {
  let killed = false;
  edit((s) => {
    const key = String(id);
    const cur = typeof s.dlHP[key] === 'number' ? s.dlHP[key] : maxHP;
    const next = Math.max(0, cur - Math.max(0, damage));
    s.dlHP[key] = next;
    if (next === 0 && !s.dlBeaten.includes(id)) {
      s.dlBeaten.push(id);
      s.counters.dlBosses += 1;
      killed = true;
      // Only a kill landed before the due date is worth loot.
      if (beforeDue) {
        const missing = SKINS.map((sk) => `skin:${sk.id}`).find((sk) => !s.loot.includes(sk));
        if (missing) s.loot.push(missing);
      }
    }
  });
  if (killed) addXP(30, 'deadline boss');
  return killed;
}

/** Loot dropped by the most recent deadline-boss kill, for the toast. */
export function lastDeadlineLoot(): string {
  const s = get(quest);
  const owned = s.loot.filter((x) => x.startsWith('skin:'));
  return owned.length ? lootLabel(owned[owned.length - 1]) : '';
}

// ------------------------------------------------------------- quiz bank

export interface BankCard {
  q: Question;
  courseID: number;
  courseCode: string;
}

/**
 * Every generated quiz question, attributed to a course via the file tree —
 * the same trick Quiz Rush uses, since a Quiz only carries file ids.
 */
export async function loadQuizBank(): Promise<BankCard[]> {
  const list = get(courses);
  const [quizzes, trees] = await Promise.all([
    api.getQuizzes(0),
    Promise.all(list.map((c) => api.getTree(c.ID).catch(() => [] as FileNode[]))),
  ]);
  const courseOf = new Map<number, number>();
  list.forEach((c, i) => {
    const walk = (nodes: FileNode[]) => {
      for (const n of nodes) {
        if (n.IsDir) walk(n.Children ?? []);
        else courseOf.set(n.ID, c.ID);
      }
    };
    walk(trees[i] ?? []);
  });

  const out: BankCard[] = [];
  for (const quiz of quizzes ?? []) {
    const courseID = (quiz.FileIDs ?? []).map((id) => courseOf.get(id)).find((x) => x !== undefined) ?? 0;
    const code = list.find((c) => c.ID === courseID)?.Code ?? 'Unfiled';
    for (const q of quiz.Questions ?? []) {
      if (q.Type !== 'mcq') continue;
      if ((q.Options ?? []).length < 2) continue;
      out.push({ q, courseID, courseCode: code });
    }
  }
  return out;
}

export function shuffle<T>(list: T[]): T[] {
  const a = [...list];
  for (let i = a.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [a[i], a[j]] = [a[j], a[i]];
  }
  return a;
}

// --------------------------------------------------------------------- init

let started = false;

/** Wire xp logging, the daily roll-over, and paper/deadline watchers. */
export function initQuest(): () => void {
  if (started || typeof window === 'undefined') return () => {};
  started = true;

  rollover();
  const offXP = onXP((points) => logXP(points));

  // Pet quips gain progression lines without pet.ts knowing this module exists.
  const offQuips = registerQuipSource(() => {
    const s = get(quest);
    const today = s.days[dayKey()] ?? 0;
    const live = get(streak);
    const lines: string[] = [];
    if (live > 1) lines.push(`Streak ${live}. Don't break it.`);
    if (today < s.goal) lines.push(`Daily goal ${today}/${s.goal}.`);
    else lines.push(`Goal cleared: ${today}/${s.goal}. Showoff.`);
    if (s.tier < 10) lines.push(`Boss ${s.tier + 1} still standing.`);
    if (s.freezes > 0) lines.push(`${s.freezes} streak freeze in the bag.`);
    return lines;
  });

  // +50, once, when a deadline boss's assignment flips to Submitted.
  const offDeadlines = deadlines.subscribe(($d) => {
    if ($d.length === 0) return;
    const fresh = $d.filter((d) => d.Submitted).map((d) => d.ID);
    let gained = 0;
    edit((s) => {
      for (const id of fresh) {
        if (s.dlDone.includes(id)) continue;
        s.dlDone.push(id);
        // Only count it as a boss kill once; a fight may already have done so.
        if (!s.dlBeaten.includes(id)) {
          s.dlBeaten.push(id);
          s.counters.dlBosses += 1;
        }
        gained += 50;
      }
    });
    if (gained > 0) addXP(gained, 'deadline boss submitted');
  });

  // Papers cannot be observed from a store, so poll the "done" shelf on the
  // same event the Papers view refreshes on (plus once at start).
  const scanPapers = async () => {
    try {
      const done = (await api.getLibrary('done')) ?? [];
      const ids = done.map((p) => p.ID).filter(Boolean);
      let gained = 0;
      edit((s) => {
        for (const id of ids) {
          if (s.paperIDs.includes(id)) continue;
          s.paperIDs.push(id);
          s.counters.papers += 1;
          gained += 20;
        }
      });
      if (gained > 0) addXP(gained, 'paper done');
    } catch {
      /* the library is optional; never let it break the app */
    }
  };
  void scanPapers();
  const offPapers = on('papers:updated', () => void scanPapers());

  // Midnight rolls over while the app is open.
  const tick = setInterval(rollover, 60_000);

  return () => {
    started = false;
    offXP();
    offQuips();
    offDeadlines();
    try {
      offPapers();
    } catch {
      /* ignore */
    }
    clearInterval(tick);
  };
}
