<script lang="ts">
  /**
   * The Arcade: a picker for the two games plus their high scores. Each card
   * carries a small static canvas drawn from the same primitives the games
   * use, so the previews follow the theme.
   */
  import { onDestroy } from 'svelte';
  import Icon from '../components/Icon.svelte';
  import { level } from '../pet';
  import { deadlines, navigate, resolvedTheme } from '../stores';
  import { streak, tierBeaten } from '../quest';
  import { arcadePick, type ArcadeGame } from '../games/arcade';
  import {
    fitCanvas,
    luminance,
    mix,
    readPalette,
    rgbCSS,
    roundRect,
    todayKey,
    type Palette,
  } from '../games/engine';
  import { hurdlesFromDeadlines } from '../games/nibbleRun';
  import { dailyBestToday, mergeSave, runSave } from '../games/scores';
  import { drawFileGlyph, drawPet } from '../games/sprites';
  import LectureMerge from '../games/LectureMerge.svelte';
  import QuizRush from './QuizRush.svelte';
  import NibbleRun from '../games/NibbleRun.svelte';

  const PREVIEW_W = 268;
  const PREVIEW_H = 96;

  let runCanvas = $state<HTMLCanvasElement | null>(null);
  let mergeCanvas = $state<HTMLCanvasElement | null>(null);

  const hurdleCount = $derived(hurdlesFromDeadlines($deadlines).length);
  const todayBest = $derived(dailyBestToday($runSave));

  function play(g: ArcadeGame) {
    arcadePick.set(g);
  }

  function back() {
    arcadePick.set(null);
  }

  // Leaving the view always returns to the picker next time.
  onDestroy(() => arcadePick.set(null));

  // Previews redraw whenever the theme flips.
  $effect(() => {
    void $resolvedTheme;
    const p = readPalette();
    if (runCanvas) drawRunPreview(fitCanvas(runCanvas, PREVIEW_W, PREVIEW_H), p);
    if (mergeCanvas) drawMergePreview(fitCanvas(mergeCanvas, PREVIEW_W, PREVIEW_H), p);
  });

  function drawRunPreview(ctx: CanvasRenderingContext2D, p: Palette) {
    const W = PREVIEW_W;
    const H = PREVIEW_H;
    const ground = H - 20;
    ctx.clearRect(0, 0, W, H);
    ctx.fillStyle = rgbCSS(p.elevated);
    ctx.fillRect(0, 0, W, H);

    for (let i = 0; i < 16; i++) {
      const x = ((i * 97) % W) + 6;
      const y = 10 + ((i * 53) % (ground - 30));
      ctx.fillStyle = rgbCSS(p.faint, 0.18 + (i % 3) * 0.08);
      ctx.beginPath();
      ctx.arc(x, y, 1 + (i % 3) * 0.7, 0, Math.PI * 2);
      ctx.fill();
    }

    ctx.strokeStyle = rgbCSS(p.border);
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.moveTo(0, ground + 0.5);
    ctx.lineTo(W, ground + 0.5);
    ctx.stroke();

    ctx.fillStyle = rgbCSS(p.accent, 0.12);
    ctx.strokeStyle = rgbCSS(p.accent, 0.85);
    ctx.lineWidth = 1.4;
    roundRect(ctx, W - 92, ground - 42, 40, 42, 5);
    ctx.fill();
    ctx.stroke();

    drawFileGlyph(ctx, p, W - 34, ground - 46, 18);
    drawPet(ctx, p, 46, ground, 40, { level: $level });
  }

  function drawMergePreview(ctx: CanvasRenderingContext2D, p: Palette) {
    const W = PREVIEW_W;
    const H = PREVIEW_H;
    ctx.clearRect(0, 0, W, H);
    ctx.fillStyle = rgbCSS(p.elevated);
    ctx.fillRect(0, 0, W, H);

    const ranks = [1, 2, 3, 5, 8, 15];
    const size = 40;
    const gap = 7;
    const total = ranks.length * size + (ranks.length - 1) * gap;
    let x = (W - total) / 2;
    const y = (H - size) / 2;
    for (const rank of ranks) {
      const t = Math.min(1, (rank - 1) / 10);
      let bg = mix(p.subtle, p.accent, 0.1 + t * 0.9);
      if (rank > 11) bg = mix(bg, p.dark ? [255, 255, 255] : [0, 0, 0], (rank - 11) * 0.07);
      ctx.fillStyle = rgbCSS(bg);
      roundRect(ctx, x, y, size, size, 7);
      ctx.fill();
      ctx.fillStyle = luminance(bg) > 0.42 ? rgbCSS(p.text) : '#fff';
      ctx.font = `640 ${rank >= 14 ? 10 : 13}px ${p.font}`;
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText(rank <= 13 ? `L${rank}` : 'Final', x + size / 2, y + size / 2 + 0.5);
      x += size + gap;
    }
  }
</script>

