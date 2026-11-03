import type { Board } from '@entities/board';
import './legend.css';

/** The board's colour key: one chip per card kind it uses. */
export function Legend({ items }: { items: Board['legend'] }) {
  if (!items.length) return null;
  return (
    <div className="legend">
      {items.map(l => <span key={l.kind} className={`chip k-${l.kind}`}><i />{l.label}</span>)}
    </div>
  );
}
