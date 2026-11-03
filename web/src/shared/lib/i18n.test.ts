import { describe, expect, it } from 'vitest';
import { catalogs, detectLocale, fill, formatDateTime, LANGS, missingKeys, pickLang, plural, translate } from './i18n';

describe('catalogs', () => {
  it('every key exists, and is not empty, in ru, uk and en', () => {
    expect(missingKeys(catalogs)).toEqual([]);
  });

  it('names the language and the key that is missing', () => {
    const cats = { ru: { a: 'а', b: 'б' }, uk: { a: 'а' }, en: { a: 'a', b: '' } };
    expect(missingKeys(cats)).toEqual(['uk:b', 'en:b']);
  });

  it('a plural needs the forms its language has: four in ru and uk, two in en', () => {
    const cats = {
      ru: { 'n.one': '{n} карта', 'n.few': '{n} карты', 'n.many': '{n} карт', 'n.other': '{n} карты' },
      uk: { 'n.one': '{n} картка', 'n.few': '{n} картки', 'n.many': '', 'n.other': '{n} картки' },
      en: { 'n.one': '{n} card', 'n.other': '{n} cards' },
    };
    expect(missingKeys(cats)).toEqual(['uk:n.many']);
  });

  it('every language has the same keys (no catalog carries a key another does not)', () => {
    const keys = (l: 'ru' | 'uk' | 'en') => Object.keys(catalogs[l]).sort();
    expect(keys('uk')).toEqual(keys('ru'));
    expect(keys('en')).toEqual(keys('ru'));
  });

  it('knows exactly ru, uk and en', () => {
    expect([...LANGS]).toEqual(['ru', 'uk', 'en']);
    expect(Object.keys(catalogs)).toEqual(['ru', 'uk', 'en']);
  });
});

describe('translate', () => {
  it('answers in the chosen language', () => {
    expect([translate('ru', 'bar.all'), translate('uk', 'bar.all'), translate('en', 'bar.all')]).toEqual(['Вся схема', 'Уся схема', 'Whole board']);
  });

  it('a key a language lacks falls back to English, never uk to Russian', () => {
    const cats = { ru: { k: 'по-русски' }, uk: {}, en: { k: 'in English' } };
    expect(translate('uk', 'k', cats)).toBe('in English');
    expect(translate('ru', 'k', cats)).toBe('по-русски');
  });

  it('an unknown key shows as itself', () => {
    expect(translate('en', 'nope')).toBe('nope');
  });
});

describe('fill', () => {
  it('puts values into their slots, and leaves a slot with no value as it is', () => {
    expect(fill('Шаг {n} из {total}', { n: 2, total: 5 })).toBe('Шаг 2 из 5');
    expect(fill('{n} of {total}', { n: 1 })).toBe('1 of {total}');
    expect(fill('plain')).toBe('plain');
  });
  it('the catalog texts take their values in every language', () => {
    expect(fill(translate('uk', 'bar.step'), { n: 2, total: 5 })).toBe('Крок 2 з 5');
    expect(fill(translate('en', 'error.layout'), { detail: 'boom' })).toBe('Layout failed: boom');
  });
});

describe('plural', () => {
  const cats = {
    ru: { 'n.one': '{n} карта', 'n.few': '{n} карты', 'n.many': '{n} карт', 'n.other': '{n} карты' },
    uk: { 'n.one': '{n} картка', 'n.few': '{n} картки', 'n.many': '{n} карток', 'n.other': '{n} картки' },
    en: { 'n.one': '{n} card', 'n.other': '{n} cards' },
  };
  const of = (l: 'ru' | 'uk' | 'en', n: number) => fill(plural(l, 'n', n, cats), { n });

  it('ru and uk pick one, few, many by the number', () => {
    expect([1, 2, 5, 11, 21, 22, 25].map(n => of('ru', n))).toEqual(['1 карта', '2 карты', '5 карт', '11 карт', '21 карта', '22 карты', '25 карт']);
    expect([1, 3, 7, 12, 21].map(n => of('uk', n))).toEqual(['1 картка', '3 картки', '7 карток', '12 карток', '21 картка']);
  });
  it('en has one and other', () => {
    expect([0, 1, 2].map(n => of('en', n))).toEqual(['0 cards', '1 card', '2 cards']);
  });
  it('a form the catalog lacks falls back to other', () => {
    expect(plural('ru', 'n', 5, { ...cats, ru: { 'n.other': 'ещё {n}' } })).toBe('ещё {n}');
  });
});

