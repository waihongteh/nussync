<script lang="ts">
  import { api, errMsg } from '../api';
  import Icon from '../components/Icon.svelte';
  import { courses, loadCourses, settings, theme, toast } from '../stores';
  import type { Settings, TelegramStatus } from '../types';
  import { durLabel, parseDurLabel } from '../util';

  let draft = $state<Settings | null>(null);
  let baseline = $state<string>('');
  let saving = $state(false);

  let showCanvasToken = $state(false);
  let showTelegramToken = $state(false);

  let testing = $state(false);
  let canvasName = $state('');
  let canvasError = $state('');

  let tg = $state<TelegramStatus>({ Configured: false, ChatID: '', BotName: 'nuscanvassync_bot' });
  let pairing = $state(false);
  let sendingTest = $state(false);

  let extInput = $state('');
  let ladderInput = $state('');

  const INTERVALS = [
    { v: 15, label: 'Every 15 minutes' },
    { v: 30, label: 'Every 30 minutes' },
    { v: 60, label: 'Hourly' },
    { v: 180, label: 'Every 3 hours' },
    { v: 360, label: 'Every 6 hours' },
    { v: 720, label: 'Every 12 hours' },
    { v: 1440, label: 'Daily' },
    { v: 0, label: 'Manual only' },
  ];

  // Seed the local draft from the store once it arrives.
  $effect(() => {
    const s = $settings;
    if (s && draft === null) {
      draft = structuredClone($state.snapshot(s)) as Settings;
      baseline = JSON.stringify(draft);
    }
  });

  $effect(() => {
    void api
      .getTelegramStatus()
      .then((v) => v && (tg = v))
      .catch(() => {});
  });

  const dirty = $derived(draft !== null && JSON.stringify(draft) !== baseline);

  async function save() {
    if (!draft) return;
    saving = true;
    try {
      const payload = structuredClone($state.snapshot(draft)) as Settings;
      await api.saveSettings(payload);
      settings.set(payload);
      if (payload.Theme === 'light' || payload.Theme === 'dark' || payload.Theme === 'system') {
        theme.set(payload.Theme);
      }
      baseline = JSON.stringify(draft);
      toast('Settings saved', 'success');
    } catch (err) {
      toast(`Save failed: ${errMsg(err)}`, 'error');
    } finally {
      saving = false;
    }
  }

  function revert() {
    if (!$settings) return;
    draft = structuredClone($state.snapshot($settings)) as Settings;
    baseline = JSON.stringify(draft);
  }

  async function testCanvas() {
    testing = true;
    canvasName = '';
    canvasError = '';
    try {
      canvasName = await api.testCanvas();
    } catch (err) {
      canvasError = errMsg(err);
    } finally {
      testing = false;
    }
  }

  async function chooseDir() {
    try {
      const dir = await api.chooseSyncDir();
      if (dir && draft) draft.SyncDir = dir;
    } catch (err) {
      toast(errMsg(err), 'error');
    }
  }

  async function toggleCourse(id: number, enabled: boolean) {
    try {
      await api.setCourseEnabled(id, enabled);
      courses.update((list) => list.map((c) => (c.ID === id ? { ...c, Enabled: enabled } : c)));
    } catch (err) {
      toast(errMsg(err), 'error');
      void loadCourses();
    }
  }

  function addExt() {
    if (!draft) return;
    let v = extInput.trim().toLowerCase();
    if (!v) return;
    if (!v.startsWith('.')) v = '.' + v;
    if (!draft.SkipExts.includes(v)) draft.SkipExts = [...draft.SkipExts, v];
    extInput = '';
  }

  function removeExt(v: string) {
    if (!draft) return;
    draft.SkipExts = draft.SkipExts.filter((x) => x !== v);
  }

  function addLadder() {
    if (!draft) return;
    const parsed = parseDurLabel(ladderInput);
    if (!parsed) {
      toast('Use a value like 3d, 12h or 30m', 'error');
      return;
    }
    if (!draft.ReminderLadder.includes(parsed)) {
      draft.ReminderLadder = [...draft.ReminderLadder, parsed].sort(
        (a, b) => hours(b) - hours(a),
      );
    }
    ladderInput = '';
  }

  function hours(d: string): number {
    const m = /^(\d+(?:\.\d+)?)(h|m|s)$/.exec(d);
    if (!m) return 0;
    const n = parseFloat(m[1]);
    return m[2] === 'h' ? n : m[2] === 'm' ? n / 60 : n / 3600;
  }

  function removeLadder(v: string) {
    if (!draft) return;
    draft.ReminderLadder = draft.ReminderLadder.filter((x) => x !== v);
  }

  async function pair() {
    pairing = true;
    try {
      const chatID = await api.pairTelegram();
      tg = await api.getTelegramStatus();
      if (draft) draft.TelegramChatID = chatID;
      toast(`Paired with Telegram chat ${chatID}`, 'success');
    } catch (err) {
      toast(`Pairing failed: ${errMsg(err)}`, 'error');
    } finally {
      pairing = false;
    }
  }

  async function sendTest() {
    sendingTest = true;
    try {
      await api.sendTestTelegram();
    } catch (err) {
      toast(errMsg(err), 'error');
    } finally {
      sendingTest = false;
    }
  }
