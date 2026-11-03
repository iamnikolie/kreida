import { useMemo, type ReactNode } from 'react';
import { Background, BackgroundVariant, Controls, MiniMap, ReactFlow } from '@xyflow/react';
import { MatrixFrame, Zone } from '@entities/group';
import { Routed } from '@entities/edge';
import type { BoardResp } from '@entities/board';
import { EditableCard } from '@features/edit-card';
import { useOpenStep, useStepFit } from '@features/fit-view';
import { useLiveValues } from '@features/live-poll';
import { useMoveCard } from '@features/move-card';
import type { Presentation } from '@features/present';
import { colorModeOf } from '@features/theme';
import { useBoardActions, useEditable, useMedia, useMode, usePhone } from '@shared/lib';
import { decorateEdges, decorateNodes, type Marks } from '../lib/decorate';
import { useGraph } from '../model/useGraph';
import './canvas.css';

const nodeTypes = { card: EditableCard, zone: Zone, matrix: MatrixFrame };
const edgeTypes = { routed: Routed };

interface Props {
  resp: BoardResp | null;
  /** The open board's name. */
  name: string | null;
  theme: string;
  sketch: boolean;
  presentation: Presentation;
  minimap: boolean;
  /** The groups folded into tiles (a view state). */
  folds: Set<string>;
  /** Badges: open questions and notes per card. */
  marks: Marks;
  onError(message: string): void;
  /** Panels laid over the canvas (head, toolbar, steps...). */
  children: ReactNode;
}

/** The board itself: React Flow with its graph, live values, changed-card glow, presentation fog and the sketch look. */
export function Canvas({ resp, name, theme, sketch, presentation, minimap, folds, marks, onError, children }: Props) {
  const { mode } = useMode();
  const editable = useEditable();
  const actions = useBoardActions();
  const phone = usePhone();
  // A share link on touch: a double tap zooms in (the editor keeps the double click for editing a card).
  const doubleTap = useMedia('(pointer: coarse)') && mode === 'share';
  const { fog, reveal, step, on: presenting } = presentation;

  const fitStep = useStepFit(fog, step, reveal);
  const openStep = useOpenStep();
  const graph = useGraph({
    resp, name, phone, folds, onError,
    onShown: (first, { board, nodes }) => { presentation.boardShown(first, openStep(board, nodes)); setTimeout(fitStep, 30); },
  });
  const live = useLiveValues(name, resp?.board);
  const onNodeDragStop = useMoveCard(name, graph.pos);

  const nodes = useMemo(() => decorateNodes(graph.nodes, { fresh: graph.fresh, fog, live, sketch, ...marks }), [graph.nodes, graph.fresh, fog, live, sketch, marks]);
  const edges = useMemo(() => decorateEdges(graph.edges, { fog, sketch }), [graph.edges, fog, sketch]);

  return (
    <ReactFlow nodes={nodes} edges={edges} nodeTypes={nodeTypes} edgeTypes={edgeTypes} onNodesChange={graph.onNodesChange}
      onNodeDragStop={onNodeDragStop} colorMode={colorModeOf(theme)} minZoom={0.1} maxZoom={3}
      nodesConnectable={false} nodesDraggable={editable} deleteKeyCode={editable ? 'Backspace' : null}
      onPaneContextMenu={actions.note && (e => { e.preventDefault(); actions.note!('', { x: e.clientX, y: e.clientY }); })}
      proOptions={{ hideAttribution: true }} zoomOnDoubleClick={doubleTap}>
      <Background variant={BackgroundVariant.Dots} gap={22} size={1.6} color="var(--dot)" bgColor="var(--bg)" />
      {children}
      {!presenting && !phone && <Controls showInteractive={false} position="bottom-right" />}
      {minimap && !presenting && !phone && <MiniMap position="bottom-left" pannable zoomable nodeStrokeWidth={2} />}
    </ReactFlow>
  );
}
