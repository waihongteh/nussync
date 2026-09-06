<script lang="ts">
  /**
   * The single tooltip bubble. Mounted once from App; every `use:tip` anchor
   * feeds it through the shared store, so only one can ever be on screen.
   */
  import { tipState } from '../tooltip';

  const GAP = 8;
  const MARGIN = 6;

  let el = $state<HTMLDivElement | null>(null);
  let x = $state(0);
  let y = $state(0);

  /**
   * Measure after render, then clamp into the viewport. `$state` for the
   * measured size keeps this a single extra frame rather than a layout loop.
   */
  $effect(() => {
    const s = $tipState;
    if (!s || !el) return;
    const r = el.getBoundingClientRect();
    if (s.side === 'top') {
      x = Math.min(window.innerWidth - MARGIN - r.width, Math.max(MARGIN, s.x - r.width / 2));
      y = Math.max(MARGIN, s.y - r.height - GAP);
    } else {
      x = Math.min(window.innerWidth - MARGIN - r.width, s.x + GAP);
      y = Math.min(window.innerHeight - MARGIN - r.height, Math.max(MARGIN, s.y - r.height / 2));
    }
  });

  /** Any scroll or key wipes it, the way an OS tooltip goes away. */
  function dismiss() {
    tipState.set(null);
  }
</script>

<svelte:window onkeydown={dismiss} onscroll={dismiss} onresize={dismiss} />

{#if $tipState}
  <div bind:this={el} class="tip" style="left:{x}px;top:{y}px" role="tooltip">{$tipState.text}</div>
{/if}

<style>
  .tip {
    position: fixed;
    z-index: 210;
    max-width: 260px;
    padding: 4px 8px;
    border: 1px solid var(--border-strong);
    border-radius: 6px;
    background: var(--bg-elevated);
    box-shadow: var(--shadow-pop);
    color: var(--text);
    font-size: 11.5px;
    line-height: 1.4;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    pointer-events: none;
    animation: tip-in 90ms ease-out;
  }

  @keyframes tip-in {
    from {
      opacity: 0;
    }
  }
</style>
