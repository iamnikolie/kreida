import { Lock, LockOpen } from 'lucide-react';
import { t } from '@shared/lib';
import type { ViewLock } from '../model/useViewLock';

/** The lock with its word: «Просмотр» while the canvas changes nothing, «Правка» while it does. */
export function LockButton({ lock }: { lock: ViewLock }) {
  return (
    <button className={`lock${lock.locked ? ' on' : ''}`} title={t('lock.title')} aria-pressed={lock.locked} onClick={lock.toggle}>
      {lock.locked ? <Lock size={14} /> : <LockOpen size={14} />}
      <span>{lock.locked ? t('lock.view') : t('lock.edit')}</span>
    </button>
  );
}
