<script lang="ts">
  import { untrack } from 'svelte';
  import { api, errMsg } from '../api';
  import ContextMenu from '../components/ContextMenu.svelte';
  import FolderTree from '../components/FolderTree.svelte';
  import Icon from '../components/Icon.svelte';
  import ResizeHandle from '../components/ResizeHandle.svelte';
  import Viewer from '../components/Viewer.svelte';
  import {
    LIST_RAIL_W,
    TREE_AUTOHIDE_BELOW,
    TREE_W_DEFAULT,
    TREE_W_MAX,
    TREE_W_MIN,
    listCollapsed,
    setTreeWidth,
    toggleListCollapsed,
    toggleTree,
    treeOpen,
    treeW,
    viewerFocus,
  } from '../layout';
  import { tip } from '../tooltip';
  import {
    courses,
    courseByID,
    flatFiles,
    openChat,
    openFileNode,
    revealFileID,
    selectedCourseID,
    setContext,
    toast,
  } from '../stores';
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
  const VIEWER_W_KEY = 'nussync.viewer.w';
  const VIEWER_MIN = 320;
  /**
   * Narrowest the list pane is still worth showing. Below this it collapses to
   * the strip rather than shrinking further — a 260px table of four columns is
   * the "S… / Lectur… / 4…" mess this replaced.
   */
  const LIST_MIN = 320;
  /** …and the width below which the viewer stops giving ground instead. */
  const VIEWER_FLOOR = 420;
  /** `ResizeHandle`'s own width, which the panes row also has to pay for. */
  const HANDLE_W = 5;

  /**
   * Column budget for the list, measured on the pane itself (not the window —
   * the tree, the viewer and a docked chat all change it independently). Each
   * column drops out at the point where keeping it would start eating the name.
   */
  const COL_TIME_MIN = 560;
  const COL_SIZE_MIN = 470;
  const COL_MOD_MIN = 380;
  /** Below this the "New" badge is a dot; the word no longer earns its width. */
  const CHIP_DOT_BELOW = 360;

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
  let viewerOpen = $state(false);
  let viewerW = $state(Math.max(VIEWER_MIN, lsGet<number>(VIEWER_W_KEY, 520)));
  let panesEl = $state<HTMLDivElement | null>(null);
  let listEl = $state<HTMLDivElement | null>(null);
  let tableWrapEl = $state<HTMLDivElement | null>(null);
  /** Measured width of the panes row, for the tree's auto-hide rule. */
  let panesW = $state(1200);
  /** Measured width of the list pane, for the responsive columns. */
  let listW = $state(900);

  $effect(() => {
    const el = panesEl;
    if (!el || typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver(([entry]) => (panesW = entry.contentRect.width));
    ro.observe(el);
    return () => ro.disconnect();
  });

  $effect(() => {
    const el = listEl;
    if (!el || typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver(([entry]) => (listW = entry.contentRect.width));
    ro.observe(el);
    return () => ro.disconnect();
  });

  /**
   * Which optional columns survive at the current pane width. Dropping them in
   * this order — Modified, then Size, then Module/folder — always leaves the
   * name the space, which is the only column you cannot guess from context.
   */
  const cols = $derived({
    mod: listW >= COL_MOD_MIN,
    size: listW >= COL_SIZE_MIN,
    time: listW >= COL_TIME_MIN,
  });

  /** Fixed-table column widths for whichever set is showing. */
  const colW = $derived.by(() => {
    if (cols.time) return { name: '46%', mod: '26%', size: '12%', time: '16%' };
    if (cols.size) return { name: '54%', mod: '30%', size: '16%', time: '0' };
    if (cols.mod) return { name: '64%', mod: '36%', size: '0', time: '0' };
    return { name: '100%', mod: '0', size: '0', time: '0' };
  });

  const chipDot = $derived(listW < CHIP_DOT_BELOW);
  /** Narrow enough that the full crumb trail would push the count off the row. */
  const crumbTight = $derived(listW < COL_TIME_MIN);

  /**
   * The tree yields to the viewer on a narrow content area — the breadcrumb is
   * still there to navigate with, which is exactly why it grew an "All files"
   * root. The user's own preference is untouched, so it comes back on resize.
   */
  const treeShown = $derived($treeOpen && !(viewerOpen && panesW < TREE_AUTOHIDE_BELOW));

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
      IsNew: false,
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

  /** 1-based position of the selection, or 0 when nothing is selected. */
  const selIndex = $derived(visibleFiles.findIndex((f) => keyOf(f) === selectedFile) + 1);

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

  /**
   * Single click: select, make this the chat panel's context, and preview.
   * Double click still hands the file to the OS (see `activate`).
   */
  function select(f: FileNode, preview = true) {
    selectedFile = keyOf(f);
    setContext(f.ID, '', f.Name);
    if (preview) viewerOpen = true;
  }

  /** The node the viewer pane is showing, resolved from the selection key. */
  const previewFile = $derived(
    selectedFile
      ? visibleFiles.find((f) => keyOf(f) === selectedFile) ?? paneFiles.find((f) => keyOf(f) === selectedFile) ?? null
      : null,
  );

  /**
   * The chat's context chip asks for its file to be shown here. The folder
   * filter and the search box are both cleared first, otherwise the row we
   * want to scroll to may not be in the table at all.
   */
  $effect(() => {
    const id = $revealFileID;
    if (!id) return;
    revealFileID.set(0);
    const node = untrack(() => $flatFiles).find((f) => f.ID === id);
    if (!node) return;
    untrack(() => {
      if (query) {
        query = '';
        serverHits = [];
      }
      if (selectedFolder && !paneFiles.some((f) => f.ID === id)) {
        selectedFolder = '';
        lsSet(SELECTED_KEY, '');
      }
      selectedFile = keyOf(node);
      setContext(node.ID, '', node.Name);
    });
    queueMicrotask(() => tableWrapEl?.querySelector('tr.sel')?.scrollIntoView({ block: 'center' }));
  });

  function closeViewer() {
    viewerOpen = false;
  }

  function toggleViewer() {
    if (viewerOpen) {
      viewerOpen = false;
      return;
    }
    if (!previewFile && visibleFiles.length > 0) select(visibleFiles[0], false);
    viewerOpen = true;
  }

  /** Move the selection up/down the list while the viewer is open. */
  function step(delta: number) {
    const list = visibleFiles;
    if (list.length === 0) return;
    const i = list.findIndex((f) => keyOf(f) === selectedFile);
    const next = list[Math.min(list.length - 1, Math.max(0, (i < 0 ? 0 : i) + delta))];
    // Follow with the viewer only if it is already open — stepping through a
    // collapsed list should not conjure a preview pane.
    if (next) select(next, viewerOpen);
  }

  /** True while the user is typing somewhere a bare arrow key belongs to them. */
  function inField(t: EventTarget | null): boolean {
    const el = t as HTMLElement | null;
    if (!el) return false;
    return el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName);
  }

  function onKey(e: KeyboardEvent) {
    // The docked chat is a sibling pane, not part of this view — its keys
    // (Esc to close, arrows in the composer) are its own.
    if ((e.target as HTMLElement | null)?.closest?.('aside.chat')) return;
    const mod = e.ctrlKey || e.metaKey;
    // Ctrl+Shift+P belongs to the viewer's focus mode, so shift is excluded.
    if (mod && !e.shiftKey && (e.key === 'p' || e.key === 'P')) {
      e.preventDefault();
      toggleViewer();
      return;
    }
    if (mod && e.shiftKey && (e.key === 'e' || e.key === 'E')) {
      e.preventDefault();
      toggleTree();
      return;
    }
    if (mod && e.shiftKey && (e.key === 'l' || e.key === 'L')) {
      e.preventDefault();
      toggleListCollapsed();
      return;
    }
    // In focus mode the Viewer owns Esc and the arrows.
    if ($viewerFocus) return;
    if (e.key === 'Escape' && viewerOpen && !menu) {
      // Only when the search box is not the one asking to be cleared.
      if (!(inField(e.target) && query)) {
        e.preventDefault();
        closeViewer();
      }
      return;
    }
    // Arrows drive the selection whenever the list is not the thing you would
    // be clicking in: a viewer to update, or a collapsed strip to step through.
    if ((!viewerOpen && !listCollapsedEff) || inField(e.target)) return;
    if (e.key === 'ArrowDown' || e.key === 'ArrowRight') {
      e.preventDefault();
      step(1);
    } else if (e.key === 'ArrowUp' || e.key === 'ArrowLeft') {
      e.preventDefault();
      step(-1);
    }
  }

  // ------------------------------------------------------- viewer resizing

  /**
   * Width left for the list and the viewer once the tree (and the dividers)
   * have taken their share. `panesW` is measured, so a docked chat column has
   * already been subtracted by the time this runs.
   */
  const availW = $derived(
    Math.max(0, panesW - (treeShown ? $treeW + HANDLE_W : 0) - (viewerOpen ? HANDLE_W : 0)),
  );

  /**
   * The list gives up its table before the viewer gives up its width: once the
   * row cannot seat both a readable list and a usable viewer, the list goes to
   * its strip. `listCollapsed` is the user's own choice and is never written by
   * this, so expanding again works the moment there is room.
   */
  const listCollapsedEff = $derived($listCollapsed || (viewerOpen && availW - VIEWER_FLOOR < LIST_MIN));

  /** The list's share of the row: the strip when collapsed, LIST_MIN otherwise. */
  const listKeep = $derived(listCollapsedEff ? LIST_RAIL_W : LIST_MIN);

  /** Always leave room for the list pane (and the tree, when it is showing). */
  const viewerMax = $derived(Math.max(VIEWER_MIN, availW - listKeep));

  /**
   * What the viewer pane is actually given, as opposed to what the user asked
   * for. Squeezing the content area — opening the chat, narrowing the window —
   * spends the space in a fixed order: the tree hides (see `treeShown`), then
   * the list collapses to its strip (see `listCollapsedEff`) while the viewer
   * keeps its preference, and only then does the viewer give ground, down to
   * VIEWER_FLOOR. Past that even the strip yields, because a 300px PDF is no
   * use to anyone.
   *
   * `viewerW` is never written by this, so the preference comes back untouched
   * as soon as there is room for it again.
   */
  const effViewerW = $derived.by(() => {
    if (!viewerOpen) return viewerW;
    const wanted = Math.min(viewerW, Math.max(availW - listKeep, VIEWER_FLOOR));
    // Never wider than the row itself, whatever the floor says.
    return Math.max(VIEWER_MIN, Math.min(wanted, Math.max(VIEWER_MIN, availW - LIST_RAIL_W)));
  });

  function setViewerW(v: number) {
    viewerW = Math.min(viewerMax, Math.max(VIEWER_MIN, v));
    lsSet(VIEWER_W_KEY, Math.round(viewerW));
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
    select(f);
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
        {
          label: 'Chat about this',
          icon: 'chat',
          run: () => openChat({ fileID: f.ID, paperID: '', name: f.Name }),
        },
      ],
    };
  }

  /** Snippet-bearing hits only — plain filename matches are already in the table. */
  const contentHits = $derived(serverHits.filter((h) => h.Snippet));

  /**
   * Clickable crumb trail. With the tree hidden this is the only way up, so it
   * always starts at an "All files" root, then the course, then each folder.
   * `key` matches `keyOf`, which makes every crumb directly selectable.
   */
  const crumbs = $derived.by(() => {
    const out: Array<{ label: string; key: string }> = [{ label: 'All files', key: '' }];
    if (!activeFolder) return out;
    const cid = activeFolder.CourseID;
    out.push({ label: $courseByID.get(cid)?.Code ?? '—', key: `${cid}:` });
    let acc = '';
    for (const seg of activeFolder.RelPath ? activeFolder.RelPath.split('/') : []) {
      acc = acc ? `${acc}/${seg}` : seg;
      out.push({ label: seg, key: `${cid}:${acc}` });
    }
    return out;
  });

  /**
   * What the header actually renders. Narrow, the middle of the trail becomes a
   * single "…" that still navigates — to the parent, which is the crumb anyone
   * reaches for — so the row never wraps or shoves the file count off the end.
   */
  const shownCrumbs = $derived(
    crumbTight && crumbs.length > 2
      ? [{ label: '…', key: crumbs[crumbs.length - 2].key }, crumbs[crumbs.length - 1]]
      : crumbs,
  );

  const breadcrumb = $derived(crumbs.map((c) => c.label).join(' / '));

  /**
   * `tip`, but only once the name is actually clipped. The check runs on hover
   * rather than up front because the pane resizes under the row; registering it
   * *before* `tip` matters, so the freshly computed `disabled` is the one the
   * tooltip's own pointerenter handler reads.
   */
  function nameTip(node: HTMLElement, text: string) {
    let cur = text;
    let t: ReturnType<typeof tip> | undefined;
    const check = () =>
      t?.update({ text: cur, side: 'top', disabled: node.scrollWidth <= node.clientWidth + 1 });
    node.addEventListener('pointerenter', check);
    t = tip(node, { text, side: 'top', disabled: true });
    return {
      update(next: string) {
        cur = next;
        t?.update({ text: next, side: 'top', disabled: true });
      },
      destroy() {
        node.removeEventListener('pointerenter', check);
        t?.destroy();
      },
    };
  }

  function goCrumb(key: string) {
    selectedFolder = key;
    lsSet(SELECTED_KEY, key);
  }
