<script lang="ts">
  /**
   * The creature itself: pure inline SVG + CSS, no state of its own beyond a
   * blink timer. Shared by the docked sidebar pet and the free-roaming overlay.
   */
  import type { Mood } from '../pet';

  interface Props {
    mood?: Mood;
    level?: number;
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
  style="--grow:{grow};--blink:{blinkDur.toFixed(2)}s;--face:{facing}"
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
              <rect class="leg l" x="36" y="82" width="8" height="16" rx="4" />
              <rect class="leg r" x="56" y="82" width="8" height="16" rx="4" />
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
    transform-box: fill-box;
    transform-origin: 50% 10%;
  }

  .sprite.walking .leg.l {
    animation: step 0.42s ease-in-out infinite;
  }

  .sprite.walking .leg.r {
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
    .leg,
    .sprite.squash .squasher {
      animation: none !important;
    }
  }
</style>
