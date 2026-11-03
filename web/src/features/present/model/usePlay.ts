import { useEffect, useRef, useState } from 'react';
import { useMedia, useMode } from '@shared/lib';
import { canPlay, endsPlay, PLAY_ENDERS, PLAY_MS, playNext } from '../lib/play';
import type { Presentation } from './usePresentation';

/**
 * `?play=1` on a share link: the board walks its own steps, each for 2.5 s, then shows the whole board for 2.5 s and
 * loops (last step -> everything -> first step; a board whose whole cannot be read in this frame has no such beat, see
 * playNext). It presents (the steps bar captions each step, the toolbar steps aside) and stops for good at the first hover,
 * press, touch or key: the visitor keeps the step on screen and the arrows of the steps bar. Under prefers-reduced-motion
 * it never plays. Returns whether it is playing.
 */
export function usePlay(presentation: Presentation): boolean {
  const { mode, query, capture } = useMode();
  const reducedMotion = useMedia('(prefers-reduced-motion: reduce)');
  const [taken, setTaken] = useState(false);
  const playing = !taken && canPlay({ mode, query, capture, reducedMotion, stepped: presentation.stepped });
  const { step, steps, overview, start, goTo } = presentation;
  const next = useRef(() => {});
  next.current = () => goTo(playNext(step, steps.length, overview));

  useEffect(() => { if (playing) start(); }, [playing, start]);

  // One beat per step change: a step the board moved to by itself restarts the clock, so a late layout never shortens a beat.
  useEffect(() => {
    if (!playing) return;
    const id = setTimeout(() => next.current(), PLAY_MS);
    return () => clearTimeout(id);
  }, [playing, step]);

  useEffect(() => {
    if (!playing) return;
    const stop = (e: Event) => { if (endsPlay(e.type, (e as PointerEvent).pointerType)) setTaken(true); };
    for (const type of PLAY_ENDERS) addEventListener(type, stop, { capture: true, passive: true });
    return () => { for (const type of PLAY_ENDERS) removeEventListener(type, stop, { capture: true }); };
  }, [playing]);

  return playing;
}
