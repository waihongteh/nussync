/**
 * Layout state: which panes are showing and how wide they are.
 *
 * Everything here is a local view preference, so it lives in localStorage only
 * — never in the backend settings. Plain Svelte stores (not runes) so this can
 * be a `.ts` module imported from anywhere.
 *
 * The sidebar has three tiers:
 *   'auto'      follow the window width (rail below AUTO_RAIL_BELOW)
 *   'expanded'  an explicit choice, honoured at any width
 *   'rail'      likewise
 *
 * An explicit choice is not permanent: once the window is widened back past
 * AUTO_EXPAND_ABOVE the mode falls back to 'auto', so a narrow-window override
 * does not silently follow the user to a big monitor. The revert fires only on
 * the crossing, which is why `initLayout` keeps the previous width — otherwise
 * picking 'rail' on a wide window would be undone immediately.
 */

import { derived, get, writable } from 'svelte/store';
import { lsGet, lsSet } from './util';

export type SidebarMode = 'expanded' | 'rail' | 'auto';

/**
 * How the Claude chat presents itself.
 *   'docked'   a real column in the content grid — the view shrinks beside it
 *   'overlay'  the old floating panel that sits on top of the view
 * Docked is the default: covering the PDF you are asking about defeats the
 * point of asking about it.
 */
export type ChatMode = 'docked' | 'overlay';

const SIDEBAR_KEY = 'nussync.layout.sidebar';
const SIDEBAR_W_KEY = 'nussync.layout.sidebarW';
const TREE_KEY = 'nussync.layout.tree';
const TREE_W_KEY = 'nussync.layout.treeW';
const LIST_KEY = 'nussync.layout.list';
const CHAT_MODE_KEY = 'nussync.layout.chatMode';
const CHAT_W_KEY = 'nussync.layout.chatW';

/** Collapsed icon-rail width. Matches the `.sidebar.rail` CSS. */
export const RAIL_W = 56;

export const SIDEBAR_W_DEFAULT = 224;
export const SIDEBAR_W_MIN = 200;
export const SIDEBAR_W_MAX = 280;

export const TREE_W_DEFAULT = 250;
export const TREE_W_MIN = 180;
export const TREE_W_MAX = 420;

export const CHAT_W_DEFAULT = 380;
export const CHAT_W_MIN = 320;
export const CHAT_W_MAX = 560;
/** Width of the docked chat once the collapse chevron has been used. */
export const CHAT_RAIL_W = 40;

/**
 * Width of the Files list pane once it has been collapsed to its strip — just
 * enough for a rotated "n files" label and the ▲/▼ step buttons.
 */
export const LIST_RAIL_W = 44;

/** Below this window width, 'auto' means rail. */
export const AUTO_RAIL_BELOW = 1100;
/** Widening past this drops an explicit override back to 'auto'. */
export const AUTO_EXPAND_ABOVE = 1200;

/** Below this *content* width, Files hides the tree while a viewer is open. */
export const TREE_AUTOHIDE_BELOW = 900;

/**
 * Below this *content* width (window minus sidebar) a docked chat would leave
 * the view too little room, so it falls back to the overlay. The stored
 * preference is untouched, so widening the window docks it again.
 */
export const CHAT_DOCK_MIN_CONTENT = 1100;

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v));

function readMode(): SidebarMode {
  const v = lsGet<string>(SIDEBAR_KEY, 'auto');
  return v === 'expanded' || v === 'rail' ? v : 'auto';
}

function readChatMode(): ChatMode {
  return lsGet<string>(CHAT_MODE_KEY, 'docked') === 'overlay' ? 'overlay' : 'docked';
}

export const sidebarMode = writable<SidebarMode>(readMode());
export const sidebarW = writable<number>(
  clamp(lsGet<number>(SIDEBAR_W_KEY, SIDEBAR_W_DEFAULT), SIDEBAR_W_MIN, SIDEBAR_W_MAX),
);

/** Files' folder tree visibility. */
export const treeOpen = writable<boolean>(lsGet<boolean>(TREE_KEY, true));
export const treeW = writable<number>(clamp(lsGet<number>(TREE_W_KEY, TREE_W_DEFAULT), TREE_W_MIN, TREE_W_MAX));

/**
 * Files' list pane collapsed to a strip. Persisted, unlike the chat's collapse:
 * with a viewer and a docked chat open this is a working posture, not a
 * momentary one, and having to re-collapse it every launch would be a chore.
 * Files also collapses the list *automatically* when the row runs out of room;
 * that is derived locally and never written back here, so the user's own choice
 * survives a resize.
 */
export const listCollapsed = writable<boolean>(lsGet<boolean>(LIST_KEY, false));

/** Chat placement preference. */
export const chatMode = writable<ChatMode>(readChatMode());
export const chatW = writable<number>(clamp(lsGet<number>(CHAT_W_KEY, CHAT_W_DEFAULT), CHAT_W_MIN, CHAT_W_MAX));
/** Docked chat squeezed down to its rail. Momentary, so not persisted. */
export const chatCollapsed = writable(false);

/** Viewer focus mode. Deliberately not persisted — it is a momentary state. */
export const viewerFocus = writable(false);

