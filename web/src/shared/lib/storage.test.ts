import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { store, stored } from './storage';

let data: Record<string, string>;
beforeEach(() => {
  data = {};
  vi.stubGlobal('localStorage', {
    getItem: (k: string) => data[k] ?? null,
    setItem: (k: string, v: string) => { data[k] = v; },
  });
});
afterEach(() => vi.unstubAllGlobals());

it('reads and writes a kreida.* key', () => {
  store('kreida.theme', 'dark');
  expect(data).toEqual({ 'kreida.theme': 'dark' });
  expect(stored('kreida.theme')).toBe('dark');
});

it('falls back to the pre-rename doska.* key until the new one is written', () => {
  data['doska.theme'] = 'dark';
  expect(stored('kreida.theme')).toBe('dark');
  data['kreida.theme'] = 'light';
  expect(stored('kreida.theme')).toBe('light');
});

it('unset keys read null, and other prefixes get no fallback', () => {
  expect(stored('kreida.theme')).toBeNull();
  data['doska.other'] = 'x';
  expect(stored('other.thing')).toBeNull();
});

it('storage that throws (private mode) is swallowed', () => {
  vi.stubGlobal('localStorage', { getItem: () => { throw new Error('blocked'); }, setItem: () => { throw new Error('blocked'); } });
  expect(stored('kreida.theme')).toBeNull();
  expect(() => store('kreida.theme', 'dark')).not.toThrow();
});
