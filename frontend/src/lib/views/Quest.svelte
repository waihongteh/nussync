<script lang="ts">
  /**
   * Quest: the daily goal + streak dashboard and the boss ladder.
   *
   * Boss fights are turn-based quizzes drawn from the Study quiz bank
   * (GetQuizzes(0), flattened and attributed to a course through the file
   * tree). A correct answer is a hit for ATK — doubled under four seconds —
   * and a wrong answer or a timeout lets the boss hit back for
   * (boss ATK − DEF/2). Nothing here needs the backend beyond the existing
   * study bindings.
   */
  import { onDestroy } from 'svelte';
  import BossSprite from '../components/BossSprite.svelte';
  import Icon from '../components/Icon.svelte';
  import PetSprite from '../components/PetSprite.svelte';
  import { beProud, level, petName, xp } from '../pet';
  import {
    GEAR,
    GOALS,
    SKINS,
    TITLES,
    damageDeadlineBoss,
    deadlineBosses,
    equipSkin,
    equipTitle,
    freezes,
    gear,
    goalProgress,
    lootLabel,
    loadQuizBank,
    loseTier,
    petStats,
    quest,
    recordBossHit,
    setGoal,
    shuffle,
    streak,
    tierDef,
    tierUnlocked,
    todayXP,
    unlockedSkins,
    unlockedTitles,
    week,
    winTier,
    type BankCard,
    type DeadlineBoss,
    type Goal,
  } from '../quest';
  import { courses, navigate, toast } from '../stores';
  import { countdown } from '../util';

  // --------------------------------------------------------------- bank

  let bank = $state<BankCard[]>([]);
  let loading = $state(true);

  $effect(() => {
    void $courses;
    let alive = true;
    loading = true;
    loadQuizBank()
      .then((b) => {
        if (alive) bank = b;
      })
      .catch(() => {
        if (alive) bank = [];
      })
      .finally(() => {
        if (alive) loading = false;
      });
    return () => {
      alive = false;
    };
  });

  const courseIDs = $derived([...new Set(bank.map((c) => c.courseID))].sort((a, b) => a - b));

  /** Which course a course-tier draws from; tier 6+ mixes everything. */
  function poolFor(tier: number): BankCard[] {
    const def = tierDef(tier);
    if (def.mixed || courseIDs.length === 0) return bank;
    const id = courseIDs[(tier - 1) % courseIDs.length];
    const sub = bank.filter((c) => c.courseID === id);
    return sub.length >= 3 ? sub : bank;
  }

  function courseLabel(tier: number): string {
    const def = tierDef(tier);
    if (def.mixed) return 'All courses';
    const pool = poolFor(tier);
    return pool[0]?.courseCode ?? 'Any';
  }

  // --------------------------------------------------------------- fight

  type Phase = 'ask' | 'reveal' | 'won' | 'lost';

  interface Fight {
    mode: 'tier' | 'deadline';
    tier: number;
    name: string;
    /** deadline bosses only */
    dl: DeadlineBoss | null;
    cards: BankCard[];
    idx: number;
    bossHP: number;
    bossMax: number;
    hp: number;
    maxHP: number;
    timer: number;
    bossAtk: number;
    phase: Phase;
    chosen: string;
    lastHit: string;
    dealt: number;
    loot: string[];
  }

  let fight = $state<Fight | null>(null);
  let remaining = $state(0);
  let hurtBoss = $state(false);
  let askedAt = 0;
  let ticker: ReturnType<typeof setInterval> | undefined;
  let holdTimer: ReturnType<typeof setTimeout> | undefined;

  const card = $derived(fight?.cards[fight.idx] ?? null);
  const stats = $derived($petStats);

  function clearTimers() {
    if (ticker) clearInterval(ticker);
    if (holdTimer) clearTimeout(holdTimer);
    ticker = undefined;
    holdTimer = undefined;
  }

  onDestroy(clearTimers);

  function heartsFor(hp: number): number {
    return Math.max(3, Math.min(10, hp));
  }

  /** Roughly 70% of a perfect run, so the ladder tracks your ATK, not luck. */
  function bossHPFor(questions: number): number {
    return Math.max(3, Math.round(stats.atk * questions * 0.7));
  }

  function startTier(tier: number) {
    const def = tierDef(tier);
    const pool = poolFor(tier);
    if (pool.length === 0) return;
    let deck = shuffle(pool);
    while (deck.length < def.questions) deck = [...deck, ...shuffle(pool)];
    const maxHP = heartsFor(stats.hp);
    fight = {
      mode: 'tier',
      tier,
      name: def.name,
      dl: null,
      cards: deck.slice(0, def.questions),
      idx: 0,
      bossHP: bossHPFor(def.questions),
      bossMax: bossHPFor(def.questions),
      hp: maxHP,
      maxHP,
      timer: def.timer,
      bossAtk: def.atk,
      phase: 'ask',
      chosen: '',
      lastHit: '',
      dealt: 0,
      loot: [],
    };
    ask();
  }

  function startDeadline(b: DeadlineBoss) {
    const sub = bank.filter((c) => c.courseID === b.courseID);
    const pool = sub.length >= 3 ? sub : bank;
    if (pool.length === 0) return;
    let deck = shuffle(pool);
    while (deck.length < 3) deck = [...deck, ...shuffle(pool)];
    const maxHP = heartsFor(stats.hp);
    fight = {
      mode: 'deadline',
      tier: 1,
      name: b.title,
      dl: b,
      cards: deck.slice(0, 3),
      idx: 0,
      bossHP: b.hp,
      bossMax: b.maxHP,
      hp: maxHP,
      maxHP,
      timer: 12_000,
      bossAtk: 2,
      phase: 'ask',
      chosen: '',
      lastHit: '',
      dealt: 0,
      loot: [],
    };
    ask();
  }

  function ask() {
    if (!fight) return;
    fight.phase = 'ask';
    fight.chosen = '';
    fight.lastHit = '';
    askedAt = Date.now();
    remaining = fight.timer;
    clearTimers();
    ticker = setInterval(() => {
      if (!fight || fight.phase !== 'ask') return;
      remaining = Math.max(0, fight.timer - (Date.now() - askedAt));
      if (remaining === 0) resolve('');
    }, 90);
  }

  function letterOf(i: number) {
    return String.fromCharCode(65 + i);
  }

  function resolve(letter: string) {
    const f = fight;
    const c = card;
    if (!f || !c || f.phase !== 'ask') return;
    clearTimers();
    const elapsed = Date.now() - askedAt;
    const right = letter !== '' && letter.toUpperCase() === c.q.Answer.trim().toUpperCase();
    f.chosen = letter;
    f.phase = 'reveal';

    if (right) {
      recordBossHit();
      const crit = elapsed < 4_000;
      const dmg = f.mode === 'deadline' ? 1 : stats.atk * (crit ? 2 : 1);
      f.bossHP = Math.max(0, f.bossHP - dmg);
      f.dealt += dmg;
      f.lastHit = crit && f.mode === 'tier' ? `Critical! −${dmg} HP` : `−${dmg} HP`;
      hurtBoss = true;
      setTimeout(() => (hurtBoss = false), 300);
    } else {
      const dmg = Math.max(1, f.bossAtk - Math.floor(stats.def / 2));
      f.hp = Math.max(0, f.hp - dmg);
      f.lastHit = letter === '' ? `Out of time — you take ${dmg}` : `Wrong — you take ${dmg}`;
    }

    holdTimer = setTimeout(step, right ? 1_100 : 1_900);
  }

  function step() {
    const f = fight;
    if (!f) return;
    if (f.bossHP <= 0) return end(true);
    if (f.hp <= 0) return end(false);
    if (f.idx + 1 >= f.cards.length) return end(f.mode === 'deadline' ? f.dealt > 0 : false);
    f.idx += 1;
    ask();
  }

  function end(won: boolean) {
    const f = fight;
    if (!f) return;
    clearTimers();
    f.phase = won ? 'won' : 'lost';

    if (f.mode === 'tier') {
      if (won) {
        f.loot = winTier(f.tier).map(lootLabel);
        beProud();
        toast(`${f.name} defeated!`, 'success');
      } else {
        loseTier(f.tier);
      }
      return;
    }

    // Deadline boss: damage lands once, at the end of the attack round.
    if (f.dl && f.dealt > 0) {
      const killed = damageDeadlineBoss(f.dl.id, f.dealt, f.dl.maxHP, !f.dl.overdue);
      if (killed) {
        beProud();
        if (!f.dl.overdue) f.loot = ['a skin from the drop table'];
        toast(`${f.dl.title} defeated!`, 'success');
      }
    }
  }

  function leaveFight() {
    clearTimers();
    fight = null;
    hurtBoss = false;
  }

  function onKey(e: KeyboardEvent) {
    const f = fight;
    if (!f) return;
    const target = e.target as HTMLElement | null;
    if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA')) return;
    if (e.key === 'Escape') {
      e.preventDefault();
      leaveFight();
      return;
    }
    if (f.phase === 'ask' && e.key >= '1' && e.key <= '4') {
      const i = Number(e.key) - 1;
      if (i < (card?.q.Options ?? []).length) {
        e.preventDefault();
        resolve(letterOf(i));
      }
      return;
    }
    if ((f.phase === 'won' || f.phase === 'lost') && e.key === 'Enter') {
      e.preventDefault();
      leaveFight();
    }
  }

  // ----------------------------------------------------------- dashboard

  const RING = 2 * Math.PI * 26;
  const tiers = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10];
  const weekMax = $derived(Math.max(1, ...$week.map((d) => d.xp)));
  const liveBosses = $derived($deadlineBosses.filter((b) => !b.submitted));
  const doneBosses = $derived($deadlineBosses.filter((b) => b.submitted));
  const gearOwned = $derived(GEAR.filter((g) => $gear[g.id]));

  const STAT_ROWS = $derived([
    { key: 'ATK', value: stats.atk, from: 'correct answers', n: $quest.counters.correct },
    { key: 'DEF', value: stats.def, from: 'flashcards reviewed', n: $quest.counters.cards },
    {
      key: 'HP',
      value: stats.hp,
      from: 'overviews + previews',
      n: $quest.counters.overviews + $quest.counters.previews,
    },
    { key: 'SPD', value: stats.spd, from: 'best rush streak', n: $quest.counters.rushStreak },
  ]);
