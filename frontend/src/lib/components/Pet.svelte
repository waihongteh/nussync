<script lang="ts">
  /**
   * The docked sidebar pet: bubble, level meter and click handling around the
   * shared <PetSprite>. Mood, level and quips all come from ../pet.ts.
   */
  import { bubble, level, levelProgress, mood, petName, pokePet, xp } from '../pet';
  import { equippedTitle, streak } from '../quest';
  import PetSprite from './PetSprite.svelte';
  import { tip } from '../tooltip';

  interface Props {
    /** Rail mode: a 28px head, no level meter, quip bubble floats to the right. */
    compact?: boolean;
  }

  let { compact = false }: Props = $props();

  let squashing = $state(false);
  let squashTimer: ReturnType<typeof setTimeout> | undefined;

  $effect(() => () => clearTimeout(squashTimer));

  const label = $derived(`${$petName}, feeling ${$mood}. Level ${$level}, ${$xp} xp.`);

  function poke() {
    squashing = false;
    clearTimeout(squashTimer);
    // restart the keyframe on repeat clicks
    requestAnimationFrame(() => {
      squashing = true;
      squashTimer = setTimeout(() => (squashing = false), 320);
    });
    // Quips as before; five clicks inside two seconds opens the Arcade.
    pokePet();
  }
</script>

<div class="pet-slot" class:compact>
  {#if $bubble}
    <div class="bubble" class:side={compact} role="status">{$bubble}</div>
  {:else}
    <!-- keep role=status mounted so updates are announced -->
    <div class="bubble-sr" role="status"></div>
  {/if}

  <button
    class="pet"
    onclick={poke}
    aria-label={label}
    title={compact ? undefined : `${$petName} — ${$mood}`}
    use:tip={{ text: `${$petName} — ${$mood} · Lv ${$level}`, side: 'right', disabled: !compact }}
    type="button"
  >
    <PetSprite mood={$mood} level={$level} size={compact ? 28 : 76} squash={squashing} />
  </button>

  {#if !compact}
    {#if $equippedTitle}
      <span class="title-tag truncate" title="Equipped title">{$equippedTitle}</span>
    {/if}

    <div class="meta">
      <span class="lv">Lv {$level}</span>
      {#if $streak > 0}
        <span class="streak" title="{$streak}-day streak">🔥{$streak}</span>
      {/if}
      <span class="xpbar"><span class="xpfill" style="width:{Math.round($levelProgress * 100)}%"></span></span>
    </div>
  {/if}
</div>

<style>
  .pet-slot {
    position: relative;
    flex: none;
    height: 128px;
    margin: 0 10px 2px;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: flex-end;
    padding-bottom: 2px;
  }

  .pet-slot.compact {
    height: 40px;
    margin: 0 0 2px;
    justify-content: center;
    padding-bottom: 0;
  }

  .pet {
    display: block;
    border-radius: 50%;
    line-height: 0;
  }

  /* Rail: the quip cannot fit above a 56px column, so it floats to the right. */
  .bubble.side {
    left: calc(100% + 8px);
    right: auto;
    bottom: 4px;
    width: 172px;
    text-align: left;
    z-index: 40;
  }

  .bubble.side::after {
    left: -5px;
    bottom: 12px;
    margin-left: 0;
    border-right: none;
    border-bottom: none;
    border-left: 1px solid var(--border);
    border-top: 1px solid var(--border);
  }

  /* -------------------------------------------------------------- bubble */

  .bubble {
    position: absolute;
    left: 2px;
    right: 2px;
    bottom: 104px;
    z-index: 2;
    padding: 6px 9px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-elevated);
    box-shadow: var(--shadow-pop);
    color: var(--text);
    font-size: 11.5px;
    line-height: 1.35;
    text-align: center;
    animation: bubble-in 140ms ease-out;
  }

  .bubble::after {
    content: '';
    position: absolute;
    left: 50%;
    bottom: -5px;
    width: 8px;
    height: 8px;
    margin-left: -4px;
    transform: rotate(45deg);
    background: var(--bg-elevated);
    border-right: 1px solid var(--border);
    border-bottom: 1px solid var(--border);
  }

  .bubble-sr {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
  }

  @keyframes bubble-in {
    from {
      opacity: 0;
      transform: translateY(3px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  /* ---------------------------------------------------------------- meta */

  .meta {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 7px;
    width: 100%;
    max-width: 116px;
    margin-top: 2px;
  }

  .title-tag {
    max-width: 116px;
    font-size: 10px;
    font-weight: 600;
    letter-spacing: 0.01em;
    color: var(--accent-text);
  }

  .streak {
    font-size: 9.5px;
    font-weight: 650;
    color: var(--amber);
    font-variant-numeric: tabular-nums;
    flex: none;
  }

  .lv {
    font-size: 10px;
    font-weight: 600;
    letter-spacing: 0.04em;
    color: var(--text-faint);
    font-variant-numeric: tabular-nums;
    flex: none;
  }

  .xpbar {
    flex: 1;
    height: 2px;
    border-radius: 999px;
    background: var(--border);
    overflow: hidden;
  }

  .xpfill {
    display: block;
    height: 100%;
    border-radius: 999px;
    background: var(--accent);
    transition: width 300ms cubic-bezier(0.4, 0, 0.2, 1);
  }

  @media (prefers-reduced-motion: reduce) {
    .bubble {
      animation: none;
    }
  }
</style>
