/**
 * Nibble Run — endless runner where the hurdles are your own unsubmitted
 * deadlines. Pure simulation + canvas rendering; the Svelte component owns the
 * loop, the keyboard and the HUD.
 */

import type { Deadline } from '../types';
import { makeRNG, readPalette, rgbCSS, roundRect, type Palette } from './engine';
import { drawCheck, drawFileGlyph, drawPet } from './sprites';

export type RunState = 'ready' | 'running' | 'dead';

export interface HurdleSpec {
  code: string;
  title: string;
  /** taller = more urgent; `overdue` is wide and red. */
  size: 'small' | 'medium' | 'tall' | 'overdue';
}

const DAY = 86_400_000;

/** Fallback hurdles for an account with nothing due. */
const GENERIC: HurdleSpec[] = [
  { code: 'GEN', title: 'Tutorial', size: 'small' },
  { code: 'GEN', title: 'Quiz', size: 'medium' },
  { code: 'GEN', title: 'Reading', size: 'small' },
  { code: 'GEN', title: 'Lab', size: 'tall' },
];

/** Turn real deadlines into hurdle specs (unsubmitted only, soonest first). */
export function hurdlesFromDeadlines(list: Deadline[], now = Date.now()): HurdleSpec[] {
  const out: HurdleSpec[] = [];
  for (const d of list) {
    if (d.Submitted) continue;
    const t = Date.parse(d.DueAt);
    if (!isFinite(t)) continue;
    const dt = t - now;
    const size: HurdleSpec['size'] =
      dt < 0 ? 'overdue' : dt <= DAY ? 'tall' : dt <= 3 * DAY ? 'medium' : 'small';
    out.push({ code: d.CourseCode || '', title: d.Title || 'Deadline', size });
  }
  out.sort((a, b) => a.title.localeCompare(b.title));
  return out.length > 0 ? out : GENERIC;
}

const SIZES: Record<HurdleSpec['size'], { w: number; h: number }> = {
  small: { w: 44, h: 34 },
  medium: { w: 48, h: 52 },
  tall: { w: 50, h: 70 },
  overdue: { w: 96, h: 50 },
};

interface Obstacle {
  x: number;
  w: number;
  h: number;
  gate: boolean;
  /** gates hang from `top` down to `top + h`; hurdles sit on the ground */
  top: number;
  spec: HurdleSpec | null;
}

interface Pickup {
  x: number;
  y: number;
  taken: boolean;
}

interface Dot {
  x: number;
  y: number;
  r: number;
  depth: number;
}

export interface RunCallbacks {
  /** A file glyph was collected (score already updated). */
  onCollect: () => void;
  /** The run ended; `by` is the thing that got you. */
  onDeath: (score: number, by: string) => void;
}

const GRAVITY = 2100;
const JUMP_V = -700;
const DOUBLE_V = -620;
const BASE_SPEED = 290;
const MAX_SPEED = 720;
const PET_H = 46;

export class NibbleRun {
  // simulation
  state: RunState = 'ready';
  score = 0;
  files = 0;
  speed = BASE_SPEED;
  dist = 0;
  deathBy = '';

  private w = 960;
  private h = 540;
  private ground = 470;
  private y = 0; // pet feet offset above the ground (0 = grounded)
  private vy = 0;
  private jumps = 0;
  private ducking = false;
  private obstacles: Obstacle[] = [];
  private pickups: Pickup[] = [];
  private dots: Dot[] = [];
  private nextSpawn = 0;
  private specIndex = 0;
  private order: HurdleSpec[] = [];
  private rng: () => number = Math.random;
  private deathT = 0;
  private blinkT = 0;
  private palette: Palette = readPalette();

  constructor(
    private readonly specs: HurdleSpec[],
    private readonly seed: number,
    private readonly petLevel: number,
    private readonly reduced: boolean,
    private readonly cb: RunCallbacks,
  ) {
    this.reset();
  }

  /** Re-read CSS tokens (call on theme change). */
  refreshPalette(): void {
    this.palette = readPalette();
  }

  resize(w: number, h: number): void {
    this.w = w;
    this.h = h;
    this.ground = Math.round(h * 0.82);
    this.seedDots();
  }

  get doubleJump(): boolean {
    return this.petLevel >= 4;
  }

  reset(): void {
    this.rng = makeRNG(this.seed);
    this.state = 'ready';
    this.score = 0;
    this.files = 0;
    this.speed = BASE_SPEED;
    this.dist = 0;
    this.y = 0;
    this.vy = 0;
    this.jumps = 0;
    this.ducking = false;
    this.deathT = 0;
    this.deathBy = '';
    this.obstacles = [];
    this.pickups = [];
    this.nextSpawn = this.w + 160;
    this.specIndex = 0;
    // Seeded shuffle so a daily run always presents the same order.
    this.order = [...this.specs];
    for (let i = this.order.length - 1; i > 0; i--) {
      const j = Math.floor(this.rng() * (i + 1));
      [this.order[i], this.order[j]] = [this.order[j], this.order[i]];
    }
    this.seedDots();
  }