</script>

<div class="view">
  <div class="view-narrow settings">
    <header class="head">
      <div>
        <h1 class="page-title">Settings</h1>
        <p class="muted sub">Canvas credentials, sync behaviour and reminders.</p>
      </div>
      <div class="actions">
        {#if dirty}
          <span class="chip amber">Unsaved changes</span>
          <button class="btn" onclick={revert} disabled={saving}>Revert</button>
        {/if}
        <button class="btn primary" onclick={save} disabled={!dirty || saving}>
          {#if saving}<span class="spinner"></span>{/if}
          Save
        </button>
      </div>
    </header>

    {#if !draft}
      <div class="card"><div class="empty">Loading settings…</div></div>
    {:else}
      <!-- ----------------------------------------------------------- Canvas -->
      <section class="card section">
        <div class="section-head">
          <h2>Canvas</h2>
          <p class="muted">Where NUSSync reads your courses from.</p>
        </div>
        <div class="fields">
          <label class="field">
            <span class="lbl">Canvas URL</span>
            <input class="input" type="text" bind:value={draft.CanvasURL} placeholder="https://canvas.nus.edu.sg" spellcheck="false" />
          </label>
          <label class="field">
            <span class="lbl">Access token</span>
            <span class="with-btn">
              <input
                class="input"
                type={showCanvasToken ? 'text' : 'password'}
                bind:value={draft.CanvasToken}
                placeholder="7~…"
                spellcheck="false"
                autocomplete="off"
              />
              <button class="reveal" onclick={() => (showCanvasToken = !showCanvasToken)} aria-label="Toggle token visibility" type="button">
                <Icon name={showCanvasToken ? 'eyeOff' : 'eye'} size={14} />
              </button>
            </span>
            <span class="help">Generate one in Canvas under Account → Settings → New access token.</span>
          </label>
          <div class="field row-field">
            <button class="btn" onclick={testCanvas} disabled={testing}>
              {#if testing}<span class="spinner"></span>{:else}<Icon name="link" size={13} />{/if}
              Test connection
            </button>
            {#if canvasName}
              <span class="chip green"><Icon name="check" size={11} /> Connected as {canvasName}</span>
            {:else if canvasError}
              <span class="chip red"><Icon name="alert" size={11} /> {canvasError}</span>
            {/if}
          </div>
        </div>
      </section>

      <!-- ------------------------------------------------------------- Sync -->
      <section class="card section">
        <div class="section-head">
          <h2>Sync</h2>
          <p class="muted">Where files land and what gets skipped.</p>
        </div>
        <div class="fields">
          <label class="field">
            <span class="lbl">Sync folder</span>
            <span class="with-btn">
              <input class="input" type="text" bind:value={draft.SyncDir} spellcheck="false" />
              <button class="btn choose" onclick={chooseDir} type="button">
                <Icon name="folderCog" size={13} /> Choose…
              </button>
            </span>
          </label>

          <div class="grid2">
            <label class="field">
              <span class="lbl">Sync interval</span>
              <select class="select" bind:value={draft.SyncIntervalMin}>
                {#each INTERVALS as i (i.v)}
                  <option value={i.v}>{i.label}</option>
                {/each}
              </select>
            </label>
            <label class="field">
              <span class="lbl">Max file size (MB)</span>
              <input class="input" type="number" min="0" step="1" bind:value={draft.MaxFileMB} />
              <span class="help">0 means no limit.</span>
            </label>
          </div>

          <div class="field">
            <span class="lbl">Skip extensions</span>
            <div class="chips">
              {#each draft.SkipExts as e (e)}
                <span class="chip removable">
                  {e}
                  <button class="chip-x" onclick={() => removeExt(e)} aria-label="Remove {e}" type="button">
                    <Icon name="x" size={10} />
                  </button>
                </span>
              {/each}
              <input
                class="chip-input"
                type="text"
                placeholder="add .ext"
                bind:value={extInput}
                onkeydown={(e) => {
                  if (e.key === 'Enter' || e.key === ',') {
                    e.preventDefault();
                    addExt();
                  }
                }}
                onblur={addExt}
                spellcheck="false"
              />
            </div>
          </div>

          <div class="field">
            <span class="lbl">Courses to sync</span>
            <div class="course-grid">
              {#each $courses as c (c.ID)}
                <label class="course-toggle">
                  <input type="checkbox" checked={c.Enabled} onchange={(e) => toggleCourse(c.ID, e.currentTarget.checked)} />
                  <span class="track"><span class="knob"></span></span>
                  <span class="dot" style="background:{c.Color}"></span>
                  <span class="c-code">{c.Code}</span>
                  <span class="c-name truncate faint">{c.Name}</span>
                </label>
              {/each}
            </div>
          </div>
        </div>
      </section>

      <!-- --------------------------------------------------------- Telegram -->
      <section class="card section">
        <div class="section-head">
          <h2>Telegram</h2>
          <p class="muted">Reminders are delivered by a Telegram bot.</p>
        </div>
        <div class="fields">
          <label class="field">
            <span class="lbl">Bot token</span>
            <span class="with-btn">
              <input
                class="input"
                type={showTelegramToken ? 'text' : 'password'}
                bind:value={draft.TelegramToken}
                placeholder="123456:ABC-DEF…"
                spellcheck="false"
                autocomplete="off"
              />
              <button class="reveal" onclick={() => (showTelegramToken = !showTelegramToken)} aria-label="Toggle token visibility" type="button">
                <Icon name={showTelegramToken ? 'eyeOff' : 'eye'} size={14} />
              </button>
            </span>
          </label>

          <div class="tg-card" class:paired={tg.Configured}>
            <div class="tg-icon"><Icon name="telegram" size={16} /></div>
            <div class="tg-body">
              {#if tg.Configured}
                <div class="tg-title">Paired with @{tg.BotName || 'nuscanvassync_bot'}</div>
                <div class="tg-sub">Chat ID <span class="mono">{tg.ChatID}</span></div>
              {:else}
                <div class="tg-title">Not paired yet</div>
                <div class="tg-sub">
                  Open Telegram, send <span class="mono">/start</span> to
                  <span class="mono">@{tg.BotName || 'nuscanvassync_bot'}</span>, then click Pair — NUSSync waits up to 60&nbsp;seconds
                  for your message.
                </div>
              {/if}
            </div>
            <div class="tg-actions">
              <button class="btn" onclick={pair} disabled={pairing}>
                {#if pairing}<span class="spinner"></span> Waiting…{:else}{tg.Configured ? 'Re-pair' : 'Pair'}{/if}
              </button>
              <button class="btn" onclick={sendTest} disabled={!tg.Configured || sendingTest}>
                {#if sendingTest}<span class="spinner"></span>{:else}<Icon name="send" size={13} />{/if}
                Send test
              </button>
            </div>
          </div>
        </div>
      </section>

      <!-- -------------------------------------------------------- Reminders -->
      <section class="card section">
        <div class="section-head">
          <h2>Reminders</h2>
          <p class="muted">How far ahead of a deadline you get pinged.</p>
        </div>
        <div class="fields">
          <div class="field">
            <span class="lbl">Reminder ladder</span>
            <div class="chips">
              {#each draft.ReminderLadder as d (d)}
                <span class="chip accent removable">
                  {durLabel(d)}
                  <button class="chip-x" onclick={() => removeLadder(d)} aria-label="Remove {durLabel(d)}" type="button">
                    <Icon name="x" size={10} />
                  </button>
                </span>
              {/each}
              <input
                class="chip-input"
                type="text"
                placeholder="add 3d / 6h / 30m"
                bind:value={ladderInput}
                onkeydown={(e) => {
                  if (e.key === 'Enter' || e.key === ',') {
                    e.preventDefault();
                    addLadder();
                  }
                }}
                spellcheck="false"
              />
            </div>
            <span class="help">Sent before every unsubmitted assignment or quiz.</span>
          </div>

          <label class="switch-row">
            <input type="checkbox" bind:checked={draft.NotifyAnnouncements} />
            <span class="track"><span class="knob"></span></span>
            <span class="switch-text">
              <span class="switch-title">Announcement alerts</span>
              <span class="help">Ping me when a course posts something new.</span>
            </span>
          </label>

          <label class="switch-row">
            <input type="checkbox" bind:checked={draft.NotifyGrades} />
            <span class="track"><span class="knob"></span></span>
            <span class="switch-text">
              <span class="switch-title">Grade alerts</span>
              <span class="help">Ping me when a grade is released.</span>
            </span>
          </label>
        </div>
      </section>

      <!-- -------------------------------------------------------------- App -->
      <section class="card section">
        <div class="section-head">
          <h2>App</h2>
          <p class="muted">Appearance and startup.</p>
        </div>
        <div class="fields">
          <label class="field narrow">
            <span class="lbl">Theme</span>
            <select
              class="select"
              value={draft.Theme}
              onchange={(e) => {
                const v = e.currentTarget.value as Settings['Theme'];
                if (draft) draft.Theme = v;
                if (v === 'light' || v === 'dark' || v === 'system') theme.set(v);
              }}
            >
              <option value="system">Match Windows</option>
              <option value="light">Light</option>
              <option value="dark">Dark</option>
            </select>
          </label>

          <label class="switch-row">
            <input type="checkbox" bind:checked={draft.LaunchAtLogin} />
            <span class="track"><span class="knob"></span></span>
            <span class="switch-text">
              <span class="switch-title">Launch at login</span>
              <span class="help">Start NUSSync minimised when Windows starts.</span>
            </span>
          </label>
        </div>
      </section>
    {/if}
  </div>
</div>

<style>
  .settings {
    padding-bottom: 30px;
  }

  .head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    margin-bottom: 20px;
  }

  .sub {
    margin-top: 3px;
    font-size: 13px;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: none;
  }

  .section {
    margin-bottom: 16px;
    overflow: hidden;
  }

  .section-head {
    padding: 13px 16px;
    border-bottom: 1px solid var(--border);
    background: var(--bg-subtle);
  }

  .section-head h2 {
    font-size: 13.5px;
    font-weight: 620;
  }

  .section-head p {
    font-size: 12px;
    margin-top: 1px;
  }

  .fields {
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 15px;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 5px;
  }

  .field.narrow {
    max-width: 260px;
  }

  .row-field {
    flex-direction: row;
    align-items: center;
    gap: 10px;
  }

  .grid2 {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 15px;
  }

  .lbl {
    font-size: 12px;
    font-weight: 550;
    color: var(--text-muted);
  }

  .help {
    font-size: 11.5px;
    color: var(--text-faint);
  }

  .with-btn {
    display: flex;
    gap: 6px;
    align-items: center;
  }

  .reveal {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    flex: none;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    background: var(--bg-elevated);
    transition: background var(--t), color var(--t);
  }

  .reveal:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .choose {
    height: 32px;
    flex: none;
  }

  .chips {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    min-height: 32px;
    padding: 5px 6px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-input);
  }

  .chips:focus-within {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-soft);
  }

  .chip.removable {
    padding-right: 3px;
  }

  .chip-x {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 14px;
    height: 14px;
    border-radius: 3px;
    color: inherit;
    opacity: 0.6;
  }

  .chip-x:hover {
    opacity: 1;
    background: var(--bg-hover);
  }

  .chip-input {
    flex: 1;
    min-width: 110px;
    height: 21px;
    border: none;
    background: none;
    outline: none;
    font-size: 12.5px;
  }

  .chip-input::placeholder {
    color: var(--text-faint);
  }

  .course-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 4px 16px;
  }

  .course-toggle,
  .switch-row {
    display: flex;
    align-items: center;
    gap: 9px;
    cursor: pointer;
    user-select: none;
    padding: 4px 0;
  }

  .switch-row {
    align-items: flex-start;
  }

  .course-toggle input,
  .switch-row input {
    position: absolute;
    opacity: 0;
    width: 0;
    height: 0;
  }

  .track {
    width: 30px;
    height: 17px;
    flex: none;
    border-radius: 999px;
    background: var(--border-strong);
    padding: 2px;
    transition: background var(--t);
  }

  .switch-row .track {
    margin-top: 1px;
  }

  .knob {
    display: block;
    width: 13px;
    height: 13px;
    border-radius: 50%;
    background: #fff;
    transition: transform var(--t);
  }

  input:checked + .track {
    background: var(--accent);
  }

  input:checked + .track .knob {
    transform: translateX(13px);
  }

  input:focus-visible + .track {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }

  .c-code {
    font-size: 12.5px;
    font-weight: 550;
    flex: none;
  }

  .c-name {
    font-size: 11.5px;
    min-width: 0;
  }

  .switch-text {
    display: flex;
    flex-direction: column;
  }

  .switch-title {
    font-size: 13px;
  }

  .tg-card {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    padding: 13px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-subtle);
  }

  .tg-card.paired {
    border-color: var(--green);
    background: var(--green-soft);
  }

  .tg-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 30px;
    height: 30px;
    flex: none;
    border-radius: var(--radius-sm);
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    color: var(--accent-text);
  }

  .tg-card.paired .tg-icon {
    color: var(--green);
  }

  .tg-body {
    flex: 1;
    min-width: 0;
  }

  .tg-title {
    font-size: 13px;
    font-weight: 550;
  }

  .tg-sub {
    font-size: 12px;
    color: var(--text-muted);
    margin-top: 2px;
    line-height: 1.55;
  }

  .tg-actions {
    display: flex;
    gap: 6px;
    flex: none;
  }

  @media (max-width: 780px) {
    .grid2,
    .course-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
