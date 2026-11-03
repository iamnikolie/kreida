import { describe, expect, it } from 'vitest';
import { CHROME_GAP, CLIENT_FLOOR, CLIENT_MAX, CLIENT_STEP_MIN, clientPad, clientStepView, clientView, DESK_PAD } from './client';

const rect = (width: number, height: number, x = 0, y = 0) => ({ x, y, width, height });
// 1440 x 900 less the padding (48 + 48, 130 + 48): 1344 x 722 free
const screen = { width: 1440, height: 900 };

describe('the reading floor', () => {
  it('a title is 16 px or more and a second line 13 px or more at the floor', () => {
    expect(15 * CLIENT_FLOOR).toBeGreaterThanOrEqual(16);
    expect(13 * CLIENT_FLOOR).toBeGreaterThanOrEqual(13);
  });
});

describe('clientView', () => {
  it('a board that fits fills the free area, centred', () => {
    const v = clientView(rect(1000, 400), screen);
    expect(v.zoom).toBeCloseTo(1344 / 1000); // 1.34 wide, 1.8 tall: the width decides
    expect(v.fits).toBe(true);
    expect(v.x).toBeCloseTo(DESK_PAD.left);
    expect(v.y).toBeCloseTo(DESK_PAD.top + (722 - 400 * v.zoom) / 2);
  });

  it('a small board stops at the max zoom instead of filling the screen with one card', () => {
    expect(clientView(rect(236, 75), screen).zoom).toBe(CLIENT_MAX);
  });

  it('a board that needs less than the floor is shown at the floor from its top left, cut', () => {
    const v = clientView(rect(2000, 300, 20, 10), screen);
    expect(v.zoom).toBeCloseTo(CLIENT_FLOOR);
    expect(v.fits).toBe(false);
    expect(v.x).toBeCloseTo(DESK_PAD.left - 20 * CLIENT_FLOOR);
    expect(v.y).toBeCloseTo(DESK_PAD.top + (722 - 300 * CLIENT_FLOOR) / 2 - 10 * CLIENT_FLOOR); // the axis that has room is centred
  });

  it('floor 0: whatever its size, the whole board', () => {
    const v = clientView(rect(6000, 300), screen, DESK_PAD, 0);
    expect(v.zoom).toBeCloseTo(1344 / 6000);
    expect(v.fits).toBe(true);
  });

  it('give: the width goes down before a card is cut, the height does not', () => {
    // 1344 wide: 1400 needs 0.96 - under the floor 1.067, over the give 0.85
    const wide = clientView(rect(1400, 100), screen, DESK_PAD, CLIENT_FLOOR, CLIENT_STEP_MIN);
    expect(wide.zoom).toBeCloseTo(1344 / 1400);
    expect(wide.fits).toBe(true);
    // wider than even the give: cut at the give
    expect(clientView(rect(2400, 100), screen, DESK_PAD, CLIENT_FLOOR, CLIENT_STEP_MIN).zoom).toBeCloseTo(CLIENT_STEP_MIN);
    // tall: 722 high, 900 needs 0.80, but the height only gives way to the floor
    expect(clientView(rect(300, 900), screen, DESK_PAD, CLIENT_FLOOR, CLIENT_STEP_MIN).zoom).toBeCloseTo(CLIENT_FLOOR);
  });
});

describe('clientStepView', () => {
  it('what the step shows, when that reads at the floor', () => {
    const shown = rect(900, 300), v = clientStepView(shown, rect(236, 75, 664, 0), screen);
    expect(v).toEqual(clientView(shown, screen));
  });

  it('what the step brings in, when everything shown needs less than the floor', () => {
    const reveal = rect(236, 75, 1800, 0), v = clientStepView(rect(2036, 400), reveal, screen);
    expect(v).toEqual(clientView(reveal, screen, DESK_PAD, CLIENT_FLOOR, CLIENT_STEP_MIN));
    expect(v.zoom).toBe(CLIENT_MAX);
  });

  it('with nothing brought in, the shown box, its width kept whole', () => {
    const v = clientStepView(rect(1400, 200), null, screen);
    expect(v.zoom).toBeCloseTo(1344 / 1400);
    expect(v.fits).toBe(true);
  });
});

describe('clientPad', () => {
  it('not measured yet: the usual padding', () => {
    expect(clientPad()).toEqual(DESK_PAD);
    expect(clientPad(0, 0)).toEqual(DESK_PAD);
  });

  it('measured: the top is where the head ends and the bottom is what the steps bar takes, each with some air', () => {
    expect(clientPad(150)).toEqual({ ...DESK_PAD, top: 150 + CHROME_GAP });
    expect(clientPad(95).top).toBe(95 + CHROME_GAP);
    expect(clientPad(0, 62)).toEqual({ ...DESK_PAD, bottom: 62 + CHROME_GAP });
  });

  it('a bar lower than the usual padding never makes room smaller than it was', () => {
    expect(clientPad(0, 20).bottom).toBe(DESK_PAD.bottom);
  });

  it('a board is framed between a head that takes a third of a 636 x 397 frame and the bar', () => {
    const frame = { width: 636, height: 397 }, pad = clientPad(150, 62), shown = rect(236, 75, 112, 184);
    const v = clientView(shown, frame, pad);
    expect(v.y + 184 * v.zoom).toBeGreaterThanOrEqual(150 + CHROME_GAP); // its top edge is under the head
    expect(v.y + (184 + 75) * v.zoom).toBeLessThanOrEqual(397 - 62 - CHROME_GAP + 0.5); // and its foot above the bar
  });
});
