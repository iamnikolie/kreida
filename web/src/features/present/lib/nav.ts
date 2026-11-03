/** The step after moving `d` from `step` among `count` steps; null = "everything" (before the first, after the last). */
export function nextStep(step: number | null, d: 1 | -1, count: number): number | null {
  if (!count) return null;
  const next = step == null ? (d > 0 ? 0 : count - 1) : step + d;
  return next < 0 || next >= count ? null : next;
}

/** `?step=N` is 1-based in the URL, 0-based inside. */
export const startStep = (query: URLSearchParams): number | null => (query.has('step') ? Number(query.get('step')) - 1 : null);

/** Whether a board has steps to walk: declared, or more than one top-level item (a single declared step still gets its caption). */
export const hasSteps = (count: number, declared: boolean): boolean => count > 1 || declared;

/**
 * Whether the steps bar shows: only while presenting a board that has steps. A headless capture shows it only
 * on a chosen step, as a caption.
 */
export function barVisible(p: { on: boolean; count: number; declared: boolean; capture: boolean; step: number | null }): boolean {
  return p.on && hasSteps(p.count, p.declared) && (!p.capture || p.step != null);
}
