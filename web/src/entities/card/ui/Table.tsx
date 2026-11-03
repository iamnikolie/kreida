import type { ReactNode } from 'react';
import { Handle } from '@xyflow/react';
import { SIDES } from '@shared/lib';
import { ROW, headH, rowSides } from '../lib/geometry';
import type { IconC } from '../lib/icon';
import type { CardData } from '../model/types';

// A node with fields: header + one row per field; FK edges attach to their row's handle.
export function Table({ data, Icon, rough }: { data: CardData; Icon: IconC | null; rough: ReactNode }) {
  const [inPos, outPos] = SIDES[data.dir];
  const [rin, rout] = rowSides(data.dir);
  const head = headH(data), y = (i: number) => ({ top: head + i * ROW + ROW / 2 });
  return (
    <div className={`card table k-${data.kind}`}>
      {rough}
      <Handle type="target" position={inPos} />
      <Handle type="source" position={outPos} />
      <div className="th" style={{ height: head }}>
        {Icon && <Icon className="ic" size={16} strokeWidth={1.8} />}
        <div className="body"><div className="t">{data.title}</div>{data.text && <div className="d">{data.text}</div>}</div>
      </div>
      {data.fields!.map((f, i) => (
        <div key={f.name} className={`tr${f.name === 'id' ? ' pk' : ''}${f.ref ? ' fk' : ''}`}>
          <span className="fn">{f.name}</span>
          {f.note && <span className="fnote">{f.note}</span>}
          {f.ref && <span className="fr">→ {f.ref}</span>}
          <Handle type="target" id={`in:${f.name}`} position={rin} style={y(i)} />
          {f.name === 'id' && <Handle type="source" id={`out:${f.name}`} position={rout} style={y(i)} />}
        </div>
      ))}
    </div>
  );
}
