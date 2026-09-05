<script lang="ts">
  /**
   * Lecture Merge — 2048 with a lecture ladder (L1…L13, Midterm, Final, A+).
   * DOM + CSS transforms rather than canvas: the tiles are text, and the
   * browser animates 16 nodes for free.
   */
  import { onDestroy } from 'svelte';
  import Icon from '../components/Icon.svelte';
  import { addXP } from '../pet';
  import { resolvedTheme } from '../stores';
  import { toast } from '../stores';
  import { keyCode, luminance, mix, prefersReducedMotion, readPalette, rgbCSS } from './engine';
  import { claimFinalBonus, mergeSave, recordMerge } from './scores';

  interface Props {
    onExit: () => void;
  }

  let { onExit }: Props = $props();

  interface Tile {
    id: number;
    rank: number;
    r: number;
    c: number;
    /** merged away this move — slides under the survivor, then removed */
    dead?: boolean;
    /** just spawned or just merged, for the pop */
    pop?: boolean;
  }

  const N = 4;
  const FINAL_RANK = 15;
  const reduced = prefersReducedMotion();
  const ANIM = reduced ? 0 : 100;

  let seq = 0;
  let tiles = $state<Tile[]>([]);
  let score = $state(0);
  let over = $state(false);
  let won = $state(false);
  let undoLeft = $state(1);
  let animating = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let snapshot = $state<{ tiles: Tile[]; score: number } | null>(null);

  const best = $derived($mergeSave.best);

  // Tile colours step through accent tints derived from the CSS tokens, so the
  // ladder stays on-brand in both themes.
  const palette = $derived.by(() => {
    void $resolvedTheme;
    return readPalette();
  });

  function tileStyle(rank: number): string {
    const p = palette;
    const t = Math.min(1, (rank - 1) / 10);
    let bg = mix(p.subtle, p.accent, 0.1 + t * 0.9);
    if (rank > 11) bg = mix(bg, p.dark ? [255, 255, 255] : [0, 0, 0], (rank - 11) * 0.07);
    const text = luminance(bg) > 0.42 ? p.text : ([255, 255, 255] as [number, number, number]);
    return `background:${rgbCSS(bg)};color:${rgbCSS(text)}`;
  }

  function label(rank: number): string {
    if (rank <= 13) return `L${rank}`;
    if (rank === 14) return 'Midterm';
    if (rank === 15) return 'Final';
    return 'A+';
  }

  function pos(i: number): number {
    const gap = 2.4;
    const cell = (100 - gap * (N + 1)) / N;
    return gap + i * (cell + gap);
  }

  const CELL = (100 - 2.4 * (N + 1)) / N;

  // ------------------------------------------------------------ mechanics

  function gridOf(list: Tile[]): (Tile | null)[][] {
    const g: (Tile | null)[][] = Array.from({ length: N }, () => Array(N).fill(null));
    for (const t of list) if (!t.dead) g[t.r][t.c] = t;
    return g;
  }

  function emptyCells(list: Tile[]): Array<[number, number]> {
    const g = gridOf(list);
    const out: Array<[number, number]> = [];
    for (let r = 0; r < N; r++) for (let c = 0; c < N; c++) if (!g[r][c]) out.push([r, c]);
    return out;
  }

  function spawn(list: Tile[]): Tile[] {
    const free = emptyCells(list);
    if (free.length === 0) return list;
    const [r, c] = free[Math.floor(Math.random() * free.length)];
    return [...list, { id: ++seq, rank: Math.random() < 0.9 ? 1 : 2, r, c, pop: true }];
  }

  function newGame() {
    clearTimeout(timer);
    animating = false;
    seq = 0;
    score = 0;
    over = false;
    won = false;
    undoLeft = 1;
    snapshot = null;
    tiles = spawn(spawn([]));
  }

  function canMove(list: Tile[]): boolean {
    const g = gridOf(list);
    for (let r = 0; r < N; r++) {
      for (let c = 0; c < N; c++) {
        const t = g[r][c];
        if (!t) return true;
        if (c + 1 < N && g[r][c + 1]?.rank === t.rank) return true;
        if (r + 1 < N && g[r + 1][c]?.rank === t.rank) return true;
      }
    }
    return false;
  }

  type Dir = 'up' | 'down' | 'left' | 'right';

  function move(dir: Dir) {
    if (animating || over) return;
    const g = gridOf(tiles);
    const before = tiles.map((t) => ({ ...t }));
    const beforeScore = score;

    const vertical = dir === 'up' || dir === 'down';
    const backwards = dir === 'down' || dir === 'right';

    const survivors: Tile[] = [];
    const dead: Tile[] = [];
    let moved = false;
    let gained = 0;
    let hitFinal = false;

    for (let line = 0; line < N; line++) {
      // read the line in travel order (nearest the destination edge first)
      const cells: Tile[] = [];
      for (let i = 0; i < N; i++) {
        const idx = backwards ? N - 1 - i : i;
        const t = vertical ? g[idx][line] : g[line][idx];
        if (t) cells.push(t);
      }

      /** each surviving tile, with the tile it swallowed (if any) */
      const out: Array<{ tile: Tile; absorbed?: Tile }> = [];
      for (let i = 0; i < cells.length; i++) {
        const cur = cells[i];
        const nxt = cells[i + 1];
        if (nxt && nxt.rank === cur.rank) {
          const rank = cur.rank + 1;
          gained += Math.pow(2, rank);
          if (rank >= FINAL_RANK) hitFinal = true;
          out.push({ tile: { ...cur, rank, pop: true }, absorbed: { ...nxt, dead: true } });
          moved = true;
          i++;
        } else {
          out.push({ tile: { ...cur, pop: false } });
        }
      }

      // write back, packed against the destination edge
      for (let i = 0; i < out.length; i++) {
        const idx = backwards ? N - 1 - i : i;
        const { tile, absorbed } = out[i];
        const nr = vertical ? idx : line;
        const nc = vertical ? line : idx;
        if (tile.r !== nr || tile.c !== nc) moved = true;
        tile.r = nr;
        tile.c = nc;
        survivors.push(tile);
        if (absorbed) {
          // slides onto the survivor, then disappears
          absorbed.r = nr;
          absorbed.c = nc;
          dead.push(absorbed);
        }
      }
    }

    if (!moved) return;

    snapshot = { tiles: before, score: beforeScore };
    score += gained;
    tiles = [...survivors, ...dead];
    animating = true;

    timer = setTimeout(() => {
      tiles = spawn(tiles.filter((t) => !t.dead).map((t) => ({ ...t, pop: false })));
      animating = false;
      recordMerge(score);
      if (hitFinal && !won) {
        won = true;
        if (claimFinalBonus()) {
          addXP(50, 'lecture merge final');
          toast('Reached Final — +50 xp', 'success');
        } else {
          toast('Reached Final', 'success');
        }
      }
      if (!canMove(tiles)) over = true;
    }, ANIM);
  }

  function undo() {
    if (!snapshot || undoLeft <= 0 || animating) return;
    clearTimeout(timer);
    tiles = snapshot.tiles;
    score = snapshot.score;
    snapshot = null;
    undoLeft = 0;
    over = false;
    animating = false;
  }

  const KEYMAP: Record<string, Dir> = {
    ArrowUp: 'up',
    ArrowDown: 'down',
    ArrowLeft: 'left',
    ArrowRight: 'right',
    KeyW: 'up',
    KeyS: 'down',
    KeyA: 'left',
    KeyD: 'right',
  };

  // Bound via <svelte:window>, so the board only listens while it is mounted.
  function onKey(e: KeyboardEvent) {
    const el = e.target as HTMLElement | null;
    if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.isContentEditable)) return;
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    const code = keyCode(e);
    const dir = KEYMAP[code];
    if (dir) {
      e.preventDefault();
      move(dir);
      return;
    }
    if (code === 'KeyU') {
      e.preventDefault();
      undo();
    }
    if (code === 'KeyR') {
      e.preventDefault();
      newGame();
    }
  }

  newGame();

  onDestroy(() => clearTimeout(timer));
