import { describe, expect, it } from 'vitest';
import type { Node } from '@xyflow/react';
import { boundsOf } from './bounds';

const node = (id: string, x: number, y: number, w: number, h: number, parentId?: string): Node =>
  ({ id, position: { x, y }, style: { width: w, height: h }, data: {}, ...(parentId ? { parentId } : {}) });

describe('boundsOf', () => {
  it('wraps the nodes', () => {
    expect(boundsOf([node('a', 0, 0, 100, 50), node('b', 200, 300, 40, 20)])).toEqual({ x: 0, y: 0, width: 240, height: 320 });
  });

  it('a child sits relative to its group, however deep', () => {
    const nodes = [node('g', 100, 100, 400, 400), node('h', 10, 20, 300, 300, 'g'), node('c', 5, 5, 50, 50, 'h')];
    expect(boundsOf(nodes, id => id === 'c')).toEqual({ x: 115, y: 125, width: 50, height: 50 });
  });

  it('only the picked nodes count', () => {
    expect(boundsOf([node('a', 0, 0, 10, 10), node('b', 500, 500, 10, 10)], id => id === 'b')).toEqual({ x: 500, y: 500, width: 10, height: 10 });
  });

  it('prefers the measured size', () => {
    const n = { ...node('a', 0, 0, 10, 10), measured: { width: 30, height: 40 } };
    expect(boundsOf([n])).toEqual({ x: 0, y: 0, width: 30, height: 40 });
  });

  it('none picked: null', () => {
    expect(boundsOf([])).toBeNull();
    expect(boundsOf([node('a', 0, 0, 1, 1)], () => false)).toBeNull();
  });
});
