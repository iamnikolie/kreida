import type { FocusEvent, KeyboardEvent, RefObject } from 'react';
import type { BNode, Dir, LiveVal } from '@shared/api';

/** What the layout and the canvas put into a card node's `data`. */
export type CardData = BNode & {
  dir: Dir; board: string; liveVal?: LiveVal; sketch?: boolean;
  /** Laid out for a phone (`size(n, { phone })`): a table card draws the phone's geometry. */
  phone?: boolean;
  /** The lane's name, shown above the title on a phone. */
  lane?: string;
  /** A radial mindmap's branch: the colour family of the root's child it hangs from. */
  branch?: number;
  /** The tile a folded group becomes: how many cards it holds and the first titles as a hint. */
  fold?: { count: number; hint: string[] };
  /** Open questions of the board about this card. */
  asks?: number;
  /** Open notes for the agent on this card. */
  notes?: number;
};

/** Inline editing of a card's text, driven from outside (features/edit-card). A card without it is read-only. */
export interface CardEdit {
  editing: boolean;
  titleRef: RefObject<HTMLDivElement | null>;
  descRef: RefObject<HTMLDivElement | null>;
  start(): void;
  onBlur(e: FocusEvent): void;
  onTitleKey(e: KeyboardEvent): void;
  onDescKey(e: KeyboardEvent): void;
}
