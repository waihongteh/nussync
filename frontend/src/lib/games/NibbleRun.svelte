<script lang="ts">
  /**
   * Nibble Run view: owns the canvas, the loop and the HUD. All simulation
   * lives in ./nibbleRun.ts.
   */
  import { onDestroy } from 'svelte';
  import Icon from '../components/Icon.svelte';
  import { addXP, level } from '../pet';
  import { deadlines, resolvedTheme } from '../stores';
  import { fitCanvas, hashSeed, Keys, Loop, prefersReducedMotion, todayKey } from './engine';
  import { hurdlesFromDeadlines, NibbleRun } from './nibbleRun';
  import { dailyBestToday, recordRun, runSave } from './scores';

  interface Props {
    /** Daily runs share one seed per calendar day; free play is random. */
    mode: 'daily' | 'free';
    onExit: () => void;
  }

  let { mode, onExit }: Props = $props();

  /** Never award more than this per run — the runner is not an xp farm. */
  const XP_CAP = 30;

  let wrapEl = $state<HTMLDivElement | null>(null);
  let canvasEl = $state<HTMLCanvasElement | null>(null);

  let score = $state(0);
  let speed = $state(0);
  let over = $state(false);
  let ready = $state(true);
  let paused = $state(false);
  let deathMsg = $state('');
  let runXP = 0;

  const reduced = prefersReducedMotion();
  const best = $derived($runSave.best);
  const daily = $derived(dailyBestToday($runSave));

  let game: NibbleRun | null = null;
  let loop: Loop | null = null;
  let keys: Keys | null = null;
  let ro: ResizeObserver | null = null;
  let cssW = $state(960);
  let cssH = $state(540);

  function seed(): number {
    return mode === 'daily' ? hashSeed(todayKey()) : (Math.random() * 0xffffffff) >>> 0;
  }

  function layout() {
    if (!wrapEl || !canvasEl || !game) return;
    const r = wrapEl.getBoundingClientRect();
    const availW = Math.max(320, r.width);
    const availH = Math.max(200, r.height);
    let w = Math.min(960, availW);
    let h = Math.round((w * 9) / 16);
    if (h > availH) {
      h = Math.min(540, availH);
      w = Math.round((h * 16) / 9);
    }
    cssW = Math.round(w);
    cssH = Math.round(h);
    game.resize(cssW, cssH);
  }

  function start() {
    if (!canvasEl) return;
    keys = new Keys(['Space', 'ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight']);
    game = new NibbleRun(hurdlesFromDeadlines($deadlines), seed(), $level, reduced, {
      onCollect: () => {
        if (runXP < XP_CAP) {
          runXP++;
          addXP(1, 'nibble run');
        }
      },
      onDeath: (s, by) => {
        over = true;
        deathMsg = `${by} got you.`;
        recordRun(s, mode === 'daily');
      },
    });
    layout();
    keys.attach();

    loop = new Loop({
      step: (dt) => {
        const g = game;
        const k = keys;
        if (!g || !k) return;
        if (k.hit('Escape', 'KeyP')) paused = !paused;
        if (!paused) {
          if (k.hit('Space', 'ArrowUp', 'KeyW')) {
            if (g.state === 'dead') restart();
            else g.jump();
          }
          g.setDuck(k.held('ArrowDown', 'KeyS'));
          g.step(dt);
        }
        k.flush();
        score = g.score;
        speed = g.speed;
        ready = g.state === 'ready';
      },
      render: () => {
        if (!game || !canvasEl) return;
        const ctx = fitCanvas(canvasEl, cssW, cssH);
        game.render(ctx);
      },
      onAutoPause: () => {
        if (game && game.state === 'running') paused = true;
      },
    });
    loop.start();

    ro = new ResizeObserver(() => layout());
    if (wrapEl) ro.observe(wrapEl);
  }

  function restart() {
    if (!game) return;
    runXP = 0;
    over = false;
    deathMsg = '';
    paused = false;
    game.reset();
    game.resize(cssW, cssH);
    game.state = 'running';
  }

  function tap() {
    if (!game) return;
    if (game.state === 'dead') restart();
    else game.jump();
  }

  $effect(() => {
    // canvasEl is bound after the first paint
    if (canvasEl && !loop) start();
  });

  // Follow the app theme without restarting the run.
  $effect(() => {
    void $resolvedTheme;
    game?.refreshPalette();
  });

  onDestroy(() => {
    loop?.stop();
    keys?.detach();
    ro?.disconnect();
    loop = null;
    keys = null;
    ro = null;
    game = null;
  });
