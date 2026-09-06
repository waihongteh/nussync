<script lang="ts">
  /**
   * Quiz Rush — an arcade run over whatever the Study view has already
   * generated. Three lives, a timer that tightens as the streak grows, a
   * multiplier that rewards not breaking it, and a short-answer boss every
   * tenth question. Nothing here generates anything: an empty bank sends you
   * to Study.
   *
   * Self-contained: it takes no props and talks to the backend directly, so
   * the Arcade can mount it with `<QuizRush />`.
   */
  import { api, errMsg } from '../api';
  import Icon from '../components/Icon.svelte';
  import { courses, toast } from '../stores';
  import type { FileNode, Question, Quiz } from '../types';
  import { lsGet, lsSet } from '../util';
  import { recordRush } from '../quest';

  const BEST_KEY = 'nussync.arcade.rush';
  const LIVES = 3;
  const BOSS_EVERY = 10;
  const TIME_MAX = 12_000;
  const TIME_MIN = 5_000;
  const WRONG_MS = 3000;

  type Phase = 'menu' | 'question' | 'wrong' | 'boss-reveal' | 'over';

  interface Card {
    q: Question;
    courseID: number;
    courseCode: string;
    quizTitle: string;
  }

  let loading = $state(true);
  let bank = $state<Card[]>([]);
  let pick = $state<number>(0); // 0 = every course
  let best = $state<Record<string, number>>(lsGet(BEST_KEY, {} as Record<string, number>));

  let phase = $state<Phase>('menu');
  let deck = $state<Card[]>([]);
  let index = $state(0);
  let lives = $state(LIVES);
  let score = $state(0);
  let streak = $state(0);
  let bestStreak = $state(0);
  let answered = $state(0);
  let redo = $state<Card[]>([]);

  let chosen = $state<string>('');
  let deadline = $state(0);
  let remaining = $state(TIME_MAX);
  let bossRevealed = $state(false);

  const card = $derived(deck[index] ?? null);
  const isBoss = $derived(answered > 0 && (answered + 1) % BOSS_EVERY === 0);
  const multiplier = $derived(Math.min(5, 1 + Math.floor(streak / 3)));
  const duration = $derived(Math.max(TIME_MIN, TIME_MAX - streak * 700));
  const bestKey = $derived(pick === 0 ? 'all' : String(pick));
  const bestScore = $derived(best[bestKey] ?? 0);

  // ------------------------------------------------------------- loading

  async function load() {
    loading = true;
    try {
      const [quizzes, trees] = await Promise.all([
        api.getQuizzes(0),
        Promise.all($courses.map((c) => api.getTree(c.ID).catch(() => [] as FileNode[]))),
      ]);
      // file id -> course, so a quiz can be attributed to the course it came from
      const courseOf = new Map<number, number>();
      $courses.forEach((c, i) => {
        const walk = (nodes: FileNode[]) => {
          for (const n of nodes) {
            if (n.IsDir) walk(n.Children ?? []);
            else courseOf.set(n.ID, c.ID);
          }
        };
        walk(trees[i] ?? []);
      });

      const out: Card[] = [];
      for (const q of quizzes ?? []) {
        const courseID = (q.FileIDs ?? []).map((id) => courseOf.get(id)).find((x) => x !== undefined) ?? 0;
        const code = $courses.find((c) => c.ID === courseID)?.Code ?? 'Unfiled';
        for (const question of q.Questions ?? []) {
          out.push({ q: question, courseID, courseCode: code, quizTitle: q.Title });
        }
      }
      bank = out;
    } catch (err) {
      toast(`Could not load the quiz bank: ${errMsg(err)}`, 'error');
      bank = [];
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void $courses;
    void load();
  });

  const byCourse = $derived.by(() => {
    const m = new Map<number, number>();
    for (const c of bank) m.set(c.courseID, (m.get(c.courseID) ?? 0) + 1);
    return m;
  });

  const pool = $derived(pick === 0 ? bank : bank.filter((c) => c.courseID === pick));

  // --------------------------------------------------------------- game

  function shuffle<T>(list: T[]): T[] {
    const a = [...list];
    for (let i = a.length - 1; i > 0; i--) {
      const j = Math.floor(Math.random() * (i + 1));
      [a[i], a[j]] = [a[j], a[i]];
    }
    return a;
  }

  function start() {
    if (pool.length === 0) return;
    // Repeat the pool if it is short — a run should not end because the bank ran dry.
    let d = shuffle(pool);
    while (d.length < 40) d = [...d, ...shuffle(pool)];
    deck = d;
    index = 0;
    lives = LIVES;
    score = 0;
    streak = 0;
    bestStreak = 0;
    answered = 0;
    redo = [];
    ask();
  }

  function ask() {
    chosen = '';
    bossRevealed = false;
    phase = 'question';
    deadline = Date.now() + duration;
    remaining = duration;
  }

  // The countdown restarts whenever a new question sets a new deadline.
  $effect(() => {
    if (phase !== 'question') return;
    const end = deadline;
    const h = setInterval(() => {
      const left = end - Date.now();
      remaining = Math.max(0, left);
      if (left <= 0) {
        clearInterval(h);
        miss(true);
      }
    }, 80);
    return () => clearInterval(h);
  });

  function letterOf(i: number) {
    return String.fromCharCode(65 + i);
  }

  /** The text of the correct answer, whichever question type it is. */
  function answerText(q: Question): string {
    if (q.Type === 'mcq') {
      const i = q.Answer.trim().toUpperCase().charCodeAt(0) - 65;
      return (q.Options ?? [])[i] ?? q.Answer;
    }
    return q.Answer;
  }

  function hit(timeLeft: number) {
    const base = isBoss ? 250 : 100;
    const bonus = Math.round((timeLeft / 1000) * 5);
    score += base * multiplier + bonus;
    streak += 1;
    bestStreak = Math.max(bestStreak, streak);
    advance();
  }

  function miss(timedOut = false) {
    streak = 0;
    lives -= 1;
    if (card) redo = [...redo, card];
    if (timedOut) chosen = '';
    phase = 'wrong';
    if (lives <= 0) {
      // still show the explanation, then land on the results screen
      setTimeout(() => finish(), WRONG_MS);
    } else {
      setTimeout(() => {
        if (phase === 'wrong') advance();
      }, WRONG_MS);
    }
  }

  function advance() {
    answered += 1;
    if (lives <= 0) {
      finish();
      return;
    }
    if (index + 1 >= deck.length) {
      finish();
      return;
    }
    index += 1;
    ask();
  }

  function finish() {
    phase = 'over';
    if (score > bestScore) {
      const next = { ...best, [bestKey]: score };
      best = next;
      lsSet(BEST_KEY, next);
    }
    try {
      // Quest owns the rush payout (score/10) and tracks SPD from the streak.
      recordRush(score, bestStreak);
    } catch {
      /* the pet is a nicety, never a failure mode */
    }
  }

  function answerMCQ(letter: string) {
    if (phase !== 'question' || !card) return;
    chosen = letter;
    const left = remaining;
    if (letter.toUpperCase() === card.q.Answer.trim().toUpperCase()) hit(left);
    else miss();
  }

  function revealBoss() {
    if (phase !== 'question') return;
    bossRevealed = true;
    phase = 'boss-reveal';
    remaining = 0;
  }

  function bossMark(correct: boolean) {
    if (correct) hit(0);
    else miss();
  }

  function goToStudy() {
    window.dispatchEvent(new CustomEvent('nussync:navigate', { detail: { view: 'study' } }));
  }

  function onKey(e: KeyboardEvent) {
    const target = e.target as HTMLElement | null;
    if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA')) return;

    if (phase === 'question' && card) {
      if (isBoss) {
        if (e.key === 'Enter') {
          e.preventDefault();
          revealBoss();
        }
        return;
      }
      if (e.key >= '1' && e.key <= '4') {
        const i = Number(e.key) - 1;
        if (i < (card.q.Options ?? []).length) {
          e.preventDefault();
          answerMCQ(letterOf(i));
        }
      }
      return;
    }
    if (phase === 'wrong' && e.key === 'Enter') {
      e.preventDefault();
      if (lives > 0) advance();
      else finish();
    }
  }

  const timePct = $derived(Math.max(0, Math.min(100, (remaining / duration) * 100)));
