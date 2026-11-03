// Normalized boards (fixtures/*.json, produced by the backend's parser) must lay out with no overlapping siblings.
import { describe, expect, it } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';
import { clientFrame } from '@features/fit-view';
import { flowsDown, layout, wrapsTo, type Board } from '@entities/board';

globalThis.innerWidth = 1600; globalThis.innerHeight = 900;
const dir = path.join(__dirname, 'fixtures');

function overlaps(nodes: Awaited<ReturnType<typeof layout>>['nodes']) {
  const bad: string[] = [];
  for (const a of nodes) for (const b of nodes) {
    if (a.id >= b.id || (a.parentId ?? null) !== (b.parentId ?? null)) continue;
    const [ax, ay, aw, ah] = [a.position.x, a.position.y, Number(a.style!.width), Number(a.style!.height)];
    const [bx, by, bw, bh] = [b.position.x, b.position.y, Number(b.style!.width), Number(b.style!.height)];
    if (ax < bx + bw && bx < ax + aw && ay < by + bh && by < ay + ah) bad.push(`${a.id}×${b.id}`);
  }
  return bad;
}

describe('layout', () => {
  for (const f of fs.readdirSync(dir).filter(f => f.endsWith('.json'))) it(`${f}: no overlapping siblings`, async () => {
    const b = JSON.parse(fs.readFileSync(path.join(dir, f), 'utf8')) as Board;
    expect(overlaps((await layout(b, {}, 'x')).nodes)).toEqual([]);
  });
});

// D-2026-10-29-02: on a phone held upright a left-to-right board flows down.
const board = (f: string) => JSON.parse(fs.readFileSync(path.join(dir, f), 'utf8')) as Board;
const at = (nodes: Awaited<ReturnType<typeof layout>>['nodes']) => Object.fromEntries(nodes.map(n => [n.id, [n.position.x, n.position.y]]));

describe('layout on a phone', () => {
  it('flowsDown: LR and RL boards, never lanes, never TB/BT', () => {
    const b = (direction: Board['direction'], layout: Board['layout'] = 'auto') => ({ direction, layout }) as Board;
    expect([b('LR'), b('RL')].map(x => flowsDown(x, true))).toEqual([true, true]);
    expect([b('TB'), b('BT'), b('LR', 'lanes')].map(x => flowsDown(x, true))).toEqual([false, false, false]);
    expect(flowsDown(b('LR'), false)).toBe(false);
    expect(flowsDown(b('LR'), undefined)).toBe(false);
  });

  it('an LR chain runs down the screen instead of across it', async () => {
    const b = board('template-flow.json');
    const wide = at((await layout(b, {}, 'x')).nodes), tall = await layout(b, {}, 'x', { phone: true });
    const p = at(tall.nodes);
    expect(wide.a[0]).toBeLessThan(wide.b[0]);
    expect(p.a[1]).toBeLessThan(p.b[1]);
    expect(p.b[1]).toBeLessThan(p.c[1]);
    expect(tall.nodes.every(n => (n.data as { dir: string }).dir === 'TB')).toBe(true);
  });

  it('saved positions are ignored when the board is re-laid out', async () => {
    const b = board('template-flow.json');
    const saved = { a: { x: 900, y: 900 }, b: { x: -900, y: 5 } };
    expect(at((await layout(b, saved, 'x', { phone: true })).nodes)).toEqual(at((await layout(b, {}, 'x', { phone: true })).nodes));
    expect(at((await layout(b, saved, 'x')).nodes).a).toEqual([900, 900]);
  });

  it('groups flow down too, and nothing overlaps', async () => {
    for (const f of ['template-before-after.json', 'steps.json', 'template-mindmap.json', 'template-schema.json', 'cross-group.json', 'tables.json']) {
      const nodes = (await layout(board(f), {}, 'x', { phone: true })).nodes;
      expect(overlaps(nodes), f).toEqual([]);
      for (const n of nodes) expect((n.data as { dir?: string; pdir?: string }).dir ?? 'TB', f).toBe('TB');
    }
  });

  it('a TB board keeps its directions and its saved positions (its cards are the phone\'s narrower ones)', async () => {
    const b = board('mermaid-flow.json'), saved = { a1: { x: 11, y: 22 } };
    const phone = await layout(b, saved, 'x', { phone: true }), desktop = await layout(b, saved, 'x');
    expect(at(phone.nodes).a1).toEqual([11, 22]);
    expect(phone.nodes.map(n => [n.id, (n.data as { dir?: string }).dir])).toEqual(desktop.nodes.map(n => [n.id, (n.data as { dir?: string }).dir]));
  });

  it('a board with lanes keeps its lanes and its positions', async () => {
    for (const f of ['template-lanes.json', 'template-sequence.json']) {
      const b = board(f), saved = { [b.nodes[0].id]: { x: 33, y: 44 } };
      const phone = await layout(b, saved, 'x', { phone: true });
      expect(at(phone.nodes)[b.nodes[0].id], f).toEqual([33, 44]);
      const lanes = phone.nodes.filter(n => n.type === 'zone');
      expect(lanes.length, f).toBeGreaterThan(0);
      for (const l of lanes) expect(Number(l.style!.width), f).toBeGreaterThan(Number(l.style!.height)); // still bands across the board
    }
  });
});

