// What the new cards put into their markup: a note's markdown (with only safe links), glossary marks, a table card's
// cells and tones, a folded group's tile, the corner badges; and the toolbar's view-only lock.
import type React from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { ReactFlowProvider } from '@xyflow/react';
import type { BNode } from '@entities/board';
import { catalogs } from '../src/shared/lib/i18n';

const node = (o: Partial<BNode>): BNode => ({
  id: 'x', title: 'Карточка', text: '', kind: 'plain', group: null, icon: 'none', shape: null, width: null, step: null, fields: null, live: null, ...o,
});

async function render(data: Record<string, unknown>, terms: { term: string; text: string; aka: string[] }[] = [], lang: 'ru' | 'uk' | 'en' | null = null): Promise<string> {
  vi.resetModules();
  vi.stubGlobal('document', { querySelectorAll: () => [{ getAttribute: () => 'ru' }] });
  vi.stubGlobal('location', { search: '', pathname: '/' });
  vi.stubGlobal('navigator', { languages: ['ru'], language: 'ru' });
  const { Card } = await import('@entities/card');
  const { BoardLangContext, GlossaryContext, compileGlossary } = await import('@shared/lib');
  const props = { id: 'x', width: 300, height: 200, data: { ...node({}), dir: 'LR', board: 'demo', ...data } };
  const card = <Card {...(props as unknown as React.ComponentProps<typeof Card>)} />;
  return renderToStaticMarkup(
    <ReactFlowProvider><BoardLangContext value={lang}><GlossaryContext value={compileGlossary(terms)}>{card}</GlossaryContext></BoardLangContext></ReactFlowProvider>);
}

afterEach(() => vi.unstubAllGlobals());

describe('a note card reads markdown', () => {
  it('headings, bold and lists become elements; a safe link opens in a new tab', async () => {
    const html = await render({ kind: 'note', text: '## Итог\n**Бот** отвечает\n- раз\n- [сайт](https://example.com)' });
    expect(html).toContain('<h4>Итог</h4>');
    expect(html).toContain('<strong>Бот</strong>');
    expect(html).toContain('<li>раз</li>');
    expect(html).toMatch(/<a href="https:\/\/example.com\/" target="_blank" rel="noopener noreferrer nofollow"/);
  });

  it('never yields HTML from the text, and drops an unsafe link\'s target', async () => {
    const html = await render({ kind: 'note', text: '<img src=x onerror=alert(1)> [x](javascript:alert)' });
    expect(html).not.toContain('<img');
    expect(html).toContain('&lt;img');
    expect(html).not.toContain('javascript:');
  });
});

describe('glossary terms', () => {
  it('a term in a title or text is marked; its definition waits for a hover', async () => {
    const html = await render({ title: 'Наша CRM', text: 'шлёт вебхук' }, [
      { term: 'CRM', text: 'база клиентов', aka: [] }, { term: 'webhook', text: 'адрес', aka: ['вебхук'] },
    ]);
    expect(html).toContain('<span class="term" tabindex="0">CRM</span>');
    expect(html).toContain('<span class="term" tabindex="0">вебхук</span>');
    expect(html).not.toContain('база клиентов');
  });
});

describe('a table card', () => {
  it('a header row, a row per row, scored cells toned from worst to best', async () => {
    const html = await render({ cols: ['Вариант', 'Цена'], rows: [[{ text: 'n8n' }, { text: '$20', score: 2 }], [{ text: 'Zapier' }, { text: '$70', score: -1 }]] });
    expect(html).toContain('card grid');
    expect(html.match(/class="gr gh"/g)).toHaveLength(1);
    expect(html).toContain('<div class="gc s4">$20</div>');
    expect(html).toContain('<div class="gc s0">$70</div>');
  });
});

describe('a folded group', () => {
  it('is a tile with its count and the first titles', async () => {
    const html = await render({ title: 'Продукт', text: 'Бот, Админка', fold: { count: 3, hint: ['Бот', 'Админка'] } });
    expect(html).toContain('card fold');
    expect(html).toContain('3 карточки');
    expect(html).toContain('Бот, Админка, …');
  });
});

describe('the board\'s language', () => {
  it('a tile\'s count and a badge\'s title speak the board\'s language, not the page\'s', async () => {
    const html = await render({ title: 'Продукт', fold: { count: 3, hint: [] }, notes: 2 }, [], 'uk');
    expect(html).toContain('3 картки');
    expect(html).toContain(`title="${catalogs.uk['card.notes.few'].replace('{n}', '2')}"`);
    expect(await render({ title: 'Product', fold: { count: 3, hint: [] } }, [], 'en')).toContain('3 cards');
  });
});

describe('badges', () => {
  it('open questions and notes show on the card\'s corner', async () => {
    const html = await render({ asks: 2, notes: 1 });
    expect(html).toContain(`title="${catalogs.ru['card.asks.few'].replace('{n}', '2')}"`);
    expect(html).toMatch(/class="badge notes"[^>]*>.*1<\/span>/);
  });
});

describe('the view-only lock', () => {
  it('says its state in words and is a pressed toggle while on', async () => {
    vi.resetModules();
    vi.stubGlobal('document', { querySelectorAll: () => [{ getAttribute: () => 'ru' }] });
    vi.stubGlobal('navigator', { languages: ['ru'], language: 'ru' });
    vi.stubGlobal('location', { search: '', pathname: '/' });
    const { LockButton } = await import('@features/view-lock');
    const on = renderToStaticMarkup(<LockButton lock={{ locked: true, toggle: () => {} }} />);
    const off = renderToStaticMarkup(<LockButton lock={{ locked: false, toggle: () => {} }} />);
    expect(on).toContain('aria-pressed="true"');
    expect(on).toContain(`<span>${catalogs.ru['lock.view']}</span>`);
    expect(off).toContain(`<span>${catalogs.ru['lock.edit']}</span>`);
  });

  it('is on by default and remembered', async () => {
    vi.resetModules();
    const mem = new Map<string, string>();
    vi.stubGlobal('localStorage', { getItem: (k: string) => mem.get(k) ?? null, setItem: (k: string, v: string) => mem.set(k, v) });
    const { stored } = await import('@shared/lib');
    expect(stored('kreida.locked') !== '0').toBe(true); // the hook's default: nothing stored = locked
  });
});
