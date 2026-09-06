<script lang="ts">
  /**
   * What's new: files Canvas changed since you last looked, newest first,
   * grouped by day and then by course. Opening the view does not mark it seen —
   * that is an explicit button, so a glance never clears the badge.
   */
  import { api, errMsg } from '../api';
  import ContextMenu from '../components/ContextMenu.svelte';
  import Icon from '../components/Icon.svelte';
  import Viewer from '../components/Viewer.svelte';
  import { viewerFocus } from '../layout';
  import { courseByID, toast, unseenCount } from '../stores';
  import type { FeedItem, FileNode, MenuItem } from '../types';
  import { fileKind, fmtBytes, relTime } from '../util';
  import { on } from '../api';

  const KIND_ICON: Record<string, string> = {
    pdf: 'fileText',
    doc: 'fileText',
    slides: 'slides',
    sheet: 'sheet',
    image: 'image',
    video: 'video',
    archive: 'archive',
    code: 'code',
    file: 'file',
  };

  const WINDOWS = [
    { days: 3, label: '3 days' },
    { days: 7, label: '7 days' },
    { days: 14, label: '14 days' },
    { days: 30, label: '30 days' },
  ];

  let items = $state<FeedItem[]>([]);
  let loading = $state(true);
  let sinceDays = $state(7);
  let marking = $state(false);
  let menu = $state<{ x: number; y: number; items: MenuItem[] } | null>(null);
  let tick = $state(Date.now());

  $effect(() => {
    const h = setInterval(() => (tick = Date.now()), 60_000);
    return () => clearInterval(h);
  });

  async function load(days: number) {
    loading = true;
    try {
      items = (await api.getWhatsNew(days)) ?? [];
    } catch (err) {
      toast(`Could not load the feed: ${errMsg(err)}`, 'error');
      items = [];
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void load(sinceDays);
  });

  // A sync that lands while the view is open should refresh it in place.
  $effect(() => {
    const off = on('feed:updated', () => void load(sinceDays));
    return off;
  });

  async function markSeen() {
    marking = true;
    try {
      await api.markFeedSeen();
      unseenCount.set(0);
      toast('Feed marked as seen', 'success');
    } catch (err) {
      toast(errMsg(err), 'error');
    } finally {
      marking = false;
    }
  }

  function dayKey(iso: string): string {
    const d = new Date(iso);
    if (isNaN(d.getTime())) return 'Unknown';
    const start = new Date(tick);
    start.setHours(0, 0, 0, 0);
    const diff = Math.floor((start.getTime() - new Date(d).setHours(0, 0, 0, 0)) / 86_400_000);
    if (diff <= 0) return 'Today';
    if (diff === 1) return 'Yesterday';
    if (diff < 7) return d.toLocaleDateString(undefined, { weekday: 'long' });
    return d.toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' });
  }

  interface CourseGroup {
    code: string;
    courseID: number;
    files: FeedItem[];
  }
  interface DayGroup {
    day: string;
    total: number;
    courses: CourseGroup[];
  }

  const grouped = $derived.by<DayGroup[]>(() => {
    const days = new Map<string, Map<number, FeedItem[]>>();
    for (const it of items) {
      const day = dayKey(it.ChangedAt);
      let byCourse = days.get(day);
      if (!byCourse) {
        byCourse = new Map();
        days.set(day, byCourse);
      }
      const arr = byCourse.get(it.CourseID) ?? [];
      arr.push(it);
      byCourse.set(it.CourseID, arr);
    }
    return [...days.entries()].map(([day, byCourse]) => ({
      day,
      total: [...byCourse.values()].reduce((acc, l) => acc + l.length, 0),
      courses: [...byCourse.entries()]
        .map(([courseID, files]) => ({
          courseID,
          code: files[0]?.CourseCode || $courseByID.get(courseID)?.Code || '',
          files,
        }))
        .sort((a, b) => b.files.length - a.files.length),
    }));
  });

  const newCount = $derived(items.filter((i) => i.Kind === 'new').length);

  async function open(it: FeedItem) {
    try {
      await api.openFile(it.Path);
    } catch (err) {
      toast(`Could not open ${it.Name}: ${errMsg(err)}`, 'error');
    }
  }

  /** File shown in the preview overlay, or null. */
  let preview = $state<FileNode | null>(null);

  /** FeedItem carries everything FileNode needs bar a couple of unused fields. */
  function previewIt(it: FeedItem) {
    preview = {
      ID: it.ID,
      CourseID: it.CourseID,
      Name: it.Name,
      Path: it.Path,
      RelPath: it.RelPath,
      IsDir: false,
      Size: it.Size,
      ModifiedAt: it.ChangedAt,
      Source: 'files',
      Module: it.Module,
      Synced: true,
      IsNew: it.Kind === 'new',
      Children: null,
    };
  }

  function openMenu(e: MouseEvent, it: FeedItem) {
    e.preventDefault();
    menu = {
      x: e.clientX,
      y: e.clientY,
      items: [
        { label: 'Preview', icon: 'eye', run: () => previewIt(it) },
        { label: 'Open', icon: 'external', run: () => void open(it) },
        {
          label: 'Reveal in Explorer',
          icon: 'folderOpen',
          run: () => void api.revealFile(it.Path).catch((err) => toast(errMsg(err), 'error')),
        },
        {
          label: 'Copy path',
          icon: 'copy',
          run: () =>
            void navigator.clipboard
              .writeText(it.Path)
              .then(() => toast('Path copied to clipboard', 'success'))
              .catch(() => toast('Could not access the clipboard', 'error')),
        },
      ],
    };
  }
</script>

