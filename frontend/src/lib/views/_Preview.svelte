<script lang="ts">
  /**
   * Development-only harness. Mounted by main.ts ONLY when the URL carries
   * `preview=` — production and the Wails shell never reach this file's mount
   * path, so it costs nothing at runtime beyond the bundle.
   *
   * Usage: `npm run dev` then open /?preview=1&view=study
   */
  import Toasts from '../components/Toasts.svelte';
  import { initTheme, loadAll, route, theme, wireEvents } from '../stores';
  import Deadlines from './Deadlines.svelte';
  import Files from './Files.svelte';
  import Grades from './Grades.svelte';
  import Home from './Home.svelte';
  import QuizRush from './QuizRush.svelte';
  import Settings from './Settings.svelte';
  import Study from './Study.svelte';
  import WhatsNew from './WhatsNew.svelte';

  const VIEWS = ['home', 'files', 'whatsnew', 'deadlines', 'grades', 'study', 'quizrush', 'settings'] as const;
  type View = (typeof VIEWS)[number];

  function fromURL(): View {
    const v = new URLSearchParams(location.search).get('view') ?? 'whatsnew';
    return (VIEWS as readonly string[]).includes(v) ? (v as View) : 'whatsnew';
  }

  let view = $state<View>(fromURL());

  initTheme();

  $effect(() => {
    const teardown = wireEvents();
    void loadAll();
    return teardown;
  });

  // Keep the URL honest so a reload lands on the same view.
  $effect(() => {
    const url = new URL(location.href);
    url.searchParams.set('preview', '1');
    url.searchParams.set('view', view);
    history.replaceState(null, '', url.toString());
  });

  // Views that call navigate() (Home's "see what's new" line) move the shared
  // route store; follow it so those links work in the harness too.
  $effect(() => {
    const r = $route;
    if ((VIEWS as readonly string[]).includes(r)) view = r as View;
  });

  // The arcade game asks for Study through the same DOM event the app wires.
  $effect(() => {
    const onNav = (e: Event) => {
      const want = (e as CustomEvent<{ view?: string }>).detail?.view;
      if (want && (VIEWS as readonly string[]).includes(want)) view = want as View;
    };
    window.addEventListener('nussync:navigate', onNav);
    return () => window.removeEventListener('nussync:navigate', onNav);
  });
</script>

<div class="prev">
  <header class="bar">
    <span class="tag">preview</span>
    {#each VIEWS as v (v)}
      <button class="pv" class:on={view === v} onclick={() => (view = v)}>{v}</button>
    {/each}
    <span class="grow"></span>
    <button class="pv" onclick={() => theme.update((t) => (t === 'dark' ? 'light' : 'dark'))}>theme: {$theme}</button>
  </header>

  <div class="body">
    {#if view === 'home'}
      <Home />
    {:else if view === 'files'}
      <Files />
    {:else if view === 'whatsnew'}
      <WhatsNew />
    {:else if view === 'deadlines'}
      <Deadlines />
    {:else if view === 'grades'}
      <Grades />
    {:else if view === 'study'}
      <Study />
    {:else if view === 'quizrush'}
      <QuizRush />
    {:else}
      <Settings />
    {/if}
  </div>
</div>

<Toasts />

<style>
  .prev {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }

  .bar {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 6px 10px;
    border-bottom: 1px solid var(--border);
    background: var(--bg-sidebar);
    flex: none;
  }

  .tag {
    font-size: 10.5px;
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--amber);
    margin-right: 6px;
  }

  .pv {
    height: 24px;
    padding: 0 9px;
    border-radius: var(--radius-sm);
    font-size: 12px;
    color: var(--text-muted);
  }

  .pv:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .pv.on {
    background: var(--bg-active);
    color: var(--accent-text);
  }

  .grow {
    flex: 1;
  }

  .body {
    flex: 1;
    min-height: 0;
  }
</style>
