/** Small formatting + helper utilities (no dependencies). */

const MIN = 60_000;
const HOUR = 60 * MIN;
const DAY = 24 * HOUR;

/** Relative time, e.g. "3m ago", "in 2d", "just now". Accepts RFC3339 string or Date. */
export function relTime(input: string | Date | null | undefined, now: number = Date.now()): string {
  if (!input) return '—';
  const t = input instanceof Date ? input.getTime() : Date.parse(input);
  if (!isFinite(t)) return '—';
  const diff = t - now;
  const abs = Math.abs(diff);
  const future = diff > 0;

  let value: string;
  if (abs < 45_000) return future ? 'in a moment' : 'just now';
  if (abs < HOUR) value = `${Math.round(abs / MIN)}m`;
  else if (abs < DAY) value = `${Math.round(abs / HOUR)}h`;
  else if (abs < 7 * DAY) value = `${Math.round(abs / DAY)}d`;
  else if (abs < 30 * DAY) value = `${Math.round(abs / (7 * DAY))}w`;
  else if (abs < 365 * DAY) value = `${Math.round(abs / (30 * DAY))}mo`;
  else value = `${Math.round(abs / (365 * DAY))}y`;

  return future ? `in ${value}` : `${value} ago`;
}

/** Compact countdown for deadlines: "2d 4h", "3h 12m", "overdue". */
export function countdown(input: string, now: number = Date.now()): string {
  const t = Date.parse(input);
  if (!isFinite(t)) return '';
  let diff = t - now;
  if (diff <= 0) return 'overdue';
  const d = Math.floor(diff / DAY);
  diff -= d * DAY;
  const h = Math.floor(diff / HOUR);
  diff -= h * HOUR;
  const m = Math.floor(diff / MIN);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

/** Human byte size. */
export function fmtBytes(bytes: number | null | undefined): string {
  if (bytes === null || bytes === undefined || !isFinite(bytes)) return '—';
  if (bytes < 1) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)));
  const v = bytes / Math.pow(1024, i);
  const dp = i === 0 ? 0 : v < 10 ? 1 : 0;
  return `${v.toFixed(dp)} ${units[i]}`;
}

/**
 * Trim float noise from a points value: 4.666666666666667 -> "4.67", 6 -> "6".
 * Canvas returns raw float scores, so rounding must happen at render time.
 */
export function fmtScore(n: number | null | undefined): string {
  if (n === null || n === undefined || !isFinite(n)) return '—';
  const rounded = Math.round(n * 100) / 100;
  return Number.isInteger(rounded) ? String(rounded) : rounded.toFixed(2).replace(/0$/, '');
}

/**
 * Render an FTS5 snippet (which marks matches with literal `<b>`…`</b>`) as
 * escaped HTML in which only those markers stay live. Everything else in the
 * snippet is extracted PDF text and must not be interpreted as markup.
 */
export function snippetHTML(snippet: string): string {
  const escaped = snippet
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
  return escaped.replace(/&lt;b&gt;/g, '<mark>').replace(/&lt;\/b&gt;/g, '</mark>');
}

