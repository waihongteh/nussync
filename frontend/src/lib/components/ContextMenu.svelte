<script lang="ts">
  import type { MenuItem } from '../types';
  import Icon from './Icon.svelte';

  interface Props {
    x: number;
    y: number;
    items: MenuItem[];
    onClose: () => void;
  }

  let { x, y, items, onClose }: Props = $props();

  let el = $state<HTMLDivElement | null>(null);

  // Clamp into the viewport; recomputes once `el` is bound and its size known.
  const pos = $derived.by(() => {
    const w = el?.offsetWidth ?? 190;
    const h = el?.offsetHeight ?? 120;
    return {
      left: Math.max(8, Math.min(x, window.innerWidth - w - 8)),
      top: Math.max(8, Math.min(y, window.innerHeight - h - 8)),
    };
  });

  $effect(() => {
    const off = () => onClose();
    const key = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('mousedown', off);
    window.addEventListener('resize', off);
    window.addEventListener('keydown', key);
    return () => {
      window.removeEventListener('mousedown', off);
      window.removeEventListener('resize', off);
      window.removeEventListener('keydown', key);
    };
  });
</script>

<div
  bind:this={el}
  class="menu"
  style="left:{pos.left}px; top:{pos.top}px"
  role="menu"
  tabindex="-1"
  onmousedown={(e) => e.stopPropagation()}
>
  {#each items as item (item.label)}
    <button
      class="item"
      class:danger={item.danger}
      disabled={item.disabled}
      role="menuitem"
      onclick={() => {
        onClose();
        item.run();
      }}
    >
      {#if item.icon}<Icon name={item.icon} size={13} />{:else}<span class="gap"></span>{/if}
      <span>{item.label}</span>
    </button>
  {/each}
</div>

<style>
  .menu {
    position: fixed;
    z-index: 170;
    min-width: 186px;
    padding: 4px;
    background: var(--bg-elevated);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    box-shadow: var(--shadow-pop);
    animation: pop 100ms ease;
  }

  .item {
    display: flex;
    align-items: center;
    gap: 9px;
    width: 100%;
    height: 28px;
    padding: 0 8px;
    border-radius: var(--radius-sm);
    color: var(--text);
    font-size: 12.5px;
    text-align: left;
    transition: background var(--t), color var(--t);
  }

  .item:hover:not(:disabled) {
    background: var(--bg-active);
    color: var(--accent-text);
  }

  .item:disabled {
    color: var(--text-faint);
    cursor: default;
  }

  .item.danger:hover:not(:disabled) {
    background: var(--red-soft);
    color: var(--red);
  }

  .gap {
    width: 13px;
  }

  @keyframes pop {
    from {
      opacity: 0;
      transform: scale(0.97);
    }
  }
</style>
