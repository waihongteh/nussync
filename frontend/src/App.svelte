<script lang="ts">
  import ChatPanel from './lib/components/ChatPanel.svelte';
  import CommandPalette from './lib/components/CommandPalette.svelte';
  import Icon from './lib/components/Icon.svelte';
  import PetOverlay from './lib/components/PetOverlay.svelte';
  import ResizeHandle from './lib/components/ResizeHandle.svelte';
  import Sidebar from './lib/components/Sidebar.svelte';
  import Toasts from './lib/components/Toasts.svelte';
  import Tooltip from './lib/components/Tooltip.svelte';
  import {
    CHAT_RAIL_W,
    CHAT_W_DEFAULT,
    CHAT_W_MAX,
    CHAT_W_MIN,
    chatCollapsed,
    chatW,
    initLayout,
    resolvedChatMode,
    setChatColumnWidth,
    setChatWidth,
    toggleSidebar,
  } from './lib/layout';
  import {
    chatOpen,
    initTheme,
    loadAll,
    navigate,
    paletteOpen,
    resolvedTheme,
    route,
    theme,
    wireEvents,
  } from './lib/stores';
  import { initPet } from './lib/pet';
  import { initQuest } from './lib/quest';
  import Announcements from './lib/views/Announcements.svelte';
  import Arcade from './lib/views/Arcade.svelte';
  import Deadlines from './lib/views/Deadlines.svelte';
  import Files from './lib/views/Files.svelte';
  import Grades from './lib/views/Grades.svelte';
  import Home from './lib/views/Home.svelte';
  import Papers from './lib/views/Papers.svelte';
  import Quest from './lib/views/Quest.svelte';
  import Settings from './lib/views/Settings.svelte';
  import Study from './lib/views/Study.svelte';
  import WhatsNew from './lib/views/WhatsNew.svelte';

  const TITLES: Record<string, string> = {
    home: 'Home',
    files: 'Files',
    whatsnew: "What's new",
    deadlines: 'Deadlines',
    announcements: 'Announcements',
    grades: 'Grades',
    study: 'Study',
    papers: 'Papers',
    arcade: 'Arcade',
    quest: 'Quest',
    settings: 'Settings',
  };

  initTheme();

  /** True when the chat is a column in the grid rather than a floating panel. */
  const dockedChat = $derived($chatOpen && $resolvedChatMode === 'docked');
  const chatColW = $derived($chatCollapsed ? CHAT_RAIL_W : $chatW);

  /**
   * Publish the column's real width so fixed-position things — the viewer's
   * focus mode, the pet's wander bounds — can stay clear of it.
   */
  $effect(() => {
    setChatColumnWidth(dockedChat ? chatColW : 0);
  });

  $effect(() => {
    const teardown = wireEvents();
    const petTeardown = initPet();
    const questTeardown = initQuest();
    const layoutTeardown = initLayout();
    void loadAll();
    return () => {
      teardown();
      questTeardown();
      petTeardown();
      layoutTeardown();
    };
  });

  function onKeydown(e: KeyboardEvent) {
    const mod = e.ctrlKey || e.metaKey;
    if (mod && (e.key === 'k' || e.key === 'K')) {
      e.preventDefault();
      paletteOpen.update((v) => !v);
      return;
    }
    if (mod && (e.key === 'j' || e.key === 'J')) {
      e.preventDefault();
      chatOpen.update((v) => !v);
      return;
    }
    if (mod && !e.shiftKey && (e.key === 'b' || e.key === 'B')) {
      e.preventDefault();
      toggleSidebar();
      return;
    }
    if (mod && e.key === ',') {
      e.preventDefault();
      paletteOpen.set(false);
      navigate('settings');
      return;
    }
    if (e.key === 'Escape') {
      paletteOpen.set(false);
      /*
       * A floating chat covers the view, so Esc always dismisses it. A docked
       * one is part of the layout, and the view underneath has its own Esc
       * (leave focus mode, close the preview) — so it only closes when the
       * focus is actually inside the chat.
       */
      if (!dockedChat || (e.target as HTMLElement | null)?.closest?.('aside.chat')) {
        chatOpen.set(false);
      }
    }
  }

  function cycleTheme() {
    theme.update((t) => (t === 'dark' ? 'light' : t === 'light' ? 'system' : 'dark'));
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="shell">
  <Sidebar />

  <main class="main">
    <header class="topbar">
      <h1 class="title">{TITLES[$route] ?? ''}</h1>
      <div class="grow"></div>
      <button class="search-trigger" onclick={() => paletteOpen.set(true)}>
        <Icon name="search" size={13} />
        <span>Search</span>
        <kbd>Ctrl</kbd><kbd>K</kbd>
      </button>
      <button
        class="icon-btn"
        class:on={$chatOpen}
        onclick={() => chatOpen.update((v) => !v)}
        title="Chat with Claude (Ctrl+J)"
        aria-label="Chat with Claude"
      >
        <Icon name="chat" size={14} />
      </button>
      <button class="icon-btn" onclick={cycleTheme} title="Theme: {$theme}" aria-label="Toggle theme">
        <Icon name={$theme === 'system' ? 'monitor' : $resolvedTheme === 'dark' ? 'moon' : 'sun'} size={14} />
      </button>
      <button class="icon-btn" onclick={() => navigate('settings')} title="Settings (Ctrl+,)" aria-label="Settings">
        <Icon name="settings" size={14} />
      </button>
    </header>

    <div class="content">
      {#if $route === 'home'}
        <Home />
      {:else if $route === 'files'}
        <Files />
      {:else if $route === 'deadlines'}
        <Deadlines />
      {:else if $route === 'announcements'}
        <Announcements />
      {:else if $route === 'grades'}
        <Grades />
      {:else if $route === 'whatsnew'}
        <WhatsNew />
      {:else if $route === 'study'}
        <Study />
      {:else if $route === 'papers'}
        <Papers />
      {:else if $route === 'arcade'}
        <Arcade />
      {:else if $route === 'quest'}
        <Quest />
      {:else if $route === 'settings'}
        <Settings />
      {/if}
    </div>
  </main>

  <!--
    Docked chat: a real column, so the view to its left simply gets narrower
    and the PDF underneath stays readable. The handle is skipped while the
    column is collapsed to its rail — there is nothing to size.
  -->
  {#if dockedChat}
    {#if !$chatCollapsed}
      <ResizeHandle
        value={$chatW}
        min={CHAT_W_MIN}
        max={CHAT_W_MAX}
        def={CHAT_W_DEFAULT}
        invert
        label="Resize the chat"
        onChange={setChatWidth}
      />
    {/if}
    <div class="chat-col" style="width:{chatColW}px">
      <ChatPanel docked />
    </div>
  {/if}
</div>

<!-- Free-roaming pet: a pointer-transparent layer above the views, below the
     chat panel / palette / toasts. Renders nothing in 'dock' mode. -->
<PetOverlay />

{#if !dockedChat}
  <ChatPanel />
{/if}
<CommandPalette />
<Toasts />
<Tooltip />

<style>
  .shell {
    display: flex;
    height: 100%;
    min-height: 0;
  }

  .main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  .topbar {
    display: flex;
    align-items: center;
    gap: 6px;
    height: var(--topbar-h);
    padding: 0 16px;
    border-bottom: 1px solid var(--border);
    flex: none;
  }

  .title {
    font-size: 13.5px;
    font-weight: 600;
    letter-spacing: -0.005em;
  }

  .grow {
    flex: 1;
  }

  .search-trigger {
    display: flex;
    align-items: center;
    gap: 7px;
    height: 28px;
    padding: 0 8px 0 9px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-elevated);
    color: var(--text-faint);
    font-size: 12.5px;
    transition: background var(--t), border-color var(--t), color var(--t);
  }

  .search-trigger:hover {
    background: var(--bg-hover);
    border-color: var(--border-strong);
    color: var(--text-muted);
  }

  .search-trigger kbd {
    padding: 1px 4px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg-subtle);
    font-family: var(--font);
    font-size: 10px;
    line-height: 1.3;
  }

  .icon-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border-radius: var(--radius-sm);
    color: var(--text-faint);
    transition: background var(--t), color var(--t);
  }

  .icon-btn:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .icon-btn.on {
    background: var(--bg-active);
    color: var(--accent-text);
  }

  .content {
    flex: 1;
    min-height: 0;
  }

  .chat-col {
    flex: none;
    min-width: 0;
    min-height: 0;
    display: flex;
    /* Above the pet layer (30) so the pet cannot be drawn over the chat. */
    position: relative;
    z-index: 35;
  }
</style>
