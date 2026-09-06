<script lang="ts">
  /**
   * Claude chat, to the right of every view. Toggled by the top-bar button or
   * Ctrl+J.
   *
   * Two placements share this component:
   *   docked   App.svelte renders it inside a real grid column, so the view
   *            shrinks beside it and the file underneath stays readable
   *   overlay  it floats over the view on its own, the old behaviour
   * `docked` only changes the chrome (position, border, animation, the
   * collapse chevron) — every bit of chat behaviour below is shared.
   *
   * Context comes from the `currentContext` store — Files sets the selected
   * file, Study its primary selection, Papers the selected paper. "No context"
   * is allowed: the chat still works, it just has no file to read.
   *
   * Everything runs through the Claude Code CLI, so the panel is disabled with
   * instructions when GetStudyStatus() says it is missing or signed out.
   */
  import { api, errMsg, on } from '../api';
  import { chatCollapsed, chatForcedOverlay, toggleChatCollapsed, toggleChatMode } from '../layout';
  import { chatOpen, chatPrefill, currentContext, revealInFiles, toast } from '../stores';
  import type { ChatDelta, ChatDone, ChatMessage, ChatSession, StudyStatus } from '../types';
  import { lsGet, lsSet, markdownToHTML, relTime } from '../util';
  import Icon from './Icon.svelte';

  interface Props {
    /** Rendered as a column in App's content grid rather than floating. */
    docked?: boolean;
  }

  let { docked = false }: Props = $props();

  const MODEL_KEY = 'nussync.chat.model';

  let status = $state<StudyStatus | null>(null);
  let statusLoading = $state(true);
  let model = $state<string>(lsGet(MODEL_KEY, 'sonnet'));

  let sessions = $state<ChatSession[]>([]);
  let sessionsLoading = $state(false);
  let active = $state<ChatSession | null>(null);
  let messages = $state<ChatMessage[]>([]);
  let messagesLoading = $state(false);

  let draft = $state('');
  let streaming = $state(false);
  let streamText = $state('');
  let jobID = $state('');
  let showSessions = $state(false);

  /** Ids for optimistic rows: always negative, always unique within a render. */
  let tempSeq = 0;
  const tempID = () => --tempSeq;

  let inputEl = $state<HTMLTextAreaElement | null>(null);
  let scrollEl = $state<HTMLDivElement | null>(null);

  const ctx = $derived($currentContext);
  /** The collapse chevron is a docked-only affordance. */
  const collapsed = $derived(docked && $chatCollapsed);
  const ready = $derived(!!status?.CLIFound && !!status?.LoggedIn);
  const contextLabel = $derived(ctx.name || (ctx.paperID ? ctx.paperID : ctx.fileID ? `file #${ctx.fileID}` : ''));

  $effect(() => {
    lsSet(MODEL_KEY, model);
  });

  // ------------------------------------------------------------- load

  async function loadStatus() {
    statusLoading = true;
    try {
      status = await api.getStudyStatus();
    } catch (err) {
      status = { CLIFound: false, Version: '', LoggedIn: false, Error: errMsg(err), Models: null };
    } finally {
      statusLoading = false;
    }
  }

  async function loadSessions() {
    sessionsLoading = true;
    try {
      sessions = (await api.getChats(ctx.fileID, ctx.paperID)) ?? [];
    } catch (err) {
      console.error('[chat] sessions failed', err);
      sessions = [];
    } finally {
      sessionsLoading = false;
    }
    const first = sessions[0] ?? null;
    if (!active || !sessions.some((s) => s.ID === active!.ID)) {
      active = first;
      if (first) await loadMessages(first.ID);
      else messages = [];
    }
  }

  async function loadMessages(id: string) {
    messagesLoading = true;
    try {
      messages = (await api.getChatMessages(id)) ?? [];
    } catch (err) {
      console.error('[chat] messages failed', err);
      messages = [];
    } finally {
      messagesLoading = false;
      scrollDown();
    }
  }

  // Open the panel -> check the CLI once.
  $effect(() => {
    if ($chatOpen && status === null) void loadStatus();
  });

  // Re-scope the session list whenever the panel opens or the context changes.
  $effect(() => {
    void ctx.fileID;
    void ctx.paperID;
    if (!$chatOpen) return;
    active = null;
    messages = [];
    streamText = '';
    void loadSessions();
  });

  $effect(() => {
    if (!$chatOpen) return;
    const offDelta = on('chat:delta', (d: ChatDelta) => {
      if (!d || !active || d.SessionID !== active.ID) return;
      streamText += d.Text ?? '';
      scrollDown();
    });
    const offDone = on('chat:done', (d: ChatDone) => {
      if (!d || !active || d.SessionID !== active.ID) return;
      streaming = false;
      jobID = '';
      if (d.Error && d.Error !== 'cancelled') toast(d.Error, 'error');
      const text = streamText;
      streamText = '';
      if (text.trim()) {
        // Optimistic — the reload below replaces it with the stored row.
        messages = [
          ...messages,
          { ID: tempID(), SessionID: d.SessionID, Role: 'assistant', Text: text, CreatedAt: new Date().toISOString() },
        ];
      }
      void loadMessages(d.SessionID);
      void refreshSessionTitles();
    });
    return () => {
      offDelta();
      offDone();
    };
  });

  async function refreshSessionTitles() {
    try {
      sessions = (await api.getChats(ctx.fileID, ctx.paperID)) ?? [];
      const cur = active;
      if (cur) active = sessions.find((s) => s.ID === cur.ID) ?? cur;
    } catch {
      /* titles are cosmetic */
    }
  }

  function scrollDown() {
    queueMicrotask(() => {
      if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
    });
  }

  // ---------------------------------------------------------- actions

  async function newChat(): Promise<ChatSession | null> {
    try {
      const s = await api.startChat(ctx.fileID, ctx.paperID, model);
      sessions = [s, ...sessions];
      active = s;
      messages = [];
      streamText = '';
      showSessions = false;
      return s;
    } catch (err) {
      toast(errMsg(err), 'error');
      return null;
    }
  }

  async function pick(s: ChatSession) {
    active = s;
    streamText = '';
    showSessions = false;
    await loadMessages(s.ID);
  }

  async function send() {
    const text = draft.trim();
    if (!text || streaming || !ready) return;
    let session = active;
    if (!session) session = await newChat();
    if (!session) return;

    draft = '';
    messages = [
      ...messages,
      { ID: tempID(), SessionID: session.ID, Role: 'user', Text: text, CreatedAt: new Date().toISOString() },
    ];
    streaming = true;
    streamText = '';
    scrollDown();
    try {
      jobID = await api.sendChat(session.ID, text);
    } catch (err) {
      streaming = false;
      toast(errMsg(err), 'error');
    }
  }

  async function cancel() {
    if (!jobID) {
      streaming = false;
      return;
    }
    try {
      await api.cancelStudyJob(jobID);
    } catch (err) {
      toast(errMsg(err), 'error');
    }
    streaming = false;
    jobID = '';
  }

  async function deleteSession() {
    if (!active) return;
    const id = active.ID;
    try {
      await api.deleteChat(id);
      sessions = sessions.filter((s) => s.ID !== id);
      active = sessions[0] ?? null;
      messages = [];
      streamText = '';
      if (active) await loadMessages(active.ID);
    } catch (err) {
      toast(errMsg(err), 'error');
    }
  }

  function onInputKey(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      void send();
    }
    // Shift+Enter falls through to the textarea and inserts a newline.
  }

  function close() {
    chatOpen.set(false);
  }

  // Focus the composer when the panel opens (but not when it is a rail).
  $effect(() => {
    if ($chatOpen && !collapsed) {
      queueMicrotask(() => inputEl?.focus());
    }
  });

  /**
   * Consume a prefill queued by "Ask Claude" elsewhere (the PDF viewer's
   * highlight popover). Appended rather than replacing, so a half-typed
   * question survives, and cleared immediately: the store is a one-shot
   * handoff, not state.
   */
  $effect(() => {
    const queued = $chatPrefill;
    if (!queued) return;
    chatPrefill.set('');
    draft = draft.trim() ? `${draft.replace(/\s+$/, '')}\n\n${queued}` : queued;
    queueMicrotask(() => {
      inputEl?.focus();
      inputEl?.setSelectionRange(draft.length, draft.length);
    });
  });
