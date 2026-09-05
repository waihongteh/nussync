<script lang="ts">
  import { api, errMsg } from '../api';
  import ContextMenu from '../components/ContextMenu.svelte';
  import FolderTree from '../components/FolderTree.svelte';
  import Icon from '../components/Icon.svelte';
  import { courses, courseByID, flatFiles, openFileNode, selectedCourseID, toast } from '../stores';
  import type { FileNode, MenuItem, SearchHit } from '../types';
  import { debounce, fileKind, fmtBytes, lsGet, lsSet, relTime, snippetHTML } from '../util';

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

  const EXPANDED_KEY = 'nussync.files.expanded';
  const SELECTED_KEY = 'nussync.files.selectedFolder';

  let roots = $state<FileNode[]>([]);
  let loading = $state(true);
  let expanded = $state<Set<string>>(new Set(lsGet<string[]>(EXPANDED_KEY, [])));
  let selectedFolder = $state<string>(lsGet<string>(SELECTED_KEY, ''));
  let query = $state('');
  let serverHits = $state<SearchHit[]>([]);
  let searching = $state(false);
  let selectedFile = $state<string>('');
  let menu = $state<{ x: number; y: number; items: MenuItem[] } | null>(null);
  let tick = $state(Date.now());

  $effect(() => {
    const h = setInterval(() => (tick = Date.now()), 60_000);
    return () => clearInterval(h);
  });

  const keyOf = (n: FileNode) => `${n.CourseID}:${n.RelPath}`;

  /** Wrap each course's tree in a synthetic folder so "All courses" groups cleanly. */
  function courseRoot(courseID: number, code: string, children: FileNode[]): FileNode {
    return {
      ID: 0,
      CourseID: courseID,
      Name: code,
      Path: '',
      RelPath: '',
      IsDir: true,
      Size: 0,
      ModifiedAt: '',
      Source: 'files',
      Module: '',
      Synced: true,
      Children: children,
    };
  }

  async function loadTree() {
    loading = true;
    try {
      const cid = $selectedCourseID;
      const list = cid ? $courses.filter((c) => c.ID === cid) : $courses;
      const trees = await Promise.all(list.map((c) => api.getTree(c.ID).catch(() => [] as FileNode[])));
      roots = list.map((c, i) => courseRoot(c.ID, c.Code, trees[i] ?? []));
      flatFiles.set(flatten(roots));
    } catch (err) {
      toast(`Could not load files: ${errMsg(err)}`, 'error');
      roots = [];
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    // reload whenever the course filter or course list changes
    void $selectedCourseID;
    void $courses;
    void loadTree();
  });

  function flatten(nodes: FileNode[], out: FileNode[] = []): FileNode[] {
    for (const n of nodes) {
      if (n.IsDir) flatten(n.Children ?? [], out);
      else out.push(n);
    }
    return out;
  }

  function findNode(nodes: FileNode[], key: string): FileNode | null {
    for (const n of nodes) {
      if (!n.IsDir) continue;
      if (keyOf(n) === key) return n;
      const hit = findNode(n.Children ?? [], key);
      if (hit) return hit;
    }
    return null;
  }

  const activeFolder = $derived(selectedFolder ? findNode(roots, selectedFolder) : null);

  /** Files shown in the right pane: everything under the selected folder. */
  const paneFiles = $derived(activeFolder ? flatten(activeFolder.Children ?? []) : flatten(roots));

  const visibleFiles = $derived.by(() => {
    const q = query.trim().toLowerCase();
    const list = q ? paneFiles.filter((f) => f.Name.toLowerCase().includes(q) || f.RelPath.toLowerCase().includes(q)) : paneFiles;
    return [...list].sort((a, b) => Date.parse(b.ModifiedAt) - Date.parse(a.ModifiedAt));
  });

  const totalBytes = $derived(visibleFiles.reduce((acc, f) => acc + f.Size, 0));

  function toggle(key: string) {
    const next = new Set(expanded);
    if (next.has(key)) next.delete(key);
    else next.add(key);
    expanded = next;
    lsSet(EXPANDED_KEY, [...next]);
  }

  function selectFolder(node: FileNode) {
    const key = keyOf(node);
    selectedFolder = selectedFolder === key ? '' : key;
    lsSet(SELECTED_KEY, selectedFolder);
    if (!expanded.has(key)) toggle(key);
  }

  const runSearch = debounce(async (q: string, cid: number) => {
    if (q.trim().length < 3) {
      serverHits = [];
      return;
    }
    searching = true;
    try {
      serverHits = (await api.search(q, cid)) ?? [];
    } catch (err) {
      console.error('[files] search failed', err);
      serverHits = [];
    } finally {
      searching = false;
    }
  }, 250);

  $effect(() => {
    runSearch(query, $selectedCourseID);
  });

  function onSearchKey(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      runSearch.cancel();
      void (async () => {
        searching = true;
        try {
          serverHits = (await api.search(query, $selectedCourseID)) ?? [];
        } finally {
          searching = false;
        }
      })();
    } else if (e.key === 'Escape') {
      query = '';
      serverHits = [];
    }
  }

  function activate(f: FileNode) {
    void openFileNode(f);
  }

  async function copyPath(f: FileNode) {
    try {
      await navigator.clipboard.writeText(f.Path);
      toast('Path copied to clipboard', 'success');
    } catch {
      toast('Could not access the clipboard', 'error');
    }
  }

  function openMenu(e: MouseEvent, f: FileNode) {
    e.preventDefault();
    selectedFile = keyOf(f);
    menu = {
      x: e.clientX,
      y: e.clientY,
      items: [
        { label: 'Open', icon: 'external', disabled: !f.Synced, run: () => activate(f) },
        {
          label: 'Reveal in Explorer',
          icon: 'folderOpen',
          disabled: !f.Synced,
          run: () => void api.revealFile(f.Path).catch((err) => toast(errMsg(err), 'error')),
        },
        { label: 'Copy path', icon: 'copy', run: () => void copyPath(f) },
      ],
    };
  }

  /** Snippet-bearing hits only — plain filename matches are already in the table. */
  const contentHits = $derived(serverHits.filter((h) => h.Snippet));

  const breadcrumb = $derived.by(() => {
    if (!activeFolder) return $selectedCourseID ? $courseByID.get($selectedCourseID)?.Code ?? 'All files' : 'All files';
    const code = $courseByID.get(activeFolder.CourseID)?.Code ?? '';
    return activeFolder.RelPath ? `${code} / ${activeFolder.RelPath.replace(/\//g, ' / ')}` : code;
  });