// D-2026-10-29-02 amendment, row a: a card is at most 168 px wide on a phone and its title takes two lines at most.
describe('cards on a phone', () => {
  const cards = (nodes: Awaited<ReturnType<typeof layout>>['nodes']) => nodes.filter(n => n.type === 'card' && !(n.data as { fields?: unknown }).fields);
  const width = (n: { style?: { width?: unknown } }) => Number(n.style!.width);

  it('none is wider than 168 px, in a flow, in lanes and in a sequence; a desktop card keeps its width', async () => {
    for (const f of ['template-flow.json', 'template-before-after.json', 'steps.json', 'template-lanes.json', 'template-sequence.json', 'template-mindmap.json']) {
      const phone = cards((await layout(board(f), {}, 'x', { phone: true })).nodes), desktop = cards((await layout(board(f), {}, 'x')).nodes);
      expect(phone.length, f).toBeGreaterThan(0);
      for (const n of phone) expect(width(n), `${f}:${n.id}`).toBeLessThanOrEqual(168);
      expect(Math.max(...desktop.map(width)), f).toBeGreaterThan(168);
    }
  });

  it('a long title is two lines high, not more, and a short one stays one line', async () => {
    const b = { ...board('template-flow.json'), nodes: board('template-flow.json').nodes.map((n, i) => ({ ...n, title: i ? n.title : 'Письмо на почту с вложениями и ссылками на документы и таблицы', text: '', icon: null })) };
    const phone = cards((await layout(b, {}, 'x', { phone: true })).nodes), long = phone[0], short = phone[1];
    expect(Number(long.style!.height)).toBe(30 + 2 * 21); // the title is clamped to two lines
    expect(Number(short.style!.height)).toBeLessThan(Number(long.style!.height));
  });

  it('a card the board sizes itself is capped too', async () => {
    const b = board('template-flow.json');
    const wide = { ...b, nodes: b.nodes.map(n => ({ ...n, width: 400 })) };
    for (const n of cards((await layout(wide, {}, 'x', { phone: true })).nodes)) expect(width(n)).toBe(168);
    for (const n of cards((await layout(wide, {}, 'x')).nodes)) expect(width(n)).toBe(400);
  });
});

// D-2026-10-29-02 amendment, row e: lanes on a phone have no label column; each card carries its lane above its title.
describe('lanes on a phone', () => {
  const lanes = async (phone: boolean) => (await layout(board('template-lanes.json'), {}, 'x', { phone })).nodes;
  const card = (nodes: Awaited<ReturnType<typeof layout>>['nodes'], id: string) => nodes.find(n => n.id === id)!;

  it('a card names its lane, and the desktop does not', async () => {
    const b = board('template-lanes.json'), label = (g: string | null) => b.groups.find(x => x.id === g)!.label;
    for (const n of b.nodes) {
      expect((card(await lanes(true), n.id).data as { lane?: string }).lane, n.id).toBe(label(n.group));
      expect((card(await lanes(false), n.id).data as { lane?: string }).lane, n.id).toBeUndefined();
    }
  });

  it('the cards start at the edge, not after a 176 px label column', async () => {
    const phone = await lanes(true), desktop = await lanes(false);
    const left = (ns: typeof phone) => Math.min(...ns.filter(n => n.type === 'card').map(n => n.position.x));
    expect(left(desktop)).toBeGreaterThanOrEqual(176);
    expect(left(phone)).toBeLessThan(40);
  });

  it('a card is as much taller as its caption takes, and nothing overlaps', async () => {
    const b = board('template-lanes.json'), lane = b.groups[0];
    const one = { ...b, groups: [lane], edges: [], nodes: [{ ...b.nodes[0], group: lane.id, title: 'Short', text: '', icon: null, live: null }] };
    const height = async (phone: boolean) => Number(card((await layout(one, {}, 'x', { phone })).nodes, one.nodes[0].id).style!.height);
    expect(await height(false)).toBe(58);
    expect(await height(true)).toBe(30 + 21 + 17); // padding and one title line, plus the caption's 14 px line and 3 px gap
    expect(overlaps(await lanes(true))).toEqual([]);
  });
});

