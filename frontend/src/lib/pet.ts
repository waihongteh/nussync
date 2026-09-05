/**
 * The sidebar pet: a small derived-state creature that reacts to deadlines and
 * sync activity, and slowly levels up as you use the app.
 *
 * Everything here is frontend-only — mood is derived from existing stores and
 * progression lives in localStorage, so the Go side needs no changes.
 *
 * Plain Svelte stores (same convention as stores.ts) so this stays a `.ts`
 * file importable from non-component modules.
 */

import { derived, get, writable } from 'svelte/store';
import { on } from './api';
import { courseByID, deadlines, navigate, recentFiles, stats, syncStatus } from './stores';
import type { Deadline } from './types';
import { countdown, lsGet, lsSet } from './util';

// ------------------------------------------------------------------- types

export type Mood = 'happy' | 'worried' | 'panic' | 'busy' | 'sleepy' | 'proud';

/** Persisted progression. `submitted: null` means "never seeded yet". */
interface PetSave {
  xp: number;
  /** last observed Stats.Files, -1 = not yet observed */
  files: number;
  /** IDs of deadlines already seen as submitted */
  submitted: number[] | null;
  /** YYYY-MM-DD of the last "first open of the day" bonus */
  day: string;
}

interface PetPrefs {
  enabled: boolean;
  name: string;
}

const SAVE_KEY = 'nussync.pet';
const PREFS_KEY = 'nussync.pet.prefs';

const DEFAULT_NAME = 'Nibble';
const IDLE_MS = 10 * 60_000;
const PROUD_MS = 5_000;
const BUBBLE_MS = 4_000;
const AUTO_BUBBLE_MS = 10 * 60_000;
const HOUR = 3_600_000;

// --------------------------------------------------------------- preferences

const prefs0 = lsGet<PetPrefs>(PREFS_KEY, { enabled: true, name: DEFAULT_NAME });

export const petEnabled = writable<boolean>(prefs0.enabled !== false);
export const petName = writable<string>(prefs0.name?.trim() || DEFAULT_NAME);

function savePrefs() {
  lsSet(PREFS_KEY, { enabled: get(petEnabled), name: get(petName) });
}

petEnabled.subscribe(savePrefs);
petName.subscribe(savePrefs);

// --------------------------------------------------------------- progression

const save0 = lsGet<PetSave>(SAVE_KEY, { xp: 0, files: -1, submitted: null, day: '' });

export const xp = writable<number>(Math.max(0, Math.floor(save0.xp ?? 0)));

let mem: PetSave = {
  xp: get(xp),
  files: typeof save0.files === 'number' ? save0.files : -1,
  submitted: Array.isArray(save0.submitted) ? save0.submitted : null,
  day: typeof save0.day === 'string' ? save0.day : '',
};

function persist() {
  mem.xp = get(xp);
  lsSet(SAVE_KEY, mem);
}

/** Level curve: 20 xp for level 2, 80 for level 3, 180 for level 4… */
export function levelFor(points: number): number {
  return Math.floor(Math.sqrt(Math.max(0, points) / 20)) + 1;
}

/** Total xp required to reach a level. */
export function xpForLevel(level: number): number {
  return 20 * Math.pow(Math.max(1, level) - 1, 2);
}

export const level = derived(xp, ($xp) => levelFor($xp));

/** Progress through the current level, 0..1. */
export const levelProgress = derived(xp, ($xp) => {
  const l = levelFor($xp);
  const from = xpForLevel(l);
  const to = xpForLevel(l + 1);
  if (to <= from) return 0;
  return Math.min(1, Math.max(0, ($xp - from) / (to - from)));
});

function award(points: number) {
  if (points <= 0) return;
  const before = levelFor(get(xp));
  xp.update((v) => v + points);
  persist();
  if (levelFor(get(xp)) > before) beProud();
}

/**
 * Award xp from outside this module (the Arcade uses this). Levelling, the
 * proud reaction and persistence all go through the same path as internal
 * awards; `reason` is only for debugging.
 */
export function addXP(points: number, reason = ''): void {
  const n = Math.floor(points);
  if (n <= 0) return;
  if (import.meta.env?.DEV && reason) console.debug(`[pet] +${n} xp (${reason})`);
  award(n);
}

// -------------------------------------------------------------- easter egg

