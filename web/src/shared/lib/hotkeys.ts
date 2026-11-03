import { useEffect, useRef } from 'react';

export interface Hotkey {
  /** `KeyboardEvent.key` values that fire it. */
  keys: string[];
  /** A ⌘/Ctrl chord; without it the key must come bare (no ⌘, Ctrl or Alt). */
  mod?: boolean;
  /** Active only while this holds (default: always). */
  when?: boolean;
  /** preventDefault before running (the key would scroll the page, ⌘Z would undo in the browser). */
  prevent?: boolean;
  run: () => void;
}

type KeyInfo = Pick<KeyboardEvent, 'key' | 'metaKey' | 'ctrlKey' | 'altKey'>;

/** The first binding that fires for this key press, if any. */
export function matchHotkey(e: KeyInfo, keys: Hotkey[]): Hotkey | undefined {
  const mod = e.metaKey || e.ctrlKey;
  return keys.find(k => (k.when ?? true) && k.keys.includes(e.key) && (k.mod ? mod : !mod && !e.altKey));
}

/** Typing somewhere (a field, a card being edited): the page's keys stay out of the way. */
export const isTyping = (t: EventTarget | null) => {
  const el = t as HTMLElement | null;
  return !!el && (el.isContentEditable || ['SELECT', 'INPUT', 'TEXTAREA'].includes(el.tagName));
};

/** The page's one keydown listener. Pass the whole key map each render; the latest one is used. */
export function useHotkeys(keys: Hotkey[]) {
  const latest = useRef(keys);
  latest.current = keys;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (isTyping(e.target)) return;
      const k = matchHotkey(e, latest.current);
      if (!k) return;
      if (k.prevent) e.preventDefault();
      k.run();
    };
    addEventListener('keydown', onKey);
    return () => removeEventListener('keydown', onKey);
  }, []);
}