/** Live window width, kept by `initLayout`. */
export const winW = writable<number>(typeof window === 'undefined' ? 1440 : window.innerWidth);

/** What the sidebar actually renders as, once 'auto' is resolved. */
export const resolvedSidebar = derived([sidebarMode, winW], ([$mode, $w]) =>
  $mode === 'auto' ? ($w < AUTO_RAIL_BELOW ? 'rail' : 'expanded') : $mode,
);

export const isRail = derived(resolvedSidebar, ($s) => $s === 'rail');

/** Width left for everything right of the sidebar. */
export const contentW = derived([winW, resolvedSidebar, sidebarW], ([$w, $s, $sw]) =>
  Math.max(0, $w - ($s === 'rail' ? RAIL_W : $sw)),
);

/** True when the window is too narrow to dock the chat. */
export const chatCramped = derived(contentW, ($c) => $c < CHAT_DOCK_MIN_CONTENT);

/** What the chat actually renders as, once the narrow-window fallback applies. */
export const resolvedChatMode = derived([chatMode, chatCramped], ([$m, $cramped]): ChatMode =>
  $m === 'docked' && $cramped ? 'overlay' : $m,
);

/** True only while a docked preference is being overridden by the window width. */
export const chatForcedOverlay = derived([chatMode, chatCramped], ([$m, $c]) => $m === 'docked' && $c);

sidebarMode.subscribe((v) => lsSet(SIDEBAR_KEY, v));
sidebarW.subscribe((v) => lsSet(SIDEBAR_W_KEY, Math.round(v)));
treeOpen.subscribe((v) => lsSet(TREE_KEY, v));
treeW.subscribe((v) => lsSet(TREE_W_KEY, Math.round(v)));
listCollapsed.subscribe((v) => lsSet(LIST_KEY, v));
chatMode.subscribe((v) => lsSet(CHAT_MODE_KEY, v));
chatW.subscribe((v) => lsSet(CHAT_W_KEY, Math.round(v)));

/** Flip between rail and expanded, starting from whatever is on screen now. */
export function toggleSidebar() {
  sidebarMode.set(get(resolvedSidebar) === 'rail' ? 'expanded' : 'rail');
}

export function toggleTree() {
  treeOpen.update((v) => !v);
}

export function toggleListCollapsed() {
  listCollapsed.update((v) => !v);
}

export function toggleViewerFocus() {
  viewerFocus.update((v) => !v);
}

export function setSidebarWidth(px: number) {
  sidebarW.set(clamp(Math.round(px), SIDEBAR_W_MIN, SIDEBAR_W_MAX));
}

export function setTreeWidth(px: number) {
  treeW.set(clamp(Math.round(px), TREE_W_MIN, TREE_W_MAX));
}

export function setChatWidth(px: number) {
  chatW.set(clamp(Math.round(px), CHAT_W_MIN, CHAT_W_MAX));
}

/** Send the chat to the other placement, un-collapsing it on the way. */
export function toggleChatMode() {
  chatCollapsed.set(false);
  chatMode.update((m) => (m === 'docked' ? 'overlay' : 'docked'));
}

export function toggleChatCollapsed() {
  chatCollapsed.update((v) => !v);
}

export function resetLayout() {
  sidebarMode.set('auto');
  sidebarW.set(SIDEBAR_W_DEFAULT);
  treeOpen.set(true);
  treeW.set(TREE_W_DEFAULT);
  listCollapsed.set(false);
  chatMode.set('docked');
  chatW.set(CHAT_W_DEFAULT);
  chatCollapsed.set(false);
  viewerFocus.set(false);
}

/**
 * Width the docked chat column actually occupies right now — 0 whenever the
 * chat is closed or floating. Published as `--chat-w` by `setChatColumnWidth`
 * so fixed-position things (the viewer's focus mode) can keep clear of it.
 */
export function setChatColumnWidth(px: number) {
  if (typeof document === 'undefined') return;
  document.documentElement.style.setProperty('--chat-w', `${Math.round(px)}px`);
}

let wired = false;

/**
 * Track the window width and keep `--sidebar-w` in sync with the resolved
 * mode, so anything positioning against the sidebar (the focus overlay, the
 * drag ghost) reads one number. Idempotent; returns a teardown.
 */
export function initLayout(): () => void {
  if (wired || typeof window === 'undefined') return () => {};
  wired = true;

  let prev = window.innerWidth;
  winW.set(prev);
  setChatColumnWidth(0);

  const onResize = () => {
    const w = window.innerWidth;
    if (prev <= AUTO_EXPAND_ABOVE && w > AUTO_EXPAND_ABOVE && get(sidebarMode) !== 'auto') {
      sidebarMode.set('auto');
    }
    prev = w;
    winW.set(w);
  };
  window.addEventListener('resize', onResize);

  const offW = sidebarW.subscribe(() => applyVar());
  const offS = resolvedSidebar.subscribe(() => applyVar());

  function applyVar() {
    if (typeof document === 'undefined') return;
    const px = get(resolvedSidebar) === 'rail' ? RAIL_W : get(sidebarW);
    document.documentElement.style.setProperty('--sidebar-w', `${Math.round(px)}px`);
  }

  return () => {
    window.removeEventListener('resize', onResize);
    offW();
    offS();
    wired = false;
  };
}
