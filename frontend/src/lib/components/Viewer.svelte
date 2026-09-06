<script lang="ts">
  /**
   * In-app file preview.
   *
   * Everything renders inside the WebView — no external process. Bytes come
   * from the Go asset server registered in `assets.go`:
   *   /local/{id}            for anything with a Canvas file id
   *   /local/path?p=<abs>    fallback, allowed only under SyncDir
   *
   * PDFs render in PdfViewer.svelte (PDF.js) by default, because the WebView's
   * built-in plugin renders into an opaque document with no reachable text
   * layer — no selection, no quoting, no highlighting. The plugin is still one
   * click away ("Built-in" / the Engine toggle, persisted in
   * `nussync.viewer.engine`); that path is a plain <iframe> with no sandbox
   * attribute, since sandboxing disables the plugin and yields a blank pane.
   * Everything textual is fetched and rendered by us rather than handed to the
   * WebView, so course HTML never executes in the app's own origin. Formats the
   * WebView cannot show at all (pptx/docx/xlsx) fall back to the extracted text
   * the indexer already holds, via GetFileText.
   */
  import { untrack } from 'svelte';
  import { api, errMsg } from '../api';
  import { courseByID, openChat, openFileNode, setContext, studyFile, toast } from '../stores';
  import type { FileNode } from '../types';
  import { ext, fmtBytes, fmtDateTime, lsGet, lsSet, markdownToHTML } from '../util';
  import { recordPreview } from '../quest';
  import { viewerFocus } from '../layout';
  import Icon from './Icon.svelte';
  import PdfViewer from './PdfViewer.svelte';

  interface Props {
    file: FileNode | null;
    /** Rendered as the pane's close button; omitted = no close button. */
    onClose?: () => void;
    /** Hide the Study button where it makes no sense (the Study view itself). */
    showStudy?: boolean;
    /** Focus-mode navigation. Hosts with a list pass these; others omit them. */
    onPrev?: () => void;
    onNext?: () => void;
    /** Focus-mode breadcrumb; falls back to the file's own path. */
    crumb?: string;
  }

  let { file, onClose, showStudy = true, onPrev, onNext, crumb = '' }: Props = $props();

  // -------------------------------------------------------------- focus mode

  /**
   * Focus lives in the layout store so the palette and the hosts can drive it,
   * but only a Viewer with something to show honours it — and leaving one
   * always drops it, so the next preview never opens already-expanded.
   */
  const focused = $derived($viewerFocus && !!file);

  $effect(() => () => viewerFocus.set(false));

  function toggleFocus() {
    if (file) viewerFocus.update((v) => !v);
  }

  function onWinKey(e: KeyboardEvent) {
    if (!file) return;
    const mod = e.ctrlKey || e.metaKey;
    if (mod && e.shiftKey && (e.key === 'p' || e.key === 'P')) {
      e.preventDefault();
      toggleFocus();
      return;
    }
    if (!focused) return;
    // A docked chat sits beside focus mode; its own Esc belongs to it.
    if ((e.target as HTMLElement | null)?.closest?.('aside.chat')) return;
    if (e.key === 'Escape') {
      // Swallow it so the host does not also close the pane underneath.
      e.preventDefault();
      e.stopPropagation();
      viewerFocus.set(false);
      return;
    }
    const el = e.target as HTMLElement | null;
    if (el && (el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName))) return;
    if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') {
      if (!onPrev) return;
      e.preventDefault();
      onPrev();
    } else if (e.key === 'ArrowRight' || e.key === 'ArrowDown') {
      if (!onNext) return;
      e.preventDefault();
      onNext();
    }
  }

  const crumbText = $derived(crumb || file?.RelPath || file?.Name || '');

  // ------------------------------------------------------------- pdf engine

  const ENGINE_KEY = 'nussync.viewer.engine';
  /** 'pdfjs' (default, highlights) | 'builtin' (WebView plugin, no text layer). */
  let engine = $state<'pdfjs' | 'builtin'>(lsGet<'pdfjs' | 'builtin'>(ENGINE_KEY, 'pdfjs'));

  function setEngine(v: 'pdfjs' | 'builtin') {
    engine = v;
    lsSet(ENGINE_KEY, v);
    // The iframe reports its own load; PDF.js manages its own spinner.
    if (v === 'builtin') loading = true;
    else loading = false;
  }

  /** How the preview is rendered. */
  type Mode = 'pdf' | 'image' | 'text' | 'markdown' | 'extracted' | 'none';

  const IMAGE_EXTS = new Set(['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp', 'bmp']);
  const TEXT_EXTS = new Set([
    'txt', 'log', 'csv', 'tsv', 'json', 'xml', 'yml', 'yaml', 'py', 'ts', 'tsx', 'js', 'jsx',
    'go', 'java', 'c', 'h', 'cpp', 'hpp', 'cs', 'rb', 'rs', 'php', 'sh', 'sql', 'r', 'm',
    'tex', 'css', 'html', 'htm', 'ipynb',
  ]);
  /** Office containers: no WebView renderer, but the indexer has their text. */
  const EXTRACT_EXTS = new Set(['pptx', 'docx', 'xlsx', 'ppt', 'doc', 'xls', 'odt', 'odp', 'ods', 'epub']);

  /** Text fetched straight from disk is capped so a stray 50 MB log cannot wedge the pane. */
  const MAX_TEXT = 400_000;

  function modeFor(f: FileNode | null): Mode {
    if (!f) return 'none';
    const e = ext(f.Name);
    if (e === 'pdf') return 'pdf';
    if (IMAGE_EXTS.has(e)) return 'image';
    if (e === 'md' || e === 'markdown') return 'markdown';
    if (TEXT_EXTS.has(e)) return 'text';
    if (EXTRACT_EXTS.has(e)) return 'extracted';
    return 'none';
  }

  /**
   * Asset-server URL for a node. Prefers the id route — note ids can be
   * negative (downloaded papers are numbered down from -1000), so only 0 means
   * "no id"; folders and synthetic nodes fall back to the path route.
   */
  function srcFor(f: FileNode): string {
    return f.ID !== 0 ? `/local/${f.ID}` : `/local/path?p=${encodeURIComponent(f.Path)}`;
  }

  /**
   * Quest xp for actually reading something: a file kept open for a full
   * minute is worth 2 xp, capped at ten a day by the quest store itself.
   */
  const READ_MS = 60_000;

  $effect(() => {
    const id = file?.ID ?? 0;
    if (!id) return;
    const h = setTimeout(() => recordPreview(), READ_MS);
    return () => clearTimeout(h);
  });

  const mode = $derived(modeFor(file));
  const course = $derived(file ? $courseByID.get(file.CourseID) : undefined);
  const src = $derived(file ? srcFor(file) : '');

  let loading = $state(false);
  let error = $state('');
  let text = $state('');
  let wrap = $state(true);
  let actualSize = $state(false);
  /** Bumped on every load so a stale in-flight fetch cannot overwrite the pane. */
  let loadSeq = 0;

  /**
   * Reload whenever the previewed file changes. Reads only `file` and `mode`,
   * so toggling `wrap`/`actualSize` never refetches.
   */
  $effect(() => {
    const f = file;
    const m = mode;
    const seq = ++loadSeq;

    error = '';
    text = '';
    actualSize = false;

    if (!f) {
      loading = false;
      return;
    }
    // The chat panel and Study both key off whatever is on screen here.
    setContext(f.ID, '', f.Name);

    if (!f.Synced) {
      loading = false;
      error = 'This file has not been downloaded yet — run a sync first.';
      return;
    }
    if (m === 'pdf') {
      // PdfViewer draws its own spinner; the built-in iframe reports onload.
      // untrack: flipping the engine must not re-run the whole load effect.
      loading = untrack(() => engine) === 'builtin';
      return;
    }
    if (m === 'image') {
      // The <img> loads it; its own handler flips `loading` off.
      loading = true;
      return;
    }
    if (m === 'none') {
      loading = false;
      return;
    }

    loading = true;
    void (async () => {
      try {
        let body: string;
        if (m === 'extracted') {
          body = await api.getFileText(f.ID, MAX_TEXT);
        } else {
          const res = await fetch(srcFor(f), { cache: 'no-store' });
          if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
          const raw = await res.text();
          body = raw.length > MAX_TEXT ? `${raw.slice(0, MAX_TEXT)}\n\n… truncated at ${MAX_TEXT.toLocaleString()} characters.` : raw;
        }
        if (seq !== loadSeq) return;
        text = body;
      } catch (err) {
        if (seq !== loadSeq) return;
        error = errMsg(err);
      } finally {
        if (seq === loadSeq) loading = false;
      }
    })();
  });

  function done() {
    loading = false;
  }

  function failed(what: string) {
    loading = false;
    error = `Could not render this ${what}.`;
  }

  function open() {
    if (file) void openFileNode(file);
  }

  async function reveal() {
    if (!file) return;
    try {
      await api.revealFile(file.Path);
    } catch (err) {
      toast(errMsg(err), 'error');
    }
  }

  function chat() {
    if (file) openChat({ fileID: file.ID, paperID: '', name: file.Name });
  }

  function study() {
    if (file) studyFile(file.ID);
  }

  const openLabel = $derived.by(() => {
    switch (ext(file?.Name ?? '')) {
      case 'pptx':
      case 'ppt':
        return 'PowerPoint';
      case 'docx':
      case 'doc':
        return 'Word';
      case 'xlsx':
      case 'xls':
        return 'Excel';
      default:
        return 'the default app';
    }
  });
