<script lang="ts">
  /**
   * Free-roaming pet layer, mounted once from App.svelte above every view.
   *
   * The layer itself is `pointer-events: none` and sits below the chat panel,
   * command palette and toasts, so it never blocks the UI underneath — only the
   * pet button itself takes pointer events.
   *
   * Two modes share it:
   *   drag   — the pet stays where you put it (position persisted as fractions)
   *   wander — a small rAF state machine walks it along the bottom of the
   *            content area, with mood-specific detours
   *
   * `prefers-reduced-motion` downgrades wander to the (visually static) drag
   * behaviour. All XP / mood / quip logic still lives in ../pet.ts.
   */
  import { tick, untrack } from 'svelte';
  import { chatCollapsed, chatW, resolvedChatMode } from '../layout';
  import { chatOpen } from '../stores';
  import {
    bubble,
    level,
    mood,
    petEnabled,
    petMode,
    petName,
    petPos,
    pokePet,
    xp,
  } from '../pet';
  import PetSprite from './PetSprite.svelte';

  const SIZE = 72;
  const MARGIN = 10;
  const STEP = 1 / 60;
  const WALK = 60; // px/s
  const BW = 190; // bubble width
  const DRAG_THRESHOLD = 4;

  // ------------------------------------------------------------ render state

  let px = $state(0);
  let py = $state(0);
  let facing = $state<1 | -1>(1);
  let walking = $state(false);
  let sitting = $state(false);
  let dragging = $state(false);
  let squashing = $state(false);
  let reduced = $state(false);
  let vw = $state(typeof window === 'undefined' ? 1280 : window.innerWidth);
  let vh = $state(typeof window === 'undefined' ? 800 : window.innerHeight);

  const effMode = $derived(reduced && $petMode === 'wander' ? 'drag' : $petMode);
  const shown = $derived($petEnabled && $petMode !== 'dock');
  const label = $derived(`${$petName}, feeling ${$mood}. Level ${$level}, ${$xp} xp.`);

  // ------------------------------------------------------------------ layout

  const clamp = (v: number, lo: number, hi: number) => (hi < lo ? lo : Math.min(hi, Math.max(lo, v)));

  function box(sel: string): DOMRect | null {
    const el = document.querySelector(sel);
    return el ? el.getBoundingClientRect() : null;
  }

  /**
   * Right edge of the roaming area. A docked chat is a real column, so the pet
   * would otherwise walk underneath it (the layer is below the chat and does
   * not clip) — measure the column and stop at its left edge instead. The
   * floating chat is deliberately not measured: it is transient, and the pet
   * strolling behind it is the same as it strolling behind the palette.
   */
  const rightEdge = () => {
    const c = box('aside.chat.docked');
    return c && c.width > 0 ? c.left : window.innerWidth;
  };

  const maxX = () => Math.max(MARGIN, rightEdge() - SIZE - MARGIN);
  const maxY = () => Math.max(MARGIN, window.innerHeight - SIZE - MARGIN);
  const floorY = () => maxY();
  /** Left edge of the content area (just right of the sidebar). */
  const leftEdge = () => {
    const s = box('aside.sidebar');
    return clamp((s ? s.right : 224) + MARGIN, MARGIN, maxX());
  };

  function applyFractions(p: { fx: number; fy: number }) {
    px = clamp(MARGIN + p.fx * (maxX() - MARGIN), MARGIN, maxX());
    py = clamp(MARGIN + p.fy * (maxY() - MARGIN), MARGIN, maxY());
  }

  function savePos() {
    const rx = Math.max(1, maxX() - MARGIN);
    const ry = Math.max(1, maxY() - MARGIN);
    petPos.set({ fx: clamp((px - MARGIN) / rx, 0, 1), fy: clamp((py - MARGIN) / ry, 0, 1) });
  }

  // Position follows the stored fractions whenever we are not simulating.
  $effect(() => {
    const p = $petPos;
    if (!shown) return;
    if (effMode !== 'wander') applyFractions(p);
  });

  function onResize() {
    vw = window.innerWidth;
    vh = window.innerHeight;
    if (effMode === 'wander') {
      px = clamp(px, MARGIN, maxX());
      py = clamp(py, MARGIN, maxY());
    } else {
      applyFractions($petPos);
    }
  }

  /*
   * Opening, resizing or collapsing the docked chat changes the roaming area
   * exactly as a window resize does, so re-run the same clamp. `tick()` rather
   * than rAF: the column has to be in the DOM before it can be measured, and a
   * background tab never gets a frame.
   */
  $effect(() => {
    void $chatOpen;
    void $chatW;
    void $resolvedChatMode;
    void $chatCollapsed;
    if (!shown) return;
    let live = true;
    void tick().then(() => {
      if (live) onResize();
    });
    return () => {
      live = false;
    };
  });

  $effect(() => {
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)');
    reduced = mq.matches;
    const h = () => (reduced = mq.matches);
    mq.addEventListener('change', h);
    return () => mq.removeEventListener('change', h);
  });

  // ------------------------------------------------------------------ poking

  let squashTimer: ReturnType<typeof setTimeout> | undefined;
  let paused = false;

  $effect(() => () => clearTimeout(squashTimer));

  // Resume wandering once the speech bubble has gone.
  $effect(() => {
    if (!$bubble) paused = false;
  });

  function poke() {
    squashing = false;
    clearTimeout(squashTimer);
    requestAnimationFrame(() => {
      squashing = true;
      squashTimer = setTimeout(() => (squashing = false), 320);
    });
    paused = true;
    walking = false;
    pokePet();
  }

  // ----------------------------------------------------------------- dragging

  let pressed = false;
  let moved = false;
  let sx = 0;
  let sy = 0;
  let gx = 0;
  let gy = 0;

  function onPointerDown(e: PointerEvent) {
    if (e.button !== 0) return;
    pressed = true;
    moved = false;
    sx = e.clientX;
    sy = e.clientY;
    gx = e.clientX - px;
    gy = e.clientY - py;
    try {
      (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    } catch {
      /* pointer already gone */
    }
  }

  function onPointerMove(e: PointerEvent) {
    if (!pressed || effMode !== 'drag') return;
    if (!moved && Math.hypot(e.clientX - sx, e.clientY - sy) < DRAG_THRESHOLD) return;
    moved = true;
    dragging = true;
    px = clamp(e.clientX - gx, MARGIN, maxX());
    py = clamp(e.clientY - gy, MARGIN, maxY());
  }

  function onPointerUp(e: PointerEvent) {
    if (!pressed) return;
    pressed = false;
    try {
      (e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
    } catch {
      /* not captured */
    }
    if (moved) {
      dragging = false;
      savePos();
    } else {
      poke();
    }
    moved = false;
  }

  // ------------------------------------------------------------- wander brain

  type WState = 'walk' | 'idle' | 'sit' | 'hop';

  let ws: WState = 'idle';
  let left = 0; // seconds left in the current state
  let dir: 1 | -1 = 1;
  let onDock = false;
  let hopT = 0;
  let hopDur = 0.55;
  let hx0 = 0;
  let hy0 = 0;
  let hx1 = 0;
  let hy1 = 0;
  let circleA = 0;
  let wasSpecial = false;

  const rand = (lo: number, hi: number) => lo + Math.random() * (hi - lo);

  function toward(v: number, target: number, sp: number, dt: number): number {
    const d = target - v;
    const m = sp * dt;
    return Math.abs(d) <= m ? target : v + Math.sign(d) * m;
  }

  function dockSpot() {
    const side = box('aside.sidebar');
    const pill = box('aside.sidebar .pill');
    return {
      x: clamp(side ? side.left + side.width / 2 - SIZE / 2 : MARGIN, MARGIN, maxX()),
      y: clamp(pill ? pill.top - SIZE - 4 : floorY(), MARGIN, maxY()),
    };
  }

  function startHop(toDock: boolean) {
    hx0 = px;
    hy0 = py;
    hopT = 0;
    hopDur = 0.55;
    if (toDock) {
      const d = dockSpot();
      hx1 = d.x;
      hy1 = d.y;
    } else {
      hx1 = clamp(rand(leftEdge(), leftEdge() + 200), leftEdge(), maxX());
      hy1 = floorY();
    }
    ws = 'hop';
  }

  function pick() {
    if (onDock) {
      if (ws === 'sit') {
        startHop(false);
        return;
      }
      ws = 'sit';
      left = rand(5, 10);
      return;
    }
    const r = Math.random();
    if (r < 0.1) {
      startHop(true);
      return;
    }
    if (r < 0.5) {
      ws = 'walk';
      left = rand(1.5, 4);
      dir = Math.random() < 0.5 ? -1 : 1;
    } else if (r < 0.8) {
      ws = 'idle';
      left = rand(2, 6);
    } else {
      ws = 'sit';
      left = rand(5, 10);
    }
  }

  /** Paces quickly beside the Deadlines nav item. */
  function panicStep(dt: number) {
    const nav = box('[data-nav="deadlines"]');
    const side = box('aside.sidebar');
    const anchor = side ? side.right : 224;
    const lo = clamp(anchor - 46, MARGIN, maxX());
    const hi = clamp(anchor + 120, MARGIN, maxX());
    const ty = nav ? clamp(nav.top + nav.height / 2 - SIZE / 2, MARGIN, maxY()) : floorY();
    py = toward(py, ty, 260, dt);
    px += dir * 130 * dt;
    if (px <= lo) {
      px = lo;
      dir = 1;
    } else if (px >= hi) {
      px = hi;
      dir = -1;
    }
    facing = dir;
    walking = true;
    sitting = false;
  }

  /** Runs little circles next to the sync pill. */
  function busyStep(dt: number) {
    const pill = box('aside.sidebar .pill');
    const cx = clamp((pill ? pill.right + 34 : leftEdge() + 40), MARGIN, maxX());
    const cy = clamp((pill ? pill.top + pill.height / 2 - SIZE / 2 : floorY()), MARGIN, maxY());
    circleA += dt * 2.8;
    px = clamp(cx + Math.cos(circleA) * 30, MARGIN, maxX());
    py = clamp(cy + Math.sin(circleA) * 18, MARGIN, maxY());
    facing = Math.sin(circleA) < 0 ? 1 : -1;
    walking = true;
    sitting = false;
  }

  /** Shuffles into the bottom-right corner and sleeps there. */
  function sleepyStep(dt: number) {
    const tx = maxX();
    const ty = floorY();
    const before = px;
    px = toward(px, tx, 45, dt);
    py = toward(py, ty, 120, dt);
    const arrived = px === tx && py === ty;
    facing = px >= before ? 1 : -1;
    walking = !arrived;
    sitting = arrived;
  }

  function normalStep(dt: number) {
    const lo = leftEdge();
    const hi = maxX();
    left -= dt;
    if (ws === 'hop') {
      hopT += dt;
      const k = Math.min(1, hopT / hopDur);
      px = hx0 + (hx1 - hx0) * k;
      py = hy0 + (hy1 - hy0) * k - Math.sin(k * Math.PI) * 60;
      facing = hx1 >= hx0 ? 1 : -1;
      walking = false;
      sitting = false;
      if (k >= 1) {
        onDock = !onDock;
        ws = 'idle';
        left = 0;
        pick();
      }
      return;
    }
    if (onDock) {
      walking = false;
      sitting = ws === 'sit';
      if (left <= 0) pick();
      return;
    }
    py = toward(py, floorY(), 240, dt);
    if (ws === 'walk') {
      px += dir * WALK * dt;
      if (px <= lo) {
        px = lo;
        dir = 1;
      } else if (px >= hi) {
        px = hi;
        dir = -1;
      }
      facing = dir;
      walking = true;
      sitting = false;
    } else {
      walking = false;
      sitting = ws === 'sit';
    }
    if (left <= 0) pick();
  }

  function step(dt: number) {
    if (paused) {
      walking = false;
      return;
    }
    const m = $mood;
    const special = m === 'panic' || m === 'busy' || m === 'sleepy';
    if (special) {
      wasSpecial = true;
      onDock = false;
      if (m === 'panic') panicStep(dt);
      else if (m === 'busy') busyStep(dt);
      else sleepyStep(dt);
      return;
    }
    if (wasSpecial) {
      wasSpecial = false;
      ws = 'walk';
      left = rand(1.5, 4);
      sitting = false;
    }
    normalStep(dt);
  }

  // rAF with a fixed timestep, paused while the tab is hidden.
  $effect(() => {
    if (!shown || effMode !== 'wander') {
      walking = false;
      sitting = false;
      return;
    }

    untrack(() => {
      px = clamp(px, leftEdge(), maxX());
      py = floorY();
      ws = 'walk';
      left = rand(1.5, 4);
      onDock = false;
    });

    let raf = 0;
    let last = 0;
    let acc = 0;

    const frame = (t: number) => {
      raf = requestAnimationFrame(frame);
      if (!last) last = t;
      let dt = (t - last) / 1000;
      last = t;
      if (dt > 0.25) dt = 0.25;
      acc += dt;
      let guard = 0;
      while (acc >= STEP && guard++ < 6) {
        acc -= STEP;
        step(STEP);
      }
    };

    const onVis = () => {
      if (document.hidden) {
        cancelAnimationFrame(raf);
        raf = 0;
      } else if (!raf) {
        last = 0;
        acc = 0;
        raf = requestAnimationFrame(frame);
      }
    };

    if (!document.hidden) raf = requestAnimationFrame(frame);
    document.addEventListener('visibilitychange', onVis);

    return () => {
      cancelAnimationFrame(raf);
      document.removeEventListener('visibilitychange', onVis);
    };
  });

  // ------------------------------------------------------------------ bubble

  const bub = $derived.by(() => {
    const cx = px + SIZE / 2;
    let bl = cx - BW / 2;
    if (bl < MARGIN) bl = MARGIN;
    else if (bl + BW > vw - MARGIN) bl = Math.max(MARGIN, vw - MARGIN - BW);
    const below = py < 92;
    return { left: bl, below, arrow: clamp(cx - bl, 14, BW - 14) };
  });
</script>

<svelte:window onresize={onResize} />

{#if shown}
  <div class="pet-layer">
    {#if $bubble}
      <div
        class="pet-bubble"
        class:below={bub.below}
        role="status"
        style="left:{bub.left}px;width:{BW}px;--arrow:{bub.arrow}px;{bub.below
          ? `top:${py + SIZE + 6}px`
          : `bottom:${Math.max(MARGIN, vh - py + 6)}px`}"
      >
        {$bubble}
      </div>
    {:else}
      <div class="bubble-sr" role="status"></div>
    {/if}

    <button
      class="pet-hit"
      class:dragging
      class:grabbable={effMode === 'drag'}
      style="transform:translate3d({Math.round(px)}px,{Math.round(py)}px,0)"
      onpointerdown={onPointerDown}
      onpointermove={onPointerMove}
      onpointerup={onPointerUp}
      onpointercancel={() => (pressed = false)}
      aria-label={label}
      title="{$petName} — {$mood}{effMode === 'drag' ? ' (drag me)' : ''}"
      type="button"
    >
      <PetSprite mood={$mood} level={$level} {facing} size={SIZE} {walking} {sitting} squash={squashing} />
    </button>
  </div>
{/if}

<style>
  .pet-layer {
    position: fixed;
    inset: 0;
    z-index: 30; /* below chat panel (45), palette (150) and toasts (200) */
    pointer-events: none;
    overflow: hidden;
  }

  .pet-hit {
    position: absolute;
    top: 0;
    left: 0;
    width: 72px;
    height: 72px;
    padding: 0;
    border-radius: 50%;
    background: none;
    line-height: 0;
    pointer-events: auto;
    touch-action: none;
    will-change: transform;
  }

  .pet-hit.grabbable {
    cursor: grab;
  }

  .pet-hit.dragging {
    cursor: grabbing;
  }

  .pet-bubble {
    position: absolute;
    padding: 6px 9px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-elevated);
    box-shadow: var(--shadow-pop);
    color: var(--text);
    font-size: 11.5px;
    line-height: 1.35;
    text-align: center;
    pointer-events: none;
    animation: bubble-in 140ms ease-out;
  }

  .pet-bubble::after {
    content: '';
    position: absolute;
    left: var(--arrow, 50%);
    bottom: -5px;
    width: 8px;
    height: 8px;
    margin-left: -4px;
    transform: rotate(45deg);
    background: var(--bg-elevated);
    border-right: 1px solid var(--border);
    border-bottom: 1px solid var(--border);
  }

  .pet-bubble.below::after {
    bottom: auto;
    top: -5px;
    border: none;
    border-left: 1px solid var(--border);
    border-top: 1px solid var(--border);
  }

  .bubble-sr {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
  }

  @keyframes bubble-in {
    from {
      opacity: 0;
      transform: translateY(3px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .pet-bubble {
      animation: none;
    }
  }
</style>
