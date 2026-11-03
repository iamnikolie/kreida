import type { Board, BStep } from '@shared/api';

/** True for an item (node or group id) that is hidden at the current step. */
export type Fog = (id: string) => boolean;

// Presentation steps: the board's `steps:`, else its top-level items (groups, then loose nodes) one by one.
export function stepsOf(b: Board): BStep[] {
  if (b.steps.length) return b.steps;
  const top = b.groups.filter(g => !g.parent).map(g => g.id);
  const loose = b.nodes.filter(n => !n.group || !b.groups.some(g => g.id === n.group)).map(n => n.id);
  return [...top, ...loose].map(id => ({ title: '', show: [id], hide: [] }));
}

// Visible at step i: anything a step shows starts hidden; steps 0..i show/hide in order; a node without
// its own state follows its nearest group; never-mentioned items are always visible.
export function visibleAt(b: Board, steps: BStep[], i: number) {
  const parent = new Map<string, string | null>([
    ...b.groups.map(g => [g.id, g.parent] as [string, string | null]),
    ...b.nodes.map(n => [n.id, n.group] as [string, string | null]),
  ]);
  const state = new Map<string, boolean>();
  for (const s of steps) for (const id of s.show) state.set(id, false);
  for (const s of steps.slice(0, i + 1)) { for (const id of s.show) state.set(id, true); for (const id of s.hide) state.set(id, false); }
  return (id: string) => { for (let c: string | null | undefined = id; c; c = parent.get(c)) if (state.has(c)) return state.get(c)!; return true; };
}

/** What a presentation at `step` hides (null while no step is selected: everything shows). */
export function fogOf(b: Board, steps: BStep[], step: number | null): Fog | null {
  if (step == null) return null;
  const on = visibleAt(b, steps, step);
  return id => !on(id);
}

/**
 * What step `step` brings into view: items visible at it that were hidden at the step before. A step that brings
 * nothing new (it only hides) brings everything that shows. Null while no step is selected. A phone, too narrow
 * for all that is on show, looks at this part.
 */
export function revealAt(b: Board, steps: BStep[], step: number | null): ((id: string) => boolean) | null {
  if (step == null) return null;
  const now = visibleAt(b, steps, step), before = visibleAt(b, steps, step - 1);
  const fresh = new Set([...b.groups.map(g => g.id), ...b.nodes.map(n => n.id)].filter(id => now(id) && !before(id)));
  return fresh.size ? id => fresh.has(id) : now;
}
