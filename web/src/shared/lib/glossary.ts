import { createContext, use } from 'react';

// The board's glossary (`terms:`): every mention of a term or one of its aliases in a card is marked and explains itself
// on hover or tap. Matching is case-insensitive, whole words only, the longest term first.

export interface Term { term: string; text: string; aka: string[] }
export type Segment = string | { text: string; term: Term };
export type Splitter = (s: string) => Segment[];

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
// a term's edges: no letter or digit right before or after it (\b knows no Cyrillic)
const EDGE_L = '(?<![\\p{L}\\p{N}])', EDGE_R = '(?![\\p{L}\\p{N}])';

export function compileGlossary(terms: Term[]): Splitter {
  const names = terms.flatMap(t => [t.term, ...t.aka].filter(Boolean).map(n => ({ n, t })))
    .sort((a, b) => b.n.length - a.n.length);
  if (!names.length) return s => (s ? [s] : []);
  const byName = new Map(names.map(({ n, t }) => [n.toLowerCase(), t]));
  const re = new RegExp(`${EDGE_L}(?:${names.map(({ n }) => escape(n)).join('|')})${EDGE_R}`, 'giu');
  return s => {
    const out: Segment[] = [];
    let at = 0;
    for (const m of s.matchAll(re)) {
      if (m.index > at) out.push(s.slice(at, m.index));
      out.push({ text: m[0], term: byName.get(m[0].toLowerCase())! });
      at = m.index + m[0].length;
    }
    if (at < s.length) out.push(s.slice(at));
    return out;
  };
}

const none: Splitter = s => (s ? [s] : []);
export const GlossaryContext = createContext<Splitter>(none);
export const useGlossary = () => use(GlossaryContext);
