import { useCallback, useState } from 'react';
import { useApi, type BoardStyle } from '@shared/api';
import { useEditable } from '@shared/lib';

export interface Sketch { on: boolean; toggle(): void }

/**
 * Hand-drawn style. It lives in the board file (`style: sketch`) so PNG, export and share links get it too:
 * in the editor the toggle saves it and the file's change comes back as the new `style`.
 * Where nothing can be saved (read-only pages, view-only) it flips locally instead.
 */
export function useSketch(board: string | null, style: BoardStyle | undefined): Sketch {
  const readonly = !useEditable();
  const api = useApi();
  const [local, setLocal] = useState<boolean | null>(null);
  const on = local ?? style === 'sketch';
  const toggle = useCallback(() => {
    if (readonly || !board) { setLocal(!on); return; }
    void api.setStyle(board, on ? 'clean' : 'sketch');
  }, [api, readonly, board, on]);
  return { on, toggle };
}
