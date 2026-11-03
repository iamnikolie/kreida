import type { Node } from '@xyflow/react';
import type { BNode, Board, Pos } from '@shared/api';
import { size } from '@entities/card/@x/board';

// layout: matrix — a 2×2. Cards sit in the quadrant their x/y name (high y on top, high x on the right), several to a
// quadrant in a grid; all four quadrants are the size of the fullest. Cards without a quadrant line up under it.

export const MATRIX = '__matrix';
const AXIS = 46, FOOT = 46, PAD = 22, HEAD = 40, GAP = 16, UNDER = 48;
const CORNERS = ['low-high', 'high-high', 'low-low', 'high-low'] as const;

export interface Quad { x: number; y: number; w: number; h: number }
export interface MatrixData { matrix: NonNullable<Board['matrix']>; quads: Record<string, Quad>; axis: number; foot: number }

const colsFor = (n: number) => (n >= 7 ? 3 : n >= 3 ? 2 : 1);

export function matrix(b: Board, pos: Pos, board: string, phone: boolean): Node[] {
  const sizes = new Map(b.nodes.map(n => [n.id, size(n, { phone })]));
  const cornerOf = (n: BNode) => (n.x && n.y ? `${n.x}-${n.y}` : null);
  const inQuad = new Map(CORNERS.map(c => [c as string, b.nodes.filter(n => cornerOf(n) === c)]));
  const cellW = Math.max(160, ...b.nodes.filter(cornerOf).map(n => sizes.get(n.id)!.width));

  // each quadrant's grid: rows of up to `cols` cards, a row as tall as its tallest card
  const grids = new Map([...inQuad].map(([c, ns]) => {
    const cols = colsFor(ns.length), rows: BNode[][] = [];
    ns.forEach((n, i) => (rows[Math.floor(i / cols)] ??= []).push(n));
    const rowH = rows.map(r => Math.max(...r.map(n => sizes.get(n.id)!.height)));
    return [c, { cols: Math.min(cols, Math.max(ns.length, 1)), rows, rowH, w: Math.min(cols, ns.length) * (cellW + GAP) - GAP, h: rowH.reduce((a, h) => a + h + GAP, 0) - GAP }];
  }));
  const QW = Math.max(phone ? 200 : 320, ...[...grids.values()].map(g => g.w + 2 * PAD));
  const QH = Math.max(phone ? 170 : 230, ...[...grids.values()].map(g => g.h + HEAD + PAD));
  const quads: Record<string, Quad> = Object.fromEntries(CORNERS.map(c => {
    const [x, y] = c.split('-');
    return [c, { x: AXIS + (x === 'high' ? QW : 0), y: y === 'high' ? 0 : QH, w: QW, h: QH }];
  }));
  const W = AXIS + 2 * QW, H = 2 * QH + FOOT;

  const out: Node[] = [{
    id: MATRIX, type: 'matrix', position: { x: 0, y: 0 }, draggable: false, selectable: false, zIndex: -1,
    data: { matrix: b.matrix ?? { x: { label: '', low: 'low', high: 'high' }, y: { label: '', low: 'low', high: 'high' }, quadrants: {} }, quads, axis: AXIS, foot: FOOT } satisfies MatrixData,
    style: { width: W, height: H },
  }];
  const card = (n: BNode, position: { x: number; y: number }, parent?: string): Node => {
    const sz = sizes.get(n.id)!;
    return { id: n.id, type: 'card', position: pos[n.id] ?? position, ...(parent ? { parentId: parent } : {}),
      data: { ...n, dir: 'LR', board, ...(phone ? { phone } : {}) }, style: { width: sz.width, height: sz.height } };
  };
  for (const [c, g] of grids) {
    const q = quads[c], left = q.x + (q.w - g.w) / 2;
    let y = q.y + HEAD;
    g.rows.forEach((r, i) => {
      r.forEach((n, j) => { const sz = sizes.get(n.id)!; out.push(card(n, { x: left + j * (cellW + GAP) + (cellW - sz.width) / 2, y: y + (g.rowH[i] - sz.height) / 2 }, MATRIX)); });
      y += g.rowH[i] + GAP;
    });
  }
  let x = AXIS;
  for (const n of b.nodes.filter(n => !cornerOf(n))) { out.push(card(n, { x, y: H + UNDER })); x += sizes.get(n.id)!.width + GAP * 2; }
  return out;
}