</script>

<svelte:window onkeydown={onKey} />

<div class="view">
  <div class="view-narrow rush">
    {#if loading}
      <div class="card"><div class="empty"><span class="spinner"></span> Loading the quiz bank…</div></div>

      <!-- --------------------------------------------------------- empty -->
    {:else if bank.length === 0}
      <div class="card empty-card">
        <div class="empty-icon"><Icon name="trophy" size={22} /></div>
        <h2 class="empty-title">The bank is empty</h2>
        <p class="muted empty-sub">
          Quiz Rush plays through the quizzes you have already generated in Study. Make one there first — a
          five-question quiz off a lecture PDF is enough to get going.
        </p>
        <button class="btn primary" onclick={goToStudy}><Icon name="layers" size={13} /> Open Study</button>
      </div>

      <!-- ---------------------------------------------------------- menu -->
    {:else if phase === 'menu'}
      <header class="head">
        <h1 class="page-title">Quiz Rush</h1>
        <p class="muted sub">
          {bank.length} questions in the bank. Three lives, a timer that tightens as your streak grows, and a
          short-answer boss every {BOSS_EVERY}th question.
        </p>
      </header>

      <div class="card picker">
        <span class="section-title">Course</span>
        <div class="picks">
          <button class="pickchip" class:on={pick === 0} onclick={() => (pick = 0)}>
            All courses <span class="n">{bank.length}</span>
          </button>
          {#each $courses as c (c.ID)}
            {@const n = byCourse.get(c.ID) ?? 0}
            {#if n > 0}
              <button class="pickchip" class:on={pick === c.ID} onclick={() => (pick = c.ID)}>
                <span class="dot" style="background:{c.Color}"></span>
                {c.Code} <span class="n">{n}</span>
              </button>
            {/if}
          {/each}
        </div>
      </div>

      <div class="menu-foot">
        <div class="bests">
          <span class="faint">Best here</span>
          <span class="best-v">{bestScore.toLocaleString()}</span>
        </div>
        <button class="btn primary big" onclick={start} disabled={pool.length === 0}>
          <Icon name="gamepad" size={14} /> Start run
        </button>
      </div>

      <p class="faint keys">
        <kbd>1</kbd>–<kbd>4</kbd> answer · <kbd>Enter</kbd> continue · wrong answers come back in the redo pile.
      </p>

      <!-- ---------------------------------------------------------- over -->
    {:else if phase === 'over'}
      <header class="head">
        <h1 class="page-title">Run over</h1>
        <p class="muted sub">
          {answered} question{answered === 1 ? '' : 's'} · best streak {bestStreak}
          {score >= bestScore && score > 0 ? ' · new best!' : ''}
        </p>
      </header>

      <div class="card final">
        <div class="final-score">{score.toLocaleString()}</div>
        <div class="final-meta">
          <span class="fm"><span class="k faint">Best</span><span class="v">{bestScore.toLocaleString()}</span></span>
          <span class="fm"><span class="k faint">Streak</span><span class="v">{bestStreak}</span></span>
          <span class="fm"><span class="k faint">Missed</span><span class="v">{redo.length}</span></span>
        </div>
        <div class="final-actions">
          <button class="btn primary" onclick={start}>Play again</button>
          <button class="btn" onclick={() => (phase = 'menu')}>Change course</button>
        </div>
      </div>

      {#if redo.length > 0}
        <h2 class="section-title redo-title">Redo pile</h2>
        <div class="card list">
          {#each redo as r, i (r.q.ID + '-' + i)}
            <div class="rrow">
              <span class="rcode">{r.courseCode}</span>
              <span class="rmain">
                <span class="rprompt">{r.q.Prompt}</span>
                <span class="ranswer faint">{answerText(r.q)}</span>
                <span class="rexp faint">{r.q.Explanation}</span>
              </span>
              {#if r.q.Page}<span class="chip">p.{r.q.Page}</span>{/if}
            </div>
          {/each}
        </div>
      {/if}

      <!-- -------------------------------------------------------- playing -->
    {:else if card}
      <div class="hud">
        <div class="lives">
          {#each Array(LIVES) as _, i (i)}
            <span class="life" class:lost={i >= lives}><Icon name="dot" size={11} /></span>
          {/each}
        </div>
        <span class="hud-q faint">Q{answered + 1}</span>
        <span class="chip accent mult">×{multiplier}</span>
        {#if streak > 0}<span class="chip streak">{streak} streak</span>{/if}
        <span class="grow"></span>
        <span class="hud-score">{score.toLocaleString()}</span>
      </div>

      <div class="timer" class:low={timePct < 30}>
        <span class="timer-fill" style="width:{timePct}%"></span>
      </div>

      <div class="card qcard" class:boss={isBoss}>
        <div class="qmeta">
          <span class="qcode">{card.courseCode}</span>
          {#if isBoss}<span class="chip amber">Boss · short answer</span>{/if}
        </div>
        <p class="qprompt">{card.q.Prompt}</p>

        {#if isBoss || card.q.Type === 'short'}
          {#if !bossRevealed}
            <p class="faint boss-hint">Answer it in your head, then reveal and mark yourself honestly.</p>
            <button class="btn primary" onclick={revealBoss} disabled={phase !== 'question'}>Reveal answer</button>
          {:else}
            <div class="model-answer">
              <span class="section-title">Model answer</span>
              <p>{answerText(card.q)}</p>
            </div>
            <div class="selfmark">
              <button class="btn sm primary" onclick={() => bossMark(true)}>I got it</button>
              <button class="btn sm" onclick={() => bossMark(false)}>I missed it</button>
            </div>
          {/if}
        {:else}
          <div class="opts">
            {#each card.q.Options ?? [] as opt, i (opt)}
              {@const letter = letterOf(i)}
              {@const correct = card.q.Answer.trim().toUpperCase() === letter}
              <button
                class="opt"
                class:correct={phase === 'wrong' && correct}
                class:wrong={phase === 'wrong' && chosen === letter}
                onclick={() => answerMCQ(letter)}
                disabled={phase !== 'question'}
              >
                <span class="opt-key">{i + 1}</span>
                <span class="opt-text">{opt}</span>
              </button>
            {/each}
          </div>
        {/if}

        {#if phase === 'wrong'}
          <div class="explain">
            <div class="explain-head">
              <Icon name="alert" size={13} />
              <span>{chosen ? 'Not quite' : 'Out of time'} — added to the redo pile</span>
              {#if card.q.Page}<span class="chip">page {card.q.Page}</span>{/if}
            </div>
            <p>{card.q.Explanation}</p>
            <p class="faint next-hint">Next question in a moment · <kbd>Enter</kbd> to skip ahead</p>
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>

<style>
  .rush {
    max-width: 720px;
  }

  .head {
    margin-bottom: 18px;
  }

  .sub {
    margin-top: 3px;
    font-size: 13px;
  }

  /* ---------------------------------------------------------- empty */

  .empty-card {
    padding: 34px 26px;
    text-align: center;
  }

  .empty-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 46px;
    height: 46px;
    border-radius: 50%;
    background: var(--accent-soft);
    color: var(--accent-text);
    margin-bottom: 12px;
  }

  .empty-title {
    font-size: 16px;
    font-weight: 620;
    letter-spacing: -0.015em;
  }

  .empty-sub {
    font-size: 13px;
    line-height: 1.6;
    max-width: 430px;
    margin: 6px auto 16px;
  }

  /* ----------------------------------------------------------- menu */

  .picker {
    padding: 13px 15px;
    margin-bottom: 14px;
  }

  .picker .section-title {
    display: block;
    margin-bottom: 8px;
  }

  .picks {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }

  .pickchip {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 28px;
    padding: 0 10px;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: var(--bg-elevated);
    font-size: 12.5px;
    color: var(--text-muted);
    transition: background var(--t), border-color var(--t), color var(--t);
  }

  .pickchip:hover {
    background: var(--bg-hover);
    border-color: var(--border-strong);
    color: var(--text);
  }

  .pickchip.on {
    background: var(--accent-soft);
    border-color: var(--accent);
    color: var(--accent-text);
  }

  .pickchip .n {
    font-size: 11px;
    color: var(--text-faint);
    font-variant-numeric: tabular-nums;
  }

  .menu-foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 14px;
  }

  .bests {
    display: flex;
    align-items: baseline;
    gap: 8px;
    font-size: 12px;
  }

  .best-v {
    font-size: 18px;
    font-weight: 640;
    font-variant-numeric: tabular-nums;
  }

  .big {
    height: 36px;
    padding: 0 16px;
    font-size: 13.5px;
  }

  .keys {
    margin-top: 14px;
    font-size: 11.5px;
  }

  kbd {
    padding: 1px 4px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg-subtle);
    font-family: var(--font);
    font-size: 10px;
  }

  /* --------------------------------------------------------- playing */

  .hud {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 9px;
  }

  .lives {
    display: flex;
    gap: 2px;
    color: var(--red);
  }

  .life.lost {
    color: var(--border-strong);
  }

  .hud-q {
    font-size: 11.5px;
    font-variant-numeric: tabular-nums;
  }

  .mult {
    font-variant-numeric: tabular-nums;
  }

  .chip.streak {
    background: var(--green-soft);
    color: var(--green);
  }

  .grow {
    flex: 1;
  }

  .hud-score {
    font-size: 17px;
    font-weight: 640;
    letter-spacing: -0.02em;
    font-variant-numeric: tabular-nums;
  }

  .timer {
    height: 4px;
    border-radius: 999px;
    background: var(--bg-subtle);
    overflow: hidden;
    margin-bottom: 14px;
  }

  .timer-fill {
    display: block;
    height: 100%;
    background: var(--accent);
  }

  .timer.low .timer-fill {
    background: var(--red);
  }

  .qcard {
    padding: 20px;
  }

  .qcard.boss {
    border-color: var(--amber);
  }

  .qmeta {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 9px;
  }

  .qcode {
    font-size: 11.5px;
    font-weight: 600;
    color: var(--text-muted);
  }

  .qprompt {
    font-size: 16px;
    line-height: 1.5;
    margin-bottom: 16px;
  }

  .boss-hint {
    font-size: 12.5px;
    margin-bottom: 11px;
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
    padding: 11px 12px;
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

  .opt:disabled {
    cursor: default;
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

  .opt.correct {
    border-color: var(--green);
    background: var(--green-soft);
  }

  .opt.wrong {
    border-color: var(--red);
    background: var(--red-soft);
  }

  .model-answer {
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
    gap: 8px;
    margin-top: 11px;
  }

  .explain {
    margin-top: 15px;
    padding-top: 13px;
    border-top: 1px solid var(--border);
    font-size: 12.5px;
    color: var(--text-muted);
    line-height: 1.6;
  }

  .explain-head {
    display: flex;
    align-items: center;
    gap: 7px;
    margin-bottom: 6px;
    color: var(--red);
    font-weight: 550;
  }

  .next-hint {
    margin-top: 7px;
    font-size: 11.5px;
  }

  /* ------------------------------------------------------------ over */

  .final {
    padding: 22px;
    text-align: center;
  }

  .final-score {
    font-size: 40px;
    font-weight: 660;
    letter-spacing: -0.035em;
    font-variant-numeric: tabular-nums;
  }

  .final-meta {
    display: flex;
    justify-content: center;
    gap: 22px;
    margin: 12px 0 18px;
  }

  .fm {
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .fm .k {
    font-size: 11px;
  }

  .fm .v {
    font-size: 14px;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }

  .final-actions {
    display: flex;
    justify-content: center;
    gap: 8px;
  }

  .redo-title {
    display: block;
    margin: 20px 0 8px 2px;
  }

  .list {
    overflow: hidden;
  }

  .rrow {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 10px 13px;
    border-bottom: 1px solid var(--border);
  }

  .rrow:last-child {
    border-bottom: none;
  }

  .rcode {
    flex: none;
    min-width: 62px;
    font-size: 11.5px;
    font-weight: 600;
    color: var(--text-muted);
    padding-top: 1px;
  }

  .rmain {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .rprompt {
    font-size: 12.5px;
  }

  .ranswer {
    font-size: 12px;
  }

  .rexp {
    font-size: 11.5px;
    line-height: 1.5;
  }
</style>
