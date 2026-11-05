import { memo, type MouseEvent } from 'react';
import { Handle, type NodeProps } from '@xyflow/react';
import { MessageSquare } from 'lucide-react';
import { SIDES, t, useBoardActions, useBoardT } from '@shared/lib';
import { Glossed, Markdown, RoughBox } from '@shared/ui';
import { iconFor } from '../lib/icon';
import type { CardData, CardEdit } from '../model/types';
import { CardImage } from './CardImage';
import { Grid } from './Grid';
import { Live } from './Live';
import { Table } from './Table';
import './card.css';

/** The card node. Pass `edit` to make its text editable in place; without it the card is read-only. */
export const Card = memo(function Card({ id, data, width = 0, height = 0, edit }: NodeProps & { data: CardData; edit?: CardEdit }) {
  const actions = useBoardActions();
  const bt = useBoardT(); // what the card says of itself goes into exports: the board's language
  const [inPos, outPos] = SIDES[data.dir];
  const Icon = iconFor(data.icon, data.title);
  const rough = data.sketch && data.kind !== 'note' && width > 0 &&
    <RoughBox w={width} h={height} r={data.kind === 'who' || data.shape === 'pill' ? height / 2 : 14} id={id} dashed={data.kind === 'p2' || !!data.fold} />;
  // right-click: a note for the agent about this card (where the page takes notes)
  const onContextMenu = actions.note && ((e: MouseEvent) => { e.preventDefault(); actions.note!(id, { x: e.clientX, y: e.clientY }); });
  const badges = (
    <>
      {!!data.asks && <span className="badge ask" title={bt.tn('card.asks', data.asks)}>?</span>}
      {!!data.notes && <span className="badge notes" title={bt.tn('card.notes', data.notes)}><MessageSquare size={11} strokeWidth={2.4} />{data.notes}</span>}
    </>
  );
  if (data.fields) return <div className="cardwrap" onContextMenu={onContextMenu}><Table data={data} Icon={Icon} rough={rough} />{badges}</div>;
  if (data.rows) return <div className="cardwrap" onContextMenu={onContextMenu}><Grid data={data} Icon={Icon} rough={rough} />{badges}</div>;

  if (data.fold) {
    return (
      <div className={`card fold k-${data.kind}`} onClick={() => actions.fold?.(id)} onContextMenu={onContextMenu}
        title={bt.t('fold.open')} role="button">
        {rough}
        <Handle type="target" position={inPos} />
        <div className="body">
          <div className="t">{data.title}</div>
          <div className="cnt">{bt.tn('fold.count', data.fold.count)}</div>
          {data.fold.hint.length > 0 && <div className="d">{data.fold.hint.join(', ')}{data.fold.count > data.fold.hint.length ? ', …' : ''}</div>}
        </div>
        <Handle type="source" position={outPos} />
        {badges}
      </div>
    );
  }

  const editing = !!edit?.editing;
  const md = data.kind === 'note' && !editing && !!data.text;
  return (
    <div className={`card k-${data.kind} ${data.shape ? `s-${data.shape}` : ''} ${data.branch != null ? `b-${data.branch % 5}` : ''} ${editing ? 'editing nodrag' : ''} ${data.image ? 'has-image' : ''}`}
      onDoubleClick={edit?.start} onBlur={edit?.onBlur} onContextMenu={onContextMenu}>
      {rough}
      <Handle type="target" position={inPos} />
      {data.image && <CardImage image={data.image} board={data.board} width={width} />}
      <div className="row">
        {Icon && <Icon className="ic" size={18} strokeWidth={1.8} />}
        <div className="body">
          {data.lane && <div className="cap">{data.lane}</div>}
          <div className="t" ref={edit?.titleRef} contentEditable={editing} suppressContentEditableWarning onKeyDown={edit?.onTitleKey}>
            {editing ? data.title : <Glossed text={data.title} />}
          </div>
          {md && <Markdown text={data.text} />}
          {!md && (data.text || editing) && (
            <div className="d" ref={edit?.descRef} data-placeholder={t('card.description')} contentEditable={editing} suppressContentEditableWarning onKeyDown={edit?.onDescKey}>
              {editing ? data.text : <Glossed text={data.text} />}
            </div>
          )}
          {data.live && <Live live={data.live} v={data.liveVal} board={data.board} />}
        </div>
      </div>
      <Handle type="source" position={outPos} />
      {badges}
    </div>
  );
});
