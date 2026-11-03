import type { XY } from '../model/types';

/** Polyline through the ELK bend points with rounded corners (radius r, shrunk where a segment is short). */
export function rounded(p: XY[], r: number) {
  let d = `M${p[0].x},${p[0].y}`;
  for (let i = 1; i < p.length - 1; i++) {
    const [a, b, c] = [p[i - 1], p[i], p[i + 1]];
    const k = Math.min(r, Math.hypot(b.x - a.x, b.y - a.y) / 2, Math.hypot(c.x - b.x, c.y - b.y) / 2);
    const u = (q: XY) => { const l = Math.hypot(q.x - b.x, q.y - b.y) || 1; return { x: b.x + ((q.x - b.x) / l) * k, y: b.y + ((q.y - b.y) / l) * k }; };
    const s = u(a), e = u(c);
    d += ` L${s.x},${s.y} Q${b.x},${b.y} ${e.x},${e.y}`;
  }
  const z = p[p.length - 1];
  return `${d} L${z.x},${z.y}`;
}

/** Whether a node sits (within a pixel) where ELK put it, so its computed route still fits. */
export const sitsAt = (node: { internals: { positionAbsolute: XY } } | undefined, xy?: XY) =>
  !!node && !!xy && Math.abs(node.internals.positionAbsolute.x - xy.x) < 1 && Math.abs(node.internals.positionAbsolute.y - xy.y) < 1;
