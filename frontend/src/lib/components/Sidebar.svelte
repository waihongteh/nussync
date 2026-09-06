<script lang="ts">
  import { api } from '../api';
  import {
    courses,
    deadlines,
    navigate,
    openExternal,
    route,
    selectedCourseID,
    settings,
    unreadAnnouncements,
    unseenCount,
  } from '../stores';
  import type { Route } from '../stores';
  import type { Course, MenuItem } from '../types';
  import { applyOrder, courseOrder, hiddenCourses, hideCourse, saveOrder, unhideCourse } from '../courseOrder';
  import ContextMenu from './ContextMenu.svelte';
  import Icon from './Icon.svelte';
  import Pet from './Pet.svelte';
  import SyncPill from './SyncPill.svelte';
  import { petEnabled, petMode } from '../pet';

  const NAV: Array<{ id: Route; label: string; icon: string }> = [
    { id: 'home', label: 'Home', icon: 'home' },
    { id: 'files', label: 'Files', icon: 'files' },
    { id: 'whatsnew', label: "What's new", icon: 'dot' },
    { id: 'deadlines', label: 'Deadlines', icon: 'deadlines' },
    { id: 'announcements', label: 'Announcements', icon: 'announcements' },
    { id: 'grades', label: 'Grades', icon: 'grades' },
    { id: 'study', label: 'Study', icon: 'layers' },
    { id: 'papers', label: 'Papers', icon: 'book' },
    { id: 'settings', label: 'Settings', icon: 'settings' },
    { id: 'arcade', label: 'Play', icon: 'gamepad' },
  ];

  /**
   * Unsubmitted work only. GetDeadlines already returns upcoming plus
   * overdue-unsubmitted, so "pending" is exactly what the Deadlines view shows
   * with "hide submitted" on — computed here rather than from the shared
   * upcomingCount, which drops overdue items.
   */
  const pendingDeadlines = $derived($deadlines.filter((d) => !d.Submitted).length);

  function badgeFor(id: Route): number {
    if (id === 'deadlines') return pendingDeadlines;
    if (id === 'announcements') return $unreadAnnouncements;
    if (id === 'whatsnew') return $unseenCount;
    return 0;
  }

  function pickCourse(id: number) {
    selectedCourseID.set($selectedCourseID === id ? 0 : id);
    navigate('files');
  }

  // ------------------------------------------------------- order / hiding

  const ordered = $derived(applyOrder($courses, $courseOrder));
  const visible = $derived(ordered.filter((c) => !$hiddenCourses.includes(c.ID)));
  const hiddenList = $derived(ordered.filter((c) => $hiddenCourses.includes(c.ID)));
  const byID = $derived(new Map(ordered.map((c) => [c.ID, c])));

  let showHidden = $state(false);

  /** Persist the visible sequence; hidden ids keep their relative order after it. */
  function commit(ids: number[]) {
    saveOrder([...ids, ...hiddenList.map((c) => c.ID)]);
  }

  function move(id: number, delta: number) {
    const ids = visible.map((c) => c.ID);
    const from = ids.indexOf(id);
    const to = from + delta;
    if (from < 0 || to < 0 || to >= ids.length) return;
    ids.splice(to, 0, ids.splice(from, 1)[0]);
    commit(ids);
  }

  function moveToTop(id: number) {
    const ids = visible.map((c) => c.ID);
    commit([id, ...ids.filter((x) => x !== id)]);
  }

  // ------------------------------------------------------- drag to reorder

  /** .course height + the flex gap between rows. */
  const ROW = 28;
  const HOLD_MS = 150;

  let listEl = $state<HTMLDivElement | null>(null);
  let dragID = $state(0);
  let dragIDs = $state<number[]>([]);
  let ghost = $state<{ x: number; y: number } | null>(null);
  let justDragged = false;

  let press: { id: number; y: number; pointerId: number; el: HTMLElement } | null = null;
  let holdTimer: ReturnType<typeof setTimeout> | undefined;

  /** While dragging, rows come from the live scratch order instead of the store. */
  const rows = $derived(
    dragID ? (dragIDs.map((id) => byID.get(id)).filter(Boolean) as Course[]) : visible,
  );

  function beginDrag(clientY: number) {
    if (!press) return;
    clearTimeout(holdTimer);
    dragID = press.id;
    dragIDs = visible.map((c) => c.ID);
    ghost = { x: 0, y: clientY };
  }

  function onPointerDown(e: PointerEvent, c: Course) {
    if (e.button !== 0) return;
    const el = e.currentTarget as HTMLElement;
    press = { id: c.ID, y: e.clientY, pointerId: e.pointerId, el };
    try {
      el.setPointerCapture(e.pointerId);
    } catch {
      /* synthetic events have no capturable pointer */
    }
    const onHandle = !!(e.target as HTMLElement).closest?.('.grip');
    if (onHandle) {
      e.preventDefault();
      beginDrag(e.clientY);
    } else {
      holdTimer = setTimeout(() => beginDrag(e.clientY), HOLD_MS);
    }
  }

  function onPointerMove(e: PointerEvent) {
    if (!press) return;
    if (!dragID) {
      // A real scroll gesture should not turn into a drag.
      if (Math.abs(e.clientY - press.y) > 6) {
        clearTimeout(holdTimer);
        press = null;
      }
      return;
    }
    ghost = { x: 0, y: e.clientY };
    if (!listEl) return;
    const rect = listEl.getBoundingClientRect();
    const y = e.clientY - rect.top + listEl.scrollTop;
    const from = dragIDs.indexOf(dragID);
    const to = Math.max(0, Math.min(dragIDs.length - 1, Math.floor(y / ROW)));
    if (from >= 0 && to !== from) {
      const next = [...dragIDs];
      next.splice(to, 0, next.splice(from, 1)[0]);
      dragIDs = next;
    }
  }

  function endDrag(e: PointerEvent) {
    clearTimeout(holdTimer);
    try {
      press?.el.releasePointerCapture?.(e.pointerId);
    } catch {
      /* not captured */
    }
    if (dragID) {
      commit(dragIDs);
      justDragged = true;
      dragID = 0;
      ghost = null;
    }
    press = null;
  }

  // --------------------------------------------------------- context menu

  let menu = $state<{ x: number; y: number; items: MenuItem[] } | null>(null);
  let canvasBase = $state('');

  $effect(() => {
    const url = $settings?.CanvasURL;
    if (url) canvasBase = url;
  });

  /** Canvas base URL from the settings store, falling back to one fetch. */
  async function openInCanvas(id: number) {
    let base = canvasBase;
    if (!base) {
      try {
        base = (await api.getSettings())?.CanvasURL ?? '';
        canvasBase = base;
      } catch {
        base = '';
      }
    }
    if (!base) return;
    void openExternal(`${base.replace(/\/+$/, '')}/courses/${id}`);
  }

  function openMenu(e: MouseEvent, c: Course) {
    e.preventDefault();
    menu = {
      x: e.clientX,
      y: e.clientY,
      items: [
        { label: 'Hide from sidebar', icon: 'eyeOff', run: () => hideCourse(c.ID) },
        { label: 'Move to top', icon: 'sort', run: () => moveToTop(c.ID) },
        { label: 'Open course in Canvas', icon: 'external', run: () => void openInCanvas(c.ID) },
      ],
    };
  }
