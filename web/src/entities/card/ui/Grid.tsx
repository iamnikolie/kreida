import type { CSSProperties, ReactNode } from 'react';
import { Handle } from '@xyflow/react';
import { SIDES } from '@shared/lib';
import { Glossed } from '@shared/ui';
import { PHONE_TABLE_MAX } from '../lib/geometry';
import { gridGeom, toneOf } from '../lib/grid';
import type { IconC } from '../lib/icon';
import type { CardData } from '../model/types';

// A table card (cols/rows): the title, then a header row and the rows, at the widths and heights the layout reserved.
// The first cell of a row names it; a scored cell is tinted from the table's worst (red) to its best (green).
// On a phone the heights are a floor: a row takes what its text needs there, and the card grows with it. A table wider
// than the card scrolls sideways inside it, its first column held in place.
export function Grid({ data, Icon, rough }: { data: CardData; Icon: IconC | null; rough: ReactNode }) {
  const phone = !!data.phone;
  const [inPos, outPos] = SIDES[data.dir];
  const g = gridGeom(data, phone ? { view: PHONE_TABLE_MAX } : {});
  const scroll = g.inner > g.width;
  const scores = (data.rows ?? []).flat().flatMap(c => (c.score == null ? [] : [c.score]));
  const row = (h: number): CSSProperties => ({
    gridTemplateColumns: g.cols.map(w => `${w}px`).join(' '),
    ...(phone ? { minHeight: h } : { height: h }),
    ...(scroll ? { width: g.inner } : {}),
  });
  return (
    <div className={`card grid k-${data.kind}`}>
      {rough}
      <Handle type="target" position={inPos} />
      <Handle type="source" position={outPos} />
      <div className="th" style={phone ? { minHeight: g.head } : { height: g.head }}>
        {Icon && <Icon className="ic" size={16} strokeWidth={1.8} />}
        <div className="body"><div className="t"><Glossed text={data.title} /></div>{data.text && <div className="d"><Glossed text={data.text} /></div>}</div>
      </div>
      {/* a sideways swipe scrolls the table, not the canvas */}
      <div className={scroll ? 'gs scroll nopan nodrag' : 'gs'}>
        {data.cols && (
          <div className="gr gh" style={row(g.header)}>
            {g.cols.map((_, i) => <div key={i} className="gc"><Glossed text={data.cols![i] ?? ''} /></div>)}
          </div>
        )}
        {(data.rows ?? []).map((r, j) => (
          <div key={j} className="gr" style={row(g.rows[j])}>
            {r.map((c, i) => {
              const tone = toneOf(c.score, scores);
              return <div key={i} className={`gc${tone == null ? '' : ` s${tone}`}`}><Glossed text={c.text} /></div>;
            })}
          </div>
        ))}
      </div>
    </div>
  );
}
