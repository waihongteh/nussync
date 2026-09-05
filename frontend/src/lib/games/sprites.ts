/**
 * Canvas drawings shared by the Arcade: the pet (a flat, simplified version of
 * the sidebar SVG blob-cat, accessories included) and the little file glyph
 * collected in Nibble Run. Flat shapes only — no images, no gradients.
 */

import { rgbCSS, roundRect, type Palette } from './engine';

export interface PetLook {
  /** Pet level, drives which accessory shows (bow >=3, glasses >=5, crown >=8). */
  level: number;
  /** Ducking flattens and widens the body. */
  duck?: boolean;
  /** 0..1 squash for the death animation. */
  squash?: number;
  /** Eyes closed (blink / dead). */
  shut?: boolean;
}

/**
 * Draw the pet standing on (x, y) — x is its centre, y the ground line.
 * `h` is its standing height in CSS pixels.
 */
export function drawPet(
  ctx: CanvasRenderingContext2D,
  p: Palette,
  x: number,
  y: number,
  h: number,
  look: PetLook,
): void {
  const squash = look.squash ?? 0;
  const duck = look.duck ? 0.55 : 1;
  const bodyH = h * duck * (1 - squash * 0.55);
  const bodyW = h * 0.88 * (look.duck ? 1.22 : 1) * (1 + squash * 0.35);
  const cx = x;
  const cy = y - bodyH / 2;
  const rx = bodyW / 2;
  const ry = bodyH / 2;

  const accent = rgbCSS(p.accent);
  const soft = rgbCSS(p.accent, 0.16);
  const lw = Math.max(1.6, h * 0.05);

  ctx.save();
  ctx.lineJoin = 'round';
  ctx.lineCap = 'round';

  // ears, behind the body
  ctx.fillStyle = soft;
  ctx.strokeStyle = accent;
  ctx.lineWidth = lw;
  const earH = ry * 0.5;
  for (const s of [-1, 1]) {
    ctx.beginPath();
    ctx.moveTo(cx + s * rx * 0.62, cy - ry * 0.55);
    ctx.lineTo(cx + s * rx * 0.78, cy - ry - earH);
    ctx.lineTo(cx + s * rx * 0.16, cy - ry * 0.86);
    ctx.closePath();
    ctx.fill();
    ctx.stroke();
  }

  // body
  ctx.beginPath();
  ctx.ellipse(cx, cy, rx, ry, 0, 0, Math.PI * 2);
  ctx.fill();
  ctx.stroke();

  // eyes
  const eyeY = cy - ry * 0.05;
  const eyeDX = rx * 0.36;
  const eyeR = Math.max(1.4, h * 0.058);
  ctx.fillStyle = accent;
  ctx.strokeStyle = accent;
  if (look.shut) {
    ctx.lineWidth = lw * 0.9;
    for (const s of [-1, 1]) {
      ctx.beginPath();
      ctx.moveTo(cx + s * eyeDX - eyeR, eyeY);
      ctx.lineTo(cx + s * eyeDX + eyeR, eyeY);
      ctx.stroke();
    }
  } else {
    for (const s of [-1, 1]) {
      ctx.beginPath();
      ctx.arc(cx + s * eyeDX, eyeY, eyeR, 0, Math.PI * 2);
      ctx.fill();
    }
  }

  // accessories
  if (look.level >= 3) {
    // bow at the chin
    const bx = cx;
    const by = cy + ry * 0.78;
    const bw = rx * 0.34;
    ctx.fillStyle = rgbCSS(p.accent, 0.85);
    for (const s of [-1, 1]) {
      ctx.beginPath();
      ctx.moveTo(bx, by);
      ctx.lineTo(bx + s * bw, by - bw * 0.55);
      ctx.lineTo(bx + s * bw, by + bw * 0.55);
      ctx.closePath();
      ctx.fill();
    }
  }
  if (look.level >= 5) {
    ctx.strokeStyle = rgbCSS(p.muted, 0.9);
    ctx.lineWidth = Math.max(1, lw * 0.6);
    for (const s of [-1, 1]) {
      ctx.beginPath();
      ctx.arc(cx + s * eyeDX, eyeY, eyeR * 1.95, 0, Math.PI * 2);
      ctx.stroke();
    }
    ctx.beginPath();
    ctx.moveTo(cx - eyeDX + eyeR * 1.95, eyeY);
    ctx.lineTo(cx + eyeDX - eyeR * 1.95, eyeY);
    ctx.stroke();
  }
  if (look.level >= 8) {
    const cw = rx * 0.9;
    const top = cy - ry - earH * 0.9;
    ctx.strokeStyle = rgbCSS(p.amber);
    ctx.lineWidth = Math.max(1.4, lw * 0.85);
    ctx.beginPath();
    ctx.moveTo(cx - cw / 2, top);
    ctx.lineTo(cx - cw / 2, top - cw * 0.42);
    ctx.lineTo(cx - cw / 4, top - cw * 0.16);
    ctx.lineTo(cx, top - cw * 0.5);
    ctx.lineTo(cx + cw / 4, top - cw * 0.16);
    ctx.lineTo(cx + cw / 2, top - cw * 0.42);
    ctx.lineTo(cx + cw / 2, top);
    ctx.stroke();
  }

  ctx.restore();
}

/** The small document glyph collected for score + xp. Centred on (x, y). */
export function drawFileGlyph(
  ctx: CanvasRenderingContext2D,
  p: Palette,
  x: number,
  y: number,
  s: number,
): void {
  const w = s * 0.78;
  const h = s;
  ctx.save();
  ctx.translate(x - w / 2, y - h / 2);
  ctx.fillStyle = rgbCSS(p.accent, 0.16);
  ctx.strokeStyle = rgbCSS(p.accent);
  ctx.lineWidth = 1.5;
  ctx.lineJoin = 'round';
  roundRect(ctx, 0, 0, w, h, 2.5);
  ctx.fill();
  ctx.stroke();
  // folded corner
  ctx.beginPath();
  ctx.moveTo(w * 0.62, 0);
  ctx.lineTo(w * 0.62, h * 0.28);
  ctx.lineTo(w, h * 0.28);
  ctx.stroke();
  // text lines
  ctx.beginPath();
  ctx.moveTo(w * 0.22, h * 0.56);
  ctx.lineTo(w * 0.78, h * 0.56);
  ctx.moveTo(w * 0.22, h * 0.76);
  ctx.lineTo(w * 0.66, h * 0.76);
  ctx.stroke();
  ctx.restore();
}

/** A checkmark, used on the "submitted" gates you duck under. */
export function drawCheck(
  ctx: CanvasRenderingContext2D,
  color: string,
  x: number,
  y: number,
  s: number,
): void {
  ctx.save();
  ctx.strokeStyle = color;
  ctx.lineWidth = Math.max(1.6, s * 0.16);
  ctx.lineCap = 'round';
  ctx.lineJoin = 'round';
  ctx.beginPath();
  ctx.moveTo(x - s * 0.42, y);
  ctx.lineTo(x - s * 0.1, y + s * 0.34);
  ctx.lineTo(x + s * 0.44, y - s * 0.34);
  ctx.stroke();
  ctx.restore();
}
