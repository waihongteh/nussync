<script lang="ts">
  import { cancelSync, syncNow, syncStatus } from '../stores';
  import { relTime } from '../util';
  import Icon from './Icon.svelte';
  import { tip } from '../tooltip';

  interface Props {
    /** Rail mode: one icon button with a progress ring, status in the tooltip. */
    compact?: boolean;
  }

  let { compact = false }: Props = $props();

  let tick = $state(Date.now());
  $effect(() => {
    const h = setInterval(() => (tick = Date.now()), 30_000);
    return () => clearInterval(h);
  });

  const s = $derived($syncStatus);
  const pct = $derived(s.Total > 0 ? Math.min(100, Math.round((s.Done / s.Total) * 100)) : s.Running ? 6 : 0);
  const phaseLabel = $derived(
    s.Phase === 'listing'
      ? 'Listing courses'
      : s.Phase === 'downloading'
        ? 'Downloading'
        : s.Phase === 'indexing'
          ? 'Indexing'
          : s.Phase === 'error'
            ? 'Sync failed'
            : 'Idle',
  );
  const detail = $derived(s.CurrentFile || s.Course || '');
  const lastRun = $derived(relTime(s.LastRun, tick));

  // ------------------------------------------------------------------ rail

  /** r=8 ring; the dash offset walks the circumference as the sync progresses. */
  const C = 2 * Math.PI * 8;
  const dash = $derived(C * (1 - pct / 100));

  const compactTip = $derived(
    s.Running
      ? `${phaseLabel}${s.Total > 0 ? ` ${s.Done}/${s.Total}` : ''}${detail ? ` · ${detail}` : ''} · click to cancel`
      : s.Phase === 'error'
        ? `Sync failed${s.LastError ? `: ${s.LastError}` : ''} · click to retry`
        : `Last synced ${lastRun} · click to sync now`,
  );
</script>

{#if compact}
  <button
    class="pill mini"
    class:running={s.Running}
    class:error={s.Phase === 'error'}
    onclick={() => (s.Running ? cancelSync() : syncNow())}
    aria-label={compactTip}
    use:tip={{ text: compactTip, side: 'right' }}
  >
    {#if s.Running}
      <svg class="ring" viewBox="0 0 20 20" aria-hidden="true">
        <circle class="ring-track" cx="10" cy="10" r="8" />
        <circle class="ring-fill" cx="10" cy="10" r="8" stroke-dasharray={C} stroke-dashoffset={dash} />
      </svg>
    {/if}
    <Icon name={s.Phase === 'error' ? 'alert' : s.Running ? 'sync' : 'check'} size={14} />
  </button>
{:else}
<div class="pill" class:running={s.Running} class:error={s.Phase === 'error'}>
  <div class="row">
    <span class="state">
      {#if s.Running}
        <span class="spinner"></span>
      {:else if s.Phase === 'error'}
        <Icon name="alert" size={13} />
      {:else}
        <Icon name="check" size={13} />
      {/if}
      <span class="label">{phaseLabel}</span>
    </span>
    {#if s.Running}
      <button class="act" onclick={cancelSync}>Cancel</button>
    {:else}
      <button class="act" onclick={syncNow}>
        <Icon name="sync" size={12} /> Sync now
      </button>
    {/if}
  </div>

  {#if s.Running}
    <div class="bar"><div class="fill" style="width:{pct}%"></div></div>
    <div class="sub truncate">
      {#if s.Total > 0}<span class="count">{s.Done}/{s.Total}</span>{/if}
      {detail}
    </div>
  {:else if s.Phase === 'error' && s.LastError}
    <div class="sub err truncate">{s.LastError}</div>
  {:else}
    <div class="sub">Last synced {lastRun}</div>
  {/if}
</div>
{/if}

<style>
  /* ------------------------------------------------------------------ rail */

  .pill.mini {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 34px;
    height: 34px;
    margin: 0 auto 10px;
    padding: 0;
    color: var(--text-muted);
  }

  .pill.mini:hover {
    background: var(--bg-hover);
  }

  .pill.mini.running {
    color: var(--accent-text);
  }

  .pill.mini.error {
    color: var(--red);
  }

  .ring {
    position: absolute;
    inset: 5px;
    width: 24px;
    height: 24px;
    transform: rotate(-90deg);
    overflow: visible;
  }

  .ring-track,
  .ring-fill {
    fill: none;
    stroke-width: 2;
  }

  .ring-track {
    stroke: var(--border);
  }

  .ring-fill {
    stroke: var(--accent);
    stroke-linecap: round;
    transition: stroke-dashoffset 220ms cubic-bezier(0.4, 0, 0.2, 1);
  }

  .pill {
    margin: 0 10px 10px;
    padding: 9px 10px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-elevated);
    transition: border-color var(--t), background var(--t);
  }

  .pill.running {
    border-color: var(--accent);
    background: var(--accent-soft);
  }

  .pill.error {
    border-color: var(--red);
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  .state {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    color: var(--text-muted);
  }

  .pill.running .state {
    color: var(--accent-text);
  }

  .pill.error .state {
    color: var(--red);
  }

  .label {
    font-size: 12.5px;
    font-weight: 550;
    color: var(--text);
  }

  .act {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 12px;
    font-weight: 550;
    color: var(--accent-text);
    padding: 2px 4px;
    border-radius: 4px;
    transition: background var(--t);
  }

  .act:hover {
    background: var(--bg-hover);
  }

  .bar {
    height: 3px;
    margin-top: 8px;
    border-radius: 999px;
    background: var(--border);
    overflow: hidden;
  }

  .fill {
    height: 100%;
    border-radius: 999px;
    background: var(--accent);
    transition: width 220ms cubic-bezier(0.4, 0, 0.2, 1);
  }

  .sub {
    margin-top: 5px;
    font-size: 11.5px;
    color: var(--text-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .sub.err {
    color: var(--red);
  }

  .count {
    font-variant-numeric: tabular-nums;
    color: var(--text-muted);
    margin-right: 4px;
  }
</style>
