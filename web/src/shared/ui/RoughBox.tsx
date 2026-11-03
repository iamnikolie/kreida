import { memo, useMemo } from 'react';
import { roughBox } from '../lib/rough';
import './rough.css';

// Outline over a card/zone box; stroke colour comes from the box's --kc.
export const RoughBox = memo(function RoughBox({ w, h, r, id, dashed = false }: { w: number; h: number; r: number; id: string; dashed?: boolean }) {
  const ds = useMemo(() => roughBox(w, h, r, id, dashed), [w, h, r, id, dashed]);
  return (
    <svg className="rough" width={w} height={h} viewBox={`0 0 ${w} ${h}`} aria-hidden>
      {ds.map((d, i) => <path key={i} d={d} />)}
    </svg>
  );
});
