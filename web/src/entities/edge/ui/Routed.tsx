import { memo } from 'react';
import { BaseEdge, getSmoothStepPath, useInternalNode, type Edge, type EdgeProps } from '@xyflow/react';
import { roughEdge } from '@shared/lib';
import { rounded, sitsAt } from '../lib/route';
import type { RoutedData } from '../model/types';
import './edge.css';

// The route ELK computed, as long as both ends sit where ELK put them; a dragged card (or a lanes board,
// which has no routes) falls back to smoothstep. Sketch style redraws the line with rough.js.
export const Routed = memo(function Routed(p: EdgeProps<Edge<RoutedData>>) {
  const s = useInternalNode(p.source), t = useInternalNode(p.target), d = p.data ?? {};
  const label = {
    label: p.label, labelStyle: p.labelStyle, labelShowBg: p.labelShowBg, labelBgStyle: p.labelBgStyle,
    labelBgPadding: p.labelBgPadding, labelBgBorderRadius: p.labelBgBorderRadius,
  };
  let path: string, x: number, y: number;
  if (d.points && sitsAt(s, d.src) && sitsAt(t, d.dst)) {
    const mid = d.label ?? d.points[Math.floor(d.points.length / 2)];
    [path, x, y] = [rounded(d.points, 10), mid.x, mid.y];
  } else [path, x, y] = getSmoothStepPath({ ...p, borderRadius: 16, offset: 22 });
  if (!d.sketch) return <BaseEdge id={p.id} path={path} markerEnd={p.markerEnd} style={p.style} labelX={x} labelY={y} {...label} />;
  return (
    <>
      {roughEdge(path, p.id, !!d.dashed).map((r, i) => <path key={i} className="rough-edge" d={r} />)}
      <BaseEdge id={p.id} path={path} markerEnd={p.markerEnd} style={{ ...p.style, stroke: 'transparent' }} labelX={x} labelY={y} {...label} />
    </>
  );
});
