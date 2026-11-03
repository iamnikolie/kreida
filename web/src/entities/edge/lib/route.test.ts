import { expect, it } from 'vitest';
import { rounded, sitsAt } from './route';

it('a straight two-point route is a plain line', () => {
  expect(rounded([{ x: 0, y: 0 }, { x: 40, y: 0 }], 10)).toBe('M0,0 L40,0');
});

it('a corner is rounded by the radius, shrunk to half of a short segment', () => {
  const p = [{ x: 0, y: 0 }, { x: 100, y: 0 }, { x: 100, y: 100 }];
  expect(rounded(p, 10)).toBe('M0,0 L90,0 Q100,0 100,10 L100,100');
  const short = [{ x: 0, y: 0 }, { x: 100, y: 0 }, { x: 100, y: 8 }];
  expect(rounded(short, 10)).toBe('M0,0 L96,0 Q100,0 100,4 L100,8');
});

it('a route fits only while the node stays within a pixel of where ELK put it', () => {
  const at = (x: number, y: number) => ({ internals: { positionAbsolute: { x, y } } });
  expect(sitsAt(at(10, 20), { x: 10.5, y: 19.5 })).toBe(true);
  expect(sitsAt(at(10, 20), { x: 12, y: 20 })).toBe(false);
  expect(sitsAt(undefined, { x: 0, y: 0 })).toBe(false);
  expect(sitsAt(at(0, 0), undefined)).toBe(false);
});
