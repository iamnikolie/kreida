// Where a glossary popover goes: under the card the term sits in, so it never covers that card (its title least of all),
// centred on the term; above the card only when the viewport has no room below. Kept off the side edges.

type Box = { left: number; top: number; right: number; bottom: number };

/** The popover's estimated size: at most 280 px wide, a few lines tall. */
const HALF_W = 140, H = 140, GAP = 8;

export function placePopover(term: Box, card: Box, view: { width: number; height: number }): { x: number; y: number; up: boolean } {
  const x = Math.min(Math.max((term.left + term.right) / 2, HALF_W + GAP), view.width - HALF_W - GAP);
  const below = card.bottom + GAP;
  return below + H <= view.height || card.top - GAP - H < 0 ? { x, y: below, up: false } : { x, y: card.top - GAP, up: true };
}
