// Whose board it is, on a kreida cloud link: the owner's name under the title, «Сделано в kreida» on the /b/ page only.
// Rendered to markup on the server side of react-dom: no browser needed.
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ReactNode } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { ReactFlowProvider } from '@xyflow/react';
import { ownerOf } from '@entities/board';
import type { Head } from '@widgets/head';
import type { Sheet } from '../src/widgets/phone-bar/ui/Sheet';
import type { MadeWith } from '@shared/ui';
import { catalogs, type Lang } from '../src/shared/lib/i18n';

interface Page { lang?: Lang; path?: string; search?: string }
type Parts = { Head: typeof Head; Sheet: typeof Sheet; MadeWith: typeof MadeWith };

/** Renders `ui` as a page opened at `path` in language `lang` would. The page language is read once per module load. */
async function page(opts: Page, ui: (m: Parts) => ReactNode): Promise<string> {
  const { lang = 'en', path = '/b/AbCdEf123456', search = '' } = opts;
  vi.resetModules();
  vi.stubGlobal('document', { querySelectorAll: () => [{ getAttribute: () => lang }] });
  vi.stubGlobal('location', { search, pathname: path });
  vi.stubGlobal('navigator', { languages: ['en'], language: 'en' });
  const { Head } = await import('@widgets/head');
  const { Sheet } = await import('../src/widgets/phone-bar/ui/Sheet');
  const { MadeWith } = await import('@shared/ui');
  const { ModeContext, detectMode } = await import('@shared/lib');
  const tree = ui({ Head, Sheet, MadeWith });
  return renderToStaticMarkup(
    <ModeContext value={detectMode({ pathname: path, search }, {})}>
      <ReactFlowProvider>{tree}</ReactFlowProvider>
    </ModeContext>,
  );
}

const board = { title: 'Заявка', sub: 'Как это работает' } as never;
const noSwitch = {} as never;

const head = (owner: string, o: Page = {}) => page(o, ({ Head }) => <Head board={board} owner={owner} presenting={false} switcher={noSwitch} />);
const sheet = (o: Page = {}) => page(o, ({ Sheet }) => <Sheet theme={{ theme: 'light', toggle() {} } as never} fileName="demo" legend={null} onClose={() => {}} />);
const link = (o: Page = {}) => page(o, ({ MadeWith }) => <MadeWith />);

afterEach(() => vi.unstubAllGlobals());

describe('the owner line', () => {
  it.each(['ru', 'uk', 'en'] as Lang[])('%s: under the title and the subtitle, in the catalog\'s words', async lang => {
    const html = await head('Alex Example', { lang });
    const line = catalogs[lang]['head.from'].replace('{name}', 'Alex Example');
    expect(html).toContain(`<div class="from">${line}</div>`);
    expect(html.indexOf('class="sub"')).toBeLessThan(html.indexOf('class="from"'));
    expect(html.indexOf('<h1>')).toBeLessThan(html.indexOf('class="from"'));
    expect(html).toContain('head owned');
  });

  it('the catalog words: От: / Від: / From, the name is in the nominative as given', () => {
    expect(catalogs.ru['head.from']).toBe('От: {name}');
    expect(catalogs.uk['head.from']).toBe('Від: {name}');
    expect(catalogs.en['head.from']).toBe('From {name}');
  });

  it('no name, no line', async () => {
    const html = await head('');
    expect(html).not.toContain('class="from"');
    expect(html).not.toContain('owned');
    expect(html).toContain('<h1>Заявка</h1>');
  });

  it('a share link shows the name cloud sent; absent, empty or blank is none; the editor and an export never show one', () => {
    expect(ownerOf({ owner: ' Alex Example ' }, 'share')).toBe('Alex Example');
    expect(ownerOf({}, 'share')).toBe('');
    expect(ownerOf({ owner: '' }, 'share')).toBe('');
    expect(ownerOf({ owner: '  ' }, 'share')).toBe('');
    expect(ownerOf(null, 'share')).toBe('');
    expect(ownerOf({ owner: 'Alex Example' }, 'editor')).toBe('');
    expect(ownerOf({ owner: 'Alex Example' }, 'static')).toBe('');
  });

  it('is built from the name as text, never as markup', async () => {
    expect(await head('<b>x</b> & co')).toContain('From &lt;b&gt;x&lt;/b&gt; &amp; co');
  });

  it('stays with the title while presenting', async () => {
    const html = await page({}, ({ Head }) => <Head board={board} owner="Alex Example" presenting switcher={noSwitch} />);
    expect(html).toContain('From Alex Example');
    expect(html).toContain('<h1>Заявка</h1>');
  });
});

describe('«Сделано в kreida»', () => {
  it.each(['ru', 'uk', 'en'] as Lang[])('%s: on the cloud page, a link to / that leaves an iframe', async lang => {
    const html = await link({ lang });
    expect(html).toMatch(/<a [^>]*href="\/"[^>]*>/);
    expect(html).toContain('target="_top"');
    expect(html).toContain(`>${catalogs[lang]['credit.made']}</a>`);
  });

  it('the words: Сделано в / Зроблено в / Made with kreida', () => {
    expect([catalogs.ru['credit.made'], catalogs.uk['credit.made'], catalogs.en['credit.made']]).toEqual(['Сделано в kreida', 'Зроблено в kreida', 'Made with kreida']);
  });

  it('only on /b/: not on the local /s/ link, in the editor or in a headless capture', async () => {
    expect(await link({ path: '/s/AbCdEf123456' })).toBe('');
    expect(await link({ path: '/', search: '?b=workflow' })).toBe('');
    expect(await link({ search: '?static' })).toBe('');
  });

  it('the phone sheet carries it at its foot, on /b/ only', async () => {
    const html = await sheet({ lang: 'uk' });
    expect(html).toContain('class="made-with foot"');
    expect(html).toContain('Зроблено в kreida');
    expect(html.lastIndexOf('made-with')).toBeGreaterThan(html.indexOf(catalogs.uk['sheet.png']));
    expect(await sheet({ path: '/s/AbCdEf123456' })).not.toContain('made-with');
  });
});
