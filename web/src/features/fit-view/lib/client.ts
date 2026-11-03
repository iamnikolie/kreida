import type { Rect } from '@entities/board';

/**
 * A client's desktop screen, and a frame a board is embedded in, read a card's title at 16 px and its second line at
 * 13 px at least (D-2026-10-29-03, row g). The title is 15 px (Plex; Caveat is 19) and the second line 13 px at zoom 1,
 * so the floor is 16/15. The phone keeps its own (READABLE).
 */
export const CLIENT_FLOOR = 16 / 15;

/** A small board fills its frame: a client's view zooms in up to here (the editor stops at 1.25). */
export const CLIENT_MAX = 1.6;

/** Room kept around the board: more on top, where the title and the toolbar sit (the editor's FIT padding). */
export const DESK_PAD = { top: 130, bottom: 48, left: 48, right: 48 };

/** A step wider than the screen at the floor may go down to here before a card is cut at the edge (as the phone's STEP_FLOOR does). */
export const CLIENT_STEP_MIN = 0.85;

/** Air between what is laid over the canvas's top (the head, the legend) and the board. */
export const CHROME_GAP = 16;

interface Size { width: number; height: number }
interface Pad { top: number; bottom: number; left: number; right: number }

export interface View { x: number; y: number; zoom: number; /** whether the whole box is on screen at this zoom */ fits: boolean }

/**
 * The view of `box` for a client: it fills the free area, centred, up to CLIENT_MAX. A box that would need less than the
 * floor is not shrunk to it: it is shown at the floor, from its top left (an axis that has room is still centred), and
 * `fits` says it is cut. `floor` 0 shows any box whole, whatever its size. `give`: the width may go down to it before
 * the box is cut at the side (the height never goes below the floor), for a step, where a card cut at the edge reads as broken.
 */
export function clientView(box: Rect, view: Size, pad: Pad = DESK_PAD, floor = CLIENT_FLOOR, give = floor): View {
  const w = view.width - pad.left - pad.right, h = view.height - pad.top - pad.bottom;
  const wide = w / Math.max(box.width, 1), tall = h / Math.max(box.height, 1);
  const zoom = Math.min(Math.min(wide, tall) >= floor ? Math.min(wide, tall) : Math.max(Math.min(wide, Math.max(tall, floor)), give), CLIENT_MAX);
  const at = (free: number, size: number, start: number, edge: number) => edge + Math.max(0, (free - size * zoom) / 2) - start * zoom;
  return { zoom, fits: box.width * zoom <= w + 0.5 && box.height * zoom <= h + 0.5, x: at(w, box.width, box.x, pad.left), y: at(h, box.height, box.y, pad.top) };
}

/**
 * The view for a step: everything the step shows when that reads at the floor, else what the step brings into view
 * (`reveal`), as large as the floor allows and no wider than the screen (down to CLIENT_STEP_MIN), so the view follows a
 * story that has outgrown the screen. No `reveal`: the shown box likewise.
 */
export function clientStepView(shown: Rect, reveal: Rect | null, view: Size, pad: Pad = DESK_PAD, floor = CLIENT_FLOOR): View {
  const all = clientView(shown, view, pad, floor);
  return all.fits ? all : clientView(reveal ?? shown, view, pad, floor, Math.min(floor, CLIENT_STEP_MIN));
}

/** What a board is shaped to on a client's screen: the free area inside the padding, and the zoom it should read at there. */
export const clientFrame = (screen: Size, pad: Pad = DESK_PAD, floor = CLIENT_FLOOR) =>
  ({ width: Math.max(screen.width - pad.left - pad.right, 1), height: Math.max(screen.height - pad.top - pad.bottom, 1), zoom: floor });

/**
 * The padding of a client's view: DESK_PAD, with the top at what the head and the legend really take (`top`: where they end,
 * px from the top; 0 = not measured yet) and the bottom at what the steps bar takes (`foot`, 0 = not up), each with a little
 * air, so a board is never framed under them - in a frame 400 px high the head is a quarter of it.
 */
export const clientPad = (top = 0, foot = 0, pad: Pad = DESK_PAD): Pad => ({
  ...pad, top: top > 0 ? top + CHROME_GAP : pad.top, bottom: Math.max(pad.bottom, foot > 0 ? foot + CHROME_GAP : 0),
});
