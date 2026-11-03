import type { Edge, Node } from '@xyflow/react';
import type { Board, Pos } from '@shared/api';
import { size } from '@entities/card/@x/board';
import type { RoutedData } from '@entities/edge/@x/board';
import { edgesOf } from './edges';
import { straight } from './straight';

// layout: radial — a mindmap. The first card no edge points to sits in the middle; the tree its edges make spreads around
// it in rings, each subtree taking an angle in proportion to its leaves. Every child of the root starts a colour family
// its whole branch keeps. Edges run straight. A card the tree does not reach hangs off the root.

const RING = 46, SIDE = 34, FAMILIES = 5;

export function radial(b: Board, pos: Pos, board: string): { nodes: Node[]; edges: Edge[] } {
  const visible = b.edges.filter(e => !e.hidden);
  const ids = b.nodes.map(n => n.id), known = new Set(ids);
  const sizes = new Map(b.nodes.map(n => [n.id, size(n)]));
  const pointed = new Set(visible.filter(e => known.has(e.from)).map(e => e.to));
  const root = ids.find(id => !pointed.has(id)) ?? ids[0];
  const kids = new Map<string, string[]>(ids.map(id => [id, []]));
  const depth = new Map<string, number>(), family = new Map<string, number>();
  if (root == null) return { nodes: [], edges: [] };

  // the tree: breadth first along the edges in file order; whatever it misses hangs off the root
  depth.set(root, 0);
  const queue = [root];
  const reach = () => {
    while (queue.length) {
      const id = queue.shift()!;
      for (const e of visible) if (e.from === id && known.has(e.to) && !depth.has(e.to)) {
        depth.set(e.to, depth.get(id)! + 1); kids.get(id)!.push(e.to); queue.push(e.to);
      }
    }
  };
  reach();
  for (const id of ids) if (!depth.has(id)) { depth.set(id, 1); kids.get(root)!.push(id); queue.push(id); reach(); }
  kids.get(root)!.forEach((k, i) => { const mark = (id: string) => { family.set(id, i % FAMILIES); kids.get(id)!.forEach(mark); }; mark(k); });

  const leaves = new Map<string, number>();
  const count = (id: string): number => { const n = kids.get(id)!.reduce((a, k) => a + count(k), 0) || 1; leaves.set(id, n); return n; };
  count(root);

  // angles: each subtree its share of its parent's span
  const span = new Map<string, [number, number]>([[root, [-Math.PI / 2, (3 * Math.PI) / 2]]]);
  const share = (id: string) => {
    const [a0, a1] = span.get(id)!;
    let a = a0;
    for (const k of kids.get(id)!) { const s = ((a1 - a0) * leaves.get(k)!) / leaves.get(id)!; span.set(k, [a, a + s]); a += s; share(k); }
  };
  share(root);

  // rings: far enough out to clear the ring inside, and wide enough that a card's arc holds it
  const D = Math.max(...depth.values());
  const half = (id: string) => Math.hypot(sizes.get(id)!.width, sizes.get(id)!.height) / 2;
  const at = (d: number) => ids.filter(id => depth.get(id) === d);
  const r: number[] = [0];
  for (let d = 1; d <= D; d++) {
    const out = Math.max(...at(d - 1).map(half)) + Math.max(...at(d).map(half)) + RING;
    const arc = Math.max(...at(d).map(id => { const [a0, a1] = span.get(id)!; return (sizes.get(id)!.width + SIDE) / Math.max(a1 - a0, 1e-3); }));
    r[d] = Math.max(r[d - 1] + out, arc);
  }

  const boxes = new Map(ids.map(id => {
    const [a0, a1] = span.get(id)!, a = (a0 + a1) / 2, rr = r[depth.get(id)!], sz = sizes.get(id)!;
    const c = id === root ? { x: 0, y: 0 } : { x: rr * Math.cos(a), y: rr * Math.sin(a) };
    const p = pos[id] ?? { x: c.x - sz.width / 2, y: c.y - sz.height / 2 };
    return [id, { x: p.x, y: p.y, w: sz.width, h: sz.height }];
  }));

  const nodes: Node[] = b.nodes.map(n => {
    const box = boxes.get(n.id)!, f = family.get(n.id);
    return { id: n.id, type: 'card', position: { x: box.x, y: box.y }, data: { ...n, dir: 'LR', board, ...(f != null ? { branch: f } : {}) },
      style: { width: box.w, height: box.h } };
  });
  const routes = new Map<string, RoutedData>();
  for (const e of visible) if (boxes.has(e.from) && boxes.has(e.to)) routes.set(e.id, straight(boxes.get(e.from)!, boxes.get(e.to)!));
  const edges = edgesOf(visible, routes).map(e => {
    const f = family.get(e.target), mind = { ...e, markerEnd: undefined }; // a mindmap's branches carry no arrows
    return f == null ? mind : { ...mind, className: `${e.className ?? ''} b-${f}`.trim() };
  });
  return { nodes, edges };
}
