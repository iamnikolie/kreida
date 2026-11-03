// Folding groups: a folded group is one tile on the page, edges into it end at the tile, the board file is untouched.
import { describe, expect, it } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';
import { defaultFolds, depthOf, fold, foldedInto, foldLevel, layout, rollUp, type Board } from '@entities/board';

globalThis.innerWidth = 1600; globalThis.innerHeight = 900;
const board = JSON.parse(fs.readFileSync(path.join(__dirname, 'fixtures', 'nested.json'), 'utf8')) as Board;
const ids = (b: Board) => b.nodes.map(n => n.id);
const links = (b: Board) => b.edges.map(e => `${e.from}>${e.to}${e.label ? `:${e.label}` : ''}`);

describe('fold', () => {
  it('nothing folded: the board as it is', () => {
    expect(fold(board, new Set()).board).toBe(board);
  });

  it('a folded group becomes one tile with its count and the first cards as a hint', () => {
    const { board: b, tiles } = fold(board, new Set(['product']));
    expect(ids(b)).not.toContain('p1');
    expect(b.groups.map(g => g.id)).toEqual(['what', 'scope', 'docs']);
    const tile = b.nodes.find(n => n.id === 'product')!;
    expect(tile).toMatchObject({ title: 'Продукт', group: 'what' });
    expect(tiles.get('product')).toEqual({ count: 2, hint: ['Бот', 'Админка'] });
  });

  it('edges into the group end at the tile; inside edges go, duplicates merge', () => {
    const { board: b } = fold(board, new Set(['scope', 'docs']));
    expect(links(b)).toEqual(['client>p1', 'p1>p2', 'client>scope', 'p2>scope', 'scope>docs', 'docs>out']);
  });

  it('folding an outer group hides the inner ones; the outer tile counts every card', () => {
    const { board: b, tiles } = fold(board, new Set(['what', 'product']));
    expect(b.groups).toEqual([]);
    expect(ids(b)).toEqual(['client', 'out', 'what']);
    expect(tiles.get('what')!.count).toBe(5);
    expect(links(b)).toEqual(['client>what', 'what>out']);
  });

  it('steps that name a folded card name its tile', () => {
    const stepped = { ...board, steps: [{ title: '', show: ['p1', 'client'], hide: ['s1'] }] };
    expect(fold(stepped, new Set(['product'])).board.steps).toEqual([{ title: '', show: ['product', 'client'], hide: ['s1'] }]);
  });
});

describe('fold levels', () => {
  it('depth: top-level groups are 1', () => {
    expect([depthOf(board, 'what'), depthOf(board, 'docs')]).toEqual([1, 2]);
  });

  it('level n folds the groups n deep', () => {
    expect([...foldLevel(board, 1)]).toEqual(['what']);
    expect([...foldLevel(board, 2)]).toEqual(['product', 'scope', 'docs']);
  });

  it('the board file decides what starts folded', () => {
    expect([...defaultFolds(board)]).toEqual(['scope']);
  });
});

describe('a folded board lays out', () => {
  it('the tile is a card that knows it is a fold, and nothing overlaps', async () => {
    const { nodes } = await layout(board, {}, 'x', { folds: new Set(['what']) });
    const tile = nodes.find(n => n.id === 'what')!;
    expect(tile.type).toBe('card');
    expect((tile.data as { fold?: { count: number } }).fold?.count).toBe(5);
    expect(nodes.map(n => n.id).sort()).toEqual(['client', 'out', 'what']);
  });

  it('a folded group takes less room than the open one', async () => {
    const open = await layout(board, {}, 'x', { folds: new Set() });
    const shut = await layout(board, {}, 'x', { folds: new Set(['what']) });
    const width = (ns: typeof open.nodes) => Math.max(...ns.filter(n => !n.parentId).map(n => n.position.x + Number(n.style!.width)));
    expect(width(shut.nodes)).toBeLessThan(width(open.nodes));
  });
});

describe('what a tile carries for the cards inside it', () => {
  it('foldedInto maps a hidden card or group to its tile, a shown one to itself', () => {
    const into = foldedInto(board, new Set(['what']));
    expect([into('p1'), into('scope'), into('client'), into('nowhere')]).toEqual(['what', 'what', 'client', 'nowhere']);
  });

  it('rollUp sums counts per tile', () => {
    const into = foldedInto(board, new Set(['product']));
    expect(rollUp(new Map([['p1', 1], ['p2', 2], ['client', 1]]), into)).toEqual(new Map([['product', 3], ['client', 1]]));
  });
});
