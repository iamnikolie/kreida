import { Position } from '@xyflow/react';
import type { Dir } from '../api/contract';

/** Where a flow in this direction enters (first) and leaves (second) a node. */
export const SIDES: Record<Dir, [Position, Position]> = {
  LR: [Position.Left, Position.Right], RL: [Position.Right, Position.Left],
  TB: [Position.Top, Position.Bottom], BT: [Position.Bottom, Position.Top],
};