<div class="view">
  <div class="view-narrow">
    <header class="head">
      <div>
        <h1 class="page-title">What's new</h1>
        <p class="muted sub">
          {#if loading}
            Loading…
          {:else}
            {items.length} file{items.length === 1 ? '' : 's'} changed in the last {sinceDays} days · {newCount} new,
            {items.length - newCount} updated
          {/if}
        </p>
      </div>
      <div class="actions">
        <select class="select window" bind:value={sinceDays}>
          {#each WINDOWS as w (w.days)}
            <option value={w.days}>Last {w.label}</option>
          {/each}
        </select>
        <button class="btn" onclick={markSeen} disabled={marking || $unseenCount === 0}>
          {#if marking}<span class="spinner"></span>{:else}<Icon name="check" size={13} />{/if}
          Mark all seen{$unseenCount > 0 ? ` (${$unseenCount})` : ''}
        </button>
      </div>
    </header>

    {#if loading}
      <div class="card"><div class="empty"><span class="spinner"></span> Loading the feed…</div></div>
    {:else}
      {#each grouped as g (g.day)}
        <section class="day">
          <div class="day-head">
            <h2 class="section-title">{g.day}</h2>
            <span class="faint count">{g.total} file{g.total === 1 ? '' : 's'}</span>
          </div>
          {#each g.courses as c (c.courseID)}
            <div class="card course">
              <div class="course-head">
                <span class="dot" style="background:{$courseByID.get(c.courseID)?.Color ?? 'var(--text-faint)'}"></span>
                <span class="code">{c.code}</span>
                <span class="cname truncate faint">{$courseByID.get(c.courseID)?.Name ?? ''}</span>
              </div>
              {#each c.files as it (it.ID + it.RelPath)}
                <button
                  class="row"
                  onclick={() => open(it)}
                  oncontextmenu={(e) => openMenu(e, it)}
                  title="{it.Path}&#10;Right-click for more"
                >
                  <span class="fico"><Icon name={KIND_ICON[fileKind(it.Name)] ?? 'file'} size={14} /></span>
                  <span class="name truncate">{it.Name}</span>
                  <span class="chip kind" class:new={it.Kind === 'new'}>{it.Kind}</span>
                  <span class="mod truncate faint">{it.Module || it.RelPath.split('/').slice(0, -1).join(' / ') || '—'}</span>
                  <span class="size faint">{fmtBytes(it.Size)}</span>
                  <span class="time faint">{relTime(it.ChangedAt, tick)}</span>
                </button>
              {/each}
            </div>
          {/each}
        </section>
      {:else}
        <div class="card">
          <div class="empty">
            Nothing has changed in the last {sinceDays} days. Try a wider window, or run a sync.
          </div>
        </div>
      {/each}
    {/if}
  </div>
</div>

{#if preview}
  <div
    class="pv-backdrop"
    role="presentation"
    onclick={(e) => { if (e.target === e.currentTarget) preview = null; }}
  >
    <div class="pv-shell card" role="dialog" aria-modal="true" aria-label="File preview">
      <Viewer file={preview} onClose={() => (preview = null)} />
    </div>
  </div>
{/if}

<svelte:window onkeydown={(e) => { if (e.key === 'Escape' && preview && !$viewerFocus) preview = null; }} />

{#if menu}
  <ContextMenu x={menu.x} y={menu.y} items={menu.items} onClose={() => (menu = null)} />
{/if}

<style>
  /* Preview overlay (right-click a row -> Preview). */
  .pv-backdrop {
    position: fixed;
    inset: 0;
    z-index: 60;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 34px;
    background: var(--bg-overlay);
  }

  .pv-shell {
    width: min(1000px, 100%);
    height: min(80vh, 100%);
    padding: 0;
    overflow: hidden;
    display: flex;
    box-shadow: var(--shadow-pop);
  }

  .pv-shell :global(.viewer) {
    flex: 1;
    min-width: 0;
  }

  .head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    margin-bottom: 20px;
  }

  .sub {
    margin-top: 3px;
    font-size: 13px;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: none;
  }

  .window {
    width: auto;
    height: 30px;
    font-size: 12.5px;
  }

  .day {
    margin-bottom: 22px;
  }

  .day-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 8px;
    padding: 0 2px;
  }

  .count {
    font-size: 11.5px;
  }

  .course {
    margin-bottom: 8px;
    overflow: hidden;
  }

  .course-head {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 13px;
    background: var(--bg-subtle);
    border-bottom: 1px solid var(--border);
  }

  .code {
    font-size: 12.5px;
    font-weight: 620;
    flex: none;
  }

  .cname {
    font-size: 11.5px;
    min-width: 0;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 9px;
    width: 100%;
    padding: 8px 13px;
    text-align: left;
    font-size: 13px;
    border-bottom: 1px solid var(--border);
    transition: background var(--t);
  }

  .row:last-child {
    border-bottom: none;
  }

  .row:hover {
    background: var(--bg-hover);
  }

  .fico {
    color: var(--text-faint);
    display: flex;
    flex: none;
  }

  .name {
    flex: 1.6;
    min-width: 0;
  }

  .kind {
    flex: none;
    text-transform: capitalize;
  }

  .kind.new {
    background: var(--accent-soft);
    color: var(--accent-text);
  }

  .mod {
    flex: 1;
    min-width: 0;
    font-size: 11.5px;
  }

  .size {
    flex: none;
    width: 62px;
    text-align: right;
    font-size: 11.5px;
    font-variant-numeric: tabular-nums;
  }

  .time {
    flex: none;
    width: 62px;
    text-align: right;
    font-size: 11.5px;
  }

  @media (max-width: 860px) {
    .mod,
    .size {
      display: none;
    }
  }
</style>