</script>

<svelte:window onkeydown={onKey} />

{#if fight}
  {@const f = fight}
  <div class="view">
    <div class="view-narrow arena">
      <header class="arena-top">
        <button class="btn sm ghost" onclick={leaveFight}>
          <Icon name="chevronLeft" size={13} /> Leave
        </button>
        <span class="chip accent">{f.mode === 'tier' ? `Tier ${f.tier}` : 'Deadline boss'}</span>
        <span class="grow"></span>
        <span class="faint qcount">Q{Math.min(f.idx + 1, f.cards.length)} / {f.cards.length}</span>
      </header>

      <section class="stage card">
        <div class="boss-side">
          <BossSprite tier={f.mode === 'tier' ? f.tier : ((f.dl?.id ?? 1) % 10) + 1} size={132} hurt={hurtBoss} dead={f.bossHP <= 0} />
          <div class="hpwrap">
            <div class="hpname truncate">{f.name}</div>
            <div class="hpbar">
              <div class="hpfill boss" style="width:{(f.bossHP / Math.max(1, f.bossMax)) * 100}%"></div>
            </div>
            <div class="faint hpnum">{f.bossHP} / {f.bossMax} HP</div>
          </div>
        </div>

        <div class="vs faint">vs</div>

        <div class="pet-side">
          <PetSprite mood={f.hp <= 1 ? 'panic' : f.phase === 'won' ? 'proud' : 'happy'} level={$level} size={92} />
          <div class="hpwrap">
            <div class="hpname truncate">{$petName}</div>
            <div class="hearts" aria-label="{f.hp} of {f.maxHP} hit points">
              {#each Array(f.maxHP) as _, i (i)}
                <span class="heart" class:on={i < f.hp}>♥</span>
              {/each}
            </div>
            <div class="faint hpnum">ATK {stats.atk} · DEF {stats.def}</div>
          </div>
        </div>
      </section>

      {#if f.phase === 'won' || f.phase === 'lost'}
        <section class="card outcome">
          <h2 class="outcome-title">{f.phase === 'won' ? 'Victory' : 'Defeated'}</h2>
          <p class="muted">
            {#if f.phase === 'won'}
              {f.name} is down. {f.mode === 'tier' ? `Tier ${f.tier} cleared.` : 'Nice hit.'}
            {:else if f.mode === 'deadline'}
              You dealt {f.dealt} damage. Come back and finish the job.
            {:else}
              {f.name} is still standing. Some xp for the attempt — try again.
            {/if}
          </p>
          {#if f.loot.length}
            <div class="loot">
              {#each f.loot as l (l)}
                <span class="chip accent"><Icon name="star" size={12} /> {l}</span>
              {/each}
            </div>
          {/if}
          <div class="actions">
            {#if f.mode === 'tier'}
              <button class="btn sm primary" onclick={() => { const t = f.tier; leaveFight(); startTier(t); }}>
                {f.phase === 'won' ? 'Fight again' : 'Retry'}
              </button>
            {/if}
            <button class="btn sm" onclick={leaveFight}>Back to the ladder</button>
          </div>
        </section>
      {:else if card}
        <section class="card qcard">
          <div class="timer">
            <div class="timerfill" style="width:{(remaining / f.timer) * 100}%"></div>
          </div>
          <p class="prompt">{card.q.Prompt}</p>
          <div class="opts">
            {#each card.q.Options ?? [] as opt, i (i)}
              {@const letter = letterOf(i)}
              {@const right = card.q.Answer.trim().toUpperCase() === letter}
              <button
                class="opt"
                class:correct={f.phase === 'reveal' && right}
                class:wrong={f.phase === 'reveal' && f.chosen === letter && !right}
                disabled={f.phase !== 'ask'}
                onclick={() => resolve(letter)}
              >
                <span class="key">{i + 1}</span>
                <span class="otext">{opt}</span>
              </button>
            {/each}
          </div>
          {#if f.phase === 'reveal'}
            <p class="hitline" class:good={f.lastHit.includes('−')}>{f.lastHit}</p>
            {#if card.q.Explanation}
              <p class="muted expl">{card.q.Explanation}</p>
            {/if}
          {:else}
            <p class="faint keys"><kbd>1</kbd>–<kbd>4</kbd> answer · <kbd>Esc</kbd> flee</p>
          {/if}
        </section>
      {/if}
    </div>
  </div>
{:else}
  <div class="view">
    <div class="view-narrow">
      <header class="head">
        <h1 class="page-title">Quest</h1>
        <p class="muted sub">One goal a day, one boss at a time. Everything you study feeds the pet.</p>
      </header>

      <!-- ------------------------------------------------- goal + recap -->
      <div class="top-grid">
        <section class="card goal-card">
          <div class="ring-wrap">
            <svg viewBox="0 0 60 60" width="118" height="118" class="ring">
              <circle class="track" cx="30" cy="30" r="26" />
              <circle
                class="prog"
                cx="30"
                cy="30"
                r="26"
                stroke-dasharray="{RING}"
                stroke-dashoffset={RING * (1 - $goalProgress)}
              />
            </svg>
            <div class="ring-mid">
              <span class="ring-xp">{$todayXP}</span>
              <span class="faint ring-goal">/ {$quest.goal} xp</span>
            </div>
          </div>
          <div class="goal-body">
            <div class="streak-line">
              <span class="flame" class:lit={$streak > 0}>🔥</span>
              <span class="streak-n">{$streak}</span>
              <span class="muted">day streak</span>
              {#if $freezes > 0}
                <span class="chip" title="Auto-spent on a missed day">❄ {$freezes}</span>
              {/if}
            </div>
            <p class="faint total">Level {$level} · {$xp} xp all time</p>
            <div class="goals">
              <span class="section-title">Daily goal</span>
              <div class="goal-btns">
                {#each GOALS as g (g)}
                  <button class="gbtn" class:on={$quest.goal === g} onclick={() => setGoal(g as Goal)}>{g}</button>
                {/each}
              </div>
            </div>
          </div>
        </section>

        <section class="card recap">
          <span class="section-title">Last 7 days</span>
          <div class="bars">
            {#each $week as d (d.key)}
              <div class="barcol" title="{d.key}: {d.xp} xp">
                <div class="bartrack">
                  <div
                    class="bar"
                    class:met={d.xp >= $quest.goal}
                    style="height:{Math.max(3, (d.xp / weekMax) * 100)}%"
                  ></div>
                </div>
                <span class="faint blabel">{d.label}</span>
              </div>
            {/each}
          </div>
        </section>
      </div>

      <!-- ---------------------------------------------------- pet stats -->
      <section class="card stats">
        <div class="stat-head">
          <span class="section-title">Pet stats</span>
          <span class="faint">Derived from what you actually did</span>
        </div>
        <div class="stat-grid">
          {#each STAT_ROWS as s (s.key)}
            <div class="stat">
              <div class="stat-top">
                <span class="skey">{s.key}</span>
                <span class="sval">{s.value}</span>
              </div>
              <div class="sbar"><div class="sfill" style="width:{Math.min(100, s.value * 8)}%"></div></div>
              <span class="faint sfrom">{s.n} {s.from}</span>
            </div>
          {/each}
        </div>
        <div class="gear-row">
          {#each GEAR as g (g.id)}
            <span class="chip" class:accent={$gear[g.id]} class:locked={!$gear[g.id]}>
              {g.name}{$gear[g.id] ? '' : ` · ${g.stat.toUpperCase()} ${g.at}`}
            </span>
          {/each}
        </div>
      </section>

      <!-- -------------------------------------------------- boss ladder -->
      <section class="block">
        <div class="block-head">
          <h2 class="section-title">Boss ladder</h2>
          <span class="faint">{$quest.tier} / 10 cleared</span>
        </div>

        {#if loading}
          <div class="empty">Loading the quiz bank…</div>
        {:else if bank.length === 0}
          <div class="card empty-card">
            <p class="muted">No quiz questions yet — the bosses need something to throw at you.</p>
            <button class="btn sm primary" onclick={() => navigate('study')}>
              <Icon name="layers" size={13} /> Generate a quiz first
            </button>
          </div>
        {:else}
          <div class="ladder">
            {#each tiers as t (t)}
              {@const def = tierDef(t)}
              {@const beaten = $quest.tier >= t}
              {@const open = tierUnlocked(t, $quest.tier)}
              <article class="card tile" class:locked={!open} class:beaten>
                <div class="tile-art">
                  <BossSprite tier={t} size={86} dead={beaten} />
                </div>
                <div class="tile-body">
                  <div class="tile-row">
                    <h3 class="tname truncate">{def.name}</h3>
                    <span class="chip tier-chip">T{t}</span>
                  </div>
                  <p class="faint tmeta">
                    {def.questions} Q · {def.timer / 1000}s · ATK {def.atk} · {courseLabel(t)}
                  </p>
                  {#if beaten}
                    <span class="chip green"><Icon name="check" size={11} /> Cleared</span>
                  {:else if open}
                    <button class="btn sm primary" onclick={() => startTier(t)}>Fight</button>
                  {:else}
                    <span class="chip locked">Beat tier {t - 1}</span>
                  {/if}
                </div>
              </article>
            {/each}
          </div>
        {/if}
      </section>

      <!-- ----------------------------------------------- deadline bosses -->
      <section class="block">
        <div class="block-head">
          <h2 class="section-title">Deadline bosses</h2>
          <span class="faint">{$quest.counters.dlBosses} defeated</span>
        </div>
        {#if liveBosses.length === 0 && doneBosses.length === 0}
          <div class="empty">No deadlines. Enjoy the quiet.</div>
        {:else}
          <div class="dl-list">
            {#each liveBosses as b (b.id)}
              <article class="card dlrow" class:dead={b.beaten}>
                <BossSprite tier={(b.id % 10) + 1} size={44} dead={b.beaten} />
                <div class="dlmain">
                  <div class="dltop">
                    <span class="dlname truncate">{b.title}</span>
                    <span class="chip">{b.courseCode}</span>
                  </div>
                  <div class="hpbar sm">
                    <div class="hpfill boss" style="width:{(b.hp / Math.max(1, b.maxHP)) * 100}%"></div>
                  </div>
                  <span class="faint dlmeta">
                    {b.hp} / {b.maxHP} HP · {b.overdue ? 'overdue' : `due in ${countdown(b.dueAt)}`}
                  </span>
                </div>
                {#if b.beaten}
                  <span class="chip green">Defeated</span>
                {:else if bank.length === 0}
                  <span class="chip locked">Need a quiz</span>
                {:else}
                  <button class="btn sm" onclick={() => startDeadline(b)}>Attack</button>
                {/if}
              </article>
            {/each}
            {#each doneBosses as b (b.id)}
              <article class="card dlrow dead">
                <BossSprite tier={(b.id % 10) + 1} size={44} dead />
                <div class="dlmain">
                  <div class="dltop">
                    <span class="dlname truncate">{b.title}</span>
                    <span class="chip">{b.courseCode}</span>
                  </div>
                  <span class="faint dlmeta">Submitted — bounty claimed</span>
                </div>
                <span class="chip green">Defeated</span>
              </article>
            {/each}
          </div>
        {/if}
      </section>

      <!-- ------------------------------------------------------ inventory -->
      <section class="card inv">
        <span class="section-title">Inventory</span>
        <div class="inv-group">
          <span class="faint invlabel">Skins</span>
          <div class="inv-row">
            <button class="skin" class:on={$quest.skin === ''} onclick={() => equipSkin('')} title="Default">
              <span class="swatch default"></span> Default
            </button>
            {#each SKINS as s (s.id)}
              {@const owned = $unlockedSkins.some((x) => x.id === s.id)}
              <button
                class="skin"
                class:on={$quest.skin === s.id}
                disabled={!owned}
                onclick={() => equipSkin(s.id)}
                title={owned ? `Equip ${s.name}` : 'Locked — boss loot'}
              >
                <span class="swatch" style="background:{s.accent}"></span>
                {owned ? s.name : 'Locked'}
              </button>
            {/each}
          </div>
        </div>
        <div class="inv-group">
          <span class="faint invlabel">Titles</span>
          <div class="inv-row">
            {#each TITLES as t (t.id)}
              {@const owned = $unlockedTitles.some((x) => x.id === t.id)}
              <button
                class="title-btn"
                class:on={$quest.title === t.id}
                disabled={!owned}
                onclick={() => equipTitle(t.id)}
                title={owned ? `Equip “${t.name}”` : t.how}
              >
                {owned ? t.name : `🔒 ${t.name}`}
              </button>
            {/each}
          </div>
        </div>
        <p class="faint invhint">
          Accessories equip themselves: {gearOwned.length} of {GEAR.length} unlocked.
        </p>
      </section>
    </div>
  </div>
{/if}

<style>
  .head {
    margin-bottom: 18px;
  }

  .sub {
    margin-top: 3px;
    font-size: 13px;
  }

  .grow {
    flex: 1;
  }

  /* ------------------------------------------------------ goal + recap */

  .top-grid {
    display: grid;
    grid-template-columns: minmax(280px, 1.15fr) minmax(240px, 1fr);
    gap: 14px;
  }

  .goal-card {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 16px;
  }

  .ring-wrap {
    position: relative;
    flex: none;
    width: 118px;
    height: 118px;
  }

  .ring {
    transform: rotate(-90deg);
  }

  .track {
    fill: none;
    stroke: var(--bg-subtle);
    stroke-width: 6;
  }

  .prog {
    fill: none;
    stroke: var(--accent);
    stroke-width: 6;
    stroke-linecap: round;
    transition: stroke-dashoffset 420ms cubic-bezier(0.4, 0, 0.2, 1);
  }

  .ring-mid {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 1px;
  }

  .ring-xp {
    font-size: 22px;
    font-weight: 640;
    letter-spacing: -0.02em;
    font-variant-numeric: tabular-nums;
  }

  .ring-goal {
    font-size: 11px;
  }

  .goal-body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 9px;
  }

  .streak-line {
    display: flex;
    align-items: baseline;
    gap: 7px;
    font-size: 13px;
  }

  .flame {
    font-size: 16px;
    filter: grayscale(1);
    opacity: 0.5;
  }

  .flame.lit {
    filter: none;
    opacity: 1;
  }

  .streak-n {
    font-size: 20px;
    font-weight: 650;
    font-variant-numeric: tabular-nums;
  }

  .total {
    font-size: 12px;
  }

  .goals {
    display: flex;
    flex-direction: column;
    gap: 5px;
  }

  .goal-btns {
    display: flex;
    gap: 6px;
  }

  .gbtn {
    height: 26px;
    padding: 0 11px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-elevated);
    color: var(--text-muted);
    font-size: 12.5px;
    font-weight: 550;
    font-variant-numeric: tabular-nums;
    transition: background var(--t), color var(--t), border-color var(--t);
  }

  .gbtn:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .gbtn.on {
    background: var(--accent);
    border-color: var(--accent);
    color: #fff;
  }

  .recap {
    padding: 14px 16px 12px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .bars {
    flex: 1;
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    gap: 6px;
    min-height: 92px;
  }

  .barcol {
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-height: 0;
  }

  .bartrack {
    flex: 1;
    display: flex;
    align-items: flex-end;
    background: var(--bg-subtle);
    border-radius: 4px;
    overflow: hidden;
  }

  .bar {
    width: 100%;
    background: var(--border-strong);
    border-radius: 4px;
    transition: height 300ms cubic-bezier(0.4, 0, 0.2, 1);
  }

  .bar.met {
    background: var(--accent);
  }

  .blabel {
    font-size: 10px;
    text-align: center;
  }

  /* ---------------------------------------------------------- pet stats */

  .stats {
    margin-top: 14px;
    padding: 14px 16px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .stat-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 10px;
    font-size: 11.5px;
  }

  .stat-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: 12px;
  }

  .stat {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .stat-top {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
  }

  .skey {
    font-size: 10.5px;
    font-weight: 700;
    letter-spacing: 0.06em;
    color: var(--text-faint);
  }

  .sval {
    font-size: 17px;
    font-weight: 640;
    font-variant-numeric: tabular-nums;
  }

  .sbar {
    height: 3px;
    border-radius: 999px;
    background: var(--bg-subtle);
    overflow: hidden;
  }

  .sfill {
    height: 100%;
    background: var(--accent);
    border-radius: 999px;
  }

  .sfrom {
    font-size: 10.5px;
  }

  .gear-row {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }

  .chip.locked {
    opacity: 0.5;
  }

  /* ------------------------------------------------------- boss ladder */

  .block {
    margin-top: 22px;
  }

  .block-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 10px;
    margin-bottom: 9px;
    font-size: 11.5px;
  }

  .ladder {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(212px, 1fr));
    gap: 12px;
  }

  .tile {
    display: flex;
    gap: 10px;
    padding: 10px 12px 12px 8px;
    align-items: center;
  }

  .tile.locked {
    opacity: 0.5;
  }

  .tile-art {
    flex: none;
  }

  .tile-body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 5px;
    align-items: flex-start;
  }

  .tile-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 6px;
    width: 100%;
  }

  .tname {
    font-size: 13.5px;
    font-weight: 620;
    letter-spacing: -0.01em;
  }

  .tier-chip {
    font-variant-numeric: tabular-nums;
  }

  .tmeta {
    font-size: 11px;
    line-height: 1.4;
  }

  .empty-card {
    padding: 26px 16px;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 11px;
    text-align: center;
  }

  /* ---------------------------------------------------- deadline bosses */

  .dl-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .dlrow {
    display: flex;
    align-items: center;
    gap: 11px;
    padding: 9px 12px;
  }

  .dlrow.dead {
    opacity: 0.65;
  }

  .dlmain {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .dltop {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }

  .dlname {
    font-size: 13px;
    font-weight: 550;
  }

  .dlmeta {
    font-size: 11px;
  }

  /* --------------------------------------------------------- inventory */

  .inv {
    margin-top: 22px;
    padding: 14px 16px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .inv-group {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .invlabel {
    font-size: 11px;
  }

  .inv-row {
    display: flex;
    flex-wrap: wrap;
    gap: 7px;
  }

  .skin,
  .title-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 27px;
    padding: 0 10px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-elevated);
    color: var(--text-muted);
    font-size: 12.5px;
    transition: background var(--t), border-color var(--t), color var(--t);
  }

  .skin:hover:not(:disabled),
  .title-btn:hover:not(:disabled) {
    background: var(--bg-hover);
    color: var(--text);
  }

  .skin:disabled,
  .title-btn:disabled {
    opacity: 0.45;
    cursor: default;
  }

  .skin.on,
  .title-btn.on {
    border-color: var(--accent);
    background: var(--accent-soft);
    color: var(--accent-text);
    font-weight: 550;
  }

  .swatch {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    flex: none;
    box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.14);
  }

  .swatch.default {
    background: var(--accent);
  }

  .invhint {
    font-size: 11.5px;
  }

  /* ------------------------------------------------------------- arena */

  .arena {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  .arena-top {
    display: flex;
    align-items: center;
    gap: 9px;
  }

  .qcount {
    font-size: 12px;
    font-variant-numeric: tabular-nums;
  }

  .stage {
    display: flex;
    align-items: center;
    justify-content: space-around;
    gap: 14px;
    padding: 18px 16px;
  }

  .boss-side,
  .pet-side {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    min-width: 0;
    flex: 1;
  }

  .vs {
    font-size: 12px;
    font-weight: 600;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    flex: none;
  }

  .hpwrap {
    width: 100%;
    max-width: 190px;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 4px;
  }

  .hpname {
    max-width: 100%;
    font-size: 12.5px;
    font-weight: 600;
  }

  .hpbar {
    width: 100%;
    height: 8px;
    border-radius: 999px;
    background: var(--bg-subtle);
    overflow: hidden;
  }

  .hpbar.sm {
    height: 5px;
  }

  .hpfill {
    height: 100%;
    border-radius: 999px;
    transition: width 320ms cubic-bezier(0.4, 0, 0.2, 1);
  }

  .hpfill.boss {
    background: var(--red);
  }

  .hpnum {
    font-size: 11px;
    font-variant-numeric: tabular-nums;
  }

  .hearts {
    display: flex;
    gap: 3px;
    font-size: 15px;
    line-height: 1;
  }

  .heart {
    color: var(--border-strong);
  }

  .heart.on {
    color: var(--red);
  }

  /* ------------------------------------------------------------ question */

  .qcard {
    padding: 0 0 14px;
    overflow: hidden;
  }

  .timer {
    height: 3px;
    background: var(--bg-subtle);
  }

  .timerfill {
    height: 100%;
    background: var(--accent);
    transition: width 120ms linear;
  }

  .prompt {
    padding: 15px 16px 12px;
    font-size: 14.5px;
    font-weight: 550;
    line-height: 1.45;
  }

  .opts {
    display: flex;
    flex-direction: column;
    gap: 7px;
    padding: 0 16px;
  }

  .opt {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 9px 11px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-elevated);
    color: var(--text);
    font-size: 13px;
    text-align: left;
    transition: background var(--t), border-color var(--t);
  }

  .opt:hover:not(:disabled) {
    background: var(--bg-hover);
    border-color: var(--border-strong);
  }

  .opt:disabled {
    cursor: default;
  }

  .opt.correct {
    border-color: var(--green);
    background: var(--green-soft);
  }

  .opt.wrong {
    border-color: var(--red);
    background: var(--red-soft);
  }

  .key {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 19px;
    height: 19px;
    flex: none;
    border-radius: 5px;
    background: var(--bg-subtle);
    color: var(--text-faint);
    font-size: 11px;
    font-weight: 600;
  }

  .otext {
    flex: 1;
    min-width: 0;
  }

  .hitline {
    margin: 12px 16px 0;
    font-size: 13px;
    font-weight: 600;
    color: var(--red);
  }

  .hitline.good {
    color: var(--green);
  }

  .expl {
    margin: 5px 16px 0;
    font-size: 12.5px;
    line-height: 1.5;
  }

  .keys {
    margin: 12px 16px 0;
    font-size: 11.5px;
  }

  kbd {
    padding: 1px 5px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg-subtle);
    font-family: var(--font);
    font-size: 10.5px;
  }

  /* ------------------------------------------------------------ outcome */

  .outcome {
    padding: 18px 16px;
    display: flex;
    flex-direction: column;
    gap: 9px;
    align-items: flex-start;
  }

  .outcome-title {
    font-size: 17px;
    font-weight: 640;
    letter-spacing: -0.015em;
  }

  .loot {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }

  .actions {
    display: flex;
    gap: 8px;
    margin-top: 3px;
  }

  @media (max-width: 720px) {
    .top-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
