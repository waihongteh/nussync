<script lang="ts">
  import ChatPanel from './lib/components/ChatPanel.svelte';
  import CommandPalette from './lib/components/CommandPalette.svelte';
  import Icon from './lib/components/Icon.svelte';
  import Sidebar from './lib/components/Sidebar.svelte';
  import Toasts from './lib/components/Toasts.svelte';
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
  import Announcements from './lib/views/Announcements.svelte';
  import Arcade from './lib/views/Arcade.svelte';
  import Deadlines from './lib/views/Deadlines.svelte';
  import Files from './lib/views/Files.svelte';
  import Grades from './lib/views/Grades.svelte';
  import Home from './lib/views/Home.svelte';
  import Papers from './lib/views/Papers.svelte';
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
    settings: 'Settings',
  };

  initTheme();

  $effect(() => {
    const teardown = wireEvents();
    const petTeardown = initPet();
    void loadAll();
    return () => {
      teardown();
      petTeardown();
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
    if (mod && e.key === ',') {
      e.preventDefault();
      paletteOpen.set(false);
      navigate('settings');
      return;
    }
    if (e.key === 'Escape') {
      paletteOpen.set(false);
      chatOpen.set(false);
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
      {:else if $route === 'settings'}
        <Settings />
      {/if}
    </div>
  </main>
</div>

<ChatPanel />
<CommandPalette />
<Toasts />

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
</style>
