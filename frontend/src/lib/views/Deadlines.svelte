<script lang="ts">
  import Icon from '../components/Icon.svelte';
  import { courseByID, deadlines, openExternal } from '../stores';
  import { DEADLINE_GROUP_ORDER, countdown, deadlineGroup, fmtDateTime, urgency } from '../util';
  import type { DeadlineGroup } from '../util';
  import type { Deadline } from '../types';
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
</script>

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
            <button class="row" onclick={() => openExternal(d.URL)} title="Open in Canvas">
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
              <span class="go"><Icon name="external" size={13} /></span>
            </button>
          {/each}
        </div>
      </section>
    {:else}
      <div class="card"><div class="empty">No deadlines to show.</div></div>
    {/each}
  </div>
</div>

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
</style>
