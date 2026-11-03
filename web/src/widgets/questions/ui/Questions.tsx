import { useReactFlow } from '@xyflow/react';
import { CircleCheck, CircleHelp } from 'lucide-react';
import type { Board } from '@entities/board';
import { useBoardT } from '@shared/lib';
import { Glossed } from '@shared/ui';
import './questions.css';

/**
 * The board's open questions (`questions:`): what is still needed, open ones first, resolved ones struck through. One
 * about a card shows that card when clicked.
 */
export function Questions({ items }: { items: NonNullable<Board['questions']> }) {
  const flow = useReactFlow();
  const { t } = useBoardT(); // the panel goes into PNGs with the board: its language
  if (!items.length) return null;
  const open = items.filter(q => !q.done), done = items.filter(q => q.done);
  const show = (id?: string) => { if (id) void flow.fitView({ nodes: [{ id }], duration: 400, maxZoom: 1.2, padding: 0.6 }); };
  return (
    <section className="questions" aria-label={t('questions.title')}>
      <h2>{t('questions.title')} <span>{t('questions.count', { open: open.length, total: items.length })}</span></h2>
      <ul>
        {[...open, ...done].map((q, i) => (
          <li key={i} className={`${q.done ? 'done' : ''}${q.on ? ' on' : ''}`} onClick={() => show(q.on)}>
            {q.done ? <CircleCheck size={14} aria-label={t('questions.done')} /> : <CircleHelp size={14} aria-hidden />}
            <span><Glossed text={q.text} /></span>
          </li>
        ))}
      </ul>
    </section>
  );
}
