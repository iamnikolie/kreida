// A card's second line on a client's page reads at 4.5:1 or more on every card fill, in both themes
// (D-2026-10-29-03, row j). Reads the tokens as tokens.css writes them.
import { describe, expect, it } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';

const css = fs.readFileSync(path.resolve(__dirname, '../src/app/styles/tokens.css'), 'utf8');
const block = (selector: string) => css.slice(css.indexOf(selector)).split('}')[0];
const tokens = (selector: string) => Object.fromEntries([...block(selector).matchAll(/--([\w-]+):\s*(#[0-9a-f]{6})\b/gi)].map(m => [m[1], m[2]]));
const light = tokens(':root {'), dark = { ...light, ...tokens('[data-theme=dark] {') };

const lin = (c: number) => { const s = c / 255; return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4; };
const lum = (hex: string) => { const n = parseInt(hex.slice(1), 16); return 0.2126 * lin(n >> 16) + 0.7152 * lin((n >> 8) & 255) + 0.0722 * lin(n & 255); };
const ratio = (a: string, b: string) => { const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p); return (x + 0.05) / (y + 0.05); };
/** `fg` at `alpha` over `bg`. */
const over = (fg: string, bg: string, alpha: number) => {
  const f = parseInt(fg.slice(1), 16), b = parseInt(bg.slice(1), 16);
  const ch = (shift: number) => Math.round(alpha * ((f >> shift) & 255) + (1 - alpha) * ((b >> shift) & 255));
  return `#${[16, 8, 0].map(s => ch(s).toString(16).padStart(2, '0')).join('')}`;
};

describe.each([['light', light], ['dark', dark]] as const)('%s: the second line', (_, t) => {
  const fills = { plain: t['plain-bg'] ?? '', src: t['src-bg'], core: t['core-bg'], out: t['out-bg'], warn: t['warn-bg'], info: t['info-bg'], 'p2 (on the page)': t.bg };

  it.each(Object.entries(fills))('--sub on a %s card is 4.5:1 or more', (_kind, fill) => {
    expect(fill).toMatch(/^#/);
    expect(ratio(t.sub, fill)).toBeGreaterThanOrEqual(4.5);
  });

  it('a who pill and a note keep their own ink at an opacity that passes', () => {
    expect(ratio(over(t['who-fg'], t['who-bg'], 0.7), t['who-bg'])).toBeGreaterThanOrEqual(4.5);
    expect(ratio(over(t['note-fg'], t['note-bg'], 0.85), t['note-bg'])).toBeGreaterThanOrEqual(4.5);
  });

  it('is still quieter than the title', () => {
    expect(ratio(t.sub, t['plain-bg'])).toBeLessThan(ratio(t.fg, t['plain-bg']));
  });
});
