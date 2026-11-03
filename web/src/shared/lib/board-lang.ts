import { createContext, use } from 'react';
import type { Board } from '../api/contract';
import { fill, pageLocale, plural, translate, type Key, type Lang } from './i18n';

// What the page draws on the canvas itself — the questions panel, a folded tile's count, a card's badges — goes into PNGs and
// exports with the board, so it speaks the board's language: the one the file names (`lang:`), else the one its words are
// written in. A board with no words follows the page.

const UK = /[іїєґІЇЄҐ]/;

/** The board's language: `lang:`, else Ukrainian or Russian by its Cyrillic, English by its Latin; null with no words. */
export function boardLang(b: Board): Lang | null {
  if (b.lang) return b.lang;
  const text = [b.title, b.sub, ...b.nodes.flatMap(n => [n.title, n.text]), ...b.groups.map(g => g.label)].join(' ');
  const cyr = (text.match(/\p{Script=Cyrillic}/gu) ?? []).length, lat = (text.match(/\p{Script=Latin}/gu) ?? []).length;
  if (!cyr && !lat) return null;
  if (cyr >= lat) return UK.test(text) ? 'uk' : 'ru';
  return 'en';
}

export const BoardLangContext = createContext<Lang | null>(null);

/** `t` and `tn` in the board's language (the page's when the board has none), for what sits on the canvas. */
export function useBoardT() {
  const lang = use(BoardLangContext) ?? pageLocale();
  return {
    t: (key: Key, vars?: Record<string, string | number>) => fill(translate(lang, key), vars),
    tn: (base: string, n: number) => fill(plural(lang, base, n), { n }),
  };
}
