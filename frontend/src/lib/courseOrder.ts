/**
 * Sidebar-local course presentation: manual order and hidden ids.
 *
 * This is a purely local view preference — hidden courses still sync and still
 * appear everywhere else (Files, Deadlines, Settings). Nothing here touches the
 * backend, so both stores live in localStorage only.
 */

import { get, writable } from 'svelte/store';
import type { Course } from './types';
import { lsGet, lsSet } from './util';

const ORDER_KEY = 'nussync.courses.order';
const HIDDEN_KEY = 'nussync.courses.hidden';

/** Course ids in the user's chosen order. Ids not listed sort after, in default order. */
export const courseOrder = writable<number[]>(lsGet<number[]>(ORDER_KEY, []));

/** Course ids collapsed under the sidebar's "Hidden" row. */
export const hiddenCourses = writable<number[]>(lsGet<number[]>(HIDDEN_KEY, []));

courseOrder.subscribe((v) => lsSet(ORDER_KEY, v));
hiddenCourses.subscribe((v) => lsSet(HIDDEN_KEY, v));

/**
 * Sort `list` by `order`: known ids first, in the saved sequence, then every
 * course the saved order has never seen, in the order the backend gave them.
 */
export function applyOrder(list: Course[], order: number[]): Course[] {
  const byID = new Map(list.map((c) => [c.ID, c]));
  const out: Course[] = [];
  for (const id of order) {
    const c = byID.get(id);
    if (c) {
      out.push(c);
      byID.delete(id);
    }
  }
  for (const c of list) if (byID.has(c.ID)) out.push(c);
  return out;
}

/** Persist an explicit sequence (visible rows first, hidden ones after). */
export function saveOrder(ids: number[]) {
  courseOrder.set([...ids]);
}

export function isHidden(id: number): boolean {
  return get(hiddenCourses).includes(id);
}

export function hideCourse(id: number) {
  hiddenCourses.update((h) => (h.includes(id) ? h : [...h, id]));
}

export function unhideCourse(id: number) {
  hiddenCourses.update((h) => h.filter((x) => x !== id));
}

/** Hide every course whose term reads as non-academic (Settings quick action). */
export function hideNonAcademic(list: Course[]): number {
  const ids = list.filter((c) => c.Term.trim() === '' || c.Term === 'Non-Academic').map((c) => c.ID);
  let added = 0;
  hiddenCourses.update((h) => {
    const set = new Set(h);
    for (const id of ids) if (!set.has(id)) (set.add(id), added++);
    return [...set];
  });
  return added;
}
