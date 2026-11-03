import { useCallback, useEffect, useRef } from 'react';
import { useReactFlow, useStoreApi, type Node } from '@xyflow/react';
import { boundsOf, type Board, type Fog } from '@entities/board';
import { safeInsets, useMode, usePhone } from '@shared/lib';
import { CLIENT_FLOOR, clientPad, clientStepView, clientView } from '../lib/client';
import { openStep, phonePad, stepBox, stepViewport } from '../lib/phone';
import { useFitOpts, type FitOpts } from './context';

// Room kept around the board: more on top, where the title and the toolbar sit.
const FIT = { padding: { top: '130px', bottom: '48px', left: '48px', right: '48px' } as const, duration: 300, maxZoom: 1.25 };
// On a phone the title is one line (two with the owner's name: `owned`) and the bottom bar takes the foot (see PHONE_PAD),
// plus the insets of this very moment (a notch, the home indicator: they change when the phone turns, so they are read
// whenever a fit runs).
const px = (n: number): `${number}px` => `${n}px`;
const phoneFit = (owned: boolean) => {
  const p = phonePad(safeInsets(), owned);
  return { ...FIT, padding: { top: px(p.top), bottom: px(p.bottom), left: px(p.left), right: px(p.right) } };
};

/** A client's desktop: a share link on a screen that is no phone (or the frame a board is embedded in). It reads at a floor of rendered size. */
function useClient(): boolean {
  const { mode } = useMode();
  return mode === 'share' && !usePhone();
}

/** Fit the whole board in view. On a client's desktop with steps that is the whole board at the reading floor at least, cut if it needs less. */
export function useFitView(opts?: FitOpts) {
  const { owned = false, stepped = false, top = 0, foot = 0 } = useFitOpts(opts);
  const flow = useReactFlow();
  const phone = usePhone();
  const client = useClient();
  const view = useViewSize();
  return useCallback(() => {
    const box = client ? boundsOf(flow.getNodes()) : null;
    if (box) void flow.setViewport(clientView(box, view(), clientPad(top, foot), stepped ? CLIENT_FLOOR : 0), { duration: 300 });
    else void flow.fitView(phone ? phoneFit(owned) : FIT);
  }, [flow, phone, client, view, owned, stepped, top, foot]);
}

/** The size of the canvas: what a fit has to fit into. */
function useViewSize() {
  const store = useStoreApi();
  return useCallback(() => { const { width, height } = store.getState(); return { width: width || innerWidth, height: height || innerHeight }; }, [store]);
}

/**
 * Where the page opens. A phone, and a client's desktop, open on step 1 when the whole board would need less than the
 * reading floor; everywhere else (and where the board reads whole) it returns null: the whole board.
 */
export function useOpenStep() {
  const { owned = false, top = 0, foot = 0 } = useFitOpts();
  const phone = usePhone();
  const client = useClient();
  const view = useViewSize();
  return useCallback((b: Board, nodes: Node[]) => {
    if (phone) return openStep(b, nodes, view(), phonePad(safeInsets(), owned));
    return client ? openStep(b, nodes, view(), clientPad(top, foot), CLIENT_FLOOR) : null;
  }, [phone, client, view, owned, top, foot]);
}

/**
 * Keeps the view on what matters: the whole board, or while a step is current only what that step shows.
 * Refits whenever the step changes; the returned function refits on demand (after a layout).
 * It reads the latest `fog` when it runs, so a delayed call never uses a stale step.
 * On a phone a step looks at what it brings into view (`reveal`), never zooms below the readability floor and is
 * read from its top left; so does a client's desktop (CLIENT_FLOOR), when everything the step shows would need less.
 */
export function useStepFit(fog: Fog | null, step: number | null, reveal: ((id: string) => boolean) | null = null) {
  const { owned = false, stepped = false, top = 0, foot = 0 } = useFitOpts();
  const flow = useReactFlow();
  const { capture } = useMode();
  const phone = usePhone();
  const client = useClient();
  const view = useViewSize();
  const fit = useRef(() => {});
  fit.current = () => {
    const base = phone ? phoneFit(owned) : FIT;
    const shown = fog ? flow.getNodes().filter(n => !fog(n.id)).map(n => ({ id: n.id })) : [];
    const duration = capture ? 0 : 450;
    const all = flow.getNodes();
    if (client) {
      // A client reads at a floor of rendered size: what the step shows, or what it brings in when that outgrows the screen.
      const on = fog && shown.length ? boundsOf(all, id => !fog(id)) : boundsOf(all);
      if (on) { void flow.setViewport(clientStepView(on, fog && reveal ? boundsOf(all, reveal) : null, view(), clientPad(top, foot), stepped ? CLIENT_FLOOR : 0), { duration }); return; }
    }
    const box = phone && fog && shown.length ? stepBox(all, reveal, id => !fog(id)) : null;
    if (box) void flow.setViewport(stepViewport(box, view(), phonePad(safeInsets(), owned)), { duration });
    else if (shown.length) void flow.fitView({ ...base, nodes: shown, maxZoom: 1.3, duration });
    else void flow.fitView(base);
  };
  // refits when the step changes and when what is laid over the canvas changes size (a title that wraps, a font that arrives, the steps bar coming up)
  useEffect(() => { fit.current(); }, [step, top, foot]);
  return useCallback(() => fit.current(), []);
}
