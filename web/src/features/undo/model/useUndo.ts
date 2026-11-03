import { useCallback } from 'react';
import { useApi } from '@shared/api';
import { t, useEditable } from '@shared/lib';

/** Steps the open board back one saved version (every save is kept). Reports a refusal through `onError`: the server's text, or ours when it sent none. */
export function useUndo(board: string | null, onError: (message: string) => void) {
  const readonly = !useEditable();
  const api = useApi();
  return useCallback(async () => {
    if (!board || readonly) return;
    const error = await api.undo(board);
    if (error !== null) onError(error || t('error.undo'));
  }, [api, board, readonly, onError]);
}
