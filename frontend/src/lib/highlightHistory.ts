/**
 * Undo/redo for PDF highlights.
 *
 * A tiny command stack owned by the viewer, one per open file. Commands are
 * plain data (not closures) for two reasons: consecutive note edits have to be
 * *coalesced* — the note textarea saves on a debounce, so typing a sentence
 * would otherwise push a dozen commands and undo would walk back one keystroke
 * at a time — and deleted highlights come back with a *new* id.
 *
 * Why a new id: `Store.PutHighlight` treats a non-zero ID as an UPDATE (see
 * internal/store/highlights.go), so a row that was deleted cannot be re-created
 * under its old id — the update matches nothing and errors. Undoing a delete
 * therefore inserts a fresh row and records old -> new in `remap`; every
 * command resolves its ids through that map, so a chain like
 * `add / recolour / delete` still undoes and redoes correctly afterwards.
 *
 * The stack never touches the backend itself: the viewer passes in hooks that
 * call `api.saveHighlight` / `api.deleteHighlight` and keep the local
 * `highlights` array (and therefore the rail and the page overlays) in step.
 */

import type { Highlight } from './types';

/** Deepest history we keep; older entries fall off the bottom. */
export const HISTORY_CAP = 100;

/** Note edits closer together than this fold into one undo step. */
export const NOTE_COALESCE_MS = 1500;

/** What a command needs the viewer to do. All of these must update local state. */
export interface HistoryHooks {
  /** Insert a highlight (ID is ignored / sent as 0) and return the stored row. */
  create(h: Highlight): Promise<Highlight>;
  /** Delete by id. */
  remove(id: number): Promise<void>;
  /** Apply a partial change to one highlight. */
  patch(id: number, changes: Partial<Highlight>): Promise<void>;
  /** Called after any push/undo/redo/clear so the UI can refresh button state. */
  onChange?(): void;
  /** Called when an undo or redo fails; the viewer reloads from the backend. */
  onError?(err: unknown): void;
}

type Cmd =
  /** One or more highlights created together (a selection can span pages). */
  | { type: 'add'; label: string; items: Highlight[] }
  /** One or more highlights removed together. */
  | { type: 'delete'; label: string; items: Highlight[] }
  /** A colour or note change on a single highlight. */
  | {
      type: 'update';
      label: string;
      id: number;
      before: Partial<Highlight>;
      after: Partial<Highlight>;
      /** Note edits coalesce; colour changes do not. */
      note: boolean;
      at: number;
    };

export class HighlightHistory {
  #hooks: HistoryHooks;
  #undo: Cmd[] = [];
  #redo: Cmd[] = [];
  /** old highlight id -> id it was re-created under. Chains are followed. */
  #remap = new Map<number, number>();
  /** Guards against a second undo landing mid-flight. */
  #busy = false;

  constructor(hooks: HistoryHooks) {
    this.#hooks = hooks;
  }

  get canUndo(): boolean {
    return !this.#busy && this.#undo.length > 0;
  }

  get canRedo(): boolean {
    return !this.#busy && this.#redo.length > 0;
  }

  /** Label of the next undo step, for tooltips. */
  get undoLabel(): string {
    return this.#undo[this.#undo.length - 1]?.label ?? '';
  }

  get redoLabel(): string {
    return this.#redo[this.#redo.length - 1]?.label ?? '';
  }

  /** Drop everything. Called when a different file opens. */
  clear(): void {
    this.#undo = [];
    this.#redo = [];
    this.#remap.clear();
    this.#busy = false;
    this.#hooks.onChange?.();
  }