</script>

<aside class="sidebar">
  <div class="brand">
    <span class="mark" aria-hidden="true"></span>
    <span class="word">NUSSync</span>
  </div>

  <nav class="nav">
    {#each NAV as item (item.id)}
      {@const badge = badgeFor(item.id)}
      <button
        class="nav-item"
        data-nav={item.id}
        class:active={$route === item.id}
        onclick={() => navigate(item.id)}
        aria-current={$route === item.id ? 'page' : undefined}
      >
        <Icon name={item.icon} size={15} />
        <span class="nav-label">{item.label}</span>
        {#if badge > 0}<span class="badge">{badge}</span>{/if}
      </button>
    {/each}
  </nav>

  <div class="courses">
    <div class="courses-head">
      <span class="section-title">Courses</span>
      {#if $selectedCourseID !== 0}
        <button class="clear" onclick={() => selectedCourseID.set(0)}>Clear</button>
      {/if}
    </div>
    <div class="course-list" bind:this={listEl}>
      {#each rows as c (c.ID)}
        <div
          class="course"
          class:active={$selectedCourseID === c.ID}
          class:off={!c.Enabled}
          class:dragging={dragID === c.ID}
          role="button"
          tabindex="0"
          title="{c.Code} — {c.Name}"
          onpointerdown={(e) => onPointerDown(e, c)}
          onpointermove={onPointerMove}
          onpointerup={endDrag}
          onpointercancel={endDrag}
          oncontextmenu={(e) => openMenu(e, c)}
          onclick={() => {
            if (justDragged) {
              justDragged = false;
              return;
            }
            pickCourse(c.ID);
          }}
          onkeydown={(e) => {
            if (e.altKey && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
              e.preventDefault();
              move(c.ID, e.key === 'ArrowUp' ? -1 : 1);
              return;
            }
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault();
              pickCourse(c.ID);
            }
          }}
        >
          <span class="grip" aria-hidden="true"><Icon name="sort" size={11} /></span>
          <span class="dot" style="background:{c.Color}"></span>
          <span class="code truncate">{c.Code}</span>
          <span class="count">{c.FileCount}</span>
        </div>
      {/each}

      {#if hiddenList.length > 0}
        <button class="hidden-row" onclick={() => (showHidden = !showHidden)} aria-expanded={showHidden}>
          <span class="tw" class:open={showHidden}><Icon name="chevronRight" size={11} /></span>
          <span class="code truncate">Hidden ({hiddenList.length})</span>
        </button>
        {#if showHidden}
          {#each hiddenList as c (c.ID)}
            <div class="course hidden-course" title="{c.Code} — {c.Name}">
              <span class="dot" style="background:{c.Color}"></span>
              <span class="code truncate">{c.Code}</span>
              <button class="unhide" onclick={() => unhideCourse(c.ID)}>Unhide</button>
            </div>
          {/each}
        {/if}
      {/if}

      {#if $courses.length === 0}
        <div class="no-courses faint">No courses yet</div>
      {/if}
    </div>
  </div>

  <!-- Docked pet only in 'dock' mode; the other modes render in PetOverlay. -->
  {#if $petEnabled && $petMode === 'dock'}
    <Pet />
  {/if}

  <SyncPill />
</aside>

{#if ghost && dragID}
  {@const g = byID.get(dragID)}
  {#if g}
    <div class="ghost" style="top:{ghost.y - 13}px">
      <span class="dot" style="background:{g.Color}"></span>
      <span class="code truncate">{g.Code}</span>
    </div>
  {/if}
{/if}

{#if menu}
  <ContextMenu x={menu.x} y={menu.y} items={menu.items} onClose={() => (menu = null)} />
{/if}

<style>
  .sidebar {
    width: var(--sidebar-w);
    flex: none;
    display: flex;
    flex-direction: column;
    min-height: 0;
    background: var(--bg-sidebar);
    border-right: 1px solid var(--border);
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 8px;
    height: var(--topbar-h);
    padding: 0 14px;
    flex: none;
  }

  .mark {
    width: 15px;
    height: 15px;
    border-radius: 5px;
    background: var(--accent);
    box-shadow: inset 0 0 0 3.5px var(--bg-sidebar);
  }

  .word {
    font-size: 13.5px;
    font-weight: 650;
    letter-spacing: -0.01em;
  }

  .nav {
    display: flex;
    flex-direction: column;
    gap: 1px;
    padding: 4px 8px 10px;
    flex: none;
  }

  .nav-item {
    display: flex;
    align-items: center;
    gap: 9px;
    width: 100%;
    height: 30px;
    padding: 0 8px;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    font-size: 13px;
    font-weight: 500;
    transition: background var(--t), color var(--t);
  }

  .nav-item:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .nav-item.active {
    background: var(--bg-active);
    color: var(--accent-text);
    font-weight: 560;
  }

  .nav-label {
    flex: 1;
    text-align: left;
  }

  .badge {
    min-width: 17px;
    height: 17px;
    padding: 0 5px;
    border-radius: 999px;
    background: var(--bg-subtle);
    color: var(--text-muted);
    font-size: 10.5px;
    font-weight: 600;
    line-height: 17px;
    text-align: center;
    font-variant-numeric: tabular-nums;
  }

  .nav-item.active .badge {
    background: var(--accent);
    color: #fff;
  }

  .courses {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    padding: 0 8px;
  }

  .courses-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 4px 8px 6px;
  }

  .clear {
    font-size: 11px;
    color: var(--accent-text);
    font-weight: 550;
  }

  .clear:hover {
    text-decoration: underline;
  }

  .course-list {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 1px;
    padding-bottom: 10px;
  }

  .course {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    height: 27px;
    flex: none;
    padding: 0 8px;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    font-size: 12.5px;
    cursor: pointer;
    user-select: none;
    touch-action: none;
    transition: background var(--t), color var(--t);
  }

  .course:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .course:focus-visible {
    outline: 1px solid var(--accent);
    outline-offset: -1px;
  }

  .course.active {
    background: var(--bg-active);
    color: var(--accent-text);
    font-weight: 560;
  }

  .course.off {
    opacity: 0.45;
  }

  .course.dragging {
    opacity: 0.3;
  }

  /* The grip only claims space on hover, so the resting row is unchanged. */
  .grip {
    width: 0;
    overflow: hidden;
    display: flex;
    align-items: center;
    color: var(--text-faint);
    cursor: grab;
    transition: width var(--t);
  }

  .course:hover .grip {
    width: 11px;
  }

  .code {
    flex: 1;
    text-align: left;
  }

  .count {
    font-size: 11px;
    color: var(--text-faint);
    font-variant-numeric: tabular-nums;
  }

  .hidden-row {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    height: 24px;
    flex: none;
    margin-top: 4px;
    padding: 0 8px;
    border-radius: var(--radius-sm);
    color: var(--text-faint);
    font-size: 11.5px;
  }

  .hidden-row:hover {
    background: var(--bg-hover);
    color: var(--text-muted);
  }

  .tw {
    display: flex;
    transition: transform var(--t);
  }

  .tw.open {
    transform: rotate(90deg);
  }

  .hidden-course {
    opacity: 0.65;
    cursor: default;
  }

  .unhide {
    font-size: 11px;
    color: var(--accent-text);
  }

  .unhide:hover {
    text-decoration: underline;
  }

  .ghost {
    position: fixed;
    left: 10px;
    z-index: 200;
    display: flex;
    align-items: center;
    gap: 8px;
    width: calc(var(--sidebar-w) - 20px);
    height: 27px;
    padding: 0 8px;
    border-radius: var(--radius-sm);
    background: var(--bg-elevated);
    border: 1px solid var(--border-strong);
    box-shadow: var(--shadow-pop);
    color: var(--text);
    font-size: 12.5px;
    pointer-events: none;
  }

  .no-courses {
    padding: 8px;
    font-size: 12px;
  }
</style>
