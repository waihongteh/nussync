<script lang="ts">
  import Icon from '../components/Icon.svelte';
  import { courses, grades, openExternal } from '../stores';
  import { fmtDate, fmtScore } from '../util';
  import type { Grade } from '../types';

  interface Group {
    code: string;
    color: string;
    name: string;
    items: Grade[];
    score: number;
    possible: number;
  }

  const grouped = $derived.by<Group[]>(() => {
    const byCode = new Map<string, Grade[]>();
    for (const g of $grades) {
      const arr = byCode.get(g.CourseCode) ?? [];
      arr.push(g);
      byCode.set(g.CourseCode, arr);
    }
    return [...byCode.entries()].map(([code, items]) => {
      const course = $courses.find((c) => c.Code === code);
      return {
        code,
        color: course?.Color ?? 'var(--text-faint)',
        name: course?.Name ?? '',
        items: [...items].sort((a, b) => Date.parse(b.GradedAt) - Date.parse(a.GradedAt)),
        score: items.reduce((acc, x) => acc + x.Score, 0),
        possible: items.reduce((acc, x) => acc + x.Possible, 0),
      };
    });
  });

  /** Means arrive only once GetDeadlineDetail has cached them, so many stay 0. */
  const meanCount = $derived($grades.filter((g) => g.Mean > 0).length);

  function pct(score: number, possible: number): number {
    if (!possible) return 0;
    return Math.max(0, Math.min(100, (score / possible) * 100));
  }

  function tone(p: number): string {
    if (p >= 80) return 'green';
    if (p >= 60) return 'accent';
    if (p >= 45) return 'amber';
    return 'red';
  }
</script>

<div class="view">
  <div class="view-narrow">
    <header class="head">
      <h1 class="page-title">Grades</h1>
      <p class="muted sub">
        {$grades.length} graded items across {grouped.length} courses{meanCount
          ? ` · class mean known for ${meanCount}`
          : ''}
      </p>
    </header>

    {#each grouped as g (g.code)}
      {@const overall = pct(g.score, g.possible)}
      <section class="course card">
        <div class="course-head">
          <span class="dot" style="background:{g.color}"></span>
          <span class="code">{g.code}</span>
          <span class="name truncate muted">{g.name}</span>
          <span class="overall">
            <span class="chip {tone(overall)}">{overall.toFixed(1)}%</span>
            <span class="raw faint">{fmtScore(g.score)} / {fmtScore(g.possible)}</span>
          </span>
        </div>

        <table class="table">
          <thead>
            <tr>
              <th class="c-title">Item</th>
              <th class="c-score">Score</th>
              <th class="c-mean">Class mean</th>
              <th class="c-bar">Percentage</th>
              <th class="c-date">Graded</th>
            </tr>
          </thead>
          <tbody>
            {#each g.items as item (item.CourseCode + item.Title)}
              {@const p = pct(item.Score, item.Possible)}
              <tr onclick={() => openExternal(item.URL)}>
                <td class="c-title truncate">{item.Title}</td>
                <td class="c-score">{fmtScore(item.Score)} / {fmtScore(item.Possible)}</td>
                <td class="c-mean">
                  {#if item.Mean}
                    {@const delta = item.Score - item.Mean}
                    <span class="mean-val">{fmtScore(item.Mean)}</span>
                    <span class="delta" class:up={delta >= 0} class:down={delta < 0}>
                      {delta >= 0 ? '+' : ''}{fmtScore(delta)} vs class
                    </span>
                  {:else}
                    <span class="faint">—</span>
                  {/if}
                </td>
                <td class="c-bar">
                  <span class="bar-wrap">
                    <span class="bar"><span class="fill {tone(p)}" style="width:{p}%"></span></span>
                    <span class="pct">{p.toFixed(0)}%</span>
                  </span>
                </td>
                <td class="c-date">
                  {fmtDate(item.GradedAt)}
                  <span class="go"><Icon name="external" size={12} /></span>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </section>
    {:else}
      <div class="card"><div class="empty">No grades released yet.</div></div>
    {/each}
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

  .course {
    margin-bottom: 16px;
    overflow: hidden;
  }

  .course-head {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 11px 14px;
    border-bottom: 1px solid var(--border);
    background: var(--bg-subtle);
  }

  .code {
    font-size: 13px;
    font-weight: 620;
  }

  .name {
    flex: 1;
    min-width: 0;
    font-size: 12.5px;
  }

  .overall {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: none;
  }

  .raw {
    font-size: 11.5px;
    font-variant-numeric: tabular-nums;
  }

  .table {
    width: 100%;
    border-collapse: collapse;
    table-layout: fixed;
  }

  th {
    padding: 6px 14px;
    text-align: left;
    font-size: 10.5px;
    font-weight: 600;
    letter-spacing: 0.03em;
    text-transform: uppercase;
    color: var(--text-faint);
    border-bottom: 1px solid var(--border);
  }

  tbody tr {
    border-bottom: 1px solid var(--border);
    cursor: pointer;
    transition: background var(--t);
  }

  tbody tr:last-child {
    border-bottom: none;
  }

  tbody tr:hover {
    background: var(--bg-hover);
  }

  td {
    padding: 8px 14px;
    font-size: 12.5px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .c-title {
    width: 32%;
    font-size: 13px;
  }

  .c-score {
    width: 14%;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
  }

  .c-bar {
    width: 22%;
  }

  .c-mean {
    width: 18%;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
  }

  .mean-val {
    margin-right: 5px;
  }

  .delta {
    font-size: 11px;
    font-weight: 550;
  }

  .delta.up {
    color: var(--green);
  }

  .delta.down {
    color: var(--amber);
  }

  .c-date {
    width: 16%;
    color: var(--text-faint);
    text-align: right;
  }

  .bar-wrap {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .bar {
    flex: 1;
    height: 5px;
    border-radius: 999px;
    background: var(--bg-subtle);
    overflow: hidden;
  }

  .fill {
    display: block;
    height: 100%;
    border-radius: 999px;
    transition: width var(--t);
  }

  .fill.green {
    background: var(--green);
  }

  .fill.accent {
    background: var(--accent);
  }

  .fill.amber {
    background: var(--amber);
  }

  .fill.red {
    background: var(--red);
  }

  .pct {
    width: 34px;
    text-align: right;
    font-size: 11.5px;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
  }

  .go {
    display: inline-flex;
    vertical-align: -2px;
    margin-left: 6px;
    opacity: 0;
    transition: opacity var(--t);
  }

  tbody tr:hover .go {
    opacity: 1;
  }
</style>
