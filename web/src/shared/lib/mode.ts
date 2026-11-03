import { createContext, use } from 'react';
import type { BoardResp } from '../api/contract';

/**
 * How this page was opened:
 *  - editor: served by the local server, full editing;
 *  - share:  /s/<token> (local) or /b/<token> (kreida cloud), one board, read-only, still live; both talk to
 *            /api/share/<token>;
 *  - static: a standalone export with the board inlined, no server at all.
 */
export type Mode = 'editor' | 'share' | 'static';

export interface PageMode {
  mode: Mode;
  /** Derived: only the editor writes. */
  readonly: boolean;
  /** `?static`: a headless render (PNG) - no open streams, timers or animation, so the page can go idle. */
  capture: boolean;
  /** The board the page opens first: the inlined one, the share token, or `?b=`. */
  board: string | null;
  /** The share token, in share mode. */
  share: string | null;
  /** A kreida cloud link (`/b/<token>`), not the local `/s/<token>`: the page the cloud serves, with its owner's name and its "made with" link. */
  cloud: boolean;
  /** The board data inlined into an exported page. */
  inlined: BoardResp | null;
  /** The page's query string, parsed once (`theme`, `present`, `step`, ...). */
  query: URLSearchParams;
}

declare global {
  interface Window { __KREIDA__?: BoardResp }
}

export function detectMode(
  loc: { pathname: string; search: string } = location,
  win: { __KREIDA__?: BoardResp } = window,
): PageMode {
  const query = new URLSearchParams(loc.search);
  const inlined = win.__KREIDA__ ?? null;
  const link = loc.pathname.match(/^\/([sb])\/([\w-]+)/);
  const share = link?.[2] ?? null;
  const mode: Mode = inlined ? 'static' : share ? 'share' : 'editor';
  return {
    mode, readonly: mode !== 'editor', capture: query.has('static'), board: inlined?.name ?? share ?? query.get('b'), share,
    cloud: mode === 'share' && link?.[1] === 'b', inlined, query,
  };
}

export const ModeContext = createContext<PageMode | null>(null);

export function useMode(): PageMode {
  const m = use(ModeContext);
  if (!m) throw new Error('useMode outside ModeContext');
  return m;
}
