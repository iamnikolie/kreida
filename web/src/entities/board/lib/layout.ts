import ELK, { type ElkNode, type ElkExtendedEdge } from 'elkjs/lib/elk.bundled.js';
import { Position, type Edge, type Node } from '@xyflow/react';
import type { Board, Dir, Pos } from '@shared/api';
import { BORDER, ROW, headH, rowSides, size } from '@entities/card/@x/board';
import type { RoutedData } from '@entities/edge/@x/board';
import { edgesOf } from './edges';
import { defaultFolds, fold, type Tile } from './fold';
import { lanes } from './lanes';
import { MATRIX, matrix } from './matrix';
import { radial } from './radial';
import { straight } from './straight';

const elk = new ELK();
const ELKDIR: Record<Dir, string> = { LR: 'RIGHT', RL: 'LEFT', TB: 'DOWN', BT: 'UP' };
const ELKSIDE: Record<string, string> = { [Position.Left]: 'WEST', [Position.Right]: 'EAST' };

const base = {
  'elk.algorithm': 'layered',
  'elk.spacing.nodeNode': '34',
  'elk.layered.spacing.nodeNodeBetweenLayers': '76',
  'elk.spacing.componentComponent': '56',
  'elk.spacing.edgeNode': '24',
  'elk.spacing.edgeEdge': '14',
  'elk.layered.spacing.edgeEdgeBetweenLayers': '14',
  'elk.layered.spacing.edgeNodeBetweenLayers': '24',
  'elk.layered.nodePlacement.strategy': 'NETWORK_SIMPLEX',
  'elk.edgeLabels.placement': 'CENTER',
  'elk.json.edgeCoords': 'ROOT',
};
// Keep the file's node order. Under INCLUDE_CHILDREN these must sit on the root only:
// on a child container ELK throws "Cannot read properties of undefined (reading 'a')".
const order = {
  'elk.layered.considerModelOrder.strategy': 'NODES_AND_EDGES',
  'elk.layered.crossingMinimization.forceNodeModelOrder': 'true',
};

export interface LayoutOptions {
  /** A phone held upright: a left-to-right board is laid out top-to-bottom (see `flowsDown`). */
  phone?: boolean;
  /**
   * The room the board is laid out for (a client's screen, or the frame it is embedded in, less what the chrome takes): a
   * flat board that comes out more than twice as wide as this is laid out again as rows of its aspect (see `wrapsTo`).
   * Without it a flat chain of 7 or more cards wraps for the window it is opened in (the editor).
   */
  frame?: Frame;
  /** The groups folded into tiles (a view state, see `fold`); without it, what the board file folds (`collapsed: true`). */
  folds?: Set<string>;
}

/** FOLD_LINE: the line a folded tile adds under its title for the count. */
const FOLD_LINE = 18;

/** The room a board is shaped to. `zoom`: how large it should read there (1 by default): a board whose width fits at that zoom needs no rows. */
export interface Frame { width: number; height: number; zoom?: number }

/** How much wider than its frame a board may come out before it is laid out again as rows. */
const WIDER = 2;

/**
 * Whether a board of this size, laid out for `frame`, is to be shaped to it: more than twice as wide (by aspect) as the
 * frame and too wide for it at the zoom it should read at. A strip that already fills the width at that zoom is left alone.
 */
export const wrapsTo = (box: { width: number; height: number }, frame: Frame): boolean =>
  box.width / Math.max(box.height, 1) > WIDER * (frame.width / Math.max(frame.height, 1)) && box.width * (frame.zoom ?? 1) > frame.width;

/** A phone is too narrow for a left-to-right flow: LR/RL boards flow DOWN there, the board file stays as it is. Lanes are rows by meaning and keep theirs. */
export const flowsDown = (b: Board, phone: boolean | undefined): boolean => !!phone && b.layout !== 'lanes' && (b.direction === 'LR' || b.direction === 'RL');

