// Card geometry. Layout runs before the DOM exists, so sizes are estimated from the text, generously.
import { Position } from '@xyflow/react';
import type { BNode, Dir } from '@shared/api';
import { hasIcon } from '@shared/lib';
import { gridGeom } from './grid';
import { noteHeight, noteWidth } from './note';

// Table cards (nodes with fields): fixed geometry, so FK edges can attach to a row exactly.
// Row height must match `.card.table .tr` in card.css.
export const ROW = 24;
export const BORDER = 1.5;

const tableWidth = (n: BNode) => n.width ?? Math.min(360, Math.max(220,
  n.title.length * 8.9 + 40 + (hasIcon(n) ? 28 : 0),
  ...n.fields!.map(f => (f.name.length + (f.ref ? f.ref.length + 3 : 0) + (f.note ? f.note.length + 2 : 0)) * 7.4 + 32)));

export function headH(n: BNode) {
  const dl = n.text ? Math.ceil((n.text.length * 7.3) / (tableWidth(n) - 28 - (hasIcon(n) ? 28 : 0))) : 0;
  return 20 + 21 + (dl ? 4 + dl * 18 : 0);
}

/** Table rows sit on the in/out sides of the flow; TB/BT boards still put them left/right. */
export const rowSides = (dir: Dir): [Position, Position] =>
  (dir === 'RL' ? [Position.Right, Position.Left] : [Position.Left, Position.Right]);

/** On a phone a card is at most this wide, and its title wraps to at most two lines (D-2026-10-29-02, row a). */
export const PHONE_CARD_MAX = 168;
/** A table or a note on a phone: wider than a card, so its columns and prose still read. */
export const PHONE_TABLE_MAX = 320;
/** The lane caption above a card's title on a phone: its 14 px line and the 3 px gap (`.card .cap`). */
const CAPTION = 17;

export interface SizeOpts {
  /** A phone held upright: cards are narrower. */
  phone?: boolean;
  /** The card carries its lane's name above the title. */
  caption?: boolean;
}

/**
 * The height a card's live chip takes: its 7 px gap and 19 px line (a little more than the chip itself), 30 px in all. On a
 * phone the chip may not fit its card's 168 px on one line: its dot, label and value then flow onto more lines (the CSS lets
 * them), so a live value is never cut short. The label's width is known here, the value's is not: a short one is assumed.
 */
function chipHeight(n: BNode, inner: number, phone: boolean): number {
  if (!n.live) return 0;
  if (!phone) return 30;
  const room = inner - 18, label = n.live.label ? n.live.label.length * 6.6 + 6 : 0; // 18: the chip's own padding
  return 11 + 19 * Math.ceil((13 + label + 34) / room); // the dot and its gap, the label and its gap, a short value
}

export function size(n: BNode, { phone = false, caption = false }: SizeOpts = {}) {
  if (n.fields) return { width: tableWidth(n), height: Math.ceil(2 * BORDER + headH(n) + n.fields.length * ROW + 6) };
  if (n.rows) {
    const g = gridGeom(n, phone ? { view: PHONE_TABLE_MAX } : {});
    return { width: g.width, height: g.height };
  }
  if (n.kind === 'note' && n.text) {
    const width = Math.min(noteWidth(n), phone ? PHONE_TABLE_MAX : Infinity);
    return { width, height: noteHeight(n, width) + (caption ? CAPTION : 0) };
  }
  const most = phone ? PHONE_CARD_MAX : Infinity, lines = (n: number) => (phone ? Math.min(n, 2) : n), extra = caption ? CAPTION : 0;
  if (n.kind === 'who' || n.shape === 'pill') {
    const w = Math.max(110, n.title.length * 9.5 + 60, n.text.length * 7.6 + 60) + (hasIcon(n) ? 26 : 0);
    const width = Math.min(n.width ?? w, most);
    const tl = lines(Math.max(1, Math.ceil((n.title.length * 9.5) / (width - 40 - (hasIcon(n) ? 26 : 0)))));
    const chip = phone ? chipHeight(n, width - 40 - (hasIcon(n) ? 26 : 0), true) : 0; // a pill's chip counts on a phone, where a value must not drop
    return { width, height: (n.text ? 66 : 48) + (tl - 1) * 21 + chip + extra };
  }
  const width = Math.min(n.width ?? (n.kind === 'note' ? 210 : 236), most);
  const inner = width - 36 - (hasIcon(n) ? 28 : 0);
  const tl = lines(Math.max(1, Math.ceil((n.title.length * 8.9) / inner)));
  // on a phone the icon takes its column from the text too, and a column this narrow wastes a share of each line to word breaks
  const dl = n.text ? Math.ceil((n.text.length * 7.3) / ((width - 36 - (phone && hasIcon(n) ? 28 : 0)) * (phone ? 0.85 : 1))) : 0;
  return { width, height: Math.max(58, 30 + tl * 21 + (dl ? 6 + dl * 18 : 0) + chipHeight(n, inner, phone) + extra) };
}
