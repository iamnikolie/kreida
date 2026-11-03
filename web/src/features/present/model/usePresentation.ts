import { useCallback, useMemo, useState } from 'react';
import { fogOf, revealAt, stepsOf, type Board, type BStep, type Fog } from '@entities/board';
import { useMode } from '@shared/lib';
import { barVisible, hasSteps, nextStep, startStep } from '../lib/nav';

export interface Presentation {
  /** Presenting: the chrome hides and the arrows walk the steps. */
  on: boolean;
  /** The current step, null = everything shown. */
  step: number | null;
  steps: BStep[];
  /** What the current step hides, null when nothing is hidden. */
  fog: Fog | null;
  /** What the current step brings into view (a phone too narrow for all of it looks here), null while no step is selected. */
  reveal: ((id: string) => boolean) | null;
  /** Whether the board has steps to walk (a phone shows its own step controls when it does). */
  stepped: boolean;
  /** Whether the whole board can be shown at the reading size in this frame: false where the board opened on step 1 because it could not (a phone, a small embed). */
  overview: boolean;
  /** Whether the steps bar shows. */
  barVisible: boolean;
  go(d: 1 | -1): void;
  /** To this step (null = everything), as the autoplay does. */
  goTo(step: number | null): void;
  toggle(): void;
  start(): void;
  exit(): void;
  showAll(): void;
  /** A board was (re)laid out; `first` is the page's very first one. Goes to `start` (default "everything") unless `?step=` asked for a step. */
  boardShown(first: boolean, start?: number | null): void;
}

/** Presentation state: on/off, the current step, and what that step hides. Starts from `?present` and `?step=N`. */
export function usePresentation(board: Board | undefined): Presentation {
  const { query, capture } = useMode();
  const [on, setOn] = useState(() => query.has('present'));
  const [step, setStep] = useState<number | null>(() => startStep(query));
  const [overview, setOverview] = useState(true);

  const steps = useMemo(() => (board ? stepsOf(board) : []), [board]);
  const fog = useMemo(() => (board ? fogOf(board, steps, step) : null), [board, steps, step]);

  const reveal = useMemo(() => (board ? revealAt(board, steps, step) : null), [board, steps, step]);

  const go = useCallback((d: 1 | -1) => setStep(s => nextStep(s, d, steps.length)), [steps]);
  const toggle = useCallback(() => { setOn(v => !v); setStep(null); }, []);
  const start = useCallback(() => { setOn(true); setStep(null); }, []);
  const exit = useCallback(() => { setOn(false); setStep(null); }, []);
  const showAll = useCallback(() => setStep(null), []);
  // A board that opens on a step (a phone, a client's screen too small for the whole board) is presenting: the steps bar names the step and walks on.
  const boardShown = useCallback((first: boolean, start: number | null = null) => {
    setOverview(start == null);
    if (first && query.has('step')) return;
    setStep(start);
    if (start != null) setOn(true);
  }, [query]);

  return {
    on, step, steps, fog, reveal, go, goTo: setStep, toggle, start, exit, showAll, boardShown,
    stepped: hasSteps(steps.length, !!board?.steps.length), overview,
    barVisible: barVisible({ on, count: steps.length, declared: !!board?.steps.length, capture, step }),
  };
}
