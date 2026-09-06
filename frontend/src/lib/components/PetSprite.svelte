<script lang="ts">
  /**
   * The creature itself: pure inline SVG + CSS, no state of its own beyond a
   * blink timer. Shared by the docked sidebar pet and the free-roaming overlay.
   */
  import type { Mood } from '../pet';
  import { gear, skinAccent } from '../quest';

  interface Props {
    mood?: Mood;
    level?: number;
    /** Preview a loot skin's accent without equipping it ('' = use the equipped one). */
    skin?: string;
    /** 1 = facing right, -1 = facing left. */
    facing?: 1 | -1;
    /** Rendered box in px (the level growth is applied on top of this). */
    size?: number;
    walking?: boolean;
    sitting?: boolean;
    squash?: boolean;
  }

  let {
    mood = 'happy',
    level = 1,
    skin = '',
    facing = 1,
    size = 76,
    walking = false,
    sitting = false,
    squash = false,
  }: Props = $props();

  // Randomised blink cadence, 4–7s, re-rolled periodically.
  let blinkDur = $state(4 + Math.random() * 3);

  $effect(() => {
    const h = setInterval(() => (blinkDur = 4 + Math.random() * 3), 12_000);
    return () => clearInterval(h);
  });

  // Size grows 4% per level, capped at level 5.
  const grow = $derived(1 + 0.04 * (Math.min(5, level) - 1));
  const hasBow = $derived(level >= 3);
  const hasGlasses = $derived(level >= 5);
  const hasCrown = $derived(level >= 8);

  // Quest loot. Gear is stat-derived, so it equips itself; the skin only
  // repaints --accent locally, which every part of the sprite already uses.
  const accent = $derived(skin || $skinAccent);
  const soft = $derived(accent ? hexA(accent, 0.14) : '');
  const skinVars = $derived(accent ? `--accent:${accent};--accent-soft:${soft};` : '');

  const hasSword = $derived($gear.sword || $gear.steel);
  const steel = $derived($gear.steel);
  const hasScarf = $derived($gear.scarf);
  const hasShield = $derived($gear.shield);
  const hasBadge = $derived($gear.badge);
  const hasShoes = $derived($gear.sneakers);

  /** #rrggbb -> rgba(), so a skin gets a matching soft fill. */
  function hexA(hex: string, alpha: number): string {
    const m = /^#?([0-9a-f]{6})$/i.exec(hex.trim());
    if (!m) return hex;
    const n = parseInt(m[1], 16);
    return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`;
  }

  // Sitting reads as "eyes half closed", same lids the sleepy mood uses.
  const lidded = $derived(sitting || mood === 'sleepy');
</script>

<svg
  class="sprite mood-{mood}"
  class:walking
  class:sitting
  class:squash
  viewBox="0 0 100 104"
  width={size}
  height={size}
  style="--grow:{grow};--blink:{blinkDur.toFixed(2)}s;--face:{facing};{skinVars}"
  aria-hidden="true"
>
  <g class="facer">
    <g class="bob">
      <g class="wiggle">
        <g class="squasher">
          {#if hasCrown}
            <path class="crown" d="M38 15V5l6 5 6-9 6 9 6-5v10z" />
          {/if}

          <!-- legs (behind the body) -->
          {#if !sitting}
            <g class="legs">
              <g class="legw l">
                <rect class="leg" x="36" y="82" width="8" height="16" rx="4" />
                {#if hasShoes}<rect class="shoe" x="32" y="93" width="16" height="7" rx="3.5" />{/if}
              </g>
              <g class="legw r">
                <rect class="leg" x="56" y="82" width="8" height="16" rx="4" />
                {#if hasShoes}<rect class="shoe" x="52" y="93" width="16" height="7" rx="3.5" />{/if}
              </g>
            </g>
          {/if}

          <!-- quest gear that sits behind the body -->
          {#if hasShield}
            <path class="shield" d="M6 55 20 49l14 6v10c0 8.5-7.6 12.8-14 15.4C13.6 77.8 6 73.5 6 65z" />
            <path class="shield-boss" d="M20 58v13" />
          {/if}
          {#if hasSword}
            <g class="sword" class:steel>
              <path class="blade" d="M88 85 95 46l5 2-7 39z" />
              <path class="guard" d="M82 79 96 87" />
              <path class="grip" d="M85 84 90 93" />
            </g>
          {/if}

          <!-- ears (behind the head) -->
          <path class="ear" d="M28 34 25.5 16 44 25.5z" />
          <path class="ear" d="M72 34 74.5 16 56 25.5z" />

          <!-- head / body -->
          <path
            class="body"
            d="M50 24c24 0 36 15 36 34 0 20-15 34-36 34S14 78 14 58c0-19 12-34 36-34z"
          />

          {#if hasScarf}
            <path class="scarf" d="M23 79q27 13 54 0v8q-27 13-54 0z" />
            <path class="scarf tail" d="M71 85 80 99l-7-2-4-8z" />
          {/if}

          {#if hasBadge}
            <path
              class="badge"
              d="M29 75.6c-1.7-2.7-5.6-1.4-5.6 1.5 0 2.7 3.6 4.6 5.6 6.3 2-1.7 5.6-3.6 5.6-6.3 0-2.9-3.9-4.2-5.6-1.5z"
            />
          {/if}

          {#if hasBow}
            <g class="bow">
              <path d="M50 86 42.5 81v10z" />
              <path d="M50 86 57.5 81v10z" />
              <circle cx="50" cy="86" r="2.4" />
            </g>
          {/if}

          <!-- brows -->
          {#if !lidded && (mood === 'worried' || mood === 'panic')}
            <path class="brow" d="M30 44 41 40.5" />
            <path class="brow" d="M70 44 59 40.5" />
          {/if}

          <!-- eyes -->
          {#if lidded}
            <path class="lid" d="M32 57q6 5.5 12 0" />
            <path class="lid" d="M56 57q6 5.5 12 0" />
          {:else if mood === 'busy'}
            <g class="spinner-eye" style="transform-origin:38px 57px">
              <path class="arc" d="M43 57a5 5 0 1 0-5 5" />
            </g>
            <g class="spinner-eye slow" style="transform-origin:62px 57px">
              <path class="arc" d="M67 57a5 5 0 1 0-5 5" />
            </g>
          {:else if mood === 'panic'}
            <g class="blinker">
              <circle class="eye-white" cx="38" cy="57" r="7" />
              <circle class="eye-white" cx="62" cy="57" r="7" />
              <circle class="pupil" cx="38" cy="57.5" r="2.8" />
              <circle class="pupil" cx="62" cy="57.5" r="2.8" />
            </g>
          {:else}
            <g class="blinker">
              <circle class="eye" cx="38" cy="57" r="4.6" />
              <circle class="eye" cx="62" cy="57" r="4.6" />
            </g>
          {/if}

          {#if hasGlasses}
            <g class="glasses">
              <circle cx="38" cy="57" r="9" />
              <circle cx="62" cy="57" r="9" />
              <path d="M47 57h6" />
            </g>
          {/if}

          <!-- mouth -->
          {#if mood === 'panic'}
            <ellipse class="mouth-fill" cx="50" cy="73" rx="3.6" ry="4.6" />
          {:else if mood === 'worried'}
            <path class="mouth" d="M43 74q7-4 14 0" />
          {:else if mood === 'busy'}
            <path class="mouth" d="M45 73h10" />
          {:else if lidded}
            <path class="mouth" d="M46 72q4 3.5 8 0" />
          {:else if mood === 'proud'}
            <path class="mouth" d="M41 70q9 9 18 0" />
          {:else}
            <path class="mouth" d="M43 70q7 6.5 14 0" />
          {/if}
        </g>
      </g>

      <!-- mood extras -->
      {#if mood === 'panic'}
        <path class="sweat" d="M92 40s-4 5-4 7.4a4 4 0 0 0 8 0C96 45 92 40 92 40z" />
      {/if}

      {#if mood === 'sleepy'}
        <text class="zzz z1" x="76" y="34">z</text>
        <text class="zzz z2" x="84" y="24">z</text>
      {/if}

      {#if mood === 'proud'}
        <g class="sparkles">
          <path class="sp s1" d="M84 30 85.4 34.6 90 36 85.4 37.4 84 42 82.6 37.4 78 36 82.6 34.6z" />
          <path class="sp s2" d="M17 44 18 47.2 21.2 48 18 48.8 17 52 16 48.8 12.8 48 16 47.2z" />
          <path class="sp s3" d="M70 12 71 15.2 74.2 16 71 16.8 70 20 69 16.8 65.8 16 69 15.2z" />
        </g>
      {/if}
    </g>
  </g>
</svg>

<style>
  .sprite {
    display: block;
    transform: scale(var(--grow, 1));
    transform-origin: 50% 100%;
    overflow: visible;
  }

  .facer {
    transform-box: view-box;
    transform-origin: 50px 52px;
    transform: scaleX(var(--face, 1));
  }

  /* ------------------------------------------------------------ creature */

  .body {
    fill: var(--accent-soft);
    stroke: var(--accent);
    stroke-width: 2.2;
    stroke-linejoin: round;
  }

  .ear {
    fill: var(--accent-soft);
    stroke: var(--accent);
    stroke-width: 2.2;
    stroke-linejoin: round;
  }

  .leg {
    fill: var(--accent-soft);
    stroke: var(--accent);
    stroke-width: 2;
  }

  .legw {
    transform-box: fill-box;
    transform-origin: 50% 10%;
  }

  .sprite.walking .legw.l {
    animation: step 0.42s ease-in-out infinite;
  }

  .sprite.walking .legw.r {
    animation: step 0.42s ease-in-out 0.21s infinite;
  }

  .eye,
  .pupil {
    fill: var(--accent);
  }

  .eye-white {
    fill: var(--bg-elevated);
    stroke: var(--accent);
    stroke-width: 2;
  }

  .lid,
  .mouth,
  .brow,
  .arc {
    fill: none;
    stroke: var(--accent);
    stroke-width: 2.2;
    stroke-linecap: round;
  }

  .mouth-fill {
    fill: var(--accent);
  }

  .brow {
    stroke-width: 2;
  }

  .arc {
    stroke-width: 2.4;
  }

  .sweat {
    fill: var(--accent);
    opacity: 0.55;
    animation: drip 1.6s ease-in-out infinite;
  }

  .zzz {
    fill: var(--text-faint);
    font-family: var(--font);
    font-size: 13px;
    font-weight: 650;
  }

  .z1 {
    animation: float 2.6s ease-in-out infinite;
  }

  .z2 {
    font-size: 10px;
    animation: float 2.6s ease-in-out 1.3s infinite;
  }

  .sp {
    fill: var(--accent);
  }

  .s1 {
    animation: twinkle 1.4s ease-in-out infinite;
  }

  .s2 {
    animation: twinkle 1.4s ease-in-out 0.45s infinite;
  }

  .s3 {
    animation: twinkle 1.4s ease-in-out 0.9s infinite;
  }

  /* accessories */

  .bow path {
    fill: var(--accent);
    opacity: 0.85;
  }

  .bow circle {
    fill: var(--bg-sidebar);
  }

  .crown {
    fill: none;
    stroke: var(--amber);
    stroke-width: 2.2;
    stroke-linejoin: round;
  }

  .glasses {
    fill: none;
    stroke: var(--text-muted);
    stroke-width: 1.6;
    opacity: 0.85;
  }

  /* quest gear */

  .shoe {
    fill: var(--text-muted);
    stroke: var(--text);
    stroke-width: 1.4;
    opacity: 0.9;
  }

  .shield {
    fill: var(--bg-subtle);
    stroke: var(--text-muted);
    stroke-width: 2.2;
    stroke-linejoin: round;
  }

  .shield-boss {
    fill: none;
    stroke: var(--text-muted);
    stroke-width: 2;
    stroke-linecap: round;
  }

  .blade {
    fill: #b98a4e;
    stroke: #8a6534;
    stroke-width: 1.6;
    stroke-linejoin: round;
  }

  .sword.steel .blade {
    fill: #cdd2da;
    stroke: #8b919b;
  }

  .guard,
  .grip {
    fill: none;
    stroke: #8a6534;
    stroke-width: 3;
    stroke-linecap: round;
  }

  .sword.steel .guard,
  .sword.steel .grip {
    stroke: #6f7681;
  }

  .scarf {
    fill: var(--red);
    opacity: 0.85;
  }

  .badge {
    fill: var(--red);
    stroke: var(--bg-elevated);
    stroke-width: 1.2;
  }

  /* ----------------------------------------------------------- animation */

  .bob {
    animation: bob 3s ease-in-out infinite;
    transform-box: view-box;
    transform-origin: 50px 92px;
  }

  .mood-worried .bob {
    animation-duration: 2s;
  }

  .mood-panic .bob {
    animation-duration: 1.3s;
  }

  .mood-busy .bob {
    animation: pulse 1.1s ease-in-out infinite;
  }

  .mood-sleepy .bob {
    animation-duration: 5s;
  }

  .sprite.walking .bob {
    animation-duration: 0.42s;
  }

  .sprite.sitting .bob {
    animation-duration: 4.5s;
    transform: translateY(7px);
  }

  .wiggle {
    transform-box: view-box;
    transform-origin: 50px 92px;
  }

  .mood-panic .wiggle {
    animation: wiggle 0.34s ease-in-out infinite;
  }

  .blinker {
    transform-box: fill-box;
    transform-origin: center;
    animation: blink var(--blink, 5s) steps(1, end) infinite;
  }

  .spinner-eye {
    transform-box: view-box;
    animation: spin-eye 1.1s linear infinite;
  }

  .spinner-eye.slow {
    animation-duration: 1.5s;
  }

  .squasher {
    transform-box: view-box;
    transform-origin: 50px 92px;
  }

  .sprite.squash .squasher {
    animation: squash 300ms cubic-bezier(0.34, 1.4, 0.5, 1) 1;
  }

  @keyframes step {
    0%,
    100% {
      transform: rotate(-16deg);
    }
    50% {
      transform: rotate(16deg);
    }
  }

  @keyframes bob {
    0%,
    100% {
      transform: translateY(0);
    }
    50% {
      transform: translateY(-3.5px);
    }
  }

  @keyframes pulse {
    0%,
    100% {
      transform: scale(1);
    }
    50% {
      transform: scale(1.045);
    }
  }

  @keyframes wiggle {
    0%,
    100% {
      transform: translateX(-1.1px) rotate(-1.1deg);
    }
    50% {
      transform: translateX(1.1px) rotate(1.1deg);
    }
  }

  @keyframes blink {
    0%,
    94% {
      transform: scaleY(1);
    }
    96%,
    98% {
      transform: scaleY(0.1);
    }
    100% {
      transform: scaleY(1);
    }
  }

  @keyframes spin-eye {
    to {
      transform: rotate(360deg);
    }
  }

  @keyframes squash {
    0% {
      transform: scale(1, 1);
    }
    40% {
      transform: scale(1.1, 0.86);
    }
    100% {
      transform: scale(1, 1);
    }
  }

  @keyframes drip {
    0%,
    100% {
      transform: translateY(0);
      opacity: 0.55;
    }
    60% {
      transform: translateY(4px);
      opacity: 0.25;
    }
  }

  @keyframes float {
    0% {
      transform: translate(0, 0);
      opacity: 0;
    }
    30% {
      opacity: 0.9;
    }
    100% {
      transform: translate(5px, -12px);
      opacity: 0;
    }
  }

  @keyframes twinkle {
    0%,
    100% {
      opacity: 0.15;
      transform: scale(0.75);
    }
    50% {
      opacity: 1;
      transform: scale(1);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .bob,
    .wiggle,
    .spinner-eye,
    .blinker,
    .sweat,
    .z1,
    .z2,
    .sp,
    .legw,
    .sprite.squash .squasher {
      animation: none !important;
    }
  }
</style>
