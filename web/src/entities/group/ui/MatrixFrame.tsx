import { memo } from 'react';
import type { NodeProps } from '@xyflow/react';
import { Glossed } from '@shared/ui';
import './matrix.css';

interface Axis { label: string; low: string; high: string }
interface Quad { x: number; y: number; w: number; h: number }
export type MatrixFrameData = { matrix: { x: Axis; y: Axis; hot?: string; quadrants: Record<string, string> }; quads: Record<string, Quad>; axis: number; foot: number };

/** The 2×2 of `layout: matrix`: four quadrants (the hot one tinted, each with its label), y up the left, x along the foot. */
export const MatrixFrame = memo(function MatrixFrame({ data }: NodeProps & { data: MatrixFrameData }) {
  const { matrix: m, quads, axis, foot } = data;
  const q = quads['low-high'];
  return (
    <div className="matrix">
      {Object.entries(quads).map(([c, r]) => (
        <div key={c} className={`mq${m.hot === c ? ' hot' : ''}`} style={{ left: r.x, top: r.y, width: r.w, height: r.h }}>
          {m.quadrants[c] && <div className="mql"><Glossed text={m.quadrants[c]} /></div>}
        </div>
      ))}
      <div className="max y" style={{ width: axis, top: 0, height: 2 * q.h }}>
        <span className="hi">{m.y.high}</span><span className="lbl">{m.y.label}</span><span className="lo">{m.y.low}</span>
      </div>
      <div className="max x" style={{ left: axis, height: foot, top: 2 * q.h, width: 2 * q.w }}>
        <span className="lo">{m.x.low}</span><span className="lbl">{m.x.label}</span><span className="hi">{m.x.high}</span>
      </div>
    </div>
  );
});
