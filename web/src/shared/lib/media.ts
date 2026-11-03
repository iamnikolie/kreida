import { useSyncExternalStore } from 'react';

/** Whether a CSS media query matches right now, and keeps matching as the window changes. */
export function useMedia(query: string): boolean {
  return useSyncExternalStore(
    notify => { const m = matchMedia(query); m.addEventListener('change', notify); return () => m.removeEventListener('change', notify); },
    () => matchMedia(query).matches,
  );
}
