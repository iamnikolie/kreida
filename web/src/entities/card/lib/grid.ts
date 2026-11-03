// A table card (`cols:` / `rows:`): a comparison or a scored decision. Layout runs before the DOM exists, so the column
// widths and row heights are decided here and the card draws exactly these (Grid.tsx sets them inline).
import type { BNode } from '@shared/api';
import { hasIcon } from '@shared/lib';

// widths per character: a cell, a row's first (bold) cell, a header (caps, spaced)
const CH = 7.9, CH_BOLD = 8.5, CH_HEAD = 9, LINE = 18, CELL_PAD = 22, ROW_PAD = 12, MIN_COL = 64, MAX_COL = 240, MAX_W = 760;

export interface GridGeom {
  /** Column widths. */
  cols: number[];
  /** Heights: the title head, the header row (0 without cols), each row, the padding under the last. */
  head: number; header: number; rows: number[]; pad: number;
  width: number; height: number;
  /** The table's own width: the card's on a desktop, wider than the card on a phone, where it scrolls sideways inside it. */
  inner: number;
}

export interface GridOpts {
  /** A phone: the most the card may take (PHONE_TABLE_MAX). A wider table keeps its columns and scrolls. */
  view?: number;
}

/** On a phone the first column, held in place while the rest scrolls, takes at most this share of the card. */
const STICKY = 0.4;

const longestWord = (s: string) => Math.max(0, ...s.split(/\s+/).map(w => w.length));
/**
 * Lines a cell takes at width w: words never break, so a line holds whole words. Half a px of slack: a column is often
 * exactly as wide as its longest cell, and float sums or a measured width rounded to whole px must not wrap that cell.
 */
function lines(s: string, w: number, ch: number): number {
  const room = w - CELL_PAD + 0.5;
  let n = 1, at = 0;
  for (const word of s.split(/\s+/).filter(Boolean)) {
    const len = word.length * ch;
    if (at && at + ch + len > room) { n++; at = len; } else at += (at ? ch : 0) + len;
  }
  return n;
}

export function gridGeom(n: BNode, { view }: GridOpts = {}): GridGeom {
  const rows = n.rows ?? [], cols = n.cols ?? [];
  const k = Math.max(cols.length, ...rows.map(r => r.length), 1);
  const ch = (i: number) => (i === 0 ? CH_BOLD : CH);
  const cells = (i: number) => rows.map(r => r[i]?.text ?? '');
  // a column fits its text on one line up to MAX_COL, and never gets narrower than its longest word; long text wraps
  const floor = Array.from({ length: k }, (_, i) => Math.max(longestWord(cols[i] ?? '') * CH_HEAD + CELL_PAD, ...cells(i).map(s => longestWord(s) * ch(i) + CELL_PAD)));
  let w = Array.from({ length: k }, (_, i) => Math.max(
    Math.min(MAX_COL, Math.max(MIN_COL, (cols[i] ?? '').length * CH_HEAD + CELL_PAD, ...cells(i).map(s => s.length * ch(i) + CELL_PAD))), floor[i]));
  const icon = hasIcon(n) ? 28 : 0, titleW = n.title.length * 8.9 + 36 + icon;
  let total = w.reduce((a, b) => a + b, 0);
  // a phone ignores the desktop's width: a table wider than the card narrows its row names to wrap, and scrolls the rest
  if (view && Math.max(total, titleW, 200) > view) {
    w[0] = Math.max(floor[0], Math.min(w[0], view * STICKY));
    total = w.reduce((a, b) => a + b, 0);
  }
  // whole px: React Flow hands the card its measured width, rounded, and the drawn rows must be the ones laid out
  const width = Math.ceil(view
    ? Math.min(view, Math.max(total, titleW, 200))
    : n.width ?? Math.min(MAX_W, Math.max(total, titleW, 200)));
  if (!view || total < width) { w = w.map(x => (x * width) / total); total = width; } // shared out to the card's width
  const dl = n.text ? Math.ceil((n.text.length * CH) / (width - 36 - icon)) : 0;
  const head = 20 + 21 + (dl ? 4 + dl * LINE : 0);
  const rowH = (texts: string[], head: boolean) => ROW_PAD + LINE * Math.max(...texts.map((s, i) => lines(s, w[i], head ? CH_HEAD : ch(i))));
  const header = cols.length ? rowH(cols, true) : 0;
  const rh = rows.map(r => rowH(r.map(c => c.text), false));
  const pad = 4;
  return { cols: w, head, header, rows: rh, pad, width, height: Math.ceil(head + header + rh.reduce((a, b) => a + b, 0) + pad), inner: total };
}

/**
 * A scored cell's tone, 0 (worst of the table) to 4 (best); null without a score. With one distinct score its sign
 * decides: above zero is good, below bad.
 */
export function toneOf(score: number | undefined, all: number[]): number | null {
  if (score == null) return null;
  const lo = Math.min(...all), hi = Math.max(...all);
  if (hi === lo) return score > 0 ? 4 : score < 0 ? 0 : 2;
  return Math.round(((score - lo) / (hi - lo)) * 4);
}
