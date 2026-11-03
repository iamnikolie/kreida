import { useCallback, type RefObject } from 'react';
import type { Node } from '@xyflow/react';
import { useApi, type Pos } from '@shared/api';
import { useEditable } from '@shared/lib';

/**
 * Dragging a card keeps it where it was dropped: `pos` is the board's saved positions (kept by the canvas,
 * which also lays the board out with them); each drop updates it and saves the lot.
 */
export function useMoveCard(board: string | null, pos: RefObject<Pos>) {
  const readonly = !useEditable();
  const api = useApi();
  return useCallback((_: unknown, __: Node, dragged: Node[]) => {
    if (readonly || !board) return;
    for (const n of dragged) pos.current[n.id] = { x: Math.round(n.position.x), y: Math.round(n.position.y) };
    void api.savePos(board, pos.current);
  }, [api, readonly, board, pos]);
}
