import { describe, expect, it } from 'vitest';
import { blocks, inline, safeHref } from './markdown';

describe('safeHref', () => {
  it('lets http(s) and mailto through', () => {
    expect(safeHref('https://example.com/a?b=1')).toBe('https://example.com/a?b=1');
    expect(safeHref('http://example.com')).toBe('http://example.com/');
    expect(safeHref('mailto:kate@example.com')).toBe('mailto:kate@example.com');
  });

  it('refuses scripts, data, relative and broken links', () => {
    for (const h of ['javascript:alert(1)', ' JavaScript:alert(1)', 'data:text/html,x', 'vbscript:x', '/local', 'example.com', '', 'http//x'])
      expect(safeHref(h), h).toBeNull();
  });
});

describe('inline', () => {
  it('reads bold, italic, code and links', () => {
    expect(inline('a **b** *c* `d` [e](https://x.io) f')).toEqual([
      { t: 'text', s: 'a ' }, { t: 'b', s: 'b' }, { t: 'text', s: ' ' }, { t: 'i', s: 'c' }, { t: 'text', s: ' ' },
      { t: 'code', s: 'd' }, { t: 'text', s: ' ' }, { t: 'a', s: 'e', href: 'https://x.io/' }, { t: 'text', s: ' f' },
    ]);
  });

  it('a link to an unsafe target stays plain text', () => {
    expect(inline('[click](javascript:void0)')).toEqual([{ t: 'text', s: 'click' }]);
  });

  it('an unclosed mark is text', () => {
    expect(inline('2 * 3 = **6')).toEqual([{ t: 'text', s: '2 * 3 = **6' }]);
  });
});

describe('blocks', () => {
  it('reads headings, lists and paragraphs', () => {
    expect(blocks('## Итог\nБот отвечает\nсам.\n\n- раз\n- два\n1. первый\n2. второй')).toEqual([
      { t: 'h', c: [{ t: 'text', s: 'Итог' }] },
      { t: 'p', c: [{ t: 'text', s: 'Бот отвечает сам.' }] },
      { t: 'ul', items: [[{ t: 'text', s: 'раз' }], [{ t: 'text', s: 'два' }]] },
      { t: 'ol', items: [[{ t: 'text', s: 'первый' }], [{ t: 'text', s: 'второй' }]] },
    ]);
  });

  it('plain text is one paragraph', () => {
    expect(blocks('просто текст')).toEqual([{ t: 'p', c: [{ t: 'text', s: 'просто текст' }] }]);
    expect(blocks('')).toEqual([]);
  });
});
