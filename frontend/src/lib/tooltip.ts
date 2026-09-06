/**
 * Native-feeling tooltips: a `use:tip` action that drives one shared bubble
 * rendered by <Tooltip> (mounted once, from App).
 *
 * Written by hand rather than using `title` because the rail needs the delay,
 * the side and the styling to match the rest of the app — the OS tooltip is
 * slow, unstyled and cannot be positioned.
 */

import { writable } from 'svelte/store';

export type TipSide = 'right' | 'top';

export interface TipOptions {
  text: string;
  side?: TipSide;
  /** Skip showing entirely (e.g. the label is already visible in expanded mode). */
  disabled?: boolean;
}

export interface TipState {
  text: string;
  side: TipSide;
  /** Anchor rect in viewport coordinates. */
  x: number;
  y: number;
}

/** The one visible tooltip, or null. */
export const tipState = writable<TipState | null>(null);

const DELAY = 400;

function normalize(opts: TipOptions | string): TipOptions {
  return typeof opts === 'string' ? { text: opts } : opts;
}

export function tip(node: HTMLElement, opts: TipOptions | string) {
  let o = normalize(opts);
  let timer: ReturnType<typeof setTimeout> | undefined;

  function hide() {
    clearTimeout(timer);
    tipState.update((s) => (s && s.text === o.text ? null : s));
  }

  function show() {
    if (!o.text || o.disabled) return;
    const r = node.getBoundingClientRect();
    tipState.set({
      text: o.text,
      side: o.side ?? 'right',
      x: o.side === 'top' ? r.left + r.width / 2 : r.right,
      y: o.side === 'top' ? r.top : r.top + r.height / 2,
    });
  }

  function schedule() {
    clearTimeout(timer);
    if (!o.text || o.disabled) return;
    timer = setTimeout(show, DELAY);
  }

  /** Focus is a deliberate "show me" — no delay there. */
  function immediate() {
    clearTimeout(timer);
    show();
  }

  node.addEventListener('pointerenter', schedule);
  node.addEventListener('pointerleave', hide);
  node.addEventListener('pointerdown', hide);
  node.addEventListener('focusin', immediate);
  node.addEventListener('focusout', hide);

  return {
    update(next: TipOptions | string) {
      o = normalize(next);
      if (o.disabled) hide();
    },
    destroy() {
      hide();
      node.removeEventListener('pointerenter', schedule);
      node.removeEventListener('pointerleave', hide);
      node.removeEventListener('pointerdown', hide);
      node.removeEventListener('focusin', immediate);
      node.removeEventListener('focusout', hide);
    },
  };
}
