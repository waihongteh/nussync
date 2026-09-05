<script lang="ts">
  import { api } from '../api';
  import {
    courseByID,
    flatFiles,
    navigate,
    openFileNode,
    paletteOpen,
    selectedCourseID,
    syncNow,
    theme,
  } from '../stores';
  import type { Route } from '../stores';
  import type { FileNode } from '../types';
  import { playGame } from '../games/arcade';
  import { fileKind, fmtBytes, fuzzyScore } from '../util';
  import Icon from './Icon.svelte';

  interface Item {
    key: string;
    kind: 'command' | 'file';
    label: string;
    hint: string;
    icon: string;
    color?: string;
    run: () => void;
  }

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

  let query = $state('');
  let cursor = $state(0);
  let inputEl = $state<HTMLInputElement | null>(null);
  let listEl = $state<HTMLDivElement | null>(null);
  let loading = $state(false);

  const NAV_COMMANDS: Array<{ label: string; icon: string; route: Route }> = [
    { label: 'Go to Home', icon: 'home', route: 'home' },
    { label: 'Go to Files', icon: 'files', route: 'files' },
    { label: 'Go to Deadlines', icon: 'deadlines', route: 'deadlines' },
    { label: 'Go to Announcements', icon: 'announcements', route: 'announcements' },
    { label: 'Go to Grades', icon: 'grades', route: 'grades' },
    { label: 'Go to Arcade', icon: 'gamepad', route: 'arcade' },
    { label: 'Go to Settings', icon: 'settings', route: 'settings' },
  ];

  const commands = $derived<Item[]>([
    ...NAV_COMMANDS.map((c) => ({
      key: `nav:${c.route}`,
      kind: 'command' as const,
      label: c.label,
      hint: 'Navigation',
      icon: c.icon,
      run: () => navigate(c.route),
    })),
    {
      key: 'cmd:sync',
      kind: 'command' as const,
      label: 'Sync now',
      hint: 'Action',
      icon: 'sync',
      run: () => void syncNow(),
    },
    {
      key: 'cmd:all-courses',
      kind: 'command' as const,
      label: 'Show all courses',
      hint: 'Action',
      icon: 'layers',
      run: () => {
        selectedCourseID.set(0);
        navigate('files');
      },
    },
    {
      key: 'cmd:play-run',
      kind: 'command' as const,
      label: 'Play: Nibble Run',
      hint: 'Arcade',
      icon: 'gamepad',
      run: () => playGame('run-daily'),
    },
    {
      key: 'cmd:play-merge',
      kind: 'command' as const,
      label: 'Play: Lecture Merge',
      hint: 'Arcade',
      icon: 'gamepad',
      run: () => playGame('merge'),
    },
    {
      key: 'cmd:theme-light',
      kind: 'command' as const,
      label: 'Switch to light theme',
      hint: 'Appearance',
      icon: 'sun',
      run: () => theme.set('light'),
    },
    {
      key: 'cmd:theme-system',
      kind: 'command' as const,
      label: 'Match system theme',
      hint: 'Appearance',
      icon: 'monitor',
      run: () => theme.set('system'),
    },
    {
      key: 'cmd:theme-dark',
      kind: 'command' as const,
      label: 'Switch to dark theme',
      hint: 'Appearance',
      icon: 'moon',
      run: () => theme.set('dark'),
    },
  ]);

  const fileItems = $derived<Item[]>(
    $flatFiles.map((f: FileNode) => ({
      key: `file:${f.CourseID}:${f.RelPath}`,
      kind: 'file' as const,
      label: f.Name,
      hint: `${$courseByID.get(f.CourseID)?.Code ?? ''} · ${f.RelPath.split('/').slice(0, -1).join(' / ') || 'root'} · ${fmtBytes(f.Size)}`,
      icon: KIND_ICON[fileKind(f.Name)] ?? 'file',
      color: $courseByID.get(f.CourseID)?.Color,
      run: () => void openFileNode(f),
    })),
  );

  const results = $derived.by<Item[]>(() => {
    const q = query.trim();
    if (!q) return [...commands, ...fileItems.slice(0, 30)];
    const scored: Array<{ item: Item; score: number }> = [];
    for (const item of commands) {
      const s = fuzzyScore(q, item.label);
      if (s >= 0) scored.push({ item, score: s + 30 });
    }
    for (const item of fileItems) {
      const s = fuzzyScore(q, item.label);
      if (s >= 0) scored.push({ item, score: s });
    }
    scored.sort((a, b) => b.score - a.score);
    return scored.slice(0, 50).map((x) => x.item);
  });

  $effect(() => {
    // keep the cursor inside the result list
    void results;
    cursor = 0;
  });

  async function ensureFiles() {
    if ($flatFiles.length > 0 || loading) return;
    loading = true;
    try {
      const tree = await api.getTree(0);
      const out: FileNode[] = [];
      const walk = (nodes: FileNode[]) => {
        for (const n of nodes) {
          if (n.IsDir) walk(n.Children ?? []);
          else out.push(n);
        }
      };
      walk(tree ?? []);
      flatFiles.set(out);
    } catch (err) {
      console.error('[palette] failed to load file index', err);
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    if ($paletteOpen) {
      query = '';
      cursor = 0;
      void ensureFiles();
      queueMicrotask(() => inputEl?.focus());
    }
  });

  function close() {
    paletteOpen.set(false);
  }

  function choose(item: Item | undefined) {
    if (!item) return;
    close();
    item.run();
  }

  function scrollCursorIntoView() {
    queueMicrotask(() => {
      const el = listEl?.querySelector<HTMLElement>('.row.sel');
      el?.scrollIntoView({ block: 'nearest' });
    });
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      cursor = results.length ? (cursor + 1) % results.length : 0;
      scrollCursorIntoView();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      cursor = results.length ? (cursor - 1 + results.length) % results.length : 0;
      scrollCursorIntoView();
    } else if (e.key === 'Enter') {
      e.preventDefault();
      choose(results[cursor]);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      close();
    }
  }
