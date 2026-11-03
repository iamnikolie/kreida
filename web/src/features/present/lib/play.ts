import type { Mode } from '@shared/lib';
import { nextStep } from './nav';

/** How long each step, and then the whole board, stays on screen while a board plays itself. */
export const PLAY_MS = 2500;

/**
 * Whether `?play=1` starts the autoplay: a share link only (the demo embedded in a page), never a headless capture,
 * never under `prefers-reduced-motion` (the board stays whole and still), and only a board that has steps to walk.
 * `?play=0` is off.
 */
export function canPlay(p: { mode: Mode; query: URLSearchParams; capture: boolean; reducedMotion: boolean; stepped: boolean }): boolean {
  return p.mode === 'share' && p.query.has('play') && p.query.get('play') !== '0' && !p.capture && !p.reducedMotion && p.stepped;
}

/** The events that end the autoplay for good (the visitor has taken over); see `endsPlay`. */
export const PLAY_ENDERS = ['pointerenter', 'pointermove', 'pointerdown', 'touchstart', 'keydown'] as const;

/**
 * Whether this event ends the autoplay: a press or a key anywhere, a touch, or a pointer that hovers (a mouse or a pen
 * moving over the page - a finger only moves while it is down, and that press has stopped the play already).
 */
export function endsPlay(type: string, pointerType?: string): boolean {
  if (type === 'pointerenter' || type === 'pointermove') return pointerType !== 'touch';
  return (PLAY_ENDERS as readonly string[]).includes(type);
}

/**
 * The step after `step` in the loop: what nextStep says (the last step goes to the whole board), except on a board whose
 * whole cannot be shown at the reading size in this frame (`overview` false: a small embed): it has no whole-board beat,
 * so after the last step comes the first.
 */
export function playNext(step: number | null, count: number, overview: boolean): number | null {
  const next = nextStep(step, 1, count);
  return next == null && !overview && count > 0 ? 0 : next;
}