// D-2026-10-29-03 amendment, row h: a client's screen (or an embed frame) shapes a flat chain that is far wider than it
// into rows; boards of groups, saved positions, a phone and the editor keep what they had.
describe('layout for a frame', () => {
  const chain = (n: number): Board => ({
    title: '', sub: '', direction: 'LR', layout: 'auto', style: 'sketch', legend: [], groups: [], steps: [],
    nodes: Array.from({ length: n }, (_, i) => ({ id: `c${i}`, title: `Card ${i}`, text: 'a line', kind: 'core', group: null, icon: null, shape: null, width: null, step: null, fields: null, live: null })),
    edges: Array.from({ length: n - 1 }, (_, i) => ({ id: `e${i}`, from: `c${i}`, to: `c${i + 1}`, label: '' })),
  });
  const rows = (nodes: Awaited<ReturnType<typeof layout>>['nodes']) => new Set(nodes.map(n => Math.round(n.position.y / 40))).size;
  // a 1440 x 900 screen less the padding: 1344 x 722, read at 16/15 or more
  const wide = clientFrame({ width: 1440, height: 900 });

  it('wrapsTo: more than twice as wide, by aspect, as the frame, and too wide for it at the reading zoom', () => {
    expect(wrapsTo({ width: 1500, height: 80 }, wide)).toBe(true);
    expect(wrapsTo({ width: 3800, height: 1000 }, wide)).toBe(true);
    expect(wrapsTo({ width: 3700, height: 1000 }, wide)).toBe(false); // 3.7 is not over 2 x 1.86
    expect(wrapsTo({ width: 800, height: 600 }, wide)).toBe(false);
    expect(wrapsTo({ width: 600, height: 1200 }, wide)).toBe(false);
    expect(wrapsTo({ width: 1200, height: 80 }, wide)).toBe(false); // a strip that fills the width at 16/15 reads as it is
    expect(wrapsTo({ width: 1300, height: 80 }, wide)).toBe(true);
  });

  it('a five-card chain is one strip for no frame and several rows for a 16:10 one, wider than 2 x its aspect no more', async () => {
    const b = chain(5);
    expect(rows((await layout(b, {}, 'x')).nodes)).toBe(1);
    const nodes = (await layout(b, {}, 'x', { frame: wide })).nodes;
    expect(rows(nodes)).toBeGreaterThan(1);
    const xs = nodes.map(n => n.position.x), ys = nodes.map(n => n.position.y);
    const box = { width: Math.max(...xs) + 236 - Math.min(...xs), height: Math.max(...ys) + 75 - Math.min(...ys) };
    expect(wrapsTo(box, wide)).toBe(false);
    expect(overlaps(nodes)).toEqual([]);
    for (const id of ['c0', 'c1', 'c2', 'c3', 'c4']) expect(nodes.some(n => n.id === id)).toBe(true);
  });

  it('every edge of a wrapped chain still has a route to draw', async () => {
    const r = await layout(chain(5), {}, 'x', { frame: wide });
    expect(r.edges).toHaveLength(4);
    for (const e of r.edges) expect((e.data as { points?: unknown[] }).points?.length, e.id).toBeGreaterThanOrEqual(2);
  });

  it('a strip that already fills the screen at the reading zoom is left alone', async () => {
    const b = chain(4), plain = at((await layout(b, {}, 'x')).nodes);
    expect(at((await layout(b, {}, 'x', { frame: wide })).nodes)).toEqual(plain);
    expect(rows((await layout(chain(5), {}, 'x', { frame: wide })).nodes)).toBeGreaterThan(1); // one card more does not fit
  });

  it('boards of groups, a TB board, saved positions and a phone keep their layout', async () => {
    const groups = board('template-before-after.json');
    expect(at((await layout(groups, {}, 'x', { frame: wide })).nodes)).toEqual(at((await layout(groups, {}, 'x')).nodes));
    const tb = { ...chain(5), direction: 'TB' as const };
    expect(at((await layout(tb, {}, 'x', { frame: wide })).nodes)).toEqual(at((await layout(tb, {}, 'x')).nodes));
    const saved = { c0: { x: 5, y: 5 } };
    expect(at((await layout(chain(5), saved, 'x', { frame: wide })).nodes)).toEqual(at((await layout(chain(5), saved, 'x')).nodes));
    expect(at((await layout(chain(5), {}, 'x', { frame: wide, phone: true })).nodes)).toEqual(at((await layout(chain(5), {}, 'x', { phone: true })).nodes));
  });
});