  private seedDots(): void {
    const rng = makeRNG(this.seed ^ 0x9e3779b9);
    const n = Math.round((this.w * this.h) / 14000);
    this.dots = [];
    for (let i = 0; i < n; i++) {
      const depth = 0.15 + rng() * 0.55;
      this.dots.push({
        x: rng() * this.w,
        y: 24 + rng() * (this.ground - 60),
        r: 1 + depth * 2,
        depth,
      });
    }
  }

  // ------------------------------------------------------------- input

  jump(): void {
    if (this.state === 'ready') {
      this.state = 'running';
      return;
    }
    if (this.state !== 'running') return;
    const max = this.doubleJump ? 2 : 1;
    if (this.jumps >= max) return;
    this.vy = this.jumps === 0 ? JUMP_V : DOUBLE_V;
    this.jumps++;
    this.ducking = false;
  }

  setDuck(on: boolean): void {
    if (this.state !== 'running') return;
    this.ducking = on;
    // fast-fall out of a jump
    if (on && this.y > 0 && this.vy < 300) this.vy += 260;
  }

  // -------------------------------------------------------------- step

  step(dt: number): void {
    this.blinkT += dt;
    if (this.state === 'dead') {
      this.deathT += dt;
      return;
    }
    if (this.state !== 'running') {
      // idle bob on the start screen
      return;
    }

    this.speed = Math.min(MAX_SPEED, BASE_SPEED + this.dist * 0.011);
    const dx = this.speed * dt;
    this.dist += dx;

    // pet physics — `y` is height above the ground, `vy` positive means falling
    this.vy += GRAVITY * dt;
    this.y -= this.vy * dt;
    if (this.y <= 0) {
      this.y = 0;
      this.vy = 0;
      this.jumps = 0;
    }

    // parallax
    for (const d of this.dots) {
      d.x -= dx * d.depth * 0.55;
      if (d.x < -4) {
        d.x += this.w + 8;
        d.y = 24 + this.rng() * (this.ground - 60);
      }
    }

    // obstacles
    for (const o of this.obstacles) o.x -= dx;
    for (const p of this.pickups) p.x -= dx;
    this.obstacles = this.obstacles.filter((o) => o.x + o.w > -20);
    this.pickups = this.pickups.filter((p) => p.x > -20 && !p.taken);

    this.nextSpawn -= dx;
    if (this.nextSpawn <= 0) this.spawn();

    // collisions
    const box = this.petBox();
    for (const o of this.obstacles) {
      const oy = o.gate ? o.top : this.ground - o.h;
      if (
        box.x < o.x + o.w &&
        box.x + box.w > o.x &&
        box.y < oy + o.h &&
        box.y + box.h > oy
      ) {
        this.die(o);
        return;
      }
    }
    for (const p of this.pickups) {
      if (p.taken) continue;
      if (Math.abs(p.x - (box.x + box.w / 2)) < 18 && Math.abs(p.y - (box.y + box.h / 2)) < 30) {
        p.taken = true;
        this.files++;
        this.cb.onCollect();
      }
    }

    this.score = Math.floor(this.dist / 30) + this.files;
  }

  private petBox() {
    const h = this.ducking && this.y === 0 ? PET_H * 0.58 : PET_H * 0.92;
    const w = this.ducking && this.y === 0 ? PET_H * 0.98 : PET_H * 0.72;
    return { x: this.petX() - w / 2, y: this.ground - this.y - h, w, h };
  }

  private petX(): number {
    return Math.max(84, this.w * 0.14);
  }

  private die(o: Obstacle): void {
    this.state = 'dead';
    this.deathT = 0;
    this.deathBy = o.gate ? 'A submitted quiz' : (o.spec?.title ?? 'Something');
    this.cb.onDeath(this.score, this.deathBy);
  }

  private spawn(): void {
    const gate = this.rng() < 0.18 && this.dist > 900;
    if (gate) {
      // Hangs low enough that only a ducking pet fits under it.
      const h = 100;
      this.obstacles.push({
        x: this.w + 30,
        w: 58,
        h,
        gate: true,
        top: this.ground - 130,
        spec: null,
      });
    } else {
      const spec = this.order[this.specIndex % this.order.length];
      this.specIndex++;
      const s = SIZES[spec.size];
      this.obstacles.push({
        x: this.w + 30,
        w: s.w,
        h: s.h,
        gate: false,
        top: 0,
        spec,
      });
    }

    const gap = Math.max(210, this.speed * (0.78 + this.rng() * 0.55));
    this.nextSpawn = gap;

    // a collectible floating in the gap
    if (this.rng() < 0.6) {
      this.pickups.push({
        x: this.w + 30 + gap * 0.55,
        y: this.ground - 52 - this.rng() * 62,
        taken: false,
      });
    }
  }

