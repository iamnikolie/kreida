// A card on a phone keeps all its content (D-2026-10-29-02 amendment): title, text, icon, table fields and above all a live
// chip, which never drops. The layout reserves the chip's height (a narrow card puts its label and value on more lines), the
// markup carries it, and the CSS lets a card grow rather than cut the value short.
import type React from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';
import { renderToStaticMarkup } from 'react-dom/server';
import { ReactFlowProvider } from '@xyflow/react';
import { layout, type Board, type BNode } from '@entities/board';
import { catalogs } from '../src/shared/lib/i18n';

globalThis.innerWidth = 390; globalThis.innerHeight = 844;

const node = (id: string, o: Partial<BNode>): BNode => ({
  id, title: 'MySQL', text: '', kind: 'core', group: null, icon: null, shape: null, width: null, step: null, fields: null, live: null, ...o,
});
const live = (label: string) => ({ source: 'cmd:echo 42', every: 5, path: null, label });
const board = (nodes: BNode[]): Board => ({
  title: '', sub: '', direction: 'TB', layout: 'auto', style: 'clean', legend: [], groups: [], nodes, edges: [], steps: [],
});
const heights = async (nodes: BNode[], phone: boolean) =>
  Object.fromEntries((await layout(board(nodes), {}, 'x', { phone })).nodes.map(n => [n.id, Number(n.style!.height)]));

describe('a live card on a phone has the height for its chip', () => {
  it('a short label fits the chip on one line: the card is as tall as on the desktop', async () => {
    const n = [node('a', { live: live('rows') }), node('b', { icon: 'database', live: live('rows') }), node('c', {})];
    const phone = await heights(n, true), desktop = await heights(n, false);
    expect(phone.a).toBe(desktop.a);
    expect(phone.b).toBe(desktop.b);
    expect(phone.a).toBe(30 + 21 + 30); // padding, one title line, the chip's 30 px: more than the 58 px a plain card has
    expect(phone.a).toBeGreaterThan(phone.c);
  });

  it('a label too long for the card\'s 168 px takes more lines, and so does the card', async () => {
    const n = [node('a', { title: 'Queue', live: live('Задач в очереди') }), node('b', { title: 'Queue', live: live('rows') })];
    const phone = await heights(n, true), desktop = await heights(n, false);
    expect(phone.a).toBeGreaterThan(phone.b);
    expect(phone.a).toBeGreaterThan(desktop.a);
    expect(desktop.a).toBe(desktop.b);
  });

  it('a pill (a who card, a rounded one) counts its chip on a phone, where a value must not drop', async () => {
    const n = [node('a', { kind: 'who', title: 'Reviewer', live: live('status') }), node('b', { kind: 'who', title: 'Reviewer' })];
    const phone = await heights(n, true);
    expect(phone.a - phone.b).toBeGreaterThanOrEqual(30);
  });

  it('an icon takes its column from the text, so a text beside one needs more height', async () => {
    const text = 'сколько задач сейчас ждёт обработки и сколько уже сделано';
    const n = [node('a', { title: 'Queue', text, icon: 'inbox' }), node('b', { title: 'Queue', text })]; // (a title like MySQL has a logo of its own)
    const phone = await heights(n, true);
    expect(phone.a).toBeGreaterThan(phone.b);
  });
});

