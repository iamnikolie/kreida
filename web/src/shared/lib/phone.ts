import { useSyncExternalStore } from 'react';
import { useMode, type Mode } from './mode';

/** A screen narrower than this on its short side is a phone, not a tablet or a desktop window. */
const PHONE_SHORT_SIDE = 600;

/** Portrait (as the `orientation` media query: height >= width) and a short side under 600 px. */
export const phoneViewport = (width: number, height: number): boolean => Math.min(width, height) < PHONE_SHORT_SIDE && height >= width;

/** A phone is only ever a client's share link on a phone held upright; the editor and the exported file never change. */
export const isPhone = (mode: Mode, width: number, height: number): boolean => mode === 'share' && phoneViewport(width, height);

const subscribe = (notify: () => void) => {
  addEventListener('resize', notify); addEventListener('orientationchange', notify);
  return () => { removeEventListener('resize', notify); removeEventListener('orientationchange', notify); };
};

/** Whether this page is a phone right now; flips with a resize or a turn of the device. Rendered on a server (a test) it is not. */
export function usePhone(): boolean {
  const { mode } = useMode();
  return useSyncExternalStore(subscribe, () => isPhone(mode, innerWidth, innerHeight), () => false);
}
