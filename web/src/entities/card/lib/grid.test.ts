import { describe, expect, it } from 'vitest';
import type { BNode } from '@shared/api';
import { gridGeom, toneOf } from './grid';
import { PHONE_TABLE_MAX, size } from './geometry';

const node = (o: Partial<BNode>): BNode => ({
  id: 't', title: 'Варианты', text: '', kind: 'plain', group: null, icon: null, shape: null, width: null, step: null, fields: null, live: null, ...o,
});

describe('a table card (cols/rows)', () => {
  const n = node({ cols: ['Вариант', 'Цена'], rows: [[{ text: 'n8n' }, { text: '$20' }], [{ text: 'Zapier с очень длинным описанием, которое переносится' }, { text: '$70' }]] });

  it('its size is its columns and rows', () => {
    const g = gridGeom(n);
    expect(g.cols).toHaveLength(2);
    expect(g.width).toBe(Math.max(g.cols.reduce((a, b) => a + b, 0), g.width));
    expect(g.rows[1]).toBeGreaterThan(g.rows[0]); // the long cell wraps
    expect(size(n)).toEqual({ width: g.width, height: g.height });
    expect(g.height).toBe(g.head + g.header + g.rows[0] + g.rows[1] + g.pad);
  });

  it('a given width is shared out among the columns', () => {
    const g = gridGeom({ ...n, width: 600 });
    expect(g.width).toBe(600);
    expect(g.cols.reduce((a, b) => a + b, 0)).toBeCloseTo(600, 0);
  });

  // React Flow hands the card its measured width, a whole number of px. A table whose columns are exactly as wide as
  // their longest cell must not wrap those cells when drawn at that width, or the drawn rows outgrow the reserved card.
  const c = (text: string, score?: number) => ({ text, score });
  const paths = node({
    title: 'Пять путей', cols: ['Вариант', 'В месяц', 'Запуск', 'Риск', 'Агентам'],
    rows: [
      [c('Fibery Free, базы слить'), c('$0', 3), c('полдня', 3), c('лимит 10 баз', 1), c('как сейчас', 3)],
      [c('Fibery Standard'), c('$12×N, −50% UA', 0), c('0', 3), c('растёт с людьми', 1), c('как сейчас', 3)],
      [c('GitLab Issues'), c('$0', 3), c('1 день', 2), c('блокеры платные', 0), c('gl CLI есть', 2)],
      [c('Plane self-hosted'), c('$0 + сервер', 2), c('1–2 дня', 1), c('тяжёлый стек', -1), c('нужна обёртка', 1)],
      [c('Своё, агентный трекер'), c('$0 + токены', 1), c('1,5–2 недели', -2), c('свой прод', -1), c('лучше всех', 3)],
    ],
  });

  it('is a whole number of px wide, so the measured width draws the same rows', () => {
    const g = gridGeom(paths);
    expect(Number.isInteger(g.width)).toBe(true);
    const drawn = gridGeom({ ...paths, width: Math.round(g.width) });
    expect(drawn.rows).toEqual(g.rows);
    expect(drawn.height).toBe(g.height);
  });

  it('a cell exactly as wide as its column stays on one line', () => {
    expect(gridGeom(paths).rows).toEqual([30, 30, 30, 30, 30]);
  });

  // on a phone the card is at most PHONE_TABLE_MAX wide, and a wide table keeps its own readable width inside it and
  // scrolls sideways instead of being squeezed until words break mid-word.
  it('on a phone a wide table keeps its columns and scrolls inside the card', () => {
    const authored = { ...paths, width: 820 }, desk = gridGeom(paths), g = gridGeom(authored, { view: PHONE_TABLE_MAX });
    expect(g.width).toBe(PHONE_TABLE_MAX);
    expect(g.inner).toBeGreaterThan(g.width);
    g.cols.slice(1).forEach((w, i) => expect(w).toBeCloseTo(desk.cols[i + 1], 0)); // the desktop's one-line columns: no word is cut
    expect(g.cols[0]).toBeLessThanOrEqual(PHONE_TABLE_MAX * 0.4); // the sticky row names leave room for the rest
    expect(g.cols[0]).toBeGreaterThanOrEqual('агентный'.length * 8.5 + 22); // but never cut a word either
    expect(g.inner).toBeCloseTo(g.cols.reduce((a, b) => a + b, 0), 6);
    expect(g.rows[4]).toBeGreaterThan(desk.rows[4]); // «Своё, агентный трекер» wraps in its narrower column
    expect(size(authored, { phone: true })).toEqual({ width: g.width, height: g.height });
  });

  it('on a phone a table that fits stays as on the desktop', () => {
    const g = gridGeom(n, { view: PHONE_TABLE_MAX });
    expect(g.width).toBe(gridGeom(n).width);
    expect(g.inner).toBe(g.width);
    expect(g.rows).toEqual(gridGeom(n).rows);
  });

  it('on the desktop the table is as wide as its card', () => {
    const g = gridGeom(paths);
    expect(g.inner).toBe(g.width);
  });

  it('without cols there is no header row', () => {
    expect(gridGeom(node({ rows: [[{ text: 'a' }]] })).header).toBe(0);
  });
});

describe('toneOf', () => {
  it('spreads the table\'s scores from bad to good', () => {
    const scores = [-1, 0, 1, 2, 3];
    expect(scores.map(s => toneOf(s, scores))).toEqual([0, 1, 2, 3, 4]);
  });

  it('no score: no tone; one score: its sign', () => {
    expect(toneOf(undefined, [1, 2])).toBeNull();
    expect([toneOf(5, [5]), toneOf(-2, [-2]), toneOf(0, [0])]).toEqual([4, 0, 2]);
  });
});
