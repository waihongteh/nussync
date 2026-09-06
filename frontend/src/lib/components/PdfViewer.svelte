<script lang="ts">
  /**
   * PDF.js-based reader with persistent highlights.
   *
   * Why not the WebView's built-in PDF plugin (still available behind the
   * viewer's "Use built-in viewer" toggle)? The plugin renders inside its own
   * opaque document: there is no text layer we can reach, so no selection, no
   * quoting and no highlighting. PDF.js gives us a real DOM text layer over a
   * canvas, which is what every feature here is built on.
   *
   * Layout per page: <canvas> (z0) / highlight boxes (z1, multiply-blended) /
   * find matches (z1) / .textLayer (z2, transparent text, selectable). All
   * overlays are pointer-events:none — clicks are hit-tested against the
   * highlight rectangles in JS instead, because the text spans sit on top and
   * would otherwise swallow them.
   *
   * Highlight geometry is stored normalised 0..1 against the page box, so a
   * highlight made at 175% renders correctly at 60%.
   *
   * Performance: only visible pages (±1) hold a canvas; the rest keep a
   * correctly-sized placeholder so the scrollbar never jumps. Canvas backing
   * store is capped at 2x CSS pixels — a 100-page deck on a 4K display would
   * otherwise allocate gigabytes.
   */
  import { api, errMsg, on } from '../api';
  import { askChat, toast } from '../stores';
  import type { Highlight, MenuItem, Rect } from '../types';
  import { lsGet, lsSet } from '../util';
  import ContextMenu from './ContextMenu.svelte';
  import Icon from './Icon.svelte';

  /**
   * pdf.js is ~430 KB of JS plus a 1.2 MB worker. Loading it eagerly would tax
   * every launch for a feature most screens never touch, so it is imported the
   * first time a PDF is opened and cached for the rest of the session. The
   * worker is bundled by Vite, never fetched from a CDN: the app runs on the
   * wails:// origin with no network guarantee, and course material must not
   * leak to a third party.
   */
  let pdfjsLib: typeof import('pdfjs-dist') | null = null;
  let pdfjsPromise: Promise<typeof import('pdfjs-dist')> | null = null;

  function ensurePdfjs() {
    pdfjsPromise ??= (async () => {
      const [lib, worker] = await Promise.all([
        import('pdfjs-dist'),
        import('pdfjs-dist/build/pdf.worker.min.mjs?url'),
      ]);
      lib.GlobalWorkerOptions.workerSrc = worker.default;
      pdfjsLib = lib;
      return lib;
    })();
    return pdfjsPromise;
  }

  interface Props {
    /** Canvas file id — the key highlights are stored under. */
    fileID: number;
    /** Asset-server URL, normally `/local/{id}`. */
    src: string;
    /** Used for the export filename and the toolbar's aria labels. */
    name: string;
  }

  let { fileID, src, name }: Props = $props();

  // ------------------------------------------------------------- constants

  const COLOR_KEY = 'nussync.viewer.hlColor';
  const RAIL_KEY = 'nussync.viewer.hlRail';

  /** Palette. Kept light: they are multiply-blended over white paper. */
  const COLORS: Record<string, string> = {
    yellow: '#ffe97a',
    green: '#a8e6a3',
    blue: '#9ed3f5',
    pink: '#ffaecb',
  };
  const COLOR_NAMES = Object.keys(COLORS);

  const MIN_SCALE = 0.5;
  const MAX_SCALE = 3;
  /** Gap between pages, and the scroll container's padding. Keep in sync with CSS. */
  const PAGE_GAP = 12;

  // ----------------------------------------------------------------- state

  let doc = $state<any>(null);
  let numPages = $state(0);
  let loading = $state(true);
  let error = $state('');

  /** Page box in CSS px at scale 1. Page 1's size seeds every placeholder. */
  let baseSize = $state({ w: 612, h: 792 });
  /** Real sizes, filled in as pages are fetched (decks are rarely uniform). */
  let sizes = $state<Record<number, { w: number; h: number }>>({});

  let scale = $state(1);
  /** While true, `scale` is recomputed from the container width on resize. */
  let fitWidth = $state(true);

  let visible = $state<number[]>([]);
  let currentPage = $state(1);
  let pageInput = $state('1');

  let highlights = $state<Highlight[]>([]);
  let lastColor = $state(lsGet<string>(COLOR_KEY, 'yellow'));
  let highlighterOn = $state(false);
  let railOpen = $state(lsGet<boolean>(RAIL_KEY, false));

  let findOpen = $state(false);
  let findQuery = $state('');
  let findMatches = $state<{ page: number; at: number }[]>([]);
  let findIndex = $state(0);
  let findRects = $state<Record<number, Rect[]>>({});
  let findInput = $state<HTMLInputElement | null>(null);

  let popover = $state<{ id: number; x: number; y: number } | null>(null);
  let menu = $state<{ x: number; y: number; items: MenuItem[] } | null>(null);

  let rootEl = $state<HTMLDivElement | null>(null);
  let scrollEl = $state<HTMLDivElement | null>(null);

  /**
   * Page elements are found by query rather than `bind:this` into an array:
   * Svelte 5 warns (`binding_property_non_reactive`) about binding into a plain
   * array, and making the arrays `$state` would feed every canvas swap back
   * into the render effect that caused it.
   */
  function pageEl(n: number): HTMLDivElement | null {
    return scrollEl?.querySelector<HTMLDivElement>(`.page[data-page="${n}"]`) ?? null;
  }

  function canvasEl(n: number): HTMLCanvasElement | null {
    return pageEl(n)?.querySelector('canvas') ?? null;
  }

  function textEl(n: number): HTMLDivElement | null {
    return pageEl(n)?.querySelector<HTMLDivElement>('.textLayer') ?? null;
  }

  /** page -> { scale it was rendered at, in-flight render task, TextLayer }. */
  const rendered = new Map<number, { scale: number; task: any; layer: any }>();
  /** Cached extracted text per page, for find. */
  const pageText = new Map<number, string>();
  /** Bumped on every document load so stale async work can bail. */
  let docSeq = 0;
  /**
   * Bumped when a render aborts. Cancellation is normal (zoom or scroll moved
   * on mid-render), but the page is then left with the canvas painted and an
   * empty text layer and nothing would ask for it again — the render effect
   * only wakes for doc/scale/visibility changes. Reading this in the effect
   * gives an aborted page a second chance.
   */
  let renderTick = $state(0);

  const pageNums = $derived(Array.from({ length: numPages }, (_, i) => i + 1));
  const activeHighlight = $derived(popover ? highlights.find((h) => h.ID === popover?.id) ?? null : null);

  function sizeOf(n: number) {
    return sizes[n] ?? baseSize;
  }

  function hlFor(n: number) {
    return highlights.filter((h) => h.Page === n);
  }

  // ------------------------------------------------------------ load / free

  function teardown() {
    for (const [, r] of rendered) {
      try {
        r.task?.cancel?.();
      } catch {
        /* already settled */
      }
      try {
        r.layer?.cancel?.();
      } catch {
        /* already settled */
      }
    }
    rendered.clear();
    pageText.clear();
  }

  $effect(() => {
    const url = src;
    const seq = ++docSeq;
    teardown();
    doc = null;
    numPages = 0;
    visible = [];
    currentPage = 1;
    pageInput = '1';
    error = '';
    loading = true;

    let task: any = null;
    let dead = false;

    void (async () => {
      try {
        const lib = await ensurePdfjs();
        if (dead || seq !== docSeq) return;
        task = lib.getDocument({
          url,
          isEvalSupported: false,
          // ERRORS only: pdf.js logs a warning per unsupported font opcode, and
          // real course decks produce hundreds of them.
          verbosity: 0,
        });
        const d: any = await task.promise;
        if (dead || seq !== docSeq) {
          void d.destroy();
          return;
        }
        const first = await d.getPage(1);
        const vp = first.getViewport({ scale: 1 });
        if (dead || seq !== docSeq) return;
        baseSize = { w: vp.width, h: vp.height };
        sizes = { 1: { w: vp.width, h: vp.height } };
        doc = d;
        numPages = d.numPages;
        loading = false;
        queueMicrotask(applyFitWidth);
      } catch (err) {
        if (dead || seq !== docSeq) return;
        loading = false;
        error = errMsg(err) || 'Could not open this PDF.';
      }
    })();

    return () => {
      dead = true;
      teardown();
      void task?.promise?.then((d: any) => d.destroy()).catch(() => {});
      void task?.destroy?.();
    };
  });

  // ------------------------------------------------------------- highlights

  async function loadHighlights() {
    if (!fileID) {
      highlights = [];
      return;
    }
    try {
      highlights = await api.getHighlights(fileID);
    } catch (err) {
      // Not fatal: the document still reads, you just cannot see old marks.
      console.warn('[pdf] highlights:', errMsg(err));
      highlights = [];
    }
  }

  $effect(() => {
    const id = fileID;
    void id;
    void loadHighlights();
  });

  // Another window (or the Study view) may write highlights for this file.
  $effect(() =>
    on('highlights:updated', (p: { FileID?: number } | undefined) => {
      if (!p?.FileID || p.FileID === fileID) void loadHighlights();
    }),
  );

  // --------------------------------------------------------------- rendering

  /** Pages worth holding a canvas for: everything visible, plus one either side. */
  const wanted = $derived.by(() => {
    if (!numPages) return [];
    const set = new Set<number>();
    for (const n of visible.length ? visible : [1]) {
      for (let k = n - 1; k <= n + 1; k++) {
        if (k >= 1 && k <= numPages) set.add(k);
      }
    }
    return [...set].sort((a, b) => a - b);
  });

  $effect(() => {
    const d = doc;
    const s = scale;
    const want = wanted;
    void renderTick;
    if (!d) return;

    // Sweep every page, not just the ones `rendered` knows about. A render that
    // was in flight when its page scrolled away can finish after the entry was
    // dropped, leaving a painted canvas nothing owns; keying the release off
    // the DOM makes the visible set authoritative no matter how the races land.
    for (const n of pageNums) {
      const keep = want.includes(n);
      const r = rendered.get(n);
      if (keep && r?.scale === s) continue;
      if (r) {
        try {
          r.task?.cancel?.();
        } catch {
          /* already settled */
        }
        try {
          r.layer?.cancel?.();
        } catch {
          /* already settled */
        }
        rendered.delete(n);
      }
      const c = canvasEl(n);
      if (c?.width) {
        c.width = 0;
        c.height = 0;
      }
      const t = textEl(n);
      if (t?.firstChild) t.replaceChildren();
    }
    for (const n of want) {
      if (!rendered.has(n)) void renderPage(n, s);
    }
  });

  async function renderPage(n: number, atScale: number) {
    const d = doc;
    const seq = docSeq;
    if (!d) return;
    // Claim the slot before awaiting, or the effect re-runs and double-renders.
    const slot = { scale: atScale, task: null as any, layer: null as any };
    rendered.set(n, slot);
    try {
      const page = await d.getPage(n);
      if (seq !== docSeq || rendered.get(n) !== slot) return;

      const base = page.getViewport({ scale: 1 });
      if (sizes[n]?.w !== base.width || sizes[n]?.h !== base.height) {
        sizes = { ...sizes, [n]: { w: base.width, h: base.height } };
      }

      const viewport = page.getViewport({ scale: atScale });
      const canvas = canvasEl(n);
      const textDiv = textEl(n);
      if (!canvas || !textDiv) {
        abort(n, slot);
        return;
      }

      // Cap the backing store at 2x: beyond that the sharpness gain is
      // invisible and a 100-page deck starts thrashing GPU memory.
      const dpr = Math.min(window.devicePixelRatio || 1, 2);
      canvas.width = Math.max(1, Math.floor(viewport.width * dpr));
      canvas.height = Math.max(1, Math.floor(viewport.height * dpr));
      canvas.style.width = `${viewport.width}px`;
      canvas.style.height = `${viewport.height}px`;

      const ctx = canvas.getContext('2d', { alpha: false });
      if (!ctx) return;
      slot.task = page.render({
        canvas,
        canvasContext: ctx,
        viewport,
        transform: dpr === 1 ? undefined : [dpr, 0, 0, dpr, 0, 0],
      });
      await slot.task.promise;
      if (seq !== docSeq || rendered.get(n) !== slot) return;

      textDiv.replaceChildren();
      const lib = pdfjsLib;
      if (!lib) return;
      const layer = new lib.TextLayer({
        textContentSource: page.streamTextContent(),
        container: textDiv,
        viewport,
      });
      slot.layer = layer;
      await layer.render();
      if (seq !== docSeq || rendered.get(n) !== slot) return;
      pageText.set(n, layer.textContentItemsStr.join(' '));
      if (findQuery) refreshFindRects();
    } catch (err: any) {
      // Cancellation is the normal outcome of scrolling or zooming fast, and
      // is not worth a console line; anything else is.
      const msg = errMsg(err);
      const cancelled = err?.name === 'RenderingCancelledException' || /cancel/i.test(msg);
      if (!cancelled) console.warn(`[pdf] page ${n}:`, msg);
      abort(n, slot);
    }
  }

  /** Drop a render slot we still own and ask the effect to reconsider the page. */
  function abort(n: number, slot: unknown) {
    if (rendered.get(n) !== slot) return;
    rendered.delete(n);
    queueMicrotask(() => renderTick++);
  }

  // ------------------------------------------------------- visibility + zoom

  /**
   * Which pages are on screen, and which one the indicator names. Computed from
   * scroll geometry rather than an IntersectionObserver: the observer's first
   * delivery lands before the pages have their final heights and reports the
   * whole document as intersecting, and nothing ever retracts that — a 60-page
   * deck then holds 60 canvases. Geometry is deterministic and re-derivable at
   * any moment, which is what a virtualiser needs.
   */
  const OVERSCAN = 150;

  function recomputeVisible() {
    const el = scrollEl;
    if (!el || !numPages) return;
    const top = el.scrollTop - OVERSCAN;
    const bottom = el.scrollTop + el.clientHeight + OVERSCAN;
    const mid = el.scrollTop + el.clientHeight * 0.35;
    const arr: number[] = [];
    let cur = 1;
    for (const n of pageNums) {
      const p = pageEl(n);
      if (!p) continue;
      const t = p.offsetTop;
      if (t > bottom) break;
      if (t + p.offsetHeight >= top) arr.push(n);
      if (t <= mid) cur = n;
    }
    if (arr.join() !== visible.join()) visible = arr;
    if (cur !== currentPage) {
      currentPage = cur;
      pageInput = String(cur);
    }
  }

  // Re-derive after every relayout: a new document, a zoom, or a page whose
  // real size replaced the page-1 placeholder.
  $effect(() => {
    void numPages;
    void scale;
    void sizes;
    if (scrollEl) queueMicrotask(recomputeVisible);
  });

  function applyFitWidth() {
    if (!fitWidth || !scrollEl) return;
    const avail = scrollEl.clientWidth - PAGE_GAP * 2 - 2;
    if (avail <= 0) return;
    const next = clampScale(avail / baseSize.w);
    if (Math.abs(next - scale) > 0.005) scale = next;
  }

  $effect(() => {
    const el = scrollEl;
    if (!el) return;
    const ro = new ResizeObserver(() => {
      applyFitWidth();
      recomputeVisible();
    });
    ro.observe(el);
    return () => ro.disconnect();
  });

  // Re-fit when the mode is switched back on or the document changes.
  $effect(() => {
    if (fitWidth && baseSize.w) applyFitWidth();
  });

  function clampScale(v: number) {
    return Math.min(MAX_SCALE, Math.max(MIN_SCALE, v));
  }

  /** Zoom around the scroll container's centre so the reader keeps their place. */
  function setScale(next: number) {
    const el = scrollEl;
    const v = clampScale(next);
    if (v === scale) return;
    if (!el) {
      scale = v;
      return;
    }
    const ratio = v / scale;
    const midY = el.scrollTop + el.clientHeight / 2;
    const midX = el.scrollLeft + el.clientWidth / 2;
    scale = v;
    fitWidth = false;
    requestAnimationFrame(() => {
      el.scrollTop = midY * ratio - el.clientHeight / 2;
      el.scrollLeft = midX * ratio - el.clientWidth / 2;
    });
  }

  function onWheel(e: WheelEvent) {
    if (!e.ctrlKey && !e.metaKey) return;
    e.preventDefault();
    setScale(scale * (e.deltaY < 0 ? 1.1 : 1 / 1.1));
  }

  function onScroll() {
    recomputeVisible();
    popover = null;
  }

  function scrollToPage(n: number, frac = 0) {
    const el = scrollEl;
    const p = pageEl(n);
    if (!el || !p) return;
    // Instant, not smooth: a smooth scroll depends on animation frames, and a
    // backgrounded WebView gets none — the jump would silently not happen.
    el.scrollTo({ top: Math.max(0, p.offsetTop - PAGE_GAP + frac * p.offsetHeight - 40) });
    // A programmatic scroll fires `scroll` asynchronously; do not wait for it.
    recomputeVisible();
  }

  function jumpPage() {
    const n = Number(pageInput);
    if (Number.isFinite(n) && n >= 1 && n <= numPages) scrollToPage(n);
    else pageInput = String(currentPage);
  }

  // ------------------------------------------------------ selection geometry

  function clamp01(v: number) {
    return Math.min(1, Math.max(0, v));
  }

  /**
   * Merge rectangles that share a text line and touch horizontally. The browser
   * hands back one rect per text run, and overlapping translucent boxes
   * multiply into visibly darker seams.
   */
  function mergeRects(rects: Rect[]): Rect[] {
    const sorted = [...rects].sort((a, b) => a.Y - b.Y || a.X - b.X);
    const out: Rect[] = [];
    for (const r of sorted) {
      const prev = out[out.length - 1];
      const sameLine = prev && Math.abs(prev.Y - r.Y) < r.H * 0.5 && Math.abs(prev.H - r.H) < r.H * 0.5;
      if (sameLine && r.X <= prev.X + prev.W + 0.004) {
        const right = Math.max(prev.X + prev.W, r.X + r.W);
        prev.X = Math.min(prev.X, r.X);
        prev.W = right - prev.X;
        prev.Y = Math.min(prev.Y, r.Y);
        prev.H = Math.max(prev.H, r.H);
        continue;
      }
      out.push({ ...r });
    }
    return out;
  }

  /** Normalise a client rect against a page element, dropping empty slivers. */
  function normalise(cr: DOMRect, pr: DOMRect): Rect | null {
    if (cr.width < 0.5 || cr.height < 0.5 || pr.width <= 0 || pr.height <= 0) return null;
    const l = clamp01((cr.left - pr.left) / pr.width);
    const t = clamp01((cr.top - pr.top) / pr.height);
    const r = clamp01((cr.right - pr.left) / pr.width);
    const b = clamp01((cr.bottom - pr.top) / pr.height);
    if (r - l <= 0.0005 || b - t <= 0.0005) return null;
    return { X: l, Y: t, W: r - l, H: b - t };
  }

  /**
   * Split the current selection into one {page, rects, text} group per page it
   * touches. The range is clamped to each page's text layer so a selection
   * dragged across a page break yields the right quote on both sides.
   */
  function selectionParts(): { page: number; rects: Rect[]; text: string }[] {
    const sel = window.getSelection();
    if (!sel || sel.isCollapsed || sel.rangeCount === 0) return [];
    const range = sel.getRangeAt(0);
    const out: { page: number; rects: Rect[]; text: string }[] = [];

    for (const n of pageNums) {
      const el = pageEl(n);
      const layer = textEl(n);
      if (!el || !layer || !layer.firstChild) continue;
      if (!range.intersectsNode(layer)) continue;

      const r = range.cloneRange();
      if (!layer.contains(r.startContainer)) r.setStart(layer, 0);
      if (!layer.contains(r.endContainer)) r.setEnd(layer, layer.childNodes.length);
      const text = r.toString().replace(/\s+/g, ' ').trim();

      const pr = el.getBoundingClientRect();
      const rects: Rect[] = [];
      for (const cr of Array.from(r.getClientRects())) {
        const norm = normalise(cr, pr);
        if (norm) rects.push(norm);
      }
      if (rects.length) out.push({ page: n, rects: mergeRects(rects), text });
    }
    return out;
  }

  function selectionText(): string {
    return (window.getSelection()?.toString() ?? '').replace(/\s+/g, ' ').trim();
  }

  async function highlightSelection(color = lastColor) {
    const parts = selectionParts();
    window.getSelection()?.removeAllRanges();
    if (!parts.length) return;
    lastColor = color;
    lsSet(COLOR_KEY, color);
    try {
      const saved: Highlight[] = [];
      for (const p of parts) {
        saved.push(
          await api.saveHighlight({
            ID: 0,
            FileID: fileID,
            Page: p.page,
            Rects: p.rects,
            Text: p.text,
            Color: color,
            Note: '',
            CreatedAt: '',
            UpdatedAt: '',
          }),
        );
      }
      highlights = [...highlights, ...saved].sort((a, b) => a.Page - b.Page || a.ID - b.ID);
    } catch (err) {
      toast(`Could not save the highlight: ${errMsg(err)}`, 'error');
    }
  }

  // ------------------------------------------------- right press-and-hold drag

  /**
   * Right-button drag paints a selection. The browser will not extend a
   * selection for a non-primary button, so we build the Range ourselves from
   * the caret position under the pointer; the context menu that Windows fires
   * on release is suppressed for that one event.
   */
  let rightDrag: { anchor: Range; x: number; y: number; moved: boolean } | null = null;
  let suppressMenu = false;

  function caretRange(x: number, y: number): Range | null {
    const d = document as any;
    if (typeof d.caretRangeFromPoint === 'function') return d.caretRangeFromPoint(x, y);
    if (typeof d.caretPositionFromPoint === 'function') {
      const p = d.caretPositionFromPoint(x, y);
      if (!p) return null;
      const r = document.createRange();
      r.setStart(p.offsetNode, p.offset);
      r.collapse(true);
      return r;
    }
    return null;
  }

  function onPointerDown(e: PointerEvent) {
    popover = null;
    if (e.button !== 2) return;
    const anchor = caretRange(e.clientX, e.clientY);
    if (!anchor) return;
    rightDrag = { anchor, x: e.clientX, y: e.clientY, moved: false };
    // Stop the WebView clearing the selection / starting its own drag.
    e.preventDefault();
  }

  function onWinPointerMove(e: PointerEvent) {
    if (!rightDrag) return;
    if (Math.abs(e.clientX - rightDrag.x) + Math.abs(e.clientY - rightDrag.y) > 4) rightDrag.moved = true;
    if (!rightDrag.moved) return;
    const focus = caretRange(e.clientX, e.clientY);
    if (!focus) return;
    const a = rightDrag.anchor;
    const range = document.createRange();
    try {
      if (a.compareBoundaryPoints(Range.START_TO_START, focus) <= 0) {
        range.setStart(a.startContainer, a.startOffset);
        range.setEnd(focus.startContainer, focus.startOffset);
      } else {
        range.setStart(focus.startContainer, focus.startOffset);
        range.setEnd(a.startContainer, a.startOffset);
      }
    } catch {
      return; // caret landed in a detached node mid-render
    }
    const sel = window.getSelection();
    sel?.removeAllRanges();
    sel?.addRange(range);
    e.preventDefault();
  }

  function onWinPointerUp() {
    if (!rightDrag) return;
    const moved = rightDrag.moved;
    rightDrag = null;
    if (!moved) return;
    // The contextmenu event Windows fires next belongs to this drag.
    suppressMenu = true;
    void highlightSelection();
  }

  function onContextMenu(e: MouseEvent) {
    e.preventDefault();
    if (suppressMenu) {
      suppressMenu = false;
      return;
    }
    const text = selectionText();
    const items: MenuItem[] = [
      {
        label: 'Copy',
        icon: 'copy',
        disabled: !text,
        run: () => void navigator.clipboard.writeText(text).then(() => toast('Copied', 'success')),
      },
      { label: 'Highlight selection', icon: 'highlighter', disabled: !text, run: () => void highlightSelection() },
      { label: 'Ask Claude', icon: 'chat', disabled: !text, run: () => ask(text) },
    ];
    menu = { x: e.clientX, y: e.clientY, items };
  }

  function onPointerUp(e: PointerEvent) {
    if (e.button !== 0 || !highlighterOn) return;
    if (selectionText()) void highlightSelection();
  }

  function ask(quote: string) {
    askChat({ fileID, paperID: '', name }, `About this passage from ${name}:\n\n> ${quote}\n\n`);
  }

  // -------------------------------------------------------- highlight editing

  function onPageClick(e: MouseEvent, n: number) {
    if (selectionText()) return; // finishing a drag, not clicking a mark
    const el = pageEl(n);
    if (!el) return;
    const pr = el.getBoundingClientRect();
    const x = (e.clientX - pr.left) / pr.width;
    const y = (e.clientY - pr.top) / pr.height;
    const hit = [...hlFor(n)]
      .reverse()
      .find((h) => h.Rects.some((r) => x >= r.X && x <= r.X + r.W && y >= r.Y && y <= r.Y + r.H));
    popover = hit ? { id: hit.ID, x: e.clientX, y: e.clientY } : null;
  }

  async function patch(h: Highlight, changes: Partial<Highlight>) {
    const next = { ...h, ...changes };
    highlights = highlights.map((x) => (x.ID === h.ID ? next : x));
    try {
      const saved = await api.saveHighlight(next);
      highlights = highlights.map((x) => (x.ID === saved.ID ? saved : x));
    } catch (err) {
      toast(`Could not update the highlight: ${errMsg(err)}`, 'error');
      void loadHighlights();
    }
  }

  async function remove(h: Highlight) {
    popover = null;
    highlights = highlights.filter((x) => x.ID !== h.ID);
    try {
      await api.deleteHighlight(h.ID);
    } catch (err) {
      toast(`Could not delete the highlight: ${errMsg(err)}`, 'error');
      void loadHighlights();
    }
  }

  /**
   * Notes save as you type (debounced), not on blur: any click outside the
   * popover closes it, which unmounts the textarea before a change event
   * would ever fire — typed notes were silently lost.
   */
  let noteTimer: ReturnType<typeof setTimeout> | null = null;
  let pendingNote: { h: Highlight; text: string } | null = null;

  function flushNote() {
    if (noteTimer) clearTimeout(noteTimer);
    noteTimer = null;
    const p = pendingNote;
    pendingNote = null;
    if (p) void patch(p.h, { Note: p.text });
  }

  function noteInput(h: Highlight, text: string) {
    // Optimistic, so the rail and a re-opened popover show it immediately.
    highlights = highlights.map((x) => (x.ID === h.ID ? { ...x, Note: text } : x));
    pendingNote = { h, text };
    if (noteTimer) clearTimeout(noteTimer);
    noteTimer = setTimeout(flushNote, 400);
  }

  $effect(() => () => flushNote());

  function recolour(h: Highlight, color: string) {
    lastColor = color;
    lsSet(COLOR_KEY, color);
    void patch(h, { Color: color });
  }

  function goTo(h: Highlight) {
    popover = null;
    scrollToPage(h.Page, h.Rects[0]?.Y ?? 0);
  }

  // ------------------------------------------------------------------- find

  async function ensurePageText() {
    const d = doc;
    if (!d) return;
    const seq = docSeq;
    for (const n of pageNums) {
      if (pageText.has(n)) continue;
      try {
        const page = await d.getPage(n);
        const tc = await page.getTextContent();
        if (seq !== docSeq) return;
        pageText.set(n, tc.items.map((i: any) => i.str ?? '').join(' '));
      } catch {
        pageText.set(n, '');
      }
    }
  }

  async function runFind() {
    const q = findQuery.trim();
    findIndex = 0;
    if (q.length < 2) {
      findMatches = [];
      findRects = {};
      return;
    }
    await ensurePageText();
    const needle = q.toLowerCase();
    const out: { page: number; at: number }[] = [];
    for (const n of pageNums) {
      const hay = (pageText.get(n) ?? '').toLowerCase();
      let i = hay.indexOf(needle);
      while (i >= 0 && out.length < 500) {
        out.push({ page: n, at: i });
        i = hay.indexOf(needle, i + needle.length);
      }
    }
    findMatches = out;
    refreshFindRects();
    if (out.length) scrollToPage(out[0].page);
  }

  /** Match boxes for the pages that currently have a text layer. */
  function refreshFindRects() {
    const q = findQuery.trim().toLowerCase();
    if (q.length < 2) {
      findRects = {};
      return;
    }
    const next: Record<number, Rect[]> = {};
    for (const n of pageNums) {
      const el = pageEl(n);
      const layer = textEl(n);
      if (!el || !layer?.firstChild) continue;

      const walker = document.createTreeWalker(layer, NodeFilter.SHOW_TEXT);
      const nodes: { node: Node; start: number }[] = [];
      let full = '';
      while (walker.nextNode()) {
        const node = walker.currentNode;
        nodes.push({ node, start: full.length });
        full += node.nodeValue ?? '';
      }
      const hay = full.toLowerCase();
      const pr = el.getBoundingClientRect();
      const rects: Rect[] = [];
      let i = hay.indexOf(q);
      while (i >= 0 && rects.length < 400) {
        const range = rangeAt(nodes, i, i + q.length);
        if (range) {
          for (const cr of Array.from(range.getClientRects())) {
            const norm = normalise(cr, pr);
            if (norm) rects.push(norm);
          }
        }
        i = hay.indexOf(q, i + q.length);
      }
      if (rects.length) next[n] = mergeRects(rects);
    }
    findRects = next;
  }

  /** Build a Range covering [from,to) of the concatenated text nodes. */
  function rangeAt(nodes: { node: Node; start: number }[], from: number, to: number): Range | null {
    let startNode: Node | null = null;
    let startOff = 0;
    let endNode: Node | null = null;
    let endOff = 0;
    for (const { node, start } of nodes) {
      const len = (node.nodeValue ?? '').length;
      if (!startNode && from >= start && from < start + len) {
        startNode = node;
        startOff = from - start;
      }
      if (to > start && to <= start + len) {
        endNode = node;
        endOff = to - start;
      }
    }
    if (!startNode || !endNode) return null;
    try {
      const r = document.createRange();
      r.setStart(startNode, startOff);
      r.setEnd(endNode, endOff);
      return r;
    } catch {
      return null;
    }
  }

  function stepFind(delta: number) {
    if (!findMatches.length) return;
    findIndex = (findIndex + delta + findMatches.length) % findMatches.length;
    scrollToPage(findMatches[findIndex].page);
  }

  function closeFind() {
    findOpen = false;
    findQuery = '';
    findMatches = [];
    findRects = {};
  }

  // Keep match boxes glued to the text through zoom and lazy page renders.
  $effect(() => {
    void scale;
    void wanted;
    if (findQuery.trim().length >= 2) queueMicrotask(refreshFindRects);
  });

  // -------------------------------------------------------------- exporting

  async function exportMarkdown() {
    try {
      const md = await api.exportHighlights(fileID);
      await navigator.clipboard.writeText(md).catch(() => {});
      const base = name.replace(/\.[^.]+$/, '') || 'highlights';
      const path = await api.saveTextFile(`${base} — highlights.md`, md);
      toast(path ? `Copied and saved to ${path}` : 'Copied to the clipboard', 'success');
    } catch (err) {
      toast(`Export failed: ${errMsg(err)}`, 'error');
    }
  }

  // -------------------------------------------------------------- keyboard

  function onWinKey(e: KeyboardEvent) {
    if (!rootEl) return;
    const target = e.target as HTMLElement | null;
    // `contains` throws on anything that is not a Node, and a key event
    // dispatched at the window has `window` as its target.
    const inside = target instanceof Node && rootEl.contains(target);
    const typing = !!target && (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName));

    if ((e.ctrlKey || e.metaKey) && (e.key === 'f' || e.key === 'F') && inside) {
      e.preventDefault();
      findOpen = true;
      queueMicrotask(() => findInput?.select());
      return;
    }
    if (!inside || typing) return;

    if (e.key === 'PageDown' || e.key === 'PageUp') {
      e.preventDefault();
      scrollToPage(Math.min(numPages, Math.max(1, currentPage + (e.key === 'PageDown' ? 1 : -1))));
      return;
    }
    if ((e.ctrlKey || e.metaKey) && (e.key === '+' || e.key === '=')) {
      e.preventDefault();
      setScale(scale + 0.15);
    } else if ((e.ctrlKey || e.metaKey) && e.key === '-') {
      e.preventDefault();
      setScale(scale - 0.15);
    } else if ((e.ctrlKey || e.metaKey) && e.key === '0') {
      e.preventDefault();
      fitWidth = true;
      applyFitWidth();
    }
  }

  function onFindKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      closeFind();
    } else if (e.key === 'Enter') {
      e.preventDefault();
      if (findMatches.length) stepFind(e.shiftKey ? -1 : 1);
      else void runFind();
    }
  }

  $effect(() => {
    lsSet(RAIL_KEY, railOpen);
  });
