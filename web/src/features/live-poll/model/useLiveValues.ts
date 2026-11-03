import { useEffect, useState } from 'react';
import { useApi, type Board, type LiveValues } from '@shared/api';

/**
 * Values of the board's live cards, refreshed while it is open at the fastest card's interval (the server
 * caches per card). Empty until the first answer, and on pages with no server.
 */
export function useLiveValues(name: string | null, board: Board | undefined): LiveValues {
  const api = useApi();
  const [live, setLive] = useState<LiveValues>({});
  const every = board ? Math.min(...board.nodes.filter(n => n.live).map(n => n.live!.every)) : Infinity;
  useEffect(() => {
    setLive({});
    if (!name || !Number.isFinite(every)) return;
    return api.watchLive(name, every, setLive);
  }, [api, name, every]);
  return live;
}