{#if $arcadePick === 'run-daily' || $arcadePick === 'run-free'}
  <NibbleRun mode={$arcadePick === 'run-daily' ? 'daily' : 'free'} onExit={back} />
{:else if $arcadePick === 'merge'}
  <LectureMerge onExit={back} />
{:else if $arcadePick === 'quiz-rush'}
  <QuizRush />
{:else}
  <div class="view">
    <div class="view-narrow">
      <header class="head">
        <h1 class="page-title">Arcade</h1>
        <p class="muted sub">Small breaks. Your deadlines and quizzes still show up.</p>
      </header>

      <div class="cards">
        <section class="card game">
          <canvas bind:this={runCanvas} class="preview"></canvas>
          <div class="body">
            <div class="row">
              <h2 class="name">Nibble Run</h2>
              <span class="chip accent">Daily run · {todayKey()}</span>
            </div>
            <p class="muted desc">
              Endless runner. The hurdles are your {hurdleCount} unsubmitted deadlines — the closer
              they are, the taller they get. Collect files for score and xp.
            </p>
            <div class="scores">
              <span class="stat"><span class="k">Best</span><span class="v">{$runSave.best}</span></span>
              <span class="stat"><span class="k">Today</span><span class="v">{todayBest}</span></span>
            </div>
            <p class="faint keys">
              <kbd>Space</kbd> jump · <kbd>↓</kbd> duck · <kbd>Esc</kbd> pause{$level >= 4
                ? ' · double jump unlocked'
                : ''}
            </p>
            <div class="actions">
              <button class="btn sm primary" onclick={() => play('run-daily')}>
                <Icon name="gamepad" size={13} />
                Daily run
              </button>
              <button class="btn sm" onclick={() => play('run-free')}>Free play</button>
            </div>
          </div>
        </section>

        <section class="card game">
          <canvas bind:this={mergeCanvas} class="preview"></canvas>
          <div class="body">
            <div class="row">
              <h2 class="name">Lecture Merge</h2>
              <span class="chip">2048</span>
            </div>
            <p class="muted desc">
              Slide and merge lectures: L1 through L13, then Midterm, Final, A+. Reaching Final is
              worth 50 xp, once a day.
            </p>
            <div class="scores">
              <span class="stat"><span class="k">Best</span><span class="v">{$mergeSave.best}</span></span>
            </div>
            <p class="faint keys">
              <kbd>↑↓←→</kbd> or <kbd>WASD</kbd> · <kbd>U</kbd> undo once · <kbd>R</kbd> restart
            </p>
            <div class="actions">
              <button class="btn sm primary" onclick={() => play('merge')}>
                <Icon name="gamepad" size={13} />
                Play
              </button>
            </div>
          </div>
        </section>
        <section class="card game">
          <div class="body">
            <div class="row">
              <h2 class="name">Quiz Rush</h2>
              <span class="chip accent">Study bank</span>
            </div>
            <p class="muted desc">
              Race your own quizzes. Three lives, a timer that tightens as your streak grows,
              a short-answer boss every 10th question. Missed ones come back in a redo pile.
            </p>
            <p class="faint keys"><kbd>1</kbd>–<kbd>4</kbd> answer · <kbd>Enter</kbd> continue</p>
            <div class="actions">
              <button class="btn sm primary" onclick={() => play('quiz-rush')}>
                <Icon name="trophy" size={13} /> Play
              </button>
            </div>
          </div>
        </section>

        <section class="card game">
          <div class="body">
            <div class="row">
              <h2 class="name">Boss ladder</h2>
              <span class="chip accent">Quest</span>
            </div>
            <p class="muted desc">
              Ten tiers of turn-based quiz bosses, plus one boss per unsubmitted deadline. Your
              pet's ATK, DEF, HP and SPD come from real studying — and so does the daily streak.
            </p>
            <div class="scores">
              <span class="stat"><span class="k">Tiers</span><span class="v">{$tierBeaten}/10</span></span>
              <span class="stat"><span class="k">Streak</span><span class="v">{$streak}</span></span>
            </div>
            <p class="faint keys"><kbd>1</kbd>–<kbd>4</kbd> answer · <kbd>Esc</kbd> flee</p>
            <div class="actions">
              <button class="btn sm primary" onclick={() => navigate('quest')}>
                <Icon name="sword" size={13} /> Open Quest
              </button>
            </div>
          </div>
        </section>
      </div>

      <p class="faint egg">Tip: poke the pet five times in a row to get back here.</p>
    </div>
  </div>
{/if}

<style>
  .head {
    margin-bottom: 18px;
  }

  .sub {
    margin-top: 3px;
    font-size: 13px;
  }

  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
    gap: 14px;
  }

  .game {
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .preview {
    display: block;
    margin: 0 auto;
    border-bottom: 1px solid var(--border);
    background: var(--bg-elevated);
  }

  .body {
    padding: 13px 14px 14px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    flex: 1;
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  .name {
    font-size: 14.5px;
    font-weight: 620;
    letter-spacing: -0.01em;
  }

  .desc {
    font-size: 12.5px;
    line-height: 1.5;
  }

  .scores {
    display: flex;
    gap: 16px;
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
    font-size: 15px;
    font-weight: 620;
  }

  .keys {
    font-size: 11.5px;
    margin-top: auto;
  }

  kbd {
    padding: 1px 5px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg-subtle);
    font-family: var(--font);
    font-size: 10.5px;
  }

  .actions {
    display: flex;
    gap: 8px;
  }

  .egg {
    margin-top: 16px;
    font-size: 11.5px;
    text-align: center;
  }
</style>
