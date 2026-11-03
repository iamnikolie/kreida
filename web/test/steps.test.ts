import { describe, expect, it } from 'vitest';
import steps from './fixtures/steps.json';
import noSteps from './fixtures/no-steps.json';
import { fogOf, revealAt, stepsOf, visibleAt, type Board } from '@entities/board';

it('declared steps: shown items start hidden, hide removes, unmentioned stay visible', () => {
  const b = steps as unknown as Board, s = stepsOf(b);
  expect(['a', 'b', 'client', 'now', 'later'].map(visibleAt(b, s, 0))).toEqual([true, false, true, true, false]);
  expect(['a', 'b', 'client', 'now', 'later'].map(visibleAt(b, s, 1))).toEqual([false, true, true, false, true]);
});

it('no steps: top-level items reveal one by one', () => {
  const b = noSteps as unknown as Board, s = stepsOf(b);
  expect(s.map(x => x.show)).toEqual([['g'], ['x']]);
  expect(['a', 'x'].map(visibleAt(b, s, 0))).toEqual([true, false]);
});

it('fog is the opposite of visible, and absent while no step is selected', () => {
  const b = steps as unknown as Board, s = stepsOf(b);
  expect(fogOf(b, s, null)).toBeNull();
  const fog = fogOf(b, s, 1)!;
  expect(['a', 'b', 'client', 'now', 'later'].map(fog)).toEqual([true, false, false, true, false]);
});

it('a node follows its group through nested groups', () => {
  const b = {
    groups: [{ id: 'outer', parent: null }, { id: 'inner', parent: 'outer' }],
    nodes: [{ id: 'n', group: 'inner' }, { id: 'free', group: null }],
    steps: [{ title: 'one', show: ['outer'], hide: [] }, { title: 'two', show: [], hide: ['outer'] }],
  } as unknown as Board;
  const s = stepsOf(b);
  expect(['n', 'inner', 'free'].map(visibleAt(b, s, 0))).toEqual([true, true, true]);
  expect(['n', 'inner', 'free'].map(visibleAt(b, s, 1))).toEqual([false, false, true]);
  expect(['n', 'free'].map(fogOf(b, s, 1)!)).toEqual([true, false]);
  expect(['n'].map(visibleAt(b, [], 0))).toEqual([true]);
});

describe('revealAt', () => {
  it('is what the step brings into view, not what earlier steps already showed', () => {
    const b = steps as unknown as Board, s = stepsOf(b), ids = ['a', 'b', 'client', 'now', 'later'];
    expect(revealAt(b, s, null)).toBeNull();
    expect(ids.map(revealAt(b, s, 0)!)).toEqual([true, false, false, true, false]);
    expect(ids.map(revealAt(b, s, 1)!)).toEqual([false, true, false, false, true]);
  });

  it('with steps that add up, the newest group is what is new', () => {
    const b = noSteps as unknown as Board, s = stepsOf(b);
    expect(['g', 'a', 'x'].map(revealAt(b, s, 0)!)).toEqual([true, true, false]);
    expect(['g', 'a', 'x'].map(revealAt(b, s, 1)!)).toEqual([false, false, true]);
  });

  it('a step that only hides brings everything that shows', () => {
    const b = { groups: [], nodes: [{ id: 'p', group: null }, { id: 'q', group: null }], steps: [{ title: '', show: ['p'], hide: [] }, { title: '', show: [], hide: ['p'] }] } as unknown as Board;
    expect(['p', 'q'].map(revealAt(b, stepsOf(b), 1)!)).toEqual([false, true]);
  });
});