describe('a card on a phone keeps all its content in its markup', () => {
  afterEach(() => vi.unstubAllGlobals());

  async function render(data: Record<string, unknown>): Promise<string> {
    vi.resetModules();
    vi.stubGlobal('document', { querySelectorAll: () => [{ getAttribute: () => 'ru' }] });
    vi.stubGlobal('location', { search: '', pathname: '/b/AbCdEf123456' });
    vi.stubGlobal('navigator', { languages: ['ru'], language: 'ru' });
    const { Card } = await import('@entities/card');
    const props = { id: 'x', width: 168, height: 120, data: { ...node('x', {}), dir: 'TB', board: 'demo', ...data } };
    const card = <Card {...(props as unknown as React.ComponentProps<typeof Card>)} />;
    return renderToStaticMarkup(<ReactFlowProvider>{card}</ReactFlowProvider>);
  }

  const chip = (html: string) => html.match(/<div class="live[^"]*"[^>]*>.*?<\/div>/)?.[0] ?? '';

  it.each<[string, { kind: string; icon?: string; text?: string }]>([
    ['a plain card', { kind: 'core' }],
    ['a card with an icon and a text', { kind: 'src', icon: 'inbox', text: 'сколько задач ждёт' }],
    ['a pill', { kind: 'who', icon: 'user', text: 'дежурный' }],
    ['a note', { kind: 'note', text: 'короткий текст' }],
  ])('%s: title, text, icon and the live chip with its label and value', async (_, extra) => {
    const html = await render({ title: 'Очередь', live: live('Задач в очереди'), liveVal: { value: '1204' }, ...extra });
    expect(html).toContain('Очередь');
    if (extra.text) expect(html).toContain(extra.text);
    if (extra.icon) expect(html).toContain('<svg');
    expect(chip(html)).toContain('Задач в очереди');
    expect(chip(html)).toContain('<b>1204</b>');
  });

  it('a live card whose value has not come yet shows its chip with an ellipsis, not nothing', async () => {
    const html = await render({ live: live('rows') });
    expect(chip(html)).toContain('<b>…</b>');
  });

  it('a blocked or failed source shows its word in the chip', async () => {
    expect(chip(await render({ live: live('rows'), liveVal: { blocked: true } }))).toContain(catalogs.ru['live.blocked']);
    expect(chip(await render({ live: live('rows'), liveVal: { error: 'boom' } }))).toContain(catalogs.ru['live.error']);
  });

  it('a table keeps its title, its text and every field with its note and reference', async () => {
    const html = await render({ title: 'users', text: 'кто в системе', fields: [
      { name: 'id', ref: null, dashed: false, note: '' }, { name: 'email', ref: null, dashed: false, note: 'логин' }, { name: 'city_id', ref: 'cities', dashed: false, note: '' }] });
    for (const s of ['users', 'кто в системе', '>id<', '>email<', 'логин', '>city_id<', 'cities']) expect(html, s).toContain(s);
  });
});

describe('the CSS lets a card on a phone grow and never ellipsizes a live value', () => {
  const css = fs.readFileSync(path.resolve(__dirname, '../src/entities/card/ui/card.css'), 'utf8');
  const rule = (selector: string) => css.match(new RegExp(`${selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\s*\\{([^}]*)\\}`))?.[1] ?? '';

  it('a phone card is at least its box, and more when its content needs it', () => {
    expect(rule('.phone .card')).toMatch(/height:\s*auto/);
    expect(rule('.phone .card')).toMatch(/min-height:\s*100%/);
  });

  it('the chip flows onto lines and its value shows whole', () => {
    expect(rule('.phone .card .live')).toMatch(/display:\s*inline-block/);
    expect(rule('.phone .card .live b')).toMatch(/overflow:\s*visible/);
    expect(rule('.phone .card .live b')).toMatch(/white-space:\s*normal/);
    expect(rule('.phone .card .live b')).not.toMatch(/ellipsis/);
  });
});

// a wide table card on a phone keeps a readable width and scrolls sideways inside its card.
describe('a wide table on a phone', () => {
  const c = (text: string) => ({ text });
  const table = node('t', { kind: 'plain', title: 'Пять путей', width: 820, cols: ['Вариант', 'В месяц', 'Запуск', 'Риск', 'Агентам'], rows: [
    [c('Fibery Free, базы слить'), c('$0'), c('полдня'), c('лимит 10 баз'), c('как сейчас')],
    [c('Своё, агентный трекер'), c('$0 + токены'), c('1,5–2 недели'), c('свой прод'), c('лучше всех')]] });

  it('is laid out at the phone\'s table width and tells the card it is on a phone', async () => {
    const { nodes } = await layout(board([table]), {}, 'x', { phone: true });
    expect(Number(nodes[0].style!.width)).toBe(320);
    expect(nodes[0].data.phone).toBe(true);
    const desk = await layout(board([table]), {}, 'x', { phone: false });
    expect(Number(desk.nodes[0].style!.width)).toBe(820);
    expect(desk.nodes[0].data.phone).toBeUndefined();
  });

  it('the CSS never breaks a word and holds the row names while the rest scrolls', () => {
    const css = fs.readFileSync(path.resolve(__dirname, '../src/entities/card/ui/card.css'), 'utf8');
    const rule = (selector: string) => css.match(new RegExp(`${selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\s*\\{([^}]*)\\}`))?.[1] ?? '';
    expect(rule('.phone .card.grid .gc')).toMatch(/overflow-wrap:\s*normal/);
    expect(css).not.toMatch(/word-break:\s*break-all/);
    expect(rule('.phone .card.grid .gs.scroll')).toMatch(/overflow-x:\s*auto/);
    expect(rule('.phone .card.grid .gs.scroll')).toMatch(/overscroll-behavior-x:\s*contain/);
    expect(rule('.phone .card.grid .gs.scroll .gc:first-child')).toMatch(/position:\s*sticky/);
  });
});
