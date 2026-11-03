// The share link's toolbar speaks the language the page was served in: the `kreida-lang` meta tag (kreida cloud writes
// the link's language there) wins over the browser's. Rendered to markup on the server side of react-dom: no browser needed.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { ReactFlowProvider } from '@xyflow/react';
import { catalogs, type Lang } from '../src/shared/lib/i18n';

/** The share-mode toolbar as a page with this meta tag and browser language renders it. */
async function toolbar(meta: string | null, languages: string[]): Promise<string> {
  vi.resetModules(); // the page language is read once per module load
  vi.stubGlobal('document', { querySelectorAll: () => (meta ? [{ getAttribute: () => meta }] : []) });
  vi.stubGlobal('location', { search: '', pathname: '/b/AbCdEf123456' });
  vi.stubGlobal('navigator', { languages, language: languages[0] });
  const { Toolbar } = await import('@widgets/toolbar');
  const { ModeContext, detectMode } = await import('@shared/lib');
  const off = { on: false, toggle() {} };
  return renderToStaticMarkup(
    <ModeContext value={detectMode({ pathname: '/b/AbCdEf123456', search: '' }, {})}>
      <ReactFlowProvider>
        <Toolbar board={null} fileName="demo" undo={async () => {}} minimap={off} sketch={off}
          theme={{ theme: 'light', toggle() {} } as never} presentation={{ on: false, start() {} } as never} />
      </ReactFlowProvider>
    </ModeContext>,
  );
}

const SHARED = ['tool.fit', 'tool.minimap', 'tool.theme', 'tool.sketch', 'tool.png', 'tool.present'];
// attribute values as markup writes them
const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

beforeEach(() => vi.unstubAllGlobals());
afterEach(() => vi.unstubAllGlobals());

// the first render imports the whole toolbar cold (with the fold levels, the board entity and its layout engine)
describe('the share-mode toolbar', { timeout: 20000 }, () => {
  it('renders uk titles and aria-labels when the meta says uk, whatever the browser speaks', async () => {
    const html = await toolbar('uk', ['ru-RU', 'en-US']);
    for (const key of SHARED) {
      const label = esc(catalogs.uk[key]);
      expect(html, key).toContain(`title="${label}"`);
      expect(html, key).toContain(`aria-label="${label}"`);
    }
    expect(html).toContain(`aria-label="${catalogs.uk['tool.bar']}"`);
    // nothing of the other languages
    for (const l of ['ru', 'en'] as Lang[]) for (const key of SHARED.filter(k => catalogs[l][k] !== catalogs.uk[k]))
      expect(html, `${l}:${key}`).not.toContain(esc(catalogs[l][key]));
  });

  it('is in the language of the meta in ru and en too', async () => {
    for (const l of ['ru', 'en'] as Lang[]) {
      const html = await toolbar(l, ['uk']);
      for (const key of SHARED) expect(html, `${l}:${key}`).toContain(`title="${esc(catalogs[l][key])}"`);
    }
  });

  it('without a meta tag follows the browser, and without either is English', async () => {
    expect(await toolbar(null, ['uk-UA'])).toContain(`title="${catalogs.uk['tool.fit']}"`);
    expect(await toolbar('', ['de-DE'])).toContain(`title="${catalogs.en['tool.fit']}"`);
  });

  it('a share link has no editing tools', async () => {
    const html = await toolbar('uk', []);
    expect(html).not.toContain(catalogs.uk['tool.undo']);
    expect(html).not.toContain(catalogs.uk['tool.relayout']);
  });
});