/** Absolute datetime, e.g. "Mon 12 May, 23:59". */
export function fmtDateTime(input: string | null | undefined): string {
  if (!input) return '—';
  const d = new Date(input);
  if (isNaN(d.getTime())) return '—';
  return d.toLocaleString(undefined, {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export function fmtDate(input: string | null | undefined): string {
  if (!input) return '—';
  const d = new Date(input);
  if (isNaN(d.getTime())) return '—';
  return d.toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' });
}

/** Urgency bucket used for deadline chips. */
export type Urgency = 'done' | 'overdue' | 'today' | 'soon' | 'later';

export function urgency(dueAt: string, submitted: boolean, now = Date.now()): Urgency {
  if (submitted) return 'done';
  const t = Date.parse(dueAt);
  if (!isFinite(t)) return 'later';
  const diff = t - now;
  if (diff < 0) return 'overdue';
  if (diff <= DAY) return 'today';
  if (diff <= 3 * DAY) return 'soon';
  return 'later';
}

export type DeadlineGroup = 'Overdue' | 'Today' | 'Tomorrow' | 'This week' | 'Later';

export function deadlineGroup(dueAt: string, now = Date.now()): DeadlineGroup {
  const t = Date.parse(dueAt);
  if (!isFinite(t)) return 'Later';
  if (t < now) return 'Overdue';
  const start = new Date(now);
  start.setHours(0, 0, 0, 0);
  const dayIndex = Math.floor((t - start.getTime()) / DAY);
  if (dayIndex <= 0) return 'Today';
  if (dayIndex === 1) return 'Tomorrow';
  if (dayIndex <= 7) return 'This week';
  return 'Later';
}

export const DEADLINE_GROUP_ORDER: DeadlineGroup[] = ['Overdue', 'Today', 'Tomorrow', 'This week', 'Later'];

/** File extension without the dot, lowercased. */
export function ext(name: string): string {
  const i = name.lastIndexOf('.');
  return i > 0 ? name.slice(i + 1).toLowerCase() : '';
}

/** Coarse file kind used to pick an icon/colour. */
export type FileKind = 'pdf' | 'doc' | 'slides' | 'sheet' | 'image' | 'video' | 'archive' | 'code' | 'file';

export function fileKind(name: string): FileKind {
  switch (ext(name)) {
    case 'pdf':
      return 'pdf';
    case 'doc':
    case 'docx':
    case 'rtf':
    case 'txt':
    case 'md':
      return 'doc';
    case 'ppt':
    case 'pptx':
    case 'key':
      return 'slides';
    case 'xls':
    case 'xlsx':
    case 'csv':
      return 'sheet';
    case 'png':
    case 'jpg':
    case 'jpeg':
    case 'gif':
    case 'svg':
    case 'webp':
      return 'image';
    case 'mp4':
    case 'mov':
    case 'mkv':
    case 'avi':
      return 'video';
    case 'zip':
    case 'rar':
    case '7z':
    case 'tar':
    case 'gz':
      return 'archive';
    case 'py':
    case 'ts':
    case 'js':
    case 'go':
    case 'java':
    case 'c':
    case 'cpp':
    case 'ipynb':
    case 'm':
    case 'r':
      return 'code';
    default:
      return 'file';
  }
}

/**
 * Subsequence fuzzy match. Returns a score (higher = better) or -1 for no match.
 * Rewards consecutive runs, matches at word boundaries, and short haystacks.
 */
export function fuzzyScore(needle: string, haystack: string): number {
  if (!needle) return 0;
  const n = needle.toLowerCase();
  const h = haystack.toLowerCase();
  let score = 0;
  let hi = 0;
  let run = 0;
  for (let i = 0; i < n.length; i++) {
    const c = n[i];
    if (c === ' ') continue;
    const found = h.indexOf(c, hi);
    if (found === -1) return -1;
    if (found === hi && hi > 0) {
      run += 1;
      score += 6 + run * 2;
    } else {
      run = 0;
      score += 1;
    }
    const prev = found > 0 ? h[found - 1] : '';
    if (found === 0 || prev === ' ' || prev === '/' || prev === '\\' || prev === '_' || prev === '-' || prev === '.') {
      score += 8;
    }
    hi = found + 1;
  }
  // prefer shorter haystacks
  score -= Math.min(20, Math.floor(h.length / 12));
  return score;
}

/** Debounce helper. */
export function debounce<T extends (...args: any[]) => void>(fn: T, ms: number) {
  let handle: ReturnType<typeof setTimeout> | undefined;
  const wrapped = (...args: Parameters<T>) => {
    if (handle) clearTimeout(handle);
    handle = setTimeout(() => fn(...args), ms);
  };
  wrapped.cancel = () => {
    if (handle) clearTimeout(handle);
    handle = undefined;
  };
  return wrapped;
}

/**
 * Sanitize Canvas announcement HTML: drops script/style/iframe/object, all
 * `on*` handler attributes, and javascript:/data: URLs. Returns safe HTML.
 */
export function sanitizeHTML(html: string): string {
  if (!html) return '';
  const doc = new DOMParser().parseFromString(html, 'text/html');
  const BANNED = new Set(['SCRIPT', 'STYLE', 'IFRAME', 'OBJECT', 'EMBED', 'LINK', 'META', 'BASE', 'FORM', 'INPUT', 'BUTTON', 'SVG']);
  const walk = (node: Element) => {
    for (const child of Array.from(node.children)) {
      if (BANNED.has(child.tagName)) {
        child.remove();
        continue;
      }
      for (const attr of Array.from(child.attributes)) {
        const name = attr.name.toLowerCase();
        const value = (attr.value || '').trim();
        if (name.startsWith('on')) {
          child.removeAttribute(attr.name);
          continue;
        }
        if (name === 'style') {
          child.removeAttribute(attr.name);
          continue;
        }
        if ((name === 'href' || name === 'src' || name === 'xlink:href') && /^\s*(javascript|data|vbscript):/i.test(value)) {
          child.removeAttribute(attr.name);
        }
      }
      if (child.tagName === 'A') {
        child.setAttribute('rel', 'noopener noreferrer');
        child.setAttribute('target', '_blank');
      }
      walk(child);
    }
  };
  walk(doc.body);
  return doc.body.innerHTML;
}

/** localStorage JSON helpers that never throw. */
export function lsGet<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key);
    if (raw === null) return fallback;
    return JSON.parse(raw) as T;
  } catch {
    return fallback;
  }
}

export function lsSet(key: string, value: unknown): void {
  try {
    localStorage.setItem(key, JSON.stringify(value));
  } catch {
    /* ignore */
  }
}

/** Parse a Go duration ("72h", "30m") to a readable label ("3d", "30m"). */
export function durLabel(d: string): string {
  const m = /^(\d+(?:\.\d+)?)(h|m|s)$/.exec(d.trim());
  if (!m) return d;
  const n = parseFloat(m[1]);
  if (m[2] === 'h' && n >= 24 && n % 24 === 0) return `${n / 24}d`;
  return `${n}${m[2]}`;
}

/** Inverse of durLabel: "3d" -> "72h", "1h" -> "1h". Returns null if unparseable. */
export function parseDurLabel(s: string): string | null {
  const m = /^(\d+(?:\.\d+)?)\s*(d|h|m|s)$/i.exec(s.trim());
  if (!m) return null;
  const n = parseFloat(m[1]);
  const unit = m[2].toLowerCase();
  if (unit === 'd') return `${n * 24}h`;
  return `${n}${unit}`;
}
