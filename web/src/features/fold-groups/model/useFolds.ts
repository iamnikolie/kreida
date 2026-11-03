import { useCallback, useMemo, useState } from 'react';
import { defaultFolds, depthOf, foldLevel, type Board } from '@entities/board';
import { store, stored } from '@shared/lib';

const key = (board: string) => `kreida.fold.${board}`;

export interface Folds {
  /** The folded groups. */
  folds: Set<string>;
  /** How deep the board's groups nest (0: no groups to fold). */
  depth: number;
  toggle(group: string): void;
  /** Fold every group `n` deep (and so what is inside them). */
  level(n: number): void;
  unfoldAll(): void;
}

/**
 * Which groups are folded into tiles: a view state, never a board edit, so it works on read-only pages and in view-only
 * mode alike. Remembered per board on this browser; a board nobody folded yet starts as its file says (`collapsed: true`).
 */
export function useFolds(name: string | null, board: Board | undefined): Folds {
  const [mine, setMine] = useState<{ name: string | null; folds: Set<string> | null }>({ name: null, folds: null });
  const saved = useMemo(() => {
    if (!name) return null;
    try { const v = stored(key(name)); return v ? new Set(JSON.parse(v) as string[]) : null; } catch { return null; }
  }, [name]);
  const folds = useMemo(() => (mine.name === name && mine.folds) || saved || (board ? defaultFolds(board) : new Set<string>()),
    [mine, name, saved, board]);
  const depth = useMemo(() => (board && board.layout === 'auto' ? Math.max(0, ...board.groups.map(g => depthOf(board, g.id))) : 0), [board]);

  const set = useCallback((next: Set<string>) => {
    setMine({ name, folds: next });
    if (name) store(key(name), JSON.stringify([...next]));
  }, [name]);
  const toggle = useCallback((g: string) => { const next = new Set(folds); if (!next.delete(g)) next.add(g); set(next); }, [folds, set]);
  const level = useCallback((n: number) => { if (board) set(foldLevel(board, n)); }, [board, set]);
  const unfoldAll = useCallback(() => set(new Set()), [set]);
  return { folds, depth, toggle, level, unfoldAll };
}
