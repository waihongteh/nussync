<script lang="ts">
  import Icon from '../components/Icon.svelte';
  import {
    announcements,
    courseByID,
    deadlines,
    navigate,
    openExternal,
    openFileNode,
    recentFiles,
    stats,
  } from '../stores';
  import { countdown, fileKind, fmtBytes, relTime, urgency } from '../util';

  let tick = $state(Date.now());
  $effect(() => {
    const h = setInterval(() => (tick = Date.now()), 60_000);
    return () => clearInterval(h);
  });

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

  const greeting = $derived.by(() => {
    const h = new Date(tick).getHours();
    if (h < 5) return 'Still up';
    if (h < 12) return 'Good morning';
    if (h < 18) return 'Good afternoon';
    return 'Good evening';
  });

  const dueSoon = $derived(
    [...$deadlines]
      .filter((d) => !d.Submitted || Date.parse(d.DueAt) > tick)
      .sort((a, b) => Date.parse(a.DueAt) - Date.parse(b.DueAt))
      .slice(0, 5),
  );

  const latest = $derived($announcements.slice(0, 3));

  const tiles = $derived([
    { label: 'Files synced', value: $stats ? $stats.Files.toLocaleString() : '—', icon: 'files', route: 'files' as const },
    { label: 'Storage used', value: $stats ? fmtBytes($stats.Bytes) : '—', icon: 'database', route: 'files' as const },
    { label: 'Courses', value: $stats ? String($stats.Courses) : '—', icon: 'layers', route: 'settings' as const },
    { label: 'Upcoming', value: $stats ? String($stats.Deadlines) : '—', icon: 'clock', route: 'deadlines' as const },
  ]);

  const URGENCY_CLASS: Record<string, string> = {
    done: 'green',
    overdue: 'red',
    today: 'red',
    soon: 'amber',
    later: '',
  };
</script>

<div class="view">
  <div class="view-narrow">
    <header class="head">
      <h1 class="page-title">{greeting}</h1>
      <p class="muted sub">
        {#if $stats?.LastSync}
          Everything is up to date as of {relTime($stats.LastSync, tick)}.
        {:else}
          Run a sync to pull your Canvas files down.
        {/if}
      </p>
    </header>

    <section class="tiles">
      {#each tiles as t (t.label)}
        <button class="tile card" onclick={() => navigate(t.route)}>
          <span class="tile-icon"><Icon name={t.icon} size={14} /></span>
          <span class="tile-value">{t.value}</span>
          <span class="tile-label">{t.label}</span>
        </button>
      {/each}
    </section>

    <section class="block">
      <div class="block-head">
        <h2 class="section-title">Due soon</h2>
        <button class="more" onclick={() => navigate('deadlines')}>All deadlines</button>
      </div>
      <div class="card list">
        {#each dueSoon as d (d.ID)}
          {@const u = urgency(d.DueAt, d.Submitted, tick)}
          <button class="row" onclick={() => openExternal(d.URL)}>
            <span class="dot" style="background:{$courseByID.get(d.CourseID)?.Color ?? 'var(--text-faint)'}"></span>
            <span class="code">{d.CourseCode}</span>
            <span class="title truncate">{d.Title}</span>
            {#if d.Submitted}
              <span class="chip green"><Icon name="check" size={11} /> Submitted</span>
            {:else}
              <span class="chip {URGENCY_CLASS[u]}">{countdown(d.DueAt, tick)}</span>
            {/if}
          </button>
        {:else}
          <div class="empty">Nothing due — enjoy it while it lasts.</div>
        {/each}
      </div>
    </section>

    <div class="cols">
      <section class="block">
        <div class="block-head">
          <h2 class="section-title">Recent files</h2>
          <button class="more" onclick={() => navigate('files')}>Browse</button>
        </div>
        <div class="card list">
          {#each $recentFiles as f (f.CourseID + f.RelPath)}
            <button class="row file" class:unsynced={!f.Synced} ondblclick={() => openFileNode(f)} onclick={() => openFileNode(f)}>
              <span class="fico"><Icon name={KIND_ICON[fileKind(f.Name)] ?? 'file'} size={14} /></span>
              <span class="title truncate">{f.Name}</span>
              <span class="meta">{$courseByID.get(f.CourseID)?.Code ?? ''}</span>
              <span class="meta time">{relTime(f.ModifiedAt, tick)}</span>
            </button>
          {:else}
            <div class="empty">No files yet.</div>
          {/each}
        </div>
      </section>

      <section class="block">
        <div class="block-head">
          <h2 class="section-title">Latest announcements</h2>
          <button class="more" onclick={() => navigate('announcements')}>All</button>
        </div>
        <div class="card list">
          {#each latest as a (a.ID)}
            <button class="row ann" onclick={() => navigate('announcements')}>
              <span class="ann-top">
                <span class="code">{a.CourseCode}</span>
                <span class="title truncate">{a.Title}</span>
                {#if !a.Read}<span class="unread" title="Unread"></span>{/if}
              </span>
              <span class="ann-sub truncate">{relTime(a.PostedAt, tick)} · {a.Text}</span>
            </button>
          {:else}
            <div class="empty">Nothing posted yet.</div>
          {/each}
        </div>
      </section>
    </div>
  </div>
</div>

<style>
  .head {
    margin-bottom: 20px;
  }

  .sub {
    margin-top: 3px;
    font-size: 13px;
  }

  .tiles {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 10px;
    margin-bottom: 26px;
  }

  .tile {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 1px;
    padding: 13px 14px;
    text-align: left;
    transition: border-color var(--t), background var(--t);
  }

  .tile:hover {
    border-color: var(--border-strong);
    background: var(--bg-hover);
  }

  .tile-icon {
    color: var(--text-faint);
    margin-bottom: 7px;
  }

  .tile-value {
    font-size: 20px;
    font-weight: 620;
    letter-spacing: -0.02em;
    font-variant-numeric: tabular-nums;
  }

  .tile-label {
    font-size: 11.5px;
    color: var(--text-muted);
  }

  .block {
    margin-bottom: 22px;
    min-width: 0;
  }

  .block-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 8px;
    padding: 0 2px;
  }

  .more {
    font-size: 11.5px;
    font-weight: 550;
    color: var(--accent-text);
  }

  .more:hover {
    text-decoration: underline;
  }

  .list {
    overflow: hidden;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 9px;
    width: 100%;
    padding: 9px 12px;
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

  .code {
    flex: none;
    font-size: 11.5px;
    font-weight: 600;
    color: var(--text-muted);
    min-width: 62px;
  }

  .title {
    flex: 1;
    min-width: 0;
  }

  .fico {
    color: var(--text-faint);
    display: flex;
    flex: none;
  }

  .file.unsynced {
    color: var(--text-faint);
  }

  .meta {
    flex: none;
    font-size: 11.5px;
    color: var(--text-faint);
  }

  .time {
    min-width: 62px;
    text-align: right;
  }

  .cols {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 18px;
  }

  .ann {
    flex-direction: column;
    align-items: stretch;
    gap: 2px;
  }

  .ann-top {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }

  .ann-sub {
    font-size: 11.5px;
    color: var(--text-faint);
  }

  .unread {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--accent);
    flex: none;
  }

  @media (max-width: 900px) {
    .tiles {
      grid-template-columns: repeat(2, 1fr);
    }
    .cols {
      grid-template-columns: 1fr;
    }
  }
</style>
