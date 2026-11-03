import { MessageSquarePlus } from 'lucide-react';
import { t, useMode } from '@shared/lib';
import { ToolButton } from '@shared/ui';

/** The toolbar's way to a note on the whole board (a card's note is a right-click on it). */
export function NoteButton({ onOpen }: { onOpen(at: { x: number; y: number }): void }) {
  const { mode } = useMode();
  const title = mode === 'share' ? t('note.commentBoard') : t('note.board');
  return <ToolButton title={title} onClick={() => onOpen({ x: innerWidth - 340, y: 64 })}><MessageSquarePlus size={16} /></ToolButton>;
}
