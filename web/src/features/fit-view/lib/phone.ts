import { boundsOf, stepsOf, type Board, type Rect } from '@entities/board';
import type { Node } from '@xyflow/react';
import type { Insets } from '@shared/lib';

/** Cards read from this zoom up (description 11 px, title 12.75 px). Below it the page opens on a step, not on the whole board. */
export const READABLE = 0.85;

/**
 * A step may go below READABLE, down to here, when its width forces it: a card cut off at the edge reads worse than a
 * small one (D-2026-10-29-02, row a). Only the width does that; the open-whole decision keeps READABLE.
 */
export const STEP_FLOOR = 0.7;

/** Room kept around the board on a phone: the title above, the bottom bar below, a little at the sides. */
export const PHONE_PAD = { top: 64, bottom: 96, left: 16, right: 16 };

/** The title bar grows from 56 to 64 px when it carries the owner's name (D-2026-10-29-03, B): a fit keeps as much more clear of the top. */
export const OWNER_BAR_EXTRA = 8;

const WHOLE_MAX = 1.25, STEP_MAX = 1.3;

interface Size { width: number; height: number }
interface Pad { top: number; bottom: number; left: number; right: number }

/**
 * PHONE_PAD plus what a notch, a rounded corner or the home indicator takes from each edge, so a fitted board never sits
 * under the title or the bar there. `owned`: the title bar carries the owner's name and is taller.
 */
export const phonePad = (i: Insets, owned = false): Pad => ({ top: PHONE_PAD.top + (owned ? OWNER_BAR_EXTRA : 0) + i.top, bottom: PHONE_PAD.bottom + i.bottom, left: PHONE_PAD.left + i.left, right: PHONE_PAD.right + i.right });

/** The zoom that fits `box` into the view inside the padding, up to `max` (what fitView would pick). */
export function fitZoom(box: Rect, view: Size, pad: Pad = PHONE_PAD, max = WHOLE_MAX): number {
  const w = view.width - pad.left - pad.right, h = view.height - pad.top - pad.bottom;
  return Math.min(w / Math.max(box.width, 1), h / Math.max(box.height, 1), max);
}

/**
 * The box a phone frames for a step: what the step brings into view (`reveal`), else all that shows. On a board of lanes
 * only the cards count, never the band of a lane (D-2026-10-29-02, row e): a lane is as long as the whole board, so
 * framing one frames empty space and leaves its card off the screen.
 */
export function stepBox(nodes: Node[], reveal: ((id: string) => boolean) | null, shown: (id: string) => boolean): Rect | null {
  const lanes = nodes.some(n => n.type === 'zone' && (n.data as { lane?: boolean }).lane);
  const cards = new Set(nodes.filter(n => n.type === 'card').map(n => n.id));
  const only = (pick: (id: string) => boolean) => (id: string) => pick(id) && (!lanes || cards.has(id));
  return (reveal && boundsOf(nodes, only(reveal))) || boundsOf(nodes, only(shown));
}

/**
 * The zoom of a step on a phone: its whole width fits the screen (every card of the step is seen, none cut at the edge),
 * down to STEP_FLOOR. Its height gives way sooner, to READABLE: a tall step is read by panning down.
 */
export function stepZoom(box: Rect, view: Size, pad: Pad = PHONE_PAD): number {
  const wide = (view.width - pad.left - pad.right) / Math.max(box.width, 1), tall = (view.height - pad.top - pad.bottom) / Math.max(box.height, 1);
  return Math.max(Math.min(wide, Math.max(tall, READABLE), STEP_MAX), STEP_FLOOR);
}

/**
 * The view for one step on a phone (see stepZoom). A step is read from the top left, under the title: it sits at the
 * top edge of the free area, and at the left one when it is wider than the screen even at STEP_FLOOR (what does not
 * fit is cut off at the far side); a step narrower than the screen is centred across.
 */
export function stepViewport(box: Rect, view: Size, pad: Pad = PHONE_PAD): { x: number; y: number; zoom: number } {
  const zoom = stepZoom(box, view, pad);
  const room = view.width - pad.left - pad.right - box.width * zoom;
  return { zoom, x: pad.left + Math.max(0, room / 2) - box.x * zoom, y: pad.top - box.y * zoom };
}

/**
 * Where a phone opens: step 1 (null = the whole board) - the first step when the whole board would need less than
 * the readability floor, else the whole board. A board with nothing to step through opens whole. A client's desktop
 * asks the same of its own floor (CLIENT_FLOOR).
 */
export function openStep(b: Board, nodes: Node[], view: Size, pad: Pad = PHONE_PAD, floor = READABLE): number | null {
  const whole = boundsOf(nodes);
  if (!whole || !stepsOf(b).length) return null;
  return fitZoom(whole, view, pad) >= floor ? null : 0;
}
