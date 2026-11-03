// layout: matrix — a 2×2: cards sit in the quadrant their x/y name, cards without one under it.
import { describe, expect, it } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';
import { layout, type Board } from '@entities/board';

globalThis.innerWidth = 1600; globalThis.innerHeight = 900;
const board = JSON.parse(fs.readFileSync(path.join(__dirname, 'fixtures', 'matrix.json'), 'utf8')) as Board;

const rect = (n: { position: { x: number; y: number }; style?: { width?: unknown; height?: unknown } }) =>
  ({ x: n.position.x, y: n.position.y, w: Number(n.style!.width), h: Number(n.style!.height) });

describe('matrix layout', async () => {
  const { nodes } = await layout(board, {}, 'x');
  const frame = nodes.find(n => n.type === 'matrix')!;
  const byId = Object.fromEntries(nodes.map(n => [n.id, n]));
  const q = (frame.data as { quads: Record<string, { x: number; y: number; w: number; h: number }> }).quads;

  it('one frame holds the quadrants, the axes and the cards', () => {
    expect(frame).toBeDefined();
    expect(frame.draggable).toBe(false);
    expect(Object.keys(q).sort()).toEqual(['high-high', 'high-low', 'low-high', 'low-low']);
    for (const id of ['a', 'b', 'c', 'd']) expect(byId[id].parentId).toBe(frame.id);
  });

  it('a card sits inside the quadrant its x/y name: high y on top, high x on the right', () => {
    const inside = (id: string, k: string) => {
      const r = rect(byId[id]), z = q[k];
      return r.x >= z.x && r.y >= z.y && r.x + r.w <= z.x + z.w && r.y + r.h <= z.y + z.h;
    };
    expect([inside('a', 'low-high'), inside('b', 'high-high'), inside('c', 'low-low'), inside('d', 'high-low')]).toEqual([true, true, true, true]);
    expect(q['low-high'].y).toBeLessThan(q['low-low'].y);
    expect(q['low-high'].x).toBeLessThan(q['high-high'].x);
  });

  it('a card with no quadrant goes under the matrix', () => {
    const e = rect(byId.e), f = rect(frame);
    expect(byId.e.parentId).toBeUndefined();
    expect(e.y).toBeGreaterThanOrEqual(f.y + f.h);
  });

  it('saved positions win', async () => {
    const r = await layout(board, { a: { x: 5, y: 7 } }, 'x');
    expect(r.nodes.find(n => n.id === 'a')!.position).toEqual({ x: 5, y: 7 });
  });

  it('a crowded quadrant grows and its cards do not overlap', async () => {
    const many = { ...board, nodes: Array.from({ length: 7 }, (_, i) => ({ ...board.nodes[0], id: `m${i}` })) };
    const r = await layout(many, {}, 'x');
    const cards = r.nodes.filter(n => n.type === 'card').map(rect);
    for (const a of cards) for (const b of cards)
      if (a !== b) expect(a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h).toBe(false);
  });
});
