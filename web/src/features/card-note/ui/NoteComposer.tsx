import { useState, type KeyboardEvent } from 'react';
import { formatDateTime, pageLocale, t, useHotkeys, useMode } from '@shared/lib';
import type { Notes } from '../model/useNotes';
import './note-composer.css';

/** Where the composer opens and what about: a card's id and title ('' for the whole board). */
export interface NoteTarget { node: string; label?: string; x: number; y: number }

const W = 320;

/**
 * The composer: a note for the agent about one card (right-click it) or the whole board. Shows that card's thread first,
 * as plain text. ⌘Enter sends, Escape closes. On a kreida cloud link the same box takes the client's comments.
 */
export function NoteComposer({ notes, target, onClose, onSent }: { notes: Notes; target: NoteTarget; onClose(): void; onSent(msg: string): void }) {
  const { mode } = useMode();
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);
  useHotkeys([{ keys: ['Escape'], run: onClose }]);
  const client = mode === 'share';
  const title = target.node
    ? `${client ? t('note.comment') : t('note.card')} · ${target.label || target.node}`
    : client ? t('note.commentBoard') : t('note.board');
  const thread = notes.list.filter(n => n.node === target.node);
  const send = async () => {
    if (!text.trim() || busy) return;
    setBusy(true);
    const err = await notes.add(target.node, text.trim()).catch(() => '');
    setBusy(false);
    if (err == null) { onSent(t('note.sent')); onClose(); } else onSent(err || t('note.failed'));
  };
  const onKey = (e: KeyboardEvent) => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); void send(); } };
  const left = Math.max(8, Math.min(target.x, innerWidth - W - 8)), top = Math.max(8, Math.min(target.y, innerHeight - 300));
  return (
    <>
      <button className="note-backdrop" aria-label={t('note.cancel')} onClick={onClose} />
      <div className="note-box" role="dialog" aria-label={title} style={{ left, top, width: W }}>
        <h2>{title}</h2>
        {thread.length > 0 && (
          <ol className="thread">
            {thread.map(n => (
              <li key={n.id} className={n.resolved ? 'done' : ''}>
                <p>{n.text}</p>
                <small>{n.author || t('note.you')} · {formatDateTime(n.created, pageLocale())}{n.resolved ? ` · ${t('note.done')}` : ''}</small>
              </li>
            ))}
          </ol>
        )}
        <textarea autoFocus value={text} maxLength={2000} placeholder={t('note.placeholder')} onChange={e => setText(e.target.value)} onKeyDown={onKey} />
        <div className="acts">
          <button onClick={onClose}>{t('note.cancel')}</button>
          <button className="go" disabled={!text.trim() || busy} onClick={() => void send()}>{t('note.send')}</button>
        </div>
      </div>
    </>
  );
}