</script>

<div class="files">
  <div class="toolbar">
    <button
      class="btn sm tree-toggle"
      class:on={$treeOpen}
      onclick={toggleTree}
      title="Toggle the folder tree (Ctrl+Shift+E)"
      aria-label="Toggle the folder tree"
      aria-pressed={$treeOpen}
    >
      <Icon name="panelLeft" size={14} />
    </button>
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
    <button
      class="btn sm"
      class:on={viewerOpen}
      onclick={toggleViewer}
      title="Toggle the preview pane (Ctrl+P)"
      aria-pressed={viewerOpen}
    >
      <Icon name={viewerOpen ? 'eyeOff' : 'eye'} size={13} />
      Preview
    </button>
  </div>

  <div class="panes" bind:this={panesEl}>
    {#if treeShown}
    <div class="tree-pane" style="width:{$treeW}px">
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
    <ResizeHandle
      value={$treeW}
      min={TREE_W_MIN}
      max={TREE_W_MAX}
      def={TREE_W_DEFAULT}
      label="Resize the folder tree"
      onChange={setTreeWidth}
    />
    {/if}

    <div class="list-pane" class:collapsed={listCollapsedEff} bind:this={listEl}>
      {#if listCollapsedEff}
        <!--
          The strip. The whole thing expands on click, so the two step buttons
          have to stop the event from reaching it — otherwise every ▲ press
          would also throw the table back open.
        -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="list-rail"
          role="button"
          tabindex="0"
          aria-label="Expand the file list (Ctrl+Shift+L)"
          onclick={() => listCollapsed.set(false)}
          onkeydown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault();
              listCollapsed.set(false);
            }
          }}
        >
          <button
            class="rail-btn"
            onclick={(e) => { e.stopPropagation(); step(-1); }}
            title="Previous file (↑)"
            aria-label="Previous file"
            disabled={visibleFiles.length === 0}
          >
            <Icon name="chevronUp" size={13} />
          </button>
          <span class="rail-label">{visibleFiles.length} files</span>
          <span class="rail-idx">{selIndex || '–'}/{visibleFiles.length}</span>
          <button
            class="rail-btn"
            onclick={(e) => { e.stopPropagation(); step(1); }}
            title="Next file (↓)"
            aria-label="Next file"
            disabled={visibleFiles.length === 0}
          >
            <Icon name="chevronDown" size={13} />
          </button>
        </div>
      {:else}
      <div class="crumb">
        <nav class="crumb-path" aria-label="Folder breadcrumb">
          {#each shownCrumbs as c, i (c.key)}
            {#if i > 0}<span class="sep" aria-hidden="true">/</span>{/if}
            <button
              class="crumb-btn truncate"
              class:last={i === shownCrumbs.length - 1}
              onclick={() => goCrumb(c.key)}
              disabled={i === shownCrumbs.length - 1}
            >{c.label}</button>
          {/each}
        </nav>
        <span class="crumb-meta">{visibleFiles.length} files{crumbTight ? '' : ` · ${fmtBytes(totalBytes)}`}</span>
        <button
          class="crumb-collapse"
          onclick={toggleListCollapsed}
          title="Collapse the file list (Ctrl+Shift+L)"
          aria-label="Collapse the file list"
        >
          <Icon name="chevronLeft" size={13} />
        </button>
      </div>

      <div class="table-wrap" bind:this={tableWrapEl}>
        <table class="table">
          <thead>
            <tr>
              <th class="c-name" style="width:{colW.name}">Name</th>
              {#if cols.mod}<th class="c-mod" style="width:{colW.mod}">Module / folder</th>{/if}
              {#if cols.size}<th class="c-size" style="width:{colW.size}">Size</th>{/if}
              {#if cols.time}<th class="c-time" style="width:{colW.time}">Modified</th>{/if}
            </tr>
          </thead>
          <tbody>
            {#each visibleFiles as f (keyOf(f))}
              <tr
                class:sel={selectedFile === keyOf(f)}
                class:unsynced={!f.Synced}
                onclick={() => select(f)}
                ondblclick={() => activate(f)}
                oncontextmenu={(e) => openMenu(e, f)}
              >
                <td class="c-name">
                  <span class="cell">
                    <span class="dot" style="background:{$courseByID.get(f.CourseID)?.Color ?? 'var(--text-faint)'}"></span>
                    <Icon name={KIND_ICON[fileKind(f.Name)] ?? 'file'} size={14} />
                    <span class="truncate" use:nameTip={f.Name}>{f.Name}</span>
                    {#if f.IsNew}
                      <span
                        class="chip nw"
                        class:dot-only={chipDot}
                        title="Changed since you last opened What's new"
                      >{chipDot ? '' : 'New'}</span>
                    {/if}
                    {#if !f.Synced && !chipDot}<span class="chip ns">not synced</span>{/if}
                  </span>
                </td>
                {#if cols.mod}
                  <td class="c-mod truncate">{f.Module || f.RelPath.split('/').slice(0, -1).join(' / ') || '—'}</td>
                {/if}
                {#if cols.size}<td class="c-size">{fmtBytes(f.Size)}</td>{/if}
                {#if cols.time}<td class="c-time">{relTime(f.ModifiedAt, tick)}</td>{/if}
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
      {/if}
    </div>

    {#if viewerOpen}
      <ResizeHandle
        value={effViewerW}
        min={VIEWER_MIN}
        max={viewerMax}
        def={520}
        invert
        label="Resize the preview pane"
        onChange={setViewerW}
      />
      <div class="viewer-pane" style="width:{effViewerW}px">
        <Viewer
          file={previewFile}
          onClose={closeViewer}
          crumb={previewFile ? `${breadcrumb} / ${previewFile.Name}` : breadcrumb}
          onPrev={visibleFiles.length > 1 ? () => step(-1) : undefined}
          onNext={visibleFiles.length > 1 ? () => step(1) : undefined}
        />
      </div>
    {/if}
  </div>
</div>

<svelte:window onkeydown={onKey} />

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

  .viewer-pane {
    flex: none;
    min-width: 0;
    min-height: 0;
    display: flex;
  }

  .viewer-pane :global(.viewer) {
    flex: 1;
    min-width: 0;
  }

  .tree-toggle {
    flex: none;
    width: 32px;
    padding: 0;
    justify-content: center;
    color: var(--text-faint);
  }

  .tree-toggle.on {
    color: var(--accent-text);
    border-color: var(--border-strong);
  }

  .tree-pane {
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

  .list-pane.collapsed {
    flex: none;
    width: 44px;
    border-right: 1px solid var(--border);
    background: var(--bg-subtle);
  }

  .list-rail {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    height: 100%;
    padding: 10px 0;
    cursor: pointer;
    color: var(--text-faint);
    transition: background var(--t);
  }

  .list-rail:hover,
  .list-rail:focus-visible {
    background: var(--bg-hover);
    outline: none;
  }

  .rail-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    flex: none;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    transition: background var(--t), color var(--t);
  }

  .rail-btn:hover:not(:disabled) {
    background: var(--bg-active);
    color: var(--text);
  }

  .rail-btn:disabled {
    opacity: 0.4;
    cursor: default;
  }

  /* Bottom-to-top so the label reads upward, the way a spine label does. */
  .rail-label {
    flex: 1;
    min-height: 0;
    writing-mode: vertical-rl;
    transform: rotate(180deg);
    text-align: center;
    overflow: hidden;
    white-space: nowrap;
    font-size: 11.5px;
    font-weight: 550;
    letter-spacing: 0.02em;
    color: var(--text-muted);
  }

  .rail-idx {
    flex: none;
    writing-mode: vertical-rl;
    transform: rotate(180deg);
    font-size: 10.5px;
    font-variant-numeric: tabular-nums;
    color: var(--text-faint);
  }

  .crumb {
    display: flex;
    align-items: center;
    gap: 10px;
    height: 34px;
    padding: 0 16px;
    border-bottom: 1px solid var(--border);
    font-size: 12px;
    color: var(--text-muted);
    flex: none;
  }

  .crumb-path {
    display: flex;
    align-items: center;
    gap: 5px;
    min-width: 0;
    flex: 1;
    font-weight: 550;
    color: var(--text);
  }

  .crumb-btn {
    max-width: 220px;
    padding: 1px 4px;
    border-radius: 4px;
    color: var(--text-muted);
    font-size: 12px;
    font-weight: 550;
    transition: background var(--t), color var(--t);
  }

  .crumb-btn:hover:not(:disabled) {
    background: var(--bg-hover);
    color: var(--text);
  }

  .crumb-btn.last {
    color: var(--text);
    cursor: default;
  }

  .sep {
    color: var(--text-faint);
    flex: none;
  }

  .crumb-meta {
    flex: none;
    color: var(--text-faint);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .crumb-collapse {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 22px;
    flex: none;
    margin-right: -6px;
    border-radius: var(--radius-sm);
    color: var(--text-faint);
    transition: background var(--t), color var(--t);
  }

  .crumb-collapse:hover {
    background: var(--bg-hover);
    color: var(--text);
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
    /* Never wrap: a two-line "MODULE / FOLDER" is what started all this. */
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
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

  .c-mod {
    color: var(--text-muted);
    font-size: 12.5px;
  }

  .c-size {
    color: var(--text-faint);
    font-size: 12.5px;
    font-variant-numeric: tabular-nums;
  }

  .c-time {
    color: var(--text-faint);
    font-size: 12.5px;
  }

  .chip.nw {
    height: 17px;
    padding: 0 6px;
    font-size: 10.5px;
    background: var(--accent-soft);
    color: var(--accent-text);
    flex: none;
  }

  /* Too narrow for the word: keep the signal, drop the label. */
  .chip.nw.dot-only {
    width: 7px;
    min-width: 7px;
    height: 7px;
    padding: 0;
    border-radius: 50%;
    background: var(--accent);
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