</script>

<svelte:window onkeydown={onWinKey} />

<section class="viewer" class:focused aria-label="File preview">
  {#if focused}
    <div class="strip">
      <span class="strip-crumb truncate" title={crumbText}>{crumbText}</span>
      <div class="strip-nav">
        <button class="icon-btn" onclick={() => onPrev?.()} disabled={!onPrev} aria-label="Previous file">
          <Icon name="chevronLeft" size={14} />
        </button>
        <button class="icon-btn" onclick={() => onNext?.()} disabled={!onNext} aria-label="Next file">
          <Icon name="chevronRight" size={14} />
        </button>
      </div>
      <button class="btn sm" onclick={() => viewerFocus.set(false)} title="Leave focus mode (Esc)">
        <Icon name="shrink" size={12} /> Exit
      </button>
    </div>
  {/if}

  {#if !file}
    <div class="blank">
      <Icon name="eye" size={22} />
      <p>Select a file to preview it here.</p>
    </div>
  {:else}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <header class="vhead" ondblclick={toggleFocus}>
      <div class="vtitle">
        <span class="vname truncate" title={file.RelPath || file.Name}>{file.Name}</span>
        <span class="vmeta truncate">
          {#if course}<span class="dot" style="background:{course.Color}"></span>{course.Code} · {/if}
          {fmtBytes(file.Size)}{file.ModifiedAt ? ` · ${fmtDateTime(file.ModifiedAt)}` : ''}
        </span>
      </div>
      <div class="vactions">
        {#if mode === 'pdf'}
          <button
            class="btn sm"
            onclick={() => setEngine(engine === 'pdfjs' ? 'builtin' : 'pdfjs')}
            title={engine === 'pdfjs'
              ? "Switch to the WebView's built-in PDF viewer (no highlighting)"
              : 'Switch back to the reader with highlighting'}
          >
            {engine === 'pdfjs' ? 'Use built-in viewer' : 'Use reader'}
          </button>
        {:else if mode === 'image'}
          <button class="btn sm" onclick={() => (actualSize = !actualSize)}>{actualSize ? 'Fit' : '1:1'}</button>
        {:else if mode === 'text' || mode === 'extracted'}
          <button class="btn sm" onclick={() => (wrap = !wrap)}>{wrap ? 'No wrap' : 'Wrap'}</button>
        {/if}
        <button class="btn sm" onclick={open} disabled={!file.Synced} title="Open with the default application">
          <Icon name="external" size={12} /> Open
        </button>
        <button class="btn sm" onclick={() => void reveal()} disabled={!file.Synced} title="Show in Explorer">
          <Icon name="folderOpen" size={12} />
        </button>
        <button class="btn sm" onclick={chat} title="Chat about this file">
          <Icon name="chat" size={12} />
        </button>
        {#if showStudy}
          <button class="btn sm" onclick={study} title="Open in Study">
            <Icon name="layers" size={12} />
          </button>
        {/if}
        <button
          class="icon-btn"
          onclick={toggleFocus}
          aria-pressed={focused}
          title={focused ? 'Leave focus mode (Ctrl+Shift+P)' : 'Focus mode (Ctrl+Shift+P)'}
          aria-label={focused ? 'Leave focus mode' : 'Focus mode'}
        >
          <Icon name={focused ? 'shrink' : 'expand'} size={13} />
        </button>
        {#if onClose}
          <button class="icon-btn" onclick={onClose} aria-label="Close preview (Esc)"><Icon name="x" size={13} /></button>
        {/if}
      </div>
    </header>

    <div
      class="vbody"
      class:pad={mode !== 'pdf' && mode !== 'image'}
      class:nested={mode === 'pdf' && engine === 'pdfjs'}
    >
      {#if error}
        <div class="state err">
          <Icon name="alert" size={18} />
          <p>{error}</p>
          <div class="row">
            <button class="btn sm" onclick={open} disabled={!file.Synced}><Icon name="external" size={12} /> Open externally</button>
            <button class="btn sm" onclick={() => void reveal()} disabled={!file.Synced}>Reveal</button>
          </div>
        </div>
      {:else}
        {#if loading}
          <div class="state"><span class="spinner"></span> Loading preview…</div>
        {/if}

        {#if mode === 'pdf'}
          {#if engine === 'pdfjs'}
            {#key file.ID}
              <PdfViewer fileID={file.ID} {src} name={file.Name} />
            {/key}
          {:else}
            <iframe class="pdf" class:hide={loading} src="{src}#toolbar=1&view=FitH" title="Preview of {file.Name}" onload={done} onerror={() => failed('PDF')}></iframe>
          {/if}
        {:else if mode === 'image'}
          <div class="imgwrap" class:actual={actualSize}>
            <img class:hide={loading} src={src} alt={file.Name} onload={done} onerror={() => failed('image')} />
          </div>
        {:else if mode === 'markdown'}
          {#if !loading}<div class="md">{@html markdownToHTML(text)}</div>{/if}
        {:else if mode === 'text'}
          {#if !loading}<pre class="code" class:wrap>{text}</pre>{/if}
        {:else if mode === 'extracted'}
          {#if !loading}
            <div class="note">
              <Icon name="info" size={12} />
              Text preview — open in {openLabel} for the full file.
            </div>
            <pre class="code" class:wrap>{text}</pre>
          {/if}
        {:else}
          <div class="state">
            <Icon name="file" size={20} />
            <p>No in-app preview for <strong>.{ext(file.Name) || 'this format'}</strong> files.</p>
            <div class="row">
              <button class="btn sm primary" onclick={open} disabled={!file.Synced}><Icon name="external" size={12} /> Open</button>
              <button class="btn sm" onclick={() => void reveal()} disabled={!file.Synced}><Icon name="folderOpen" size={12} /> Reveal</button>
            </div>
          </div>
        {/if}
      {/if}
    </div>
  {/if}
</section>

<style>
  .viewer {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
    height: 100%;
    background: var(--bg-elevated);
  }

  /**
   * Focus mode covers the content area — everything right of the sidebar and
   * below the top bar. `--sidebar-w` is kept live by lib/layout.ts and
   * `--chat-w` by App.svelte (0 unless the chat is docked open), so the rail, a
   * dragged sidebar and a docked chat all land correctly. Focus mode plus a
   * docked chat is the "read and ask" layout: viewer and chat, nothing else.
   */
  .viewer.focused {
    position: fixed;
    top: var(--topbar-h);
    left: var(--sidebar-w);
    right: var(--chat-w, 0px);
    bottom: 0;
    z-index: 40;
    height: auto;
    border-left: 1px solid var(--border);
  }

  .strip {
    flex: none;
    display: flex;
    align-items: center;
    gap: 10px;
    height: 30px;
    padding: 0 8px 0 14px;
    border-bottom: 1px solid var(--border);
    background: var(--bg-subtle);
    font-size: 11.5px;
    color: var(--text-muted);
  }

  .strip-crumb {
    flex: 1;
    min-width: 0;
  }

  .strip-nav {
    display: flex;
    gap: 2px;
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

  .blank {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    color: var(--text-faint);
    font-size: 12.5px;
  }

  .vhead {
    flex: none;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 10px 8px 14px;
    border-bottom: 1px solid var(--border);
    min-height: 46px;
  }

  .vtitle {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .vname {
    font-size: 13px;
    font-weight: 600;
    color: var(--text);
  }

  .vmeta {
    font-size: 11.5px;
    color: var(--text-faint);
    display: flex;
    align-items: center;
    gap: 5px;
  }

  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex: none;
  }

  .vactions {
    flex: none;
    display: flex;
    align-items: center;
    gap: 5px;
  }

  .vbody {
    flex: 1;
    min-height: 0;
    position: relative;
    overflow: auto;
    background: var(--bg-subtle);
  }

  /* PdfViewer owns its own scrolling; a second scrollbar out here would fight it. */
  .vbody.nested {
    overflow: hidden;
    display: flex;
  }

  .vbody.nested > :global(*) {
    flex: 1;
    min-width: 0;
  }

  .vbody.pad {
    background: var(--bg-elevated);
    padding: 12px 14px 24px;
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

  .state.err {
    color: var(--text-muted);
  }

  .state p {
    margin: 0;
    max-width: 34ch;
  }

  .state .row {
    display: flex;
    gap: 6px;
  }

  .pdf {
    width: 100%;
    height: 100%;
    border: none;
    background: var(--bg-subtle);
  }

  .hide {
    visibility: hidden;
    position: absolute;
  }

  .imgwrap {
    min-height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 14px;
  }

  .imgwrap img {
    max-width: 100%;
    max-height: 100%;
    object-fit: contain;
    border-radius: var(--radius-sm);
  }

  .imgwrap.actual {
    display: block;
    overflow: auto;
  }

  .imgwrap.actual img {
    max-width: none;
    max-height: none;
  }

  .note {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 10px;
    padding: 6px 9px;
    border-radius: var(--radius-sm);
    background: var(--accent-soft);
    color: var(--accent-text);
    font-size: 11.5px;
  }

  .code {
    margin: 0;
    font-family: var(--mono);
    font-size: 12px;
    line-height: 1.55;
    color: var(--text);
    white-space: pre;
    overflow-x: auto;
    tab-size: 4;
  }

  .code.wrap {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    overflow-x: hidden;
  }

  .md {
    font-size: 13px;
    line-height: 1.6;
    color: var(--text);
  }

  .md :global(h1),
  .md :global(h2),
  .md :global(h3) {
    margin: 14px 0 6px;
    font-size: 14px;
    font-weight: 650;
  }

  .md :global(p) {
    margin: 0 0 8px;
  }

  .md :global(ul),
  .md :global(ol) {
    margin: 0 0 8px;
    padding-left: 20px;
  }

  .md :global(code) {
    font-family: var(--mono);
    font-size: 11.5px;
    background: var(--bg-subtle);
    padding: 1px 4px;
    border-radius: 4px;
  }

  .md :global(pre) {
    background: var(--bg-subtle);
    padding: 9px 11px;
    border-radius: var(--radius-sm);
    overflow-x: auto;
  }

  .md :global(a) {
    color: var(--accent-text);
  }
</style>
