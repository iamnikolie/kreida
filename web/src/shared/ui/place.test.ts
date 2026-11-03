import { describe, expect, it } from 'vitest';
import { placePopover } from './place';

const view = { width: 1440, height: 800 };
const card = { left: 300, top: 400, right: 480, bottom: 460 };
const term = { left: 380, top: 435, right: 410, bottom: 450 };

describe('placePopover', () => {
  it('below the card the term is in, centred on the term', () => {
    expect(placePopover(term, card, view)).toEqual({ x: 395, y: 468, up: false });
  });

  it('above the card when there is no room below', () => {
    const low = { ...card, top: 700, bottom: 760 };
    expect(placePopover({ ...term, top: 735, bottom: 750 }, low, view)).toEqual({ x: 395, y: 692, up: true });
  });

  it('never past the side edges', () => {
    expect(placePopover({ ...term, left: 2, right: 20 }, card, view).x).toBeGreaterThanOrEqual(148);
    expect(placePopover({ ...term, left: 1420, right: 1438 }, card, view).x).toBeLessThanOrEqual(1440 - 148);
  });
});
