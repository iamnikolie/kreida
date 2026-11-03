import { useCallback, useEffect, useMemo, useState } from 'react';
import type { BoardResp } from '@entities/board';
import { useApi, type Note } from '@shared/api';
import { useMode } from '@shared/lib';

export interface Notes {
  /** The page takes notes: the editor, or a kreida cloud link that takes comments. */
  can: boolean;
  /** The notes the page shows: the board's open ones (the editor), the holder's own thread (a cloud link). */
  list: Note[];
  /** Open notes per card id ('' = the whole board). */
  open: Map<string, number>;
  /** Writes one; resolves to an error text, or null. */
  add(node: string, text: string): Promise<string | null>;
}

/**
 * Notes for the agent. A note never touches the board file: the server keeps it next to the board and the agent reads it
 * with `kreida inbox`. Re-read whenever the board's response changes (the server's event for a new note brings one).
 * A headless render shows none.
 */
export function useNotes(name: string | null, resp: BoardResp | null): Notes {
  const { mode, capture } = useMode();
  const api = useApi();
  const can = !capture && !!name && (mode === 'editor' || (mode === 'share' && resp?.comments === true));
  const [list, setList] = useState<Note[]>([]);
  useEffect(() => {
    if (!can || !name) { setList([]); return; }
    let on = true;
    api.notes(name).then(ns => { if (on) setList(Array.isArray(ns) ? ns : []); }).catch(() => {});
    return () => { on = false; };
  }, [api, can, name, resp]);
  const open = useMemo(() => {
    const m = new Map<string, number>();
    for (const n of list) if (!n.resolved) m.set(n.node, (m.get(n.node) ?? 0) + 1);
    return m;
  }, [list]);
  const add = useCallback(async (node: string, text: string) => {
    if (!name) return null;
    const err = await api.addNote(name, { node, text });
    if (err == null) setList(await api.notes(name).catch(() => list));
    return err;
  }, [api, name, list]);
  return { can, list, open, add };
}
