<script lang="ts">
  import { api, errMsg } from '../api';
  import Icon from '../components/Icon.svelte';
  import { petEnabled, petMode, petName, resetPetPos } from '../pet';
  import type { PetMode } from '../pet';

  import { courses, loadCourses, settings, theme, toast } from '../stores';
  import { hideNonAcademic } from '../courseOrder';
  import { chatMode, resetLayout, sidebarMode } from '../layout';
  import type { ChatMode, SidebarMode } from '../layout';
  import type { Settings, StudyStatus, TelegramStatus } from '../types';
  import { deepEqual, durLabel, lsGet, lsSet, parseDurLabel } from '../util';

  const PET_MODES: Array<{ id: PetMode; label: string; help: string }> = [
    { id: 'dock', label: 'Docked', help: 'Sits at the bottom of the sidebar.' },
    { id: 'drag', label: 'Draggable', help: 'Floats over the app; drag it anywhere.' },
    { id: 'wander', label: 'Wander', help: 'Walks around on its own. Click it to say hi.' },
  ];

  // `draft` stays null until GetSettings has actually resolved. Nothing may
  // be saved before then: a draft seeded from an empty object would send blank
  // credentials to SaveSettings, which is how config.json lost its Canvas
  // token on 2026-09-06.
  let draft = $state<Settings | null>(null);
  /** The exact object GetSettings returned, for a truthful dirty check. */
  let baseline = $state<Settings | null>(null);
  let loaded = $state(false);
  let saving = $state(false);

  let showCanvasToken = $state(false);
  let showTelegramToken = $state(false);

  let testing = $state(false);
  let canvasName = $state('');
  let canvasError = $state('');

  let tg = $state<TelegramStatus>({ Configured: false, ChatID: '', BotName: '@nuscanvassync_bot' });
  let pairing = $state(false);
  let sendingTest = $state(false);

  let extInput = $state('');
  let ladderInput = $state('');

  // ------------------------------------------------------------- papers

  let keywordInput = $state('');
  let categoryInput = $state('');
  let venueInput = $state('');
  let showPaperToken = $state(false);
  let paperTg = $state<TelegramStatus>({ Configured: false, ChatID: '', BotName: '@nuspapertracker_bot' });
  let paperPairing = $state(false);
  let paperTesting = $state(false);

  const DIGEST_HOURS = Array.from({ length: 24 }, (_, h) => ({
    v: h,
    label: `${String(h).padStart(2, '0')}:00`,
  }));

  const PAPER_COMMANDS: Array<[string, string]> = [
    ['/paper', 'Today\u2019s digest \u2014 new arXiv matches plus recommendations'],
    ['/save <n>', 'Save the nth paper from the last digest into the library'],
    ['/reading', 'What you are part-way through, with page progress'],
    ['/help', 'The command list for the paper bot'],
  ];

  $effect(() => {
    void api
      .getPaperTelegramStatus()
      .then((v) => v && (paperTg = v))
      .catch(() => {});
  });

  type ChipList = 'PaperKeywords' | 'PaperCategories' | 'PaperTopVenues';

  function addChip(list: ChipList, raw: string) {
    if (!draft) return;
    const v = raw.trim().replace(/,+$/, '');
    if (!v) return;
    const cur = draft[list] ?? [];
    if (!cur.includes(v)) draft[list] = [...cur, v];
  }

  function removeChip(list: ChipList, v: string) {
    if (!draft) return;
    draft[list] = (draft[list] ?? []).filter((x) => x !== v);
  }

  async function pairPaper() {
    paperPairing = true;
    try {
      const chatID = await api.pairPaperTelegram();
      paperTg = await api.getPaperTelegramStatus();
      if (draft) draft.PaperTelegramChatID = chatID;
      toast(`Paper bot paired with chat ${chatID}`, 'success');
    } catch (err) {
      toast(`Pairing failed: ${errMsg(err)}`, 'error');
    } finally {
      paperPairing = false;
    }
  }

  async function sendPaperTest() {
    paperTesting = true;
    try {
      await api.sendPaperTestTelegram();
    } catch (err) {
      toast(errMsg(err), 'error');
    } finally {
      paperTesting = false;
    }
  }

  // ------------------------------------------------------------- study

  const MODEL_KEY = 'nussync.study.model';

  const MODELS = [
    { v: 'opus', label: 'Opus — slowest, strongest' },
    { v: 'sonnet', label: 'Sonnet — the balanced default' },
    { v: 'haiku', label: 'Haiku — fastest, cheapest' },
  ];

  let study = $state<StudyStatus | null>(null);
  let studyLoading = $state(true);
  let studyModel = $state<string>(lsGet(MODEL_KEY, 'sonnet'));

  $effect(() => {
    lsSet(MODEL_KEY, studyModel);
  });

  async function loadStudy() {
    studyLoading = true;
    try {
      study = await api.getStudyStatus();
    } catch (err) {
      study = { CLIFound: false, Version: '', LoggedIn: false, Error: errMsg(err), Models: null };
    } finally {
      studyLoading = false;
    }
  }

  $effect(() => {
    void loadStudy();
  });

  const TELEGRAM_COMMANDS: Array<[string, string]> = [
    ['/due', 'Next 10 unsubmitted deadlines, grouped by how soon they are'],
    ['/new', 'Files changed in the last 24 hours, grouped by course'],
    ['/files <query>', 'Full-text search across your library, top 8 hits'],
    ['/sync', 'Kick off a sync and reply with the summary when it lands'],
    ['/grades', 'Last 10 graded submissions, with the class mean when known'],
    ['/help', 'The command list (so is anything else it does not recognise)'],
  ];

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
      // The live theme store wins over whatever the backend last stored, so the
      // dropdown always shows what the window is actually rendering.
      draft.Theme = $theme;
      baseline = structuredClone($state.snapshot(draft)) as Settings;
      loaded = true;
    }
  });

  $effect(() => {
    void api
      .getTelegramStatus()
      .then((v) => v && (tg = v))
      .catch(() => {});
  });

  // Deep, key-order-insensitive: re-assigning a field through Svelte's state
  // proxy can reorder keys, and a stringify comparison then claims "Unsaved
  // changes" for an edit that never happened.
  const dirty = $derived(loaded && draft !== null && baseline !== null && !deepEqual(draft, baseline));

  async function save() {
    if (!draft || !loaded) return;
    saving = true;
    try {
      // Send the CURRENT stored settings with the draft laid over them, never
      // the draft alone: any field this build does not know about (a newer
      // backend, a section that never rendered) then survives the round trip.
      const current = $settings ? (structuredClone($state.snapshot($settings)) as Settings) : null;
      const payload = {
        ...(current ?? {}),
        ...(structuredClone($state.snapshot(draft)) as Settings),
      } as Settings;
      await api.saveSettings(payload);
      settings.set(payload);
      if (payload.Theme === 'light' || payload.Theme === 'dark' || payload.Theme === 'system') {
        theme.set(payload.Theme);
      }
      baseline = structuredClone($state.snapshot(draft)) as Settings;
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
    baseline = structuredClone($state.snapshot(draft)) as Settings;
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

  /** Sidebar-only: hidden courses keep syncing and stay in Files/Deadlines. */
  function hideNonAcademicCourses() {
    const n = hideNonAcademic($courses);
    toast(n === 0 ? 'No non-academic courses to hide' : `Hid ${n} course${n === 1 ? '' : 's'} from the sidebar`, n === 0 ? 'info' : 'success');
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
        <button class="btn primary" onclick={save} disabled={!loaded || !dirty || saving}>
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
            {/if}
          </div>
          {#if canvasError}
            <div class="err-strip" role="alert">
              <Icon name="alert" size={12} />
              <span class="err-text">{canvasError}</span>
            </div>
          {/if}
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
            <span class="lbl">Sidebar</span>
            <div class="with-btn">
              <button class="btn" type="button" onclick={hideNonAcademicCourses}>
                <Icon name="eyeOff" size={13} /> Hide non-academic courses
              </button>
            </div>
            <span class="help">
              Collapses courses with no term (or “Non-Academic”) under the sidebar’s Hidden row. They keep
              syncing and still show in Files and Deadlines.
            </span>
          </div>

          <div class="field">
            <span class="lbl">Courses to sync</span>
            <div class="course-grid">
              <!-- Negative ids are synthetic (the "Papers" pseudo-course, id -1).
                   Canvas never syncs them, so a toggle here would do nothing. -->
              {#each $courses.filter((c) => c.ID > 0) as c (c.ID)}
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
                <div class="tg-title">Paired with {tg.BotName || '@nuscanvassync_bot'}</div>
                <div class="tg-sub">Chat ID <span class="mono">{tg.ChatID}</span></div>
              {:else}
                <div class="tg-title">Not paired yet</div>
                <div class="tg-sub">
                  Open Telegram, send <span class="mono">/start</span> to
                  <span class="mono">{tg.BotName || '@nuscanvassync_bot'}</span>, then click Pair — NUSSync waits up to 60&nbsp;seconds
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

          <div class="field">
            <span class="lbl">Bot commands</span>
            <div class="cmd-card">
              {#each TELEGRAM_COMMANDS as [cmd, what] (cmd)}
                <div class="cmd-row">
                  <span class="mono cmd">{cmd}</span>
                  <span class="cmd-what muted">{what}</span>
                </div>
              {/each}
            </div>
            <span class="help">Send these to the bot from any device. Messages from other chats are ignored.</span>
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

          <label class="switch-row">
            <input type="checkbox" bind:checked={draft.NotifyDesktop} />
            <span class="track"><span class="knob"></span></span>
            <span class="switch-text">
              <span class="switch-title">Desktop notifications</span>
              <span class="help">
                Windows toast after a sync brings new files — “3 new files in CS4246, MA3236”. Clicking it
                brings NUSSync to the front.
              </span>
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
              value={$theme}
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

          <label class="field narrow">
            <span class="lbl">Global hotkey</span>
            <input
              class="input mono"
              type="text"
              bind:value={draft.Hotkey}
              placeholder="ctrl+shift+n"
              spellcheck="false"
              autocomplete="off"
            />
            <span class="help">
              Shows or hides the window from anywhere. Modifiers <span class="mono">ctrl alt shift win</span>
              joined by <span class="mono">+</span>, then one key (a letter, a digit, <span class="mono">f1</span>–<span
                class="mono">f12</span
              >, or a named key). Leave it empty to disable. Best effort — another app may already own the
              combination.
            </span>
          </label>

          <label class="switch-row">
            <input type="checkbox" bind:checked={draft.LaunchAtLogin} />
            <span class="track"><span class="knob"></span></span>
            <span class="switch-text">
              <span class="switch-title">Launch at login</span>
              <span class="help">Start NUSSync minimised when Windows starts.</span>
            </span>
          </label>

          <!-- Layout: local to this machine, so nothing here touches `draft`. -->
          <div class="sub-head">Layout</div>

          <label class="field narrow">
            <span class="lbl">Sidebar</span>
            <select
              class="select"
              value={$sidebarMode}
              onchange={(e) => sidebarMode.set(e.currentTarget.value as SidebarMode)}
            >
              <option value="auto">Auto (rail on a narrow window)</option>
              <option value="expanded">Always expanded</option>
              <option value="rail">Always a rail</option>
            </select>
            <span class="help">
              Ctrl+B collapses it to the icon rail. An explicit choice reverts to Auto once the window is wide again.
            </span>
          </label>

          <label class="field narrow">
            <span class="lbl">Chat placement</span>
            <select
              class="select"
              value={$chatMode}
              onchange={(e) => chatMode.set(e.currentTarget.value as ChatMode)}
            >
              <option value="docked">Docked column (shrinks the view)</option>
              <option value="overlay">Overlay (floats over the view)</option>
            </select>
            <span class="help">
              Docked keeps the file you are asking about visible beside the chat. On a narrow window the chat
              falls back to the overlay until there is room again.
            </span>
          </label>

          <div class="field start">
            <button
              class="btn"
              onclick={() => {
                resetLayout();
                toast('Layout reset', 'success');
              }}
            >
              <Icon name="sort" size={13} /> Reset layout
            </button>
            <span class="help">
              Sidebar mode and width, folder tree visibility and width, chat placement and width, preview focus.
            </span>
          </div>
        </div>
      </section>

      <!-- ------------------------------------------------------------ Study -->
      <section class="card section">
        <div class="section-head">
          <h2>Study</h2>
          <p class="muted">Overviews, quizzes and flashcards run through your local Claude Code CLI.</p>
        </div>
        <div class="fields">
          <div class="study-card" class:ready={!!study?.CLIFound && !!study?.LoggedIn}>
            <div class="study-icon">
              <Icon name={study?.CLIFound && study?.LoggedIn ? 'checkCircle' : 'alert'} size={16} />
            </div>
            <div class="study-body">
              {#if studyLoading}
                <div class="study-title"><span class="spinner"></span> Checking the CLI…</div>
              {:else if !study?.CLIFound}
                <div class="study-title">Claude Code CLI not found</div>
                <div class="study-sub">
                  Install it and make sure <span class="mono">claude</span> is on your PATH, then re-check.
                  Nothing in the Study view will run until it is.
                  {#if study?.Error}<span class="err-text block faint">{study.Error}</span>{/if}
                </div>
              {:else if !study?.LoggedIn}
                <div class="study-title">CLI found, but not signed in</div>
                <div class="study-sub">
                  Run <span class="mono">claude auth login</span> in a terminal, then re-check.
                  {#if study?.Error}<span class="err-text block faint">{study.Error}</span>{/if}
                </div>
              {:else}
                <div class="study-title">Ready</div>
                <div class="study-sub">
                  <span class="mono">claude</span> {study.Version || ''} · signed in. Generation is always on
                  demand — nothing runs by itself.
                </div>
              {/if}
            </div>
            <div class="study-actions">
              <button class="btn" onclick={loadStudy} disabled={studyLoading}>
                {#if studyLoading}<span class="spinner"></span>{:else}<Icon name="sync" size={13} />{/if}
                Re-check
              </button>
            </div>
          </div>

          <label class="field narrow">
            <span class="lbl">Default model</span>
            <select class="select" bind:value={studyModel}>
              {#each MODELS as m (m.v)}
                <option value={m.v} disabled={!!study?.Models && !study.Models.includes(m.v)}>{m.label}</option>
              {/each}
            </select>
            <span class="help">Used for new overviews, quizzes, asks and flashcards. Saved on this machine only.</span>
          </label>
        </div>
      </section>

      <!-- ----------------------------------------------------------- Papers -->
      <section class="card section">
        <div class="section-head">
          <h2>Papers</h2>
          <p class="muted">Search topics, the daily digest, and the separate Telegram bot that delivers it.</p>
        </div>
        <div class="fields">
          <div class="field">
            <span class="lbl">Topics</span>
            <div class="chips">
              {#each draft.PaperKeywords ?? [] as k (k)}
                <span class="chip accent removable">
                  {k}
                  <button class="chip-x" onclick={() => removeChip('PaperKeywords', k)} aria-label="Remove {k}" type="button">
                    <Icon name="x" size={10} />
                  </button>
                </span>
              {/each}
              <input
                class="chip-input"
                type="text"
                placeholder="add a topic"
                bind:value={keywordInput}
                onkeydown={(e) => {
                  if (e.key === 'Enter' || e.key === ',') {
                    e.preventDefault();
                    addChip('PaperKeywords', keywordInput);
                    keywordInput = '';
                  }
                }}
                onblur={() => {
                  addChip('PaperKeywords', keywordInput);
                  keywordInput = '';
                }}
                spellcheck="false"
              />
            </div>
            <span class="help">Matched against new arXiv titles and abstracts for the digest.</span>
          </div>

          <div class="field">
            <span class="lbl">arXiv categories</span>
            <div class="chips">
              {#each draft.PaperCategories ?? [] as c (c)}
                <span class="chip removable">
                  {c}
                  <button class="chip-x" onclick={() => removeChip('PaperCategories', c)} aria-label="Remove {c}" type="button">
                    <Icon name="x" size={10} />
                  </button>
                </span>
              {/each}
              <input
                class="chip-input"
                type="text"
                placeholder="add cs.CL"
                bind:value={categoryInput}
                onkeydown={(e) => {
                  if (e.key === 'Enter' || e.key === ',') {
                    e.preventDefault();
                    addChip('PaperCategories', categoryInput);
                    categoryInput = '';
                  }
                }}
                onblur={() => {
                  addChip('PaperCategories', categoryInput);
                  categoryInput = '';
                }}
                spellcheck="false"
              />
            </div>
          </div>

          <div class="field">
            <span class="lbl">Top venues</span>
            <div class="chips">
              {#each draft.PaperTopVenues ?? [] as v (v)}
                <span class="chip removable">
                  {v}
                  <button class="chip-x" onclick={() => removeChip('PaperTopVenues', v)} aria-label="Remove {v}" type="button">
                    <Icon name="x" size={10} />
                  </button>
                </span>
              {/each}
              <input
                class="chip-input"
                type="text"
                placeholder="add NeurIPS"
                bind:value={venueInput}
                onkeydown={(e) => {
                  if (e.key === 'Enter' || e.key === ',') {
                    e.preventDefault();
                    addChip('PaperTopVenues', venueInput);
                    venueInput = '';
                  }
                }}
                onblur={() => {
                  addChip('PaperTopVenues', venueInput);
                  venueInput = '';
                }}
                spellcheck="false"
              />
            </div>
            <span class="help">
              Papers from these venues get the gold badge and rank first. Anything else with a
              detected venue counts as published; workshop papers rank one tier lower.
            </span>
          </div>

          <label class="switch-row">
            <input type="checkbox" bind:checked={draft.PaperPreferPublished} />
            <span class="track"><span class="knob"></span></span>
            <span class="switch-text">
              <span class="switch-title">Prefer published papers</span>
              <span class="help">Rank peer-reviewed work above preprints in search, the digest and recommendations.</span>
            </span>
          </label>

          <label class="field narrow">
            <span class="lbl">Digest hour</span>
            <select class="select" bind:value={draft.PaperDigestHour}>
              {#each DIGEST_HOURS as h (h.v)}
                <option value={h.v}>{h.label}</option>
              {/each}
            </select>
            <span class="help">Local time the paper-of-the-day message goes out.</span>
          </label>

          <label class="switch-row">
            <input type="checkbox" bind:checked={draft.NotifyPapers} />
            <span class="track"><span class="knob"></span></span>
            <span class="switch-text">
              <span class="switch-title">Daily paper digest</span>
              <span class="help">New arXiv matches plus a few recommendations, once a day.</span>
            </span>
          </label>

          <label class="field">
            <span class="lbl">Paper bot token</span>
            <span class="with-btn">
              <input
                class="input"
                type={showPaperToken ? 'text' : 'password'}
                bind:value={draft.PaperTelegramToken}
                placeholder="123456:ABC-DEF…"
                spellcheck="false"
                autocomplete="off"
              />
              <button class="reveal" onclick={() => (showPaperToken = !showPaperToken)} aria-label="Toggle token visibility" type="button">
                <Icon name={showPaperToken ? 'eyeOff' : 'eye'} size={14} />
              </button>
            </span>
            <span class="help">A second bot, separate from the course bot — papers only.</span>
          </label>

          <div class="tg-card" class:paired={paperTg.Configured}>
            <div class="tg-icon"><Icon name="book" size={16} /></div>
            <div class="tg-body">
              {#if paperTg.Configured}
                <div class="tg-title">Paired with {paperTg.BotName || 'the paper bot'}</div>
                <div class="tg-sub">Chat ID <span class="mono">{paperTg.ChatID}</span></div>
              {:else}
                <div class="tg-title">Paper bot not paired yet</div>
                <div class="tg-sub">
                  Send <span class="mono">/start</span> to <span class="mono">{paperTg.BotName || 'the paper bot'}</span>,
                  then click Pair — NUSSync waits up to 60&nbsp;seconds for your message.
                </div>
              {/if}
            </div>
            <div class="tg-actions">
              <button class="btn" onclick={pairPaper} disabled={paperPairing}>
                {#if paperPairing}<span class="spinner"></span> Waiting…{:else}{paperTg.Configured ? 'Re-pair' : 'Pair'}{/if}
              </button>
              <button class="btn" onclick={sendPaperTest} disabled={!paperTg.Configured || paperTesting}>
                {#if paperTesting}<span class="spinner"></span>{:else}<Icon name="send" size={13} />{/if}
                Send test
              </button>
            </div>
          </div>

          <div class="field">
            <span class="lbl">Paper bot commands</span>
            <div class="cmd-card">
              {#each PAPER_COMMANDS as [cmd, what] (cmd)}
                <div class="cmd-row">
                  <span class="mono cmd">{cmd}</span>
                  <span class="cmd-what muted">{what}</span>
                </div>
              {/each}
            </div>
          </div>
        </div>
      </section>

      <!-- -------------------------------------------------------------- Pet -->
      <section class="card section">
        <div class="section-head">
          <h2>Pet</h2>
          <p class="muted">The small creature that reacts to your deadlines and sync.</p>
        </div>
        <div class="fields">
          <label class="switch-row">
            <input type="checkbox" checked={$petEnabled} onchange={(e) => petEnabled.set(e.currentTarget.checked)} />
            <span class="track"><span class="knob"></span></span>
            <span class="switch-text">
              <span class="switch-title">Show pet</span>
              <span class="help">It reacts to your deadlines and sync. Saved on this machine only.</span>
            </span>
          </label>

          <label class="field narrow">
            <span class="lbl">Name</span>
            <input
              class="input"
              type="text"
              maxlength="18"
              placeholder="Nibble"
              value={$petName}
              disabled={!$petEnabled}
              oninput={(e) => petName.set(e.currentTarget.value)}
              onblur={(e) => petName.set(e.currentTarget.value.trim() || 'Nibble')}
              spellcheck="false"
            />
            <span class="help">Used in a few of its remarks.</span>
          </label>

          <div class="field">
            <span class="lbl">Placement</span>
            <div class="seg" role="group" aria-label="Pet placement">
              {#each PET_MODES as m (m.id)}
                <button
                  class="seg-btn"
                  class:on={$petMode === m.id}
                  disabled={!$petEnabled}
                  aria-pressed={$petMode === m.id}
                  onclick={() => petMode.set(m.id)}
                  type="button"
                >
                  {m.label}
                </button>
              {/each}
            </div>
            <span class="help">{PET_MODES.find((m) => m.id === $petMode)?.help ?? ''}</span>
          </div>

          {#if $petMode !== 'dock'}
            <div class="field start">
              <button
                class="btn sm"
                type="button"
                disabled={!$petEnabled}
                onclick={() => {
                  resetPetPos();
                  toast('Pet moved back to its corner');
                }}
              >
                Reset position
              </button>
              <span class="help">Puts the floating pet back in the bottom-right corner.</span>
            </div>
          {/if}
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

  .field.start {
    align-items: flex-start;
  }

  .sub-head {
    margin-top: 4px;
    padding-top: 13px;
    border-top: 1px solid var(--border);
    font-size: 11.5px;
    font-weight: 600;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    color: var(--text-faint);
  }

  /* segmented control (pet placement) */

  .seg {
    display: inline-flex;
    align-self: flex-start;
    padding: 2px;
    gap: 2px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
  }

  .seg-btn {
    height: 26px;
    padding: 0 12px;
    border-radius: 4px;
    color: var(--text-muted);
    font-size: 12.5px;
    font-weight: 500;
    transition: background var(--t), color var(--t);
  }

  .seg-btn:hover:not(:disabled):not(.on) {
    background: var(--bg-hover);
    color: var(--text);
  }

  .seg-btn.on {
    background: var(--bg-elevated);
    color: var(--text);
    font-weight: 570;
    box-shadow: 0 1px 2px rgba(15, 15, 25, 0.08);
  }

  .seg-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
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

  /*
   * Error text is arbitrary length — a Canvas body, a Claude CLI failure — so
   * every inline error in this view wraps and scrolls instead of stretching
   * the card off-screen. `.chip` cannot do this: it is a fixed 21px nowrap pill.
   */
  .err-strip {
    display: flex;
    align-items: flex-start;
    gap: 6px;
    margin-top: 8px;
    padding: 7px 9px;
    border-radius: 6px;
    background: var(--red-soft);
    color: var(--red);
    font-size: 12px;
    line-height: 1.45;
  }

  .err-strip :global(svg) {
    flex: 0 0 auto;
    margin-top: 2px;
  }

  .err-text {
    min-width: 0;
    max-height: 8.7em;
    overflow-y: auto;
    white-space: pre-wrap;
    word-break: break-word;
    overflow-wrap: anywhere;
  }

  .err-text.block {
    display: block;
    margin-top: 4px;
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

  .cmd-card {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    overflow: hidden;
  }

  .cmd-row {
    display: flex;
    align-items: baseline;
    gap: 10px;
    padding: 7px 11px;
    border-bottom: 1px solid var(--border);
  }

  .cmd-row:last-child {
    border-bottom: none;
  }

  .cmd {
    flex: none;
    min-width: 108px;
    color: var(--accent-text);
    font-weight: 550;
  }

  .cmd-what {
    font-size: 12px;
    min-width: 0;
  }

  .study-card {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    padding: 13px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-subtle);
  }

  .study-card.ready {
    border-color: var(--green);
    background: var(--green-soft);
  }

  .study-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 30px;
    height: 30px;
    flex: none;
    border-radius: var(--radius-sm);
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    color: var(--amber);
  }

  .study-card.ready .study-icon {
    color: var(--green);
  }

  .study-body {
    flex: 1;
    min-width: 0;
  }

  .study-title {
    display: flex;
    align-items: center;
    gap: 7px;
    font-size: 13px;
    font-weight: 550;
  }

  .study-sub {
    font-size: 12px;
    color: var(--text-muted);
    margin-top: 2px;
    line-height: 1.55;
  }

  .study-actions {
    flex: none;
  }

  @media (max-width: 780px) {
    .grid2,
    .course-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
