import { useRef, useState, type FocusEvent, type KeyboardEvent } from 'react';
import type { CardData, CardEdit } from '@entities/card';
import { useApi } from '@shared/api';
import { useEditable } from '@shared/lib';

/**
 * Double-click a card to edit its title and text in place. Enter in the title moves to the text, Enter or
 * Escape finishes; leaving the card saves what changed (the server writes it back into the board file).
 */
export function useCardEdit(id: string, data: CardData): CardEdit {
  const readonly = !useEditable();
  const api = useApi();
  const [editing, setEditing] = useState(false);
  const titleRef = useRef<HTMLDivElement>(null), descRef = useRef<HTMLDivElement>(null);

  const start = () => {
    if (readonly) return;
    setEditing(true);
    setTimeout(() => titleRef.current?.focus(), 0);
  };
  const onBlur = (e: FocusEvent) => {
    if (e.currentTarget.contains(e.relatedTarget as globalThis.Node)) return;
    setEditing(false);
    const title = titleRef.current?.innerText.trim() ?? data.title, text = descRef.current?.innerText.trim() ?? '';
    if (title !== data.title || text !== data.text) void api.patchNode(data.board, id, { title, text });
  };
  const onTitleKey = (e: KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); if (descRef.current) descRef.current.focus(); else titleRef.current?.blur(); }
    if (e.key === 'Escape') titleRef.current?.blur();
  };
  const onDescKey = (e: KeyboardEvent) => {
    if (e.key === 'Escape' || (e.key === 'Enter' && !e.shiftKey)) { e.preventDefault(); descRef.current?.blur(); }
  };

  return { editing, titleRef, descRef, start, onBlur, onTitleKey, onDescKey };
}
