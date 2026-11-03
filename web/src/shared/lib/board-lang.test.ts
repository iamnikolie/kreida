import { describe, expect, it } from 'vitest';
import type { Board } from '../api/contract';
import { boardLang } from './board-lang';

const board = (texts: string[], lang?: Board['lang']): Board => ({
  title: texts[0] ?? '', sub: '', direction: 'LR', layout: 'auto', style: 'clean', legend: [], groups: [], edges: [], steps: [], lang,
  nodes: texts.slice(1).map((t, i) => ({ id: `n${i}`, title: t, text: '', kind: 'plain', group: null, icon: null, shape: null, width: null, step: null, fields: null, live: null })),
});

describe('boardLang', () => {
  it('the board file says it', () => {
    expect(boardLang(board(['Ассистент'], 'en'))).toBe('en');
  });

  it('Cyrillic with і ї є ґ reads as Ukrainian, other Cyrillic as Russian', () => {
    expect(boardLang(board(['Ассистент для агентства', 'Заявка в Telegram']))).toBe('ru');
    expect(boardLang(board(['Асистент для агенції', 'Заявка в Telegram']))).toBe('uk');
  });

  it('Latin text reads as English; a board with no words has none', () => {
    expect(boardLang(board(['Release check', 'Manual smoke test']))).toBe('en');
    expect(boardLang(board(['', '123']))).toBeNull();
  });
});
