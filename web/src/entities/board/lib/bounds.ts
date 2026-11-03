import type { Node } from '@xyflow/react';

export interface Rect { x: number; y: number; width: number; height: number }

/**
 * The box around the laid-out nodes (all of them, or those `pick` accepts) in canvas coordinates. A child sits
 * relative to its group, so the parents are walked. Sizes come from the layout, which is what the card renders at.
 * Null when no node is picked.
 */
export function boundsOf(nodes: Node[], pick: (id: string) => boolean = () => true): Rect | null {
  const byId = new Map(nodes.map(n => [n.id, n]));
  let x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity;
  for (const n of nodes) {
    if (!pick(n.id)) continue;
    let x = n.position.x, y = n.position.y;
    for (let p = byId.get(n.parentId ?? ''); p; p = byId.get(p.parentId ?? '')) { x += p.position.x; y += p.position.y; }
    const w = n.measured?.width ?? Number(n.style?.width ?? n.width ?? 0), h = n.measured?.height ?? Number(n.style?.height ?? n.height ?? 0);
    x0 = Math.min(x0, x); y0 = Math.min(y0, y); x1 = Math.max(x1, x + w); y1 = Math.max(y1, y + h);
  }
  return x0 === Infinity ? null : { x: x0, y: y0, width: x1 - x0, height: y1 - y0 };
}
