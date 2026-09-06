<script lang="ts">
  /**
   * Procedural boss monsters — ten distinct creatures generated from the tier
   * number alone, so the ladder needs no image assets. The tier seeds a hue,
   * one of three silhouettes (blob / spike / eye), and the eye and horn count.
   */
  interface Props {
    tier: number;
    size?: number;
    /** flash red on taking a hit */
    hurt?: boolean;
    /** slumped and faded once beaten */
    dead?: boolean;
  }

  let { tier = 1, size = 150, hurt = false, dead = false }: Props = $props();

  const t = $derived(Math.max(1, Math.min(10, Math.round(tier))));
  const hue = $derived((t * 41 + 6) % 360);
  const fill = $derived(`hsl(${hue} 58% 54%)`);
  const deep = $derived(`hsl(${hue} 54% 33%)`);
  const pale = $derived(`hsl(${hue} 72% 78%)`);

  /** 0 = blob, 1 = spike, 2 = eye. */
  const shape = $derived((t - 1) % 3);
  const eyeCount = $derived(1 + ((t + 1) % 3));
  const horns = $derived(t >= 4 ? 2 + (t % 2) : 0);
  const fangs = $derived(t >= 5);

  /** Deterministic 0..1 from an integer — same monster every render. */
  function rand(seed: number): number {
    const x = Math.sin(seed * 127.1 + 311.7) * 43758.5453;
    return x - Math.floor(x);
  }

  const CX = 60;
  const CY = 66;

  function bodyPath(tier: number, kind: number): string {
    if (kind === 2) {
      // A single great eyeball: a plain circle, drawn as two arcs.
      const r = 38;
      return `M${CX - r} ${CY}a${r} ${r} 0 1 0 ${r * 2} 0a${r} ${r} 0 1 0 ${-r * 2} 0z`;
    }

    const n = kind === 1 ? 18 : 12;
    const pts: Array<[number, number]> = [];
    for (let i = 0; i < n; i++) {
      const a = (i / n) * Math.PI * 2 - Math.PI / 2;
      let r: number;
      if (kind === 1) r = i % 2 === 0 ? 44 : 27;
      else r = 31 + rand(tier * 17 + i) * 11;
      pts.push([CX + Math.cos(a) * r * 1.06, CY + Math.sin(a) * r]);
    }

    if (kind === 1) {
      // Spiky: straight edges, so the points stay sharp.
      return `M${pts.map(([x, y]) => `${x.toFixed(1)} ${y.toFixed(1)}`).join('L')}z`;
    }

    // Blobby: quadratic curves through edge midpoints.
    const mid = (a: [number, number], b: [number, number]) =>
      [(a[0] + b[0]) / 2, (a[1] + b[1]) / 2] as [number, number];
    const start = mid(pts[n - 1], pts[0]);
    let d = `M${start[0].toFixed(1)} ${start[1].toFixed(1)}`;
    for (let i = 0; i < n; i++) {
      const c = pts[i];
      const m = mid(c, pts[(i + 1) % n]);
      d += `Q${c[0].toFixed(1)} ${c[1].toFixed(1)} ${m[0].toFixed(1)} ${m[1].toFixed(1)}`;
    }
    return `${d}z`;
  }

  const body = $derived(bodyPath(t, shape));

  /** Eyes spread across the face, sized down as the count goes up. */
  const eyeSpots = $derived.by(() => {
    const n = eyeCount;
    const r = shape === 2 ? 17 : n === 1 ? 12 : n === 2 ? 8.5 : 6.5;
    const spread = shape === 2 ? 0 : n === 1 ? 0 : n === 2 ? 15 : 19;
    const out: Array<{ x: number; y: number; r: number }> = [];
    for (let i = 0; i < n; i++) {
      const off = n === 1 ? 0 : (i / (n - 1)) * 2 - 1;
      out.push({ x: CX + off * spread, y: CY - (shape === 2 ? 2 : 8) + (n === 3 && i === 1 ? -5 : 0), r });
    }
    return out;
  });

  const hornPath = $derived.by(() => {
    const out: string[] = [];
    for (let i = 0; i < horns; i++) {
      const off = horns === 1 ? 0 : (i / (horns - 1)) * 2 - 1;
      const x = CX + off * 26;
      const h = 14 + rand(t * 5 + i) * 12;
      out.push(`M${(x - 7).toFixed(1)} 36L${x.toFixed(1)} ${(36 - h).toFixed(1)}L${(x + 7).toFixed(1)} 36z`);
    }
    return out;
  });
