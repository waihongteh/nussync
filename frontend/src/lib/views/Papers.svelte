<script lang="ts">
  /**
   * Papers — search arXiv / Semantic Scholar, keep a reading library, and hang
   * the on-demand Claude features (key-point summary, chat) off each entry.
   *
   * Google Scholar has no API and is never scraped: the only thing we do with
   * it is hand the query to the browser (OpenScholar).
   */
  import { api, errMsg, on } from '../api';
  import Icon from '../components/Icon.svelte';
  import { openChat, setContext, studyFile, toast } from '../stores';
  import type { CitationLink, LibraryPaper, Paper, PaperDigest, PaperSummary, StudyJob } from '../types';
  import { lsGet, lsSet, markdownToHTML, relTime } from '../util';

  const MODEL_KEY = 'nussync.study.model';
  const SOURCE_KEY = 'nussync.papers.source';
  const SORT_KEY = 'nussync.papers.sort';

  const SOURCES = [
    { v: 'all', label: 'All' },
    { v: 'arxiv', label: 'arXiv' },
    { v: 's2', label: 'S2' },
  ] as const;

  const STATUS_TABS = [
    { v: 'toread', label: 'To read' },
    { v: 'reading', label: 'Reading' },
    { v: 'done', label: 'Done' },
    { v: 'all', label: 'All' },
  ] as const;

  const STATUS_OPTIONS = [
    { v: 'toread', label: 'To read' },
    { v: 'reading', label: 'Reading' },
    { v: 'done', label: 'Done' },
  ];

  const SORTS = [
    { v: 'added', label: 'Recently added' },
    { v: 'citations', label: 'Most cited' },
    { v: 'year', label: 'Newest first' },
  ] as const;

  type Sort = (typeof SORTS)[number]['v'];

  // ------------------------------------------------------------- state

  let query = $state('');
  let source = $state<string>(lsGet(SOURCE_KEY, 'all'));
  let results = $state<Paper[]>([]);
  let total = $state(0);
  let searching = $state(false);
  let searched = $state(false);

  let library = $state<LibraryPaper[]>([]);
  let libLoading = $state(true);
  let statusTab = $state<string>('all');
  let sort = $state<Sort>(lsGet(SORT_KEY, 'added'));

  let expanded = $state<Set<string>>(new Set());
  let openCards = $state<Set<string>>(new Set());
  let selected = $state<Set<string>>(new Set());
  let tagDrafts = $state<Record<string, string>>({});

  let summaries = $state<Record<string, PaperSummary>>({});
  let summaryJob = $state<{ id: string; paperID: string; line: string } | null>(null);
  let model = $state<string>(lsGet(MODEL_KEY, 'sonnet'));

  let drawer = $state<{ paperID: string; kind: 'citations' | 'references' } | null>(null);
  let drawerLinks = $state<CitationLink[]>([]);
  let drawerLoading = $state(false);

  let recs = $state<Paper[]>([]);
  let digest = $state<PaperDigest | null>(null);
  let sendingDigest = $state(false);

  let bib = $state<string>('');
  let bibOpen = $state(false);
  let bibBusy = $state(false);

  let busyIDs = $state<Set<string>>(new Set());

  const libIDs = $derived(new Set(library.map((p) => p.ID)));

  const counts = $derived.by(() => {
    const c: Record<string, number> = { toread: 0, reading: 0, done: 0, all: library.length };
    for (const p of library) c[p.Status] = (c[p.Status] ?? 0) + 1;
    return c;
  });

  const visibleLibrary = $derived.by(() => {
    const list = statusTab === 'all' ? library : library.filter((p) => p.Status === statusTab);
    const sorted = [...list];
    if (sort === 'citations') sorted.sort((a, b) => b.CitationCount - a.CitationCount);
    else if (sort === 'year') sorted.sort((a, b) => b.Year - a.Year || b.CitationCount - a.CitationCount);
    else sorted.sort((a, b) => Date.parse(b.AddedAt) - Date.parse(a.AddedAt));
    return sorted;
  });

  $effect(() => {
    lsSet(SOURCE_KEY, source);
  });
  $effect(() => {
    lsSet(SORT_KEY, sort);
  });

  // -------------------------------------------------------------- load

  async function loadLibrary() {
    libLoading = true;
    try {
      library = (await api.getLibrary('all')) ?? [];
      await Promise.all(library.map((p) => loadSummary(p.ID)));
    } catch (err) {
      toast(`Could not load the library: ${errMsg(err)}`, 'error');
      library = [];
    } finally {
      libLoading = false;
    }
  }

  async function loadSummary(paperID: string) {
    try {
      const s = await api.getPaperSummary(paperID);
      if (s?.Markdown) summaries = { ...summaries, [paperID]: s };
    } catch {
      /* a missing summary is the normal case, not an error */
    }
  }

  async function loadRail() {
    try {
      recs = (await api.getRecommendations(6)) ?? [];
    } catch (err) {
      console.error('[papers] recommendations failed', err);
      recs = [];
    }
    try {
      digest = await api.getPaperDigest('');
    } catch (err) {
      console.error('[papers] digest failed', err);
      digest = null;
    }
  }

  $effect(() => {
    void loadLibrary();
    void loadRail();
  });

  $effect(() => {
    const off = on('papers:updated', () => {
      void loadLibrary();
    });
    const offJob = on('study:job', (j: StudyJob) => {
      if (!j || j.Kind !== 'paper_summary') return;
      if (summaryJob && j.ID !== summaryJob.id) return;
      if (j.Status === 'queued' || j.Status === 'running') {
        summaryJob = { id: j.ID, paperID: summaryJob?.paperID ?? '', line: j.Progress || 'Working…' };
      } else if (j.Status === 'done') {
        const pid = summaryJob?.paperID ?? '';
        summaryJob = null;
        if (pid) {
          void loadSummary(pid);
          openCards = new Set([...openCards, pid]);
        }
      } else {
        if (j.Status === 'error') toast(j.Error || 'Summary failed', 'error');
        summaryJob = null;
      }
    });
    return () => {
      off();
      offJob();
    };
  });

  // ------------------------------------------------------------ search

  async function runSearch() {
    const q = query.trim();
    if (!q) return;
    searching = true;
    searched = true;
    try {
      const res = await api.searchPapers(q, source, 20);
      results = res?.Papers ?? [];
      total = res?.Total ?? results.length;
    } catch (err) {
      toast(`Search failed: ${errMsg(err)}`, 'error');
      results = [];
      total = 0;
    } finally {
      searching = false;
    }
  }

  function onSearchKey(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault();
      void runSearch();
    }
  }

  async function scholar() {
    try {
      await api.openScholar(query.trim());
    } catch (err) {
      toast(errMsg(err), 'error');
    }
  }

  // ---------------------------------------------------------- actions

  function mark(id: string, on_: boolean) {
    const next = new Set(busyIDs);
    if (on_) next.add(id);
    else next.delete(id);
    busyIDs = next;
  }

  async function add(p: Paper) {
    if (libIDs.has(p.ID)) return;
    mark(p.ID, true);
    try {
      await api.addPaperToLibrary(p);
      await loadLibrary();
    } catch (err) {
      toast(errMsg(err), 'error');
    } finally {
      mark(p.ID, false);
    }
  }

  async function download(p: Paper) {
    mark(p.ID, true);
    try {
      await api.downloadPaperPDF(p.ID);
      await loadLibrary();
    } catch (err) {
      toast(`Download failed: ${errMsg(err)}`, 'error');
    } finally {
      mark(p.ID, false);
    }
  }

  async function openPaper(p: Paper) {
    try {
      await api.openURL(p.URL);
    } catch (err) {
      toast(errMsg(err), 'error');
    }
  }

  async function remove(p: LibraryPaper) {
    try {
      await api.removePaperFromLibrary(p.ID);
      library = library.filter((x) => x.ID !== p.ID);
    } catch (err) {
      toast(errMsg(err), 'error');
    }
  }

  // ------------------------------------------------- library card edits

  const saveTimers = new Map<string, ReturnType<typeof setTimeout>>();

  /** Patch a library row locally, then push it to the backend (debounced). */
  function patch(p: LibraryPaper, fields: Partial<LibraryPaper>, immediate = false) {
    const next = { ...p, ...fields } as LibraryPaper;
    library = library.map((x) => (x.ID === p.ID ? next : x));
    const existing = saveTimers.get(p.ID);
    if (existing) clearTimeout(existing);
    const push = async () => {
      saveTimers.delete(p.ID);
      try {
        const saved = await api.updateLibraryPaper($state.snapshot(next) as LibraryPaper);
        if (saved?.ID) library = library.map((x) => (x.ID === saved.ID ? saved : x));
      } catch (err) {
        toast(`Could not save: ${errMsg(err)}`, 'error');
      }
    };
    if (immediate) void push();
    else saveTimers.set(p.ID, setTimeout(push, 700));
  }

  function addTag(p: LibraryPaper) {
    const raw = (tagDrafts[p.ID] ?? '').trim().replace(/,+$/, '');
    if (!raw) return;
    const tags = p.Tags ?? [];
    if (!tags.includes(raw)) patch(p, { Tags: [...tags, raw] }, true);
    tagDrafts = { ...tagDrafts, [p.ID]: '' };
  }

  function removeTag(p: LibraryPaper, tag: string) {
    patch(p, { Tags: (p.Tags ?? []).filter((t) => t !== tag) }, true);
  }

  function clampPage(p: LibraryPaper, v: number) {
    const pages = p.Pages || 0;
    const page = Math.max(0, pages ? Math.min(pages, Math.round(v || 0)) : Math.round(v || 0));
    patch(p, { Page: page });
  }

  // ------------------------------------------------------- claude bits

  async function summarise(p: LibraryPaper) {
    if (summaryJob) return;
    try {
      const id = await api.startPaperSummary(p.ID, model);
      summaryJob = { id, paperID: p.ID, line: 'Queued' };
    } catch (err) {
      toast(errMsg(err), 'error');
    }
  }

  async function cancelSummary() {
    if (!summaryJob) return;
    try {
      await api.cancelStudyJob(summaryJob.id);
    } catch (err) {
      toast(errMsg(err), 'error');
    }
    summaryJob = null;
  }

  function chatAbout(p: LibraryPaper) {
    openChat({ fileID: p.FileID, paperID: p.ID, name: p.Title });
  }

  function study(p: LibraryPaper) {
    if (!p.FileID) {
      toast('Download the PDF first — Study reads the local file', 'info');
      return;
    }
    setContext(p.FileID, p.ID, p.Title);
    studyFile(p.FileID);
  }

  // ------------------------------------------------------------ drawer

  async function toggleDrawer(p: LibraryPaper, kind: 'citations' | 'references') {
    if (drawer && drawer.paperID === p.ID && drawer.kind === kind) {
      drawer = null;
      return;
    }
    drawer = { paperID: p.ID, kind };
    drawerLoading = true;
    drawerLinks = [];
    try {
      drawerLinks = (kind === 'citations' ? await api.getCitations(p.ID, 12) : await api.getReferences(p.ID, 12)) ?? [];
    } catch (err) {
      toast(errMsg(err), 'error');
      drawerLinks = [];
    } finally {
      drawerLoading = false;
    }
  }

  // ------------------------------------------------------------ bibtex

  async function exportBib() {
    bibBusy = true;
    try {
      const ids = selected.size ? [...selected] : library.map((p) => p.ID);
      bib = await api.exportBibTeX(ids);
      bibOpen = true;
    } catch (err) {
      toast(errMsg(err), 'error');
    } finally {
      bibBusy = false;
    }
  }

  async function copyBib() {
    try {
      await navigator.clipboard.writeText(bib);
      toast('BibTeX copied', 'success');
    } catch {
      toast('Could not access the clipboard', 'error');
    }
  }

  function toggleSelected(id: string) {
    const next = new Set(selected);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    selected = next;
  }

  function toggleExpanded(id: string) {
    const next = new Set(expanded);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    expanded = next;
  }

  function toggleCard(id: string) {
    const next = new Set(openCards);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    openCards = next;
  }

  async function sendDigest() {
    sendingDigest = true;
    try {
      await api.sendPaperDigestNow();
    } catch (err) {
      toast(errMsg(err), 'error');
    } finally {
      sendingDigest = false;
    }
  }

  // ------------------------------------------------------------ helpers

  function authorLine(p: Paper): string {
    const a = p.Authors ?? [];
    if (!a.length) return 'Unknown authors';
    if (a.length <= 3) return a.join(', ');
    return `${a.slice(0, 3).join(', ')} +${a.length - 3} more`;
  }

  function metaLine(p: Paper): string {
    const bits = [p.Year ? String(p.Year) : '', p.Venue].filter(Boolean);
    return bits.join(' · ');
  }

  function blurb(p: Paper): string {
    return p.TLDR || p.Abstract || '';
  }

  function statusLabel(s: string): string {
    return STATUS_OPTIONS.find((o) => o.v === s)?.label ?? s;
  }
  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape' && bibOpen) {
      e.stopPropagation();
      bibOpen = false;
    }
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="papers">
  <!-- ----------------------------------------------------------- main -->
  <div class="col">
    <div class="searchbar">
      <div class="search">
        <Icon name="search" size={14} />
        <input
          class="q"
          type="text"
          placeholder="Search papers — “LLM unlearning”, “knowledge editing”…"
          bind:value={query}
          onkeydown={onSearchKey}
          spellcheck="false"
          autocomplete="off"
        />
        {#if searching}<span class="spinner"></span>{/if}
        {#if query}
          <button class="clear" onclick={() => { query = ''; results = []; searched = false; }} aria-label="Clear">
            <Icon name="x" size={12} />
          </button>
        {/if}
      </div>

      <div class="seg" role="group" aria-label="Source">
        {#each SOURCES as s (s.v)}
          <button class="seg-btn" class:on={source === s.v} onclick={() => (source = s.v)}>{s.label}</button>
        {/each}
      </div>

      <button class="btn" onclick={() => void runSearch()} disabled={!query.trim() || searching}>Search</button>
      <button class="btn" onclick={scholar} title="Google Scholar has no API — this just opens it in your browser">
        <Icon name="external" size={13} /> Open in Google Scholar
      </button>
    </div>

    <!-- ------------------------------------------------------ results -->
    {#if searched}
      <section class="block">
        <header class="block-head">
          <h2 class="section-title">Results</h2>
          <span class="faint count">{searching ? 'searching…' : `${results.length} of ${total}`}</span>
        </header>

        {#if searching}
          <div class="card"><div class="empty"><span class="spinner"></span> Searching…</div></div>
        {:else if results.length === 0}
          <div class="card"><div class="empty">Nothing matched “{query}”. Try a broader phrase, or a different source.</div></div>
        {:else}
          <div class="cards">
            {#each results as p (p.ID)}
              {@const inLib = libIDs.has(p.ID)}
              <article class="card rcard">
                <h3 class="ptitle">{p.Title}</h3>
                <p class="pauthors truncate">{authorLine(p)}</p>
                <p class="pmeta">
                  <span>{metaLine(p) || '—'}</span>
                  <span class="sep">·</span>
                  <span class="cites"><Icon name="quote" size={11} /> {p.CitationCount}</span>
                  <span class="chip src">{p.Source}</span>
                </p>
                {#if blurb(p)}
                  <p class="pblurb" class:clamped={!expanded.has(p.ID)}>{blurb(p)}</p>
                  <button class="linkish" onclick={() => toggleExpanded(p.ID)}>
                    {expanded.has(p.ID) ? 'Show less' : 'Show more'}
                  </button>
                {/if}
                <div class="pactions">
                  <button class="btn sm" onclick={() => void add(p)} disabled={inLib || busyIDs.has(p.ID)}>
                    <Icon name={inLib ? 'check' : 'plus'} size={12} />
                    {inLib ? 'In library' : 'Add'}
                  </button>
                  <button class="btn sm" onclick={() => void download(p)} disabled={busyIDs.has(p.ID)}>
                    <Icon name="download" size={12} /> Download
                  </button>
                  <button class="btn sm" onclick={() => void openPaper(p)}>
                    <Icon name="external" size={12} /> Open
                  </button>
                </div>
              </article>
            {/each}
          </div>
        {/if}
      </section>
    {/if}

    <!-- ------------------------------------------------------ library -->
    <section class="block">
      <header class="block-head">
        <h2 class="section-title">Library</h2>
        <div class="tabs">
          {#each STATUS_TABS as t (t.v)}
            <button class="tab" class:on={statusTab === t.v} onclick={() => (statusTab = t.v)}>
              {t.label}<span class="tab-n">{counts[t.v] ?? 0}</span>
            </button>
          {/each}
        </div>
        <div class="grow"></div>
        <label class="sortwrap">
          <Icon name="sort" size={13} />
          <select class="select sortsel" bind:value={sort} aria-label="Sort library">
            {#each SORTS as s (s.v)}<option value={s.v}>{s.label}</option>{/each}
          </select>
        </label>
        <button class="btn sm" onclick={() => void exportBib()} disabled={bibBusy || library.length === 0}>
          {#if bibBusy}<span class="spinner"></span>{:else}<Icon name="copy" size={12} />{/if}
          Export BibTeX{selected.size ? ` (${selected.size})` : ''}
        </button>
      </header>

      {#if libLoading}
        <div class="card"><div class="empty"><span class="spinner"></span> Loading library…</div></div>
      {:else if visibleLibrary.length === 0}
        <div class="card">
          <div class="empty">
            {#if library.length === 0}
              Nothing saved yet — search above and hit <strong>Add</strong> to start the library.
            {:else}
              No papers marked “{statusLabel(statusTab)}”.
            {/if}
          </div>
        </div>
      {:else}
        <div class="lib">
          {#each visibleLibrary as p (p.ID)}
            {@const open = openCards.has(p.ID)}
            {@const sum = summaries[p.ID]}
            {@const running = summaryJob?.paperID === p.ID}
            <article class="card lcard">
              <header class="lhead">
                <label class="pick" title="Include in BibTeX export">
                  <input type="checkbox" checked={selected.has(p.ID)} onchange={() => toggleSelected(p.ID)} />
                  <span class="box"><Icon name="check" size={10} /></span>
                </label>
                <button class="ltitle-btn" onclick={() => toggleCard(p.ID)}>
                  <h3 class="ptitle">{p.Title}</h3>
                  <p class="pauthors truncate">{authorLine(p)} · {metaLine(p) || '—'}</p>
                </button>
                <span class="stars" aria-label="{p.Stars} of 5">
                  {#each [1, 2, 3, 4, 5] as n (n)}
                    <button
                      class="star"
                      class:on={p.Stars >= n}
                      onclick={() => patch(p, { Stars: p.Stars === n ? 0 : n }, true)}
                      aria-label="{n} star{n === 1 ? '' : 's'}"
                    ></button>
                  {/each}
                </span>
                <select
                  class="select statussel"
                  value={p.Status}
                  onchange={(e) => patch(p, { Status: e.currentTarget.value }, true)}
                  aria-label="Reading status"
                >
                  {#each STATUS_OPTIONS as o (o.v)}<option value={o.v}>{o.label}</option>{/each}
                </select>
                <button class="icon-btn" onclick={() => toggleCard(p.ID)} aria-label={open ? 'Collapse' : 'Expand'}>
                  <Icon name={open ? 'chevronDown' : 'chevronRight'} size={13} />
                </button>
              </header>

              <div class="lprogress">
                <div class="bar"><span class="fill" style="width:{p.Pages ? Math.min(100, (p.Page / p.Pages) * 100) : 0}%"></span></div>
                <span class="pnum">
                  <input
                    class="pageinput"
                    type="number"
                    min="0"
                    max={p.Pages || undefined}
                    value={p.Page}
                    onchange={(e) => clampPage(p, e.currentTarget.valueAsNumber)}
                    aria-label="Current page"
                  />
                  <span class="faint">/</span>
                  <input
                    class="pageinput"
                    type="number"
                    min="0"
                    value={p.Pages}
                    onchange={(e) => patch(p, { Pages: Math.max(0, Math.round(e.currentTarget.valueAsNumber || 0)) })}
                    aria-label="Total pages"
                  />
                  <span class="faint">p</span>
                </span>
                <span class="chip cites"><Icon name="quote" size={10} /> {p.CitationCount}</span>
                {#if p.LocalPath}<span class="chip green">PDF</span>{/if}
                <span class="faint added">added {relTime(p.AddedAt)}</span>
              </div>

              {#if open}
                <div class="lbody">
                  <div class="field">
                    <span class="lbl">Key idea</span>
                    <input
                      class="input"
                      type="text"
                      placeholder="One line you want to remember"
                      value={p.KeyIdea}
                      oninput={(e) => patch(p, { KeyIdea: e.currentTarget.value })}
                    />
                  </div>

                  <div class="field">
                    <span class="lbl">Tags</span>
                    <div class="chips">
                      {#each p.Tags ?? [] as t (t)}
                        <span class="chip accent removable">
                          {t}
                          <button class="chip-x" onclick={() => removeTag(p, t)} aria-label="Remove {t}">
                            <Icon name="x" size={10} />
                          </button>
                        </span>
                      {/each}
                      <input
                        class="chip-input"
                        type="text"
                        placeholder="add tag"
                        value={tagDrafts[p.ID] ?? ''}
                        oninput={(e) => (tagDrafts = { ...tagDrafts, [p.ID]: e.currentTarget.value })}
                        onkeydown={(e) => {
                          if (e.key === 'Enter' || e.key === ',') {
                            e.preventDefault();
                            addTag(p);
                          }
                        }}
                        onblur={() => addTag(p)}
                        spellcheck="false"
                      />
                    </div>
                  </div>

                  <div class="field">
                    <span class="lbl">Notes <span class="faint">— saved as you type</span></span>
                    <textarea
                      class="input notes"
                      rows="4"
                      placeholder="What matters in this paper, and why you saved it."
                      value={p.Notes}
                      oninput={(e) => patch(p, { Notes: e.currentTarget.value })}
                    ></textarea>
                  </div>

                  <div class="lactions">
                    <button class="btn sm primary" onclick={() => void summarise(p)} disabled={!!summaryJob}>
                      <Icon name="sparkles" size={12} />
                      {sum ? 'Re-summarise' : 'Summarise'}
                    </button>
                    <button class="btn sm" onclick={() => study(p)}><Icon name="layers" size={12} /> Study</button>
                    <button class="btn sm" onclick={() => chatAbout(p)}><Icon name="chat" size={12} /> Chat</button>
                    <button class="btn sm" class:on={drawer?.paperID === p.ID && drawer?.kind === 'citations'} onclick={() => void toggleDrawer(p, 'citations')}>
                      Citations
                    </button>
                    <button class="btn sm" class:on={drawer?.paperID === p.ID && drawer?.kind === 'references'} onclick={() => void toggleDrawer(p, 'references')}>
                      References
                    </button>
                    <button class="btn sm" onclick={() => void download(p)} disabled={busyIDs.has(p.ID)}>
                      <Icon name="download" size={12} /> {p.LocalPath ? 'Re-download' : 'Download'}
                    </button>
                    <button class="btn sm" onclick={() => void openPaper(p)}><Icon name="external" size={12} /> Open</button>
                    <div class="grow"></div>
                    <button class="btn sm danger" onclick={() => void remove(p)}><Icon name="trash" size={12} /> Remove</button>
                  </div>

                  {#if running}
                    <div class="jobbar">
                      <span class="spinner"></span>
                      <span class="job-line truncate">{summaryJob?.line}</span>
                      <button class="btn sm" onclick={() => void cancelSummary()}>Cancel</button>
                    </div>
                  {/if}

                  {#if sum?.Markdown}
                    <div class="summary">
                      <div class="summary-head">
                        <Icon name="sparkles" size={12} />
                        <span class="section-title">Key points</span>
                        <span class="faint">{relTime(sum.CreatedAt)} · {sum.Model}</span>
                      </div>
                      <div class="md">{@html markdownToHTML(sum.Markdown)}</div>
                    </div>
                  {/if}

                  {#if drawer && drawer.paperID === p.ID}
                    <div class="drawer">
                      <div class="drawer-head">
                        <span class="section-title">{drawer.kind === 'citations' ? 'Cited by' : 'References'}</span>
                        <span class="faint">{drawerLoading ? 'loading…' : `${drawerLinks.length}`}</span>
                        <div class="grow"></div>
                        <button class="icon-btn" onclick={() => (drawer = null)} aria-label="Close"><Icon name="x" size={12} /></button>
                      </div>
                      {#if drawerLoading}
                        <div class="empty"><span class="spinner"></span> Loading…</div>
                      {:else if drawerLinks.length === 0}
                        <div class="empty">Nothing on record for this paper.</div>
                      {:else}
                        {#each drawerLinks as l (l.ID)}
                          <div class="link-row">
                            <div class="lr-main">
                              <span class="lr-title truncate">{l.Title}</span>
                              <span class="lr-meta faint truncate">{authorLine(l)} · {metaLine(l)}</span>
                            </div>
                            <span class="chip cites"><Icon name="quote" size={10} /> {l.CitationCount}</span>
                            {#if l.InLibrary}
                              <span class="chip green">in library{l.Status ? ` · ${statusLabel(l.Status)}` : ''}</span>
                            {:else}
                              <button class="btn sm" onclick={() => void add(l)} disabled={busyIDs.has(l.ID)}>
                                <Icon name="plus" size={11} /> Add
                              </button>
                            {/if}
                          </div>
                        {/each}
                      {/if}
                    </div>
                  {/if}
                </div>
              {/if}
            </article>
          {/each}
        </div>
      {/if}
    </section>
  </div>

  <!-- ------------------------------------------------------- right rail -->
  <aside class="rail">
    <section class="card rail-card">
      <header class="rail-head">
        <span class="section-title">Recommended</span>
        <button class="icon-btn" onclick={() => void loadRail()} aria-label="Refresh"><Icon name="sync" size={12} /></button>
      </header>
      {#if recs.length === 0}
        <div class="empty">Add a few papers and recommendations show up here.</div>
      {:else}
        {#each recs as p (p.ID)}
          <div class="rail-row">
            <button class="rr-title" onclick={() => void openPaper(p)} title={p.Title}>{p.Title}</button>
            <div class="rr-meta faint truncate">{metaLine(p)} · {p.CitationCount} cites</div>
            <button class="btn sm" onclick={() => void add(p)} disabled={libIDs.has(p.ID) || busyIDs.has(p.ID)}>
              <Icon name={libIDs.has(p.ID) ? 'check' : 'plus'} size={11} />
              {libIDs.has(p.ID) ? 'Saved' : 'Add'}
            </button>
          </div>
        {/each}
      {/if}
    </section>

    <section class="card rail-card">
      <header class="rail-head">
        <span class="section-title">Today's digest</span>
        <span class="faint">{digest?.Date ?? ''}</span>
      </header>
      {#if !digest || (digest.Papers ?? []).length === 0}
        <div class="empty">No digest for today yet.</div>
      {:else}
        {#each digest.Papers ?? [] as p, i (p.ID)}
          <div class="rail-row">
            <button class="rr-title" onclick={() => void openPaper(p)} title={p.Title}>{p.Title}</button>
            <div class="rr-meta faint truncate">{(digest?.Reason ?? [])[i] ?? metaLine(p)}</div>
            <button class="btn sm" onclick={() => void add(p)} disabled={libIDs.has(p.ID) || busyIDs.has(p.ID)}>
              <Icon name={libIDs.has(p.ID) ? 'check' : 'plus'} size={11} />
              {libIDs.has(p.ID) ? 'Saved' : 'Add'}
            </button>
          </div>
        {/each}
        <button class="btn send" onclick={() => void sendDigest()} disabled={sendingDigest}>
          {#if sendingDigest}<span class="spinner"></span>{:else}<Icon name="telegram" size={13} />{/if}
          Send to Telegram
        </button>
      {/if}
    </section>
  </aside>
</div>

{#if bibOpen}
  <div
    class="scrim"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget) bibOpen = false;
    }}
  >
    <div class="modal card" role="dialog" aria-modal="true" aria-label="BibTeX export">
      <header class="modal-head">
        <h2>BibTeX</h2>
        <span class="faint">{selected.size ? `${selected.size} selected` : `${library.length} papers`}</span>
        <div class="grow"></div>
        <button class="btn sm" onclick={() => void copyBib()}><Icon name="copy" size={12} /> Copy</button>
        <button class="icon-btn" onclick={() => (bibOpen = false)} aria-label="Close"><Icon name="x" size={13} /></button>
      </header>
      <pre class="bib">{bib}</pre>
    </div>
  </div>
{/if}

<style>
  .papers {
    display: flex;
    gap: 18px;
    height: 100%;
    overflow-y: auto;
    padding: 22px 26px 44px;
  }

  .col {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 20px;
  }

  .grow {
    flex: 1;
  }

  /* ------------------------------------------------------------ search */

  .searchbar {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .search {
    flex: 1;
    min-width: 240px;
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
    min-width: 0;
    border: none;
    outline: none;
    background: none;
    font-size: 13px;
    color: var(--text);
  }

  .q::placeholder {
    color: var(--text-faint);
  }

  .clear {
    color: var(--text-faint);
    display: flex;
  }

  .clear:hover {
    color: var(--text);
  }

  .seg {
    display: flex;
    padding: 2px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
  }

  .seg-btn {
    height: 26px;
    padding: 0 11px;
    border-radius: 4px;
    color: var(--text-muted);
    font-size: 12.5px;
    font-weight: 500;
    transition: background var(--t), color var(--t);
  }

  .seg-btn:hover {
    color: var(--text);
  }

  .seg-btn.on {
    background: var(--bg-elevated);
    color: var(--text);
    box-shadow: 0 1px 2px rgba(15, 15, 25, 0.08);
  }

  /* ------------------------------------------------------------ blocks */

  .block {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .block-head {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
  }

  .count {
    font-size: 11.5px;
  }

  .tabs {
    display: flex;
    gap: 2px;
    padding: 2px;
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
  }

  .tab {
    display: flex;
    align-items: center;
    gap: 6px;
    height: 24px;
    padding: 0 9px;
    border-radius: 4px;
    color: var(--text-muted);
    font-size: 12.5px;
    font-weight: 500;
    transition: background var(--t), color var(--t);
  }

  .tab:hover {
    color: var(--text);
  }

  .tab.on {
    background: var(--bg-elevated);
    color: var(--text);
  }

  .tab-n {
    font-size: 10.5px;
    font-variant-numeric: tabular-nums;
    color: var(--text-faint);
  }

  .sortwrap {
    display: flex;
    align-items: center;
    gap: 5px;
    color: var(--text-faint);
  }

  .sortsel {
    width: auto;
    height: 26px;
    font-size: 12.5px;
    padding: 0 26px 0 8px;
    background-position: calc(100% - 13px) 11px, calc(100% - 8px) 11px;
  }

  /* ------------------------------------------------------ result cards */

  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
    gap: 10px;
  }

  .rcard {
    padding: 12px 14px 11px;
    display: flex;
    flex-direction: column;
    gap: 5px;
  }

  .ptitle {
    font-size: 13.5px;
    font-weight: 600;
    line-height: 1.35;
    letter-spacing: -0.01em;
  }

  .pauthors {
    font-size: 12px;
    color: var(--text-muted);
  }

  .pmeta {
    display: flex;
    align-items: center;
    gap: 7px;
    font-size: 11.5px;
    color: var(--text-faint);
  }

  .cites {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    font-variant-numeric: tabular-nums;
  }

  .chip.src {
    text-transform: uppercase;
    letter-spacing: 0.04em;
    font-size: 10px;
    height: 18px;
  }

  .pblurb {
    font-size: 12.5px;
    color: var(--text-muted);
    line-height: 1.55;
  }

  .pblurb.clamped {
    display: -webkit-box;
    -webkit-line-clamp: 3;
    line-clamp: 3;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .linkish {
    align-self: flex-start;
    font-size: 11.5px;
    font-weight: 550;
    color: var(--accent-text);
  }

  .linkish:hover {
    text-decoration: underline;
  }

  .pactions {
    display: flex;
    gap: 6px;
    margin-top: 4px;
  }

  /* ----------------------------------------------------- library cards */

  .lib {
    display: flex;
    flex-direction: column;
    gap: 9px;
  }

  .lcard {
    padding: 11px 13px;
  }

  .lhead {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .pick {
    display: flex;
    cursor: pointer;
  }

  .pick input {
    position: absolute;
    opacity: 0;
    width: 0;
    height: 0;
  }

  .pick .box {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 15px;
    height: 15px;
    border: 1px solid var(--border-strong);
    border-radius: 4px;
    color: transparent;
    transition: background var(--t), border-color var(--t), color var(--t);
  }

  .pick input:checked + .box {
    background: var(--accent);
    border-color: var(--accent);
    color: #fff;
  }

  .ltitle-btn {
    flex: 1;
    min-width: 0;
    text-align: left;
  }

  .stars {
    display: flex;
    gap: 3px;
    flex: none;
  }

  .star {
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: var(--bg-subtle);
    box-shadow: inset 0 0 0 1px var(--border-strong);
    transition: background var(--t);
  }

  .star.on {
    background: var(--amber);
    box-shadow: none;
  }

  .statussel {
    width: auto;
    height: 26px;
    font-size: 12px;
    padding: 0 26px 0 8px;
    background-position: calc(100% - 13px) 11px, calc(100% - 8px) 11px;
    flex: none;
  }

  .icon-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    border-radius: var(--radius-sm);
    color: var(--text-faint);
    flex: none;
    transition: background var(--t), color var(--t);
  }

  .icon-btn:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .lprogress {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-top: 9px;
  }

  .bar {
    flex: 1;
    min-width: 60px;
    height: 3px;
    border-radius: 999px;
    background: var(--bg-subtle);
    overflow: hidden;
  }

  .fill {
    display: block;
    height: 100%;
    background: var(--accent);
    transition: width var(--t);
  }

  .pnum {
    display: flex;
    align-items: center;
    gap: 3px;
    font-size: 11.5px;
    color: var(--text-muted);
  }

  .pageinput {
    width: 42px;
    height: 22px;
    padding: 0 4px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg-input);
    font-size: 11.5px;
    text-align: right;
    font-variant-numeric: tabular-nums;
  }

  .pageinput:focus {
    outline: none;
    border-color: var(--accent);
  }

  .added {
    font-size: 11px;
  }

  .lbody {
    display: flex;
    flex-direction: column;
    gap: 12px;
    margin-top: 12px;
    padding-top: 12px;
    border-top: 1px solid var(--border);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 5px;
  }

  .lbl {
    font-size: 11.5px;
    font-weight: 550;
    color: var(--text-muted);
  }

  .notes {
    height: auto;
    padding: 8px 10px;
    line-height: 1.55;
    resize: vertical;
  }

  .chips {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 5px;
    min-height: 32px;
    padding: 4px 6px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-input);
  }

  .chips:focus-within {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-soft);
  }

  .chip.removable {
    padding-right: 3px;
  }

  .chip-x {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 14px;
    height: 14px;
    border-radius: 3px;
    color: inherit;
    opacity: 0.6;
  }

  .chip-x:hover {
    opacity: 1;
    background: var(--bg-hover);
  }

  .chip-input {
    flex: 1;
    min-width: 80px;
    height: 21px;
    border: none;
    outline: none;
    background: none;
    font-size: 12px;
  }

  .lactions {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    align-items: center;
  }

  .btn.on {
    border-color: var(--accent);
    color: var(--accent-text);
  }

  .btn.danger:hover:not(:disabled) {
    border-color: var(--red);
    color: var(--red);
  }

  .jobbar {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 8px 10px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
    font-size: 12.5px;
  }

  .job-line {
    flex: 1;
    min-width: 0;
    color: var(--text-muted);
  }

  .summary {
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
  }

  .summary-head {
    display: flex;
    align-items: center;
    gap: 7px;
    padding: 8px 12px;
    border-bottom: 1px solid var(--border);
    color: var(--text-faint);
    font-size: 11px;
  }

  /* ------------------------------------------------------------ drawer */

  .drawer {
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }

  .drawer-head {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 7px 10px;
    background: var(--bg-subtle);
    border-bottom: 1px solid var(--border);
    font-size: 11px;
  }

  .link-row {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 8px 10px;
    border-bottom: 1px solid var(--border);
  }

  .link-row:last-child {
    border-bottom: none;
  }

  .lr-main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .lr-title {
    font-size: 12.5px;
    font-weight: 500;
  }

  .lr-meta {
    font-size: 11px;
  }

  /* -------------------------------------------------------------- rail */

  .rail {
    width: 268px;
    flex: none;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .rail-card {
    padding: 10px 12px 12px;
  }

  .rail-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    margin-bottom: 6px;
    font-size: 11px;
  }

  .rail-row {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 3px;
    padding: 8px 0;
    border-top: 1px solid var(--border);
  }

  .rr-title {
    text-align: left;
    font-size: 12.5px;
    font-weight: 500;
    line-height: 1.35;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .rr-title:hover {
    color: var(--accent-text);
  }

  .rr-meta {
    font-size: 11px;
    max-width: 100%;
  }

  .send {
    width: 100%;
    margin-top: 10px;
    justify-content: center;
  }

  /* ------------------------------------------------------------- modal */

  .scrim {
    position: fixed;
    inset: 0;
    z-index: 60;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 40px;
    background: var(--bg-overlay);
  }

  .modal {
    width: min(760px, 100%);
    max-height: 100%;
    display: flex;
    flex-direction: column;
    box-shadow: var(--shadow-pop);
    overflow: hidden;
  }

  .modal-head {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 11px 12px;
    border-bottom: 1px solid var(--border);
  }

  .modal-head h2 {
    font-size: 13.5px;
    font-weight: 620;
  }

  .bib {
    margin: 0;
    padding: 14px;
    overflow: auto;
    font-family: var(--mono);
    font-size: 12px;
    line-height: 1.6;
    white-space: pre;
  }

  /* ---------------------------------------------------------- markdown */

  .md {
    padding: 12px 14px;
    font-size: 12.5px;
    line-height: 1.62;
  }

  .md :global(h2) {
    font-size: 12.5px;
    font-weight: 620;
    margin: 12px 0 5px;
  }

  .md :global(h2:first-child) {
    margin-top: 0;
  }

  .md :global(h3) {
    font-size: 12px;
    font-weight: 600;
    color: var(--text-muted);
    margin: 10px 0 4px;
  }

  .md :global(p) {
    margin: 0 0 8px;
  }

  .md :global(ul),
  .md :global(ol) {
    margin: 0 0 9px;
    padding-left: 18px;
    list-style: revert;
  }

  .md :global(li) {
    margin-bottom: 2px;
  }

  .md :global(code) {
    font-family: var(--mono);
    font-size: 11.5px;
    background: var(--bg-elevated);
    border-radius: 4px;
    padding: 1px 4px;
  }

  .md :global(pre) {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 9px 11px;
    overflow-x: auto;
    margin: 0 0 9px;
  }

  .md :global(blockquote) {
    margin: 0 0 9px;
    padding-left: 10px;
    border-left: 2px solid var(--border-strong);
    color: var(--text-muted);
  }

  @media (max-width: 1080px) {
    .papers {
      flex-direction: column;
    }
    .rail {
      width: auto;
    }
  }
</style>
