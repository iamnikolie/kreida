import { useState } from 'react';
import { useApi, type BoardItem } from '@shared/api';
import { t } from '@shared/lib';
import './board-switch.css';

const NAME = /^[\w.-]+$/;

export interface BoardSwitchProps {
  list: BoardItem[];
  /** The open board. */
  name: string | null;
  onSelect(name: string): void;
  /** Refreshes `list` (after a board was created). */
  reload(): Promise<void>;
  onError(message: string): void;
}

/** Pick another board, or create one: the last option turns the select into a name field. */
export function BoardSwitch({ list, name, onSelect, reload, onError }: BoardSwitchProps) {
  const api = useApi();
  const [creating, setCreating] = useState(false);

  const create = async (n: string) => {
    setCreating(false);
    if (!NAME.test(n)) { if (n) onError(t('switch.badName')); return; }
    const error = await api.create(n);
    if (error !== null) onError(error); else { await reload(); onSelect(n); }
  };

  return creating ? (
    <input className="board-switch" autoFocus aria-label={t('switch.label')} placeholder={t('switch.name')} onBlur={e => void create(e.target.value.trim())}
      onKeyDown={e => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); if (e.key === 'Escape') setCreating(false); }} />
  ) : (
    <select className="board-switch" aria-label={t('switch.label')} value={name ?? ''} onChange={e => (e.target.value === '__new' ? setCreating(true) : onSelect(e.target.value))}>
      {list.map(x => <option key={x.name} value={x.name}>{x.title || x.name}</option>)}
      <option value="__new">{t('switch.new')}</option>
    </select>
  );
}