</script>

<svelte:window onkeydown={onWinKey} onpointermove={onWinPointerMove} onpointerup={onWinPointerUp} />

<div class="pdfv" bind:this={rootEl}>
  <div class="ptools">
    <div class="grp">
      <button class="icon-btn" onclick={() => setScale(scale - 0.15)} title="Zoom out (Ctrl+-)" aria-label="Zoom out">
        <Icon name="zoomOut" size={13} />
      </button>
      <span class="zoom">{Math.round(scale * 100)}%</span>
      <button class="icon-btn" onclick={() => setScale(scale + 0.15)} title="Zoom in (Ctrl++)" aria-label="Zoom in">
        <Icon name="zoomIn" size={13} />
      </button>
      <button
        class="btn sm"
        class:on={fitWidth}
        onclick={() => {
          fitWidth = true;
          applyFitWidth();
        }}
        title="Fit the page width (Ctrl+0)">Fit</button
      >
    </div>

    <div class="grp pager">
      <input
        class="input pageno"
        value={pageInput}
        oninput={(e) => (pageInput = e.currentTarget.value)}
        onkeydown={(e) => e.key === 'Enter' && jumpPage()}
        onblur={jumpPage}
        aria-label="Page number"
      />
      <span class="of">/ {numPages || '—'}</span>
    </div>

    <div class="spacer"></div>

    <div class="grp">
      <button
        class="icon-btn"
        class:on={highlighterOn}
        aria-pressed={highlighterOn}
        onclick={() => (highlighterOn = !highlighterOn)}
        title={highlighterOn
          ? 'Highlighter on — left-drag marks text'
          : 'Highlighter: left-drag marks text (right-drag always does)'}
        aria-label="Highlighter"
      >
        <Icon name="highlighter" size={13} />
      </button>
      {#each COLOR_NAMES as c (c)}
        <button
          class="swatch"
          class:sel={lastColor === c}
          style="background:{COLORS[c]}"
          onclick={() => {
            lastColor = c;
            lsSet(COLOR_KEY, c);
          }}
          title="{c[0].toUpperCase()}{c.slice(1)} highlights"
          aria-label="{c} highlights"
          aria-pressed={lastColor === c}
        ></button>
      {/each}
      <button
        class="icon-btn"
        onclick={() => {
          findOpen = !findOpen;
          if (findOpen) queueMicrotask(() => findInput?.focus());
          else closeFind();
        }}
        aria-pressed={findOpen}
        title="Find in document (Ctrl+F)"
        aria-label="Find"
      >
        <Icon name="search" size={13} />
      </button>
      <button
        class="icon-btn"
        class:on={railOpen}
        onclick={() => (railOpen = !railOpen)}
        aria-pressed={railOpen}
        title="Highlights ({highlights.length})"
        aria-label="Highlights"
      >
        <Icon name="quote" size={13} />
      </button>
    </div>
  </div>

  {#if findOpen}
    <div class="findbar">
      <Icon name="search" size={12} />
      <input
        class="input"
        bind:this={findInput}
        bind:value={findQuery}
        oninput={() => void runFind()}
        onkeydown={onFindKey}
        placeholder="Find in document…"
        aria-label="Find in document"
      />
      <span class="count">
        {#if findQuery.trim().length < 2}
          type 2+ characters
        {:else}
          {findMatches.length ? `${findIndex + 1} / ${findMatches.length}` : 'no matches'}
        {/if}
      </span>
      <button class="icon-btn" onclick={() => stepFind(-1)} disabled={!findMatches.length} aria-label="Previous match">
        <Icon name="chevronUp" size={13} />
      </button>
      <button class="icon-btn" onclick={() => stepFind(1)} disabled={!findMatches.length} aria-label="Next match">
        <Icon name="chevronDown" size={13} />
      </button>
      <button class="icon-btn" onclick={closeFind} aria-label="Close find"><Icon name="x" size={13} /></button>
    </div>
  {/if}

  <div class="pmain">
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="pscroll"
      class:marker={highlighterOn}
      bind:this={scrollEl}
      onscroll={onScroll}
      onwheel={onWheel}
      onpointerdown={onPointerDown}
      onpointerup={onPointerUp}
      oncontextmenu={onContextMenu}
    >
      {#if error}
        <div class="state err"><Icon name="alert" size={18} /><p>{error}</p></div>
      {:else if loading}
        <div class="state"><span class="spinner"></span> Loading PDF…</div>
      {:else}
        {#each pageNums as n (n)}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <div
            class="page"
            data-page={n}
            style="width:{sizeOf(n).w * scale}px; height:{sizeOf(n).h * scale}px; --total-scale-factor:{scale}"
            onclick={(e) => onPageClick(e, n)}
          >
            <canvas></canvas>
            <div class="hl">
              {#each hlFor(n) as h (h.ID)}
                {#each h.Rects as r, i (i)}
                  <div
                    class="hlrect"
                    class:active={popover?.id === h.ID}
                    style="left:{r.X * 100}%; top:{r.Y * 100}%; width:{r.W * 100}%; height:{r.H * 100}%;
                           background:{COLORS[h.Color] ?? COLORS.yellow}"
                  ></div>
                {/each}
              {/each}
              {#each findRects[n] ?? [] as r, i (i)}
                <div
                  class="findrect"
                  style="left:{r.X * 100}%; top:{r.Y * 100}%; width:{r.W * 100}%; height:{r.H * 100}%"
                ></div>
              {/each}
            </div>
            <div class="textLayer"></div>
            <span class="pnum">{n}</span>
          </div>
        {/each}
      {/if}
    </div>

    {#if railOpen}
      <aside class="prail" aria-label="Highlights">
        <header>
          <strong>Highlights</strong>
          <span class="n">{highlights.length}</span>
          <button class="btn sm" onclick={() => void exportMarkdown()} disabled={!highlights.length}>
            <Icon name="download" size={11} /> Export
          </button>
        </header>
        {#if !highlights.length}
          <p class="empty">Right-drag over text to highlight it. Marks are saved with the file.</p>
        {:else}
          <ul>
            {#each highlights as h (h.ID)}
              <li>
                <button class="hitem" onclick={() => goTo(h)}>
                  <span class="meta">
                    <span class="cdot" style="background:{COLORS[h.Color] ?? COLORS.yellow}"></span>
                    p.{h.Page}
                  </span>
                  <span class="snip">{h.Text || '(no text)'}</span>
                  {#if h.Note}<span class="note">{h.Note}</span>{/if}
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </aside>
    {/if}
  </div>
</div>

{#if activeHighlight && popover}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="pop"
    style="left:{Math.min(popover.x, window.innerWidth - 280)}px; top:{Math.min(
      popover.y + 8,
      window.innerHeight - 220,
    )}px"
    onpointerdown={(e) => e.stopPropagation()}
    oncontextmenu={(e) => e.stopPropagation()}
  >
    <div class="swatches">
      {#each COLOR_NAMES as c (c)}
        <button
          class="swatch"
          class:sel={activeHighlight.Color === c}
          style="background:{COLORS[c]}"
          onclick={() => recolour(activeHighlight, c)}
          aria-label="Colour {c}"
        ></button>
      {/each}
      <div class="spacer"></div>
      <button class="icon-btn" onclick={() => (popover = null)} aria-label="Close"><Icon name="x" size={12} /></button>
    </div>
    <p class="quote">{activeHighlight.Text || '(no text captured)'}</p>
    <textarea
      class="input box"
      rows="2"
      placeholder="Add a note…"
      value={activeHighlight.Note}
      oninput={(e) => noteInput(activeHighlight, e.currentTarget.value)}
      onchange={flushNote}
    ></textarea>
    <div class="popactions">
      <button
        class="btn sm"
        onclick={() => void navigator.clipboard.writeText(activeHighlight.Text).then(() => toast('Copied', 'success'))}
      >
        <Icon name="copy" size={11} /> Copy
      </button>
      <button class="btn sm" onclick={() => ask(activeHighlight.Text)}><Icon name="chat" size={11} /> Ask Claude</button>
      <button class="btn sm danger" onclick={() => void remove(activeHighlight)} aria-label="Delete highlight">
        <Icon name="trash" size={11} />
      </button>
    </div>
  </div>
{/if}

{#if menu}
  <ContextMenu x={menu.x} y={menu.y} items={menu.items} onClose={() => (menu = null)} />
{/if}

<style>
  .pdfv {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
    background: var(--bg-subtle);
  }

  .ptools {
    flex: none;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 5px 8px;
    border-bottom: 1px solid var(--border);
    background: var(--bg-elevated);
  }

  .grp {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .spacer {
    flex: 1;
  }

  .zoom {
    min-width: 42px;
    text-align: center;
    font-size: 11.5px;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
  }

  .pageno {
    width: 46px;
    text-align: center;
    padding: 2px 4px;
    font-size: 11.5px;
    font-variant-numeric: tabular-nums;
  }

  .of {
    font-size: 11.5px;
    color: var(--text-faint);
  }

  .icon-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    border-radius: var(--radius-sm);
    color: var(--text-faint);
    transition: background var(--t), color var(--t);
  }

  .icon-btn:hover:not(:disabled) {
    background: var(--bg-hover);
    color: var(--text);
  }

  .icon-btn:disabled {
    opacity: 0.4;
    cursor: default;
  }

  .icon-btn.on,
  :global(.pdfv .btn.sm.on) {
    background: var(--accent-soft);
    color: var(--accent-text);
  }

  .swatch {
    width: 15px;
    height: 15px;
    border-radius: 50%;
    border: 1px solid rgb(0 0 0 / 0.25);
    flex: none;
  }

  .swatch.sel {
    outline: 2px solid var(--accent);
    outline-offset: 1px;
  }

  .findbar {
    flex: none;
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 5px 8px;
    border-bottom: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-faint);
  }

  .findbar .input {
    flex: 1;
    min-width: 0;
    padding: 3px 7px;
    font-size: 12px;
  }

  .count {
    font-size: 11px;
    color: var(--text-faint);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .pmain {
    flex: 1;
    min-height: 0;
    display: flex;
  }

  .pscroll {
    flex: 1;
    min-width: 0;
    overflow: auto;
    /* Must be the pages' offsetParent: scrollToPage and the page indicator both
       read `.page.offsetTop`, and without this they measure from whatever
       positioned ancestor the host happens to have (focus mode is fixed), so
       every jump lands at the top of the document. */
    position: relative;
    padding: 12px;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    /* Ctrl+wheel zoom must not also pinch-zoom the WebView. */
    overscroll-behavior: contain;
  }

  .pscroll.marker {
    cursor: crosshair;
  }

  .state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 9px;
    min-height: 180px;
    padding: 28px 20px;
    color: var(--text-faint);
    font-size: 12.5px;
    text-align: center;
  }

  .page {
    position: relative;
    flex: none;
    background: #fff;
    box-shadow: 0 1px 4px rgb(0 0 0 / 0.22);
  }

  .page canvas {
    display: block;
    width: 100%;
    height: 100%;
  }

  .pnum {
    position: absolute;
    right: 4px;
    bottom: 2px;
    font-size: 9px;
    color: rgb(0 0 0 / 0.32);
    pointer-events: none;
  }

  /* Marks sit under the text layer and multiply into the glyphs, the way a
     real highlighter does. Hit-testing happens in JS, so they never steal a
     click from the selectable text above them. */
  .hl {
    position: absolute;
    inset: 0;
    z-index: 1;
    pointer-events: none;
    mix-blend-mode: multiply;
  }

  .hlrect {
    position: absolute;
    border-radius: 1px;
    opacity: 0.62;
  }

  .hlrect.active {
    outline: 1px solid rgb(0 0 0 / 0.35);
  }

  .findrect {
    position: absolute;
    background: #ff9f1c;
    opacity: 0.5;
    border-radius: 1px;
  }

  /* ------------------------------------------------------------ text layer
     Trimmed from pdfjs-dist/web/pdf_viewer.css (260 KB, almost all of it for
     the annotation and editor layers we do not use). `--total-scale-factor` is
     set on .page; pdf.js positions spans in percentages and sizes them from
     --font-height, so this is scale-independent. */
  .textLayer {
    position: absolute;
    inset: 0;
    z-index: 2;
    overflow: clip;
    opacity: 1;
    line-height: 1;
    text-align: initial;
    text-size-adjust: none;
    forced-color-adjust: none;
    transform-origin: 0 0;
    caret-color: CanvasText;
    --min-font-size: 1;
    --text-scale-factor: calc(var(--total-scale-factor) * var(--min-font-size));
    --min-font-size-inv: calc(1 / var(--min-font-size));
  }

  .textLayer :global(span),
  .textLayer :global(br) {
    color: transparent;
    position: absolute;
    white-space: pre;
    cursor: text;
    transform-origin: 0% 0%;
  }

  .textLayer :global(> :not(.markedContent)),
  .textLayer :global(.markedContent span:not(.markedContent)) {
    z-index: 1;
    --font-height: 0;
    font-size: calc(var(--text-scale-factor) * var(--font-height));
    --scale-x: 1;
    --rotate: 0deg;
    transform: rotate(var(--rotate)) scaleX(var(--scale-x)) scale(var(--min-font-size-inv));
  }

  .textLayer :global(.markedContent) {
    display: contents;
  }

  .textLayer :global(span[role='img']) {
    user-select: none;
    cursor: default;
  }

  .textLayer :global(::selection) {
    background: rgb(50 120 255 / 0.3);
  }

  /* ------------------------------------------------------------------ rail */

  .prail {
    flex: none;
    width: 250px;
    border-left: 1px solid var(--border);
    background: var(--bg-elevated);
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  .prail header {
    flex: none;
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 7px 8px;
    border-bottom: 1px solid var(--border);
    font-size: 12px;
  }

  .prail .n {
    flex: 1;
    color: var(--text-faint);
    font-size: 11px;
  }

  .prail .empty {
    margin: 0;
    padding: 14px 12px;
    font-size: 11.5px;
    color: var(--text-faint);
    line-height: 1.5;
  }

  .prail ul {
    flex: 1;
    min-height: 0;
    overflow: auto;
    margin: 0;
    padding: 4px;
    list-style: none;
  }

  .hitem {
    display: flex;
    flex-direction: column;
    gap: 2px;
    width: 100%;
    text-align: left;
    padding: 6px 7px;
    border-radius: var(--radius-sm);
  }

  .hitem:hover {
    background: var(--bg-hover);
  }

  .meta {
    display: flex;
    align-items: center;
    gap: 5px;
    font-size: 10.5px;
    color: var(--text-faint);
  }

  .cdot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    border: 1px solid rgb(0 0 0 / 0.25);
    flex: none;
  }

  .snip {
    font-size: 11.5px;
    color: var(--text);
    line-height: 1.45;
    display: -webkit-box;
    -webkit-line-clamp: 3;
    line-clamp: 3;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .note {
    font-size: 11px;
    color: var(--accent-text);
    line-height: 1.4;
  }

  /* --------------------------------------------------------------- popover */

  .pop {
    position: fixed;
    z-index: 60;
    width: 264px;
    display: flex;
    flex-direction: column;
    gap: 7px;
    padding: 8px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-elevated);
    box-shadow: 0 8px 26px rgb(0 0 0 / 0.28);
  }

  .swatches {
    display: flex;
    align-items: center;
    gap: 5px;
  }

  .quote {
    margin: 0;
    max-height: 66px;
    overflow: auto;
    font-size: 11.5px;
    line-height: 1.45;
    color: var(--text-muted);
    border-left: 2px solid var(--border);
    padding-left: 7px;
  }

  .popactions {
    display: flex;
    gap: 5px;
  }
</style>
