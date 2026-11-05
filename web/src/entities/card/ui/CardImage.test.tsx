import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { ApiContext, createApi, type BImage } from '@shared/api';
import { BoardLangContext, detectMode } from '@shared/lib';
import type { BoardResp } from '@shared/api';
import { CardImage } from './CardImage';

const pic: BImage = { sha256: 'abc', type: 'image/png', w: 200, h: 100 };
const draw = (image: BImage, mode: ReturnType<typeof detectMode>, width = 300) => renderToStaticMarkup(
  <BoardLangContext.Provider value="en"><ApiContext.Provider value={createApi(mode)}><CardImage image={image} board="demo" width={width} /></ApiContext.Provider></BoardLangContext.Provider>);

const editor = detectMode({ pathname: '/', search: '?b=demo' }, {});
const share = detectMode({ pathname: '/s/tok-0123456789abcdef', search: '' }, {});
const exported = detectMode({ pathname: '/', search: '' }, { __KREIDA__: { name: 'demo', images: { abc: 'data:image/png;base64,QUJD' } } as BoardResp });

describe('a card picture', () => {
  it('comes from the route of the page it is on', () => {
    expect(draw(pic, editor)).toContain('src="/api/boards/demo/images/abc"');
    expect(draw(pic, share)).toContain('src="/api/share/tok-0123456789abcdef/images/abc"');
    expect(draw(pic, exported)).toContain('src="data:image/png;base64,QUJD"');
  });

  it('takes the height the layout reserved', () => {
    expect(draw(pic, editor)).toContain('height:131px');
  });

  it('says why it cannot be shown, and shows no address', () => {
    const html = draw({ error: 'missing' }, editor);
    expect(html).toContain('cimg bad');
    expect(html).toContain('File not found');
    expect(html).not.toContain('<img');
  });
});
