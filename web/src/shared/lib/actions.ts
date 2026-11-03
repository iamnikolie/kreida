import { createContext, use } from 'react';

/**
 * What a card or a group may ask of the page, without knowing the features behind it: fold or unfold a group (a group's
 * header, a folded tile) and open the note composer on a card. Absent = not on this page.
 */
export interface BoardActions {
  fold?(group: string): void;
  note?(node: string, at: { x: number; y: number }): void;
}

export const BoardActionsContext = createContext<BoardActions>({});
export const useBoardActions = () => use(BoardActionsContext);
