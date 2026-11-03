export interface Insets { top: number; right: number; bottom: number; left: number }

export const NO_INSETS: Insets = { top: 0, right: 0, bottom: 0, left: 0 };

let probe: HTMLElement | null = null;

/**
 * The safe-area insets as they are now, in px: what a notch, a rounded corner or the home indicator takes from the
 * screen's edges (the page reaches under them: `viewport-fit=cover`). The CSS reads them as `--safe-top` and the rest
 * (tokens.css); this reads the very same variables through a hidden element's padding, so a value a page sets by hand
 * to try a device counts here too, and a turn of the phone is seen on the next read.
 */
export function safeInsets(): Insets {
  if (typeof document === 'undefined' || !document.body) return NO_INSETS;
  if (!probe) {
    probe = document.createElement('div');
    probe.setAttribute('aria-hidden', 'true');
    probe.style.cssText = 'position:fixed;left:0;top:0;width:0;height:0;visibility:hidden;pointer-events:none;'
      + 'padding:var(--safe-top) var(--safe-right) var(--safe-bottom) var(--safe-left)';
  }
  if (!probe.isConnected) document.body.append(probe);
  const s = getComputedStyle(probe), px = (v: string) => parseFloat(v) || 0;
  return { top: px(s.paddingTop), right: px(s.paddingRight), bottom: px(s.paddingBottom), left: px(s.paddingLeft) };
}
