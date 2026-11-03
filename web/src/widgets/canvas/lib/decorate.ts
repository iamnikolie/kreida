import type { Edge, Node } from '@xyflow/react';
import type { Fog, LiveValues } from '@entities/board';

// What the canvas adds on top of the laid-out graph, without re-running the layout: the "changed" glow, the
// presentation fog, live values and the sketch flag. Untouched items keep their identity, so React skips them.

export interface Marks {
  /** Open questions of the board per card id. */
  asks?: Map<string, number>;
  /** Open notes for the agent per card id. */
  notes?: Map<string, number>;
}

export function decorateNodes(nodes: Node[], v: { fresh: Set<string>; fog: Fog | null; live: LiveValues; sketch: boolean } & Marks): Node[] {
  return nodes.map(n => {
    const c = [v.fresh.has(n.id) && 'fresh', v.fog?.(n.id) && 'fog'].filter(Boolean).join(' ');
    const live = v.live[n.id], asks = v.asks?.get(n.id), notes = v.notes?.get(n.id);
    const m = live || v.sketch || asks || notes
      ? { ...n, data: { ...n.data, liveVal: live, sketch: v.sketch, ...(asks ? { asks } : {}), ...(notes ? { notes } : {}) } } : n;
    return c ? { ...m, className: c } : m;
  });
}

export function decorateEdges(edges: Edge[], v: { fog: Fog | null; sketch: boolean }): Edge[] {
  return edges.map(e => {
    const m = v.sketch ? { ...e, data: { ...e.data, sketch: v.sketch } } : e;
    return v.fog && (v.fog(e.source) || v.fog(e.target)) ? { ...m, className: `${e.className ?? ''} fog` } : m;
  });
}
