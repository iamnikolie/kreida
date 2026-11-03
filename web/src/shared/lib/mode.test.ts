import { describe, expect, it } from 'vitest';
import type { BoardResp } from '../api/contract';
import { detectMode } from './mode';

const where = (pathname: string, search = '') => ({ pathname, search });
const inlined = { name: 'workflow' } as BoardResp;

describe('detectMode', () => {
  it('editor by default, board from ?b=', () => {
    const m = detectMode(where('/', '?b=workflow'), {});
    expect(m).toMatchObject({ mode: 'editor', readonly: false, capture: false, board: 'workflow', share: null, inlined: null });
  });

  it('a share link is read-only and its key is the token', () => {
    const m = detectMode(where('/s/AbC-123_xyz', '?b=ignored'), {});
    expect(m).toMatchObject({ mode: 'share', readonly: true, share: 'AbC-123_xyz', board: 'AbC-123_xyz' });
  });

  it('a kreida cloud link (/b/<token>) is a share too', () => {
    const m = detectMode(where('/b/AbC-123_xyz'), {});
    expect(m).toMatchObject({ mode: 'share', readonly: true, share: 'AbC-123_xyz', board: 'AbC-123_xyz' });
    expect(detectMode(where('/b/'), {}).mode).toBe('editor');
    expect(detectMode(where('/boards/abc'), {}).mode).toBe('editor');
  });

  it('only a /b/ link is the cloud page: not a local /s/ link, not the editor, not an export', () => {
    expect(detectMode(where('/b/AbC-123_xyz'), {}).cloud).toBe(true);
    expect(detectMode(where('/s/AbC-123_xyz'), {}).cloud).toBe(false);
    expect(detectMode(where('/', '?b=workflow'), {}).cloud).toBe(false);
    expect(detectMode(where('/b/'), {}).cloud).toBe(false);
    expect(detectMode(where('/b/AbC-123_xyz'), { __KREIDA__: inlined }).cloud).toBe(false);
  });

  it('an exported page carries its board and is read-only', () => {
    const m = detectMode(where('/', ''), { __KREIDA__: inlined });
    expect(m).toMatchObject({ mode: 'static', readonly: true, board: 'workflow', inlined });
  });

  it('the inlined board wins over a /s/ path', () => {
    expect(detectMode(where('/s/abcdefghijklmnop'), { __KREIDA__: inlined }).mode).toBe('static');
  });

  it('?static marks a headless capture, whatever the mode', () => {
    expect(detectMode(where('/', '?b=x&static=1'), {}).capture).toBe(true);
    expect(detectMode(where('/s/abc', '?static'), {}).capture).toBe(true);
    expect(detectMode(where('/', '?b=x'), {}).capture).toBe(false);
  });

  it('keeps the parsed query for theme, present and step', () => {
    const m = detectMode(where('/', '?b=x&theme=dark&present=1&step=2'), {});
    expect([m.query.get('theme'), m.query.has('present'), m.query.get('step')]).toEqual(['dark', true, '2']);
  });

  it('no board named: board is null', () => {
    expect(detectMode(where('/'), {}).board).toBeNull();
  });
});
