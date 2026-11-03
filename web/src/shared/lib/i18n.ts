// Minimal UI i18n: one catalog per language (catalog/ru.ts is the base, uk.ts and en.ts are typed after it),
// `t(key)`. Every string kreida's own UI shows - the share link's page and the editor alike - comes from here; i18n.test.ts
// fails on a key that any language lacks, and hardcoded.test.ts on text written straight into a component.

import { ru, type Key } from './catalog/ru';
import { uk } from './catalog/uk';
import { en } from './catalog/en';

export type { Key };
export type Lang = 'ru' | 'uk' | 'en';
export const LANGS: readonly Lang[] = ['ru', 'uk', 'en'];

export const catalogs: Record<Lang, Record<string, string>> = { ru, uk, en };

const PLURAL = /^(.+)\.(zero|one|two|few|many|other)$/;
const categories = (l: Lang) => new Intl.PluralRules(l).resolvedOptions().pluralCategories;

/**
 * `lang:key` for every key that some language lacks or leaves empty. A plural (`base.one`, `base.few`, ...) needs the
 * forms its language has - Russian and Ukrainian four, English two - not the same suffixes everywhere.
 */
export function missingKeys(cats: Record<Lang, Record<string, string>>): string[] {
  const keys = LANGS.flatMap(l => Object.keys(cats[l]));
  const plain = new Set(keys.filter(k => !PLURAL.test(k)));
  const bases = new Set(keys.flatMap(k => PLURAL.exec(k)?.[1] ?? []));
  return LANGS.flatMap(l => [
    ...[...plain].filter(k => !cats[l][k]),
    ...[...bases].flatMap(b => categories(l).map(c => `${b}.${c}`).filter(k => !cats[l][k])),
  ].map(k => `${l}:${k}`));
}

/** The language of a tag (`uk-UA` -> uk); null for one we have no catalog for. */
const langOf = (tag: string | null | undefined): Lang | null => LANGS.find(l => l === tag?.toLowerCase().split(/[-_]/)[0]) ?? null;

/** The first language of the candidates that has a catalog; anything else is English. */
export function pickLang(tags: readonly (string | null | undefined)[]): Lang {
  for (const tag of tags) { const l = langOf(tag); if (l) return l; }
  return 'en';
}

/**
 * Where the page's language comes from, in this order: `<meta name="kreida-lang">` (kreida cloud writes the link's
 * language into the served index.html; a strict CSP rules out a script), then `?lang=`, then the browser's languages.
 */
export function detectLocale(from: { meta: readonly (string | null)[]; search: string; languages: readonly string[] }): Lang {
  return pickLang([...from.meta, new URLSearchParams(from.search).get('lang'), ...from.languages]);
}

/** A key missing from a catalog falls back to English, never to another language (uk must not show Russian). */
export function translate(lang: Lang, key: string, cats = catalogs): string {
  return cats[lang][key] || cats.en[key] || key;
}

/** Puts values into the `{name}` slots of a text; a slot with no value stays as it is. */
export const fill = (text: string, vars?: Record<string, string | number>): string =>
  vars ? text.replace(/\{(\w+)\}/g, (slot, name: string) => (name in vars ? String(vars[name]) : slot)) : text;

/** The form of `base` that suits `n` in this language (`base.one`, `base.few`, ...), by Intl.PluralRules; `base.other` when the language has no such form. */
export function plural(lang: Lang, base: string, n: number, cats = catalogs): string {
  return cats[lang][`${base}.${new Intl.PluralRules(lang).select(n)}`] || translate(lang, `${base}.other`, cats);
}

/**
 * A date with its time as the cloud writes it: day month year, 24-hour (`29 окт 2026, 11:04`) - never the en-US
 * `Oct 29, 2026, 11:04 AM`. Empty for a value that is no date.
 */
export function formatDateTime(date: Date | string | number, lang: Lang): string {
  const d = new Date(date);
  if (Number.isNaN(d.getTime())) return '';
  const p = Object.fromEntries(new Intl.DateTimeFormat(lang, { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' })
    .formatToParts(d).map(x => [x.type, x.value]));
  return `${p.day} ${p.month.replace(/\.$/, '')} ${p.year}, ${p.hour}:${p.minute}`;
}

let current: Lang | null = null;

/** The language of this page, read once. Every string of kreida's own UI is looked up in the catalog of this language. */
export function pageLocale(): Lang {
  return (current ??= detectLocale({
    meta: [...document.querySelectorAll('meta[name="kreida-lang"]')].map(m => m.getAttribute('content')),
    search: location.search,
    languages: navigator.languages?.length ? navigator.languages : [navigator.language],
  }));
}

export const t = (key: Key, vars?: Record<string, string | number>): string => fill(translate(pageLocale(), key), vars);

/** A count with its noun in this page's language: `tn('cards', 3)` looks up `cards.one|few|many|other` and fills `{n}`. */
export const tn = (base: string, n: number): string => fill(plural(pageLocale(), base, n), { n });
