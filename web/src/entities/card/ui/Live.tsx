import type { BLive, LiveVal } from '@shared/api';
import { formatDateTime, pageLocale, t } from '@shared/lib';
import { tone } from '../lib/tone';

// The value under a live card. Blocked sources say so — the board never runs a command silently.
export function Live({ live, v, board }: { live: BLive; v?: LiveVal; board: string }) {
  const tip = v?.at ? [live.source.replace(/^(cmd|url):/, ''), formatDateTime(v.at, pageLocale())].filter(Boolean).join(' · ') : live.source;
  if (v?.blocked) return <div className="live bad" title={`kreida live ${board} --allow`}><i />{t('live.blocked')}</div>;
  if (v?.error) return <div className="live bad" title={`${tip}\n${v.error}`}><i />{t('live.error')}</div>;
  const val = v?.value ?? '…';
  return <div className={`live ${tone(val)}`} title={tip}><i />{live.label && <span className="ll">{live.label}</span>}{' '}<b>{val}</b></div>;
}
