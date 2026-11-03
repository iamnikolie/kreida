import type { BNode, Board } from '@shared/api';

// Folding groups is a view of the board, never an edit of it: a folded group becomes one tile (its id, its label, how
// many cards it holds), edges into the group end at the tile, edges inside it go and duplicates merge. A folded group
// inside a folded one is hidden with it.

export interface Tile { count: number; hint: string[] }

/** How deep a group sits: a top-level group is 1. */
export function depthOf(b: Board, id: string): number {
  const parent = new Map(b.groups.map(g => [g.id, g.parent]));
  let d = 0;
  for (let g: string | null | undefined = id; g && parent.has(g) && d <= b.groups.length; g = parent.get(g)) d++;
  return d;
}

/** The groups `level` deep: folding them is "fold to level n". */
export const foldLevel = (b: Board, level: number): Set<string> => new Set(b.groups.filter(g => depthOf(b, g.id) === level).map(g => g.id));

/** What the board file folds to begin with (`collapsed: true`). */
export const defaultFolds = (b: Board): Set<string> => new Set(b.groups.filter(g => g.collapsed).map(g => g.id));

const HINT = 2;

/** Where a card or group shows with these groups folded: the tile of its outermost folded group, else itself. */
export function foldedInto(b: Board, folds: Set<string>): (id: string) => string {
  const groups = new Map(b.groups.map(g => [g.id, g]));
  const shut = new Set([...folds].filter(id => groups.has(id)));
  const groupOf = new Map(b.nodes.map(n => [n.id, n.group]));
  const outer = (gid: string | null | undefined): string | null => {
    let found: string | null = null;
    for (let g = gid, n = 0; g && groups.has(g) && n <= b.groups.length; g = groups.get(g)!.parent, n++) if (shut.has(g)) found = g;
    return found;
  };
  return id => (groups.has(id) ? outer(id) : outer(groupOf.get(id))) ?? id;
}

/** Counts per card added up per place they show: a folded tile carries its cards' notes and open questions. */
export function rollUp(counts: Map<string, number>, into: (id: string) => string): Map<string, number> {
  const out = new Map<string, number>();
  for (const [id, n] of counts) { const t = into(id); out.set(t, (out.get(t) ?? 0) + n); }
  return out;
}

export function fold(b: Board, folds: Set<string>): { board: Board; tiles: Map<string, Tile> } {
  const groups = new Map(b.groups.map(g => [g.id, g]));
  const shut = new Set([...folds].filter(id => groups.has(id)));
  if (!shut.size) return { board: b, tiles: new Map() };

  // the outermost folded group a group lies in (itself included), null when none
  const outer = (gid: string | null): string | null => {
    let found: string | null = null;
    for (let g = gid, n = 0; g && groups.has(g) && n <= b.groups.length; g = groups.get(g)!.parent, n++) if (shut.has(g)) found = g;
    return found;
  };
  const target = foldedInto(b, shut);

  const tiles = new Map<string, Tile>(), kept: BNode[] = [];
  for (const n of b.nodes) {
    const o = outer(n.group);
    if (!o) { kept.push(n); continue; }
    const t = tiles.get(o) ?? { count: 0, hint: [] };
    t.count++;
    if (t.hint.length < HINT) t.hint.push(n.title);
    tiles.set(o, t);
  }
  const shownGroups = b.groups.filter(g => !outer(g.id));
  const tileNodes: BNode[] = b.groups.filter(g => shut.has(g.id) && outer(g.id) === g.id).map(g => {
    tiles.set(g.id, tiles.get(g.id) ?? { count: 0, hint: [] });
    const t = tiles.get(g.id)!;
    return {
      id: g.id, title: g.label, text: t.hint.join(', ') + (t.count > t.hint.length ? ', …' : ''), kind: g.kind === 'p2' ? 'p2' : 'plain',
      group: g.parent, icon: 'none', shape: null, width: null, step: null, fields: null, live: null,
    };
  });

  const seen = new Set<string>();
  const edges = b.edges.flatMap(e => {
    const from = target(e.from), to = target(e.to);
    if (from === to) return [];
    const key = `${from}>${to}`;
    if (seen.has(key)) return [];
    seen.add(key);
    if (from === e.from && to === e.to) return [e];
    return [{ ...e, from, to, fromPort: from === e.from ? e.fromPort : undefined, toPort: to === e.to ? e.toPort : undefined }];
  });
  const ids = (xs: string[]) => [...new Set(xs.map(target))];
  return {
    board: {
      ...b, groups: shownGroups, nodes: [...kept, ...tileNodes], edges,
      steps: b.steps.map(s => ({ ...s, show: ids(s.show), hide: ids(s.hide) })),
      questions: b.questions?.map(q => (q.on ? { ...q, on: target(q.on) } : q)),
    },
    tiles,
  };
}
