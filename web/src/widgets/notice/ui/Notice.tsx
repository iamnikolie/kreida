import { Panel } from '@xyflow/react';
import { t, type Key } from '@shared/lib';
import './notice.css';

// What the local server answers, in English, for a link that is gone: the page says it in its own language.
// Any other text shows as it came: kreida cloud words its answers in the link's language, a broken board names its line.
const KNOWN: Record<string, Key> = { 'link expired': 'notice.expired' };

/** Top centre: an error (the board's parse error or a message from an action) and the editor's empty-state hint. */
export function Notice({ error, empty }: { error: string; empty: boolean }) {
  return (
    <>
      {error && <Panel position="top-center" className="err">{KNOWN[error] ? t(KNOWN[error]) : error}</Panel>}
      {empty && <Panel position="top-center" className="empty">{t('notice.empty')} <code>kreida skill</code></Panel>}
    </>
  );
}
