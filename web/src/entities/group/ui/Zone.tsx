import { memo } from 'react';
import { Handle, type NodeProps } from '@xyflow/react';
import { ChevronDown } from 'lucide-react';
import type { Dir } from '@shared/api';
import { SIDES, useBoardActions, useBoardT } from '@shared/lib';
import { RoughBox } from '@shared/ui';
import './zone.css';

export type ZoneData = { label: string; kind: string | null; dir: Dir; pdir: Dir; lane?: boolean; sketch?: boolean; /** Its header folds it into a tile. */ foldable?: boolean };

export const Zone = memo(function Zone({ id, data, width = 0, height = 0 }: NodeProps & { data: ZoneData }) {
  const [inPos, outPos] = SIDES[data.pdir]; // edges to a group come from its parent's flow
  const { fold } = useBoardActions();
  const { t } = useBoardT();
  const folds = data.foldable && !data.lane && fold;
  return (
    <div className={`zone ${data.kind ? `k-${data.kind}` : ''} ${data.lane ? 'lane' : ''}`}>
      {data.sketch && width > 0 && <RoughBox w={width} h={height} r={data.lane ? 16 : 22} id={id} dashed={data.kind === 'p2'} />}
      <Handle type="target" position={inPos} />
      {folds
        ? <button className="zl fold nodrag" title={t('fold.close')} onClick={() => folds(id)}>{data.label}<ChevronDown size={13} strokeWidth={2.4} /></button>
        : <div className="zl">{data.label}</div>}
      <Handle type="source" position={outPos} />
    </div>
  );
});
