import type { Ref } from 'react';
import { Panel } from '@xyflow/react';
import { BoardSwitch, type BoardSwitchProps } from '@features/board-switch';
import type { Board } from '@entities/board';
import { t, useMode } from '@shared/lib';
import './head.css';

interface Props {
  board: Board | undefined;
  /** The owner's name on a kreida cloud link, '' when there is none to show. */
  owner: string;
  /** Presenting: the brand goes, the title and the owner's name stay. */
  presenting: boolean;
  switcher: BoardSwitchProps;
  /** The panel, for the page to measure where the head ends (a fit keeps the board below it). */
  ref?: Ref<HTMLDivElement>;
}

/** Top left: the logo with the board switch (editor only), then the board's title and subtitle and, under them, whose board it is. */
export function Head({ board, owner, presenting, switcher, ref }: Props) {
  const { readonly } = useMode();
  return (
    <Panel ref={ref} position="top-left" className={`head${owner ? ' owned' : ''}`}>
      {!presenting && !readonly && (
        <div className="brand">
          <span className="logo">kreida</span>
          <BoardSwitch {...switcher} />
        </div>
      )}
      {board?.title && <h1>{board.title}</h1>}
      {board?.sub && <div className="sub">{board.sub}</div>}
      {owner && <div className="from">{t('head.from', { name: owner })}</div>}
    </Panel>
  );
}
