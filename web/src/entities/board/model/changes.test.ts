import { expect, it } from 'vitest';
import type { Board } from '@shared/api';
import { changedCards, signatures } from './changes';

const card = (id: string, title: string, extra: object = {}) =>
  ({ id, title, text: '', kind: 'src', group: null, icon: null, fields: null, ...extra });
const board = (...nodes: ReturnType<typeof card>[]) => ({ nodes }) as unknown as Board;

it('a card that changed its text is reported, the rest are not', () => {
  const before = signatures(board(card('a', 'A'), card('b', 'B'), card('c', 'C')));
  const now = signatures(board(card('a', 'A'), card('b', 'B2'), card('c', 'C')));
  expect(changedCards(before, now)).toEqual(new Set(['b']));
});

it('a new card counts as changed', () => {
  const before = signatures(board(card('a', 'A'), card('b', 'B')));
  const now = signatures(board(card('a', 'A'), card('b', 'B'), card('c', 'C')));
  expect(changedCards(before, now)).toEqual(new Set(['c']));
});

it('nothing changed: null', () => {
  const s = signatures(board(card('a', 'A'), card('b', 'B')));
  expect(changedCards(s, signatures(board(card('a', 'A'), card('b', 'B'))))).toBeNull();
});

it('everything changed (another board, not an edit): null', () => {
  const before = signatures(board(card('a', 'A'), card('b', 'B')));
  expect(changedCards(before, signatures(board(card('x', 'X'), card('y', 'Y'))))).toBeNull();
});

it('only what a reader sees counts: kind, group, icon and fields, not the card width', () => {
  const before = signatures(board(card('a', 'A'), card('b', 'B')));
  expect(changedCards(before, signatures(board(card('a', 'A', { width: 400 }), card('b', 'B'))))).toBeNull();
  expect(changedCards(before, signatures(board(card('a', 'A', { kind: 'out' }), card('b', 'B'))))).toEqual(new Set(['a']));
});

it('a card whose picture changed glows, though its text did not', () => {
  const pic = (sha256: string) => ({ image: { sha256 } });
  const before = signatures(board(card('a', 'A', pic('one')), card('b', 'B', pic('two'))));
  const now = signatures(board(card('a', 'A', pic('new')), card('b', 'B', pic('two'))));
  expect(changedCards(before, now)).toEqual(new Set(['a']));
});
