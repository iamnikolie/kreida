import { useCallback } from 'react';
import { useApi } from '@shared/api';

/** Forgets the hand-placed card positions, so the board lays itself out again. */
export function useRelayout(board: string | null) {
  const api = useApi();
  return useCallback(() => { if (board) void api.resetPos(board); }, [api, board]);
}