const COMBO_WINDOW_MS = 2_000;
const COMBO_CLICKS = 5;

let pokeTimes: number[] = [];

/**
 * Handle a click on the pet: normally just a quip, but five clicks inside two
 * seconds opens the Arcade.
 */
export function pokePet(): void {
  const now = Date.now();
  pokeTimes = pokeTimes.filter((t) => now - t < COMBO_WINDOW_MS);
  pokeTimes.push(now);
  if (pokeTimes.length >= COMBO_CLICKS) {
    pokeTimes = [];
    say('Fine. Arcade.');
    navigate('arcade');
    return;
  }
  say();
}

// ------------------------------------------------------------------- ticking

/** Coarse clock so mood re-evaluates without a per-second render. */
export const petTick = writable<number>(Date.now());
const lastActivity = writable<number>(Date.now());
const proudUntil = writable<number>(0);

export function beProud() {
  proudUntil.set(Date.now() + PROUD_MS);
  petTick.set(Date.now());
}

/** Nearest deadline that still needs doing (overdue ones count, and come first). */
export const nextDeadline = derived([deadlines, petTick], ([$d]) => {
  let best: Deadline | null = null;
  let bestT = Infinity;
  for (const d of $d) {
    if (d.Submitted) continue;
    const t = Date.parse(d.DueAt);
    if (!isFinite(t)) continue;
    if (t < bestT) {
      bestT = t;
      best = d;
    }
  }
  return best;
});

export const mood = derived(
  [syncStatus, nextDeadline, petTick, lastActivity, proudUntil],
  ([$sync, $next, $now, $active, $proud]): Mood => {
    const now = $now || Date.now();
    if ($proud > now) return 'proud';
    if ($sync.Running) return 'busy';
    if (now - $active > IDLE_MS) return 'sleepy';
    if ($next) {
      const dt = Date.parse($next.DueAt) - now;
      if (dt <= 24 * HOUR) return 'panic';
      if (dt <= 72 * HOUR) return 'worried';
    }
    return 'happy';
  },
);

// -------------------------------------------------------------------- quips

const QUIPS: Record<Mood, string[]> = {
  happy: [
    'All clear. Go touch grass.',
    'Nothing due. Suspicious.',
    "You're ahead. Tell no one.",
    'Zero deadlines. Enjoy it.',
    '{name} is unbothered.',
  ],
  worried: [
    '{title} due {rel}. Just saying.',
    '{rel} until {title}. No pressure.',
    '{title} is {rel} away. Me watching you.',
    'The clock is doing its thing.',
    'Still time. Barely counts as time.',
  ],
  panic: [
    '{title} due {rel}. Me watching you.',
    '{title}. {rel}. Do something.',
    'We are well past chill.',
    "I'd panic, but that's your job.",
    '{rel}. I believe in you, mostly.',
  ],
  busy: [
    'Syncing {n} files. Brb.',
    'Hoovering up Canvas. One sec.',
    'Downloading your problems.',
    'Working. Unusual, I know.',
  ],
  sleepy: ['zzz', 'Wake me for deadlines.', 'Do not perceive me.', 'Resting my eyes. Forever.'],
  proud: [
    'You submitted {title}. Proud of you.',
    'Look at you go.',
    'New notes dropped in {course}.',
    'Fresh files. Nice.',
    '{name} is quietly impressed.',
  ],
};

/** Course code to name in quips: newest synced file's course, else next deadline's. */
function currentCourse(): string {
  const recent = get(recentFiles);
  if (recent.length > 0) {
    const c = get(courseByID).get(recent[0].CourseID);
    if (c?.Code) return c.Code;
  }
  return get(nextDeadline)?.CourseCode ?? '';
}

function shorten(title: string, max = 26): string {
  const t = title.trim();
  return t.length > max ? t.slice(0, max - 1).trimEnd() + '…' : t;
}

/** Values available for `{token}` interpolation; '' means "not available". */
function tokens(): Record<string, string> {
  const next = get(nextDeadline);
  const s = get(syncStatus);
  const total = s.Total > 0 ? s.Total : 0;
  return {
    name: get(petName),
    title: next ? shorten(next.Title) : '',
    rel: next ? relDue(next.DueAt) : '',
    course: currentCourse(),
    n: total > 0 ? String(total) : '',
  };
}

