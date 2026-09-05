<script lang="ts">
  import { cancelSync, syncNow, syncStatus } from '../stores';
  import { relTime } from '../util';
  import Icon from './Icon.svelte';

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
</script>

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

<style>
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
