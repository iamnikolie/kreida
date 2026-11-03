import { describe, expect, it } from 'vitest';
import { canPlay, endsPlay, PLAY_MS, playNext } from './play';
import { nextStep } from './nav';

const base = { mode: 'share' as const, query: new URLSearchParams('play=1'), capture: false, reducedMotion: false, stepped: true };

describe('canPlay', () => {
  it('?play=1 on a share link with steps', () => {
    expect(canPlay(base)).toBe(true);
  });
  it('needs ?play', () => {
    expect(canPlay({ ...base, query: new URLSearchParams('present=1') })).toBe(false);
    expect(canPlay({ ...base, query: new URLSearchParams('play=0') })).toBe(false);
    expect(canPlay({ ...base, query: new URLSearchParams('theme=dark&play') })).toBe(true);
  });
  it('share mode only: not the editor, not an exported file', () => {
    expect(canPlay({ ...base, mode: 'editor' })).toBe(false);
    expect(canPlay({ ...base, mode: 'static' })).toBe(false);
  });
  it('never in a headless capture', () => {
    expect(canPlay({ ...base, capture: true })).toBe(false);
  });
  it('never under prefers-reduced-motion: the whole board, still', () => {
    expect(canPlay({ ...base, reducedMotion: true })).toBe(false);
  });
  it('a board with no steps has nothing to play', () => {
    expect(canPlay({ ...base, stepped: false })).toBe(false);
  });
});

describe('endsPlay', () => {
  it('a mouse that hovers or moves ends it', () => {
    expect(endsPlay('pointerenter', 'mouse')).toBe(true);
    expect(endsPlay('pointermove', 'mouse')).toBe(true);
    expect(endsPlay('pointermove', 'pen')).toBe(true);
  });
  it('a press, a touch or a key anywhere ends it', () => {
    expect(endsPlay('pointerdown', 'mouse')).toBe(true);
    expect(endsPlay('pointerdown', 'touch')).toBe(true);
    expect(endsPlay('touchstart')).toBe(true);
    expect(endsPlay('keydown')).toBe(true);
  });
  it('a finger entering or dragging is no hover (its press has ended the play before)', () => {
    expect(endsPlay('pointerenter', 'touch')).toBe(false);
    expect(endsPlay('pointermove', 'touch')).toBe(false);
  });
  it('anything else leaves it playing', () => {
    expect(endsPlay('scroll')).toBe(false);
    expect(endsPlay('pointerup', 'mouse')).toBe(false);
  });
});

describe('the loop', () => {
  it('walks every step, then the whole board, then the first step again', () => {
    const seen: (number | null)[] = [];
    let step: number | null = null;
    for (let i = 0; i < 9; i++) { step = nextStep(step, 1, 3); seen.push(step); }
    expect(seen).toEqual([0, 1, 2, null, 0, 1, 2, null, 0]);
  });
  it('a beat is 2.5 s', () => {
    expect(PLAY_MS).toBe(2500);
  });
});

describe('playNext', () => {
  const lap = (count: number, overview: boolean, n: number) => {
    const seen: (number | null)[] = [];
    let step: number | null = null;
    for (let i = 0; i < n; i++) { step = playNext(step, count, overview); seen.push(step); }
    return seen;
  };

  it('a frame that can read the whole board: steps, the whole board, steps again', () => {
    expect(lap(3, true, 9)).toEqual([0, 1, 2, null, 0, 1, 2, null, 0]);
  });
  it('a frame that cannot (a small embed): steps only, round and round', () => {
    expect(lap(3, false, 8)).toEqual([0, 1, 2, 0, 1, 2, 0, 1]);
  });
  it('without steps there is nowhere to go', () => {
    expect(playNext(null, 0, false)).toBeNull();
    expect(playNext(null, 0, true)).toBeNull();
  });
});
