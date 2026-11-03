import { describe, expect, it } from 'vitest';
import type { Node } from '@xyflow/react';
import type { Board } from '@entities/board';
import { fitZoom, openStep, OWNER_BAR_EXTRA, PHONE_PAD, phonePad, READABLE, STEP_FLOOR, stepBox, stepViewport, stepZoom } from './phone';

const phone = { width: 390, height: 844 };
// 390 - 32 = 358 wide, 844 - 64 - 96 = 684 high inside the padding.
const rect = (width: number, height: number, x = 0, y = 0) => ({ x, y, width, height });
const node = (id: string, w: number, h: number, y = 0): Node => ({ id, position: { x: 0, y }, style: { width: w, height: h }, data: {} });
const board = (o: Partial<Board>) => ({ groups: [], nodes: [], steps: [], ...o }) as unknown as Board;

describe('fitZoom', () => {
  it('fits both sides inside the padding', () => {
    expect(fitZoom(rect(716, 100), phone)).toBeCloseTo(0.5);
    expect(fitZoom(rect(100, 1368), phone)).toBeCloseTo(0.5);
  });
  it('a small board stops at the max zoom', () => {
    expect(fitZoom(rect(50, 50), phone)).toBe(1.25);
  });
});

describe('stepViewport', () => {
  it('a step narrower than the screen is centred across and sits at the top of the free area', () => {
    const v = stepViewport(rect(300, 200, 100, 200), phone);
    expect(v.zoom).toBeCloseTo(358 / 300);
    expect(v.x).toBeCloseTo(PHONE_PAD.left - 100 * v.zoom);
    expect(v.y).toBeCloseTo(PHONE_PAD.top - 200 * v.zoom);
    const narrow = stepViewport(rect(200, 200, 100, 200), phone);
    expect(narrow.x).toBeCloseTo(PHONE_PAD.left + (358 - 200 * narrow.zoom) / 2 - 100 * narrow.zoom);
  });
  it('a step wider than the screen at the readability floor zooms out to fit its width, so no card is cut', () => {
    // 358 wide inside the padding: 447 wide needs 0.80, 460 needs 0.778
    const v = stepViewport(rect(447, 300, 20, 0), phone);
    expect(v.zoom).toBeCloseTo(358 / 447);
    expect(v.zoom).toBeLessThan(READABLE);
    expect(20 * v.zoom + v.x).toBeCloseTo(PHONE_PAD.left); // its left edge
    expect((20 + 447) * v.zoom + v.x).toBeCloseTo(390 - PHONE_PAD.right); // and its right edge are both on screen
  });
  it('only a step too wide even then is cut, at the step floor, which the readability floor does not change', () => {
    expect(STEP_FLOOR).toBeLessThan(READABLE);
    expect(stepViewport(rect(1400, 300), phone).zoom).toBe(STEP_FLOOR);
    expect(stepViewport(rect(358 / STEP_FLOOR + 1, 300), phone).zoom).toBe(STEP_FLOOR);
  });
  it('a tall step gives its height way to the readability floor, not to the floor of the width', () => {
    expect(stepViewport(rect(300, 3000), phone).zoom).toBe(READABLE);
    expect(stepZoom(rect(300, 3000), phone)).toBe(READABLE);
    expect(stepZoom(rect(400, 3000), phone)).toBe(READABLE); // the width would allow 0.895; a tall step reads at 0.85
    expect(stepZoom(rect(440, 3000), phone)).toBeCloseTo(358 / 440); // but 0.81 is what fits the width, and the width wins
  });
  it('what does not fit is pinned to the top left, not centred', () => {
    const v = stepViewport(rect(1400, 3000, 50, 70), phone);
    expect(v.x).toBeCloseTo(PHONE_PAD.left - 50 * STEP_FLOOR);
    expect(v.y).toBeCloseTo(PHONE_PAD.top - 70 * STEP_FLOOR);
  });
  it('a wide, short step is pinned left and top', () => {
    const v = stepViewport(rect(1400, 100), phone);
    expect(v.x).toBeCloseTo(PHONE_PAD.left);
    expect(v.y).toBeCloseTo(PHONE_PAD.top);
  });
});

