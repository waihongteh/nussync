<script lang="ts">
  import { api, errMsg } from '../api';
  import Icon from '../components/Icon.svelte';
  import { announcements, openExternal, toast } from '../stores';
  import { relTime, sanitizeHTML } from '../util';

  let openID = $state<number | null>(null);
  let tick = $state(Date.now());

  $effect(() => {
    const h = setInterval(() => (tick = Date.now()), 60_000);
    return () => clearInterval(h);
  });

  const bodyCache = new Map<number, string>();

  function bodyFor(id: number, html: string, text: string): string {
    let cached = bodyCache.get(id);
    if (cached === undefined) {
      cached = html ? sanitizeHTML(html) : `<p>${text.replace(/[<>&]/g, '')}</p>`;
      bodyCache.set(id, cached);
    }
    return cached;
  }

  async function toggle(id: number, read: boolean) {
    if (openID === id) {
      openID = null;
      return;
    }
    openID = id;
    if (read) return;
    try {
      await api.markAnnouncementRead(id);
      announcements.update((list) => list.map((a) => (a.ID === id ? { ...a, Read: true } : a)));
    } catch (err) {
      toast(`Could not mark as read: ${errMsg(err)}`, 'error');
    }
  }

  /** Links inside announcement bodies go through the backend, not the webview. */
  function interceptLinks(e: MouseEvent) {
    const anchor = (e.target as HTMLElement | null)?.closest?.('a');
    if (!anchor) return;
    e.preventDefault();
    const href = anchor.getAttribute('href') ?? '';
    if (href) void openExternal(href);
  }

  const unread = $derived($announcements.filter((a) => !a.Read).length);
</script>

<div class="view">
  <div class="view-narrow">
    <header class="head">
      <h1 class="page-title">Announcements</h1>
      <p class="muted sub">{$announcements.length} posts · {unread} unread</p>
    </header>

    <div class="card list">
      {#each $announcements as a (a.ID)}
        {@const isOpen = openID === a.ID}
        <div class="item" class:open={isOpen}>
          <button class="row" onclick={() => toggle(a.ID, a.Read)} aria-expanded={isOpen}>
            <span class="tw" class:open={isOpen}><Icon name="chevronRight" size={12} /></span>
            <span class="code">{a.CourseCode}</span>
            <span class="title truncate" class:unread={!a.Read}>{a.Title}</span>
            {#if !a.Read}<span class="dotmark" title="Unread"></span>{/if}
            <span class="time">{relTime(a.PostedAt, tick)}</span>
          </button>

          {#if isOpen}
            <div class="body">
              <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
              <div class="prose" onclick={interceptLinks}>
                {@html bodyFor(a.ID, a.HTML, a.Text)}
              </div>
              {#if a.URL}
                <button class="btn sm open-canvas" onclick={() => openExternal(a.URL)}>
                  <Icon name="external" size={12} /> Open in Canvas
                </button>
              {/if}
            </div>
          {/if}
        </div>
      {:else}
        <div class="empty">No announcements yet.</div>
      {/each}
    </div>
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

  .list {
    overflow: hidden;
  }

  .item {
    border-bottom: 1px solid var(--border);
  }

  .item:last-child {
    border-bottom: none;
  }

  .item.open {
    background: var(--bg-subtle);
  }

  .row {
    display: flex;
    align-items: center;
    gap: 9px;
    width: 100%;
    padding: 10px 13px;
    text-align: left;
    transition: background var(--t);
  }

  .row:hover {
    background: var(--bg-hover);
  }

  .tw {
    color: var(--text-faint);
    display: flex;
    transition: transform var(--t);
    flex: none;
  }

  .tw.open {
    transform: rotate(90deg);
  }

  .code {
    flex: none;
    min-width: 64px;
    font-size: 11.5px;
    font-weight: 600;
    color: var(--text-muted);
  }

  .title {
    flex: 1;
    min-width: 0;
    font-size: 13px;
  }

  .title.unread {
    font-weight: 600;
  }

  .dotmark {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--accent);
    flex: none;
  }

  .time {
    flex: none;
    font-size: 11.5px;
    color: var(--text-faint);
    min-width: 64px;
    text-align: right;
  }

  .body {
    padding: 4px 16px 16px 47px;
    animation: reveal 140ms ease;
  }

  .prose {
    font-size: 13px;
    line-height: 1.62;
    color: var(--text-muted);
    max-width: 68ch;
  }

  .prose :global(p) {
    margin: 0 0 9px;
  }

  .prose :global(ul),
  .prose :global(ol) {
    margin: 0 0 9px;
    padding-left: 20px;
    list-style: disc;
  }

  .prose :global(ol) {
    list-style: decimal;
  }

  .prose :global(li) {
    margin-bottom: 3px;
  }

  .prose :global(strong) {
    color: var(--text);
    font-weight: 600;
  }

  .prose :global(a) {
    color: var(--accent-text);
  }

  .prose :global(img) {
    max-width: 100%;
    border-radius: var(--radius-sm);
  }

  .prose :global(table) {
    border-collapse: collapse;
    font-size: 12.5px;
  }

  .prose :global(td),
  .prose :global(th) {
    border: 1px solid var(--border);
    padding: 4px 8px;
  }

  .open-canvas {
    margin-top: 6px;
  }

  @keyframes reveal {
    from {
      opacity: 0;
      transform: translateY(-3px);
    }
  }
</style>