</script>

<div class="game">
  <header class="bar">
    <button class="btn sm ghost" onclick={onExit}>
      <Icon name="chevronLeft" size={13} />
      Arcade
    </button>
    <span class="chip {mode === 'daily' ? 'accent' : ''}">
      {mode === 'daily' ? `Daily run · ${todayKey()}` : 'Free play'}
    </span>
    <div class="grow"></div>
    <span class="stat"><span class="k">Score</span><span class="v">{score}</span></span>
    <span class="stat"><span class="k">Best</span><span class="v">{best}</span></span>
    {#if mode === 'daily'}
      <span class="stat"><span class="k">Today</span><span class="v">{daily}</span></span>
    {/if}
    <span class="stat"><span class="k">Speed</span><span class="v">{(speed / 100).toFixed(1)}x</span></span>
  </header>

  <div class="stage" bind:this={wrapEl}>
    <div class="frame" style="width:{cssW}px;height:{cssH}px">
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <canvas bind:this={canvasEl} onpointerdown={tap}></canvas>

      {#if ready && !over}
        <div class="overlay">
          <p class="big">Nibble Run</p>
          <p class="muted">Space or click to jump · hold Down to duck</p>
          {#if $level >= 4}<p class="faint sm">Double jump unlocked</p>{/if}
        </div>
      {:else if over}
        <div class="overlay">
          <p class="big">{deathMsg}</p>
          <p class="muted">Score {score} · best {best}</p>
          <p class="faint sm">Space to run it back</p>
        </div>
      {:else if paused}
        <div class="overlay">
          <p class="big">Paused</p>
          <p class="muted">Esc or P to resume</p>
        </div>
      {/if}
    </div>
  </div>

  <footer class="hints faint">
    <kbd>Space</kbd> jump · <kbd>↓</kbd> duck · <kbd>Esc</kbd> pause · hurdles are your unsubmitted
    deadlines
  </footer>
</div>

<style>
  .game {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
    padding: 14px 18px 12px;
    gap: 10px;
  }

  .bar {
    display: flex;
    align-items: center;
    gap: 10px;
    flex: none;
  }

  .grow {
    flex: 1;
  }

  .stat {
    display: inline-flex;
    align-items: baseline;
    gap: 5px;
    font-variant-numeric: tabular-nums;
  }

  .stat .k {
    font-size: 10.5px;
    font-weight: 600;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    color: var(--text-faint);
  }

  .stat .v {
    font-size: 14px;
    font-weight: 620;
  }

  .stage {
    flex: 1;
    min-height: 0;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .frame {
    position: relative;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    overflow: hidden;
    background: var(--bg-elevated);
  }

  canvas {
    display: block;
    touch-action: none;
  }

  .overlay {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 4px;
    text-align: center;
    background: color-mix(in srgb, var(--bg-elevated) 78%, transparent);
    pointer-events: none;
  }

  .big {
    font-size: 18px;
    font-weight: 620;
    letter-spacing: -0.01em;
  }

  .sm {
    font-size: 11.5px;
  }

  .hints {
    flex: none;
    font-size: 11.5px;
    text-align: center;
  }

  kbd {
    padding: 1px 5px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg-subtle);
    font-family: var(--font);
    font-size: 10.5px;
  }
</style>