</script>

{#if $paletteOpen}
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div class="scrim" onclick={close}></div>
  <div class="palette" role="dialog" aria-modal="true" aria-label="Command palette">
    <div class="search">
      <Icon name="search" size={15} class="search-icon" />
      <input
        bind:this={inputEl}
        bind:value={query}
        onkeydown={onKeydown}
        class="q"
        type="text"
        placeholder="Search files or run a command…"
        spellcheck="false"
        autocomplete="off"
      />
      {#if loading}<span class="spinner"></span>{/if}
      <kbd>Esc</kbd>
    </div>

    <div class="list" bind:this={listEl}>
      {#each results as item, i (item.key)}
        <button
          class="row"
          class:sel={i === cursor}
          onmouseenter={() => (cursor = i)}
          onclick={() => choose(item)}
        >
          {#if item.color}
            <span class="dot" style="background:{item.color}"></span>
          {:else}
            <span class="ico"><Icon name={item.icon} size={14} /></span>
          {/if}
          <span class="label truncate">{item.label}</span>
          <span class="hint truncate">{item.hint}</span>
        </button>
      {/each}
      {#if results.length === 0}
        <div class="empty">No matches for “{query}”</div>
      {/if}
    </div>

    <div class="footer">
      <span><kbd>↑</kbd><kbd>↓</kbd> navigate</span>
      <span><kbd>↵</kbd> open</span>
      <span class="grow"></span>
      <span>{results.length} result{results.length === 1 ? '' : 's'}</span>
    </div>
  </div>
{/if}

<style>
  .scrim {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    z-index: 150;
    animation: fade 120ms ease;
  }

  .palette {
    position: fixed;
    top: 14vh;
    left: 50%;
    transform: translateX(-50%);
    width: min(620px, calc(100vw - 48px));
    max-height: 62vh;
    display: flex;
    flex-direction: column;
    background: var(--bg-elevated);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-pop);
    z-index: 151;
    overflow: hidden;
    animation: pop 140ms cubic-bezier(0.2, 0.9, 0.3, 1);
  }

  .search {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 0 12px;
    height: 46px;
    border-bottom: 1px solid var(--border);
    color: var(--text-faint);
    flex: none;
  }

  .q {
    flex: 1;
    height: 100%;
    border: none;
    background: none;
    outline: none;
    font-size: 14.5px;
    color: var(--text);
  }

  .q::placeholder {
    color: var(--text-faint);
  }

  .list {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 6px;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    height: 34px;
    padding: 0 9px;
    border-radius: var(--radius-sm);
    color: var(--text);
    font-size: 13px;
    text-align: left;
  }

  .row.sel {
    background: var(--bg-active);
  }

  .ico {
    color: var(--text-faint);
    display: flex;
  }

  .row.sel .ico {
    color: var(--accent-text);
  }

  .label {
    flex: none;
    max-width: 58%;
    font-weight: 500;
  }

  .hint {
    flex: 1;
    text-align: right;
    color: var(--text-faint);
    font-size: 11.5px;
  }

  .footer {
    display: flex;
    align-items: center;
    gap: 12px;
    height: 32px;
    padding: 0 12px;
    border-top: 1px solid var(--border);
    background: var(--bg-subtle);
    color: var(--text-faint);
    font-size: 11.5px;
    flex: none;
  }

  .grow {
    flex: 1;
  }

  kbd {
    display: inline-block;
    min-width: 17px;
    padding: 1px 4px;
    margin-right: 3px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg-subtle);
    font-family: var(--font);
    font-size: 10.5px;
    color: var(--text-muted);
    text-align: center;
  }

  .search kbd {
    background: var(--bg-elevated);
    margin-right: 0;
  }

  @keyframes fade {
    from {
      opacity: 0;
    }
  }

  @keyframes pop {
    from {
      opacity: 0;
      transform: translateX(-50%) translateY(-6px) scale(0.99);
    }
  }
</style>
