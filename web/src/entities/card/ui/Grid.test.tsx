// a table card on a phone. Its rows take the height their text needs at the width they are drawn at (the layout's
// is a floor), words never break mid-word, and a table wider than the card scrolls sideways with its row names held.
import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { ReactFlowProvider } from '@xyflow/react';
import type { BNode } from '@shared/api';
import { Grid } from './Grid';
import type { CardData } from '../model/types';

const c = (text: string) => ({ text });
const data = (o: Partial<BNode>): CardData => ({
  id: 't', title: 'Пять путей', text: '', kind: 'plain', group: null, icon: null, shape: null, width: 820, step: null, fields: null, live: null,
  cols: ['Вариант', 'В месяц', 'Запуск', 'Риск', 'Агентам'],
  rows: [[c('Своё, агентный трекер'), c('$0 + токены'), c('1,5–2 недели'), c('свой прод'), c('лучше всех')]],
  dir: 'LR', board: 'b', ...o,
});
const draw = (phone: boolean, o: Partial<BNode> = {}) =>
  renderToStaticMarkup(<ReactFlowProvider><Grid data={{ ...data(o), phone }} Icon={null} rough={null} /></ReactFlowProvider>);

describe('a table card', () => {
  it('on the desktop has fixed rows and no scroller', () => {
    const html = draw(false);
    expect(html).not.toContain('min-height');
    expect(html).not.toContain('scroll');
    expect(html).toMatch(/class="gr" style="grid-template-columns:[^"]*;height:\d+px"/);
  });

  it('on a phone grows its rows with their text and scrolls a wide table', () => {
    const html = draw(true);
    expect(html).toContain('class="gs scroll nopan nodrag"');
    expect(html).toMatch(/class="gr" style="grid-template-columns:[^"]*;min-height:\d+px;width:\d+(\.\d+)?px"/);
    expect(html).not.toMatch(/[;"]height:/);
  });

  it('on a phone a narrow table does not scroll', () => {
    const html = draw(true, { width: null, cols: ['A', 'B'], rows: [[c('x'), c('y')]] });
    expect(html).toContain('class="gs"');
    expect(html).toContain('min-height');
  });
});