  // ------------------------------------------------------------ render

  render(ctx: CanvasRenderingContext2D): void {
    const p = this.palette;
    const W = this.w;
    const H = this.h;

    let shakeX = 0;
    let shakeY = 0;
    if (this.state === 'dead' && !this.reduced && this.deathT < 0.45) {
      const k = (1 - this.deathT / 0.45) * 7;
      shakeX = (Math.random() - 0.5) * k;
      shakeY = (Math.random() - 0.5) * k;
    }

    ctx.save();
    ctx.clearRect(0, 0, W, H);
    ctx.fillStyle = rgbCSS(p.elevated);
    ctx.fillRect(0, 0, W, H);
    ctx.translate(shakeX, shakeY);

    // parallax dots
    for (const d of this.dots) {
      ctx.fillStyle = rgbCSS(p.faint, 0.1 + d.depth * 0.28);
      ctx.beginPath();
      ctx.arc(d.x, d.y, d.r, 0, Math.PI * 2);
      ctx.fill();
    }

    // ground line
    ctx.strokeStyle = rgbCSS(p.border);
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.moveTo(0, this.ground + 0.5);
    ctx.lineTo(W, this.ground + 0.5);
    ctx.stroke();

    // ground ticks scroll with the world
    const tick = 64;
    const off = -(this.dist % tick);
    ctx.strokeStyle = rgbCSS(p.faint, 0.35);
    ctx.lineWidth = 1;
    for (let x = off; x < W; x += tick) {
      ctx.beginPath();
      ctx.moveTo(x, this.ground + 6);
      ctx.lineTo(x + 14, this.ground + 6);
      ctx.stroke();
    }

    // pickups
    for (const pk of this.pickups) {
      if (pk.taken) continue;
      drawFileGlyph(ctx, p, pk.x, pk.y, 20);
    }

    // obstacles
    ctx.font = `600 10px ${p.font}`;
    for (const o of this.obstacles) {
      if (o.gate) this.drawGate(ctx, o);
      else this.drawHurdle(ctx, o);
    }

    // pet
    const dead = this.state === 'dead';
    const squash = dead ? Math.min(1, this.deathT * 5) : 0;
    const blink = Math.sin(this.blinkT * 1.7) > 0.985;
    drawPet(ctx, p, this.petX(), this.ground - this.y, PET_H, {
      level: this.petLevel,
      duck: this.ducking && this.y === 0 && !dead,
      squash: squash * 0.7,
      shut: dead || blink,
    });

    ctx.restore();
  }

  private drawHurdle(ctx: CanvasRenderingContext2D, o: Obstacle): void {
    const p = this.palette;
    const overdue = o.spec?.size === 'overdue';
    const y = this.ground - o.h;
    const tint = overdue ? p.red : p.accent;
    ctx.fillStyle = rgbCSS(tint, overdue ? 0.16 : 0.12);
    ctx.strokeStyle = rgbCSS(tint, 0.85);
    ctx.lineWidth = 1.5;
    roundRect(ctx, o.x, y, o.w, o.h, 5);
    ctx.fill();
    ctx.stroke();

    const label = `${o.spec?.code ?? ''} ${short(o.spec?.title ?? '')}`.trim();
    ctx.save();
    ctx.fillStyle = rgbCSS(overdue ? p.red : p.text, 0.9);
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    if (overdue) {
      ctx.font = `600 10px ${p.font}`;
      ctx.fillText(label, o.x + o.w / 2, y + o.h / 2, o.w - 8);
    } else {
      // vertical text reads better on a narrow block
      ctx.translate(o.x + o.w / 2, y + o.h / 2);
      ctx.rotate(-Math.PI / 2);
      ctx.font = `600 9.5px ${p.font}`;
      ctx.fillText(label, 0, 0, o.h - 8);
    }
    ctx.restore();
  }

  private drawGate(ctx: CanvasRenderingContext2D, o: Obstacle): void {
    const p = this.palette;
    ctx.fillStyle = rgbCSS(p.green, 0.14);
    ctx.strokeStyle = rgbCSS(p.green, 0.85);
    ctx.lineWidth = 1.5;
    roundRect(ctx, o.x, o.top, o.w, o.h, 5);
    ctx.fill();
    ctx.stroke();
    drawCheck(ctx, rgbCSS(p.green), o.x + o.w / 2, o.top + o.h - 26, 20);
  }
}

function short(t: string, max = 16): string {
  const s = t.trim();
  return s.length > max ? s.slice(0, max - 1).trimEnd() + '…' : s;
}
