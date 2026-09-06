<script lang="ts">
  /**
   * One draggable column divider, shared by the sidebar, the folder tree and
   * the viewer pane.
   *
   * Deliberately delta-based rather than "pane width = pointer x − pane left":
   * the same component then works for a pane on either side of the handle, and
   * a pane that is already clamped does not jump when the pointer comes back.
   *
   * Keyboard: focus it and use ←/→ (Shift for a bigger step). Double-click
   * resets to `def`, the way editors do.
   */
  interface Props {
    value: number;
    min: number;
    max: number;
    /** Width restored by a double-click. */
    def: number;
    /** True when the resized pane is to the *right* of the handle. */
    invert?: boolean;
    label: string;
    /** Position absolutely against the parent's right edge instead of flowing. */
    absolute?: boolean;
    onChange: (v: number) => void;
  }

  let { value, min, max, def, invert = false, label, absolute = false, onChange }: Props = $props();

  const STEP = 16;
  const BIG = 48;

  let dragging = $state(false);
  let startX = 0;
  let startV = 0;

  const clamp = (v: number) => Math.min(max, Math.max(min, v));

  function onPointerDown(e: PointerEvent) {
    if (e.button !== 0) return;
    e.preventDefault();
    dragging = true;
    startX = e.clientX;
    startV = value;
    try {
      (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    } catch {
      /* synthetic events have no capturable pointer */
    }
  }

  function onPointerMove(e: PointerEvent) {
    if (!dragging) return;
    const d = e.clientX - startX;
    onChange(clamp(startV + (invert ? -d : d)));
  }

  function endDrag(e: PointerEvent) {
    if (!dragging) return;
    dragging = false;
    try {
      (e.currentTarget as HTMLElement).releasePointerCapture?.(e.pointerId);
    } catch {
      /* not captured */
    }
  }

  function onKeyDown(e: KeyboardEvent) {
    const dir = e.key === 'ArrowLeft' ? -1 : e.key === 'ArrowRight' ? 1 : 0;
    if (!dir) return;
    e.preventDefault();
    e.stopPropagation();
    onChange(clamp(value + dir * (e.shiftKey ? BIG : STEP) * (invert ? -1 : 1)));
  }
</script>

<!--
  A focusable `separator` (the ARIA window-splitter pattern) rather than a
  button: it holds a value, and a button may not take that role.
-->
<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
<div
  tabindex="0"
  class="rh"
  class:dragging
  class:abs={absolute}
  aria-label="{label} (arrow keys to resize, double-click to reset)"
  aria-valuenow={Math.round(value)}
  aria-valuemin={min}
  aria-valuemax={max}
  role="separator"
  aria-orientation="vertical"
  onpointerdown={onPointerDown}
  onpointermove={onPointerMove}
  onpointerup={endDrag}
  onpointercancel={endDrag}
  ondblclick={() => onChange(def)}
  onkeydown={onKeyDown}
></div>

<style>
  .rh {
    flex: none;
    width: 5px;
    padding: 0;
    border: none;
    border-radius: 0;
    background: var(--border);
    cursor: col-resize;
    position: relative;
    touch-action: none;
    transition: background var(--t);
  }

  /* Widen the hit area without widening the line. */
  .rh::before {
    content: '';
    position: absolute;
    inset: 0 -3px;
  }

  .rh.abs {
    position: absolute;
    top: 0;
    right: -3px;
    bottom: 0;
    width: 6px;
    z-index: 3;
    background: transparent;
  }

  .rh:hover,
  .rh:focus-visible,
  .rh.dragging {
    background: var(--accent);
    outline: none;
  }
</style>
