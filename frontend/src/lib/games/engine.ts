/**
 * Tiny shared game engine for the Arcade.
 *
 * Deliberately dependency-free and small: a fixed-timestep rAF loop that
 * auto-pauses when the window is hidden or blurred, window-level keyboard
 * tracking that is only attached while a game is actually running, DPR-aware
 * canvas sizing, a seeded PRNG so the "daily run" is identical all day, and a
 * couple of colour helpers that read the app's CSS tokens so games inherit
 * light/dark for free.
 */

// ------------------------------------------------------------------ debug

/**
 * Live loop bookkeeping, mirrored onto `window.__arcade` so a stray rAF loop
 * is observable from the console (`__arcade.loops` must be 0 after leaving the
 * Arcade view).
 */
export const engineStats = { loops: 0, frames: 0 };

if (typeof window !== 'undefined') {
  (window as unknown as Record<string, unknown>).__arcade = engineStats;
}

// ------------------------------------------------------------------- motion

/** True when the user asked for reduced motion. */
export function prefersReducedMotion(): boolean {
  if (typeof window === 'undefined' || !window.matchMedia) return false;
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

// --------------------------------------------------------------------- rng

/** mulberry32 — 32 bits of state, good enough for hurdle order. */
export function makeRNG(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

/** FNV-1a, for turning a seed string into a number. */
export function hashSeed(s: string): number {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

/** Today as YYYY-MM-DD, in local time — the daily-run seed. */
export function todayKey(d: Date = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

// ------------------------------------------------------------------ canvas

/**
 * Size `canvas` to `w`x`h` CSS pixels at the device pixel ratio and return a
 * context whose units are CSS pixels. Cheap enough to call every frame.
 */
export function fitCanvas(canvas: HTMLCanvasElement, w: number, h: number): CanvasRenderingContext2D {
  const dpr = Math.min(3, (typeof window !== 'undefined' && window.devicePixelRatio) || 1);
  const bw = Math.max(1, Math.round(w * dpr));
  const bh = Math.max(1, Math.round(h * dpr));
  if (canvas.width !== bw || canvas.height !== bh) {
    canvas.width = bw;
    canvas.height = bh;
  }
  canvas.style.width = `${w}px`;
  canvas.style.height = `${h}px`;
  const ctx = canvas.getContext('2d') as CanvasRenderingContext2D;
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  return ctx;
}

/** Rounded rect path (Path2D.roundRect is not everywhere yet). */
export function roundRect(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  w: number,
  h: number,
  r: number,
): void {
  const rr = Math.max(0, Math.min(r, Math.min(w, h) / 2));
  ctx.beginPath();
  ctx.moveTo(x + rr, y);
  ctx.lineTo(x + w - rr, y);
  ctx.quadraticCurveTo(x + w, y, x + w, y + rr);
  ctx.lineTo(x + w, y + h - rr);
  ctx.quadraticCurveTo(x + w, y + h, x + w - rr, y + h);
  ctx.lineTo(x + rr, y + h);
  ctx.quadraticCurveTo(x, y + h, x, y + h - rr);
  ctx.lineTo(x, y + rr);
  ctx.quadraticCurveTo(x, y, x + rr, y);
  ctx.closePath();
}

// ------------------------------------------------------------------ colours

export type RGB = [number, number, number];

/** Parse `#rgb`, `#rrggbb` or `rgb()/rgba()` into RGB. Falls back to grey. */
export function parseColor(input: string): RGB {
  const s = (input || '').trim();
  if (s.startsWith('#')) {
    const hex = s.slice(1);
    if (hex.length === 3) {
      return [
        parseInt(hex[0] + hex[0], 16),
        parseInt(hex[1] + hex[1], 16),
        parseInt(hex[2] + hex[2], 16),
      ];
    }
    if (hex.length >= 6) {
      return [
        parseInt(hex.slice(0, 2), 16),
        parseInt(hex.slice(2, 4), 16),
        parseInt(hex.slice(4, 6), 16),
      ];
    }
  }
  const m = s.match(/rgba?\(([^)]+)\)/);
  if (m) {
    const parts = m[1].split(/[,\s/]+/).filter(Boolean).map(Number);
    if (parts.length >= 3) return [parts[0], parts[1], parts[2]];
  }
  return [128, 128, 128];
}

export function rgbCSS([r, g, b]: RGB, alpha = 1): string {
  const c = (n: number) => Math.max(0, Math.min(255, Math.round(n)));
  return alpha >= 1 ? `rgb(${c(r)} ${c(g)} ${c(b)})` : `rgb(${c(r)} ${c(g)} ${c(b)} / ${alpha})`;
}

/** Linear blend, `t` = 0 gives `a`, 1 gives `b`. */
export function mix(a: RGB, b: RGB, t: number): RGB {
  const k = Math.max(0, Math.min(1, t));
  return [a[0] + (b[0] - a[0]) * k, a[1] + (b[1] - a[1]) * k, a[2] + (b[2] - a[2]) * k];
}

/** Relative luminance, 0..1 — used to pick readable text over a tint. */
export function luminance([r, g, b]: RGB): number {
  const f = (v: number) => {
    const x = v / 255;
    return x <= 0.03928 ? x / 12.92 : Math.pow((x + 0.055) / 1.055, 2.4);
  };
  return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b);
}

/** The design tokens a game needs, resolved to concrete colours. */
export interface Palette {
  bg: RGB;
  elevated: RGB;
  subtle: RGB;
  border: RGB;
  text: RGB;
  muted: RGB;
  faint: RGB;
  accent: RGB;
  green: RGB;
  amber: RGB;
  red: RGB;
  dark: boolean;
  font: string;
}

const TOKENS: Array<[keyof Palette, string]> = [
  ['bg', '--bg'],
  ['elevated', '--bg-elevated'],
  ['subtle', '--bg-subtle'],
  ['border', '--border-strong'],
  ['text', '--text'],
  ['muted', '--text-muted'],
  ['faint', '--text-faint'],
  ['accent', '--accent'],
  ['green', '--green'],
  ['amber', '--amber'],
  ['red', '--red'],
];

/** Read the current theme's tokens off `:root`. Call again when theme flips. */
export function readPalette(): Palette {
  const fallback: Palette = {
    bg: [251, 251, 252],
    elevated: [255, 255, 255],
    subtle: [241, 241, 244],
    border: [211, 211, 218],
    text: [23, 23, 28],
    muted: [107, 107, 120],
    faint: [150, 150, 159],
    accent: [91, 91, 214],
    green: [14, 159, 110],
    amber: [194, 116, 10],
    red: [220, 38, 38],
    dark: false,
    font: 'Inter, ui-sans-serif, system-ui, sans-serif',
  };
  if (typeof document === 'undefined') return fallback;
  const cs = getComputedStyle(document.documentElement);
  const out = { ...fallback };
  for (const [key, token] of TOKENS) {
    const v = cs.getPropertyValue(token);
    if (v) (out[key] as RGB) = parseColor(v);
  }
  out.dark = document.documentElement.getAttribute('data-theme') === 'dark';
  const font = cs.getPropertyValue('--font').trim();
  if (font) out.font = font;
  return out;
}

// -------------------------------------------------------------------- keys

/**
 * Normalise a key event to a `KeyboardEvent.code`-style name. Synthetic events
 * (and a few odd input stacks) leave `code` empty, so fall back to `key`.
 */
export function keyCode(e: KeyboardEvent): string {
  if (e.code) return e.code;
  const k = e.key;
  if (k === ' ' || k === 'Spacebar') return 'Space';
  if (/^[a-zA-Z]$/.test(k)) return `Key${k.toUpperCase()}`;
  return k ?? '';
}

/**
 * Window-level key tracking. Only attached between `attach()` and `detach()`,
 * so a game never sees keystrokes once its view is gone; games also skip it
 * while the document is hidden.
 */
export class Keys {
  readonly down = new Set<string>();
  private hits = new Set<string>();
  private attached = false;

  /** `event.code` values whose default (scrolling, mostly) we swallow. */
  constructor(private readonly prevent: readonly string[] = []) {}

  attach(): void {
    if (this.attached || typeof window === 'undefined') return;
    this.attached = true;
    window.addEventListener('keydown', this.onDown);
    window.addEventListener('keyup', this.onUp);
    window.addEventListener('blur', this.clear);
  }

  detach(): void {
    if (!this.attached) return;
    this.attached = false;
    window.removeEventListener('keydown', this.onDown);
    window.removeEventListener('keyup', this.onUp);
    window.removeEventListener('blur', this.clear);
    this.down.clear();
    this.hits.clear();
  }

  /** True while held. */
  held(...codes: string[]): boolean {
    return codes.some((c) => this.down.has(c));
  }

  /** True once per physical press; consumes the press. */
  hit(...codes: string[]): boolean {
    for (const c of codes) {
      if (this.hits.has(c)) {
        this.hits.delete(c);
        return true;
      }
    }
    return false;
  }

  /** Drop any presses the game did not consume this frame. */
  flush(): void {
    this.hits.clear();
  }

  /** Synthesise a press (used for click/tap-to-jump). */
  press(code: string): void {
    this.hits.add(code);
  }

  private clear = () => {
    this.down.clear();
    this.hits.clear();
  };

  private onDown = (e: KeyboardEvent) => {
    const t = e.target as HTMLElement | null;
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    const code = keyCode(e);
    if (this.prevent.includes(code)) e.preventDefault();
    if (!e.repeat) this.hits.add(code);
    this.down.add(code);
  };

  private onUp = (e: KeyboardEvent) => {
    this.down.delete(keyCode(e));
  };
}

// -------------------------------------------------------------------- loop

export interface LoopOpts {
  /** Advance simulation by a fixed `dt` (seconds). */
  step: (dt: number) => void;
  /** Draw; `alpha` is the 0..1 leftover of the accumulator. */
  render: (alpha: number) => void;
  /** Simulation rate, default 60 Hz. */
  hz?: number;
  /** Called when auto-pause kicks in (tab hidden / window blurred). */
  onAutoPause?: () => void;
}

/**
 * Fixed-timestep rAF loop. `stop()` cancels the frame and removes every
 * listener, so unmounting a game view leaves nothing behind.
 */
export class Loop {
  private raf = 0;
  private last = 0;
  private acc = 0;
  private readonly hz: number;
  running = false;

  constructor(private readonly opts: LoopOpts) {
    this.hz = opts.hz ?? 60;
  }

  start(): void {
    if (this.running || typeof window === 'undefined') return;
    this.running = true;
    engineStats.loops++;
    this.last = performance.now();
    this.acc = 0;
    document.addEventListener('visibilitychange', this.onHide);
    window.addEventListener('blur', this.onHide);
    this.raf = requestAnimationFrame(this.frame);
  }

  stop(): void {
    if (!this.running) return;
    this.running = false;
    engineStats.loops = Math.max(0, engineStats.loops - 1);
    cancelAnimationFrame(this.raf);
    this.raf = 0;
    document.removeEventListener('visibilitychange', this.onHide);
    window.removeEventListener('blur', this.onHide);
  }

  // Fires for both events; `visibilitychange` also fires on *becoming*
  // visible, so only pause when we really lost the tab or the focus.
  private onHide = () => {
    if (document.visibilityState === 'hidden' || !document.hasFocus()) this.opts.onAutoPause?.();
  };

  private frame = (t: number) => {
    if (!this.running) return;
    this.raf = requestAnimationFrame(this.frame);
    engineStats.frames++;
    // A long tab-switch must not be simulated all at once.
    const dt = Math.min(0.25, Math.max(0, (t - this.last) / 1000));
    this.last = t;
    const fixed = 1 / this.hz;
    this.acc += dt;
    let n = 0;
    while (this.acc >= fixed && n < 6) {
      this.opts.step(fixed);
      this.acc -= fixed;
      n++;
    }
    if (n === 6) this.acc = 0;
    this.opts.render(this.acc / fixed);
  };
}