</script>

{#if $chatOpen}
  <aside class="chat" class:docked class:floating={!docked} class:collapsed aria-label="Claude chat">
    {#if collapsed}
      <div class="rail">
        <button
          class="icon-btn"
          onclick={toggleChatCollapsed}
          title="Expand the chat"
          aria-label="Expand the chat"
        >
          <Icon name="chevronLeft" size={13} />
        </button>
        <Icon name="chat" size={14} />
        <div class="grow"></div>
        <button class="icon-btn" onclick={close} title="Close chat" aria-label="Close chat">
          <Icon name="x" size={13} />
        </button>
      </div>
    {:else}
    <header class="head">
      <Icon name="chat" size={14} />
      <span class="head-title">Chat</span>
      {#if $chatForcedOverlay}
        <span class="chip hint-chip" title="The window is too narrow to dock the chat — widen it to get the column back.">
          narrow window
        </span>
      {/if}
      <div class="grow"></div>
      <select class="select model" bind:value={model} aria-label="Model">
        {#each status?.Models ?? ['opus', 'sonnet', 'haiku'] as m (m)}
          <option value={m}>{m}</option>
        {/each}
      </select>
      <button class="icon-btn" onclick={() => (showSessions = !showSessions)} title="Chat history" aria-label="Chat history">
        <Icon name="layers" size={13} />
      </button>
      <button class="icon-btn" onclick={() => void newChat()} title="New chat" aria-label="New chat">
        <Icon name="plus" size={13} />
      </button>
      <button
        class="icon-btn"
        onclick={toggleChatMode}
        title={docked ? 'Pop out — float the chat over the view' : 'Dock the chat as a column'}
        aria-label={docked ? 'Pop the chat out' : 'Dock the chat'}
      >
        <Icon name={docked ? 'external' : 'sidebarCollapse'} size={13} />
      </button>
      {#if docked}
        <button class="icon-btn" onclick={toggleChatCollapsed} title="Collapse the chat" aria-label="Collapse the chat">
          <Icon name="chevronRight" size={13} />
        </button>
      {/if}
      <button class="icon-btn" onclick={close} title={docked ? 'Close chat' : 'Close (Esc)'} aria-label="Close chat">
        <Icon name="x" size={13} />
      </button>
    </header>

    <div class="ctx">
      {#if contextLabel}
        {#if ctx.fileID}
          <button
            class="chip accent ctx-chip linky"
            title="{contextLabel} — show it in Files"
            onclick={() => revealInFiles(ctx.fileID)}
          >
            <Icon name={ctx.paperID ? 'book' : 'fileText'} size={11} />
            <span class="truncate">{contextLabel}</span>
          </button>
        {:else}
          <span class="chip accent ctx-chip" title={contextLabel}>
            <Icon name={ctx.paperID ? 'book' : 'fileText'} size={11} />
            <span class="truncate">{contextLabel}</span>
          </span>
        {/if}
      {:else}
        <span class="chip ctx-chip faint">No file selected — chatting without context</span>
      {/if}
      {#if active}
        <div class="grow"></div>
        <button class="linkish" onclick={() => void deleteSession()}>Delete chat</button>
      {/if}
    </div>

    {#if showSessions}
      <div class="sessions">
        {#if sessionsLoading}
          <div class="s-empty"><span class="spinner"></span> Loading…</div>
        {:else if sessions.length === 0}
          <div class="s-empty">No chats for this context yet.</div>
        {:else}
          {#each sessions as s (s.ID)}
            <button class="s-row" class:on={active?.ID === s.ID} onclick={() => void pick(s)}>
              <span class="s-title truncate">{s.Title || 'Untitled'}</span>
              <span class="s-time faint">{relTime(s.UpdatedAt)}</span>
            </button>
          {/each}
        {/if}
      </div>
    {/if}

    {#if statusLoading}
      <div class="banner"><span class="spinner"></span> Checking the Claude Code CLI…</div>
    {:else if !ready}
      <div class="banner warn">
        <Icon name="alert" size={13} />
        <div class="banner-text">
          {#if !status?.CLIFound}
            Claude Code CLI not found. Install it and make sure <span class="mono">claude</span> is on your PATH.
          {:else}
            Not signed in. Run <span class="mono">claude auth login</span> in a terminal, then re-check.
          {/if}
          {#if status?.Error}<div class="faint">{status.Error}</div>{/if}
        </div>
        <button class="btn sm" onclick={() => void loadStatus()}>Re-check</button>
      </div>
    {/if}

    <div class="msgs" bind:this={scrollEl}>
      {#if messagesLoading}
        <div class="m-empty"><span class="spinner"></span> Loading…</div>
      {:else if messages.length === 0 && !streaming}
        <div class="m-empty">
          {#if contextLabel}
            Ask anything about <strong>{contextLabel}</strong>. Claude reads the file before answering and cites pages.
          {:else}
            No context selected. Pick a file in Files or a paper in Papers, or just ask a question here.
          {/if}
        </div>
      {/if}

      {#each messages as m (m.ID)}
        <div class="msg" class:user={m.Role === 'user'}>
          <div class="role">{m.Role === 'user' ? 'You' : 'Claude'}</div>
          {#if m.Role === 'user'}
            <div class="bubble">{m.Text}</div>
          {:else}
            <div class="bubble md">{@html markdownToHTML(m.Text)}</div>
          {/if}
        </div>
      {/each}

      {#if streaming}
        <div class="msg">
          <div class="role">Claude</div>
          {#if streamText}
            <div class="bubble md">{@html markdownToHTML(streamText)}</div>
          {:else}
            <div class="bubble thinking"><span class="spinner"></span> Thinking…</div>
          {/if}
        </div>
      {/if}
    </div>

    <div class="composer">
      <textarea
        class="input box"
        rows="2"
        placeholder={ready ? 'Ask about this file… (Enter to send, Shift+Enter for a newline)' : 'Chat needs the Claude Code CLI'}
        bind:value={draft}
        bind:this={inputEl}
        onkeydown={onInputKey}
        disabled={!ready}
      ></textarea>
      <div class="composer-row">
        <span class="faint hint">Enter sends · Shift+Enter newline</span>
        <div class="grow"></div>
        {#if streaming}
          <button class="btn sm" onclick={() => void cancel()}>Cancel</button>
        {:else}
          <button class="btn sm primary" onclick={() => void send()} disabled={!ready || !draft.trim()}>
            <Icon name="send" size={12} /> Send
          </button>
        {/if}
      </div>
    </div>
    {/if}
  </aside>
{/if}

<style>
  .chat {
    display: flex;
    flex-direction: column;
    min-height: 0;
    background: var(--bg-elevated);
    border-left: 1px solid var(--border);
  }

  /* Floating: on top of the view, with the shadow and the slide-in. */
  .chat.floating {
    position: fixed;
    top: 0;
    right: 0;
    bottom: 0;
    width: 390px;
    max-width: 100vw;
    z-index: 45;
    box-shadow: var(--shadow-pop);
    animation: slide-in 160ms cubic-bezier(0.4, 0, 0.2, 1);
  }

  /*
   * Docked: a plain column. App.svelte owns the width, so there is no
   * positioning, no shadow and no animation here — a column that slides in
   * would drag the whole view with it on every toggle.
   */
  .chat.docked {
    flex: 1;
    min-width: 0;
    height: 100%;
  }

  @keyframes slide-in {
    from {
      transform: translateX(16px);
      opacity: 0;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .chat.floating {
      animation: none;
    }
  }

  .rail {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    height: 100%;
    padding: 12px 0;
    color: var(--text-faint);
  }

  .hint-chip {
    height: 18px;
    padding: 0 6px;
    font-size: 10px;
    color: var(--text-faint);
    flex: none;
  }

  .linky {
    cursor: pointer;
    transition: filter var(--t);
  }

  .linky:hover {
    filter: brightness(1.08);
    text-decoration: underline;
  }

  .grow {
    flex: 1;
  }

  .head {
    display: flex;
    align-items: center;
    gap: 7px;
    height: var(--topbar-h);
    padding: 0 10px 0 14px;
    border-bottom: 1px solid var(--border);
    flex: none;
  }

  .head-title {
    font-size: 13px;
    font-weight: 600;
  }

  .model {
    width: auto;
    height: 26px;
    font-size: 12px;
    padding: 0 24px 0 8px;
    background-position: calc(100% - 12px) 11px, calc(100% - 7px) 11px;
  }

  .icon-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 26px;
    border-radius: var(--radius-sm);
    color: var(--text-faint);
    flex: none;
    transition: background var(--t), color var(--t);
  }

  .icon-btn:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .ctx {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 12px;
    border-bottom: 1px solid var(--border);
    flex: none;
  }

  /* Flexible rather than a fixed 250px, so a dragged column keeps it whole. */
  .ctx-chip {
    flex: 0 1 auto;
    min-width: 0;
    max-width: 100%;
    height: 22px;
  }

  .linkish {
    font-size: 11.5px;
    font-weight: 550;
    color: var(--text-faint);
  }

  .linkish:hover {
    color: var(--red);
    text-decoration: underline;
  }

  .sessions {
    max-height: 190px;
    overflow-y: auto;
    border-bottom: 1px solid var(--border);
    background: var(--bg-subtle);
    flex: none;
  }

  .s-row {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 7px 12px;
    text-align: left;
    transition: background var(--t);
  }

  .s-row:hover {
    background: var(--bg-hover);
  }

  .s-row.on {
    background: var(--bg-active);
  }

  .s-title {
    flex: 1;
    min-width: 0;
    font-size: 12.5px;
  }

  .s-time {
    font-size: 11px;
    flex: none;
  }

  .s-empty {
    padding: 12px;
    font-size: 12px;
    color: var(--text-faint);
    text-align: center;
  }

  .banner {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    padding: 9px 12px;
    border-bottom: 1px solid var(--border);
    font-size: 12px;
    color: var(--text-muted);
    flex: none;
  }

  .banner.warn {
    background: var(--amber-soft);
    color: var(--amber);
  }

  .banner-text {
    flex: 1;
    min-width: 0;
    line-height: 1.5;
  }

  .msgs {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 14px 12px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  .m-empty {
    margin: auto 4px;
    text-align: center;
    font-size: 12.5px;
    line-height: 1.6;
    color: var(--text-faint);
  }

  .msg {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .role {
    font-size: 10.5px;
    font-weight: 600;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    color: var(--text-faint);
  }

  .bubble {
    padding: 9px 11px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
    font-size: 12.5px;
    line-height: 1.6;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  .msg.user .bubble {
    background: var(--accent-soft);
    border-color: transparent;
  }

  .bubble.md {
    white-space: normal;
  }

  .thinking {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--text-faint);
  }

  .composer {
    flex: none;
    padding: 10px 12px 12px;
    border-top: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    gap: 7px;
  }

  .box {
    height: auto;
    padding: 8px 10px;
    line-height: 1.5;
    resize: none;
  }

  .composer-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .hint {
    font-size: 11px;
  }

  /* ---------------------------------------------------------- markdown */

  .md :global(h1),
  .md :global(h2),
  .md :global(h3) {
    font-size: 12.5px;
    font-weight: 620;
    margin: 10px 0 4px;
  }

  .md :global(h1:first-child),
  .md :global(h2:first-child),
  .md :global(h3:first-child) {
    margin-top: 0;
  }

  .md :global(p) {
    margin: 0 0 7px;
  }

  .md :global(p:last-child) {
    margin-bottom: 0;
  }

  .md :global(ul),
  .md :global(ol) {
    margin: 0 0 8px;
    padding-left: 18px;
    list-style: revert;
  }

  .md :global(li) {
    margin-bottom: 2px;
  }

  .md :global(code) {
    font-family: var(--mono);
    font-size: 11.5px;
    background: var(--bg-subtle);
    border-radius: 4px;
    padding: 1px 4px;
  }

  .md :global(pre) {
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 9px 10px;
    overflow-x: auto;
    margin: 0 0 8px;
  }

  .md :global(pre code) {
    background: none;
    padding: 0;
  }

  .md :global(blockquote) {
    margin: 0 0 8px;
    padding-left: 10px;
    border-left: 2px solid var(--border-strong);
    color: var(--text-muted);
  }
</style>
