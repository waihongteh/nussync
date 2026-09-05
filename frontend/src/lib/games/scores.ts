/**
 * Arcade high scores. Plain stores backed by localStorage, so the picker can
 * show bests live while a game is being played.
 */

import { get, writable } from 'svelte/store';
import { lsGet, lsSet } from '../util';
import { todayKey } from './engine';

const RUN_KEY = 'nussync.arcade.run';
const MERGE_KEY = 'nussync.arcade.merge';

export interface RunSave {
  best: number;
  dailyBest: number;
  /** YYYY-MM-DD the daily best belongs to */
  dailyDate: string;
}

export interface MergeSave {
  best: number;
  /** YYYY-MM-DD of the last "reached Final" xp bonus */
  finalDay: string;
}

const runDefault: RunSave = { best: 0, dailyBest: 0, dailyDate: '' };
const mergeDefault: MergeSave = { best: 0, finalDay: '' };

export const runSave = writable<RunSave>({ ...runDefault, ...lsGet(RUN_KEY, runDefault) });
export const mergeSave = writable<MergeSave>({ ...mergeDefault, ...lsGet(MERGE_KEY, mergeDefault) });

runSave.subscribe((v) => lsSet(RUN_KEY, v));
mergeSave.subscribe((v) => lsSet(MERGE_KEY, v));

/** Record a finished run; daily runs also update today's best. */
export function recordRun(score: number, daily: boolean): void {
  const today = todayKey();
  runSave.update((s) => {
    const stale = s.dailyDate !== today;
    return {
      best: Math.max(s.best, score),
      dailyBest: daily ? Math.max(stale ? 0 : s.dailyBest, score) : stale ? 0 : s.dailyBest,
      dailyDate: daily ? today : stale ? today : s.dailyDate,
    };
  });
}

export function recordMerge(score: number): void {
  mergeSave.update((s) => ({ ...s, best: Math.max(s.best, score) }));
}

/** True (once per calendar day) the first time Lecture Merge reaches Final. */
export function claimFinalBonus(): boolean {
  const today = todayKey();
  if (get(mergeSave).finalDay === today) return false;
  mergeSave.update((s) => ({ ...s, finalDay: today }));
  return true;
}

/** Today's daily best, or 0 when the stored one is from another day. */
export function dailyBestToday(s: RunSave): number {
  return s.dailyDate === todayKey() ? s.dailyBest : 0;
}
