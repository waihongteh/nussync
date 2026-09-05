<script lang="ts">
  import { dismissToast, toasts } from '../stores';
  import Icon from './Icon.svelte';

  const ICON: Record<string, string> = {
    info: 'info',
    success: 'checkCircle',
    error: 'alert',
  };
</script>

<div class="stack" role="status" aria-live="polite">
  {#each $toasts as t (t.id)}
    <div class="toast {t.level}">
      <Icon name={ICON[t.level] ?? 'info'} size={14} />
      <span class="msg">{t.message}</span>
      <button class="close" onclick={() => dismissToast(t.id)} aria-label="Dismiss">
        <Icon name="x" size={12} />
      </button>
    </div>
  {/each}
</div>

<style>
  .stack {
    position: fixed;
    right: 16px;
    bottom: 16px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    z-index: 200;
    pointer-events: none;
  }

  .toast {
    display: flex;
    align-items: center;
    gap: 9px;
    min-width: 240px;
    max-width: 380px;
    padding: 9px 10px 9px 11px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-elevated);
    box-shadow: var(--shadow-toast);
    font-size: 13px;
    pointer-events: auto;
    animation: slide-in 160ms cubic-bezier(0.2, 0.9, 0.3, 1);
  }

  .toast.success {
    color: var(--green);
  }

  .toast.error {
    color: var(--red);
  }

  .toast.info {
    color: var(--accent-text);
  }

  .msg {
    flex: 1;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .close {
    color: var(--text-faint);
    padding: 2px;
    border-radius: 4px;
    transition: color var(--t), background var(--t);
  }

  .close:hover {
    color: var(--text);
    background: var(--bg-hover);
  }

  @keyframes slide-in {
    from {
      opacity: 0;
      transform: translateY(6px) scale(0.98);
    }
    to {
      opacity: 1;
      transform: none;
    }
  }
</style>
