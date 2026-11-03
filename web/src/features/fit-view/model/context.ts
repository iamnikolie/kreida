import { createContext, use } from 'react';

/** What the page tells a fit about the board on show. */
export interface FitOpts {
  /** The title bar carries the owner's name: on a phone it is taller, and the fit keeps clear of it. */
  owned?: boolean;
  /** The board has steps to walk: a client's desktop may then open on one and show "all" at the reading floor, cut. */
  stepped?: boolean;
  /** Where the head and the legend laid over the canvas end (px from the top, measured): a client's desktop keeps the board below it. 0 = not measured. */
  top?: number;
  /** How much of the canvas's foot the steps bar takes (px, measured), 0 when it is not up: the board is framed above it. */
  foot?: number;
}

/** Provided by the board page around the canvas, so a fit anywhere below (the bar, the toolbar, the canvas) agrees. */
export const FitContext = createContext<FitOpts>({});

export const useFitOpts = (own?: FitOpts): FitOpts => { const inherited = use(FitContext); return own ?? inherited; };