describe('formatDateTime', () => {
  // local time on purpose: the page shows the viewer's own clock
  const at = new Date(2026, 9, 29, 11, 4), midnight = new Date(2026, 9, 5, 0, 7), evening = new Date(2026, 11, 1, 18, 30);

  it('day month year, 24-hour, in every language', () => {
    expect(formatDateTime(at, 'ru')).toBe('29 окт 2026, 11:04');
    expect(formatDateTime(at, 'uk')).toBe('29 жовт 2026, 11:04');
    expect(formatDateTime(at, 'en')).toBe('29 Oct 2026, 11:04');
  });
  it('never a 12-hour clock or the en-US order', () => {
    expect(formatDateTime(evening, 'en')).toBe('1 Dec 2026, 18:30');
    expect(formatDateTime(midnight, 'en')).toBe('5 Oct 2026, 00:07');
    expect(formatDateTime(midnight, 'ru')).toBe('5 окт 2026, 00:07');
  });
  it('takes an ISO string, and says nothing for what is no date', () => {
    expect(formatDateTime(at.toISOString(), 'en')).toBe('29 Oct 2026, 11:04');
    expect(formatDateTime('not a date', 'ru')).toBe('');
  });
});

describe('pickLang', () => {
  it('the first candidate that has a catalog, region ignored', () => {
    expect(pickLang(['uk-UA'])).toBe('uk');
    expect(pickLang(['EN', 'ru'])).toBe('en');
    expect(pickLang(['en_GB'])).toBe('en');
    expect(pickLang(['ru-RU', 'en-US'])).toBe('ru');
  });
  it('skips what it has no catalog for, and what is not there', () => {
    expect(pickLang([null, undefined, '', 'de-DE', 'uk'])).toBe('uk');
  });
  it('nothing usable is English, and uk does not turn into ru', () => {
    expect(pickLang(['de-DE', 'pl'])).toBe('en');
    expect(pickLang([])).toBe('en');
    expect(pickLang(['be'])).toBe('en');
  });
});

describe('detectLocale', () => {
  const from = (o: Partial<Parameters<typeof detectLocale>[0]>) => detectLocale({ meta: [], search: '', languages: [], ...o });

  it('the page\'s meta tag wins over ?lang= and over the browser', () => {
    expect(from({ meta: ['uk'], search: '?lang=ru', languages: ['en-US'] })).toBe('uk');
    expect(from({ meta: ['ru'], search: '?lang=en', languages: ['uk'] })).toBe('ru');
  });
  it('then ?lang=, then the browser\'s languages in order', () => {
    expect(from({ search: '?lang=uk', languages: ['ru-RU'] })).toBe('uk');
    expect(from({ meta: [null], search: '?x=1&lang=en', languages: ['ru-RU'] })).toBe('en');
    expect(from({ languages: ['de-DE', 'uk-UA', 'ru'] })).toBe('uk');
  });
  it('a meta tag with no usable value (a placeholder, another language) lets the next source speak', () => {
    expect(from({ meta: ['', 'de'], search: '?lang=ru' })).toBe('ru');
    expect(from({ meta: [''], languages: ['uk'] })).toBe('uk');
  });
  it('no source at all: English; an unknown ?lang= does not block the browser', () => {
    expect(from({})).toBe('en');
    expect(from({ search: '?lang=xx', languages: ['ru'] })).toBe('ru');
  });
});
