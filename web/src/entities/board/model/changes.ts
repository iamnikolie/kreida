import type { Board } from '@shared/api';

/** A fingerprint per card: what a reader sees of it (not its position or edges). */
export type Signatures = Map<string, string>;

export const signatures = (b: Board): Signatures =>
  new Map(b.nodes.map(n => [n.id, JSON.stringify([n.title, n.text, n.kind, n.group, n.icon, n.fields])]));

/**
 * The cards that differ between two versions of the same board, so the page can make them glow.
 * Null when nothing changed, or when everything did (a different board, not an edit).
 */
export function changedCards(before: Signatures, now: Signatures): Set<string> | null {
  const changed = new Set([...now].filter(([id, s]) => before.get(id) !== s).map(([id]) => id));
  return changed.size && changed.size < now.size ? changed : null;
}
