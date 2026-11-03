import { useCallback, useState } from 'react';
import { store, stored, useMode } from '@shared/lib';

const KEY = 'kreida.locked';

export interface ViewLock { locked: boolean; toggle(): void }

/**
 * View-only on the editor: no dragging, no editing a card's text, no undo, no relayout; pan, zoom, glossary, links and
 * notes for the agent still work. On by default (agents edit the board through its YAML, a person mostly reads it) and
 * remembered for every board on this browser. Read-only pages have nothing to lock.
 */
export function useViewLock(): ViewLock {
  const { readonly } = useMode();
  const [locked, setLocked] = useState(() => stored(KEY) !== '0');
  const toggle = useCallback(() => setLocked(v => { store(KEY, v ? '0' : '1'); return !v; }), []);
  return { locked: !readonly && locked, toggle };
}