  /** Follow the re-creation chain for an id. */
  #live(id: number): number {
    let cur = id;
    // The chain is short (one link per undo of a delete) but guard anyway.
    for (let i = 0; i < 100; i++) {
      const next = this.#remap.get(cur);
      if (next === undefined || next === cur) break;
      cur = next;
    }
    return cur;
  }

  #note(oldID: number, newID: number): void {
    if (oldID !== newID) this.#remap.set(oldID, newID);
  }

  #push(cmd: Cmd): void {
    this.#redo = [];
    this.#undo.push(cmd);
    if (this.#undo.length > HISTORY_CAP) this.#undo.splice(0, this.#undo.length - HISTORY_CAP);
    this.#hooks.onChange?.();
  }

  // ------------------------------------------------------------- recording
  //
  // The viewer performs the original action itself (it already has the
  // optimistic-update code) and then records it here.

  /** Record highlights the user just created. */
  recordAdd(items: Highlight[]): void {
    if (!items.length) return;
    this.#push({
      type: 'add',
      label: items.length > 1 ? `Add ${items.length} highlights` : 'Add highlight',
      items: items.map((h) => ({ ...h })),
    });
  }

  /** Record highlights the user just deleted (snapshot taken before removal). */
  recordDelete(items: Highlight[]): void {
    if (!items.length) return;
    this.#push({
      type: 'delete',
      label: items.length > 1 ? `Delete ${items.length} highlights` : 'Delete highlight',
      items: items.map((h) => ({ ...h })),
    });
  }

  /**
   * Record a colour or note change. Consecutive note edits on the same
   * highlight within NOTE_COALESCE_MS fold into the pending command, keeping
   * its original `before`, so one undo restores the note as it was before the
   * user started typing.
   */
  recordUpdate(
    id: number,
    before: Partial<Highlight>,
    after: Partial<Highlight>,
    opts: { note?: boolean } = {},
  ): void {
    const note = !!opts.note;
    const now = Date.now();
    const top = this.#undo[this.#undo.length - 1];
    if (
      note &&
      !this.#redo.length &&
      top &&
      top.type === 'update' &&
      top.note &&
      this.#live(top.id) === this.#live(id) &&
      now - top.at <= NOTE_COALESCE_MS
    ) {
      top.after = { ...top.after, ...after };
      top.at = now;
      this.#hooks.onChange?.();
      return;
    }
    this.#push({
      type: 'update',
      label: note ? 'Edit note' : 'Change colour',
      id,
      before: { ...before },
      after: { ...after },
      note,
      at: now,
    });
  }

  // ------------------------------------------------------------ undo / redo

  async undo(): Promise<string> {
    if (this.#busy) return '';
    const cmd = this.#undo.pop();
    if (!cmd) return '';
    this.#busy = true;
    this.#hooks.onChange?.();
    try {
      await this.#apply(cmd, 'undo');
      this.#redo.push(cmd);
      return cmd.label;
    } catch (err) {
      // The stack is no longer a truthful description of the backend; drop it
      // rather than let a later undo do something surprising.
      this.#undo = [];
      this.#redo = [];
      this.#hooks.onError?.(err);
      return '';
    } finally {
      this.#busy = false;
      this.#hooks.onChange?.();
    }
  }

  async redo(): Promise<string> {
    if (this.#busy) return '';
    const cmd = this.#redo.pop();
    if (!cmd) return '';
    this.#busy = true;
    this.#hooks.onChange?.();
    try {
      await this.#apply(cmd, 'redo');
      this.#undo.push(cmd);
      return cmd.label;
    } catch (err) {
      this.#undo = [];
      this.#redo = [];
      this.#hooks.onError?.(err);
      return '';
    } finally {
      this.#busy = false;
      this.#hooks.onChange?.();
    }
  }

  /** Re-insert a command's highlights and remember the ids they came back with. */
  async #restore(items: Highlight[]): Promise<void> {
    for (const h of items) {
      const id = this.#live(h.ID);
      const saved = await this.#hooks.create({ ...h, ID: 0 });
      this.#note(id, saved.ID);
      this.#note(h.ID, saved.ID);
    }
  }

  async #erase(items: Highlight[]): Promise<void> {
    for (const h of items) await this.#hooks.remove(this.#live(h.ID));
  }

  async #apply(cmd: Cmd, dir: 'undo' | 'redo'): Promise<void> {
    switch (cmd.type) {
      case 'add':
        if (dir === 'undo') await this.#erase(cmd.items);
        else await this.#restore(cmd.items);
        return;
      case 'delete':
        if (dir === 'undo') await this.#restore(cmd.items);
        else await this.#erase(cmd.items);
        return;
      case 'update':
        await this.#hooks.patch(this.#live(cmd.id), dir === 'undo' ? cmd.before : cmd.after);
        return;
    }
  }
}
