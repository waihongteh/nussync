<script lang="ts">
  /**
   * Study — on demand only. Nothing here runs by itself: every generation is a
   * button press that queues one Claude Code CLI job, tracked through the
   * `study:job` / `study:progress` events.
   *
   * Left: a course/file picker (PDFs first, page count fetched lazily on
   * select, multi-select for quiz and ask). Right: Overview / Quiz / Ask /
   * Flashcards, each showing whatever is already cached before you generate
   * anything new.
   */
  import { api, errMsg, on } from '../api';
  import Icon from '../components/Icon.svelte';
  import Viewer from '../components/Viewer.svelte';
  import { courses, courseByID, setContext, studyPreselect, toast } from '../stores';
  import type { AskResult, FileNode, Flashcard, Overview, Quiz, QuizAttempt, StudyJob, StudyStatus } from '../types';
  import { fileKind, fmtBytes, lsGet, lsSet, markdownToHTML, relTime } from '../util';
  import { recordCorrect, recordFlashcard, recordOverview, recordQuizDone } from '../quest';

  const KIND_ICON: Record<string, string> = {
    pdf: 'fileText',
    doc: 'fileText',
    slides: 'slides',
    sheet: 'sheet',
    image: 'image',
    video: 'video',
    archive: 'archive',
    code: 'code',
    file: 'file',
  };

  const MODEL_KEY = 'nussync.study.model';
  const TABS = [
    { id: 'overview', label: 'Overview' },
    { id: 'quiz', label: 'Quiz' },
    { id: 'ask', label: 'Ask' },
    { id: 'flashcards', label: 'Flashcards' },
  ] as const;
  type Tab = (typeof TABS)[number]['id'];

  const GRADES: Array<{ g: number; label: string; hint: string }> = [
    { g: 0, label: 'Again', hint: 'back in a minute' },
    { g: 1, label: 'Hard', hint: 'shorter interval' },
    { g: 2, label: 'Good', hint: 'normal interval' },
    { g: 3, label: 'Easy', hint: 'longer interval' },
  ];

  // ------------------------------------------------------------- state

  let status = $state<StudyStatus | null>(null);
  let statusLoading = $state(true);
  let model = $state<string>(lsGet(MODEL_KEY, 'sonnet'));

  let filesByCourse = $state<Map<number, FileNode[]>>(new Map());
  let treeLoading = $state(true);
  let filter = $state('');
  let collapsed = $state<Set<number>>(new Set());
  let selectedIDs = $state<number[]>([]);
  let pageCounts = $state<Map<number, number>>(new Map());

  let tab = $state<Tab>('overview');
  /** In-app preview of the primary selection, on the overview tab. */
  let previewOpen = $state(false);

  let job = $state<StudyJob | null>(null);
  let jobLine = $state('');
  let clock = $state(Date.now());

  let overview = $state<Overview | null>(null);
  let overviewLoading = $state(false);
  let quizzes = $state<Quiz[]>([]);
  let asks = $state<AskResult[]>([]);
  let cards = $state<Flashcard[]>([]);
  let attempts = $state<QuizAttempt[]>([]);

  let quizN = $state(5);
  let question = $state('');

  // quiz-taking
  let taking = $state<Quiz | null>(null);
  let qIndex = $state(0);
  let answers = $state<Record<number, string>>({});
  let revealed = $state(false);
  let shortDraft = $state('');
  let result = $state<QuizAttempt | null>(null);
  let submitting = $state(false);

  // flashcard review
  let cardIndex = $state(0);
  let flipped = $state(false);

  const ready = $derived(!!status?.CLIFound && !!status?.LoggedIn);
  const busy = $derived(job !== null && (job.Status === 'queued' || job.Status === 'running'));

  const allFiles = $derived.by(() => {
    const out: FileNode[] = [];
    for (const list of filesByCourse.values()) out.push(...list);
    return out;
  });

  const fileByID = $derived.by(() => {
    const m = new Map<number, FileNode>();
    for (const f of allFiles) m.set(f.ID, f);
    return m;
  });

  const selectedFiles = $derived(selectedIDs.map((id) => fileByID.get(id)).filter(Boolean) as FileNode[]);
  const primary = $derived(selectedFiles[0] ?? null);

  $effect(() => {
    lsSet(MODEL_KEY, model);
  });

  // -------------------------------------------------------------- load

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

  function flatten(nodes: FileNode[], out: FileNode[] = []): FileNode[] {
    for (const n of nodes) {
      if (n.IsDir) flatten(n.Children ?? [], out);
      else out.push(n);
    }
    return out;
  }

  const STUDYABLE = new Set(['pdf', 'doc', 'slides', 'sheet', 'code']);

  async function loadTrees(list: typeof $courses) {
    treeLoading = true;
    try {
      const trees = await Promise.all(list.map((c) => api.getTree(c.ID).catch(() => [] as FileNode[])));
      const map = new Map<number, FileNode[]>();
      list.forEach((c, i) => {
        const files = flatten(trees[i] ?? [])
          .filter((f) => STUDYABLE.has(fileKind(f.Name)))
          .sort((a, b) => {
            const ap = /\.pdf$/i.test(a.Name) ? 0 : 1;
            const bp = /\.pdf$/i.test(b.Name) ? 0 : 1;
            if (ap !== bp) return ap - bp;
            return a.Name.localeCompare(b.Name);
          });
        if (files.length) map.set(c.ID, files);
      });
      filesByCourse = map;
    } finally {
      treeLoading = false;
    }
  }

  $effect(() => {
    void loadStatus();
  });

  /**
   * Papers hands over a file id through `studyPreselect` before navigating
   * here. Consume it once — leaving it set would re-select on every mount.
   */
  $effect(() => {
    const id = $studyPreselect;
    if (!id) return;
    studyPreselect.set(0);
    if (!selectedIDs.includes(id)) selectedIDs = [id, ...selectedIDs];
  });

  // Keep the chat panel pointed at whatever Study is working on.
  $effect(() => {
    const f = primary;
    if (f) setContext(f.ID, '', f.Name);
  });

  $effect(() => {
    const list = $courses;
    if (list.length) void loadTrees(list);
    else treeLoading = false;
  });

  // Elapsed-time ticker, only while a job is in flight.
  $effect(() => {
    if (!busy) return;
    const h = setInterval(() => (clock = Date.now()), 250);
    return () => clearInterval(h);
  });

  $effect(() => {
    const offJob = on('study:job', (j: StudyJob) => {
      if (!j) return;
      if (job && j.ID !== job.ID && (j.Status === 'queued' || j.Status === 'running')) return;
      job = j;
      if (j.Progress) jobLine = j.Progress;
      if (j.Status === 'done') {
        // Quest xp for a finished overview (quiz/flashcard xp is earned by use).
        if (j.Kind === 'overview') recordOverview();
        void refreshFor(j.Kind);
        // Leave the finished job on screen briefly so the line does not flash.
        const id = j.ID;
        setTimeout(() => {
          if (job?.ID === id) job = null;
        }, 1400);
      } else if (j.Status === 'error') {
        toast(j.Error || 'Study job failed', 'error');
      } else if (j.Status === 'cancelled') {
        const id = j.ID;
        setTimeout(() => {
          if (job?.ID === id) job = null;
        }, 800);
      }
    });
    const offProg = on('study:progress', (p: { JobID: string; Text: string }) => {
      if (p?.Text && (!job || p.JobID === job.ID)) jobLine = p.Text;
    });
    return () => {
      offJob();
      offProg();
    };
  });

  const elapsed = $derived.by(() => {
    if (!job?.StartedAt) return '';
    const t = Date.parse(job.StartedAt);
    if (!isFinite(t)) return '';
    return `${Math.max(0, Math.round((clock - t) / 1000))}s`;
  });

  async function refreshFor(kind: string) {
    if (kind === 'overview') await loadOverview();
    else if (kind === 'quiz') await loadQuizzes();
    else if (kind === 'ask') await loadAsks();
    else if (kind === 'flashcards') await loadCards();
  }

  // ------------------------------------------------------- tab loaders

  async function loadOverview() {
    if (!primary) {
      overview = null;
      return;
    }
    overviewLoading = true;
    try {
      const o = await api.getOverview(primary.ID);
      overview = o && o.Markdown ? o : null;
    } catch (err) {
      console.error('[study] overview failed', err);
      overview = null;
    } finally {
      overviewLoading = false;
    }
  }

  async function loadQuizzes() {
    try {
      quizzes = (await api.getQuizzes(primary?.ID ?? 0)) ?? [];
    } catch (err) {
      console.error('[study] quizzes failed', err);
      quizzes = [];
    }
  }

  async function loadAsks() {
    try {
      asks = (await api.getAsks(primary?.ID ?? 0)) ?? [];
    } catch (err) {
      console.error('[study] asks failed', err);
      asks = [];
    }
  }

  async function loadCards() {
    try {
      cards = (await api.getDueFlashcards(50)) ?? [];
      cardIndex = 0;
      flipped = false;
    } catch (err) {
      console.error('[study] flashcards failed', err);
      cards = [];
    }
  }

  // Reload whatever the visible tab shows when the tab or the selection changes.
  $effect(() => {
    void primary;
    const t = tab;
    if (t === 'overview') void loadOverview();
    else if (t === 'quiz') void loadQuizzes();
    else if (t === 'ask') void loadAsks();
    else if (t === 'flashcards') void loadCards();
  });

  // ------------------------------------------------------------ picker

  function toggleCourse(id: number) {
    const next = new Set(collapsed);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    collapsed = next;
  }

  async function toggleFile(f: FileNode) {
    if (selectedIDs.includes(f.ID)) {
      selectedIDs = selectedIDs.filter((id) => id !== f.ID);
      return;
    }
    selectedIDs = [...selectedIDs, f.ID];
    if (!pageCounts.has(f.ID)) {
      try {
        const n = await api.getFilePageCount(f.ID);
        const next = new Map(pageCounts);
        next.set(f.ID, n);
        pageCounts = next;
      } catch {
        /* page count is a nicety, never an error the user needs */
      }
    }
  }

  const visibleCourses = $derived.by(() => {
    const q = filter.trim().toLowerCase();
    return $courses
      .map((c) => {
        const files = (filesByCourse.get(c.ID) ?? []).filter(
          (f) => !q || f.Name.toLowerCase().includes(q) || f.RelPath.toLowerCase().includes(q),
        );
        return { course: c, files };
      })
      .filter((g) => g.files.length > 0);
  });

  // --------------------------------------------------------- generate

  async function guarded(label: string, fn: () => Promise<string>) {
    if (!ready || busy) return;
    try {
      const id = await fn();
      jobLine = 'Queued';
      job = {
        ID: id,
        Kind: label,
        FileIDs: [...selectedIDs],
        Status: 'queued',
        Progress: 'Queued',
        Error: '',
        StartedAt: new Date().toISOString(),
        FinishedAt: '',
        Model: model,
        CostUSD: 0,
      };
      clock = Date.now();
    } catch (err) {
      toast(errMsg(err), 'error');
    }
  }

  const genOverview = () => {
    if (!primary) return;
    void guarded('overview', () => api.startOverview(primary.ID, model));
  };

  const genQuiz = () => {
    if (!selectedIDs.length) return;
    void guarded('quiz', () => api.startQuiz([...selectedIDs], model, quizN));
  };

  const genAsk = () => {
    const q = question.trim();
    if (!q || !selectedIDs.length) return;
    void guarded('ask', () => api.startAsk([...selectedIDs], q, model));
    question = '';
  };

  const genCards = () => {
    if (!primary) return;
    void guarded('flashcards', () => api.startFlashcards(primary.ID, model, 0));
  };

  async function cancelJob() {
    if (!job) return;
    try {
      await api.cancelStudyJob(job.ID);
    } catch (err) {
      toast(errMsg(err), 'error');
    }
  }

  // ------------------------------------------------------- taking a quiz

  async function startTaking(q: Quiz) {
    taking = q;
    qIndex = 0;
    answers = {};
    revealed = false;
    shortDraft = '';
    result = null;
    try {
      attempts = (await api.getQuizAttempts(q.ID)) ?? [];
    } catch {
      attempts = [];
    }
  }

  function exitTaking() {
    taking = null;
    result = null;
    revealed = false;
  }

  const current = $derived(taking?.Questions?.[qIndex] ?? null);
  const total = $derived(taking?.Questions?.length ?? 0);

  /** When the visible question first appeared — quest xp pays a speed bonus. */
  let shownAt = $state(Date.now());
  /** Question ids already awarded, so a re-render cannot double-pay. */
  let awarded = new Set<number>();

  $effect(() => {
    void current?.ID;
    shownAt = Date.now();
  });

  function awardCorrect(id: number) {
    if (awarded.has(id)) return;
    awarded.add(id);
    recordCorrect(Date.now() - shownAt);
  }

  function pickOption(letter: string) {
    if (!current || revealed) return;
    answers = { ...answers, [current.ID]: letter };
    revealed = true;
    if (letter.toUpperCase() === current.Answer.trim().toUpperCase()) awardCorrect(current.ID);
  }

  function revealShort() {
    if (!current) return;
    revealed = true;
  }

  function selfMark(correct: boolean) {
    if (!current) return;
    answers = { ...answers, [current.ID]: correct ? 'correct' : 'wrong' };
    if (correct) awardCorrect(current.ID);
  }

  async function next() {
    if (!taking) return;
    if (!revealed) {
      if (current?.Type === 'short') revealShort();
      return;
    }
    if (current?.Type === 'short' && answers[current.ID] === undefined) return;
    if (qIndex + 1 < total) {
      qIndex += 1;
      revealed = false;
      shortDraft = '';
      return;
    }
    await submit();
  }

  async function submit() {
    if (!taking || submitting) return;
    submitting = true;
    try {
      result = await api.submitQuizAttempt({
        QuizID: taking.ID,
        Answers: { ...answers },
        Score: 0,
        Total: total,
        TakenAt: '',
      });
      attempts = (await api.getQuizAttempts(taking.ID)) ?? [];
      recordQuizDone();
      toast(`Quiz done — ${result.Score}/${result.Total}`, 'success');
    } catch (err) {
      toast(errMsg(err), 'error');
    } finally {
      submitting = false;
    }
  }

  const letterOf = (i: number) => String.fromCharCode(65 + i);

  function onKey(e: KeyboardEvent) {
    if (!taking || result) return;
    const target = e.target as HTMLElement | null;
    if (target && (target.tagName === 'TEXTAREA' || target.tagName === 'INPUT')) return;
    if (e.key >= '1' && e.key <= '4' && current?.Type === 'mcq' && !revealed) {
      const i = Number(e.key) - 1;
      if (i < (current.Options ?? []).length) {
        e.preventDefault();
        pickOption(letterOf(i));
      }
      return;
    }
    if (e.key === 'Enter') {
      e.preventDefault();
      void next();
    }
  }

  // ---------------------------------------------------------- flashcards

  const currentCard = $derived(cards[cardIndex] ?? null);

  async function reviewCard(grade: number) {
    const c = currentCard;
    if (!c) return;
    try {
      await api.reviewFlashcard(c.ID, grade);
      recordFlashcard(grade);
    } catch (err) {
      toast(errMsg(err), 'error');
    }
    flipped = false;
    if (cardIndex + 1 < cards.length) cardIndex += 1;
    else void loadCards();
  }

  const overviewHTML = $derived(overview?.Markdown ? markdownToHTML(overview.Markdown) : '');
