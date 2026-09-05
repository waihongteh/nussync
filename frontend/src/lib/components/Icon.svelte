<script lang="ts">
  /**
   * Inline lucide-style icons. Hand-written paths — no icon library, so the
   * bundle stays tiny and the stroke weight stays consistent everywhere.
   */
  interface Props {
    name: string;
    size?: number;
    stroke?: number;
    class?: string;
  }

  let { name, size = 15, stroke = 1.6, class: klass = '' }: Props = $props();

  const PATHS: Record<string, string> = {
    home: '<path d="M3 9.5 10 3l7 6.5"/><path d="M4.7 8.6V16a1 1 0 0 0 1 1h3.1v-4.2h2.4V17h3.1a1 1 0 0 0 1-1V8.6"/>',
    files: '<path d="M2.5 5.2a1.2 1.2 0 0 1 1.2-1.2h3.1l1.5 1.9h6.9a1.2 1.2 0 0 1 1.2 1.2v7.7a1.2 1.2 0 0 1-1.2 1.2H3.7a1.2 1.2 0 0 1-1.2-1.2z"/>',
    deadlines: '<circle cx="10" cy="10.6" r="6.6"/><path d="M10 7.2v3.6l2.4 1.4"/><path d="M6.4 2.6 4 4.4"/><path d="m13.6 2.6 2.4 1.8"/>',
    announcements: '<path d="M4 8.2v3.6h2.6L11 15V5.4L6.6 8.2z"/><path d="M13.6 7.6a3.6 3.6 0 0 1 0 4.8"/><path d="M15.6 5.4a6.4 6.4 0 0 1 0 9.2"/>',
    grades: '<path d="M3.5 16.5V9"/><path d="M8.2 16.5V4.6"/><path d="M12.9 16.5v-5.2"/><path d="M17 16.5V7.2"/>',
    settings: '<circle cx="10" cy="10" r="2.6"/><path d="M15.9 12.3a1.3 1.3 0 0 0 .26 1.43l.05.05a1.6 1.6 0 1 1-2.26 2.26l-.05-.05a1.3 1.3 0 0 0-1.43-.26 1.3 1.3 0 0 0-.79 1.19v.14a1.6 1.6 0 0 1-3.2 0v-.07a1.3 1.3 0 0 0-.85-1.19 1.3 1.3 0 0 0-1.43.26l-.05.05a1.6 1.6 0 1 1-2.26-2.26l.05-.05a1.3 1.3 0 0 0 .26-1.43 1.3 1.3 0 0 0-1.19-.79h-.14a1.6 1.6 0 0 1 0-3.2h.07a1.3 1.3 0 0 0 1.19-.85 1.3 1.3 0 0 0-.26-1.43l-.05-.05a1.6 1.6 0 1 1 2.26-2.26l.05.05a1.3 1.3 0 0 0 1.43.26h.06a1.3 1.3 0 0 0 .79-1.19v-.14a1.6 1.6 0 0 1 3.2 0v.07a1.3 1.3 0 0 0 .79 1.19 1.3 1.3 0 0 0 1.43-.26l.05-.05a1.6 1.6 0 1 1 2.26 2.26l-.05.05a1.3 1.3 0 0 0-.26 1.43v.06a1.3 1.3 0 0 0 1.19.79h.14a1.6 1.6 0 0 1 0 3.2h-.07a1.3 1.3 0 0 0-1.19.79z"/>',
    search: '<circle cx="9" cy="9" r="5.4"/><path d="m17 17-4.2-4.2"/>',
    sync: '<path d="M16.5 8.4a6.6 6.6 0 0 0-11.3-3L3 7.6"/><path d="M3.5 11.6a6.6 6.6 0 0 0 11.3 3L17 12.4"/><path d="M3 3.6v4h4"/><path d="M17 16.4v-4h-4"/>',
    chevronRight: '<path d="m7.6 4.6 5.2 5.4-5.2 5.4"/>',
    chevronDown: '<path d="m4.6 7.6 5.4 5.2 5.4-5.2"/>',
    chevronLeft: '<path d="M12.4 4.6 7.2 10l5.2 5.4"/>',
    folder: '<path d="M2.6 5.4a1.2 1.2 0 0 1 1.2-1.2h3.2l1.5 1.9h6.7a1.2 1.2 0 0 1 1.2 1.2v7.3a1.2 1.2 0 0 1-1.2 1.2H3.8a1.2 1.2 0 0 1-1.2-1.2z"/>',
    folderOpen: '<path d="M2.6 15.4V5.4a1.2 1.2 0 0 1 1.2-1.2h3.2l1.5 1.9h6.7a1.2 1.2 0 0 1 1.2 1.2v1.1"/><path d="M2.6 15.4 4.9 8.9h13l-2.3 6.5z"/>',
    file: '<path d="M11.4 2.8H6a1.4 1.4 0 0 0-1.4 1.4v11.6A1.4 1.4 0 0 0 6 17.2h8a1.4 1.4 0 0 0 1.4-1.4V6.8z"/><path d="M11.4 2.8v4h4"/>',
    fileText: '<path d="M11.4 2.8H6a1.4 1.4 0 0 0-1.4 1.4v11.6A1.4 1.4 0 0 0 6 17.2h8a1.4 1.4 0 0 0 1.4-1.4V6.8z"/><path d="M11.4 2.8v4h4"/><path d="M7.4 11h5.2"/><path d="M7.4 13.8h5.2"/>',
    slides: '<rect x="3" y="3.6" width="14" height="9.6" rx="1.3"/><path d="M10 13.2v3.2"/><path d="M7 16.4h6"/>',
    sheet: '<rect x="3.2" y="3.6" width="13.6" height="12.8" rx="1.3"/><path d="M3.2 8.4h13.6"/><path d="M8.2 8.4v8"/>',
    image: '<rect x="3" y="3.6" width="14" height="12.8" rx="1.4"/><circle cx="7.6" cy="8" r="1.2"/><path d="m3.4 14.4 3.8-3.6 3 2.7 2.6-2.3 3.8 3.5"/>',
    video: '<rect x="2.6" y="4.6" width="10.4" height="10.8" rx="1.4"/><path d="m13 10.6 4.4-2.6v4l-4.4-2.6z"/>',
    archive: '<rect x="2.8" y="3.6" width="14.4" height="3.6" rx="1"/><path d="M4.2 7.2v8a1.3 1.3 0 0 0 1.3 1.3h9a1.3 1.3 0 0 0 1.3-1.3v-8"/><path d="M8.4 10.4h3.2"/>',
    code: '<path d="m7 6.6-4 3.8 4 3.8"/><path d="m13 6.6 4 3.8-4 3.8"/>',
    external: '<path d="M11.6 3.6h4.8v4.8"/><path d="m16.4 3.6-7 7"/><path d="M14 11.8v3.6a1.4 1.4 0 0 1-1.4 1.4H4.8a1.4 1.4 0 0 1-1.4-1.4V7.6a1.4 1.4 0 0 1 1.4-1.4h3.6"/>',
    check: '<path d="m4.4 10.4 3.6 3.6 7.6-8"/>',
    checkCircle: '<circle cx="10" cy="10" r="7"/><path d="m6.8 10.2 2.2 2.2 4.2-4.6"/>',
    alert: '<circle cx="10" cy="10" r="7"/><path d="M10 6.4v4.2"/><path d="M10 13.4h.01"/>',
    info: '<circle cx="10" cy="10" r="7"/><path d="M10 9.4v4.2"/><path d="M10 6.6h.01"/>',
    x: '<path d="m5 5 10 10"/><path d="m15 5-10 10"/>',
    plus: '<path d="M10 4.4v11.2"/><path d="M4.4 10h11.2"/>',
    eye: '<path d="M1.8 10S4.8 4.6 10 4.6 18.2 10 18.2 10 15.2 15.4 10 15.4 1.8 10 1.8 10z"/><circle cx="10" cy="10" r="2.4"/>',
    eyeOff: '<path d="M8.2 4.9A6.9 6.9 0 0 1 10 4.6c5.2 0 8.2 5.4 8.2 5.4a13 13 0 0 1-2.3 3"/><path d="M11.7 11.7a2.4 2.4 0 0 1-3.4-3.4"/><path d="M4.4 6.3A13 13 0 0 0 1.8 10s3 5.4 8.2 5.4a7 7 0 0 0 3-.65"/><path d="m3 3 14 14"/>',
    folderCog: '<path d="M2.6 5.4a1.2 1.2 0 0 1 1.2-1.2h3.2l1.5 1.9h6.7a1.2 1.2 0 0 1 1.2 1.2v3"/><path d="M2.6 5.4v9a1.2 1.2 0 0 0 1.2 1.2h5"/><circle cx="14.4" cy="13.6" r="2"/>',
    send: '<path d="M17.4 2.6 9 11"/><path d="M17.4 2.6 12.1 17.4 9 11 2.6 7.9z"/>',
    bell: '<path d="M14.6 8.4a4.6 4.6 0 1 0-9.2 0c0 5-2 6.4-2 6.4h13.2s-2-1.4-2-6.4"/><path d="M11.4 17a1.6 1.6 0 0 1-2.8 0"/>',
    link: '<path d="M8.4 11.6a3 3 0 0 0 4.5.32l2.1-2.1a3 3 0 0 0-4.24-4.24l-1.2 1.2"/><path d="M11.6 8.4a3 3 0 0 0-4.5-.32L5 10.2a3 3 0 0 0 4.24 4.24l1.2-1.2"/>',
    copy: '<rect x="7" y="7" width="9.4" height="9.4" rx="1.4"/><path d="M13 7V5a1.4 1.4 0 0 0-1.4-1.4H5A1.4 1.4 0 0 0 3.6 5v6.6A1.4 1.4 0 0 0 5 13h2"/>',
    database: '<ellipse cx="10" cy="5.2" rx="6.4" ry="2.4"/><path d="M3.6 5.2v9.6c0 1.32 2.87 2.4 6.4 2.4s6.4-1.08 6.4-2.4V5.2"/><path d="M3.6 10c0 1.32 2.87 2.4 6.4 2.4s6.4-1.08 6.4-2.4"/>',
    layers: '<path d="m10 2.8 7 3.6-7 3.6-7-3.6z"/><path d="m3 10.4 7 3.6 7-3.6"/><path d="m3 14 7 3.6 7-3.6"/>',
    clock: '<circle cx="10" cy="10" r="7"/><path d="M10 5.8V10l2.8 1.6"/>',
    command: '<path d="M6.4 3.6a2.4 2.4 0 1 1-2.4 2.4v8a2.4 2.4 0 1 1 2.4-2.4h7.2a2.4 2.4 0 1 1 2.4 2.4V6a2.4 2.4 0 1 1-2.4 2.4z"/>',
    telegram: '<path d="M17.4 3.4 2.8 9.1l3.9 1.3 1.5 4.7 2.2-2.6 3.9 2.9z"/><path d="m6.7 10.4 8.3-5.4-6.8 6.8"/>',
    sun: '<circle cx="10" cy="10" r="3.4"/><path d="M10 1.8v1.8"/><path d="M10 16.4v1.8"/><path d="m4.2 4.2 1.3 1.3"/><path d="m14.5 14.5 1.3 1.3"/><path d="M1.8 10h1.8"/><path d="M16.4 10h1.8"/><path d="m4.2 15.8 1.3-1.3"/><path d="m14.5 5.5 1.3-1.3"/>',
    moon: '<path d="M16.6 11.4A6.9 6.9 0 0 1 8.6 3.4a6.9 6.9 0 1 0 8 8z"/>',
    monitor: '<rect x="2.6" y="3.6" width="14.8" height="9.6" rx="1.4"/><path d="M7 16.4h6"/><path d="M10 13.2v3.2"/>',
    dot: '<circle cx="10" cy="10" r="3.4"/>',
  };

  const d = $derived(PATHS[name] ?? PATHS.dot);
</script>

<svg
  class={klass}
  width={size}
  height={size}
  viewBox="0 0 20 20"
  fill="none"
  stroke="currentColor"
  stroke-width={stroke}
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
>
  {@html d}
</svg>
