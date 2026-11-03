import { useCallback, useEffect, useRef, useState } from 'react';
import { applyNodeChanges, type Edge, type Node, type NodeChange } from '@xyflow/react';
import { clientFrame } from '@features/fit-view';
import { changedCards, layout, signatures, type Board, type BoardResp, type Pos, type Signatures } from '@entities/board';
import { t, useMode } from '@shared/lib';

interface Options {
  resp: BoardResp | null;
  /** The open board's name (the key cards use to talk back to the server). */
  name: string | null;
  /** A phone held upright: left-to-right boards are laid out top-to-bottom. */
  phone: boolean;
  /** The groups folded into tiles. */
  folds: Set<string>;
  /** A different board (or a restructured one) was laid out. `first`: the page's very first. */
  onShown(first: boolean, shown: { board: Board; nodes: Node[] }): void;
  onError(message: string): void;
}

/** The laid-out graph of the response. Re-lays out on every new response (and when the phone turns) and notes which cards changed. */
export function useGraph({ resp, name, phone, folds, onShown, onError }: Options) {
  const { mode } = useMode();
  const [nodes, setNodes] = useState<Node[]>([]);
  const [edges, setEdges] = useState<Edge[]>([]);
  const [fresh, setFresh] = useState<Set<string>>(new Set());
  /** Saved positions: from the response, then updated by drags (features/move-card). */
  const pos = useRef<Pos>({});
  const prev = useRef<{ name: string; sig: Signatures } | null>(null);
  const shown = useRef('');
  const latest = useRef({ onShown, onError });
  latest.current = { onShown, onError };

  useEffect(() => {
    if (!resp?.board) {
      // A share link that answers without a board is off (revoked, expired): drop what it showed. The editor
      // keeps the last good layout while a YAML edit is broken.
      if (resp && mode === 'share') { setNodes([]); setEdges([]); prev.current = null; shown.current = ''; }
      return;
    }
    if (!name) return;
    const b = resp.board;
    pos.current = resp.pos ?? {};
    // Highlight what changed since the previous version of the same board.
    const now = signatures(b);
    if (prev.current?.name === name) {
      const changed = changedCards(prev.current.sig, now);
      if (changed) { setFresh(changed); setTimeout(() => setFresh(new Set()), 4000); }
    }
    prev.current = { name, sig: now };
    let current = true;
    // A client's screen (or the frame a board is embedded in) shapes a flat chain into rows; the owner's editor does not.
    const frame = mode === 'share' && !phone ? clientFrame({ width: innerWidth, height: innerHeight }) : undefined;
    layout(b, pos.current, name, { phone, frame, folds }).then(r => {
      if (!current) return;
      setNodes(r.nodes);
      setEdges(r.edges);
      const key = `${name}${phone ? ':phone' : ''}:${r.nodes.map(n => n.id).join(',')}`;
      if (key !== shown.current) {
        const first = !shown.current; shown.current = key;
        latest.current.onShown(first, { board: b, nodes: r.nodes });
      }
    }).catch(e => { if (current) latest.current.onError(t('error.layout', { detail: e instanceof Error ? e.message : String(e) })); });
    return () => { current = false; };
  }, [resp, phone, folds]); // eslint-disable-line react-hooks/exhaustive-deps

  const onNodesChange = useCallback((c: NodeChange[]) => setNodes(ns => applyNodeChanges(c, ns)), []);
  return { nodes, edges, fresh, pos, onNodesChange };
}
