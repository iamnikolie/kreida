import type { RoutedData, XY } from '@entities/edge/@x/board';

type Box = { x: number; y: number; w: number; h: number };

/** Where the line from a box's centre towards `to` leaves the box. */
function exit(b: Box, to: XY): XY {
  const c = { x: b.x + b.w / 2, y: b.y + b.h / 2 }, dx = to.x - c.x, dy = to.y - c.y;
  const k = Math.min(Math.abs(dx) > 1e-9 ? b.w / 2 / Math.abs(dx) : Infinity, Math.abs(dy) > 1e-9 ? b.h / 2 / Math.abs(dy) : Infinity, 1);
  return { x: c.x + dx * k, y: c.y + dy * k };
}

/** A straight route between two placed cards (absolute boxes), from border to border; the label sits halfway. */
export function straight(a: Box, b: Box): RoutedData {
  const ca = { x: a.x + a.w / 2, y: a.y + a.h / 2 }, cb = { x: b.x + b.w / 2, y: b.y + b.h / 2 };
  const p = exit(a, cb), q = exit(b, ca);
  return { points: [p, q], label: { x: (p.x + q.x) / 2, y: (p.y + q.y) / 2 }, src: { x: a.x, y: a.y }, dst: { x: b.x, y: b.y } };
}