/** Board -> positioned React Flow nodes and routed edges. Saved positions (`pos`) override the computed ones, except on a phone that re-lays a board out. */
export async function layout(src: Board, pos: Pos, board: string, { phone, frame, folds }: LayoutOptions = {}): Promise<{ nodes: Node[]; edges: Edge[] }> {
  if (src.layout === 'matrix') {
    const nodes = matrix(src, pos, board, !!phone), at = new Map<string, { x: number; y: number; w: number; h: number }>();
    for (const n of nodes) {
      const o = n.parentId ? nodes.find(p => p.id === n.parentId)!.position : { x: 0, y: 0 };
      at.set(n.id, { x: o.x + n.position.x, y: o.y + n.position.y, w: Number(n.style!.width), h: Number(n.style!.height) });
    }
    const visible = src.edges.filter(e => !e.hidden && at.has(e.from) && at.has(e.to) && e.from !== MATRIX);
    return { nodes, edges: edgesOf(visible, new Map(visible.map(e => [e.id, straight(at.get(e.from)!, at.get(e.to)!)]))) };
  }
  if (src.layout === 'radial' && !phone) return radial(src, pos, board);
  // folding is for the flow of groups; lanes are rows by meaning
  const { board: b, tiles } = src.layout === 'lanes' ? { board: src, tiles: new Map<string, Tile>() } : fold(src, folds ?? defaultFolds(src));
  const down = flowsDown(b, phone);
  if (down) pos = {};
  const flow = (d: Dir): Dir => (down && (d === 'LR' || d === 'RL') ? 'TB' : d);
  const groups = new Map(b.groups.map(g => [g.id, g]));
  const nodes = new Map(b.nodes.map(n => [n.id, n]));
  const parentOf = (id: string): string | null => {
    const p = groups.has(id) ? groups.get(id)!.parent : nodes.get(id)?.group ?? null;
    return p && groups.has(p) ? p : null;
  };
  const chain = (id: string | null) => { const out: (string | null)[] = []; while (id) { out.push(id); id = parentOf(id); } out.push(null); return out; };
  const lca = (a: string, c: string) => { const ca = chain(parentOf(a)); const cc = new Set(chain(parentOf(c))); return ca.find(x => cc.has(x)) ?? null; };
  const dirOf = (gid: string | null): Dir => { while (gid) { const g = groups.get(gid)!; if (g.direction) return flow(g.direction); gid = g.parent; } return flow(b.direction); };

  const visible = b.edges.filter(e => !e.hidden);
  if (b.layout === 'lanes') return { nodes: lanes(b, pos, board, !!phone), edges: edgesOf(visible) };
  const include = visible.some(e => parentOf(e.from) !== parentOf(e.to));

  /** The ELK graph of the board; `wrap` shapes a flat chain into rows of this aspect ratio. */
  const graph = (wrap: number | null): ElkNode => {
    const containers = new Map<string | null, ElkNode>();
    const opts = (dir: Dir, pad: boolean) => ({
      ...base, ...(include && pad ? {} : order), 'elk.direction': ELKDIR[dir],
      'elk.hierarchyHandling': include ? 'INCLUDE_CHILDREN' : 'SEPARATE_CHILDREN',
      ...(pad ? { 'elk.padding': '[top=48,left=28,bottom=28,right=28]' } : {}),
    });
    const root: ElkNode = { id: '__root', layoutOptions: opts(flow(b.direction), false), children: [], edges: [] };
    containers.set(null, root);
    for (const g of b.groups) containers.set(g.id, { id: g.id, layoutOptions: opts(dirOf(g.id), true), children: [], edges: [] });
    for (const g of b.groups) containers.get(parentOf(g.id))!.children!.push(containers.get(g.id)!);
    for (const n of b.nodes) {
      const sz = size(n, { phone }), el: ElkNode = { id: n.id, ...sz };
      if (tiles.has(n.id)) el.height = sz.height + FOLD_LINE;
      if (n.fields) {
        const [inS, outS] = rowSides(dirOf(parentOf(n.id))).map(p => ELKSIDE[p]), head = headH(n);
        const port = (side: string, f: string, i: number) => ({ id: `${n.id}::${side === inS ? 'in' : 'out'}:${f}`, width: 1, height: 1,
          x: side === 'WEST' ? 0 : sz.width - 1, y: BORDER + head + i * ROW + ROW / 2, layoutOptions: { 'elk.port.side': side } });
        el.layoutOptions = { 'elk.portConstraints': 'FIXED_POS' };
        el.ports = n.fields.flatMap((f, i) => [port(inS, f.name, i), ...(f.name === 'id' ? [port(outS, f.name, i)] : [])]);
      }
      containers.get(parentOf(n.id))!.children!.push(el);
    }
    // Rows: ELK breaks the chain where the drawing would pass `wrap` (width / height).
    if (wrap != null)
      Object.assign(root.layoutOptions!, {
        'elk.layered.wrapping.strategy': 'MULTI_EDGE', 'elk.aspectRatio': String(wrap),
        'elk.layered.wrapping.additionalEdgeSpacing': '24',
      });

    for (const e of b.edges) {
      const where = include ? root : containers.get(lca(e.from, e.to))!;
      const labels = e.label && !e.hidden ? [{ id: `${e.id}:l`, text: e.label, width: e.label.length * 7 + 24, height: 22 }] : undefined;
      const sources = [e.fromPort ? `${e.from}::out:${e.fromPort}` : e.from], targets = [e.toPort ? `${e.to}::in:${e.toPort}` : e.to];
      where.edges!.push({ id: e.id, sources, targets, labels } as ElkExtendedEdge);
    }
    // Containers with no edges at all: keep declared order as a line in the container's direction.
    if (!include) for (const c of containers.values()) {
      if (c.edges!.length || c.children!.length < 2) continue;
      c.children!.slice(1).forEach((k, i) => c.edges!.push({ id: `__chain${c.id}${i}`, sources: [c.children![i].id], targets: [k.id] }));
    }
    return root;
  };

  // A flat chain that does not fit its frame wraps into rows; boards of groups keep their declared stacking.
  const flat = !b.groups.length && !down && b.direction !== 'TB' && b.direction !== 'BT';
  let out: ElkNode;
  if (frame) {
    out = await elk.layout(graph(null));
    // saved positions keep the board as its owner drew it
    if (flat && !Object.keys(pos).length && wrapsTo({ width: out.width ?? 0, height: out.height ?? 0 }, frame))
      out = await elk.layout(graph(frame.width / frame.height));
  } else out = await elk.layout(graph(flat && b.nodes.length >= 7 ? (innerWidth / innerHeight) * 1.1 : null));
  const rf: Node[] = [];
  const abs = new Map<string, { x: number; y: number }>();
  const routes = new Map<string, RoutedData>();
  const walk = (k: ElkNode, parent: string | null, ox: number, oy: number) => {
    for (const e of k.edges ?? []) {
      const sec = e.sections?.[0], l = e.labels?.[0];
      if (sec) routes.set(e.id, {
        points: [sec.startPoint, ...(sec.bendPoints ?? []), sec.endPoint],
        label: l ? { x: (l.x ?? 0) + (l.width ?? 0) / 2, y: (l.y ?? 0) + (l.height ?? 0) / 2 } : undefined,
        src: { x: 0, y: 0 }, dst: { x: 0, y: 0 },
      });
    }
    for (const c of k.children ?? []) {
      abs.set(c.id, { x: ox + (c.x ?? 0), y: oy + (c.y ?? 0) });
      const position = pos[c.id] ?? { x: c.x ?? 0, y: c.y ?? 0 };
      const common = { id: c.id, position, ...(parent ? { parentId: parent } : {}) };
      if (groups.has(c.id)) {
        const g = groups.get(c.id)!;
        rf.push({ ...common, type: 'zone', data: { label: g.label, kind: g.kind, dir: dirOf(g.id), pdir: dirOf(g.parent), foldable: true },
          style: { width: c.width, height: c.height }, zIndex: -1 });
        walk(c, c.id, ox + (c.x ?? 0), oy + (c.y ?? 0));
      } else {
        const n = nodes.get(c.id)!;
        const tile = tiles.get(n.id);
        rf.push({ ...common, type: 'card', expandParent: true, data: { ...n, dir: dirOf(parentOf(n.id)), board, ...(phone ? { phone } : {}), ...(tile ? { fold: tile } : {}) },
          style: { width: c.width, height: c.height } });
      }
    }
  };
  walk(out, null, 0, 0);
  for (const e of visible) {
    const r = routes.get(e.id), s = abs.get(e.from), t = abs.get(e.to);
    if (r && s && t) { r.src = s; r.dst = t; } else routes.delete(e.id);
  }

  return { nodes: rf, edges: edgesOf(visible, routes) };
}
