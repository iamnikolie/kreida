import type { Node } from '@xyflow/react';
import type { BNode, Board, Pos } from '@shared/api';
import { size } from '@entities/card/@x/board';

// Swimlanes: top-level groups are horizontal bands (who), columns are steps (when).
// Column = node.step, else the longest path from a start node.
// On a phone there is no room for the column of lane names: the bands start at the edge and every card names its lane above its title.
export function lanes(b: Board, pos: Pos, board: string, phone = false): Node[] {
  const LABEL = phone ? 16 : 176, GX = 64, GY = 18, PT = 26, PB = 26, PR = 36, LANE_GAP = 14;
  const top = b.groups.filter(g => !g.parent);
  const laneOf = (n: BNode) => (n.group && top.some(g => g.id === n.group) ? n.group : '__none');
  const ids = new Set(b.nodes.map(n => n.id));
  const rank = new Map(b.nodes.map(n => [n.id, n.step ?? 0]));
  const fixed = new Set(b.nodes.filter(n => n.step != null).map(n => n.id));
  const es = b.edges.filter(e => !e.hidden && ids.has(e.from) && ids.has(e.to));
  for (let i = 0; i < b.nodes.length; i++) {
    let moved = false;
    for (const e of es) if (!fixed.has(e.to) && rank.get(e.to)! < rank.get(e.from)! + 1) { rank.set(e.to, rank.get(e.from)! + 1); moved = true; }
    if (!moved) break;
  }
  const laneLabel = (n: BNode) => top.find(g => g.id === laneOf(n))?.label ?? '';
  const sizes = new Map(b.nodes.map(n => [n.id, size(n, { phone, caption: phone && !!laneLabel(n) })]));
  const R = Math.max(0, ...rank.values()) + 1;
  const colW = Array.from({ length: R }, (_, r) => Math.max(0, ...b.nodes.filter(n => rank.get(n.id) === r).map(n => sizes.get(n.id)!.width)));
  const colX: number[] = []; colW.reduce((x, w, r) => { colX[r] = x; return x + (w ? w + GX : 0); }, LABEL);
  const width = colX[R - 1] + colW[R - 1] + PR;

  const out: Node[] = [];
  const all = [...top.map(g => ({ id: g.id, label: g.label, kind: g.kind })),
    ...(b.nodes.some(n => laneOf(n) === '__none') ? [{ id: '__none', label: '', kind: null }] : [])];
  let y = 0;
  for (const lane of all) {
    const mine = b.nodes.filter(n => laneOf(n) === lane.id);
    const stacks = Array.from({ length: R }, (_, r) => mine.filter(n => rank.get(n.id) === r));
    const stackH = stacks.map(st => st.reduce((h, n, i) => h + sizes.get(n.id)!.height + (i ? GY : 0), 0));
    const inner = Math.max(58, ...stackH), height = PT + inner + PB;
    out.push({ id: lane.id, type: 'zone', position: pos[lane.id] ?? { x: 0, y }, zIndex: -1, draggable: false,
      data: { label: lane.label, kind: lane.kind, dir: 'LR', pdir: 'TB', lane: true }, style: { width, height } });
    stacks.forEach((st, r) => {
      let cy = PT + (inner - stackH[r]) / 2;
      for (const n of st) {
        const sz = sizes.get(n.id)!;
        out.push({ id: n.id, type: 'card', parentId: lane.id, expandParent: true,
          position: pos[n.id] ?? { x: colX[r] + (colW[r] - sz.width) / 2, y: cy },
          data: { ...n, dir: 'LR', board, ...(phone ? { phone } : {}), ...(phone && lane.label ? { lane: lane.label } : {}) }, style: { width: sz.width, height: sz.height } });
        cy += sz.height + GY;
      }
    });
    y += height + LANE_GAP;
  }
  return out;
}
