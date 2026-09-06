/**
 * Application state. Plain Svelte stores (not runes) so this can live in a
 * `.ts` file and be imported from anywhere, including non-component modules.
 */

import { derived, get, writable } from 'svelte/store';
import { api, emitLocal, errMsg, on } from './api';
import type {
  Announcement,
  Course,
  Deadline,
  FileNode,
  Grade,
  Settings,
  Stats,
  SyncStatus,
  ToastPayload,
} from './types';
import { lsGet, lsSet } from './util';

// ------------------------------------------------------------------ routing

export const ROUTES = [
  'home',
  'files',
  'whatsnew',
  'deadlines',
  'announcements',
  'grades',
  'study',
  'papers',
  'arcade',
  'quest',
  'settings',
] as const;
export type Route = (typeof ROUTES)[number];

export const route = writable<Route>('home');

export function navigate(to: Route) {
  route.set(to);
}

/**
 * Views that cannot import the router (self-contained arcade games, for
 * instance) ask for a route change by dispatching
 * `new CustomEvent('nussync:navigate', { detail: { view: 'study' } })` on
 * window. Wired once, from wireEvents.
 */
const NAV_EVENT = 'nussync:navigate';

// ------------------------------------------------------------------- toasts

export interface Toast {
  id: number;
  level: 'info' | 'success' | 'error';
  message: string;
}

let toastSeq = 0;
export const toasts = writable<Toast[]>([]);

export function toast(message: string, level: Toast['level'] = 'info', ttl = 4200) {
  const id = ++toastSeq;
  toasts.update((list) => [...list, { id, level, message }].slice(-5));
  setTimeout(() => dismissToast(id), ttl);
  return id;
}

export function dismissToast(id: number) {
  toasts.update((list) => list.filter((t) => t.id !== id));
}

// -------------------------------------------------------------------- theme

const THEME_KEY = 'nussync.theme';

/**
 * Whether the user has already picked a theme on this machine. Captured before
 * `initTheme` subscribes (which writes the key back immediately), so it stays a
 * truthful "has an explicit local choice" signal — `loadSettings` uses it to
 * avoid clobbering that choice with the backend's default.
 */
const hasLocalTheme = (() => {
  try {
    return localStorage.getItem(THEME_KEY) !== null;
  } catch {
    return false;
  }
})();

export const theme = writable<'system' | 'light' | 'dark'>(lsGet(THEME_KEY, 'system'));
export const resolvedTheme = writable<'light' | 'dark'>('light');

let mql: MediaQueryList | undefined;

function computeTheme() {
  const t = get(theme);
  const dark = t === 'dark' || (t === 'system' && !!mql?.matches);
  resolvedTheme.set(dark ? 'dark' : 'light');
  if (typeof document !== 'undefined') {
    document.documentElement.setAttribute('data-theme', dark ? 'dark' : 'light');
    document.documentElement.style.colorScheme = dark ? 'dark' : 'light';
  }
}

let themeWired = false;

/** Idempotent — safe to call from both the bootstrap and the root component. */
export function initTheme() {
  if (themeWired) return;
  themeWired = true;
  if (typeof window !== 'undefined' && window.matchMedia) {
    mql = window.matchMedia('(prefers-color-scheme: dark)');
    // `change` fires whenever the OS flips, so "system" tracks it live.
    mql.addEventListener('change', computeTheme);
  }
  theme.subscribe((t) => {
    lsSet(THEME_KEY, t);
    computeTheme();
  });
}

// ------------------------------------------------------------------ domain

export const courses = writable<Course[]>([]);
export const deadlines = writable<Deadline[]>([]);
export const announcements = writable<Announcement[]>([]);
export const grades = writable<Grade[]>([]);
export const recentFiles = writable<FileNode[]>([]);
export const stats = writable<Stats | null>(null);
export const settings = writable<Settings | null>(null);

/**
 * Files changed since the What's-new feed was last marked seen. Refreshed on
 * `feed:updated` (emitted after every sync and after MarkFeedSeen) so the nav
 * badge stays honest without polling.
 */
export const unseenCount = writable<number>(0);

export const syncStatus = writable<SyncStatus>({
  Running: false,
  Phase: 'idle',
  Course: '',
  Done: 0,
  Total: 0,
  CurrentFile: '',
  LastRun: '',
  LastError: '',
  BytesDownloaded: 0,
});

/** Course filter applied to the Files view (0 = all courses). */
export const selectedCourseID = writable<number>(0);

/** Command palette visibility. */
export const paletteOpen = writable(false);

// -------------------------------------------------------- chat / context

/**
 * What the Claude chat panel should treat as context. Set by Files (selected
 * file), Study (primary selection) and Papers (selected paper). Either id may
 * be zero/empty — "no context" is a valid state and chat still works.
 */
export interface ChatContext {
  fileID: number;
  paperID: string;
  name: string;
}

export const currentContext = writable<ChatContext>({ fileID: 0, paperID: '', name: '' });

export function setContext(fileID: number, paperID: string, name: string) {
  currentContext.set({ fileID, paperID, name });
}

/** Right-side chat panel visibility (Ctrl+J, top-bar button, Esc to close). */
export const chatOpen = writable(false);

/** Open the panel on a specific context in one step. */
export function openChat(ctx?: Partial<ChatContext>) {
  if (ctx) {
    currentContext.set({ fileID: ctx.fileID ?? 0, paperID: ctx.paperID ?? '', name: ctx.name ?? '' });
  }
  chatOpen.set(true);
}

/**
 * File id the Study view should preselect the next time it mounts. Papers sets
 * it before navigating to `study`; Study consumes and clears it.
 */
export const studyPreselect = writable<number>(0);

