// layout: radial — the first root in the middle, its branches around it in rings, each branch one colour family.
import { describe, expect, it } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';
import { layout, type Board } from '@entities/board';

globalThis.innerWidth = 1600; globalThis.innerHeight = 900;
const board = JSON.parse(fs.readFileSync(path.join(__dirname, 'fixtures', 'radial.json'), 'utf8')) as Board;
const center = (n: { position: { x: number; y: number }; style?: { width?: unknown; height?: unknown } }) =>
  ({ x: n.position.x + Number(n.style!.width) / 2, y: n.position.y + Number(n.style!.height) / 2 });

describe('radial layout', async () => {
  const { nodes, edges } = await layout(board, {}, 'x');
  const byId = Object.fromEntries(nodes.map(n => [n.id, n]));
  const dist = (id: string) => Math.hypot(center(byId[id]).x, center(byId[id]).y);

  it('the root sits at the centre, its children on a ring, theirs further out', () => {
    expect(dist('root')).toBeLessThan(1);
    expect(Math.abs(dist('a') - dist('b'))).toBeLessThan(1);
    expect(Math.abs(dist('a') - dist('c'))).toBeLessThan(1);
    expect(dist('a1')).toBeGreaterThan(dist('a') + 40);
  });

  it('a branch keeps one colour family, the root has none', () => {
    const fam = (id: string) => (byId[id].data as { branch?: number }).branch;
    expect(fam('root')).toBeUndefined();
    expect(fam('a1')).toBe(fam('a'));
    expect(fam('a2')).toBe(fam('a'));
    expect(fam('b1')).toBe(fam('b'));
    expect(new Set([fam('a'), fam('b'), fam('c')]).size).toBe(3);
    expect(edges.find(e => e.source === 'a' && e.target === 'a1')!.className).toContain(`b-${fam('a')}`);
  });

  it('edges run straight between the cards', () => {
    const e = edges.find(x => x.source === 'root' && x.target === 'b')!;
    expect((e.data as { points: unknown[] }).points).toHaveLength(2);
  });

  it('cards do not overlap', () => {
    const r = nodes.map(n => ({ ...n.position, w: Number(n.style!.width), h: Number(n.style!.height) }));
    for (const a of r) for (const b of r)
      if (a !== b) expect(a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h).toBe(false);
  });

  it('on a phone the mindmap is a tree flowing down', async () => {
    const r = await layout(board, {}, 'x', { phone: true });
    const p = Object.fromEntries(r.nodes.map(n => [n.id, n.position]));
    expect(p.root.y).toBeLessThan(p.a.y);
    expect(p.a.y).toBeLessThan(p.a1.y);
  });
});