describe('stepBox', () => {
  const card = (id: string, x: number, y: number, parentId?: string): Node => ({ id, type: 'card', parentId, position: { x, y }, style: { width: 168, height: 60 }, data: {} });
  const zone = (id: string, y: number, lane: boolean): Node => ({ id, type: 'zone', position: { x: 0, y }, style: { width: 900, height: 120 }, data: { lane } });

  it('a flow frames what the step brings, group included, else all that shows', () => {
    const nodes = [zone('g', 0, false), card('a', 20, 40, 'g'), card('b', 20, 300)];
    expect(stepBox(nodes, id => id === 'g' || id === 'a', () => true)).toEqual({ x: 0, y: 0, width: 900, height: 120 });
    expect(stepBox(nodes, null, id => id === 'b')).toEqual({ x: 20, y: 300, width: 168, height: 60 });
    expect(stepBox(nodes, () => false, id => id === 'b')).toEqual({ x: 20, y: 300, width: 168, height: 60 });
  });

  it('lanes: the cards only, never the empty lane around them', () => {
    const nodes = [zone('l1', 0, true), zone('l2', 134, true), card('a', 16, 26, 'l1'), card('b', 400, 160, 'l2')];
    // the step shows lane l2 and its card: the band is 900 wide from x = 0, the card is one 168 wide at x = 400
    expect(stepBox(nodes, id => id === 'l2' || id === 'b', () => true)).toEqual({ x: 400, y: 294, width: 168, height: 60 });
  });

  it('nothing to frame is null', () => {
    expect(stepBox([zone('l1', 0, true)], id => id === 'l1', () => true)).toBeNull();
    expect(stepBox([], null, () => true)).toBeNull();
  });
});

describe('phonePad', () => {
  it('is PHONE_PAD without insets, and adds each inset to its own side', () => {
    expect(phonePad({ top: 0, right: 0, bottom: 0, left: 0 })).toEqual(PHONE_PAD);
    expect(phonePad({ top: 47, right: 3, bottom: 34, left: 5 })).toEqual({ top: 64 + 47, bottom: 96 + 34, left: 16 + 5, right: 16 + 3 });
  });
  it('the title bar with the owner\'s name is 64 px, not 56: the fit keeps 8 px more clear of the top, nothing else moves', () => {
    const none = { top: 0, right: 0, bottom: 0, left: 0 };
    expect(OWNER_BAR_EXTRA).toBe(64 - 56);
    expect(phonePad(none, true)).toEqual({ ...PHONE_PAD, top: 72 });
    expect(phonePad({ top: 47, right: 3, bottom: 34, left: 5 }, true)).toEqual({ top: 72 + 47, bottom: 96 + 34, left: 16 + 5, right: 16 + 3 });
    expect(phonePad(none, false)).toEqual(PHONE_PAD);
    // a step sits under the taller bar
    expect(stepViewport(rect(300, 200, 100, 200), phone, phonePad(none, true)).y).toBeCloseTo(72 - 200 * (358 / 300));
  });
  it('a notch and the home indicator shrink the room a board is fitted into, and move a step away from them', () => {
    const pad = phonePad({ top: 47, right: 0, bottom: 34, left: 0 });
    expect(fitZoom(rect(100, 1368), phone, pad)).toBeCloseTo((844 - 111 - 130) / 1368);
    expect(stepViewport(rect(300, 200, 100, 200), phone, pad).y).toBeCloseTo(64 + 47 - 200 * (358 / 300));
  });
  it('a board that read whole opens on step 1 once the insets take its room', () => {
    const steps = [{ title: '', show: ['a'], hide: [] }];
    // 684 high inside the padding: 800 needs 0.855; the insets leave 603, which needs 0.754
    expect(openStep(board({ steps }), [node('a', 236, 800)], phone)).toBeNull();
    expect(openStep(board({ steps }), [node('a', 236, 800)], phone, phonePad({ top: 47, right: 0, bottom: 34, left: 0 }))).toBe(0);
  });
});

describe('openStep', () => {
  const steps = [{ title: '', show: ['a'], hide: [] }];
  it('a board that reads whole at the floor opens whole', () => {
    // 358 / 0.85 = 421 wide fits exactly
    expect(openStep(board({ steps }), [node('a', 421, 500)], phone)).toBeNull();
    expect(openStep(board({ steps }), [node('a', 236, 300)], phone)).toBeNull();
  });
  it('a board that needs less opens on step 1', () => {
    expect(openStep(board({ steps }), [node('a', 423, 500)], phone)).toBe(0);
    expect(openStep(board({ steps }), [node('a', 236, 200), node('b', 236, 200, 1000)], phone)).toBe(0);
  });
  it('without declared steps the top-level items are the steps, as in presentation', () => {
    const b = board({ nodes: [{ id: 'a', group: null }, { id: 'b', group: null }] as Board['nodes'] });
    expect(openStep(b, [node('a', 236, 200), node('b', 236, 200, 1000)], phone)).toBe(0);
  });
  it('nothing to step through: opens whole whatever the zoom', () => {
    expect(openStep(board({}), [node('a', 2000, 2000)], phone)).toBeNull();
    expect(openStep(board({ steps }), [], phone)).toBeNull();
  });
  it('a shorter phone needs the steps sooner', () => {
    const n = [node('a', 236, 640)];
    expect(openStep(board({ steps }), n, { width: 390, height: 844 })).toBeNull();
    expect(openStep(board({ steps }), n, { width: 375, height: 667 })).toBe(0);
  });
});
