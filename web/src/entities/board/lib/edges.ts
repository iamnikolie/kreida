import { MarkerType, type Edge } from '@xyflow/react';
import type { Board } from '@shared/api';
import type { RoutedData } from '@entities/edge/@x/board';

// Edges follow the route ELK computed (no crossing through cards); lanes and unrouted edges draw as smoothstep.
export function edgesOf(visible: Board['edges'], routes?: Map<string, RoutedData>): Edge[] {
  return visible.map(e => ({
    id: e.id, source: e.from, target: e.to, label: e.label || undefined, animated: e.animated,
    sourceHandle: e.fromPort ? `out:${e.fromPort}` : undefined, targetHandle: e.toPort ? `in:${e.toPort}` : undefined,
    type: 'routed', data: { ...routes?.get(e.id), dashed: e.dashed },
    className: [e.dashed && 'dashed', e.thick && 'thick'].filter(Boolean).join(' '),
    markerEnd: { type: MarkerType.ArrowClosed, width: 18, height: 18 },
    labelBgPadding: [9, 4] as [number, number], labelBgBorderRadius: 999,
  }));
}
