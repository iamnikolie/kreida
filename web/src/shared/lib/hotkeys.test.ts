import { describe, expect, it, vi } from 'vitest';
import { isTyping, matchHotkey, type Hotkey } from './hotkeys';

const key = (k: string, mods: Partial<Record<'metaKey' | 'ctrlKey' | 'altKey', boolean>> = {}) =>
  ({ key: k, metaKey: false, ctrlKey: false, altKey: false, ...mods });
const run = () => {};
const map: Hotkey[] = [
  { keys: ['z'], mod: true, prevent: true, run },
  { keys: ['p'], run },
  { keys: ['Home'], when: false, run },
  { keys: ['f', '0'], run },
];

describe('matchHotkey', () => {
  it('a bare key fires its binding', () => {
    expect(matchHotkey(key('p'), map)).toBe(map[1]);
    expect(matchHotkey(key('0'), map)).toBe(map[3]);
    expect(matchHotkey(key('q'), map)).toBeUndefined();
  });

  it('⌘/Ctrl chords only fire chord bindings, and chord bindings need the modifier', () => {
    expect(matchHotkey(key('z', { metaKey: true }), map)).toBe(map[0]);
    expect(matchHotkey(key('z', { ctrlKey: true }), map)).toBe(map[0]);
    expect(matchHotkey(key('z'), map)).toBeUndefined();
    expect(matchHotkey(key('p', { metaKey: true }), map)).toBeUndefined();
  });

  it('Alt blocks a bare key but not a chord', () => {
    expect(matchHotkey(key('p', { altKey: true }), map)).toBeUndefined();
    expect(matchHotkey(key('z', { metaKey: true, altKey: true }), map)).toBe(map[0]);
  });

  it('a binding whose `when` is false stays silent', () => {
    expect(matchHotkey(key('Home'), map)).toBeUndefined();
    expect(matchHotkey(key('Home'), [{ keys: ['Home'], when: true, run }])).toBeDefined();
  });

  it('is case sensitive, like the keys it replaces (Shift+P is not p)', () => {
    expect(matchHotkey(key('P'), map)).toBeUndefined();
  });
});

describe('isTyping', () => {
  const el = (tagName: string, isContentEditable = false) => ({ tagName, isContentEditable }) as unknown as EventTarget;
  it('fields and editable cards keep the keys to themselves', () => {
    expect(isTyping(el('INPUT'))).toBe(true);
    expect(isTyping(el('TEXTAREA'))).toBe(true);
    expect(isTyping(el('SELECT'))).toBe(true);
    expect(isTyping(el('DIV', true))).toBe(true);
  });
  it('the canvas and the body do not', () => {
    expect(isTyping(el('DIV'))).toBe(false);
    expect(isTyping(el('BODY'))).toBe(false);
    expect(isTyping(null)).toBe(false);
  });
});

it('run is what the caller wired', () => {
  const f = vi.fn();
  matchHotkey(key('p'), [{ keys: ['p'], run: f }])!.run();
  expect(f).toHaveBeenCalledOnce();
});
