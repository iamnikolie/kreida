import { createContext, use } from 'react';
import { useMode } from './mode';

/** View-only on the editor: the canvas changes nothing in the board (features/view-lock holds the switch). */
export const LockContext = createContext(false);

/** Whether the canvas may change the board: the editor, not locked to view-only. */
export function useEditable(): boolean {
  const { readonly } = useMode();
  return !readonly && !use(LockContext);
}