</script>

<div class="files">
  <div class="toolbar">
    <div class="search">
      <Icon name="search" size={14} />
      <input
        class="q"
        type="text"
        placeholder="Filter files… (Enter to search file contents)"
        bind:value={query}
        onkeydown={onSearchKey}
        spellcheck="false"
        autocomplete="off"
      />
      {#if searching}<span class="spinner"></span>{/if}
      {#if query}
        <button class="clear" onclick={() => { query = ''; serverHits = []; }} aria-label="Clear">
          <Icon name="x" size={12} />
        </button>
      {/if}
    </div>
    <select class="select course-pick" bind:value={$selectedCourseID}>
      <option value={0}>All courses</option>
      {#each $courses as c (c.ID)}
        <option value={c.ID}>{c.Code}</option>
      {/each}
    </select>
  </div>

  <div class="panes">
    <div class="tree-pane">
      <button
        class="all-row"
        class:sel={selectedFolder === ''}
        onclick={() => {
          selectedFolder = '';
          lsSet(SELECTED_KEY, '');
        }}
      >
        <Icon name="layers" size={14} />
        <span class="truncate">All files</span>
        <span class="n">{flatten(roots).length}</span>
      </button>
      <div class="tree-scroll">
        {#if loading}
          <div class="tree-loading"><span class="spinner"></span> Loading tree…</div>
        {:else}
          <FolderTree
            nodes={roots}
            {expanded}
            selected={selectedFolder}
            onToggle={toggle}
            onSelect={selectFolder}
            {keyOf}
          />
        {/if}
      </div>
    </div>

    <div class="list-pane">
      <div class="crumb">
        <span class="crumb-path truncate">{breadcrumb}</span>
        <span class="crumb-meta">{visibleFiles.length} files · {fmtBytes(totalBytes)}</span>
      </div>

      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th class="c-name">Name</th>
              <th class="c-mod">Module / folder</th>
              <th class="c-size">Size</th>
              <th class="c-time">Modified</th>
            </tr>
          </thead>
          <tbody>
            {#each visibleFiles as f (keyOf(f))}
              <tr
                class:sel={selectedFile === keyOf(f)}
                class:unsynced={!f.Synced}
                onclick={() => (selectedFile = keyOf(f))}
                ondblclick={() => activate(f)}
                oncontextmenu={(e) => openMenu(e, f)}
              >
                <td class="c-name">
                  <span class="cell">
                    <span class="dot" style="background:{$courseByID.get(f.CourseID)?.Color ?? 'var(--text-faint)'}"></span>
                    <Icon name={KIND_ICON[fileKind(f.Name)] ?? 'file'} size={14} />
                    <span class="truncate">{f.Name}</span>
                    {#if !f.Synced}<span class="chip ns">not synced</span>{/if}
                  </span>
                </td>
                <td class="c-mod truncate">{f.Module || f.RelPath.split('/').slice(0, -1).join(' / ') || '—'}</td>
                <td class="c-size">{fmtBytes(f.Size)}</td>
                <td class="c-time">{relTime(f.ModifiedAt, tick)}</td>
              </tr>
            {/each}
          </tbody>
        </table>

        {#if visibleFiles.length === 0 && !loading}
          <div class="empty">
            {#if query}No files match “{query}” here.{:else}This folder is empty.{/if}
          </div>
        {/if}

        {#if contentHits.length > 0}
          <div class="hits">
            <div class="hits-head section-title">Content matches ({contentHits.length})</div>
            {#each contentHits as h (h.CourseCode + h.File.RelPath)}
              <button class="hit" ondblclick={() => activate(h.File)} onclick={() => activate(h.File)}>
                <span class="hit-top">
                  <Icon name={KIND_ICON[fileKind(h.File.Name)] ?? 'file'} size={13} />
                  <span class="truncate hit-name">{h.File.Name}</span>
                  <span class="hit-course">{h.CourseCode}</span>
                </span>
                <span class="hit-snip truncate">{@html snippetHTML(h.Snippet)}</span>
              </button>
            {/each}
          </div>
        {/if}
      </div>
    </div>
  </div>
</div>

{#if menu}
  <ContextMenu x={menu.x} y={menu.y} items={menu.items} onClose={() => (menu = null)} />
{/if}

<style>
  .files {
    height: 100%;
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  .toolbar {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 12px 16px;
    border-bottom: 1px solid var(--border);
    flex: none;
  }

  .search {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 8px;
    height: 32px;
    padding: 0 10px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-input);
    color: var(--text-faint);
    transition: border-color var(--t), box-shadow var(--t);
  }

  .search:focus-within {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-soft);
  }

  .q {
    flex: 1;
    border: none;
    background: none;
    outline: none;
    font-size: 13px;
    color: var(--text);
  }

  .q::placeholder {
    color: var(--text-faint);
  }

  .clear {
    color: var(--text-faint);
    display: flex;
    padding: 2px;
    border-radius: 4px;
  }

  .clear:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .course-pick {
    width: 158px;
    flex: none;
  }

  .panes {
    flex: 1;
    min-height: 0;
    display: flex;
  }

  .tree-pane {
    width: 250px;
    flex: none;
    display: flex;
    flex-direction: column;
    min-height: 0;
    border-right: 1px solid var(--border);
    background: var(--bg-sidebar);
    padding: 8px;
  }

  .all-row {
    display: flex;
    align-items: center;
    gap: 7px;
    width: 100%;
    height: 28px;
    padding: 0 8px;
    margin-bottom: 2px;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    font-size: 12.5px;
    font-weight: 500;
    transition: background var(--t), color var(--t);
  }

  .all-row:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .all-row.sel {
    background: var(--bg-active);
    color: var(--accent-text);
  }

  .all-row .n {
    margin-left: auto;
    font-size: 10.5px;
    color: var(--text-faint);
    font-variant-numeric: tabular-nums;
  }

  .tree-scroll {
    flex: 1;
    min-height: 0;
    overflow: auto;
  }

  .tree-loading {
    display: flex;
    align-items: center;
    gap: 7px;
    padding: 10px 8px;
    font-size: 12px;
    color: var(--text-faint);
  }

  .list-pane {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  .crumb {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    height: 34px;
    padding: 0 16px;
    border-bottom: 1px solid var(--border);
    font-size: 12px;
    color: var(--text-muted);
    flex: none;
  }

  .crumb-path {
    font-weight: 550;
    color: var(--text);
  }

  .crumb-meta {
    flex: none;
    color: var(--text-faint);
    font-variant-numeric: tabular-nums;
  }

  .table-wrap {
    flex: 1;
    min-height: 0;
    overflow: auto;
  }

  .table {
    width: 100%;
    border-collapse: collapse;
    table-layout: fixed;
  }

  thead th {
    position: sticky;
    top: 0;
    z-index: 1;
    background: var(--bg);
    border-bottom: 1px solid var(--border);
    padding: 7px 12px;
    text-align: left;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.03em;
    text-transform: uppercase;
    color: var(--text-faint);
  }

  tbody tr {
    border-bottom: 1px solid var(--border);
    cursor: default;
    transition: background var(--t);
  }

  tbody tr:hover {
    background: var(--bg-hover);
  }

  tbody tr.sel {
    background: var(--bg-active);
  }

  tbody tr.unsynced {
    color: var(--text-faint);
  }

  td {
    padding: 7px 12px;
    font-size: 13px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .cell {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    color: var(--text-faint);
  }

  .cell .truncate {
    color: var(--text);
  }

  tbody tr.unsynced .cell .truncate {
    color: var(--text-faint);
  }

  .c-name {
    width: 46%;
  }

  .c-mod {
    width: 26%;
    color: var(--text-muted);
    font-size: 12.5px;
  }

  .c-size {
    width: 12%;
    color: var(--text-faint);
    font-size: 12.5px;
    font-variant-numeric: tabular-nums;
  }

  .c-time {
    width: 16%;
    color: var(--text-faint);
    font-size: 12.5px;
  }

  .chip.ns {
    height: 18px;
    font-size: 10.5px;
    flex: none;
  }

  .hits {
    border-top: 1px solid var(--border);
    padding: 12px 12px 20px;
  }

  .hits-head {
    padding: 0 4px 8px;
  }

  .hit {
    display: flex;
    flex-direction: column;
    gap: 2px;
    width: 100%;
    padding: 8px 10px;
    border-radius: var(--radius-sm);
    text-align: left;
    transition: background var(--t);
  }

  .hit:hover {
    background: var(--bg-hover);
  }

  .hit-top {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    color: var(--text-faint);
  }

  .hit-name {
    font-size: 13px;
    color: var(--text);
    font-weight: 500;
  }

  .hit-course {
    margin-left: auto;
    font-size: 11px;
    color: var(--text-faint);
  }

  .hit-snip {
    font-size: 12px;
    color: var(--text-muted);
    padding-left: 21px;
  }

  .hit-snip :global(mark) {
    background: var(--accent-soft, rgba(91, 91, 214, 0.18));
    color: var(--text);
    border-radius: 3px;
    padding: 0 1px;
  }
</style>
