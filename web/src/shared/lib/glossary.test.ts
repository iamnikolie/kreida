import { describe, expect, it } from 'vitest';
import { compileGlossary } from './glossary';

const terms = [
  { term: 'CRM', text: 'база клиентов', aka: [] },
  { term: 'webhook', text: 'адрес для событий', aka: ['вебхук', 'вебхуки'] },
  { term: 'CRM система', text: 'вся система', aka: [] },
];

describe('compileGlossary', () => {
  const split = compileGlossary(terms);

  it('marks terms case-insensitively, keeping the text as written', () => {
    expect(split('в crm и Webhook')).toEqual([
      'в ', { text: 'crm', term: terms[0] }, ' и ', { text: 'Webhook', term: terms[1] },
    ]);
  });

  it('prefers the longest match and reads aliases', () => {
    expect(split('CRM система шлёт вебхуки')).toEqual([
      { text: 'CRM система', term: terms[2] }, ' шлёт ', { text: 'вебхуки', term: terms[1] },
    ]);
  });

  it('only whole words', () => {
    expect(split('CRMка и webhooks')).toEqual(['CRMка и webhooks']);
  });

  it('no terms: the text as is', () => {
    expect(compileGlossary([])('abc')).toEqual(['abc']);
    expect(split('')).toEqual([]);
  });

  it('regex characters in a term are literal', () => {
    const t = { term: 'C++', text: 'язык', aka: [] };
    expect(compileGlossary([t])('на C++ и C')).toEqual(['на ', { text: 'C++', term: t }, ' и C']);
  });
});
