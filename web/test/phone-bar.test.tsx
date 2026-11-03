// The phone's bottom bar: with the whole board on show the steps read «всё» (not "– / 3"), ‹ is off and › starts step 1.
import { afterEach, describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { ReactFlowProvider } from '@xyflow/react';
import { catalogs, type Lang } from '../src/shared/lib/i18n';

async function bar(lang: Lang, presentation: { step: number | null; stepped?: boolean }): Promise<string> {
  vi.resetModules();
  vi.stubGlobal('document', { querySelectorAll: () => [{ getAttribute: () => lang }] });
  vi.stubGlobal('location', { search: '', pathname: '/b/AbCdEf123456' });
  vi.stubGlobal('navigator', { languages: ['en'], language: 'en' });
  const { PhoneBar } = await import('@widgets/phone-bar');
  const { ModeContext, detectMode } = await import('@shared/lib');
  const p = { stepped: true, steps: [{}, {}, {}], go() {}, showAll() {}, ...presentation };
  return renderToStaticMarkup(
    <ModeContext value={detectMode({ pathname: '/b/AbCdEf123456', search: '' }, {})}>
      <ReactFlowProvider>
        <PhoneBar presentation={p as never} theme={{ theme: 'light', toggle() {} } as never} fileName="demo" legend={null} />
      </ReactFlowProvider>
    </ModeContext>,
  );
}

const button = (html: string, label: string) => html.match(new RegExp(`<button[^>]*aria-label="${label}"[^>]*>`))?.[0] ?? '';

afterEach(() => vi.unstubAllGlobals());

describe('the phone bar counter', () => {
  it.each(['ru', 'uk', 'en'] as Lang[])('%s: the whole board says its word, previous is off, next is on', async lang => {
    const html = await bar(lang, { step: null });
    expect(html).toContain(`>${catalogs[lang]['bar.none']}</span>`);
    expect(html).not.toContain('–');
    expect(html).not.toContain('/ 3');
    expect(button(html, catalogs[lang]['bar.prev'])).toContain('disabled');
    expect(button(html, catalogs[lang]['bar.next'])).not.toContain('disabled');
  });

  it('on a step it counts, and both arrows work', async () => {
    const html = await bar('uk', { step: 1 });
    expect(html).toContain('>2 / 3</span>');
    expect(html).toContain(`aria-label="${catalogs.uk['bar.step'].replace('{n}', '2').replace('{total}', '3')}"`);
    expect(html).not.toContain(`>${catalogs.uk['bar.none']}</span>`);
    expect(button(html, catalogs.uk['bar.prev'])).not.toContain('disabled');
    expect(button(html, catalogs.uk['bar.next'])).not.toContain('disabled');
  });

  it('a board without steps has no counter at all', async () => {
    const html = await bar('en', { step: null, stepped: false });
    expect(html).not.toContain('stepper');
  });
});