/** "in 3h", "overdue", "in 2d" — deadline-flavoured, shorter than relTime. */
function relDue(dueAt: string): string {
  const c = countdown(dueAt);
  if (!c) return '';
  return c === 'overdue' ? 'overdue' : `in ${c}`;
}

/** Pick a quip for a mood, dropping any whose tokens cannot be filled. */
export function quipFor(m: Mood): string {
  const vars = tokens();
  const usable = QUIPS[m].filter((q) => {
    const needed = q.match(/\{(\w+)\}/g) ?? [];
    return needed.every((tok) => (vars[tok.slice(1, -1)] ?? '') !== '');
  });
  const pool = usable.length > 0 ? usable : QUIPS.happy.filter((q) => !/\{/.test(q));
  const raw = pool[Math.floor(Math.random() * pool.length)] ?? 'Hi.';
  return raw.replace(/\{(\w+)\}/g, (_, k: string) => vars[k] ?? '');
}

// ------------------------------------------------------------------- bubble

export const bubble = writable<string>('');

let bubbleTimer: ReturnType<typeof setTimeout> | undefined;
let lastBubbleAt = 0;

export function say(text?: string) {
  const msg = text ?? quipFor(get(mood));
  lastBubbleAt = Date.now();
  bubble.set(msg);
  if (bubbleTimer) clearTimeout(bubbleTimer);
  bubbleTimer = setTimeout(() => bubble.set(''), BUBBLE_MS);
}

export function hushBubble() {
  if (bubbleTimer) clearTimeout(bubbleTimer);
  bubble.set('');
}

// --------------------------------------------------------------------- init

let started = false;

/** Wire activity tracking, xp accounting and the occasional unprompted quip. */
export function initPet(): () => void {
  if (started || typeof window === 'undefined') return () => {};
  started = true;

  // +5 for the first open of each calendar day.
  const today = new Date().toISOString().slice(0, 10);
  if (mem.day !== today) {
    mem.day = today;
    award(5);
    persist();
  }

  let lastMove = 0;
  const poke = () => {
    const now = Date.now();
    if (now - lastMove < 1_000) return;
    lastMove = now;
    lastActivity.set(now);
  };
  window.addEventListener('mousemove', poke, { passive: true });
  window.addEventListener('keydown', poke, { passive: true });
  window.addEventListener('pointerdown', poke, { passive: true });

  // +1 xp per newly synced file (Stats.Files diff between observations).
  const offStats = stats.subscribe(($s) => {
    if (!$s) return;
    const n = $s.Files ?? 0;
    if (mem.files < 0) {
      mem.files = n;
      persist();
      return;
    }
    if (n > mem.files) {
      const gained = n - mem.files;
      mem.files = n;
      award(gained);
    } else if (n < mem.files) {
      mem.files = n;
      persist();
    }
  });

  // +25 per deadline observed flipping unsubmitted -> submitted.
  const offDeadlines = deadlines.subscribe(($d) => {
    if ($d.length === 0) return;
    const done = $d.filter((x) => x.Submitted).map((x) => x.ID);
    if (mem.submitted === null) {
      mem.submitted = done;
      persist();
      return;
    }
    const known = new Set(mem.submitted);
    const fresh = done.filter((id) => !known.has(id));
    if (fresh.length > 0) {
      mem.submitted = [...mem.submitted, ...fresh];
      award(25 * fresh.length);
      beProud();
    }
  });

  const offSync = on('sync:done', () => beProud());

  const tick = setInterval(() => petTick.set(Date.now()), 20_000);

  // At most one unprompted quip every 10 minutes, and never while asleep.
  const chatter = setInterval(() => {
    if (!get(petEnabled)) return;
    if (get(bubble)) return;
    if (Date.now() - lastBubbleAt < AUTO_BUBBLE_MS) return;
    if (get(mood) === 'sleepy') return;
    if (Math.random() < 0.25) say();
  }, 60_000);

  return () => {
    started = false;
    window.removeEventListener('mousemove', poke);
    window.removeEventListener('keydown', poke);
    window.removeEventListener('pointerdown', poke);
    offStats();
    offDeadlines();
    try {
      offSync();
    } catch {
      /* ignore */
    }
    clearInterval(tick);
    clearInterval(chatter);
    if (bubbleTimer) clearTimeout(bubbleTimer);
  };
}