</script>

<svg
  class="boss"
  class:hurt
  class:dead
  viewBox="0 0 120 120"
  width={size}
  height={size}
  style="--fill:{fill};--deep:{deep};--pale:{pale}"
  aria-hidden="true"
>
  <ellipse class="shade" cx={CX} cy="110" rx="34" ry="6" />

  <g class="creature">
    {#each hornPath as d (d)}
      <path class="horn" {d} />
    {/each}

    <path class="skin" d={body} />

    {#if shape !== 2}
      <path class="belly" d="M{CX - 20} {CY + 16}q20 14 40 0q-4 20-20 20t-20-20z" />
    {/if}

    {#each eyeSpots as e, i (i)}
      <circle class="white" cx={e.x} cy={e.y} r={e.r} />
      <circle class="pupil" cx={e.x} cy={e.y + e.r * 0.14} r={Math.max(2.2, e.r * 0.42)} />
      <circle class="glint" cx={e.x - e.r * 0.3} cy={e.y - e.r * 0.34} r={Math.max(1, e.r * 0.16)} />
    {/each}

    {#if fangs}
      <path class="mouth" d="M{CX - 15} {CY + 26}q15 12 30 0q-15 20-30 0z" />
      <path class="fang" d="M{CX - 9} {CY + 28}l4 8 4-8z" />
      <path class="fang" d="M{CX + 1} {CY + 28}l4 8 4-8z" />
    {:else}
      <path class="grin" d="M{CX - 13} {CY + 26}q13 11 26 0" />
    {/if}
  </g>
</svg>

<style>
  .boss {
    display: block;
    overflow: visible;
  }

  .shade {
    fill: rgba(0, 0, 0, 0.14);
  }

  .creature {
    transform-box: view-box;
    transform-origin: 60px 104px;
    animation: hover 2.8s ease-in-out infinite;
  }

  .skin {
    fill: var(--fill);
    stroke: var(--deep);
    stroke-width: 2.4;
    stroke-linejoin: round;
  }

  .belly {
    fill: var(--pale);
    opacity: 0.45;
  }

  .horn {
    fill: var(--deep);
  }

  .white {
    fill: #fff;
    stroke: var(--deep);
    stroke-width: 1.8;
  }

  .pupil {
    fill: #17171c;
  }

  .glint {
    fill: #fff;
  }

  .mouth {
    fill: var(--deep);
  }

  .fang {
    fill: #fff;
  }

  .grin {
    fill: none;
    stroke: var(--deep);
    stroke-width: 2.6;
    stroke-linecap: round;
  }

  .boss.hurt .creature {
    animation: shake 260ms ease-in-out 1;
  }

  .boss.hurt .skin {
    fill: #ef4444;
  }

  .boss.dead {
    opacity: 0.35;
    filter: grayscale(0.85);
  }

  .boss.dead .creature {
    animation: none;
    transform: rotate(14deg) translateY(8px);
  }

  @keyframes hover {
    0%,
    100% {
      transform: translateY(0) scale(1, 1);
    }
    50% {
      transform: translateY(-5px) scale(1.02, 0.98);
    }
  }

  @keyframes shake {
    0%,
    100% {
      transform: translateX(0);
    }
    25% {
      transform: translateX(-5px) rotate(-3deg);
    }
    75% {
      transform: translateX(5px) rotate(3deg);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .creature {
      animation: none !important;
    }
  }
</style>