</script>

<svelte:window onkeydown={onKey} />

<div class="game">
  <header class="bar">
    <button class="btn sm ghost" onclick={onExit}>
      <Icon name="chevronLeft" size={13} />
      Arcade
    </button>
    <div class="grow"></div>
    <span class="stat"><span class="k">Score</span><span class="v">{score}</span></span>
    <span class="stat"><span class="k">Best</span><span class="v">{best}</span></span>
    <button class="btn sm" onclick={undo} disabled={undoLeft === 0 || !snapshot}>Undo</button>
    <button class="btn sm" onclick={newGame}>New game</button>
  </header>

  <div class="stage">
    <div class="board" style="--anim:{ANIM}ms">
      {#each Array(N * N) as _, i (i)}
        <div class="slot" style="left:{pos(i % N)}%;top:{pos(Math.floor(i / N))}%;width:{CELL}%;height:{CELL}%"></div>
      {/each}

      {#each tiles as t (t.id)}
        <div
          class="tile"
          class:pop={t.pop && !reduced}
          class:dead={t.dead}
          class:long={t.rank >= 14}
          style="left:{pos(t.c)}%;top:{pos(t.r)}%;width:{CELL}%;height:{CELL}%;{tileStyle(t.rank)}"
        >
          {label(t.rank)}
        </div>
      {/each}

      {#if over}
        <div class="overlay">
          <p class="big">No moves left</p>
          <p class="muted">Score {score} · best {best}</p>
          <button class="btn sm primary" onclick={newGame}>Play again</button>
        </div>
      {/if}
    </div>
  </div>

  <footer class="hints faint">
    <kbd>↑↓←→</kbd> or <kbd>WASD</kbd> to merge · <kbd>U</kbd> undo (once) · <kbd>R</kbd> new game
  </footer>
</div>

<style>
  .game {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
    padding: 14px 18px 12px;
    gap: 10px;
  }

  .bar {
    display: flex;
    align-items: center;
    gap: 10px;
    flex: none;
  }

  .grow {
    flex: 1;
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
    font-size: 14px;
    font-weight: 620;
  }

  .stage {
    flex: 1;
    min-height: 0;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .board {
    position: relative;
    height: min(420px, 100%);
    aspect-ratio: 1;
    max-width: 100%;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--bg-elevated);
  }

  .slot {
    position: absolute;
    border-radius: var(--radius);
    background: var(--bg-subtle);
  }

  .tile {
    position: absolute;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: var(--radius);
    font-size: 17px;
    font-weight: 640;
    letter-spacing: -0.01em;
    font-variant-numeric: tabular-nums;
    transition: left var(--anim) ease, top var(--anim) ease;
  }

  .tile.long {
    font-size: 13px;
  }

  .tile.dead {
    z-index: 0;
  }

  .tile.pop {
    animation: pop 140ms cubic-bezier(0.34, 1.4, 0.5, 1);
    z-index: 1;
  }

  @keyframes pop {
    0% {
      transform: scale(0.72);
    }
    100% {
      transform: scale(1);
    }
  }

  .overlay {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    border-radius: var(--radius-lg);
    background: color-mix(in srgb, var(--bg-elevated) 80%, transparent);
  }

  .big {
    font-size: 18px;
    font-weight: 620;
  }

  .hints {
    flex: none;
    font-size: 11.5px;
    text-align: center;
  }

  kbd {
    padding: 1px 5px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg-subtle);
    font-family: var(--font);
    font-size: 10.5px;
  }
</style>
