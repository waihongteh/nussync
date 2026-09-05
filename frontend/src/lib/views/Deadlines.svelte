<script lang="ts">
  import { api, errMsg } from '../api';
  import Icon from '../components/Icon.svelte';
  import { courseByID, deadlines, openExternal, toast } from '../stores';
  import {
    DEADLINE_GROUP_ORDER,
    countdown,
    deadlineGroup,
    fmtDateTime,
    fmtBytes,
    fmtScore,
    sanitizeHTML,
    urgency,
  } from '../util';
  import type { DeadlineGroup } from '../util';
  import type { Deadline, FileNode } from '../types';
  import { lsGet, lsSet } from '../util';

  let hideSubmitted = $state(lsGet('nussync.deadlines.hideSubmitted', false));
  let tick = $state(Date.now());

  $effect(() => {
    const h = setInterval(() => (tick = Date.now()), 30_000);
    return () => clearInterval(h);
  });

  $effect(() => {
    lsSet('nussync.deadlines.hideSubmitted', hideSubmitted);
  });

  const TYPE_LABEL: Record<string, string> = {
    assignment: 'Assignment',
    quiz: 'Quiz',
    discussion: 'Discussion',
  };

  const URGENCY_CLASS: Record<string, string> = {
    done: 'green',
    overdue: 'red',
    today: 'red',
    soon: 'amber',
    later: '',
  };

  const grouped = $derived.by(() => {
    const list = hideSubmitted ? $deadlines.filter((d) => !d.Submitted) : $deadlines;
    const map = new Map<DeadlineGroup, Deadline[]>();
    for (const d of [...list].sort((a, b) => Date.parse(a.DueAt) - Date.parse(b.DueAt))) {
      const g = deadlineGroup(d.DueAt, tick);
      const arr = map.get(g) ?? [];
      arr.push(d);
      map.set(g, arr);
    }
    return DEADLINE_GROUP_ORDER.filter((g) => (map.get(g) ?? []).length > 0).map((g) => ({
      group: g,
      items: map.get(g)!,
    }));
  });

  const submittedCount = $derived($deadlines.filter((d) => d.Submitted).length);

  // ------------------------------------------------------------- detail

  const SUBMISSION_LABEL: Record<string, string> = {
    online_upload: 'File upload',
    online_text_entry: 'Text entry',
    online_url: 'Website URL',
    online_quiz: 'Online quiz',
    discussion_topic: 'Discussion',
    media_recording: 'Media recording',
    on_paper: 'On paper',
    none: 'No submission',
  };

  let openID = $state<number | null>(null);
  let detail = $state<Deadline | null>(null);
  let detailLoading = $state(false);
  let detailError = $state('');

  /** Detail is cached for the session — the backend caches for 6 hours anyway. */
  const detailCache = new Map<number, Deadline>();

  async function select(d: Deadline) {
    if (openID === d.ID) {
      closePanel();
      return;
    }
    openID = d.ID;
    detailError = '';
    const cached = detailCache.get(d.ID);
    if (cached) {
      detail = cached;
      detailLoading = false;
      return;
    }
    // Show the row we already have while the detail call is in flight.
    detail = d;
    detailLoading = true;
    try {
      const full = await api.getDeadlineDetail(d.ID);
      if (openID !== d.ID) return;
      detailCache.set(d.ID, full);
      detail = full;
    } catch (err) {
      if (openID === d.ID) detailError = errMsg(err);
    } finally {
      if (openID === d.ID) detailLoading = false;
    }
  }

  function closePanel() {
    openID = null;
    detail = null;
    detailError = '';
  }

  const descHTML = $derived(detail?.Description ? sanitizeHTML(detail.Description) : '');

  /** Position of a score inside [min, max] as a percentage, clamped. */
  function markPct(value: number, min: number, max: number): number {
    if (!isFinite(value) || max <= min) return 50;
    return Math.max(0, Math.min(100, ((value - min) / (max - min)) * 100));
  }

  async function openAttachment(f: FileNode) {
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

  function onPanelKey(e: KeyboardEvent) {
    if (e.key === 'Escape' && openID !== null) closePanel();
  }
</script>

<svelte:window onkeydown={onPanelKey} />

<div class="view">
  <div class="view-narrow">
    <header class="head">
      <div>
        <h1 class="page-title">Deadlines</h1>
        <p class="muted sub">{$deadlines.length} tracked · {submittedCount} submitted</p>
      </div>
      <label class="toggle">
        <input type="checkbox" bind:checked={hideSubmitted} />
        <span class="track"><span class="knob"></span></span>
        <span>Hide submitted</span>
      </label>
    </header>

    {#each grouped as g (g.group)}
      <section class="group">
        <h2 class="section-title" class:overdue={g.group === 'Overdue'}>{g.group}</h2>
        <div class="card">
          {#each g.items as d (d.ID)}
            {@const u = urgency(d.DueAt, d.Submitted, tick)}
            <button class="row" class:open={openID === d.ID} onclick={() => select(d)} title="Show detail">
              <span class="dot" style="background:{$courseByID.get(d.CourseID)?.Color ?? 'var(--text-faint)'}"></span>
              <span class="code">{d.CourseCode}</span>
              <span class="main">
                <span class="title truncate">{d.Title}</span>
                <span class="when">{fmtDateTime(d.DueAt)}{d.PointsPossible ? ` · ${d.PointsPossible} pts` : ''}</span>
              </span>
              <span class="type chip">{TYPE_LABEL[d.Type] ?? d.Type}</span>
              {#if d.Submitted}
                <span class="chip green state"><Icon name="check" size={11} /> Submitted</span>
              {:else}
                <span class="chip {URGENCY_CLASS[u]} state">{countdown(d.DueAt, tick)}</span>
              {/if}
              <span class="go"><Icon name={openID === d.ID ? 'chevronLeft' : 'chevronRight'} size={13} /></span>
            </button>
          {/each}
        </div>
      </section>
    {:else}
      <div class="card"><div class="empty">No deadlines to show.</div></div>
    {/each}
  </div>
</div>

{#if detail}
  <div class="scrim" onclick={closePanel} role="presentation"></div>
  <aside class="panel" aria-label="Deadline detail">
    <header class="p-head">
      <div class="p-title-wrap">
        <span class="p-course">
          <span class="dot" style="background:{$courseByID.get(detail.CourseID)?.Color ?? 'var(--text-faint)'}"></span>
          {detail.CourseCode}
        </span>
        <h2 class="p-title">{detail.Title}</h2>
      </div>
      <button class="icon-btn" onclick={closePanel} aria-label="Close detail"><Icon name="x" size={14} /></button>
    </header>

    <div class="p-body">
      <div class="p-meta">
        <span class="chip">{TYPE_LABEL[detail.Type] ?? detail.Type}</span>
        {#if detail.Submitted}
          <span class="chip green"><Icon name="check" size={11} /> Submitted</span>
        {:else}
          <span class="chip {URGENCY_CLASS[urgency(detail.DueAt, detail.Submitted, tick)]}">
            {countdown(detail.DueAt, tick)}
          </span>
        {/if}
        {#if detail.PointsPossible}<span class="chip">{fmtScore(detail.PointsPossible)} pts</span>{/if}
      </div>

      <p class="p-due muted">Due {fmtDateTime(detail.DueAt)}</p>

      {#if detailLoading}
        <div class="p-loading"><span class="spinner"></span> Loading detail…</div>
      {:else if detailError}
        <div class="p-error"><Icon name="alert" size={13} /> {detailError}</div>
      {/if}

      {#if (detail.SubmissionTypes ?? []).length > 0}
        <section class="p-block">
          <h3 class="section-title">Submission</h3>
          <div class="p-chips">
            {#each detail.SubmissionTypes ?? [] as t (t)}
              <span class="chip accent">{SUBMISSION_LABEL[t] ?? t.replace(/_/g, ' ')}</span>
            {/each}
          </div>
        </section>
      {/if}

      {#if detail.Graded}
        {@const possible = detail.PointsPossible || detail.Stats?.Max || 0}
        <section class="p-block">
          <h3 class="section-title">Your score</h3>
          <div class="score-line">
            <span class="score-big">{fmtScore(detail.Score)}</span>
            {#if possible}<span class="score-of muted">/ {fmtScore(possible)}</span>{/if}
            {#if detail.Stats && detail.Stats.Mean}
              {@const delta = detail.Score - detail.Stats.Mean}
              <span class="chip {delta >= 0 ? 'green' : 'amber'} delta">
                {delta >= 0 ? '+' : ''}{fmtScore(delta)} vs class
              </span>
            {/if}
          </div>

          {#if detail.Stats}
            {@const st = detail.Stats}
            <div class="range">
              <div class="range-bar">
                <span class="range-fill"></span>
                <span class="range-tick mean" style="left:{markPct(st.Mean, st.Min, st.Max)}%" title="Mean {fmtScore(st.Mean)}"></span>
                <span class="range-tick median" style="left:{markPct(st.Median, st.Min, st.Max)}%" title="Median {fmtScore(st.Median)}"></span>
                <span class="range-you" style="left:{markPct(detail.Score, st.Min, st.Max)}%" title="You {fmtScore(detail.Score)}"></span>
              </div>
              <div class="range-labels faint">
                <span>min {fmtScore(st.Min)}</span>
                <span class="range-mid">mean {fmtScore(st.Mean)} · median {fmtScore(st.Median)}</span>
                <span>max {fmtScore(st.Max)}</span>
              </div>
              <div class="range-legend faint">
                <span class="lg"><i class="sw you"></i> you</span>
                <span class="lg"><i class="sw mean"></i> mean</span>
                <span class="lg"><i class="sw median"></i> median</span>
                {#if st.Count > 0}<span class="lg">{st.Count} submissions</span>{/if}
              </div>
            </div>
          {/if}
        </section>
      {/if}

      {#if descHTML}
        <section class="p-block">
          <h3 class="section-title">Description</h3>
          <div class="p-desc">{@html descHTML}</div>
        </section>
      {:else if !detailLoading}
        <section class="p-block">
          <h3 class="section-title">Description</h3>
          <p class="faint">Canvas gave no description for this one.</p>
        </section>
      {/if}

      {#if (detail.Attachments ?? []).length > 0}
        <section class="p-block">
          <h3 class="section-title">Attachments</h3>
          <div class="p-files">
            {#each detail.Attachments ?? [] as f (f.ID + f.RelPath)}
              <button class="p-file" onclick={() => openAttachment(f)} class:unsynced={!f.Synced}>
                <Icon name="fileText" size={14} />
                <span class="truncate">{f.Name}</span>
                <span class="faint fsize">{fmtBytes(f.Size)}</span>
              </button>
            {/each}
          </div>
        </section>
      {/if}
    </div>

    <footer class="p-foot">
      <button class="btn primary" onclick={() => openExternal(detail?.URL ?? '')}>
        <Icon name="external" size={13} /> Open in Canvas
      </button>
    </footer>
  </aside>
{/if}

<style>
  .head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    margin-bottom: 22px;
  }

  .sub {
    margin-top: 3px;
    font-size: 13px;
  }

  .toggle {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    font-size: 12.5px;
    color: var(--text-muted);
    cursor: pointer;
    user-select: none;
  }

  .toggle input {
    position: absolute;
    opacity: 0;
    width: 0;
    height: 0;
  }

  .track {
    width: 30px;
    height: 17px;
    border-radius: 999px;
    background: var(--border-strong);
    padding: 2px;
    transition: background var(--t);
  }

  .knob {
    display: block;
    width: 13px;
    height: 13px;
    border-radius: 50%;
    background: #fff;
    transition: transform var(--t);
  }

  .toggle input:checked + .track {
    background: var(--accent);
  }

  .toggle input:checked + .track .knob {
    transform: translateX(13px);
  }

  .toggle input:focus-visible + .track {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }

  .group {
    margin-bottom: 20px;
  }

  .section-title {
    display: block;
    margin-bottom: 8px;
    padding: 0 2px;
  }

  .section-title.overdue {
    color: var(--red);
  }

  .row {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 10px 13px;
    text-align: left;
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
    min-width: 64px;
    font-size: 11.5px;
    font-weight: 600;
    color: var(--text-muted);
  }

  .main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .title {
    font-size: 13px;
  }

  .when {
    font-size: 11.5px;
    color: var(--text-faint);
  }

  .type {
    flex: none;
  }

  .state {
    flex: none;
    min-width: 76px;
    justify-content: center;
    font-variant-numeric: tabular-nums;
  }

  .go {
    color: var(--text-faint);
    opacity: 0;
    transition: opacity var(--t);
    flex: none;
  }

  .row:hover .go {
    opacity: 1;
  }

  .scrim {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    z-index: 40;
  }

  .panel {
    position: fixed;
    top: 0;
    right: 0;
    bottom: 0;
    width: min(440px, 92vw);
    display: flex;
    flex-direction: column;
    background: var(--bg-elevated);
    border-left: 1px solid var(--border);
    box-shadow: var(--shadow-pop);
    z-index: 41;
  }

  .p-head {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 14px 14px 12px 16px;
    border-bottom: 1px solid var(--border);
    flex: none;
  }

  .p-title-wrap {
    flex: 1;
    min-width: 0;
  }

  .p-course {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 11.5px;
    font-weight: 600;
    color: var(--text-muted);
  }

  .p-title {
    font-size: 15px;
    font-weight: 620;
    letter-spacing: -0.01em;
    margin-top: 3px;
    line-height: 1.35;
  }

  .icon-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 26px;
    flex: none;
    border-radius: var(--radius-sm);
    color: var(--text-faint);
    transition: background var(--t), color var(--t);
  }

  .icon-btn:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .p-body {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 14px 16px 20px;
  }

  .p-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }

  .p-due {
    margin-top: 7px;
    font-size: 12.5px;
  }

  .p-loading,
  .p-error {
    display: flex;
    align-items: center;
    gap: 7px;
    margin-top: 12px;
    font-size: 12.5px;
    color: var(--text-muted);
  }

  .p-error {
    color: var(--red);
  }

  .p-block {
    margin-top: 18px;
  }

  .p-block .section-title {
    display: block;
    margin-bottom: 7px;
  }

  .p-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }

  .score-line {
    display: flex;
    align-items: baseline;
    gap: 7px;
  }

  .score-big {
    font-size: 22px;
    font-weight: 640;
    letter-spacing: -0.02em;
    font-variant-numeric: tabular-nums;
  }

  .score-of {
    font-size: 13px;
  }

  .delta {
    align-self: center;
    margin-left: 2px;
  }

  .range {
    margin-top: 12px;
  }

  .range-bar {
    position: relative;
    height: 8px;
    border-radius: 999px;
    background: var(--bg-subtle);
    border: 1px solid var(--border);
  }

  .range-fill {
    position: absolute;
    inset: 0;
    border-radius: 999px;
    background: linear-gradient(90deg, var(--red-soft), var(--amber-soft), var(--green-soft));
  }

  .range-tick {
    position: absolute;
    top: -2px;
    width: 2px;
    height: 12px;
    margin-left: -1px;
    border-radius: 1px;
  }

  .range-tick.mean {
    background: var(--text-muted);
  }

  .range-tick.median {
    background: var(--text-faint);
  }

  .range-you {
    position: absolute;
    top: -4px;
    width: 10px;
    height: 16px;
    margin-left: -5px;
    border-radius: 3px;
    background: var(--accent);
    border: 2px solid var(--bg-elevated);
  }

  .range-labels {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    margin-top: 7px;
    font-size: 11px;
    font-variant-numeric: tabular-nums;
  }

  .range-mid {
    text-align: center;
  }

  .range-legend {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    margin-top: 6px;
    font-size: 11px;
  }

  .lg {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }

  .sw {
    width: 8px;
    height: 8px;
    border-radius: 2px;
    display: inline-block;
  }

  .sw.you {
    background: var(--accent);
  }

  .sw.mean {
    background: var(--text-muted);
  }

  .sw.median {
    background: var(--text-faint);
  }

  .p-desc {
    font-size: 13px;
    line-height: 1.6;
    color: var(--text);
    word-break: break-word;
  }

  .p-desc :global(p) {
    margin: 0 0 9px;
  }

  .p-desc :global(ul),
  .p-desc :global(ol) {
    margin: 0 0 9px;
    padding-left: 20px;
    list-style: revert;
  }

  .p-desc :global(li) {
    margin-bottom: 3px;
  }

  .p-desc :global(code) {
    font-family: var(--mono);
    font-size: 12px;
    background: var(--bg-subtle);
    border-radius: 4px;
    padding: 1px 4px;
  }

  .p-desc :global(a) {
    color: var(--accent-text);
  }

  .p-files {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .p-file {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 7px 9px;
    text-align: left;
    font-size: 12.5px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    color: var(--text);
    transition: background var(--t), border-color var(--t);
  }

  .p-file:hover {
    background: var(--bg-hover);
    border-color: var(--border-strong);
  }

  .p-file.unsynced {
    color: var(--text-faint);
  }

  .fsize {
    margin-left: auto;
    font-size: 11px;
    font-variant-numeric: tabular-nums;
  }

  .p-foot {
    flex: none;
    padding: 11px 16px;
    border-top: 1px solid var(--border);
  }

  .row.open {
    background: var(--bg-active);
  }
</style>