</script>

<svelte:window onkeydown={onKey} />

<div class="study">
  <!-- ------------------------------------------------------------ picker -->
  <aside class="picker">
    <div class="picker-top">
      <div class="search">
        <Icon name="search" size={13} />
        <input class="q" type="text" placeholder="Filter files…" bind:value={filter} spellcheck="false" />
        {#if filter}
          <button class="clear" onclick={() => (filter = '')} aria-label="Clear filter"><Icon name="x" size={11} /></button>
        {/if}
      </div>
      <div class="sel-line">
        <span class="faint">
          {selectedIDs.length === 0
            ? 'Nothing selected'
            : `${selectedIDs.length} selected${primary ? ` · ${primary.Name}` : ''}`}
        </span>
        {#if selectedIDs.length > 0}
          <button class="linkish" onclick={() => (selectedIDs = [])}>Clear</button>
        {/if}
      </div>
    </div>

    <div class="picker-scroll">
      {#if treeLoading}
        <div class="p-loading"><span class="spinner"></span> Loading files…</div>
      {:else}
        {#each visibleCourses as g (g.course.ID)}
          <div class="pc">
            <button class="pc-head" onclick={() => toggleCourse(g.course.ID)}>
              <Icon name={collapsed.has(g.course.ID) ? 'chevronRight' : 'chevronDown'} size={12} />
              <span class="dot" style="background:{g.course.Color}"></span>
              <span class="pc-code truncate">{g.course.Code}</span>
              <span class="pc-n faint">{g.files.length}</span>
            </button>
            {#if !collapsed.has(g.course.ID)}
              {#each g.files as f (f.ID)}
                {@const sel = selectedIDs.includes(f.ID)}
                <button class="pf" class:sel onclick={() => toggleFile(f)} title={f.RelPath}>
                  <span class="box" class:on={sel}>{#if sel}<Icon name="check" size={10} />{/if}</span>
                  <Icon name={KIND_ICON[fileKind(f.Name)] ?? 'file'} size={13} />
                  <span class="pf-name truncate">{f.Name}</span>
                  {#if pageCounts.get(f.ID)}
                    <span class="pf-pages faint">{pageCounts.get(f.ID)}p</span>
                  {:else}
                    <span class="pf-pages faint">{fmtBytes(f.Size)}</span>
                  {/if}
                </button>
              {/each}
            {/if}
          </div>
        {:else}
          <div class="p-loading faint">No study-able files yet. Run a sync first.</div>
        {/each}
      {/if}
    </div>
  </aside>

  <!-- --------------------------------------------------------- work area -->
  <section class="work">
    <header class="w-head">
      <div class="tabs">
        {#each TABS as t (t.id)}
          <button class="tab" class:on={tab === t.id} onclick={() => { tab = t.id; exitTaking(); }}>{t.label}</button>
        {/each}
      </div>
      <div class="w-actions">
        <select class="select model" bind:value={model} title="Model used for the next job">
          {#each status?.Models ?? ['opus', 'sonnet', 'haiku'] as m (m)}
            <option value={m}>{m}</option>
          {/each}
        </select>
      </div>
    </header>

    {#if statusLoading}
      <div class="banner"><span class="spinner"></span> Checking the Claude Code CLI…</div>
    {:else if !ready}
      <div class="banner warn">
        <Icon name="alert" size={14} />
        <span>
          {#if !status?.CLIFound}
            Claude Code CLI not found — install it and make sure <span class="mono">claude</span> is on your PATH.
          {:else}
            Not signed in — run <span class="mono">claude auth login</span> in a terminal, then reload.
          {/if}
          {#if status?.Error}<span class="err-text faint">{status.Error}</span>{/if}
        </span>
        <button class="btn sm" onclick={loadStatus}>Re-check</button>
      </div>
    {/if}

    {#if job}
      <div class="jobbar" class:err={job.Status === 'error'}>
        {#if busy}<span class="spinner"></span>{:else}<Icon name={job.Status === 'done' ? 'checkCircle' : 'info'} size={13} />{/if}
        <span class="job-kind">{job.Kind}</span>
        <span
          class="job-line"
          class:err-text={job.Status === 'error'}
          class:truncate={job.Status !== 'error'}>{job.Status === 'error'
            ? job.Error
            : jobLine || job.Progress}</span>
        {#if elapsed}<span class="job-time faint">{elapsed}</span>{/if}
        {#if busy}
          <button class="btn sm" onclick={cancelJob}>Cancel</button>
        {/if}
      </div>
    {/if}

    <div class="w-body">
      <!-- ------------------------------------------------------ overview -->
      {#if tab === 'overview'}
        <div class="pane-head">
          <div>
            <h2 class="pane-title">{primary ? primary.Name : 'Overview'}</h2>
            <p class="faint pane-sub">
              {#if !primary}
                Pick a file on the left.
              {:else if overview}
                Generated {relTime(overview.CreatedAt)} · {overview.Model}
              {:else}
                No overview cached for this file yet.
              {/if}
            </p>
          </div>
          <div class="head-actions">
            <button
              class="btn"
              class:on={previewOpen}
              onclick={() => (previewOpen = !previewOpen)}
              disabled={!primary}
              title="Preview the selected file in the app"
            >
              <Icon name={previewOpen ? 'eyeOff' : 'eye'} size={13} />
              Preview
            </button>
            <button class="btn primary" onclick={genOverview} disabled={!ready || busy || !primary}>
              <Icon name="layers" size={13} />
              {overview ? 'Regenerate overview' : 'Generate overview'}
            </button>
          </div>
        </div>

        {#if previewOpen && primary}
          <div class="spreview">
            <Viewer file={primary} showStudy={false} onClose={() => (previewOpen = false)} />
          </div>
        {/if}

        {#if overviewLoading}
          <div class="empty"><span class="spinner"></span> Loading…</div>
        {:else if overviewHTML}
          <article class="md card">{@html overviewHTML}</article>
        {:else}
          <div class="card"><div class="empty">Nothing generated yet — the button above runs it once, on demand.</div></div>
        {/if}

        <!-- ---------------------------------------------------------- quiz -->
      {:else if tab === 'quiz'}
        {#if taking && result}
          <div class="pane-head">
            <div>
              <h2 class="pane-title">Results — {taking.Title}</h2>
              <p class="faint pane-sub">{result.Score} of {result.Total} correct</p>
            </div>
            <button class="btn" onclick={exitTaking}><Icon name="chevronLeft" size={13} /> Back to quizzes</button>
          </div>

          <div class="card score-card">
            <div class="big-score">{result.Score}<span class="of">/{result.Total}</span></div>
            <div class="score-bar">
              <span class="score-fill" style="width:{result.Total ? (result.Score / result.Total) * 100 : 0}%"></span>
            </div>
          </div>

          <div class="review">
            {#each taking.Questions ?? [] as q, i (q.ID)}
              {@const given = answers[q.ID] ?? ''}
              {@const ok = q.Type === 'short' ? given === 'correct' : given.toUpperCase() === q.Answer.trim().toUpperCase()}
              <div class="card rq" class:ok class:bad={!ok}>
                <div class="rq-head">
                  <span class="rq-n">Q{i + 1}</span>
                  <span class="rq-prompt">{q.Prompt}</span>
                  <span class="chip {ok ? 'green' : 'red'}">{ok ? 'correct' : 'missed'}</span>
                </div>
                <div class="rq-body">
                  {#if q.Type === 'mcq'}
                    <p class="rq-line"><span class="faint">Answer:</span> {q.Answer} — {(q.Options ?? [])[q.Answer.charCodeAt(0) - 65] ?? ''}</p>
                  {:else}
                    <p class="rq-line"><span class="faint">Model answer:</span> {q.Answer}</p>
                  {/if}
                  <p class="rq-exp">{q.Explanation}</p>
                  {#if q.Page}<span class="chip page">page {q.Page}</span>{/if}
                </div>
              </div>
            {/each}
          </div>
        {:else if taking && current}
          <div class="pane-head">
            <div>
              <h2 class="pane-title">{taking.Title}</h2>
              <p class="faint pane-sub">Question {qIndex + 1} of {total} · 1–4 to answer, Enter to continue</p>
            </div>
            <button class="btn" onclick={exitTaking}>Quit</button>
          </div>

          <div class="progress">
            <span class="progress-fill" style="width:{((qIndex + (revealed ? 1 : 0)) / Math.max(1, total)) * 100}%"></span>
          </div>

          <div class="card qcard">
            <p class="qprompt">{current.Prompt}</p>

            {#if current.Type === 'mcq'}
              <div class="opts">
                {#each current.Options ?? [] as opt, i (opt)}
                  {@const letter = letterOf(i)}
                  {@const chosen = answers[current.ID] === letter}
                  {@const correct = current.Answer.trim().toUpperCase() === letter}
                  <button
                    class="opt"
                    class:chosen
                    class:correct={revealed && correct}
                    class:wrong={revealed && chosen && !correct}
                    onclick={() => pickOption(letter)}
                    disabled={revealed}
                  >
                    <span class="opt-key">{i + 1}</span>
                    <span class="opt-text">{opt}</span>
                  </button>
                {/each}
              </div>
            {:else}
              <textarea
                class="input short"
                rows="4"
                placeholder="Your answer…"
                bind:value={shortDraft}
                disabled={revealed}
              ></textarea>
              {#if !revealed}
                <button class="btn primary reveal-btn" onclick={revealShort}>Reveal model answer</button>
              {:else}
                <div class="model-answer">
                  <span class="section-title">Model answer</span>
                  <p>{current.Answer}</p>
                </div>
                <div class="selfmark">
                  <span class="faint">How did you do?</span>
                  <button class="btn sm" class:primary={answers[current.ID] === 'correct'} onclick={() => selfMark(true)}>
                    I got it
                  </button>
                  <button class="btn sm" class:primary={answers[current.ID] === 'wrong'} onclick={() => selfMark(false)}>
                    I missed it
                  </button>
                </div>
              {/if}
            {/if}

            {#if revealed}
              <div class="explain">
                <p>{current.Explanation}</p>
                {#if current.Page}<span class="chip page">page {current.Page}</span>{/if}
              </div>
              <div class="qfoot">
                <button
                  class="btn primary"
                  onclick={next}
                  disabled={submitting || (current.Type === 'short' && answers[current.ID] === undefined)}
                >
                  {#if submitting}<span class="spinner"></span>{/if}
                  {qIndex + 1 < total ? 'Next question' : 'Finish'}
                </button>
              </div>
            {/if}
          </div>
        {:else}
          <div class="pane-head">
            <div>
              <h2 class="pane-title">Quizzes</h2>
              <p class="faint pane-sub">
                {primary ? `${quizzes.length} for ${primary.Name}` : `${quizzes.length} in the bank`}
                {selectedIDs.length > 1 ? ` · ${selectedIDs.length} files selected` : ''}
              </p>
            </div>
            <div class="gen">
              <label class="nlab faint">
                n
                <input class="input nnum" type="number" min="1" max="50" bind:value={quizN} />
              </label>
              <button class="btn primary" onclick={genQuiz} disabled={!ready || busy || selectedIDs.length === 0}>
                <Icon name="plus" size={13} /> Make quiz ({quizN})
              </button>
            </div>
          </div>

          <div class="card list">
            {#each quizzes as q (q.ID)}
              <button class="qrow" onclick={() => startTaking(q)}>
                <span class="qrow-main">
                  <span class="qrow-title truncate">{q.Title}</span>
                  <span class="faint qrow-sub">
                    {(q.Questions ?? []).length} questions · {q.Model} · {relTime(q.CreatedAt)}
                  </span>
                </span>
                <span class="chip accent">Take quiz</span>
              </button>
            {:else}
              <div class="empty">
                {#if selectedIDs.length === 0}
                  Select a file on the left, then make a quiz from it.
                {:else}
                  No quizzes for this selection yet.
                {/if}
              </div>
            {/each}
          </div>

          {#if attempts.length > 0}
            <h3 class="section-title hist-title">Recent attempts</h3>
            <div class="card list">
              {#each attempts as a (a.TakenAt)}
                <div class="arow">
                  <span class="arow-score">{a.Score}/{a.Total}</span>
                  <span class="faint">{relTime(a.TakenAt)}</span>
                </div>
              {/each}
            </div>
          {/if}
        {/if}

        <!-- ----------------------------------------------------------- ask -->
      {:else if tab === 'ask'}
        <div class="pane-head">
          <div>
            <h2 class="pane-title">Ask</h2>
            <p class="faint pane-sub">
              {selectedIDs.length === 0
                ? 'Select one or more files to ask about.'
                : `Asking across ${selectedIDs.length} file${selectedIDs.length === 1 ? '' : 's'}`}
            </p>
          </div>
        </div>

        <div class="askbar card">
          <input
            class="input"
            type="text"
            placeholder="What do you want to know about these files?"
            bind:value={question}
            onkeydown={(e) => {
              if (e.key === 'Enter') genAsk();
            }}
          />
          <button class="btn primary" onclick={genAsk} disabled={!ready || busy || !question.trim() || selectedIDs.length === 0}>
            <Icon name="send" size={13} /> Ask
          </button>
        </div>

        <div class="thread">
          {#each asks as a (a.CreatedAt + a.Question)}
            <div class="card ask">
              <p class="ask-q">{a.Question}</p>
              <div class="md ask-a">{@html markdownToHTML(a.Answer)}</div>
              {#if (a.Citations ?? []).length > 0}
                <div class="cites">
                  <span class="section-title">Citations</span>
                  <div class="cite-list">
                    {#each a.Citations ?? [] as c (c)}
                      <span class="chip">{c}</span>
                    {/each}
                  </div>
                </div>
              {/if}
              <span class="faint ask-time">{relTime(a.CreatedAt)}</span>
            </div>
          {:else}
            <div class="card"><div class="empty">No questions asked yet.</div></div>
          {/each}
        </div>

        <!-- ---------------------------------------------------- flashcards -->
      {:else}
        <div class="pane-head">
          <div>
            <h2 class="pane-title">Flashcards</h2>
            <p class="faint pane-sub">
              {cards.length} due{primary ? ` · make more from ${primary.Name}` : ''}
            </p>
          </div>
          <button class="btn primary" onclick={genCards} disabled={!ready || busy || !primary}>
            <Icon name="plus" size={13} /> Make flashcards
          </button>
        </div>

        {#if currentCard}
          <div class="card fcard">
            <span class="fc-count faint">{cardIndex + 1} of {cards.length} due</span>
            <p class="fc-front">{currentCard.Front}</p>
            {#if flipped}
              <p class="fc-back">{currentCard.Back}</p>
              <div class="fc-grades">
                {#each GRADES as g (g.g)}
                  <button class="btn grade" onclick={() => reviewCard(g.g)} title={g.hint}>{g.label}</button>
                {/each}
              </div>
            {:else}
              <button class="btn primary" onclick={() => (flipped = true)}>Flip</button>
            {/if}
          </div>
        {:else}
          <div class="card">
            <div class="empty">
              Nothing due right now. {primary ? 'Make some from the selected file.' : 'Select a file to make some.'}
            </div>
          </div>
        {/if}
      {/if}
    </div>
  </section>
</div>

<style>
  .study {
    display: flex;
    height: 100%;
    min-height: 0;
  }

  /* ---------------------------------------------------------- picker */

  .picker {
    width: 268px;
    flex: none;
    display: flex;
    flex-direction: column;
    min-height: 0;
    border-right: 1px solid var(--border);
    background: var(--bg-sidebar);
  }

  .picker-top {
    padding: 10px 10px 8px;
    border-bottom: 1px solid var(--border);
    flex: none;
  }

  .search {
    display: flex;
    align-items: center;
    gap: 7px;
    height: 30px;
    padding: 0 9px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-input);
    color: var(--text-faint);
  }

  .search:focus-within {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-soft);
  }

  .q {
    flex: 1;
    min-width: 0;
    border: none;
    background: none;
    outline: none;
    font-size: 12.5px;
    color: var(--text);
  }

  .clear {
    display: flex;
    color: var(--text-faint);
  }

  .sel-line {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 8px;
    margin-top: 7px;
    font-size: 11.5px;
  }

  .sel-line .faint {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .linkish {
    flex: none;
    font-size: 11.5px;
    font-weight: 550;
    color: var(--accent-text);
  }

  .linkish:hover {
    text-decoration: underline;
  }

  .picker-scroll {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 6px;
  }

  .p-loading {
    display: flex;
    align-items: center;
    gap: 7px;
    padding: 14px 8px;
    font-size: 12.5px;
    color: var(--text-muted);
  }

  .pc {
    margin-bottom: 4px;
  }

  .pc-head {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    padding: 5px 7px;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    font-size: 12px;
    font-weight: 600;
  }

  .pc-head:hover {
    background: var(--bg-hover);
  }

  .pc-code {
    flex: 1;
    min-width: 0;
    text-align: left;
    color: var(--text);
  }

  .pc-n {
    font-size: 11px;
  }

  .pf {
    display: flex;
    align-items: center;
    gap: 7px;
    width: 100%;
    padding: 4px 7px 4px 14px;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    font-size: 12px;
    text-align: left;
  }

  .pf:hover {
    background: var(--bg-hover);
  }

  .pf.sel {
    background: var(--bg-active);
    color: var(--text);
  }

  .box {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 13px;
    height: 13px;
    flex: none;
    border: 1px solid var(--border-strong);
    border-radius: 3px;
    background: var(--bg-input);
    color: #fff;
  }

  .box.on {
    background: var(--accent);
    border-color: var(--accent);
  }

  .pf-name {
    flex: 1;
    min-width: 0;
  }

  .pf-pages {
    flex: none;
    font-size: 10.5px;
    font-variant-numeric: tabular-nums;
  }

  /* ------------------------------------------------------- work area */

  .work {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  .w-head {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 16px;
    border-bottom: 1px solid var(--border);
    flex: none;
  }

  .tabs {
    display: flex;
    gap: 2px;
    flex: 1;
  }

  .tab {
    height: 27px;
    padding: 0 11px;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    font-size: 12.5px;
    font-weight: 550;
    transition: background var(--t), color var(--t);
  }

  .tab:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .tab.on {
    background: var(--bg-active);
    color: var(--accent-text);
  }

  .model {
    width: auto;
    height: 27px;
    font-size: 12px;
  }

  .banner {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 16px;
    font-size: 12.5px;
    color: var(--text-muted);
    border-bottom: 1px solid var(--border);
    flex: none;
  }

  .banner.warn {
    background: var(--amber-soft);
    color: var(--amber);
    align-items: flex-start;
  }

  /*
   * A CLI failure or a Canvas body is arbitrary length, so every error string
   * shown inline wraps and scrolls in place instead of truncating (which hides
   * the only useful half) or stretching the bar off-screen.
   */
  .err-text {
    display: block;
    min-width: 0;
    max-height: 8.7em;
    overflow-y: auto;
    white-space: pre-wrap;
    word-break: break-word;
    overflow-wrap: anywhere;
  }

  .banner span {
    flex: 1;
    min-width: 0;
  }

  .jobbar {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 7px 16px;
    font-size: 12.5px;
    background: var(--accent-soft);
    border-bottom: 1px solid var(--border);
    flex: none;
  }

  .jobbar.err {
    background: var(--red-soft);
    color: var(--red);
    align-items: flex-start;
  }

  .job-kind {
    font-weight: 600;
    text-transform: capitalize;
    flex: none;
  }

  .job-line {
    flex: 1;
    min-width: 0;
  }

  .job-time {
    flex: none;
    font-variant-numeric: tabular-nums;
  }

  .w-body {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 18px 20px 40px;
  }

  .head-actions {
    display: flex;
    align-items: center;
    gap: 7px;
    flex: none;
  }

  .spreview {
    height: 60vh;
    min-height: 320px;
    margin-bottom: 14px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    overflow: hidden;
    display: flex;
  }

  .spreview :global(.viewer) {
    flex: 1;
    min-width: 0;
  }

  .pane-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 14px;
    margin-bottom: 14px;
  }

  .pane-title {
    font-size: 15px;
    font-weight: 620;
    letter-spacing: -0.01em;
  }

  .pane-sub {
    margin-top: 2px;
    font-size: 12px;
  }

  .gen {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: none;
  }

  .nlab {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-size: 11.5px;
  }

  .nnum {
    width: 58px;
    height: 30px;
    text-align: center;
  }

  /* ------------------------------------------------------- markdown */

  .md {
    padding: 16px 18px;
    font-size: 13px;
    line-height: 1.65;
  }

  .md :global(h1) {
    font-size: 17px;
    font-weight: 640;
    letter-spacing: -0.015em;
    margin: 0 0 10px;
  }

  .md :global(h2) {
    font-size: 13.5px;
    font-weight: 620;
    margin: 18px 0 6px;
  }

  .md :global(h3) {
    font-size: 12.5px;
    font-weight: 600;
    color: var(--text-muted);
    margin: 14px 0 5px;
  }

  .md :global(p) {
    margin: 0 0 9px;
  }

  .md :global(ul),
  .md :global(ol) {
    margin: 0 0 10px;
    padding-left: 20px;
    list-style: revert;
  }

  .md :global(li) {
    margin-bottom: 3px;
  }

  .md :global(code) {
    font-family: var(--mono);
    font-size: 12px;
    background: var(--bg-subtle);
    border-radius: 4px;
    padding: 1px 4px;
  }

  .md :global(pre) {
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 10px 12px;
    overflow-x: auto;
    margin: 0 0 10px;
  }

  .md :global(pre code) {
    background: none;
    padding: 0;
  }

  .md :global(blockquote) {
    margin: 0 0 10px;
    padding-left: 11px;
    border-left: 2px solid var(--border-strong);
    color: var(--text-muted);
  }

  /* ----------------------------------------------------------- quiz */

  .list {
    overflow: hidden;
  }

  .qrow {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 10px 13px;
    text-align: left;
    border-bottom: 1px solid var(--border);
    transition: background var(--t);
  }

  .qrow:last-child {
    border-bottom: none;
  }

  .qrow:hover {
    background: var(--bg-hover);
  }

  .qrow-main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .qrow-title {
    font-size: 13px;
  }

  .qrow-sub {
    font-size: 11.5px;
  }

  .hist-title {
    display: block;
    margin: 20px 0 8px 2px;
  }

  .arow {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    padding: 8px 13px;
    font-size: 12.5px;
    border-bottom: 1px solid var(--border);
  }

  .arow:last-child {
    border-bottom: none;
  }

  .arow-score {
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }

  .progress {
    height: 3px;
    border-radius: 999px;
    background: var(--bg-subtle);
    overflow: hidden;
    margin-bottom: 14px;
  }

  .progress-fill {
    display: block;
    height: 100%;
    background: var(--accent);
    transition: width var(--t);
  }

  .qcard {
    padding: 18px;
  }

  .qprompt {
    font-size: 15px;
    line-height: 1.5;
    margin-bottom: 14px;
  }

  .opts {
    display: flex;
    flex-direction: column;
    gap: 7px;
  }

  .opt {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    width: 100%;
    padding: 10px 12px;
    text-align: left;
    font-size: 13px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-elevated);
    transition: background var(--t), border-color var(--t);
  }

  .opt:hover:not(:disabled) {
    background: var(--bg-hover);
    border-color: var(--border-strong);
  }

  .opt-key {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    flex: none;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg-subtle);
    font-size: 11px;
    font-weight: 600;
    color: var(--text-muted);
  }

  .opt-text {
    flex: 1;
    min-width: 0;
  }

  .opt.chosen {
    border-color: var(--accent);
  }

  .opt.correct {
    border-color: var(--green);
    background: var(--green-soft);
  }

  .opt.wrong {
    border-color: var(--red);
    background: var(--red-soft);
  }

  .opt:disabled {
    cursor: default;
  }

  .short {
    height: auto;
    padding: 9px 11px;
    resize: vertical;
    line-height: 1.55;
  }

  .reveal-btn {
    margin-top: 10px;
  }

  .model-answer {
    margin-top: 12px;
    padding: 11px 13px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
    font-size: 13px;
    line-height: 1.55;
  }

  .model-answer .section-title {
    display: block;
    margin-bottom: 4px;
  }

  .selfmark {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-top: 11px;
    font-size: 12px;
  }

  .explain {
    margin-top: 14px;
    padding-top: 12px;
    border-top: 1px solid var(--border);
    font-size: 12.5px;
    color: var(--text-muted);
    line-height: 1.6;
  }

  .chip.page {
    margin-top: 7px;
  }

  .qfoot {
    display: flex;
    justify-content: flex-end;
    margin-top: 14px;
  }

  .score-card {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 18px;
    margin-bottom: 16px;
  }

  .big-score {
    font-size: 34px;
    font-weight: 660;
    letter-spacing: -0.03em;
    font-variant-numeric: tabular-nums;
    flex: none;
  }

  .of {
    font-size: 18px;
    color: var(--text-faint);
  }

  .score-bar {
    flex: 1;
    height: 8px;
    border-radius: 999px;
    background: var(--bg-subtle);
    overflow: hidden;
  }

  .score-fill {
    display: block;
    height: 100%;
    background: var(--accent);
  }

  .review {
    display: flex;
    flex-direction: column;
    gap: 9px;
  }

  .rq {
    padding: 13px 15px;
    border-left-width: 3px;
  }

  .rq.ok {
    border-left-color: var(--green);
  }

  .rq.bad {
    border-left-color: var(--red);
  }

  .rq-head {
    display: flex;
    align-items: flex-start;
    gap: 9px;
  }

  .rq-n {
    flex: none;
    font-size: 11.5px;
    font-weight: 600;
    color: var(--text-faint);
    padding-top: 1px;
  }

  .rq-prompt {
    flex: 1;
    min-width: 0;
    font-size: 13px;
  }

  .rq-body {
    margin-top: 8px;
    padding-left: 26px;
  }

  .rq-line {
    font-size: 12.5px;
    margin-bottom: 4px;
  }

  .rq-exp {
    font-size: 12.5px;
    color: var(--text-muted);
    line-height: 1.6;
  }

  /* ------------------------------------------------------------ ask */

  .askbar {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    margin-bottom: 14px;
  }

  .thread {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .ask {
    padding: 14px 16px;
  }

  .ask-q {
    font-size: 13.5px;
    font-weight: 550;
    margin-bottom: 9px;
  }

  .ask-a {
    padding: 0;
  }

  .cites {
    margin-top: 10px;
    padding-top: 10px;
    border-top: 1px solid var(--border);
  }

  .cites .section-title {
    display: block;
    margin-bottom: 5px;
  }

  .cite-list {
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
  }

  .ask-time {
    display: block;
    margin-top: 9px;
    font-size: 11px;
  }

  /* ----------------------------------------------------- flashcards */

  .fcard {
    padding: 26px 22px;
    text-align: center;
  }

  .fc-count {
    font-size: 11.5px;
  }

  .fc-front {
    font-size: 19px;
    font-weight: 600;
    letter-spacing: -0.015em;
    margin: 14px 0 16px;
  }

  .fc-back {
    font-size: 13.5px;
    line-height: 1.6;
    color: var(--text-muted);
    max-width: 560px;
    margin: 0 auto 18px;
  }

  .fc-grades {
    display: flex;
    justify-content: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .grade {
    min-width: 76px;
    justify-content: center;
  }

  @media (max-width: 900px) {
    .picker {
      width: 210px;
    }
  }
</style>
