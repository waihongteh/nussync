/**
 * Which arcade game is on screen. Lives outside the view so the command
 * palette (and anything else) can jump straight into a game.
 */

import { writable } from 'svelte/store';
import { navigate } from '../stores';

export type ArcadeGame = 'run-daily' | 'run-free' | 'merge';

/** `null` = show the picker. */
export const arcadePick = writable<ArcadeGame | null>(null);

export function playGame(g: ArcadeGame): void {
  arcadePick.set(g);
  navigate('arcade');
}