/** Jump to Study with a downloaded paper's file already selected. */
export function studyFile(fileID: number) {
  studyPreselect.set(fileID);
  navigate('study');
}

/** Every file in the tree of the currently loaded course(s), flattened. */
export const flatFiles = writable<FileNode[]>([]);

export const courseByID = derived(courses, ($courses) => {
  const map = new Map<number, Course>();
  for (const c of $courses) map.set(c.ID, c);
  return map;
});

export const unreadAnnouncements = derived(announcements, ($a) => $a.filter((x) => !x.Read).length);

export const upcomingCount = derived(deadlines, ($d) => {
  const now = Date.now();
  return $d.filter((x) => !x.Submitted && Date.parse(x.DueAt) > now).length;
});

// ------------------------------------------------------------------ loaders

async function guard<T>(label: string, fn: () => Promise<T>, set?: (v: T) => void) {
  try {
    const v = await fn();
    set?.(v);
    return v;
  } catch (err) {
    console.error(`[stores] ${label} failed`, err);
    toast(`${label} failed: ${errMsg(err)}`, 'error');
    return undefined;
  }
}

export const loadCourses = () => guard('Load courses', () => api.getCourses(), (v) => courses.set(v ?? []));
export const loadDeadlines = () => guard('Load deadlines', () => api.getDeadlines(), (v) => deadlines.set(v ?? []));
export const loadAnnouncements = () =>
  guard('Load announcements', () => api.getAnnouncements(50), (v) => announcements.set(v ?? []));
export const loadGrades = () => guard('Load grades', () => api.getGrades(), (v) => grades.set(v ?? []));
export const loadRecent = () => guard('Load recent files', () => api.getRecentFiles(10), (v) => recentFiles.set(v ?? []));
export const loadStats = () => guard('Load stats', () => api.getStats(), (v) => stats.set(v ?? null));
export const loadUnseenCount = () =>
  guard('Load unseen count', () => api.getUnseenCount(), (v) => unseenCount.set(v ?? 0));
export const loadSettings = () =>
  guard('Load settings', () => api.getSettings(), (v) => {
    if (!v) return;
    settings.set(v);
    // Only adopt the backend's theme when this machine has no explicit choice;
    // otherwise a load would stomp the toggle the user just used.
    if (!hasLocalTheme && (v.Theme === 'light' || v.Theme === 'dark' || v.Theme === 'system')) {
      theme.set(v.Theme);
    }
  });

export async function loadAll() {
  await Promise.all([
    loadCourses(),
    loadDeadlines(),
    loadAnnouncements(),
    loadGrades(),
    loadRecent(),
    loadStats(),
    loadSettings(),
    loadUnseenCount(),
  ]);
  await guard('Read sync status', () => api.getSyncStatus(), (v) => v && syncStatus.set(v));
}

export async function syncNow() {
  if (get(syncStatus).Running) return;
  try {
    await api.syncNow();
  } catch (err) {
    toast(`Sync failed to start: ${errMsg(err)}`, 'error');
  }
}

export async function cancelSync() {
  try {
    await api.cancelSync();
  } catch (err) {
    toast(errMsg(err), 'error');
  }
}

/** Open a local file, or nudge the user to sync when it is not downloaded. */
export async function openFileNode(f: FileNode) {
  if (!f.Synced) {
    toast('Sync to download', 'info');
    return;
  }
  try {
    await api.openFile(f.Path);
  } catch (err) {
    toast(`Could not open ${f.Name}: ${errMsg(err)}`, 'error');
  }
}

export async function openExternal(url: string) {
  if (!url) return;
  try {
    await api.openURL(url);
  } catch (err) {
    toast(errMsg(err), 'error');
  }
}

// ------------------------------------------------------------------- events

let wired = false;

/** Subscribe to backend events once, at app start. Returns a teardown fn. */
export function wireEvents(): () => void {
  if (wired) return () => {};
  wired = true;
  const offs: Array<() => void> = [];

  offs.push(
    on('sync:status', (s: SyncStatus) => {
      if (s) syncStatus.set(s);
    }),
  );

  offs.push(
    on('sync:done', (s: SyncStatus) => {
      if (s) syncStatus.set(s);
      void loadCourses();
      void loadRecent();
      void loadStats();
      void loadDeadlines();
      void loadUnseenCount();
    }),
  );

  offs.push(
    on('deadlines:updated', () => {
      void loadDeadlines();
    }),
  );

  offs.push(
    on('announcements:new', (items: Announcement[]) => {
      void loadAnnouncements();
      const n = Array.isArray(items) ? items.length : 0;
      if (n > 0) toast(`${n} new announcement${n === 1 ? '' : 's'}`, 'info');
    }),
  );

  offs.push(
    on('feed:updated', (n: number) => {
      unseenCount.set(typeof n === 'number' ? n : 0);
      void loadRecent();
    }),
  );

  offs.push(
    on('toast', (p: ToastPayload) => {
      if (!p?.Message) return;
      const level = p.Level === 'success' || p.Level === 'error' ? p.Level : 'info';
      toast(p.Message, level);
    }),
  );

  if (typeof window !== 'undefined') {
    const onNav = (e: Event) => {
      const view = (e as CustomEvent<{ view?: string }>).detail?.view;
      if (view && (ROUTES as readonly string[]).includes(view)) navigate(view as Route);
    };
    window.addEventListener(NAV_EVENT, onNav);
    offs.push(() => window.removeEventListener(NAV_EVENT, onNav));
  }

  return () => {
    offs.forEach((off) => {
      try {
        off();
      } catch {
        /* ignore */
      }
    });
    wired = false;
  };
}

/** Raise a toast through the same path the backend uses (handy for tests). */
export function emitToast(message: string, level: ToastPayload['Level'] = 'info') {
  emitLocal('toast', { Level: level, Message: message });
}
